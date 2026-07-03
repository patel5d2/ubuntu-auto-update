package scheduler_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/pashagolub/pgxmock/v4"

	"ubuntu-auto-update/backend/pkg/scheduler"
	"ubuntu-auto-update/backend/pkg/updater"
)

type fakeStarter struct {
	calls []updater.BulkRunOptions
	err   error
}

func (f *fakeStarter) Start(_ context.Context, opts updater.BulkRunOptions) (updater.BulkResult, error) {
	f.calls = append(f.calls, opts)
	return updater.BulkResult{GroupID: "g"}, f.err
}

func schedRows(mock pgxmock.PgxPoolIface) *pgxmock.Rows {
	return mock.NewRows([]string{"id", "name", "host_ids", "interval_minutes", "next_run_at", "enabled", "created_by", "created_at"})
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
			AddRow(int32(1), "nightly", []int32{1, 2}, int32(1440), now, true, "admin", now).
			AddRow(int32(2), "empty", []int32{}, int32(60), now, true, "admin", now))

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
			AddRow(int32(1), "a", []int32{1}, int32(60), now, true, "admin", now).
			AddRow(int32(2), "b", []int32{2}, int32(60), now, true, "admin", now))

	st := &fakeStarter{err: errors.New("host gone")}
	scheduler.Tick(context.Background(), mock, st)

	if len(st.calls) != 2 {
		t.Fatalf("a failing schedule must not stop the rest; got %d calls", len(st.calls))
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
		WithArgs("n", []int32{1}, int32(60), pgxmock.AnyArg(), "admin").
		WillReturnRows(schedRows(mock).AddRow(int32(1), "n", []int32{1}, int32(60), now, true, "admin", now))

	s, err := scheduler.Create(context.Background(), mock, "n", []int32{1}, 60, time.Time{}, "admin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if s.Name != "n" {
		t.Errorf("Name = %q", s.Name)
	}
}
