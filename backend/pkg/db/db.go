package db

import (
	"context"
	"database/sql"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"ubuntu-auto-update/backend/pkg/crypto"
	"ubuntu-auto-update/backend/pkg/models"
)

const hostColumns = `id, hostname, ssh_user, created_at, updated_at, last_seen, update_output, upgrade_output, error`

func NewConnection(ctx context.Context) (*pgxpool.Pool, error) {
	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		return nil, fmt.Errorf("DATABASE_URL environment variable not set")
	}
	pool, err := pgxpool.New(ctx, dbUrl)
	if err != nil {
		return nil, fmt.Errorf("unable to connect to database: %w", err)
	}
	return pool, nil
}

func UpsertHost(ctx context.Context, db *pgxpool.Pool, hostname, sshUser, updateOutput, upgradeOutput, errorMsg string) (models.Host, error) {
	var hostError sql.NullString
	if errorMsg != "" {
		hostError = sql.NullString{String: errorMsg, Valid: true}
	}

	rows, err := db.Query(ctx, `
		INSERT INTO hosts (hostname, ssh_user, last_seen, update_output, upgrade_output, error)
		VALUES ($1, $2, NOW(), $3, $4, $5)
		ON CONFLICT (hostname) DO UPDATE
		SET last_seen = NOW(),
		    ssh_user = $2,
		    update_output = $3,
		    upgrade_output = $4,
		    error = $5
		RETURNING `+hostColumns,
		hostname, sshUser, updateOutput, upgradeOutput, hostError)
	if err != nil {
		return models.Host{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Host])
}

func ListHosts(ctx context.Context, db *pgxpool.Pool) ([]models.Host, error) {
	rows, err := db.Query(ctx, `SELECT `+hostColumns+` FROM hosts ORDER BY hostname`)
	if err != nil {
		return nil, err
	}
	hosts, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Host])
	if err != nil {
		return nil, err
	}
	if hosts == nil {
		hosts = []models.Host{} // avoid `null` in JSON
	}
	return hosts, nil
}

func GetHost(ctx context.Context, db *pgxpool.Pool, id int32) (models.Host, error) {
	rows, err := db.Query(ctx, `SELECT `+hostColumns+` FROM hosts WHERE id = $1`, id)
	if err != nil {
		return models.Host{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Host])
}

func GetSSHKey(ctx context.Context, db *pgxpool.Pool, hostID int32) (models.SSHKey, error) {
	rows, err := db.Query(ctx, `SELECT host_id, private_key FROM ssh_keys WHERE host_id = $1`, hostID)
	if err != nil {
		return models.SSHKey{}, err
	}
	key, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.SSHKey])
	if err != nil {
		return models.SSHKey{}, err
	}

	decrypted, err := crypto.Decrypt(key.PrivateKey)
	if err != nil {
		return models.SSHKey{}, fmt.Errorf("failed to decrypt SSH key for host %d: %w", hostID, err)
	}
	key.PrivateKey = decrypted
	return key, nil
}

func AddSSHKey(ctx context.Context, db *pgxpool.Pool, hostID int32, privateKey string) error {
	encryptedKey, err := crypto.Encrypt(privateKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt SSH key: %w", err)
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
	hooks, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Webhook])
	if err != nil {
		return nil, err
	}
	if hooks == nil {
		hooks = []models.Webhook{}
	}
	return hooks, nil
}
