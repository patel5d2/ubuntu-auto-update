package ssh

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"regexp"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
)

// BootstrapResult is everything Bootstrap discovered or generated. The
// caller stores PrivateKeyPEM (encrypted) in the DB; HostKey is appended to
// known_hosts; SudoConfigured is informational so the UI can warn if
// passwordless sudo couldn't be set up (e.g. an /etc/sudoers.d/ already
// pinned the user to a different rule).
type BootstrapResult struct {
	PrivateKeyPEM  string
	AuthorizedKey  string
	HostKey        gossh.PublicKey
	SudoConfigured bool
}

// Bootstrap runs the one-shot enrollment dance against a host:
//   1. SSH in with password auth, capturing the host key (TOFU).
//   2. Generate a fresh ed25519 keypair.
//   3. Append the public key to ~/.ssh/authorized_keys.
//   4. For non-root users, write a sudoers drop-in granting passwordless
//      sudo, and verify with `visudo -cf` so a syntax error doesn't lock
//      the operator out.
//   5. Reconnect using the new key (no password) and confirm sudo -n
//      works. Returning success is proof the rest of the system can SSH
//      to this host without ever seeing the password again.
//
// The password is held only in memory for the duration of this call and
// never logged or persisted. Stdin pipes are constructed so the password
// isn't visible in `ps` either.
//
// hostname is used both for the TCP dial and the known_hosts entry.
// Callers should pass the same value they store in hosts.hostname so
// later key-based dials look the entry up correctly.
func (d *Dialer) Bootstrap(ctx context.Context, hostname, sshUser, password string) (BootstrapResult, error) {
	hostname = strings.TrimSpace(hostname)
	sshUser = strings.TrimSpace(sshUser)
	if hostname == "" || sshUser == "" || password == "" {
		return BootstrapResult{}, errors.New("hostname, ssh_user, and password are all required")
	}

	addr := hostname
	if !strings.Contains(hostname, ":") {
		addr = hostname + ":22"
	}

	// 1) Generate the new keypair up-front so we can install it during the
	//    one and only password-auth session.
	pubBytes, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("generate keypair: %w", err)
	}
	sshPub, err := gossh.NewPublicKey(pubBytes)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("derive ssh public key: %w", err)
	}
	authorizedKey := strings.TrimRight(string(gossh.MarshalAuthorizedKey(sshPub)), "\n")

	pemBlock, err := gossh.MarshalPrivateKey(privKey, "ubuntu-auto-update")
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("marshal private key: %w", err)
	}
	privPEM := string(pem.EncodeToMemory(pemBlock))

	// 2) Password-auth dial with TOFU host-key capture.
	var capturedKey gossh.PublicKey
	cfg := &gossh.ClientConfig{
		User: sshUser,
		Auth: []gossh.AuthMethod{gossh.Password(password)},
		HostKeyCallback: func(_ string, _ net.Addr, key gossh.PublicKey) error {
			capturedKey = key
			return nil
		},
		Timeout: dialTimeout,
	}

	dialCtx, cancel := context.WithTimeout(ctx, dialTimeout)
	defer cancel()
	client, err := dialContext(dialCtx, addr, cfg)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("password ssh dial: %w", classifyAuthErr(err))
	}
	defer client.Close()

	if capturedKey == nil {
		return BootstrapResult{}, errors.New("host key was not presented during ssh handshake")
	}

	// 3) Install the authorized key. The grep-then-append pattern makes the
	//    command idempotent — re-bootstrapping a host doesn't duplicate the
	//    line, which would otherwise compound on every retry.
	installScript := fmt.Sprintf(`set -e
mkdir -p "$HOME/.ssh"
chmod 700 "$HOME/.ssh"
touch "$HOME/.ssh/authorized_keys"
if ! grep -qxF %s "$HOME/.ssh/authorized_keys"; then
  printf '%%s\n' %s >> "$HOME/.ssh/authorized_keys"
fi
chmod 600 "$HOME/.ssh/authorized_keys"
`, shellQuote(authorizedKey), shellQuote(authorizedKey))

	if out, err := runCommand(client, installScript, nil); err != nil {
		return BootstrapResult{}, fmt.Errorf("install authorized_keys: %w (output: %s)", err, trimTo(out, 400))
	}

	// 4) For non-root users, configure passwordless sudo.
	sudoConfigured := sshUser == "root"
	if sshUser != "root" {
		sudoersFile := "uau-" + sanitizeForFilename(sshUser)
		sudoersContent := fmt.Sprintf("%s ALL=(ALL) NOPASSWD: ALL\n", sshUser)

		// We pipe (password + "\n" + sudoersContent) into `sudo -S sh -c '<cmd>'`.
		// sudo -S reads the password from the first line of stdin, strips it,
		// and forwards the rest to the child process. The child reads from
		// the same stdin via cat redirection. -p '' silences the prompt so
		// stderr stays clean. visudo -cf guards against syntax errors that
		// would otherwise lock root-via-sudo out of the box.
		innerCmd := fmt.Sprintf(
			"umask 077 && cat > /etc/sudoers.d/%s && chmod 0440 /etc/sudoers.d/%s && visudo -cf /etc/sudoers.d/%s",
			sudoersFile, sudoersFile, sudoersFile,
		)
		cmd := fmt.Sprintf("sudo -S -p '' sh -c %s", shellQuote(innerCmd))

		stdin := io.MultiReader(
			strings.NewReader(password+"\n"),
			strings.NewReader(sudoersContent),
		)
		if out, err := runCommand(client, cmd, stdin); err != nil {
			return BootstrapResult{}, fmt.Errorf("configure passwordless sudo: %w (output: %s)", err, trimTo(out, 400))
		}
		sudoConfigured = true
	}

	// 5) Verify everything sticks: reconnect using the new key, run sudo -n.
	signer, err := gossh.ParsePrivateKey([]byte(privPEM))
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("parse generated private key: %w", err)
	}
	verifyCfg := &gossh.ClientConfig{
		User:            sshUser,
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.FixedHostKey(capturedKey),
		Timeout:         dialTimeout,
	}
	verifyCtx, verifyCancel := context.WithTimeout(ctx, dialTimeout)
	defer verifyCancel()
	verifyClient, err := dialContext(verifyCtx, addr, verifyCfg)
	if err != nil {
		return BootstrapResult{}, fmt.Errorf("verify key auth: %w", err)
	}
	defer verifyClient.Close()

	if sshUser != "root" {
		s, err := verifyClient.NewSession()
		if err != nil {
			return BootstrapResult{}, fmt.Errorf("verify session: %w", err)
		}
		err = s.Run("sudo -n true")
		s.Close()
		if err != nil {
			return BootstrapResult{}, fmt.Errorf("verify passwordless sudo: %w", err)
		}
	}

	return BootstrapResult{
		PrivateKeyPEM:  privPEM,
		AuthorizedKey:  authorizedKey,
		HostKey:        capturedKey,
		SudoConfigured: sudoConfigured,
	}, nil
}

// AppendKnownHost adds a single host-key line and invalidates the cached
// known_hosts callback so the next regular SSH dial picks up the entry
// without a backend restart.
func (d *Dialer) AppendKnownHost(hostname string, key gossh.PublicKey) error {
	path := os.Getenv("KNOWN_HOSTS_FILE")
	if path == "" {
		path = "known_hosts"
	}

	line := knownhosts.Line([]string{hostname}, key)

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("open known_hosts: %w", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		return fmt.Errorf("write known_hosts: %w", err)
	}

	d.invalidateHostKeyCache()
	return nil
}

// dialContext threads ctx through gossh.Dial. The stdlib SSH client doesn't
// take a ctx directly (it predates the convention), so we rely on the
// dialer's Timeout for the handshake and use ctx only to wrap a generic
// network dial. This is enough to avoid hung connects on bad IPs.
func dialContext(ctx context.Context, addr string, cfg *gossh.ClientConfig) (*gossh.Client, error) {
	d := net.Dialer{Timeout: cfg.Timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	c, chans, reqs, err := gossh.NewClientConn(conn, addr, cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}
	return gossh.NewClient(c, chans, reqs), nil
}

// runCommand executes one shell command on the existing client and returns
// the combined stdout/stderr. We don't stream — bootstrap commands are
// short and deterministic.
func runCommand(client *gossh.Client, cmd string, stdin io.Reader) ([]byte, error) {
	session, err := client.NewSession()
	if err != nil {
		return nil, fmt.Errorf("new session: %w", err)
	}
	defer session.Close()
	if stdin != nil {
		session.Stdin = stdin
	}
	// 60 s is plenty for any single bootstrap step. The outer ctx will
	// also cancel us if the operator gives up.
	t := time.AfterFunc(60*time.Second, func() { _ = session.Signal(gossh.SIGKILL) })
	defer t.Stop()
	return session.CombinedOutput(cmd)
}

// classifyAuthErr converts the common "ssh: handshake failed" wrapper into
// a more actionable message for the UI. We don't pretend to enumerate
// every failure mode — better an OK string than a misleading taxonomy.
func classifyAuthErr(err error) error {
	msg := err.Error()
	switch {
	case strings.Contains(msg, "unable to authenticate"),
		strings.Contains(msg, "no supported methods remain"):
		return errors.New("authentication failed — wrong password, wrong SSH user, or password auth disabled on the host (PasswordAuthentication no)")
	case strings.Contains(msg, "connection refused"):
		return errors.New("connection refused — is sshd running on port 22?")
	case strings.Contains(msg, "no route to host"),
		strings.Contains(msg, "i/o timeout"),
		strings.Contains(msg, "deadline exceeded"):
		return errors.New("could not reach host — check the hostname/IP and that the backend container can route to it")
	default:
		return err
	}
}

var filenameSanitize = regexp.MustCompile(`[^a-zA-Z0-9._-]`)

func sanitizeForFilename(s string) string {
	out := filenameSanitize.ReplaceAllString(s, "_")
	if out == "" {
		return "user"
	}
	return out
}

// shellQuote produces a single-quoted shell literal that's safe to embed
// in any POSIX shell. The escape inside quotes is `'\''` — close, escaped
// single quote, reopen.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// trimTo limits a byte slice's length so error messages don't leak entire
// shell sessions back to the API client.
func trimTo(b []byte, n int) string {
	s := strings.TrimSpace(string(b))
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
