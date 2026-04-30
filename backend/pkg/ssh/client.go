// Package ssh wraps golang.org/x/crypto/ssh with the project's host-key
// verification policy and DB-backed key lookups.
package ssh

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"ubuntu-auto-update/backend/pkg/db"
	"ubuntu-auto-update/backend/pkg/models"
)

const dialTimeout = 30 * time.Second

// Dialer holds a cached host-key callback so we don't reread known_hosts on
// every connection. When the source is the on-disk known_hosts file the
// callback genuinely is a one-time load; for the DB-backed source the
// "cache" is just the closure pointer.
type Dialer struct {
	pool       *pgxpool.Pool
	hostKeyMu  sync.RWMutex
	hostKeyCB  ssh.HostKeyCallback
	hostKeyErr error
	hostKeyOK  bool
}

func NewDialer(pool *pgxpool.Pool) *Dialer {
	return &Dialer{pool: pool}
}

// hostKeyCallback returns the configured host-key callback.
//
// HOST_KEY_STORE selects the source:
//   - "db"   (default when HOST_KEY_STORE is unset and a pool is available)
//            uses the host_keys table from migration 000013, so all backend
//            replicas share the same view of fingerprints.
//   - "file" reads the on-disk known_hosts file at KNOWN_HOSTS_FILE
//            (default ./known_hosts) — kept as an escape hatch for legacy
//            deployments and for offline testing.
//
// Concurrency: a mutex guards the cached callback rather than sync.Once
// because invalidateHostKeyCache needs to swap the cache atomically when
// Bootstrap captures a new TOFU key. Reassigning a sync.Once would itself
// be a data race against in-flight callers.
func (d *Dialer) hostKeyCallback() (ssh.HostKeyCallback, error) {
	d.hostKeyMu.RLock()
	if d.hostKeyOK {
		cb, err := d.hostKeyCB, d.hostKeyErr
		d.hostKeyMu.RUnlock()
		return cb, err
	}
	d.hostKeyMu.RUnlock()

	d.hostKeyMu.Lock()
	defer d.hostKeyMu.Unlock()
	// Re-check under write lock — another goroutine may have populated it.
	if d.hostKeyOK {
		return d.hostKeyCB, d.hostKeyErr
	}

	mode := os.Getenv("HOST_KEY_STORE")
	if mode == "" {
		if d.pool != nil {
			mode = "db"
		} else {
			mode = "file"
		}
	}
	switch mode {
	case "db":
		d.hostKeyCB = d.dbHostKeyCallback()
	case "file":
		path := os.Getenv("KNOWN_HOSTS_FILE")
		if path == "" {
			path = "known_hosts"
		}
		d.hostKeyCB, d.hostKeyErr = knownhosts.New(path)
	default:
		d.hostKeyErr = fmt.Errorf("unknown HOST_KEY_STORE %q (want \"db\" or \"file\")", mode)
	}
	d.hostKeyOK = true
	return d.hostKeyCB, d.hostKeyErr
}

// invalidateHostKeyCache forces the next ConnectToHost call to re-read
// known_hosts. Used after Bootstrap appends a TOFU-captured host key so
// the operator doesn't have to restart the backend before the host
// becomes usable.
func (d *Dialer) invalidateHostKeyCache() {
	d.hostKeyMu.Lock()
	d.hostKeyCB = nil
	d.hostKeyErr = nil
	d.hostKeyOK = false
	d.hostKeyMu.Unlock()
}

// TestResult summarizes a quick health probe: did SSH dial succeed, how long
// did the round trip take, and is passwordless sudo available (relevant for
// non-root ssh users since apt-get upgrade needs it).
type TestResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latency_ms"`
	SudoState string `json:"sudo_state"` // "root", "available", "unavailable", "n/a"
	Greeting  string `json:"greeting"`
	Error     string `json:"error,omitempty"`
}

// TestConnection dials the host, runs a fast no-op (and `sudo -n true` for
// non-root users), and returns timing. Exists primarily so the operator UI
// can verify a host is reachable before triggering a real update.
func (d *Dialer) TestConnection(ctx context.Context, hostID int32) (TestResult, error) {
	start := time.Now()
	client, host, err := d.ConnectToHost(ctx, hostID)
	if err != nil {
		return TestResult{OK: false, Error: err.Error()}, nil
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return TestResult{OK: false, Error: "open session: " + err.Error()}, nil
	}
	defer session.Close()

	greeting, err := session.CombinedOutput("echo ubuntu-auto-update-ok && uname -sr")
	if err != nil {
		return TestResult{OK: false, Error: "exec probe: " + err.Error()}, nil
	}

	res := TestResult{
		OK:        true,
		LatencyMs: time.Since(start).Milliseconds(),
		Greeting:  string(greeting),
		SudoState: "root",
	}

	if host.SshUser != "" && host.SshUser != "root" {
		// Run sudo -n true on a fresh session; the previous one is closed already.
		s2, err := client.NewSession()
		if err != nil {
			res.SudoState = "unavailable"
			return res, nil
		}
		defer s2.Close()
		if err := s2.Run("sudo -n true"); err != nil {
			res.SudoState = "unavailable"
		} else {
			res.SudoState = "available"
		}
	}

	return res, nil
}

// ConnectToHost looks up the host + decrypted SSH key by ID and opens a client.
// Caller is responsible for closing the returned client.
func (d *Dialer) ConnectToHost(ctx context.Context, hostID int32) (*ssh.Client, models.Host, error) {
	host, err := db.GetHost(ctx, d.pool, hostID)
	if err != nil {
		return nil, models.Host{}, fmt.Errorf("get host: %w", err)
	}

	key, err := db.GetSSHKey(ctx, d.pool, hostID)
	if err != nil {
		return nil, host, fmt.Errorf("get ssh key: %w", err)
	}

	signer, err := ssh.ParsePrivateKey([]byte(key.PrivateKey))
	if err != nil {
		return nil, host, fmt.Errorf("parse private key: %w", err)
	}

	hostKeyCB, err := d.hostKeyCallback()
	if err != nil {
		return nil, host, fmt.Errorf("load known_hosts: %w", err)
	}

	cfg := &ssh.ClientConfig{
		User:            host.SshUser,
		Auth:            []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: hostKeyCB,
		Timeout:         dialTimeout,
	}
	client, err := ssh.Dial("tcp", host.Hostname+":22", cfg)
	if err != nil {
		return nil, host, fmt.Errorf("dial ssh: %w", err)
	}
	return client, host, nil
}
