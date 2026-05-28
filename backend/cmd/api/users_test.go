package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/pashagolub/pgxmock/v4"
	"ubuntu-auto-update/backend/pkg/middleware"
	"ubuntu-auto-update/backend/pkg/session"
)

func TestHandleListUsers(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	rows := mock.NewRows([]string{"id", "username", "role", "disabled_at", "created_at", "updated_at", "last_login_at", "failed_logins", "locked_until"}).
		AddRow(int32(1), "admin", "admin", nil, nil, nil, nil, 0, nil)

	mock.ExpectQuery(`SELECT (.+) FROM users ORDER BY username`).
		WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	rr := httptest.NewRecorder()
	app.handleListUsers(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// DB Error
	mock.ExpectQuery(`SELECT (.+) FROM users ORDER BY username`).WillReturnError(errors.New("db error"))
	rr = httptest.NewRecorder()
	app.handleListUsers(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for db error, got %d", rr.Code)
	}

	// Nil DB
	app.DB = nil
	rr = httptest.NewRecorder()
	app.handleListUsers(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for nil db, got %d", rr.Code)
	}
	app.DB = mock // restore

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestHandleCreateUser(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	body, _ := json.Marshal(map[string]string{
		"username": "newuser",
		"password": "password1234",
		"role":     "viewer",
	})

	rows := mock.NewRows([]string{"id", "username", "role", "disabled_at", "created_at", "updated_at", "last_login_at", "failed_logins", "locked_until"}).
		AddRow(int32(2), "newuser", "viewer", nil, nil, nil, nil, 0, nil)

	mock.ExpectQuery(`INSERT INTO users`).
		WithArgs("newuser", pgxmock.AnyArg(), "viewer").
		WillReturnRows(rows)

	mock.ExpectExec(`INSERT INTO audit_log`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	req := httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	// Add user context for audit
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", UserID: 1}))
	rr := httptest.NewRecorder()
	app.handleCreateUser(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}

	// Missing username/password
	body, _ = json.Marshal(map[string]string{"username": ""})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	app.handleCreateUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing credentials, got %d", rr.Code)
	}

	// Invalid JSON
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader([]byte("invalid json")))
	rr = httptest.NewRecorder()
	app.handleCreateUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid json, got %d", rr.Code)
	}

	// Invalid role
	body, _ = json.Marshal(map[string]string{"username": "u", "password": "password1234", "role": "invalid"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	app.handleCreateUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid role, got %d", rr.Code)
	}

	// Default role (viewer) and ErrDuplicateUsername
	body, _ = json.Marshal(map[string]string{"username": "dup", "password": "password1234"})
	mock.ExpectQuery(`INSERT INTO users`).WithArgs("dup", pgxmock.AnyArg(), "viewer").WillReturnError(&pgconn.PgError{Code: "23505"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	app.handleCreateUser(rr, req)
	if rr.Code != http.StatusConflict {
		t.Errorf("expected 409 for duplicate username, got %d", rr.Code)
	}

	// Password too short
	body, _ = json.Marshal(map[string]string{"username": "short", "password": "short"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	app.handleCreateUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short password, got %d", rr.Code)
	}

	// DB error
	body, _ = json.Marshal(map[string]string{"username": "err", "password": "password1234"})
	mock.ExpectQuery(`INSERT INTO users`).WithArgs("err", pgxmock.AnyArg(), "viewer").WillReturnError(errors.New("db err"))
	req = httptest.NewRequest(http.MethodPost, "/api/v1/users", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	app.handleCreateUser(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for db error, got %d", rr.Code)
	}
}

func TestHandleUpdateUser(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	role := "viewer"
	disabled := true
	password := "newpassword123"

	body, _ := json.Marshal(map[string]interface{}{
		"role":     &role,
		"disabled": &disabled,
		"password": &password,
	})

	mock.ExpectExec(`UPDATE users SET role = \$2`).WithArgs(int32(1), "viewer").WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`INSERT INTO audit_log`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	mock.ExpectExec(`UPDATE users SET disabled_at = \$2`).WithArgs(int32(1), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`INSERT INTO audit_log`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	mock.ExpectExec(`UPDATE users SET password_hash = \$2`).WithArgs(int32(1), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("UPDATE", 1))
	mock.ExpectExec(`INSERT INTO audit_log`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	req := httptest.NewRequest(http.MethodPatch, "/api/v1/users/1", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", UserID: 1}))
	rr := httptest.NewRecorder()
	app.handleUpdateUser(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Invalid ID string
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/users/abc", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "abc"})
	rr = httptest.NewRecorder()
	app.handleUpdateUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid id string, got %d", rr.Code)
	}

	// Missing ID in mux.Vars
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/users/", bytes.NewReader(body))
	rr = httptest.NewRecorder()
	app.handleUpdateUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing id var, got %d", rr.Code)
	}

	// Invalid body
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/users/1", bytes.NewReader([]byte("invalid json")))
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	rr = httptest.NewRecorder()
	app.handleUpdateUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid body, got %d", rr.Code)
	}

	// respondUserUpdateError: ErrUserNotFound
	body, _ = json.Marshal(map[string]interface{}{"role": "admin"})
	mock.ExpectExec(`UPDATE users SET role = \$2`).WithArgs(int32(100), "admin").WillReturnResult(pgxmock.NewResult("UPDATE", 0)) // 0 rows = ErrUserNotFound
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/users/100", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "100"})
	rr = httptest.NewRecorder()
	app.handleUpdateUser(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing user, got %d", rr.Code)
	}

	// respondUserUpdateError: ErrInvalidRole
	body, _ = json.Marshal(map[string]interface{}{"role": "invalid"})
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/users/1", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	rr = httptest.NewRecorder()
	app.handleUpdateUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid role, got %d", rr.Code)
	}

	// respondUserUpdateError: ErrPasswordTooShort
	body, _ = json.Marshal(map[string]interface{}{"password": "short"})
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/users/1", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	rr = httptest.NewRecorder()
	app.handleUpdateUser(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for short password, got %d", rr.Code)
	}

	// respondUserUpdateError: DB error
	body, _ = json.Marshal(map[string]interface{}{"role": "admin"})
	mock.ExpectExec(`UPDATE users SET role = \$2`).WithArgs(int32(1), "admin").WillReturnError(errors.New("db error"))
	req = httptest.NewRequest(http.MethodPatch, "/api/v1/users/1", bytes.NewReader(body))
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	rr = httptest.NewRecorder()
	app.handleUpdateUser(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for db error, got %d", rr.Code)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}

func TestHandleDeleteUser(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).WithArgs(int32(2)).WillReturnResult(pgxmock.NewResult("DELETE", 1))
	mock.ExpectExec(`INSERT INTO audit_log`).WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).WillReturnResult(pgxmock.NewResult("INSERT", 1))

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/users/2", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "2"})
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", UserID: 1}))
	rr := httptest.NewRecorder()
	app.handleDeleteUser(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d: %s", rr.Code, rr.Body.String())
	}

	// Delete self
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/users/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", UserID: 1}))
	rr = httptest.NewRecorder()
	app.handleDeleteUser(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 when deleting self, got %d", rr.Code)
	}

	// Delete non-existent user
	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).WithArgs(int32(3)).WillReturnResult(pgxmock.NewResult("DELETE", 0))
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/users/3", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "3"})
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", UserID: 1}))
	rr = httptest.NewRecorder()
	app.handleDeleteUser(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404 for missing user, got %d", rr.Code)
	}

	// DB error
	mock.ExpectExec(`DELETE FROM users WHERE id = \$1`).WithArgs(int32(4)).WillReturnError(errors.New("db error"))
	req = httptest.NewRequest(http.MethodDelete, "/api/v1/users/4", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "4"})
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", UserID: 1}))
	rr = httptest.NewRecorder()
	app.handleDeleteUser(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for db error, got %d", rr.Code)
	}
}

func TestHandleListAudit(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "occurred_at", "actor_user_id", "actor_label", "action", "target_type", "target_id", "request_id", "ip", "user_agent", "details"}).
		AddRow(int64(1), now, nil, "system", "test.action", "", "", "", "", "", []byte("{}"))

	mock.ExpectQuery(`SELECT (.+) FROM audit_log`).WithArgs(100).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	rr := httptest.NewRecorder()
	app.handleListAudit(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Query params
	mock.ExpectQuery(`SELECT (.+) FROM audit_log WHERE 1=1 AND action = \$1 AND target_type = \$2 AND target_id = \$3 ORDER BY occurred_at DESC LIMIT \$4`).
		WithArgs("login", "user", "2", 10).
		WillReturnRows(mock.NewRows([]string{"id", "occurred_at", "actor_user_id", "actor_label", "action", "target_type", "target_id", "request_id", "ip", "user_agent", "details"}))

	req = httptest.NewRequest(http.MethodGet, "/api/v1/audit?limit=10&action=login&target_type=user&target_id=2", nil)
	rr = httptest.NewRecorder()
	app.handleListAudit(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 with query params, got %d", rr.Code)
	}

	// DB error
	mock.ExpectQuery(`SELECT (.+) FROM audit_log`).WithArgs(100).WillReturnError(errors.New("db error"))

	req = httptest.NewRequest(http.MethodGet, "/api/v1/audit", nil)
	rr = httptest.NewRecorder()
	app.handleListAudit(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for db error, got %d", rr.Code)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
