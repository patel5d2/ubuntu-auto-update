package ssh

import (
	"crypto/ed25519"
	"crypto/rand"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"
)

// formatAuthorizedKey must produce a line whose last whitespace-separated
// token is `authorizedKeyMarker`. RotateKey's awk depends on this — without
// the marker, we cannot tell which keys we own when revoking.
func TestFormatAuthorizedKey_HasMarker(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	line := formatAuthorizedKey(sshPub)
	if strings.Contains(line, "\n") {
		t.Errorf("authorized key line must not contain newline: %q", line)
	}
	parts := strings.Fields(line)
	if len(parts) < 3 {
		t.Fatalf("expected at least <algo> <base64> <marker>; got %v", parts)
	}
	if parts[len(parts)-1] != authorizedKeyMarker {
		t.Errorf("last field should be the marker %q; got %q (full: %q)", authorizedKeyMarker, parts[len(parts)-1], line)
	}
}

// shellQuote is on the hot path during bootstrap — verify single-quote
// escaping survives the obvious adversarial inputs.
func TestShellQuote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello", "'hello'"},
		{"", "''"},
		{"it's", `'it'\''s'`},
		{"a 'b' c", `'a '\''b'\'' c'`},
	}
	for _, c := range cases {
		got := shellQuote(c.in)
		if got != c.want {
			t.Errorf("shellQuote(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestSanitizeForFilename(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"alice", "alice"},
		{"al ice", "al_ice"},
		{"../etc/passwd", ".._etc_passwd"},
		{"", "user"},
		{"!@#$%", "_____"},
	}
	for _, c := range cases {
		if got := sanitizeForFilename(c.in); got != c.want {
			t.Errorf("sanitizeForFilename(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestScopedSudoersBody(t *testing.T) {
	// Default ("apt") locks the user to apt commands only.
	if body := scopedSudoersBody("nginx", "apt"); !strings.Contains(body, "/usr/bin/apt-get") || strings.Contains(body, "NOPASSWD: ALL") {
		t.Errorf("apt scope should restrict to apt; got %q", body)
	}
	// Full opens up everything.
	if body := scopedSudoersBody("nginx", "full"); !strings.Contains(body, "NOPASSWD: ALL") {
		t.Errorf("full scope should grant ALL; got %q", body)
	}
}

func TestClassifyAuthErr(t *testing.T) {
	cases := []struct {
		raw, contains string
	}{
		{"ssh: handshake failed: ssh: unable to authenticate", "authentication failed"},
		{"ssh: handshake failed: no supported methods remain", "authentication failed"},
		{"dial tcp 1.2.3.4:22: connect: connection refused", "connection refused"},
		{"dial tcp 1.2.3.4:22: i/o timeout", "could not reach"},
		{"some other error", "some other error"},
	}
	for _, c := range cases {
		out := classifyAuthErr(errString(c.raw)).Error()
		if !strings.Contains(out, c.contains) {
			t.Errorf("classifyAuthErr(%q) = %q, want contains %q", c.raw, out, c.contains)
		}
	}
}

type errString string

func (e errString) Error() string { return string(e) }
