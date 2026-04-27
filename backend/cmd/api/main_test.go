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
	ts := middleware.GetTokenStore()
	authConfig := middleware.NewAuthConfig()
	return &Application{
		DB:         nil, // Will not be used in unit tests
		TokenStore: ts,
		AuthConfig: authConfig,
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

	// Should set auth_token cookie
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

	// Should return a token in JSON body
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
	// ADMIN_USERNAME and ADMIN_PASSWORD not set
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

// --- authMiddleware tests ---

func TestAuthMiddleware_NoAuth(t *testing.T) {
	app := testApp(t)
	handler := app.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	app := testApp(t)
	handler := app.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

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

	// Store a valid token
	app.TokenStore.StoreToken("valid-test-token", "testuser", time.Hour)

	handler := app.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

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

	// Store a valid token
	app.TokenStore.StoreToken("cookie-token", "testuser", time.Hour)

	handler := app.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

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

	// Store a token that's already expired
	app.TokenStore.StoreToken("expired-token", "testuser", -time.Hour)

	handler := app.authMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer expired-token")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for expired token, got %d", rr.Code)
	}
}

// --- corsMiddleware tests ---

func TestCorsMiddleware_SetsHeaders(t *testing.T) {
	app := testApp(t)
	handler := app.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

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
	handler := app.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound) // Should never reach here for OPTIONS
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
	handler := app.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

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
	// An app with nil DB should return unhealthy
	// We can't test this without panic, so we skip
	// In production you'd use a mock DB interface
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

	// Without mux vars, the handler should return 400
	// We need to set up mux vars
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hosts/abc", nil)
	rr := httptest.NewRecorder()

	// Without mux routing, vars won't be set. We test the ID parsing separately.
	app.handleGetHost(rr, req)

	// Should be BadRequest because mux vars won't be found
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}