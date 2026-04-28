package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
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
	api.HandleFunc("/hosts", app.handleCreateHost).Methods(http.MethodPost)
	api.HandleFunc("/hosts/{id}", app.handleGetHost).Methods(http.MethodGet)
	api.HandleFunc("/hosts/{id}", app.handleUpdateHost).Methods(http.MethodPatch)
	api.HandleFunc("/hosts/{id}", app.handleDeleteHost).Methods(http.MethodDelete)
	// WebSocket handshakes are GET-only; be explicit so the routes don't accept other verbs.
	api.HandleFunc("/hosts/{id}/preview-updates", app.handlePreviewUpdates).Methods(http.MethodGet)
	api.HandleFunc("/hosts/{id}/run-update", app.handleRunUpdate).Methods(http.MethodGet)
	api.HandleFunc("/hosts/{id}/execute-script", app.handleExecuteScript).Methods(http.MethodGet)
	api.HandleFunc("/hosts/{id}/ssh-key", app.handleAddSSHKey).Methods(http.MethodPost)
	api.HandleFunc("/hosts/{id}/runs", app.handleListRuns).Methods(http.MethodGet)
	api.HandleFunc("/runs/{id}", app.handleGetRun).Methods(http.MethodGet)
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

// handleCreateHost lets an operator create a host record without going
// through agent enrollment. Returns 409 Conflict if the hostname already
// exists.
func (app *Application) handleCreateHost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req struct {
		Hostname string `json:"hostname"`
		SshUser  string `json:"ssh_user"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.Hostname = strings.TrimSpace(req.Hostname)
	req.SshUser = strings.TrimSpace(req.SshUser)
	if req.Hostname == "" {
		writeJSONError(w, http.StatusBadRequest, "Hostname is required")
		return
	}
	if req.SshUser == "" {
		req.SshUser = "root"
	}

	host, err := db.CreateHost(r.Context(), app.DB, req.Hostname, req.SshUser)
	if err != nil {
		if errors.Is(err, db.ErrDuplicateHostname) {
			writeJSONError(w, http.StatusConflict, "A host with that hostname already exists")
			return
		}
		log.Errorf("Failed to create host: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to create host")
		return
	}

	log.Infof("Operator created host: %s (ID: %d)", host.Hostname, host.ID)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(host)
}

// handleUpdateHost applies a partial update to a host. Only ssh_user is
// editable today; hostname is the natural key and changing it would break
// the agent-report upsert path.
func (app *Application) handleUpdateHost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	id, err := parseHostID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid host ID")
		return
	}

	var req struct {
		SshUser *string `json:"ssh_user,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if req.SshUser == nil {
		writeJSONError(w, http.StatusBadRequest, "Nothing to update; only ssh_user is editable")
		return
	}
	sshUser := strings.TrimSpace(*req.SshUser)
	if sshUser == "" {
		writeJSONError(w, http.StatusBadRequest, "ssh_user cannot be empty")
		return
	}

	host, err := db.UpdateHostSSHUser(r.Context(), app.DB, id, sshUser)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Host not found")
			return
		}
		log.Errorf("Failed to update host: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to update host")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(host)
}

// handleDeleteHost removes a host and (via ON DELETE CASCADE) its SSH key.
// To prevent click-through accidents and replay-style CSRF on long-lived
// sessions, the client must echo the hostname in X-Confirm-Hostname.
func (app *Application) handleDeleteHost(w http.ResponseWriter, r *http.Request) {
	id, err := parseHostID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid host ID")
		return
	}

	host, err := db.GetHost(r.Context(), app.DB, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Host not found")
			return
		}
		log.Errorf("Failed to look up host before delete: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve host")
		return
	}

	if r.Header.Get("X-Confirm-Hostname") != host.Hostname {
		writeJSONError(w, http.StatusPreconditionFailed,
			"X-Confirm-Hostname header must match the host's hostname")
		return
	}

	rows, err := db.DeleteHost(r.Context(), app.DB, id)
	if err != nil {
		log.Errorf("Failed to delete host %d: %v", id, err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to delete host")
		return
	}
	if rows == 0 {
		writeJSONError(w, http.StatusNotFound, "Host not found")
		return
	}

	log.Infof("Deleted host: %s (ID: %d)", host.Hostname, id)
	w.WriteHeader(http.StatusNoContent)
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

// previewCommands runs read-only and never escalates privileges.
var previewCommands = []string{
	"echo '== ubuntu-auto-update: preview =='",
	"apt list --upgradable",
}

// updateCommandTemplate is built per-host because it changes based on whether
// the configured ssh_user is root. The flags neutralize the most common
// dpkg interactive prompts during apt-get upgrade.
const aptNoninteractive = `DEBIAN_FRONTEND=noninteractive ` +
	`apt-get -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" -y `

// buildUpdateScript returns the shell line that does an actual upgrade.
// Non-root users get a `sudo -n` prefix so the operation fails fast (rather
// than hanging on a password prompt) when passwordless sudo isn't set up.
func buildUpdateScript(sshUser string) string {
	prefix := ""
	if sshUser != "" && sshUser != "root" {
		prefix = "sudo -n "
	}
	return "set -o pipefail; " +
		"echo '== ubuntu-auto-update: update =='; " +
		prefix + aptNoninteractive + "update && " +
		prefix + aptNoninteractive + "upgrade"
}

// handlePreviewUpdates runs read-only `apt list --upgradable` over SSH and
// streams output back to the client. Persists a 'preview' update_runs row
// for history.
func (app *Application) handlePreviewUpdates(w http.ResponseWriter, r *http.Request) {
	id, err := parseHostID(r)
	if err != nil {
		http.Error(w, "Invalid host ID", http.StatusBadRequest)
		return
	}
	app.runHostCommand(w, r, id, models.RunKindPreview, previewCommands)
}

// handleRunUpdate runs an actual `apt-get upgrade -y` over SSH. This is the
// "single click to update" entry point — it changes system state, so the
// frontend gates it behind a confirmation dialog.
func (app *Application) handleRunUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := parseHostID(r)
	if err != nil {
		http.Error(w, "Invalid host ID", http.StatusBadRequest)
		return
	}
	// Look up the host first so we know which ssh_user is configured; that
	// controls whether the script needs `sudo -n`.
	host, err := db.GetHost(r.Context(), app.DB, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Host not found", http.StatusNotFound)
			return
		}
		log.Errorf("Failed to get host %d: %v", id, err)
		http.Error(w, "Failed to retrieve host", http.StatusInternalServerError)
		return
	}
	app.runHostCommand(w, r, id, models.RunKindUpdate, []string{buildUpdateScript(host.SshUser)})
}

// runHostCommand is the shared engine for preview/update WebSockets. It:
//   - upgrades to a WebSocket
//   - inserts an update_runs row in 'running'
//   - SSHes, executes commands, tees stdout/stderr to (a) the websocket and
//     (b) the run row, with the row's output column capped at 1 MiB
//   - marks the run terminal in all exit paths
func (app *Application) runHostCommand(w http.ResponseWriter, r *http.Request, hostID int32, kind models.RunKind, commands []string) {
	upgrader := app.wsUpgrader()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade to websocket: %v", err)
		return
	}
	defer conn.Close()

	user := middleware.GetUserFromContext(r)
	triggeredBy := "unknown"
	if user != nil {
		triggeredBy = user.Username
	}

	// Decoupled context for DB writes: if the websocket disconnects mid-run,
	// we still want to persist the final status. Use a fresh background ctx
	// that we cancel ourselves on hard exit.
	dbCtx, cancelDB := context.WithCancel(context.Background())
	defer cancelDB()

	run, err := db.CreateRun(dbCtx, app.DB, hostID, triggeredBy, kind)
	if err != nil {
		log.Errorf("Failed to create run row: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to create run record: "+err.Error()))
		return
	}
	emit(conn, fmt.Sprintf("[run #%d started by %s]\n", run.ID, triggeredBy))

	finishStatus := models.RunStatusFailed
	finishExit := -1
	finishErr := ""
	defer func() {
		if err := db.FinishRun(dbCtx, app.DB, run.ID, finishStatus, finishExit, finishErr); err != nil {
			log.Errorf("Failed to mark run %d terminal: %v", run.ID, err)
		}
		emit(conn, fmt.Sprintf("\n[run #%d finished: %s]\n", run.ID, finishStatus))
	}()

	sshClient, host, err := app.SSHDialer.ConnectToHost(r.Context(), hostID)
	if err != nil {
		finishErr = fmt.Sprintf("ssh connect: %v", err)
		log.Errorf("SSH connect to host %d failed: %v", hostID, err)
		emit(conn, "SSH connect failed: "+err.Error())
		_, _ = db.AppendRunOutput(dbCtx, app.DB, run.ID, "SSH connect failed: "+err.Error()+"\n")
		app.dispatchWebhooks("update_failure", map[string]interface{}{"host_id": hostID, "error": err.Error()})
		return
	}
	defer sshClient.Close()

	for _, cmd := range commands {
		exitCode, runErr := app.streamCommand(r.Context(), conn, sshClient, run.ID, cmd)
		if runErr != nil {
			finishErr = runErr.Error()
			finishExit = exitCode
			emit(conn, fmt.Sprintf("\nCommand failed (exit %d): %s\n", exitCode, runErr.Error()))
			app.dispatchWebhooks("update_failure", map[string]interface{}{
				"host_id": hostID, "run_id": run.ID, "command": cmd, "error": runErr.Error(),
			})
			return
		}
	}

	finishStatus = models.RunStatusSucceeded
	finishExit = 0
	event := "preview_success"
	if kind == models.RunKindUpdate {
		event = "update_success"
		// Clear the host's stored error on a successful update so the badge resets.
		_, _ = db.UpsertHost(dbCtx, app.DB, host.Hostname, host.SshUser, host.UpdateOutput, host.UpgradeOutput, "")
	}
	app.dispatchWebhooks(event, map[string]interface{}{"host_id": hostID, "run_id": run.ID})
}

// streamCommand runs one shell line on the existing SSH client, fans
// stdout/stderr to (a) the websocket and (b) the run row's output column,
// and returns the remote exit code (-1 if the SSH layer itself failed).
func (app *Application) streamCommand(ctx context.Context, conn *websocket.Conn, client *ssh.Client, runID int32, cmd string) (int, error) {
	session, err := client.NewSession()
	if err != nil {
		return -1, fmt.Errorf("create ssh session: %w", err)
	}
	defer session.Close()

	stdout, err := session.StdoutPipe()
	if err != nil {
		return -1, fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := session.StderrPipe()
	if err != nil {
		return -1, fmt.Errorf("stderr pipe: %w", err)
	}

	if err := session.Start(cmd); err != nil {
		return -1, fmt.Errorf("start ssh command: %w", err)
	}

	emit(conn, "$ "+cmd+"\n")

	// Decoupled write ctx so the DB rows still get a final flush even if r.Context()
	// is cancelled by a client disconnect mid-stream.
	dbCtx, cancelDB := context.WithCancel(context.Background())
	defer cancelDB()

	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); pumpReader(ctx, dbCtx, conn, app.DB, runID, stdout) }()
	go func() { defer wg.Done(); pumpReader(ctx, dbCtx, conn, app.DB, runID, stderr) }()

	wg.Wait()
	err = session.Wait()
	if err == nil {
		return 0, nil
	}
	var exitErr *ssh.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitStatus(), fmt.Errorf("exit status %d", exitErr.ExitStatus())
	}
	return -1, err
}

// pumpReader copies a reader to the websocket and the DB row in 4 KiB chunks.
// Backpressure: the websocket write is the slow path; if a client is gone the
// chunk is silently dropped and we keep persisting to DB so history remains
// accurate.
func pumpReader(ctx context.Context, dbCtx context.Context, conn *websocket.Conn, pool *pgxpool.Pool, runID int32, src io.Reader) {
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		n, err := src.Read(buf)
		if n > 0 {
			chunk := string(buf[:n])
			// Best-effort write to the websocket — connection might be closed.
			_ = conn.WriteMessage(websocket.TextMessage, []byte(chunk))
			// Persistent record. AppendRunOutput is a no-op past the cap.
			_, _ = db.AppendRunOutput(dbCtx, pool, runID, chunk)
		}
		if err != nil {
			return
		}
	}
}

func emit(conn *websocket.Conn, line string) {
	_ = conn.WriteMessage(websocket.TextMessage, []byte(line))
}

// handleListRuns returns the most recent runs for a host, newest-first.
// Output column is included verbatim — it's already capped at 1 MiB.
func (app *Application) handleListRuns(w http.ResponseWriter, r *http.Request) {
	id, err := parseHostID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid host ID")
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if parsed, err := strconv.Atoi(v); err == nil {
			limit = parsed
		}
	}

	runs, err := db.ListRunsForHost(r.Context(), app.DB, id, limit)
	if err != nil {
		log.Errorf("Failed to list runs for host %d: %v", id, err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve runs")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

// handleGetRun returns a single run by id, including its full output buffer.
func (app *Application) handleGetRun(w http.ResponseWriter, r *http.Request) {
	idStr, ok := mux.Vars(r)["id"]
	if !ok {
		writeJSONError(w, http.StatusBadRequest, "run id missing")
		return
	}
	id, err := strconv.Atoi(idStr)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid run ID")
		return
	}

	run, err := db.GetRun(r.Context(), app.DB, int32(id))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Run not found")
			return
		}
		log.Errorf("Failed to get run %d: %v", id, err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve run")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(run)
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
