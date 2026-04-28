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
// every connection.
type Dialer struct {
	pool        *pgxpool.Pool
	hostKeyOnce sync.Once
	hostKeyCB   ssh.HostKeyCallback
	hostKeyErr  error
}

func NewDialer(pool *pgxpool.Pool) *Dialer {
	return &Dialer{pool: pool}
}

// hostKeyCallback returns a cached known_hosts callback. The known_hosts path
// can be overridden with KNOWN_HOSTS_FILE; defaults to ./known_hosts.
func (d *Dialer) hostKeyCallback() (ssh.HostKeyCallback, error) {
	d.hostKeyOnce.Do(func() {
		path := os.Getenv("KNOWN_HOSTS_FILE")
		if path == "" {
			path = "known_hosts"
		}
		d.hostKeyCB, d.hostKeyErr = knownhosts.New(path)
	})
	return d.hostKeyCB, d.hostKeyErr
}

// invalidateHostKeyCache forces the next ConnectToHost call to re-read
// known_hosts. Used after Bootstrap appends a TOFU-captured host key so
// the operator doesn't have to restart the backend before the host
// becomes usable. Field writes are protected by hostKeyOnce's reset
// pattern; safe under the assumption that bootstrap is rare and not
// concurrent with itself.
func (d *Dialer) invalidateHostKeyCache() {
	d.hostKeyOnce = sync.Once{}
	d.hostKeyCB = nil
	d.hostKeyErr = nil
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
