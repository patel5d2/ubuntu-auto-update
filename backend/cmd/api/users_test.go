package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pashagolub/pgxmock/v4"
	"ubuntu-auto-update/backend/pkg/middleware"
)

func testAppWithDB(t *testing.T) (*Application, pgxmock.PgxPoolIface) {
	t.Helper()
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}

	app := &Application{
		DB:         mock,
		TokenStore: middleware.GetTokenStore(),
		AuthConfig: middleware.NewAuthConfig(),
	}
	return app, mock
}

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
	req = req.WithContext(context.WithValue(req.Context(), middleware.UserContextKey, &middleware.User{Username: "admin"}))
	rr := httptest.NewRecorder()
	app.handleCreateUser(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", rr.Code, rr.Body.String())
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unfulfilled expectations: %v", err)
	}
}
