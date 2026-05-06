package db_test

import (
	"context"
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
		WithArgs(int32(10), "admin", models.RunKindUpdate, nil).
		WillReturnRows(rows)

	run, err := db.CreateRun(context.Background(), mock, 10, "admin", models.RunKindUpdate)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if run.ID != 1 {
		t.Errorf("expected id 1, got %d", run.ID)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
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
		WithArgs(int32(10), "admin", models.RunKindUpdate, "group-123").
		WillReturnRows(rows)

	run, err := db.CreateRunWithGroup(context.Background(), mock, 10, "admin", models.RunKindUpdate, "group-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if run.RunGroupID.String != "group-123" {
		t.Errorf("expected RunGroupID group-123, got %v", run.RunGroupID.String)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
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

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestAppendRunOutput(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec(`UPDATE update_runs SET output = output \|\| \$2`).
		WithArgs(int32(1), "new output").
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

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestFinishRun(t *testing.T) {
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}
	defer mock.Close()

	mock.ExpectExec(`UPDATE update_runs SET status = \$2, exit_code = \$3, error = \$4, finished_at = NOW\(\) WHERE id = \$1`).
		WithArgs(int32(1), models.RunStatusSucceeded, int32(0), nil).
		WillReturnResult(pgxmock.NewResult("UPDATE", 1))

	err = db.FinishRun(context.Background(), mock, 1, models.RunStatusSucceeded, 0, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
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

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
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

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}