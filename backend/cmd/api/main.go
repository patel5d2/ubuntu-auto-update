package main

import (
	"context"
	"encoding/json"
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
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"ubuntu-auto-update/backend/pkg/config"
	"ubuntu-auto-update/backend/pkg/db"
	"ubuntu-auto-update/backend/pkg/middleware"
	"ubuntu-auto-update/backend/pkg/models"
	"ubuntu-auto-update/backend/pkg/webhook"
)

// maxRequestBodySize limits POST request bodies to 1MB
const maxRequestBodySize = 1 << 20

type Application struct {
	DB         *pgxpool.Pool
	TokenStore *middleware.TokenStore
	AuthConfig *middleware.AuthConfig
}

func (app *Application) sendWebhook(event string, payload interface{}) {
	webhooks, err := db.GetWebhooks(context.Background(), app.DB, event)
	if err != nil {
		log.Errorf("Failed to get webhooks: %v", err)
		return
	}

	for _, wh := range webhooks {
		if err := webhook.Send(wh.URL, payload); err != nil {
			log.Errorf("Failed to deliver webhook to %s: %v", wh.URL, err)
		}
	}
}

func (app *Application) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get allowed origins from environment, default to localhost dev servers
		allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
		if allowedOrigins == "" {
			allowedOrigins = "http://localhost:5173,http://localhost:3000"
		}

		origin := r.Header.Get("Origin")
		if origin != "" {
			for _, allowed := range strings.Split(allowedOrigins, ",") {
				if strings.TrimSpace(allowed) == origin || strings.TrimSpace(allowed) == "*" {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					break
				}
			}
		}

		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Credentials", "true")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (app *Application) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var tokenString string

		// Check for auth cookie for web UI
		if cookie, err := r.Cookie("auth_token"); err == nil {
			tokenString = cookie.Value
		}

		// Check for Authorization header for agent
		if tokenString == "" {
			authHeader := r.Header.Get("Authorization")
			if strings.HasPrefix(authHeader, "Bearer ") {
				tokenString = strings.TrimPrefix(authHeader, "Bearer ")
			}
		}

		if tokenString == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Validate token against the token store
		username, valid := app.TokenStore.ValidateToken(tokenString)
		if !valid {
			http.Error(w, "Unauthorized: invalid or expired token", http.StatusUnauthorized)
			return
		}

		log.Debugf("Authenticated request from user: %s", username)
		next.ServeHTTP(w, r)
	})
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

	// Start background token cleanup every 5 minutes
	middleware.StartTokenCleanup(tokenStore, 5*time.Minute)

	app := &Application{
		DB:         dbPool,
		TokenStore: tokenStore,
		AuthConfig: authConfig,
	}

	r := mux.NewRouter()
	r.Use(app.corsMiddleware)
	r.HandleFunc("/api/v1/health", app.handleHealth).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/enroll", app.handleEnroll).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/login", app.handleLogin).Methods(http.MethodPost, http.MethodOptions)

	api := r.PathPrefix("/api/v1").Subrouter()
	api.Use(app.authMiddleware)
	api.HandleFunc("/report", app.handleReport).Methods(http.MethodPost)
	api.HandleFunc("/hosts", app.handleListHosts).Methods(http.MethodGet)
	api.HandleFunc("/hosts/{id}", app.handleGetHost).Methods(http.MethodGet)
	api.HandleFunc("/hosts/{id}/run-update", app.handleRunUpdate)
	api.HandleFunc("/hosts/{id}/execute-script", app.handleExecuteScript)
	api.HandleFunc("/hosts/{id}/ssh-key", app.handleAddSSHKey).Methods(http.MethodPost)
	api.HandleFunc("/webhooks", app.handleAddWebhook).Methods(http.MethodPost)

	port := os.Getenv("API_PORT")
	if port == "" {
		port = "8080"
	}

	// Use http.Server with proper timeouts for production readiness
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
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

	// Validate hostname
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
	if req.EnrollmentToken != enrollmentToken {
		http.Error(w, "Invalid enrollment token", http.StatusUnauthorized)
		return
	}

	// Generate a new random authentication token
	authToken, err := middleware.GenerateSecureToken()
	if err != nil {
		log.Errorf("Failed to generate token: %v", err)
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	// Store the token for agent authentication (long-lived: 365 days)
	app.TokenStore.StoreToken(authToken, "agent:"+req.Hostname, 365*24*time.Hour)

	log.Infof("Agent enrolled successfully: %s", req.Hostname)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": authToken})
}

func (app *Application) handleLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	adminUsername := os.Getenv("ADMIN_USERNAME")
	adminPassword := os.Getenv("ADMIN_PASSWORD")

	// SECURITY: Do NOT log credentials
	if adminUsername == "" || adminPassword == "" {
		log.Error("ADMIN_USERNAME or ADMIN_PASSWORD environment variables not set")
		http.Error(w, "Authentication not configured", http.StatusInternalServerError)
		return
	}

	if req.Username == adminUsername && req.Password == adminPassword {
		authToken, err := middleware.GenerateSecureToken()
		if err != nil {
			log.Errorf("Failed to generate token: %v", err)
			http.Error(w, "Failed to generate token", http.StatusInternalServerError)
			return
		}

		// Store token with 24-hour expiry
		app.TokenStore.StoreToken(authToken, req.Username, 24*time.Hour)

		// Set secure cookie
		middleware.SetAuthCookie(w, app.AuthConfig, authToken)

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"token": authToken})
	} else {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
	}
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

func (app *Application) handleGetHost(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr, ok := vars["id"]
	if !ok {
		http.Error(w, "Host ID not found in URL", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid host ID", http.StatusBadRequest)
		return
	}

	host, err := db.GetHost(r.Context(), app.DB, int32(id))
	if err != nil {
		if err == pgx.ErrNoRows {
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

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		// Validate WebSocket origin against allowed origins
		origin := r.Header.Get("Origin")
		if origin == "" {
			return false
		}

		allowedOrigins := os.Getenv("CORS_ALLOWED_ORIGINS")
		if allowedOrigins == "" {
			allowedOrigins = "http://localhost:5173,http://localhost:3000"
		}

		for _, allowed := range strings.Split(allowedOrigins, ",") {
			if strings.TrimSpace(allowed) == origin || strings.TrimSpace(allowed) == "*" {
				return true
			}
		}
		return false
	},
}

func (app *Application) handleAddWebhook(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req models.Webhook
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate webhook URL and event
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

	vars := mux.Vars(r)
	idStr, ok := vars["id"]
	if !ok {
		http.Error(w, "Host ID not found in URL", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
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

	if err := db.AddSSHKey(r.Context(), app.DB, int32(id), req.PrivateKey); err != nil {
		log.Errorf("Failed to add SSH key: %v", err)
		http.Error(w, "Failed to add SSH key", http.StatusInternalServerError)
		return
	}

	// also update the ssh_user in the hosts table
	if _, err := app.DB.Exec(r.Context(), `UPDATE hosts SET ssh_user = $1 WHERE id = $2`, req.SshUser, id); err != nil {
		log.Errorf("Failed to update ssh_user: %v", err)
		http.Error(w, "Failed to update ssh_user", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (app *Application) handleExecuteScript(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr, ok := vars["id"]
	if !ok {
		http.Error(w, "Host ID not found in URL", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid host ID", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade to websocket: %v", err)
		return
	}
	defer conn.Close()

	// Read the script from the WebSocket connection
	_, script, err := conn.ReadMessage()
	if err != nil {
		log.Errorf("Failed to read script from websocket: %v", err)
		return
	}

	key, err := db.GetSSHKey(r.Context(), app.DB, int32(id))
	if err != nil {
		log.Errorf("Failed to get SSH key: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to get SSH key"))
		return
	}

	host, err := db.GetHost(r.Context(), app.DB, int32(id))
	if err != nil {
		log.Errorf("Failed to get host: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to get host"))
		return
	}

	signer, err := ssh.ParsePrivateKey([]byte(key.PrivateKey))
	if err != nil {
		log.Errorf("Failed to parse private key: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to parse private key"))
		return
	}

	hostKeyCallback, err := knownhosts.New("known_hosts")
	if err != nil {
		log.Errorf("Failed to create host key callback: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to create host key callback"))
		return
	}

	// Establish SSH connection
	sshConfig := &ssh.ClientConfig{
		User: host.SshUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}

	sshClient, err := ssh.Dial("tcp", host.Hostname+":22", sshConfig)
	if err != nil {
		log.Errorf("Failed to dial SSH: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to dial SSH: "+err.Error()))
		return
	}
	defer sshClient.Close()

	// Run the script
	session, err := sshClient.NewSession()
	if err != nil {
		log.Errorf("Failed to create SSH session: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to create SSH session: "+err.Error()))
		return
	}
	defer session.Close()

	output, err := session.CombinedOutput(string(script))
	if err != nil {
		log.Errorf("Failed to run script: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Failed to run script: %s", err.Error())))
		conn.WriteMessage(websocket.TextMessage, output)
		return
	}

	conn.WriteMessage(websocket.TextMessage, output)
}

func (app *Application) handleRunUpdate(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	idStr, ok := vars["id"]
	if !ok {
		http.Error(w, "Host ID not found in URL", http.StatusBadRequest)
		return
	}

	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid host ID", http.StatusBadRequest)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("Failed to upgrade to websocket: %v", err)
		return
	}
	defer conn.Close()

	host, err := db.GetHost(r.Context(), app.DB, int32(id))
	if err != nil {
		log.Errorf("Failed to get host: %v", err)
		app.sendWebhook("update_failure", map[string]interface{}{"host_id": id, "error": err.Error()})
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to get host"))
		return
	}

	key, err := db.GetSSHKey(r.Context(), app.DB, int32(id))
	if err != nil {
		log.Errorf("Failed to get SSH key: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to get SSH key"))
		return
	}

	signer, err := ssh.ParsePrivateKey([]byte(key.PrivateKey))
	if err != nil {
		log.Errorf("Failed to parse private key: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to parse private key"))
		return
	}

	hostKeyCallback, err := knownhosts.New("known_hosts")
	if err != nil {
		log.Errorf("Failed to create host key callback: %v", err)
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to create host key callback"))
		return
	}

	// Establish SSH connection
	sshConfig := &ssh.ClientConfig{
		User: host.SshUser,
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}

	sshClient, err := ssh.Dial("tcp", host.Hostname+":22", sshConfig)
	if err != nil {
		log.Errorf("Failed to dial SSH: %v", err)
		db.UpsertHost(r.Context(), app.DB, host.Hostname, host.SshUser, "", "", fmt.Sprintf("Failed to dial SSH: %v", err))
		app.sendWebhook("update_failure", map[string]interface{}{"host_id": id, "error": err.Error()})
		conn.WriteMessage(websocket.TextMessage, []byte("Failed to dial SSH: "+err.Error()))
		return
	}
	defer sshClient.Close()

	// Run commands (modified for demo - these work without sudo)
	commands := []string{
		"echo 'Starting Ubuntu update check...'",
		"apt list --upgradable",
		"echo 'Update check completed. Note: Actual updates require sudo privileges.'",
	}

	for _, cmd := range commands {
		session, err := sshClient.NewSession()
		if err != nil {
			log.Errorf("Failed to create SSH session: %v", err)
			db.UpsertHost(r.Context(), app.DB, host.Hostname, host.SshUser, "", "", fmt.Sprintf("Failed to create SSH session: %v", err))
			app.sendWebhook("update_failure", map[string]interface{}{"host_id": id, "error": err.Error()})
			conn.WriteMessage(websocket.TextMessage, []byte("Failed to create SSH session: "+err.Error()))
			return
		}
		defer session.Close()

		output, err := session.CombinedOutput(cmd)
		if err != nil {
			log.Errorf("Failed to run command '%s': %v", cmd, err)
			db.UpsertHost(r.Context(), app.DB, host.Hostname, host.SshUser, "", string(output), fmt.Sprintf("Failed to run command '%s': %v", cmd, err))
			app.sendWebhook("update_failure", map[string]interface{}{"host_id": id, "command": cmd, "error": err.Error(), "output": string(output)})
			conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Failed to run command '%s': %s", cmd, err.Error())))
			conn.WriteMessage(websocket.TextMessage, output)
			return
		}

		conn.WriteMessage(websocket.TextMessage, output)
		db.UpsertHost(r.Context(), app.DB, host.Hostname, host.SshUser, string(output), "", "")
	}

	app.sendWebhook("update_success", map[string]interface{}{"host_id": id})
}

func (app *Application) handleHealth(w http.ResponseWriter, r *http.Request) {
	// Check database connection
	err := app.DB.Ping(r.Context())
	if err != nil {
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
		"version":   "1.0.0",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
}
