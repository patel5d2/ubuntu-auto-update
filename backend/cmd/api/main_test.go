package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func truncateTables(t *testing.T, db *pgxpool.Pool) {
	_, err := db.Exec(context.Background(), "TRUNCATE hosts, ssh_keys RESTART IDENTITY")
	if err != nil {
		t.Fatal(err)
	}
}

func newTestApplication(t *testing.T) *Application {
	pool, err := pgxpool.New(context.Background(), "postgres://user:password@localhost:5432/test_uau_db?sslmode=disable")
	if err != nil {
		t.Fatal(err)
	}

	return &Application{DB: pool}
}

func TestHandleLogin(t *testing.T) {
	app := newTestApplication(t)
	truncateTables(t, app.DB)

	t.Setenv("ADMIN_USERNAME", "admin")
	t.Setenv("ADMIN_PASSWORD", "password")

	// Create a request to pass to our handler.
	loginData := map[string]string{
		"username": "admin",
		"password": "password",
	}
	body, _ := json.Marshal(loginData)
	req, err := http.NewRequest("POST", "/api/v1/login", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to record the response.
	rr := httptest.NewRecorder()
	http.HandlerFunc(app.handleLogin).ServeHTTP(rr, req)

	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}
}

func TestHandleListHosts(t *testing.T) {
	app := newTestApplication(t)
	truncateTables(t, app.DB)

	// Insert a host into the test database
	_, err := app.DB.Exec(context.Background(), "INSERT INTO hosts (hostname, last_seen, update_output, upgrade_output, error) VALUES ('test-host', NOW(), '', '', '')")
	if err != nil {
		t.Fatal(err)
	}

	req, err := http.NewRequest("GET", "/api/v1/hosts", nil)
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	http.HandlerFunc(app.handleListHosts).ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check the response body
	expected := `[{"id":1,"hostname":"test-host","created_at":"...","updated_at":"...","last_seen":"...","update_output":"","upgrade_output":"","error":{"String":"","Valid":false}}]`
	// We don't check the full response body because the timestamps are dynamic.
	if !strings.Contains(rr.Body.String(), "test-host") {
		t.Errorf("handler returned unexpected body: got %v want %v",
			rr.Body.String(), expected)
	}
}