package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"ubuntu-auto-update/backend/pkg/crypto"
	"ubuntu-auto-update/backend/pkg/models"
)

// ErrDuplicateHostname is returned when CreateHost hits a unique constraint
// on hostname (Postgres error 23505).
var ErrDuplicateHostname = errors.New("hostname already exists")

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

// CreateHost inserts a new host record. Returns ErrDuplicateHostname if a
// row with the same hostname already exists. Use UpsertHost only from the
// agent-report path; operator-driven creation should be strict.
//
// pgx v5 may defer the underlying SQL error from Query() until row
// collection runs, so we check both code paths.
func CreateHost(ctx context.Context, db *pgxpool.Pool, hostname, sshUser string) (models.Host, error) {
	rows, err := db.Query(ctx, `
		INSERT INTO hosts (hostname, ssh_user, last_seen, update_output, upgrade_output)
		VALUES ($1, $2, NOW(), '', '')
		RETURNING `+hostColumns,
		hostname, sshUser)
	if err != nil {
		return models.Host{}, mapInsertHostError(err)
	}
	host, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Host])
	if err != nil {
		return models.Host{}, mapInsertHostError(err)
	}
	return host, nil
}

func mapInsertHostError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrDuplicateHostname
	}
	return err
}

// UpdateHostSSHUser updates only the ssh_user column. Returns pgx.ErrNoRows
// if no row matches.
func UpdateHostSSHUser(ctx context.Context, db *pgxpool.Pool, id int32, sshUser string) (models.Host, error) {
	rows, err := db.Query(ctx, `
		UPDATE hosts SET ssh_user = $2 WHERE id = $1
		RETURNING `+hostColumns,
		id, sshUser)
	if err != nil {
		return models.Host{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Host])
}

// DeleteHost removes the host row. ssh_keys is set to ON DELETE CASCADE in
// the schema, so the encrypted key disappears with it. Returns the number
// of rows affected so the handler can distinguish 404 from success.
func DeleteHost(ctx context.Context, db *pgxpool.Pool, id int32) (int64, error) {
	tag, err := db.Exec(ctx, `DELETE FROM hosts WHERE id = $1`, id)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
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

// SetSSHKeyAndUser stores the SSH key and updates the host's ssh_user in a
// single transaction. The previous two-step path could leave the new key
// paired with the old ssh_user if the second statement failed.
func SetSSHKeyAndUser(ctx context.Context, db *pgxpool.Pool, hostID int32, sshUser, privateKey string) error {
	encryptedKey, err := crypto.Encrypt(privateKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt SSH key: %w", err)
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	// Rollback is a no-op after a successful Commit, so we always defer it.
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		INSERT INTO ssh_keys (host_id, private_key)
		VALUES ($1, $2)
		ON CONFLICT (host_id) DO UPDATE
		SET private_key = $2
	`, hostID, encryptedKey); err != nil {
		return fmt.Errorf("upsert ssh_key: %w", err)
	}

	tag, err := tx.Exec(ctx, `UPDATE hosts SET ssh_user = $1, updated_at = NOW() WHERE id = $2`, sshUser, hostID)
	if err != nil {
		return fmt.Errorf("update ssh_user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	return tx.Commit(ctx)
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
