package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
	"ubuntu-auto-update/backend/pkg/config"
	"ubuntu-auto-update/backend/pkg/db"
	"ubuntu-auto-update/backend/pkg/middleware"
	"ubuntu-auto-update/backend/pkg/models"
	sshpkg "ubuntu-auto-update/backend/pkg/ssh"
	"ubuntu-auto-update/backend/pkg/webhook"
)

// maxRequestBodySize limits POST request bodies to 1MB.
const maxRequestBodySize = 1 << 20

type Application struct {
	DB             *pgxpool.Pool
	TokenStore     *middleware.TokenStore
	AuthConfig     *middleware.AuthConfig
	CORS           *middleware.CORSConfig
	SSHDialer      *sshpkg.Dialer
	WebhookSender  *webhook.Dispatcher
}

// dispatchWebhooks resolves subscribers for an event and queues deliveries.
// Returns immediately; deliveries run on the dispatcher's goroutines.
func (app *Application) dispatchWebhooks(event string, payload interface{}) {
	hooks, err := db.GetWebhooks(context.Background(), app.DB, event)
	if err != nil {
		log.Errorf("Failed to get webhooks for event %s: %v", event, err)
		return
	}
	for _, h := range hooks {
		app.WebhookSender.Deliver(context.Background(), h.URL, payload)
	}
}

func main() {
	if err := config.Load(); err != nil {
		log.Warnf("Config loading: %v (continuing with env vars)", err)
	}

	log.Info("Starting application...")
	ctx := context.Background()

	dbPool, err := db.NewConnection(ctx)
	if err != nil {
		log.Fatalf("Could not connect to database: %v", err)
	}
	defer dbPool.Close()

	tokenStore := middleware.GetTokenStore()
	authConfig := middleware.NewAuthConfig()
	middleware.StartTokenCleanup(tokenStore, 5*time.Minute)

	corsCfg := middleware.LoadCORSConfig()
	dispatcher := webhook.NewDispatcher()

	app := &Application{
		DB:            dbPool,
		TokenStore:    tokenStore,
		AuthConfig:    authConfig,
		CORS:          corsCfg,
		SSHDialer:     sshpkg.NewDialer(dbPool),
		WebhookSender: dispatcher,
	}

	r := mux.NewRouter()
	r.Use(middleware.ErrorHandler) // panic recovery + request logging
	r.Use(middleware.CORS(corsCfg))

	r.HandleFunc("/api/v1/health", app.handleHealth).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/enroll", app.handleEnroll).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/login", app.handleLogin).Methods(http.MethodPost, http.MethodOptions)
	r.HandleFunc("/api/v1/logout", app.handleLogout).Methods(http.MethodPost, http.MethodOptions)

	api := r.PathPrefix("/api/v1").Subrouter()
	api.Use(middleware.TokenAuthMiddleware(tokenStore, authConfig))
	api.HandleFunc("/report", app.handleReport).Methods(http.MethodPost)
	api.HandleFunc("/hosts", app.handleListHosts).Methods(http.MethodGet)
	api.HandleFunc("/hosts/{id}", app.handleGetHost).Methods(http.MethodGet)
	// WebSocket handshakes are GET-only; be explicit so the routes don't accept other verbs.
	api.HandleFunc("/hosts/{id}/preview-updates", app.handlePreviewUpdates).Methods(http.MethodGet)
	api.HandleFunc("/hosts/{id}/execute-script", app.handleExecuteScript).Methods(http.MethodGet)
	api.HandleFunc("/hosts/{id}/ssh-key", app.handleAddSSHKey).Methods(http.MethodPost)
	api.HandleFunc("/webhooks", app.handleAddWebhook).Methods(http.MethodPost)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		sig := <-sigChan
		log.Infof("Received signal %v, shutting down gracefully...", sig)

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			log.Errorf("Server shutdown error: %v", err)
		}
		dispatcher.Wait()
	}()

	log.Infof("Starting server on :%s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	log.Info("Server stopped")
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (app *Application) handleEnroll(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req struct {
		EnrollmentToken string `json:"enrollment_token"`
		Hostname        string `json:"hostname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	req.Hostname = strings.TrimSpace(req.Hostname)
	if req.Hostname == "" {
		http.Error(w, "Hostname cannot be empty", http.StatusBadRequest)
		return
	}

	enrollmentToken := os.Getenv("ENROLLMENT_TOKEN")
	if enrollmentToken == "" {
		log.Error("ENROLLMENT_TOKEN environment variable not set")
		http.Error(w, "Enrollment not configured", http.StatusInternalServerError)
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.EnrollmentToken), []byte(enrollmentToken)) != 1 {
		http.Error(w, "Invalid enrollment token", http.StatusUnauthorized)
		return
	}

	authToken, err := middleware.GenerateSecureToken()
	if err != nil {
		log.Errorf("Failed to generate token: %v", err)
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	app.TokenStore.StoreToken(authToken, "agent:"+req.Hostname, 365*24*time.Hour)
	log.Infof("Agent enrolled successfully: %s", req.Hostname)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": authToken})
}

func (app *Application) handleLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	adminUsername := os.Getenv("ADMIN_USERNAME")
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminUsername == "" || adminPassword == "" {
		log.Error("ADMIN_USERNAME or ADMIN_PASSWORD environment variables not set")
		writeJSONError(w, http.StatusInternalServerError, "Authentication not configured on server")
		return
	}

	userOK := subtle.ConstantTimeCompare([]byte(req.Username), []byte(adminUsername)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(req.Password), []byte(adminPassword)) == 1
	if !userOK || !passOK {
		writeJSONError(w, http.StatusUnauthorized, "Invalid credentials")
		return
	}

	authToken, err := middleware.GenerateSecureToken()
	if err != nil {
		log.Errorf("Failed to generate token: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	app.TokenStore.StoreToken(authToken, req.Username, 24*time.Hour)
	middleware.SetAuthCookie(w, app.AuthConfig, authToken)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"token": authToken})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// handleLogout invalidates the caller's token (if present) and clears the auth cookie.
func (app *Application) handleLogout(w http.ResponseWriter, r *http.Request) {
	if cookie, err := r.Cookie(app.AuthConfig.CookieName); err == nil && cookie.Value != "" {
		app.TokenStore.RemoveToken(cookie.Value)
	} else if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		app.TokenStore.RemoveToken(strings.TrimPrefix(h, "Bearer "))
	}
	middleware.ClearAuthCookie(w, app.AuthConfig)
	w.WriteHeader(http.StatusOK)
}

func (app *Application) handleReport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var report models.HostReport
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	report.Hostname = strings.TrimSpace(report.Hostname)
	if report.Hostname == "" {
		http.Error(w, "Hostname cannot be empty", http.StatusBadRequest)
		return
	}

	log.Infof("Received report from host: %s", report.Hostname)

	host, err := db.UpsertHost(r.Context(), app.DB, report.Hostname, "root", report.UpdateOutput, report.UpgradeOutput, "")
	if err != nil {
		log.Errorf("Failed to upsert host: %v", err)
		http.Error(w, "Failed to process report", http.StatusInternalServerError)
		return
	}

	log.Infof("Upserted host: %s (ID: %d)", host.Hostname, host.ID)
	w.WriteHeader(http.StatusAccepted)
}

func (app *Application) handleListHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := db.ListHosts(r.Context(), app.DB)
	if err != nil {
		log.Errorf("Failed to list hosts: %v", err)
		http.Error(w, "Failed to retrieve hosts", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(hosts)
}

func parseHostID(r *http.Request) (int32, error) {
	idStr, ok := mux.Vars(r)["id"]
	if !ok {
		return 0, errors.New("host id missing")
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return 0, err
	}
	return int32(id), nil
}

func (app *Application) handleGetHost(w http.ResponseWriter, r *http.Request) {
	id, err := parseHostID(r)
	if err != nil {
		http.Error(w, "Invalid host ID", http.StatusBadRequest)
		return
	}

	host, err := db.GetHost(r.Context(), app.DB, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Host not found", http.StatusNotFound)
		} else {
			log.Errorf("Failed to get host: %v", err)
			http.Error(w, "Failed to retrieve host", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(host)
}

// upgrader is used for WebSocket handshakes. CheckOrigin uses the cached
// CORSConfig captured in main, but the upgrader itself is created per request
// because it closes over the app pointer.
func (app *Application) wsUpgrader() websocket.Upgrader {
	return websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return app.CORS.IsAllowed(r.Header.Get("Origin"))
		},
	}
}

func (app *Application) handleAddWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req models.Webhook
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	req.URL = strings.TrimSpace(req.URL)
	req.Event = strings.TrimSpace(req.Event)
	if req.URL == "" || req.Event == "" {
		http.Error(w, "URL and event are required", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		http.Error(w, "URL must start with http:// or https://", http.StatusBadRequest)
		return
	}

	if _, err := app.DB.Exec(r.Context(), `INSERT INTO webhooks (url, event) VALUES ($1, $2)`, req.URL, req.Event); err != nil {
		log.Errorf("Failed to add webhook: %v", err)
		http.Error(w, "Failed to add webhook", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (app *Application) handleAddSSHKey(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	id, err := parseHostID(r)
	if err != nil {
		http.Error(w, "Invalid host ID", http.StatusBadRequest)
		return
	}

	var req struct {
		SshUser    string `json:"ssh_user"`
		PrivateKey string `json:"private_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	req.SshUser = strings.TrimSpace(req.SshUser)
	req.PrivateKey = strings.TrimSpace(req.PrivateKey)
	if req.SshUser == "" || req.PrivateKey == "" {
		http.Error(w, "ssh_user and private_key are required", http.StatusBadRequest)
		return
	}

	if err := db.AddSSHKey(r.Context(), app.DB, id, req.PrivateKey); err != nil {
		log.Errorf("Failed to add SSH key: %v", err)
		http.Error(w, "Failed to add SSH key", http.StatusInternalServerError)
		return
	}

	if _, err := app.DB.Exec(r.Context(), `UPDATE hosts SET ssh_user = $1 WHERE id = $2`, req.SshUser, id); err != nil {
		log.Errorf("Failed to update ssh_user: %v", err)
		http.Error(w, "Failed to update ssh_user", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (app *Application) handleExecuteScript(w http.ResponseWriter, r *http.Request) {
	id, err := parseHostID(r)
	if err != nil {
		http.Error(w, "Invalid host ID", http.StatusBadRequest)
		return
	}

	upgrader := app.wsUpgrader()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade to websocket: %v", err)
		return
	}
	defer conn.Close()

	_, script, err := conn.ReadMessage()
	if err != nil {
		log.Errorf("Failed to read script from websocket: %v", err)
		return
	}

	sshClient, _, err := app.SSHDialer.ConnectToHost(r.Context(), id)
	if err != nil {
		log.Errorf("SSH connect to host %d failed: %v", id, err)
		conn.WriteMessage(websocket.TextMessage, []byte("SSH connect failed: "+err.Error()))
		return
	}
	defer sshClient.Close()

	session, err := sshClient.NewSession()
	if err != nil {
		log.Errorf("Failed to create SSH session: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to create SSH session: "+err.Error()))
		return
	}
	defer session.Close()

	output, err := session.CombinedOutput(string(script))
	if err != nil {
		log.Errorf("Script execution failed: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Script execution failed: %s", err.Error())))
	}
	conn.WriteMessage(websocket.TextMessage, output)
}

// handlePreviewUpdates runs `apt list --upgradable` over SSH and streams output
// back to the client. It does NOT actually upgrade packages — that requires
// sudo and an explicit user-driven action; see issue #14 in the audit.
func (app *Application) handlePreviewUpdates(w http.ResponseWriter, r *http.Request) {
	id, err := parseHostID(r)
	if err != nil {
		http.Error(w, "Invalid host ID", http.StatusBadRequest)
		return
	}

	upgrader := app.wsUpgrader()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade to websocket: %v", err)
		return
	}
	defer conn.Close()

	sshClient, host, err := app.SSHDialer.ConnectToHost(r.Context(), id)
	if err != nil {
		log.Errorf("SSH connect to host %d failed: %v", id, err)
		if host.Hostname != "" {
			db.UpsertHost(r.Context(), app.DB, host.Hostname, host.SshUser, "", "", err.Error())
		}
		app.dispatchWebhooks("update_failure", map[string]interface{}{"host_id": id, "error": err.Error()})
		conn.WriteMessage(websocket.TextMessage, []byte("SSH connect failed: "+err.Error()))
		return
	}
	defer sshClient.Close()

	commands := []string{
		"echo 'Starting Ubuntu update check...'",
		"apt list --upgradable",
		"echo 'Update check completed. Note: Actual updates require sudo privileges.'",
	}

	for _, cmd := range commands {
		session, err := sshClient.NewSession()
		if err != nil {
			log.Errorf("Failed to create SSH session: %v", err)
			db.UpsertHost(r.Context(), app.DB, host.Hostname, host.SshUser, "", "", err.Error())
			app.dispatchWebhooks("update_failure", map[string]interface{}{"host_id": id, "error": err.Error()})
			conn.WriteMessage(websocket.TextMessage, []byte("Failed to create SSH session: "+err.Error()))
			return
		}

		output, err := session.CombinedOutput(cmd)
		session.Close()
		if err != nil {
			log.Errorf("Failed to run command %q: %v", cmd, err)
			db.UpsertHost(r.Context(), app.DB, host.Hostname, host.SshUser, "", string(output), err.Error())
			app.dispatchWebhooks("update_failure", map[string]interface{}{
				"host_id": id, "command": cmd, "error": err.Error(), "output": string(output),
			})
			conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Failed to run command %q: %s", cmd, err.Error())))
			conn.WriteMessage(websocket.TextMessage, output)
			return
		}

		conn.WriteMessage(websocket.TextMessage, output)
		db.UpsertHost(r.Context(), app.DB, host.Hostname, host.SshUser, string(output), "", "")
	}

	app.dispatchWebhooks("preview_success", map[string]interface{}{"host_id": id})
}

func (app *Application) handleHealth(w http.ResponseWriter, r *http.Request) {
	if err := app.DB.Ping(r.Context()); err != nil {
		log.Errorf("Database health check failed: %v", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"status":    "unhealthy",
			"database":  "disconnected",
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "healthy",
		"database":  "connected",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
