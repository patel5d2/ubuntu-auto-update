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

// DBTX is an interface covering pgxpool.Pool methods used in this package.
type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Begin(ctx context.Context) (pgx.Tx, error)
	Ping(ctx context.Context) error
}

const hostColumns = `id, hostname, ssh_user, created_at, updated_at, last_seen, update_output, upgrade_output, error, tags, reboot_required, packages_updated, packages_available, os_version, kernel_version, agent_version, offline_since`

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

// ReportData carries the persistable fields of an agent report into UpsertHost.
type ReportData struct {
	UpdateOutput      string
	UpgradeOutput     string
	Error             string
	RebootRequired    bool
	PackagesUpdated   int
	PackagesAvailable int
	OsVersion         string
	KernelVersion     string
	AgentVersion      string
}

// UpsertHost records an agent report. On INSERT it seeds ssh_user; on CONFLICT
// it deliberately does NOT touch ssh_user — a report used to clobber it back to
// "root", breaking SSH for hosts enrolled as a non-root user. sshUser is only
// consulted for the initial insert.
func UpsertHost(ctx context.Context, db DBTX, hostname, sshUser string, r ReportData) (models.Host, error) {
	var hostError sql.NullString
	if r.Error != "" {
		hostError = sql.NullString{String: r.Error, Valid: true}
	}

	rows, err := db.Query(ctx, `
		INSERT INTO hosts (hostname, ssh_user, last_seen, update_output, upgrade_output, error,
		                   reboot_required, packages_updated, packages_available,
		                   os_version, kernel_version, agent_version)
		VALUES ($1, $2, NOW(), $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (hostname) DO UPDATE
		SET last_seen = NOW(),
		    update_output = $3,
		    upgrade_output = $4,
		    error = $5,
		    reboot_required = $6,
		    packages_updated = $7,
		    packages_available = $8,
		    os_version = $9,
		    kernel_version = $10,
		    agent_version = $11
		RETURNING `+hostColumns,
		hostname, sshUser, r.UpdateOutput, r.UpgradeOutput, hostError,
		r.RebootRequired, r.PackagesUpdated, r.PackagesAvailable,
		r.OsVersion, r.KernelVersion, r.AgentVersion)
	if err != nil {
		return models.Host{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Host])
}

func ListHosts(ctx context.Context, db DBTX) ([]models.Host, error) {
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

// SweepOfflineHosts is the server-side offline detector. It first clears the
// flag for hosts that have reported again, then flags hosts whose last_seen
// crossed the threshold, returning only the newly-flagged rows so the caller
// dispatches host_offline exactly once per transition.
func SweepOfflineHosts(ctx context.Context, db DBTX, thresholdMinutes int) ([]models.Host, error) {
	if _, err := db.Exec(ctx, `
		UPDATE hosts SET offline_since = NULL
		WHERE offline_since IS NOT NULL AND last_seen >= NOW() - make_interval(mins => $1)`,
		thresholdMinutes); err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, `
		UPDATE hosts SET offline_since = NOW()
		WHERE offline_since IS NULL AND last_seen < NOW() - make_interval(mins => $1)
		RETURNING `+hostColumns,
		thresholdMinutes)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[models.Host])
}

// ListHostsPage is the paginated variant for API/automation consumers.
func ListHostsPage(ctx context.Context, db DBTX, limit, offset int) ([]models.Host, error) {
	rows, err := db.Query(ctx,
		`SELECT `+hostColumns+` FROM hosts ORDER BY hostname LIMIT $1 OFFSET $2`,
		limit, offset)
	if err != nil {
		return nil, err
	}
	hosts, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.Host])
	if err != nil {
		return nil, err
	}
	if hosts == nil {
		hosts = []models.Host{}
	}
	return hosts, nil
}

// CreateHost inserts a new host record. Returns ErrDuplicateHostname if a
// row with the same hostname already exists. Use UpsertHost only from the
// agent-report path; operator-driven creation should be strict.
//
// pgx v5 may defer the underlying SQL error from Query() until row
// collection runs, so we check both code paths.
func CreateHost(ctx context.Context, db DBTX, hostname, sshUser string) (models.Host, error) {
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
func UpdateHostSSHUser(ctx context.Context, db DBTX, id int32, sshUser string) (models.Host, error) {
	rows, err := db.Query(ctx, `
		UPDATE hosts SET ssh_user = $2 WHERE id = $1
		RETURNING `+hostColumns,
		id, sshUser)
	if err != nil {
		return models.Host{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Host])
}

// UpdateHostTags replaces the host's tag list. Returns pgx.ErrNoRows if no
// row matches.
func UpdateHostTags(ctx context.Context, db DBTX, id int32, tags []string) (models.Host, error) {
	if tags == nil {
		tags = []string{}
	}
	rows, err := db.Query(ctx, `
		UPDATE hosts SET tags = $2, updated_at = NOW() WHERE id = $1
		RETURNING `+hostColumns,
		id, tags)
	if err != nil {
		return models.Host{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Host])
}

// DeleteHost removes the host row. ssh_keys is set to ON DELETE CASCADE in
// the schema, so the encrypted key disappears with it. Returns the number
// of rows affected so the handler can distinguish 404 from success.
func DeleteHost(ctx context.Context, db DBTX, id int32) (int64, error) {
	tag, err := db.Exec(ctx, `DELETE FROM hosts WHERE id = $1`, id)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func GetHost(ctx context.Context, db DBTX, id int32) (models.Host, error) {
	rows, err := db.Query(ctx, `SELECT `+hostColumns+` FROM hosts WHERE id = $1`, id)
	if err != nil {
		return models.Host{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.Host])
}

func GetSSHKey(ctx context.Context, db DBTX, hostID int32) (models.SSHKey, error) {
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

func AddSSHKey(ctx context.Context, db DBTX, hostID int32, privateKey string) error {
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
func SetSSHKeyAndUser(ctx context.Context, db DBTX, hostID int32, sshUser, privateKey string) error {
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

// ListAllWebhooks returns every webhook subscription, for the Settings UI.
func ListAllWebhooks(ctx context.Context, db DBTX) ([]models.Webhook, error) {
	rows, err := db.Query(ctx, `SELECT id, url, event FROM webhooks ORDER BY id`)
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

// DeleteWebhook removes a subscription by id, returning rows affected so the
// handler can distinguish 404 from success.
func DeleteWebhook(ctx context.Context, db DBTX, id int32) (int64, error) {
	tag, err := db.Exec(ctx, `DELETE FROM webhooks WHERE id = $1`, id)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func GetWebhooks(ctx context.Context, db DBTX, event string) ([]models.Webhook, error) {
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
