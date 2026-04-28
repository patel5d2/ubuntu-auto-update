package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"ubuntu-auto-update/backend/pkg/models"
)

const runColumns = `id, host_id, run_group_id, triggered_by, kind, status, exit_code, started_at, finished_at, output, error`

// MaxRunOutputBytes caps the size of stored output. Long apt logs blow up
// the browser and the DB row otherwise; once the cap is reached we append
// a single truncation marker and stop persisting further writes.
const MaxRunOutputBytes = 1 << 20 // 1 MiB

// CreateRun inserts a new update_runs row in 'running' state and returns
// the generated id.
func CreateRun(ctx context.Context, db *pgxpool.Pool, hostID int32, triggeredBy string, kind models.RunKind) (models.UpdateRun, error) {
	return CreateRunWithGroup(ctx, db, hostID, triggeredBy, kind, "")
}

// CreateRunWithGroup is the bulk-aware variant of CreateRun. Pass groupID =
// "" for single-host runs.
func CreateRunWithGroup(ctx context.Context, db *pgxpool.Pool, hostID int32, triggeredBy string, kind models.RunKind, groupID string) (models.UpdateRun, error) {
	var groupArg interface{}
	if groupID != "" {
		groupArg = groupID
	}
	rows, err := db.Query(ctx, `
		INSERT INTO update_runs (host_id, run_group_id, triggered_by, kind, status, started_at, output)
		VALUES ($1, $2, $3, $4, 'running', NOW(), '')
		RETURNING `+runColumns,
		hostID, groupArg, triggeredBy, kind)
	if err != nil {
		return models.UpdateRun{}, fmt.Errorf("create run: %w", err)
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.UpdateRun])
}

// ListRunsForGroup returns every run that belongs to a bulk run_group_id,
// ordered by host_id so the UI can render a stable per-host accordion.
func ListRunsForGroup(ctx context.Context, db *pgxpool.Pool, groupID string) ([]models.UpdateRun, error) {
	rows, err := db.Query(ctx, `
		SELECT `+runColumns+`
		FROM update_runs
		WHERE run_group_id = $1
		ORDER BY host_id
	`, groupID)
	if err != nil {
		return nil, err
	}
	runs, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.UpdateRun])
	if err != nil {
		return nil, err
	}
	if runs == nil {
		runs = []models.UpdateRun{}
	}
	return runs, nil
}

// AppendRunOutput appends a chunk of output to an existing run, capped at
// MaxRunOutputBytes. Returns true iff this write fit fully within the cap;
// callers can use that to decide whether to keep buffering.
func AppendRunOutput(ctx context.Context, db *pgxpool.Pool, runID int32, chunk string) (bool, error) {
	if chunk == "" {
		return true, nil
	}
	tag, err := db.Exec(ctx, `
		UPDATE update_runs
		SET output = LEFT(output || $2, $3)
		WHERE id = $1
		  AND length(output) < $3
	`, runID, chunk, MaxRunOutputBytes)
	if err != nil {
		return false, fmt.Errorf("append run output: %w", err)
	}
	return tag.RowsAffected() == 1, nil
}

// FinishRun marks a run terminal. exitCode is recorded when known; pass -1
// to leave it null (e.g. for failures before the SSH command ran).
func FinishRun(ctx context.Context, db *pgxpool.Pool, runID int32, status models.RunStatus, exitCode int, errMsg string) error {
	var exit sql.NullInt32
	if exitCode >= 0 {
		exit = sql.NullInt32{Int32: int32(exitCode), Valid: true}
	}
	var errVal sql.NullString
	if errMsg != "" {
		errVal = sql.NullString{String: errMsg, Valid: true}
	}
	_, err := db.Exec(ctx, `
		UPDATE update_runs
		SET status = $2,
		    exit_code = $3,
		    finished_at = NOW(),
		    error = $4
		WHERE id = $1
	`, runID, status, exit, errVal)
	if err != nil {
		return fmt.Errorf("finish run: %w", err)
	}
	return nil
}

// ListRunsForHost returns runs for a host newest-first. limit is hard-capped
// at 100 to keep the response bounded.
func ListRunsForHost(ctx context.Context, db *pgxpool.Pool, hostID int32, limit int) ([]models.UpdateRun, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := db.Query(ctx, `
		SELECT `+runColumns+`
		FROM update_runs
		WHERE host_id = $1
		ORDER BY started_at DESC
		LIMIT $2
	`, hostID, limit)
	if err != nil {
		return nil, err
	}
	runs, err := pgx.CollectRows(rows, pgx.RowToStructByName[models.UpdateRun])
	if err != nil {
		return nil, err
	}
	if runs == nil {
		runs = []models.UpdateRun{}
	}
	return runs, nil
}

// GetRun fetches a single run by id. Returns pgx.ErrNoRows if it doesn't
// exist.
func GetRun(ctx context.Context, db *pgxpool.Pool, id int32) (models.UpdateRun, error) {
	rows, err := db.Query(ctx, `SELECT `+runColumns+` FROM update_runs WHERE id = $1`, id)
	if err != nil {
		return models.UpdateRun{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[models.UpdateRun])
}
