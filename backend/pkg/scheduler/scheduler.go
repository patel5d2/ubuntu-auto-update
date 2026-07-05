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
	"ubuntu-auto-update/backend/pkg/models"
	"ubuntu-auto-update/backend/pkg/playbooks"
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
	// PlaybookID nil ⇒ apt-update schedule (today's behavior); set ⇒ run the
	// referenced playbook.
	PlaybookID *int32 `json:"playbook_id" db:"playbook_id"`

	// Rollout knobs passed straight to the bulk coordinator; zero = its
	// defaults, i.e. the behavior before these columns existed.
	Concurrency       int32 `json:"concurrency" db:"concurrency"`
	CanaryCount       int32 `json:"canary_count" db:"canary_count"`
	CanaryWaitSeconds int32 `json:"canary_wait_seconds" db:"canary_wait_seconds"`
	AbortOnFailurePct int32 `json:"abort_on_failure_pct" db:"abort_on_failure_pct"`

	// Maintenance window in minutes since midnight UTC; nil start = no
	// window. WindowDays is a bitmask (bit 0 = Sunday … bit 6 = Saturday)
	// keyed to the window's start day.
	WindowStartMinute *int32 `json:"window_start_minute" db:"window_start_minute"`
	WindowEndMinute   *int32 `json:"window_end_minute" db:"window_end_minute"`
	WindowDays        int16  `json:"window_days" db:"window_days"`

	// SecurityOnly (apt schedules only): unattended-upgrade instead of a
	// blanket apt-get upgrade.
	SecurityOnly bool `json:"security_only" db:"security_only"`
}

const cols = `id, name, host_ids, interval_minutes, next_run_at, enabled, created_by, created_at, playbook_id, concurrency, canary_count, canary_wait_seconds, abort_on_failure_pct, window_start_minute, window_end_minute, window_days, security_only`

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

// CreateOptions carries everything a new schedule needs. Zero-valued knobs
// mean coordinator defaults; nil window means "no maintenance window";
// WindowDays 0 is normalized to 127 (every day).
type CreateOptions struct {
	Name            string
	HostIDs         []int32
	IntervalMinutes int32
	StartAt         time.Time // zero = one interval from now
	CreatedBy       string
	PlaybookID      *int32

	Concurrency       int32
	CanaryCount       int32
	CanaryWaitSeconds int32
	AbortOnFailurePct int32

	WindowStartMinute *int32
	WindowEndMinute   *int32
	WindowDays        int16

	SecurityOnly bool
}

// Create inserts a schedule.
func Create(ctx context.Context, dbx db.DBTX, o CreateOptions) (Schedule, error) {
	if o.StartAt.IsZero() {
		o.StartAt = time.Now().Add(time.Duration(o.IntervalMinutes) * time.Minute)
	}
	if o.WindowDays == 0 {
		o.WindowDays = 127
	}
	var pbArg interface{}
	if o.PlaybookID != nil {
		pbArg = *o.PlaybookID
	}
	rows, err := dbx.Query(ctx, `
		INSERT INTO schedules (name, host_ids, interval_minutes, next_run_at, created_by, playbook_id,
		                       concurrency, canary_count, canary_wait_seconds, abort_on_failure_pct,
		                       window_start_minute, window_end_minute, window_days, security_only)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)
		RETURNING `+cols,
		o.Name, o.HostIDs, o.IntervalMinutes, o.StartAt, o.CreatedBy, pbArg,
		o.Concurrency, o.CanaryCount, o.CanaryWaitSeconds, o.AbortOnFailurePct,
		o.WindowStartMinute, o.WindowEndMinute, o.WindowDays, o.SecurityOnly)
	if err != nil {
		return Schedule{}, err
	}
	return pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[Schedule])
}

// InWindow reports whether now falls inside the schedule's maintenance
// window. Schedules without a window are always in it. A window that wraps
// midnight (start > end) belongs to its start day: with days = Sat only and
// window 22:00–02:00, Sunday 01:00 UTC is IN (Saturday's window).
func (s Schedule) InWindow(now time.Time) bool {
	if s.WindowStartMinute == nil || s.WindowEndMinute == nil {
		return true
	}
	start, end := int(*s.WindowStartMinute), int(*s.WindowEndMinute)
	now = now.UTC()
	minute := now.Hour()*60 + now.Minute()
	dayOK := func(d time.Weekday) bool { return s.WindowDays&(1<<uint(d)) != 0 }
	if start <= end {
		return dayOK(now.Weekday()) && minute >= start && minute < end
	}
	if minute >= start {
		return dayOK(now.Weekday())
	}
	if minute < end {
		return dayOK(now.Add(-24 * time.Hour).Weekday())
	}
	return false
}

// NextWindowStart returns the next moment the window opens after now.
// Schedules without a window return now unchanged.
func (s Schedule) NextWindowStart(now time.Time) time.Time {
	if s.WindowStartMinute == nil {
		return now
	}
	start := time.Duration(*s.WindowStartMinute) * time.Minute
	now = now.UTC()
	midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	for i := 0; i < 8; i++ {
		candidate := midnight.AddDate(0, 0, i).Add(start)
		if candidate.After(now) && s.WindowDays&(1<<uint(candidate.Weekday())) != 0 {
			return candidate
		}
	}
	return now.Add(24 * time.Hour) // defensive; unreachable while days >= 1
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
	now := time.Now()
	for _, s := range due {
		if len(s.HostIDs) == 0 {
			continue
		}
		// Outside the maintenance window: defer to its next opening instead
		// of the claimed now+interval, so the run fires when the window opens.
		if !s.InWindow(now) {
			next := s.NextWindowStart(now)
			if _, err := dbx.Exec(ctx, `UPDATE schedules SET next_run_at = $2 WHERE id = $1`, s.ID, next); err != nil {
				log.Errorf("scheduler: defer %q to window: %v", s.Name, err)
			} else {
				log.Infof("scheduler: %q outside window; deferred to %s", s.Name, next.Format(time.RFC3339))
			}
			continue
		}
		opts := updater.BulkRunOptions{
			HostIDs:           s.HostIDs,
			TriggeredBy:       "schedule:" + s.Name,
			Concurrency:       int(s.Concurrency),
			CanaryCount:       int(s.CanaryCount),
			CanaryWaitSeconds: int(s.CanaryWaitSeconds),
			AbortOnFailurePct: int(s.AbortOnFailurePct),
			SecurityOnly:      s.SecurityOnly,
		}
		// Playbook schedule: load steps; on error leave the schedule armed for
		// the next interval. Zero-valued opts (apt path) otherwise — unchanged.
		if s.PlaybookID != nil {
			pb, err := playbooks.Get(ctx, dbx, *s.PlaybookID)
			if err != nil {
				log.Errorf("scheduler: load playbook %d for %q: %v", *s.PlaybookID, s.Name, err)
				continue
			}
			opts.Kind = models.RunKindPlaybook
			opts.Steps = pb.Steps
			opts.UseSudo = pb.UseSudo
			opts.PlaybookID = s.PlaybookID
		}
		res, err := coord.Start(ctx, opts)
		if err != nil {
			// A deleted host in the snapshot fails the whole Start; the
			// schedule stays armed for next interval. Operator sees the log.
			log.Errorf("scheduler: fire %q: %v", s.Name, err)
			continue
		}
		log.Infof("scheduler: fired %q as group %s (%d hosts)", s.Name, res.GroupID, len(s.HostIDs))
	}
}
