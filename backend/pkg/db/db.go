package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"
	"ubuntu-auto-update/backend/pkg/crypto"
	"ubuntu-auto-update/backend/pkg/models"
)

func NewConnection(ctx context.Context) (*pgxpool.Pool, error) {
	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable not set")
	}

	pool, err := pgxpool.New(ctx, dbUrl)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %v", err)
	}

	return pool, nil
}

func UpsertHost(ctx context.Context, db *pgxpool.Pool, hostname string, sshUser string, updateOutput string, upgradeOutput string, errorMsg string) (models.Host, error) {
	var host models.Host
	var error sql.NullString
	if errorMsg != "" {
		error.String = errorMsg
		error.Valid = true
	}
	err := db.QueryRow(ctx, `
		INSERT INTO hosts (hostname, ssh_user, last_seen, update_output, upgrade_output, error)
		VALUES ($1, $2, NOW(), $3, $4, $5)
		ON CONFLICT (hostname) DO UPDATE
		SET last_seen = NOW(),
		    ssh_user = $2,
		    update_output = $3,
		    upgrade_output = $4,
		    error = $5
		RETURNING id, hostname, ssh_user, created_at, updated_at, last_seen, update_output, upgrade_output, error
	`, hostname, sshUser, updateOutput, upgradeOutput, error).Scan(&host.ID, &host.Hostname, &host.SshUser, &host.CreatedAt, &host.UpdatedAt, &host.LastSeen, &host.UpdateOutput, &host.UpgradeOutput, &host.Error)
	return host, err
}

func ListHosts(ctx context.Context, db *pgxpool.Pool) ([]models.Host, error) {
	rows, err := db.Query(ctx, `SELECT id, hostname, ssh_user, created_at, updated_at, last_seen, update_output, upgrade_output, error FROM hosts ORDER BY hostname`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var hosts []models.Host
	for rows.Next() {
		var host models.Host
		if err := rows.Scan(&host.ID, &host.Hostname, &host.SshUser, &host.CreatedAt, &host.UpdatedAt, &host.LastSeen, &host.UpdateOutput, &host.UpgradeOutput, &host.Error); err != nil {
			return nil, err
		}
		hosts = append(hosts, host)
	}

	return hosts, nil
}

func GetHost(ctx context.Context, db *pgxpool.Pool, id int32) (models.Host, error) {
	var host models.Host
	err := db.QueryRow(ctx, `SELECT id, hostname, ssh_user, created_at, updated_at, last_seen, update_output, upgrade_output, error FROM hosts WHERE id = $1`, id).Scan(&host.ID, &host.Hostname, &host.SshUser, &host.CreatedAt, &host.UpdatedAt, &host.LastSeen, &host.UpdateOutput, &host.UpgradeOutput, &host.Error)
	return host, err
}

func GetSSHKey(ctx context.Context, db *pgxpool.Pool, hostID int32) (models.SSHKey, error) {
	var key models.SSHKey
	err := db.QueryRow(ctx, `SELECT host_id, private_key FROM ssh_keys WHERE host_id = $1`, hostID).Scan(&key.HostID, &key.PrivateKey)
	if err != nil {
		return models.SSHKey{}, err
	}

	decryptedKey, err := crypto.Decrypt(key.PrivateKey)
	if err != nil {
		return models.SSHKey{}, err
	}

	key.PrivateKey = decryptedKey
	return key, nil
}

func AddSSHKey(ctx context.Context, db *pgxpool.Pool, hostID int32, privateKey string) error {
	encryptedKey, err := crypto.Encrypt(privateKey)
	if err != nil {
		return err
	}

	_, err = db.Exec(ctx, `
		INSERT INTO ssh_keys (host_id, private_key)
		VALUES ($1, $2)
		ON CONFLICT (host_id) DO UPDATE
		SET private_key = $2
	`, hostID, encryptedKey)
	return err
}

func GetWebhooks(ctx context.Context, db *pgxpool.Pool, event string) ([]models.Webhook, error) {
	rows, err := db.Query(ctx, `SELECT id, url, event FROM webhooks WHERE event = $1`, event)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var webhooks []models.Webhook
	for rows.Next() {
		var webhook models.Webhook
		if err := rows.Scan(&webhook.ID, &webhook.URL, &webhook.Event); err != nil {
			return nil, err
		}
		webhooks = append(webhooks, webhook)
	}

	return webhooks, nil
}