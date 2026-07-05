package scheduler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"ubuntu-auto-update/backend/pkg/models"
	"ubuntu-auto-update/backend/pkg/scheduler"
	"ubuntu-auto-update/backend/pkg/updater"
)

func pbRows(mock pgxmock.PgxPoolIface) *pgxmock.Rows {
	return mock.NewRows([]string{"id", "name", "description", "steps", "use_sudo", "created_by", "created_at", "updated_at"})
}

type fakeStarter struct {
	calls []updater.BulkRunOptions
	err   error
}

func (f *fakeStarter) Start(_ context.Context, opts updater.BulkRunOptions) (updater.BulkResult, error) {
	f.calls = append(f.calls, opts)
	return updater.BulkResult{GroupID: "g"}, f.err
}

func schedRows(mock pgxmock.PgxPoolIface) *pgxmock.Rows {
	return mock.NewRows([]string{"id", "name", "host_ids", "interval_minutes", "next_run_at", "enabled", "created_by", "created_at", "playbook_id",
		"concurrency", "canary_count", "canary_wait_seconds", "abort_on_failure_pct", "window_start_minute", "window_end_minute", "window_days", "security_only"})
}

func TestTickFiresDueSchedules(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	now := time.Now()
	mock.ExpectQuery(`UPDATE schedules`).
		WillReturnRows(schedRows(mock).
			AddRow(int32(1), "nightly", []int32{1, 2}, int32(1440), now, true, "admin", now, nil, int32(0), int32(0), int32(0), int32(0), nil, nil, int16(127), false).
			AddRow(int32(2), "empty", []int32{}, int32(60), now, true, "admin", now, nil, int32(0), int32(0), int32(0), int32(0), nil, nil, int16(127), false))

	st := &fakeStarter{}
	scheduler.Tick(context.Background(), mock, st)

	if len(st.calls) != 1 {
		t.Fatalf("expected 1 fire (empty host list skipped), got %d", len(st.calls))
	}
	if st.calls[0].TriggeredBy != "schedule:nightly" {
		t.Errorf("TriggeredBy = %q", st.calls[0].TriggeredBy)
	}
	if len(st.calls[0].HostIDs) != 2 {
		t.Errorf("HostIDs = %v", st.calls[0].HostIDs)
	}
}

func TestTickSurvivesStarterError(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	now := time.Now()
	mock.ExpectQuery(`UPDATE schedules`).
		WillReturnRows(schedRows(mock).
			AddRow(int32(1), "a", []int32{1}, int32(60), now, true, "admin", now, nil, int32(0), int32(0), int32(0), int32(0), nil, nil, int16(127), false).
			AddRow(int32(2), "b", []int32{2}, int32(60), now, true, "admin", now, nil, int32(0), int32(0), int32(0), int32(0), nil, nil, int16(127), false))

	st := &fakeStarter{err: errors.New("host gone")}
	scheduler.Tick(context.Background(), mock, st)

	if len(st.calls) != 2 {
		t.Fatalf("a failing schedule must not stop the rest; got %d calls", len(st.calls))
	}
}

// A playbook schedule loads the playbook and produces playbook-kind options.
func TestTickPlaybookSchedule(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	now := time.Now()
	pbID := int32(7)
	mock.ExpectQuery(`UPDATE schedules`).
		WillReturnRows(schedRows(mock).
			AddRow(int32(1), "pb-sched", []int32{5}, int32(60), now, true, "admin", now, &pbID, int32(0), int32(0), int32(0), int32(0), nil, nil, int16(127), false))
	mock.ExpectQuery(`SELECT (.+) FROM playbooks WHERE id = \$1`).
		WithArgs(pbID).
		WillReturnRows(pbRows(mock).AddRow(pbID, "harden", "", []string{"echo hi"}, true, "admin", now, now))

	st := &fakeStarter{}
	scheduler.Tick(context.Background(), mock, st)

	if len(st.calls) != 1 {
		t.Fatalf("expected 1 fire, got %d", len(st.calls))
	}
	o := st.calls[0]
	if o.Kind != models.RunKindPlaybook {
		t.Errorf("Kind = %q, want playbook", o.Kind)
	}
	if len(o.Steps) != 1 || o.Steps[0] != "echo hi" {
		t.Errorf("Steps = %v", o.Steps)
	}
	if o.PlaybookID == nil || *o.PlaybookID != pbID {
		t.Errorf("PlaybookID = %v", o.PlaybookID)
	}
}

// An apt schedule (playbook_id NULL) must produce zero-valued options — this
// pins the non-breaking claim for existing schedules.
func TestTickAptScheduleZeroValued(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	now := time.Now()
	mock.ExpectQuery(`UPDATE schedules`).
		WillReturnRows(schedRows(mock).
			AddRow(int32(1), "apt", []int32{5}, int32(60), now, true, "admin", now, nil, int32(0), int32(0), int32(0), int32(0), nil, nil, int16(127), false))

	st := &fakeStarter{}
	scheduler.Tick(context.Background(), mock, st)

	if len(st.calls) != 1 {
		t.Fatalf("expected 1 fire, got %d", len(st.calls))
	}
	o := st.calls[0]
	if o.Kind != "" || o.Steps != nil || o.PlaybookID != nil {
		t.Errorf("apt schedule must be zero-valued: kind=%q steps=%v pb=%v", o.Kind, o.Steps, o.PlaybookID)
	}
}

func TestCreateDefaultsStart(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	now := time.Now()
	mock.ExpectQuery(`INSERT INTO schedules`).
		WithArgs("n", []int32{1}, int32(60), pgxmock.AnyArg(), "admin", nil,
			int32(0), int32(0), int32(0), int32(0), (*int32)(nil), (*int32)(nil), int16(127), false).
		WillReturnRows(schedRows(mock).AddRow(int32(1), "n", []int32{1}, int32(60), now, true, "admin", now, nil, int32(0), int32(0), int32(0), int32(0), nil, nil, int16(127), false))

	s, err := scheduler.Create(context.Background(), mock, scheduler.CreateOptions{
		Name: "n", HostIDs: []int32{1}, IntervalMinutes: 60, CreatedBy: "admin",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "n" {
		t.Errorf("Name = %q", s.Name)
	}
}

func i32(v int32) *int32 { return &v }

func TestWindowLogic(t *testing.T) {
	// Tuesday 2026-07-07 01:00 UTC (weekday 2).
	tue0100 := time.Date(2026, 7, 7, 1, 0, 0, 0, time.UTC)

	base := scheduler.Schedule{WindowDays: 127}
	if !base.InWindow(tue0100) {
		t.Error("no window must always be in-window")
	}

	night := scheduler.Schedule{WindowStartMinute: i32(0), WindowEndMinute: i32(120), WindowDays: 127} // 00:00–02:00
	if !night.InWindow(tue0100) {
		t.Error("01:00 must be inside 00:00-02:00")
	}
	if night.InWindow(tue0100.Add(3 * time.Hour)) {
		t.Error("04:00 must be outside 00:00-02:00")
	}

	// Wrapping window 22:00–02:00 restricted to Monday (bit 1): Tuesday 01:00
	// belongs to Monday's window.
	monNight := scheduler.Schedule{WindowStartMinute: i32(1320), WindowEndMinute: i32(120), WindowDays: 1 << 1}
	if !monNight.InWindow(tue0100) {
		t.Error("Tue 01:00 must be inside Monday's wrapping 22:00-02:00 window")
	}
	if monNight.InWindow(tue0100.Add(24 * time.Hour)) {
		t.Error("Wed 01:00 must be outside Monday's window")
	}

	// NextWindowStart: from Tuesday 04:00, a Saturday-only (bit 6) 03:00
	// window opens Saturday 03:00.
	sat := scheduler.Schedule{WindowStartMinute: i32(180), WindowEndMinute: i32(300), WindowDays: 1 << 6}
	next := sat.NextWindowStart(tue0100.Add(3 * time.Hour))
	want := time.Date(2026, 7, 11, 3, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Errorf("NextWindowStart = %s, want %s", next, want)
	}
}

// A due schedule outside its window is deferred to the window opening, not run.
func TestTickDefersOutsideWindow(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	now := time.Now()
	// A 1-minute window that just closed: [now-2m, now-1m) — always outside.
	past := int32((now.UTC().Hour()*60 + now.UTC().Minute() + 1438) % 1440)
	end := int32((past + 1) % 1440)
	mock.ExpectQuery(`UPDATE schedules`).
		WillReturnRows(schedRows(mock).
			AddRow(int32(1), "windowed", []int32{1}, int32(60), now, true, "admin", now, nil, int32(0), int32(0), int32(0), int32(0), &past, &end, int16(127), false))
	mock.ExpectExec(`UPDATE schedules SET next_run_at`).
		WithArgs(int32(1), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	st := &fakeStarter{}
	scheduler.Tick(context.Background(), mock, st)

	if len(st.calls) != 0 {
		t.Fatalf("schedule outside window must not fire; got %d calls", len(st.calls))
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Error(err)
	}
}

// Rollout knobs stored on the schedule reach the coordinator options.
func TestTickPassesRolloutKnobs(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatal(err)
	}
	defer mock.Close()

	now := time.Now()
	mock.ExpectQuery(`UPDATE schedules`).
		WillReturnRows(schedRows(mock).
			AddRow(int32(1), "staged", []int32{1, 2, 3}, int32(60), now, true, "admin", now, nil, int32(3), int32(1), int32(120), int32(50), nil, nil, int16(127), false))

	st := &fakeStarter{}
	scheduler.Tick(context.Background(), mock, st)

	if len(st.calls) != 1 {
		t.Fatalf("expected 1 fire, got %d", len(st.calls))
	}
	o := st.calls[0]
	if o.Concurrency != 3 || o.CanaryCount != 1 || o.CanaryWaitSeconds != 120 || o.AbortOnFailurePct != 50 {
		t.Errorf("knobs not passed through: %+v", o)
	}
}
