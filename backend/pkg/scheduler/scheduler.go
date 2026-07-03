// Package scheduler fires recurring bulk updates. One goroutine polls the
// schedules table; a due schedule is claimed atomically with
// UPDATE ... RETURNING, so the pattern stays correct even if a second
// backend replica ever runs.
//
// ponytail: interval-based recurrence only ("every N minutes from a start
// time"). Upgrade to cron expressions when someone actually needs
// "weekdays at 03:00 except holidays".
package scheduler

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"

	"ubuntu-auto-update/backend/pkg/db"
	"ubuntu-auto-update/backend/pkg/updater"
)

type Schedule struct {
	ID              int32     `json:"id" db:"id"`
	Name            string    `json:"name" db:"name"`
	HostIDs         []int32   `json:"host_ids" db:"host_ids"`
	IntervalMinutes int32     `json:"interval_minutes" db:"interval_minutes"`
	NextRunAt       time.Time `json:"next_run_at" db:"next_run_at"`
	Enabled         bool      `json:"enabled" db:"enabled"`
	CreatedBy       string    `json:"created_by" db:"created_by"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

const cols = `id, name, host_ids, interval_minutes, next_run_at, enabled, created_by, created_at`

func List(ctx context.Context, dbx db.DBTX) ([]Schedule, error) {
	rows, err := dbx.Query(ctx, `SELECT `+cols+` FROM schedules ORDER BY next_run_at`)
	if err != nil {
		return nil, err
	}
	scheds, err := pgx.CollectRows(rows, pgx.RowToStructByName[Schedule])
	if err != nil {
		return nil, err
	}
	if scheds == nil {
		scheds = []Schedule{}
	}
	return scheds, nil
}

// Create inserts a schedule. startAt is the first fire time; the zero value
// means "one interval from now".
func Create(ctx context.Context, dbx db.DBTX, name string, hostIDs []int32, intervalMinutes int32, startAt time.Time, createdBy string) (Schedule, error) {
	if startAt.IsZero() {
		startAt = time.Now().Add(time.Duration(intervalMinutes) * time.Minute)
	}
	rows, err := dbx.Query(ctx, `
		INSERT INTO schedules (name, host_ids, interval_minutes, next_run_at, created_by)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING `+cols,
		name, hostIDs, intervalMinutes, startAt, createdBy)
	if err != nil {
		return Schedule{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Schedule])
}

// SetEnabled toggles a schedule. Re-enabling pushes next_run_at forward so a
// long-disabled schedule doesn't fire the moment it's switched back on.
func SetEnabled(ctx context.Context, dbx db.DBTX, id int32, enabled bool) (Schedule, error) {
	rows, err := dbx.Query(ctx, `
		UPDATE schedules
		SET enabled = $2,
		    next_run_at = CASE WHEN $2 AND next_run_at < NOW()
		                       THEN NOW() + make_interval(mins => interval_minutes)
		                       ELSE next_run_at END
		WHERE id = $1
		RETURNING `+cols,
		id, enabled)
	if err != nil {
		return Schedule{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Schedule])
}

func Delete(ctx context.Context, dbx db.DBTX, id int32) (int64, error) {
	tag, err := dbx.Exec(ctx, `DELETE FROM schedules WHERE id = $1`, id)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// claimDue atomically advances next_run_at for every due schedule and
// returns the claimed rows. Advancing from NOW() (not from the stale
// next_run_at) means a backend that was down for a week fires each schedule
// once, not once per missed interval.
func claimDue(ctx context.Context, dbx db.DBTX) ([]Schedule, error) {
	rows, err := dbx.Query(ctx, `
		UPDATE schedules
		SET next_run_at = NOW() + make_interval(mins => interval_minutes)
		WHERE enabled AND next_run_at <= NOW()
		RETURNING `+cols)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Schedule])
}

// Starter is the slice of updater.Coordinator the scheduler needs; an
// interface so tests can fake the fan-out.
type Starter interface {
	Start(ctx context.Context, opts updater.BulkRunOptions) (updater.BulkResult, error)
}

// Run polls for due schedules until ctx is cancelled. Call as a goroutine
// from main.
func Run(ctx context.Context, dbx db.DBTX, coord Starter) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		Tick(ctx, dbx, coord)
	}
}

// Tick fires every due schedule once. Split from Run for testability.
func Tick(ctx context.Context, dbx db.DBTX, coord Starter) {
	due, err := claimDue(ctx, dbx)
	if err != nil {
		log.Errorf("scheduler: claim due: %v", err)
		return
	}
	for _, s := range due {
		if len(s.HostIDs) == 0 {
			continue
		}
		res, err := coord.Start(ctx, updater.BulkRunOptions{
			HostIDs:     s.HostIDs,
			TriggeredBy: "schedule:" + s.Name,
		})
		if err != nil {
			// A deleted host in the snapshot fails the whole Start; the
			// schedule stays armed for next interval. Operator sees the log.
			log.Errorf("scheduler: fire %q: %v", s.Name, err)
			continue
		}
		log.Infof("scheduler: fired %q as group %s (%d hosts)", s.Name, res.GroupID, len(s.HostIDs))
	}
}
