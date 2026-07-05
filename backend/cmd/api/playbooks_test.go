package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
)

func pbCols(mock pgxmock.PgxPoolIface) *pgxmock.Rows {
	return mock.NewRows([]string{"id", "name", "description", "steps", "use_sudo", "created_by", "created_at", "updated_at"})
}

func TestHandleListPlaybooks(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	now := time.Now()
	mock.ExpectQuery(`SELECT (.+) FROM playbooks ORDER BY name`).
		WillReturnRows(pbCols(mock).AddRow(int32(1), "harden", "", []string{"echo hi"}, true, "admin", now, now))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/playbooks", nil)
	rr := httptest.NewRecorder()
	app.handleListPlaybooks(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestHandleCreatePlaybook_Validates(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	// Empty name → 400 (no DB call).
	body, _ := json.Marshal(map[string]interface{}{"name": "", "steps": []string{"x"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/playbooks", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	app.handleCreatePlaybook(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("empty name: expected 400, got %d", rr.Code)
	}

	// No non-empty steps → 400.
	body2, _ := json.Marshal(map[string]interface{}{"name": "n", "steps": []string{"  ", ""}})
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/playbooks", bytes.NewReader(body2))
	rr2 := httptest.NewRecorder()
	app.handleCreatePlaybook(rr2, req2)
	if rr2.Code != http.StatusBadRequest {
		t.Errorf("empty steps: expected 400, got %d", rr2.Code)
	}
}

func TestHandleCreatePlaybook_Conflict(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	// pgxmock treats a missing WithArgs as "expects 0 args", so spell them out
	// (name, description, steps, use_sudo, created_by).
	mock.ExpectQuery(`INSERT INTO playbooks`).
		WithArgs("dup", "", []string{"echo hi"}, true, "unknown").
		WillReturnError(&pgconn.PgError{Code: "23505"})

	body, _ := json.Marshal(map[string]interface{}{"name": "dup", "steps": []string{"echo hi"}})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/playbooks", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	app.handleCreatePlaybook(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("duplicate name: expected 409, got %d", rr.Code)
	}
}

func TestHandleDeletePlaybook_409WhenScheduled(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	// Precheck finds a schedule using the playbook → 409, no DELETE issued.
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM schedules WHERE playbook_id = \$1`).
		WithArgs(int32(1)).
		WillReturnRows(mock.NewRows([]string{"count"}).AddRow(2))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/playbooks/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	rr := httptest.NewRecorder()
	app.handleDeletePlaybook(rr, req)

	if rr.Code != http.StatusConflict {
		t.Fatalf("scheduled playbook delete: expected 409, got %d", rr.Code)
	}
}
