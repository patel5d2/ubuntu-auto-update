package db_test

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"ubuntu-auto-update/backend/pkg/db"
	"ubuntu-auto-update/backend/pkg/models"
)

func TestCreateRun(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "host_id", "run_group_id", "triggered_by", "kind", "status", "exit_code", "started_at", "finished_at", "output", "error"}).
		AddRow(int32(1), int32(10), nil, "admin", models.RunKindUpdate, models.RunStatusRunning, nil, now, nil, "", nil)

	mock.ExpectQuery(`INSERT INTO update_runs`).
		WithArgs(int32(10), nil, "admin", models.RunKindUpdate).
		WillReturnRows(rows)

	_, err = db.CreateRun(context.Background(), mock, 10, "admin", models.RunKindUpdate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Error path
	mock.ExpectQuery(`INSERT INTO update_runs`).
		WithArgs(int32(20), nil, "admin", models.RunKindUpdate).
		WillReturnError(errors.New("db error"))

	_, err = db.CreateRun(context.Background(), mock, 20, "admin", models.RunKindUpdate)
	if err == nil {
		t.Error("expected error")
	}
}

func TestCreateRunWithGroup(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "host_id", "run_group_id", "triggered_by", "kind", "status", "exit_code", "started_at", "finished_at", "output", "error"}).
		AddRow(int32(1), int32(10), "group-123", "admin", models.RunKindUpdate, models.RunStatusRunning, nil, now, nil, "", nil)

	mock.ExpectQuery(`INSERT INTO update_runs`).
		WithArgs(int32(10), "group-123", "admin", models.RunKindUpdate).
		WillReturnRows(rows)

	run, err := db.CreateRunWithGroup(context.Background(), mock, 10, "admin", models.RunKindUpdate, "group-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if run.RunGroupID.String != "group-123" {
		t.Errorf("expected RunGroupID group-123, got %v", run.RunGroupID.String)
	}

	// Error path
	mock.ExpectQuery(`INSERT INTO update_runs`).
		WithArgs(int32(20), "group-123", "admin", models.RunKindUpdate).
		WillReturnError(errors.New("db error"))

	_, err = db.CreateRunWithGroup(context.Background(), mock, 20, "admin", models.RunKindUpdate, "group-123")
	if err == nil {
		t.Error("expected error")
	}
}

func TestListRunsForGroup(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "host_id", "run_group_id", "triggered_by", "kind", "status", "exit_code", "started_at", "finished_at", "output", "error"}).
		AddRow(int32(1), int32(10), "group-123", "admin", models.RunKindUpdate, models.RunStatusRunning, nil, now, nil, "", nil)

	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE run_group_id = \$1`).
		WithArgs("group-123").
		WillReturnRows(rows)

	runs, err := db.ListRunsForGroup(context.Background(), mock, "group-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runs) != 1 {
		t.Errorf("expected 1 run, got %d", len(runs))
	}

	// Nil results
	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE run_group_id = \$1`).
		WithArgs("group-456").
		WillReturnRows(mock.NewRows([]string{"id", "host_id", "run_group_id", "triggered_by", "kind", "status", "exit_code", "started_at", "finished_at", "output", "error"}))

	runs, err = db.ListRunsForGroup(context.Background(), mock, "group-456")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if runs == nil {
		t.Error("expected empty slice, got nil")
	}

	// Error path
	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE run_group_id = \$1`).
		WithArgs("group-789").
		WillReturnError(errors.New("db error"))

	_, err = db.ListRunsForGroup(context.Background(), mock, "group-789")
	if err == nil {
		t.Error("expected error")
	}

	// CollectRows error path
	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE run_group_id = \$1`).
		WithArgs("group-bad-rows").
		WillReturnRows(mock.NewRows([]string{"id"}).AddRow("not-an-int"))

	_, err = db.ListRunsForGroup(context.Background(), mock, "group-bad-rows")
	if err == nil {
		t.Error("expected error from CollectRows")
	}
}

func TestAppendRunOutput(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec(`UPDATE update_runs SET output = LEFT\(output \|\| \$2, \$3\)`).
		WithArgs(int32(1), "new output", int(1<<20)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	ok, err := db.AppendRunOutput(context.Background(), mock, 1, "new output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !ok {
		t.Error("expected ok true")
	}

	// Test empty chunk short-circuit
	ok, err = db.AppendRunOutput(context.Background(), mock, 1, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected ok true for empty chunk")
	}

	// Truncated (RowsAffected == 0)
	mock.ExpectExec(`UPDATE update_runs SET output = LEFT\(output \|\| \$2, \$3\)`).
		WithArgs(int32(2), "new output", int(1<<20)).
		WillReturnResult(pgxmock.NewResult("UPDATE", 0))

	ok, err = db.AppendRunOutput(context.Background(), mock, 2, "new output")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected ok false for truncated output")
	}

	// Error path
	mock.ExpectExec(`UPDATE update_runs SET output = LEFT\(output \|\| \$2, \$3\)`).
		WithArgs(int32(3), "new output", int(1<<20)).
		WillReturnError(errors.New("db error"))

	_, err = db.AppendRunOutput(context.Background(), mock, 3, "new output")
	if err == nil {
		t.Error("expected error")
	}
}

func TestFinishRun(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec(`UPDATE update_runs SET status = \$2, exit_code = \$3, finished_at = NOW\(\), error = \$4 WHERE id = \$1`).
		WithArgs(int32(1), models.RunStatusSucceeded, sql.NullInt32{Int32: 0, Valid: true}, sql.NullString{}).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err = db.FinishRun(context.Background(), mock, 1, models.RunStatusSucceeded, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Error path (exitCode -1, non-empty error)
	mock.ExpectExec(`UPDATE update_runs SET status = \$2, exit_code = \$3, finished_at = NOW\(\), error = \$4 WHERE id = \$1`).
		WithArgs(int32(2), models.RunStatusFailed, sql.NullInt32{}, sql.NullString{String: "err", Valid: true}).
		WillReturnError(errors.New("db error"))

	err = db.FinishRun(context.Background(), mock, 2, models.RunStatusFailed, -1, "err")
	if err == nil {
		t.Error("expected error")
	}
}

func TestListRunsForHost(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "host_id", "run_group_id", "triggered_by", "kind", "status", "exit_code", "started_at", "finished_at", "output", "error"}).
		AddRow(int32(1), int32(10), nil, "admin", models.RunKindUpdate, models.RunStatusRunning, nil, now, nil, "", nil)

	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE host_id = \$1 ORDER BY started_at DESC LIMIT \$2`).
		WithArgs(int32(10), 10).
		WillReturnRows(rows)

	runs, err := db.ListRunsForHost(context.Background(), mock, 10, 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(runs) != 1 {
		t.Errorf("expected 1 run, got %d", len(runs))
	}

	// Test limit defaults (<= 0 or > 100) -> 50
	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE host_id = \$1 ORDER BY started_at DESC LIMIT \$2`).
		WithArgs(int32(10), 50).
		WillReturnRows(mock.NewRows([]string{"id", "host_id", "run_group_id", "triggered_by", "kind", "status", "exit_code", "started_at", "finished_at", "output", "error"}))
	
	_, err = db.ListRunsForHost(context.Background(), mock, 10, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Error path
	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE host_id = \$1 ORDER BY started_at DESC LIMIT \$2`).
		WithArgs(int32(20), 50).
		WillReturnError(errors.New("db error"))

	_, err = db.ListRunsForHost(context.Background(), mock, 20, 200)
	if err == nil {
		t.Error("expected error")
	}

	// CollectRows error path
	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE host_id = \$1 ORDER BY started_at DESC LIMIT \$2`).
		WithArgs(int32(30), 50).
		WillReturnRows(mock.NewRows([]string{"id"}).AddRow("not-an-int"))

	_, err = db.ListRunsForHost(context.Background(), mock, 30, 50)
	if err == nil {
		t.Error("expected error from CollectRows")
	}
}

func TestGetRun(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "host_id", "run_group_id", "triggered_by", "kind", "status", "exit_code", "started_at", "finished_at", "output", "error"}).
		AddRow(int32(1), int32(10), nil, "admin", models.RunKindUpdate, models.RunStatusRunning, nil, now, nil, "", nil)

	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE id = \$1`).
		WithArgs(int32(1)).
		WillReturnRows(rows)

	run, err := db.GetRun(context.Background(), mock, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if run.ID != 1 {
		t.Errorf("expected id 1, got %d", run.ID)
	}

	// Test ErrNoRows
	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE id = \$1`).
		WithArgs(int32(999)).
		WillReturnError(pgx.ErrNoRows)

	_, err = db.GetRun(context.Background(), mock, 999)
	if err != pgx.ErrNoRows {
		t.Errorf("expected pgx.ErrNoRows, got %v", err)
	}

	// General Error
	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE id = \$1`).
		WithArgs(int32(1000)).
		WillReturnError(errors.New("db error"))

	_, err = db.GetRun(context.Background(), mock, 1000)
	if err == nil {
		t.Error("expected error")
	}
}