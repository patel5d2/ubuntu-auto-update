package webhook

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSend_Success(t *testing.T) {
	skipSSRFCheck = true
	defer func() { skipSSRFCheck = false }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if ct := r.Header.Get("Content-Type"); ct != "application/json" {
			t.Errorf("expected application/json, got %s", ct)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	err := Send(server.URL, map[string]string{"key": "value"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSend_ServerError(t *testing.T) {
	skipSSRFCheck = true
	defer func() { skipSSRFCheck = false }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	err := Send(server.URL, map[string]string{"key": "value"})
	if err == nil {
		t.Error("expected error for server error response")
	}
}

func TestSend_NetworkError(t *testing.T) {
	skipSSRFCheck = true
	defer func() { skipSSRFCheck = false }()

	err := Send("http://127.0.0.1:1", map[string]string{"key": "value"})
	if err == nil {
		t.Error("expected error for unreachable server")
	}
}

func TestSendWithContext_Cancellation(t *testing.T) {
	skipSSRFCheck = true
	defer func() { skipSSRFCheck = false }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := SendWithContext(ctx, server.URL, map[string]string{"key": "value"})
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestSend_InvalidPayload(t *testing.T) {
	// Channels cannot be marshaled to JSON — SSRF check runs first for
	// non-localhost URLs, but here the error is from the marshal step.
	skipSSRFCheck = true
	defer func() { skipSSRFCheck = false }()

	err := Send("http://localhost", make(chan int))
	if err == nil {
		t.Error("expected error for unmarshalable payload")
	}
}

func TestIsSafeURL_Loopback(t *testing.T) {
	if err := IsSafeURL("http://127.0.0.1/test"); err == nil {
		t.Error("expected error for loopback URL")
	}
}

func TestIsSafeURL_PrivateIP(t *testing.T) {
	if err := IsSafeURL("http://192.168.1.1/test"); err == nil {
		t.Error("expected error for private IP URL")
	}
}

func TestIsSafeURL_BadScheme(t *testing.T) {
	if err := IsSafeURL("ftp://example.com"); err == nil {
		t.Error("expected error for non-http scheme")
	}
}
