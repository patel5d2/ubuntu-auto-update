package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"ubuntu-auto-update/backend/pkg/middleware"
	"ubuntu-auto-update/backend/pkg/session"
)

func TestHandleListHosts(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(1), "test-host", "root", now, now, now, "", "", nil)

	mock.ExpectQuery(`SELECT (.+) FROM hosts ORDER BY hostname`).
		WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts", nil)
	rr := httptest.NewRecorder()
	app.handleListHosts(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// DB error
	mock.ExpectQuery(`SELECT (.+) FROM hosts ORDER BY hostname`).
		WillReturnError(sql.ErrConnDone)

	rr = httptest.NewRecorder()
	app.handleListHosts(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestHandleGetHost(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(1), "test-host", "root", now, now, now, "", "", nil)

	mock.ExpectQuery(`SELECT (.+) FROM hosts WHERE id = \$1`).
		WithArgs(int32(1)).
		WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	rr := httptest.NewRecorder()
	app.handleGetHost(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// ErrNoRows
	mock.ExpectQuery(`SELECT (.+) FROM hosts WHERE id = \$1`).
		WithArgs(int32(2)).
		WillReturnError(pgx.ErrNoRows)

	req = mux.SetURLVars(req, map[string]string{"id": "2"})
	rr = httptest.NewRecorder()
	app.handleGetHost(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}

	// General error
	mock.ExpectQuery(`SELECT (.+) FROM hosts WHERE id = \$1`).
		WithArgs(int32(3)).
		WillReturnError(sql.ErrConnDone)

	req = mux.SetURLVars(req, map[string]string{"id": "3"})
	rr = httptest.NewRecorder()
	app.handleGetHost(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}

	// Invalid ID
	req = httptest.NewRequest(http.MethodGet, "/api/v1/hosts/abc", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "abc"})
	rr = httptest.NewRecorder()
	app.handleGetHost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid ID, got %d", rr.Code)
	}
}

func TestHandleCreateHost_NoEnroll(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	body, _ := json.Marshal(map[string]string{
		"hostname": "new-host",
		"ssh_user": "root",
	})

	now := time.Now()
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(1), "new-host", "root", now, now, now, "", "", nil)

	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("new-host", "root").
		WillReturnRows(rows)

	mock.ExpectExec(`INSERT INTO audit_log`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/hosts", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", UserID: 1}))
	rr := httptest.NewRecorder()
	app.handleCreateHost(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}

	// DB Error
	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("new-host", "root").
		WillReturnError(sql.ErrConnDone)
	
	req = httptest.NewRequest(http.MethodPost, "/api/v1/hosts", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	app.handleCreateHost(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}

	// ErrDuplicateHostname
	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("new-host", "root").
		WillReturnError(&pgconn.PgError{Code: "23505"})
	
	req = httptest.NewRequest(http.MethodPost, "/api/v1/hosts", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	app.handleCreateHost(rr, req)

	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", rr.Code)
	}
}

func TestHandleUpdateHost(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	body, _ := json.Marshal(map[string]string{
		"ssh_user": "ubuntu",
	})

	now := time.Now()
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(1), "test-host", "ubuntu", now, now, now, "", "", nil)

	mock.ExpectQuery(`UPDATE hosts SET ssh_user = \$2 WHERE id = \$1`).
		WithArgs(int32(1), "ubuntu").
		WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/hosts/1", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", UserID: 1}))
	rr := httptest.NewRecorder()
	app.handleUpdateHost(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// ErrNoRows
	mock.ExpectQuery(`UPDATE hosts SET ssh_user = \$2 WHERE id = \$1`).
		WithArgs(int32(2), "ubuntu").
		WillReturnError(pgx.ErrNoRows)

	req = httptest.NewRequest(http.MethodPatch, "/api/v1/hosts/2", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "2"})
	rr = httptest.NewRecorder()
	app.handleUpdateHost(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}

	// DB error
	mock.ExpectQuery(`UPDATE hosts SET ssh_user = \$2 WHERE id = \$1`).
		WithArgs(int32(3), "ubuntu").
		WillReturnError(sql.ErrConnDone)

	req = httptest.NewRequest(http.MethodPatch, "/api/v1/hosts/3", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "3"})
	rr = httptest.NewRecorder()
	app.handleUpdateHost(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}

	// Empty ssh_user
	body, _ = json.Marshal(map[string]string{
		"ssh_user": "   ",
	})
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/hosts/3", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "3"})
	rr = httptest.NewRecorder()
	app.handleUpdateHost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty ssh_user, got %d", rr.Code)
	}

	// Missing ssh_user (nil)
	body, _ = json.Marshal(map[string]string{})
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/hosts/3", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "3"})
	rr = httptest.NewRecorder()
	app.handleUpdateHost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing ssh_user, got %d", rr.Code)
	}

	// Invalid JSON
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/hosts/3", bytes.NewReader([]byte("invalid json")))
	req = mux.SetURLVars(req, map[string]string{"id": "3"})
	rr = httptest.NewRecorder()
	app.handleUpdateHost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid JSON, got %d", rr.Code)
	}
}

func TestHandleDeleteHost(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	now := time.Now()
	// Success path
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(1), "test-host", "root", now, now, now, "", "", nil)
	mock.ExpectQuery(`SELECT (.+) FROM hosts WHERE id = \$1`).WithArgs(int32(1)).WillReturnRows(rows)

	mock.ExpectExec(`DELETE FROM hosts WHERE id = \$1`).WithArgs(int32(1)).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec(`INSERT INTO audit_log`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/hosts/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	req.Header.Set("X-Confirm-Hostname", "test-host")
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", UserID: 1}))
	rr := httptest.NewRecorder()
	app.handleDeleteHost(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}

	// Mismatched hostname
	rows2 := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(2), "test-host-2", "root", now, now, now, "", "", nil)
	mock.ExpectQuery(`SELECT (.+) FROM hosts WHERE id = \$1`).WithArgs(int32(2)).WillReturnRows(rows2)

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/hosts/2", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "2"})
	req.Header.Set("X-Confirm-Hostname", "wrong")
	rr = httptest.NewRecorder()
	app.handleDeleteHost(rr, req)

	if rr.Code != http.StatusPreconditionFailed {
		t.Errorf("expected 412 for mismatched hostname, got %d: %s", rr.Code, rr.Body.String())
	}

	// DB Error on GetHost
	mock.ExpectQuery(`SELECT (.+) FROM hosts WHERE id = \$1`).WithArgs(int32(3)).WillReturnError(sql.ErrConnDone)

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/hosts/3", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "3"})
	rr = httptest.NewRecorder()
	app.handleDeleteHost(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for GetHost error, got %d", rr.Code)
	}

	// DB Error on DeleteHost
	rows4 := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(4), "test-host-4", "root", now, now, now, "", "", nil)
	mock.ExpectQuery(`SELECT (.+) FROM hosts WHERE id = \$1`).WithArgs(int32(4)).WillReturnRows(rows4)

	mock.ExpectExec(`DELETE FROM hosts WHERE id = \$1`).WithArgs(int32(4)).WillReturnError(sql.ErrConnDone)

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/hosts/4", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "4"})
	req.Header.Set("X-Confirm-Hostname", "test-host-4")
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", UserID: 1}))
	rr = httptest.NewRecorder()
	app.handleDeleteHost(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for DeleteHost error, got %d", rr.Code)
	}

	// 0 rows deleted
	rows5 := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(5), "test-host-5", "root", now, now, now, "", "", nil)
	mock.ExpectQuery(`SELECT (.+) FROM hosts WHERE id = \$1`).WithArgs(int32(5)).WillReturnRows(rows5)

	mock.ExpectExec(`DELETE FROM hosts WHERE id = \$1`).WithArgs(int32(5)).WillReturnResult(pgxmock.NewResult("DELETE", 0))

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/hosts/5", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "5"})
	req.Header.Set("X-Confirm-Hostname", "test-host-5")
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", UserID: 1}))
	rr = httptest.NewRecorder()
	app.handleDeleteHost(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for 0 rows deleted, got %d", rr.Code)
	}

	// ErrNoRows on GetHost
	mock.ExpectQuery(`SELECT (.+) FROM hosts WHERE id = \$1`).WithArgs(int32(6)).WillReturnError(pgx.ErrNoRows)

	req = httptest.NewRequest(http.MethodDelete, "/api/v1/hosts/6", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "6"})
	rr = httptest.NewRecorder()
	app.handleDeleteHost(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for ErrNoRows on GetHost, got %d", rr.Code)
	}
}

func TestHandleReport_Success(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	body, _ := json.Marshal(map[string]string{
		"hostname":       "test-host",
		"update_output":  "update",
		"upgrade_output": "upgrade",
	})

	now := time.Now()
	rows := mock.NewRows([]string{"id", "hostname", "ssh_user", "created_at", "updated_at", "last_seen", "update_output", "upgrade_output", "error"}).
		AddRow(int32(1), "test-host", "root", now, now, now, "update", "upgrade", nil)

	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("test-host", "root", "update", "upgrade", sql.NullString{}).
		WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	app.handleReport(rr, req)

	if rr.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", rr.Code)
	}
}

func TestHandleReport_DBError(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	body, _ := json.Marshal(map[string]string{
		"hostname":       "test-host",
		"update_output":  "update",
		"upgrade_output": "upgrade",
	})

	mock.ExpectQuery(`INSERT INTO hosts`).
		WithArgs("test-host", "root", "update", "upgrade", sql.NullString{}).
		WillReturnError(sql.ErrConnDone)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	app.handleReport(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}