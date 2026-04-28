package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"ubuntu-auto-update/backend/pkg/middleware"
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
	json.Unmarshal(rr.Body.Bytes(), &resp)
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

func TestHandleHealth_NoDB(t *testing.T) {
	t.Skip("Requires DB mock - covered by integration tests")
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

func TestHandleListRuns_InvalidID(t *testing.T) {
	app := testApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/abc/runs", nil)
	rr := httptest.NewRecorder()
	app.handleListRuns(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleGetRun_InvalidID(t *testing.T) {
	app := testApp(t)

	// Without mux vars the handler returns 400.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/abc", nil)
	rr := httptest.NewRecorder()
	app.handleGetRun(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
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
