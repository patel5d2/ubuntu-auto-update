package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/knownhosts"
	"ubuntu-auto-update/backend/pkg/config"
	"ubuntu-auto-update/backend/pkg/db"
	"ubuntu-auto-update/backend/pkg/models"
	"ubuntu-auto-update/backend/pkg/webhook"
)

type Application struct {
	DB *pgxpool.Pool
}

func (app *Application) sendWebhook(event string, payload interface{}) {
	webhooks, err := db.GetWebhooks(context.Background(), app.DB, event)
	if err != nil {
		log.Errorf("Failed to get webhooks: %v", err)
		return
	}

	for _, wh := range webhooks {
		webhook.Send(wh.URL, payload)
	}
}

func (app *Application) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "http://localhost:5173")
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
		// Check for auth cookie for web UI
		if _, err := r.Cookie("auth_token"); err == nil {
			next.ServeHTTP(w, r)
			return
		}

		// Check for Authorization header for agent
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// For now, just check if the token is not empty
		// TODO: Implement proper token validation
		token := strings.TrimPrefix(authHeader, "Bearer ")
		if token == "" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func main() {
	if err := config.Load(); err != nil {
		log.Fatalf("Failed to load configuration: %v", err)
	}

	log.Info("Starting application...")
	ctx := context.Background()

	dbPool, err := db.NewConnection(ctx)
	if err != nil {
		log.Fatalf("Could not connect to database: %v", err)
	}
	defer dbPool.Close()

	app := &Application{
		DB: dbPool,
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

	log.Info("Starting server on :" + port)
	if err := http.ListenAndServe(":"+port, r); err != nil {
		log.Fatal(err)
	}
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (app *Application) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		EnrollmentToken string `json:"enrollment_token"`
		Hostname        string `json:"hostname"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
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
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		log.Errorf("Failed to generate token: %v", err)
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}
	authToken := hex.EncodeToString(tokenBytes)

	// Store the token in the database
	// TODO: Implement token storage

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"token": authToken})
}

func (app *Application) handleLogin(w http.ResponseWriter, r *http.Request) {

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	adminUsername := os.Getenv("ADMIN_USERNAME")
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	log.Infof("Admin username: %s, Admin password: %s", adminUsername, adminPassword)

	if req.Username == adminUsername && req.Password == adminPassword {
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			log.Errorf("Failed to generate token: %v", err)
			http.Error(w, "Failed to generate token", http.StatusInternalServerError)
			return
		}
		authToken := hex.EncodeToString(tokenBytes)

		cookie := http.Cookie{
			Name:     "auth_token",
			Value:    authToken,
			Path:     "/",
			HttpOnly: false, // Allow JavaScript access for development
			Secure:   false, // Allow HTTP for development
			SameSite: http.SameSiteLaxMode,
		}
		http.SetCookie(w, &cookie)
		w.WriteHeader(http.StatusOK)
	} else {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
	}
}

func (app *Application) handleReport(w http.ResponseWriter, r *http.Request) {

	var report models.HostReport
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

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
	CheckOrigin: func(r *http.Request) bool { return true },
}

func (app *Application) handleAddWebhook(w http.ResponseWriter, r *http.Request) {
	var req models.Webhook
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if _, err := app.DB.Exec(r.Context(), `INSERT INTO webhooks (url, event) VALUES ($1, $2)`, req.URL, req.Event); err != nil {
		log.Errorf("Failed to add webhook: %v", err)
		http.Error(w, "Failed to add webhook", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
}

func (app *Application) handleScanHostKey(w http.ResponseWriter, r *http.Request) {
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
		log.Errorf("Failed to get host: %v", err)
		http.Error(w, "Host not found", http.StatusNotFound)
		return
	}

	// Scan host key
	cmd := exec.Command("ssh-keyscan", "-t", "rsa", host.Hostname)
	output, err := cmd.Output()
	if err != nil {
		log.Errorf("Failed to scan host key: %v", err)
		http.Error(w, "Failed to scan host key", http.StatusInternalServerError)
		return
	}

	// Add host key to known_hosts file
	f, err := os.OpenFile("known_hosts", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Errorf("Failed to open known_hosts file: %v", err)
		http.Error(w, "Failed to open known_hosts file", http.StatusInternalServerError)
		return
	}
	defer f.Close()

	if _, err := f.Write(output); err != nil {
		log.Errorf("Failed to write to known_hosts file: %v", err)
		http.Error(w, "Failed to write to known_hosts file", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
}

func (app *Application) handleAddSSHKey(w http.ResponseWriter, r *http.Request) {
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
	config := &ssh.ClientConfig{
		User: host.SshUser, // or get the user from the request
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
	}

	sshClient, err := ssh.Dial("tcp", host.Hostname+":22", config)
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
	config := &ssh.ClientConfig{
		User: host.SshUser, // or get the user from the request
		Auth: []ssh.AuthMethod{
			ssh.PublicKeys(signer),
		},
		HostKeyCallback: hostKeyCallback,
	}

	sshClient, err := ssh.Dial("tcp", host.Hostname+":22", config)
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
			"status": "unhealthy",
			"database": "disconnected",
			"timestamp": "now",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"database": "connected",
		"version": "1.0.0",
		"timestamp": "now",
	})
}
