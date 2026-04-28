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
