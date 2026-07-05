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
	return mock.NewRows([]string{"id", "name", "host_ids", "interval_minutes", "next_run_at", "enabled", "created_by", "created_at", "playbook_id"})
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
			AddRow(int32(1), "nightly", []int32{1, 2}, int32(1440), now, true, "admin", now, nil).
			AddRow(int32(2), "empty", []int32{}, int32(60), now, true, "admin", now, nil))

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
			AddRow(int32(1), "a", []int32{1}, int32(60), now, true, "admin", now, nil).
			AddRow(int32(2), "b", []int32{2}, int32(60), now, true, "admin", now, nil))

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
			AddRow(int32(1), "pb-sched", []int32{5}, int32(60), now, true, "admin", now, &pbID))
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
			AddRow(int32(1), "apt", []int32{5}, int32(60), now, true, "admin", now, nil))

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
		WithArgs("n", []int32{1}, int32(60), pgxmock.AnyArg(), "admin", nil).
		WillReturnRows(schedRows(mock).AddRow(int32(1), "n", []int32{1}, int32(60), now, true, "admin", now, nil))

	s, err := scheduler.Create(context.Background(), mock, "n", []int32{1}, 60, time.Time{}, "admin", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "n" {
		t.Errorf("Name = %q", s.Name)
	}
}
