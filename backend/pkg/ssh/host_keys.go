// DB-backed host-key store. Replaces the on-disk known_hosts file: reads
// expected fingerprints from the host_keys table and refuses any handshake
// whose key isn't on file. Multiple backend replicas can share the same
// host_keys table without coordinating writes.
//
// Bootstrap still TOFU-captures the first key it sees, but it now goes to
// the DB instead of a local file — so a different backend replica can verify
// the same fingerprint immediately.

package ssh

import (
	"context"
	"fmt"
	"net"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	gossh "golang.org/x/crypto/ssh"
)

// SaveHostKey upserts a (hostname, fingerprint) pair. Idempotent — repeated
// calls with the same fingerprint do not produce duplicates thanks to the
// UNIQUE (hostname, fingerprint_sha256) constraint.
func SaveHostKey(ctx context.Context, pool *pgxpool.Pool, hostname string, key gossh.PublicKey) error {
	keyLine := string(gossh.MarshalAuthorizedKey(key))
	fingerprint := gossh.FingerprintSHA256(key)
	_, err := pool.Exec(ctx, `
		INSERT INTO host_keys (hostname, key_line, fingerprint_sha256)
		VALUES ($1, $2, $3)
		ON CONFLICT (hostname, fingerprint_sha256) DO NOTHING`,
		hostname, keyLine, fingerprint,
	)
	if err != nil {
		return fmt.Errorf("save host key: %w", err)
	}
	return nil
}

// dbHostKeyCallback returns a callback that accepts any key whose SHA-256
// fingerprint is registered for the dialled hostname. Empty result set =
// host has no recorded key, which we treat as a hard failure.
func (d *Dialer) dbHostKeyCallback() gossh.HostKeyCallback {
	return func(hostname string, _ net.Addr, key gossh.PublicKey) error {
		// gossh strips the port and brackets. We store hostnames in the same
		// shape (the human-typed value), so this matches.
		expected := gossh.FingerprintSHA256(key)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var count int
		err := d.pool.QueryRow(ctx, `
			SELECT COUNT(*) FROM host_keys
			WHERE hostname = $1 AND fingerprint_sha256 = $2`,
			stripPort(hostname), expected,
		).Scan(&count)
		if err != nil {
			return fmt.Errorf("host_keys lookup: %w", err)
		}
		if count == 0 {
			return fmt.Errorf("host key for %s (%s) is not in host_keys; refusing connection", hostname, expected)
		}
		return nil
	}
}

func stripPort(host string) string {
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}
