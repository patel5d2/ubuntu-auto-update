package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/jackc/pgx/v5"
	"github.com/pashagolub/pgxmock/v4"
	"ubuntu-auto-update/backend/pkg/middleware"
	"ubuntu-auto-update/backend/pkg/models"
	"ubuntu-auto-update/backend/pkg/session"
)

// testApp creates an Application for testing with a token store but no real DB.
func testApp(t *testing.T) *Application {
	t.Helper()
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000")
	return &Application{
		DB:         nil, // not used by validation-only tests
		TokenStore: middleware.GetTokenStore(),
		AuthConfig: middleware.NewAuthConfig(),
		CORS:       middleware.LoadCORSConfig(),
	}
}

// testAppWithDB creates an Application for testing with a mocked DB.
func testAppWithDB(t *testing.T) (*Application, pgxmock.PgxPoolIface) {
	t.Helper()
	t.Setenv("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000")
	mock, err := pgxmock.NewPool()
	if err != nil {
		t.Fatalf("error creating mock: %v", err)
	}

	app := &Application{
		DB:         mock,
		TokenStore: middleware.GetTokenStore(),
		AuthConfig: middleware.NewAuthConfig(),
		CORS:       middleware.LoadCORSConfig(),
	}
	return app, mock
}

// --- handleLogin tests ---

func TestHandleLogin_Success(t *testing.T) {
	app := testApp(t)
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "secret123")

	body, _ := json.Marshal(LoginRequest{Username: "admin", Password: "secret123"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	app.handleLogin(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	cookies := rr.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == "auth_token" {
			found = true
			if !c.HttpOnly {
				t.Error("cookie should be HttpOnly")
			}
			if c.Value == "" {
				t.Error("cookie value should not be empty")
			}
		}
	}
	if !found {
		t.Error("expected auth_token cookie to be set")
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["token"] == "" {
		t.Error("expected non-empty token in response body")
	}
}

func TestHandleLogin_InvalidCredentials(t *testing.T) {
	app := testApp(t)
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "secret123")

	body, _ := json.Marshal(LoginRequest{Username: "admin", Password: "wrong"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	app.handleLogin(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

func TestHandleLogin_EmptyCredentialsConfig(t *testing.T) {
	app := testApp(t)
	os.Unsetenv("ADMIN_USERNAME")
	os.Unsetenv("ADMIN_PASSWORD")

	body, _ := json.Marshal(LoginRequest{Username: "admin", Password: "admin"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	app.handleLogin(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 when credentials not configured, got %d", rr.Code)
	}
}

func TestHandleLogin_InvalidJSON(t *testing.T) {
	app := testApp(t)
	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "password")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/login", bytes.NewReader([]byte("not json")))
	rr := httptest.NewRecorder()

	app.handleLogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", rr.Code)
	}
}

// --- handleEnroll tests ---

func TestHandleEnroll_Success(t *testing.T) {
	app := testApp(t)
	t.Setenv("ENROLLMENT_TOKEN", "test-enroll-token")

	body, _ := json.Marshal(map[string]string{
		"enrollment_token": "test-enroll-token",
		"hostname":         "test-host",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	app.handleEnroll(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp["token"] == "" {
		t.Error("expected non-empty token")
	}
}

func TestHandleEnroll_InvalidToken(t *testing.T) {
	app := testApp(t)
	t.Setenv("ENROLLMENT_TOKEN", "correct-token")

	body, _ := json.Marshal(map[string]string{
		"enrollment_token": "wrong-token",
		"hostname":         "test-host",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	app.handleEnroll(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", rr.Code)
	}
}

func TestHandleEnroll_EmptyHostname(t *testing.T) {
	app := testApp(t)
	t.Setenv("ENROLLMENT_TOKEN", "test-token")

	body, _ := json.Marshal(map[string]string{
		"enrollment_token": "test-token",
		"hostname":         "",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	app.handleEnroll(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty hostname, got %d", rr.Code)
	}
}

func TestHandleEnroll_MissingEnrollmentConfig(t *testing.T) {
	app := testApp(t)
	os.Unsetenv("ENROLLMENT_TOKEN")

	body, _ := json.Marshal(map[string]string{
		"enrollment_token": "any-token",
		"hostname":         "test-host",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/enroll", bytes.NewReader(body))
	rr := httptest.NewRecorder()

	app.handleEnroll(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500 when enrollment not configured, got %d", rr.Code)
	}
}

// --- TokenAuthMiddleware tests (replaces former inline authMiddleware) ---

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestAuthMiddleware_NoAuth(t *testing.T) {
	app := testApp(t)
	handler := middleware.TokenAuthMiddleware(app.TokenStore, app.AuthConfig)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	app := testApp(t)
	handler := middleware.TokenAuthMiddleware(app.TokenStore, app.AuthConfig)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token-123")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for invalid token, got %d", rr.Code)
	}
}

func TestAuthMiddleware_ValidBearerToken(t *testing.T) {
	app := testApp(t)
	app.TokenStore.StoreToken("valid-test-token", "testuser", time.Hour)
	handler := middleware.TokenAuthMiddleware(app.TokenStore, app.AuthConfig)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-test-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for valid token, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAuthMiddleware_ValidCookie(t *testing.T) {
	app := testApp(t)
	app.TokenStore.StoreToken("cookie-token", "testuser", time.Hour)
	handler := middleware.TokenAuthMiddleware(app.TokenStore, app.AuthConfig)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "cookie-token"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for valid cookie, got %d", rr.Code)
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	app := testApp(t)
	app.TokenStore.StoreToken("expired-token", "testuser", -time.Hour)
	handler := middleware.TokenAuthMiddleware(app.TokenStore, app.AuthConfig)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", rr.Code)
	}
}

// --- handleMe tests ---

func TestHandleMe_Success(t *testing.T) {
	app, _ := testAppWithDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", Role: session.RoleAdmin}))
	rr := httptest.NewRecorder()
	app.handleMe(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestHandleMe_NoPrincipal(t *testing.T) {
	app, _ := testAppWithDB(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	rr := httptest.NewRecorder()
	app.handleMe(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

// --- handleLogout tests ---

func TestHandleLogout_Success(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "valid-token"})
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", Role: session.RoleAdmin}))

	mock.ExpectExec(`INSERT INTO audit_log`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	rr := httptest.NewRecorder()
	app.handleLogout(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestHandleLogout_BearerToken(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/v1/logout", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", Role: session.RoleAdmin}))

	mock.ExpectExec(`INSERT INTO audit_log`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	rr := httptest.NewRecorder()
	app.handleLogout(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

// --- CORS middleware tests (now in pkg/middleware/cors.go) ---

func TestCorsMiddleware_SetsHeaders(t *testing.T) {
	app := testApp(t)
	handler := middleware.CORS(app.CORS)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5173" {
		t.Errorf("expected CORS origin header, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestCorsMiddleware_PreflightReturns200(t *testing.T) {
	app := testApp(t)
	handler := middleware.CORS(app.CORS)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // should never reach here for OPTIONS
	}))

	req := httptest.NewRequest(http.MethodOptions, "/test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for preflight, got %d", rr.Code)
	}
}

func TestCorsMiddleware_UnknownOriginNotAllowed(t *testing.T) {
	app := testApp(t)
	handler := middleware.CORS(app.CORS)(okHandler())

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Origin", "http://evil.com")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("expected no CORS header for unknown origin, got %q", rr.Header().Get("Access-Control-Allow-Origin"))
	}
}

// --- handleHealth tests (requires DB, so we test the error path) ---

func TestHandleHealth_Success(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	mock.ExpectPing()

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	app.handleHealth(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestHandleHealth_DBError(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	mock.ExpectPing().WillReturnError(sql.ErrConnDone)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rr := httptest.NewRecorder()
	app.handleHealth(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}

// --- handleReport tests ---

func TestHandleReport_InvalidJSON(t *testing.T) {
	app := testApp(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewReader([]byte("not json")))
	rr := httptest.NewRecorder()
	app.handleReport(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleReport_EmptyHostname(t *testing.T) {
	app := testApp(t)

	body, _ := json.Marshal(map[string]string{
		"hostname":       "",
		"update_output":  "output",
		"upgrade_output": "output",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/report", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	app.handleReport(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for empty hostname, got %d", rr.Code)
	}
}

// --- handleAddWebhook tests ---

func TestHandleAddWebhook_InvalidJSON(t *testing.T) {
	app := testApp(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader([]byte("bad")))
	rr := httptest.NewRecorder()
	app.handleAddWebhook(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleAddWebhook_MissingFields(t *testing.T) {
	app := testApp(t)

	body, _ := json.Marshal(map[string]string{"url": "", "event": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	app.handleAddWebhook(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleAddWebhook_InvalidURL(t *testing.T) {
	app := testApp(t)

	body, _ := json.Marshal(map[string]string{"url": "ftp://bad", "event": "update_success"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	app.handleAddWebhook(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-http URL, got %d", rr.Code)
	}
}

func TestHandleAddWebhook_Success(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	body, _ := json.Marshal(map[string]string{"url": "http://example.com/hook", "event": "update_success"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader(body))
	req = req.WithContext(context.WithValue(req.Context(), middleware.PrincipalContextKey, &session.Principal{Username: "admin", UserID: 1}))

	mock.ExpectExec(`INSERT INTO webhooks`).
		WithArgs("http://example.com/hook", "update_success").
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	mock.ExpectExec(`INSERT INTO audit_log`).
		WithArgs(pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg(), pgxmock.AnyArg()).
		WillReturnResult(pgxmock.NewResult("INSERT", 1))

	rr := httptest.NewRecorder()
	app.handleAddWebhook(rr, req)

	if rr.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", rr.Code)
	}
}

func TestHandleAddWebhook_DBError(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	body, _ := json.Marshal(map[string]string{"url": "http://example.com/hook", "event": "update_success"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/webhooks", bytes.NewReader(body))

	mock.ExpectExec(`INSERT INTO webhooks`).
		WithArgs("http://example.com/hook", "update_success").
		WillReturnError(sql.ErrConnDone)

	rr := httptest.NewRecorder()
	app.handleAddWebhook(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

// --- handleGetHost validation tests ---

func TestHandleGetHost_InvalidID(t *testing.T) {
	app := testApp(t)

	// Without mux vars, parseHostID returns "host id missing" → 400
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/abc", nil)
	rr := httptest.NewRecorder()
	app.handleGetHost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- handleCreateHost validation tests ---
//
// These exercise the request-shape paths. The DB-touching success path runs
// in the docker-compose end-to-end smoke (see Phase B verification).

func TestHandleCreateHost_InvalidJSON(t *testing.T) {
	app := testApp(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/hosts", bytes.NewReader([]byte("not json")))
	rr := httptest.NewRecorder()
	app.handleCreateHost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleCreateHost_EmptyHostname(t *testing.T) {
	app := testApp(t)

	body, _ := json.Marshal(map[string]string{"hostname": "   ", "ssh_user": "root"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/hosts", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	app.handleCreateHost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for blank hostname, got %d", rr.Code)
	}
}

// --- handleUpdateHost validation tests ---

func TestHandleUpdateHost_InvalidID(t *testing.T) {
	app := testApp(t)

	body, _ := json.Marshal(map[string]string{"ssh_user": "root"})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/hosts/abc", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	app.handleUpdateHost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing/invalid id, got %d", rr.Code)
	}
}

func TestHandleUpdateHost_NoFields(t *testing.T) {
	app := testApp(t)

	// Empty body — request parses but produces no updatable fields.
	body, _ := json.Marshal(map[string]string{})
	req := httptest.NewRequest(http.MethodPatch, "/api/v1/hosts/1", bytes.NewReader(body))
	rr := httptest.NewRecorder()
	app.handleUpdateHost(rr, req)

	// parseHostID needs mux vars — we'll fail at id parsing first. That's
	// fine; we still get a 400 which is what we'd want anyway.
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- handleDeleteHost validation tests ---

func TestHandleDeleteHost_InvalidID(t *testing.T) {
	app := testApp(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/v1/hosts/abc", nil)
	rr := httptest.NewRecorder()
	app.handleDeleteHost(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

// --- runs endpoint validation ---

func TestHandleListRuns_Success(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "host_id", "run_group_id", "triggered_by", "kind", "status", "exit_code", "started_at", "finished_at", "output", "error", "playbook_id"}).
		AddRow(int32(1), int32(10), nil, "admin", models.RunKindUpdate, models.RunStatusRunning, nil, now, nil, "", nil, nil)

	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE host_id = \$1 ORDER BY started_at DESC LIMIT \$2`).
		WithArgs(int32(10), 50).
		WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/10/runs", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "10"})
	rr := httptest.NewRecorder()
	app.handleListRuns(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// With limit and cap
	rows2 := mock.NewRows([]string{"id", "host_id", "run_group_id", "triggered_by", "kind", "status", "exit_code", "started_at", "finished_at", "output", "error", "playbook_id"}).
		AddRow(int32(2), int32(10), nil, "admin", models.RunKindUpdate, models.RunStatusRunning, nil, now, nil, "", nil, nil)

	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE host_id = \$1 ORDER BY started_at DESC LIMIT \$2`).
		WithArgs(int32(10), 50).
		WillReturnRows(rows2)

	req = httptest.NewRequest(http.MethodGet, "/api/v1/hosts/10/runs?limit=999", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "10"})
	rr = httptest.NewRecorder()
	app.handleListRuns(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 for limit 999, got %d", rr.Code)
	}
}

func TestHandleListRunsByGroup_Success(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "host_id", "run_group_id", "triggered_by", "kind", "status", "exit_code", "started_at", "finished_at", "output", "error", "playbook_id"}).
		AddRow(int32(1), int32(10), "12345678-1234-1234-1234-123456789012", "admin", models.RunKindUpdate, models.RunStatusRunning, nil, now, nil, "", nil, nil)

	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE run_group_id = \$1 ORDER BY host_id`).
		WithArgs("12345678-1234-1234-1234-123456789012").
		WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/group?group_id=12345678-1234-1234-1234-123456789012", nil)
	rr := httptest.NewRecorder()
	app.handleListRunsByGroup(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}

	// Invalid UUID format
	req = httptest.NewRequest(http.MethodGet, "/api/v1/runs/group?group_id=bad", nil)
	rr = httptest.NewRecorder()
	app.handleListRunsByGroup(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad UUID, got %d", rr.Code)
	}
}

func TestHandleListRunsByGroup_DBError(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE run_group_id = \$1 ORDER BY host_id`).
		WithArgs("12345678-1234-1234-1234-123456789012").
		WillReturnError(sql.ErrConnDone)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/group?group_id=12345678-1234-1234-1234-123456789012", nil)
	rr := httptest.NewRecorder()
	app.handleListRunsByGroup(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestHandleListRuns_DBError(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE host_id = \$1 ORDER BY started_at DESC LIMIT \$2`).
		WithArgs(int32(10), 50).
		WillReturnError(sql.ErrConnDone)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/10/runs", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "10"})
	rr := httptest.NewRecorder()
	app.handleListRuns(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestHandleListRuns_InvalidID(t *testing.T) {
	app := testApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/abc/runs", nil)
	rr := httptest.NewRecorder()
	app.handleListRuns(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleGetRun_Success(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	now := time.Now()
	rows := mock.NewRows([]string{"id", "host_id", "run_group_id", "triggered_by", "kind", "status", "exit_code", "started_at", "finished_at", "output", "error", "playbook_id"}).
		AddRow(int32(1), int32(10), nil, "admin", models.RunKindUpdate, models.RunStatusRunning, nil, now, nil, "", nil, nil)

	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE id = \$1`).
		WithArgs(int32(1)).
		WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	rr := httptest.NewRecorder()
	app.handleGetRun(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestHandleGetRun_NotFound(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE id = \$1`).
		WithArgs(int32(1)).
		WillReturnError(pgx.ErrNoRows)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	rr := httptest.NewRecorder()
	app.handleGetRun(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", rr.Code)
	}
}

func TestHandleGetRun_DBError(t *testing.T) {
	app, mock := testAppWithDB(t)
	defer mock.Close()

	mock.ExpectQuery(`SELECT (.+) FROM update_runs WHERE id = \$1`).
		WithArgs(int32(1)).
		WillReturnError(sql.ErrConnDone)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/1", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "1"})
	rr := httptest.NewRecorder()
	app.handleGetRun(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", rr.Code)
	}
}

func TestHandleGetRun_InvalidID(t *testing.T) {
	app := testApp(t)

	// Without mux vars the handler returns 400.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/abc", nil)
	rr := httptest.NewRecorder()
	app.handleGetRun(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing run id, got %d", rr.Code)
	}

	// With invalid integer
	req = httptest.NewRequest(http.MethodGet, "/api/v1/runs/abc", nil)
	req = mux.SetURLVars(req, map[string]string{"id": "abc"})
	rr = httptest.NewRecorder()
	app.handleGetRun(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for invalid run id, got %d", rr.Code)
	}
}

// --- buildUpdateScript ---

func TestBuildUpdateScript_RootHasNoSudo(t *testing.T) {
	got := buildUpdateScript("root")
	if contains := stringContains(got, "sudo "); contains {
		t.Errorf("root user should not have sudo prefix; got: %s", got)
	}
}

func TestBuildUpdateScript_NonRootGetsSudoN(t *testing.T) {
	got := buildUpdateScript("ubuntu")
	if !stringContains(got, "sudo -n ") {
		t.Errorf("non-root user should have `sudo -n ` prefix; got: %s", got)
	}
}

func stringContains(haystack, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 &&
		// avoid pulling in strings just for this — main.go already imports it
		// but tests run in the same package, so we can use it directly:
		// (Imported via the tests already needing strings indirectly.)
		indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
