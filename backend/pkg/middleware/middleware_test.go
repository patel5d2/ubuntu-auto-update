package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSendErrorResponse_JSON(t *testing.T) {
	rr := httptest.NewRecorder()
	SendErrorResponse(rr, http.StatusBadRequest, "bad_request", "Invalid input", nil)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("got %q, want application/json", ct)
	}

	var resp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Error != "bad_request" {
		t.Errorf("got error %q, want bad_request", resp.Error)
	}
	if resp.Timestamp == "" || resp.Timestamp == "now" {
		t.Error("timestamp should be a real RFC3339 value, not empty or 'now'")
	}
}

func TestSendAuthError(t *testing.T) {
	rr := httptest.NewRecorder()
	SendAuthError(rr, "test message")
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rr.Code)
	}
}

func TestSendForbiddenError(t *testing.T) {
	rr := httptest.NewRecorder()
	SendForbiddenError(rr, "forbidden")
	if rr.Code != http.StatusForbidden {
		t.Errorf("got %d, want 403", rr.Code)
	}
}

func TestSendNotFoundError(t *testing.T) {
	rr := httptest.NewRecorder()
	SendNotFoundError(rr, "Host")
	if rr.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", rr.Code)
	}
	var resp ErrorResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if resp.Message != "Host not found" {
		t.Errorf("got %q, want 'Host not found'", resp.Message)
	}
}

func TestSendValidationError(t *testing.T) {
	rr := httptest.NewRecorder()
	SendValidationError(rr, "email", "invalid format")
	if rr.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", rr.Code)
	}
}

func TestErrorHandler_PanicRecovery(t *testing.T) {
	handler := ErrorHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("got %d, want 500 after panic", rr.Code)
	}
}

func TestGetCurrentTimestamp(t *testing.T) {
	ts := getCurrentTimestamp()
	if ts == "now" {
		t.Error("timestamp should not be literal 'now'")
	}
	if _, err := time.Parse(time.RFC3339, ts); err != nil {
		t.Errorf("timestamp %q is not valid RFC3339: %v", ts, err)
	}
}

// --- Token Store tests ---

func TestTokenStore_StoreAndValidate(t *testing.T) {
	store := &TokenStore{tokens: make(map[string]TokenEntry)}
	store.StoreToken("tok1", "user1", time.Hour)

	user, valid := store.ValidateToken("tok1")
	if !valid {
		t.Error("expected valid token")
	}
	if user != "user1" {
		t.Errorf("got user %q, want user1", user)
	}
}

func TestTokenStore_ExpiredToken(t *testing.T) {
	store := &TokenStore{tokens: make(map[string]TokenEntry)}
	store.StoreToken("tok1", "user1", -time.Hour) // Already expired

	_, valid := store.ValidateToken("tok1")
	if valid {
		t.Error("expected expired token to be invalid")
	}
}

func TestTokenStore_RemoveToken(t *testing.T) {
	store := &TokenStore{tokens: make(map[string]TokenEntry)}
	store.StoreToken("tok1", "user1", time.Hour)
	store.RemoveToken("tok1")

	_, valid := store.ValidateToken("tok1")
	if valid {
		t.Error("expected removed token to be invalid")
	}
}

func TestTokenStore_CleanExpired(t *testing.T) {
	store := &TokenStore{tokens: make(map[string]TokenEntry)}
	store.StoreToken("active", "user1", time.Hour)
	store.StoreToken("expired", "user2", -time.Hour)

	store.CleanExpiredTokens()

	if _, valid := store.ValidateToken("active"); !valid {
		t.Error("active token should still be valid")
	}
	if _, valid := store.ValidateToken("expired"); valid {
		t.Error("expired token should have been cleaned")
	}
}

func TestGenerateSecureToken(t *testing.T) {
	t1, err := GenerateSecureToken()
	if err != nil {
		t.Fatal(err)
	}
	t2, _ := GenerateSecureToken()
	if t1 == t2 {
		t.Error("tokens should be unique")
	}
	if len(t1) != 64 { // 32 bytes = 64 hex chars
		t.Errorf("token length %d, want 64", len(t1))
	}
}
