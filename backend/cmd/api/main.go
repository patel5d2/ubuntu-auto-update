package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	pathpkg "path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5"
	log "github.com/sirupsen/logrus"
	"golang.org/x/crypto/ssh"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"ubuntu-auto-update/backend/pkg/audit"
	"ubuntu-auto-update/backend/pkg/config"
	"ubuntu-auto-update/backend/pkg/db"
	"ubuntu-auto-update/backend/pkg/events"
	"ubuntu-auto-update/backend/pkg/middleware"
	"ubuntu-auto-update/backend/pkg/models"
	"ubuntu-auto-update/backend/pkg/session"
	sshpkg "ubuntu-auto-update/backend/pkg/ssh"
	"ubuntu-auto-update/backend/pkg/updater"
	"ubuntu-auto-update/backend/pkg/users"
	"ubuntu-auto-update/backend/pkg/webhook"
)

// maxRequestBodySize limits POST request bodies to 1MB.
const maxRequestBodySize = 1 << 20

type Application struct {
	DB             db.DBTX
	TokenStore     *middleware.TokenStore // legacy in-memory store (tests + dev)
	Sessions       session.Store          // production session store (DB-backed when DB available)
	AuthConfig     *middleware.AuthConfig
	CORS           *middleware.CORSConfig
	IPAllowlist    *middleware.IPAllowlist
	LoginLimiter   *middleware.LoginRateLimiter
	SSHDialer      *sshpkg.Dialer
	WebhookSender  *webhook.Dispatcher
	BulkUpdater    *updater.Coordinator
	EventBroker    *events.Broker
}

// dispatchWebhooks resolves subscribers for an event and queues deliveries.
// Returns immediately; deliveries run on the dispatcher's goroutines.
//
// Bound the lookup with a short timeout so a stalled DB doesn't pin the
// caller (especially when invoked from the streaming run path where the
// websocket goroutine already has timing constraints).
func (app *Application) dispatchWebhooks(event string, payload interface{}) {
	lookupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	hooks, err := db.GetWebhooks(lookupCtx, app.DB, event)
	if err != nil {
		log.Errorf("Failed to get webhooks for event %s: %v", event, err)
		return
	}
	for _, h := range hooks {
		// Per-delivery timeout lives inside the dispatcher's HTTP client; we
		// pass Background here so a single slow delivery doesn't tip-over
		// every other in-flight one.
		app.WebhookSender.Deliver(context.Background(), h.URL, payload)
	}
}

// spaHandler implements http.Handler to serve static files with an SPA fallback.
type spaHandler struct {
	staticPath string
	indexPath  string
}

func (h spaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Canonicalise the URL path and reject anything that escapes the root.
	// path.Clean handles "..", "//", and trailing slashes; we also defensively
	// confirm the joined absolute path stays inside staticPath so a future
	// refactor can't reintroduce traversal.
	cleaned := pathpkg.Clean("/" + r.URL.Path)
	if cleaned == "/" {
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	}

	requested := filepath.Join(h.staticPath, filepath.FromSlash(cleaned))
	absStatic, err := filepath.Abs(h.staticPath)
	if err != nil {
		http.Error(w, "Server misconfigured", http.StatusInternalServerError)
		return
	}
	absRequested, err := filepath.Abs(requested)
	if err != nil {
		http.Error(w, "Bad path", http.StatusBadRequest)
		return
	}
	if !strings.HasPrefix(absRequested+string(filepath.Separator), absStatic+string(filepath.Separator)) && absRequested != absStatic {
		http.Error(w, "Not found", http.StatusNotFound)
		return
	}

	if _, statErr := os.Stat(absRequested); os.IsNotExist(statErr) {
		// SPA fallback for client-side routes.
		http.ServeFile(w, r, filepath.Join(h.staticPath, h.indexPath))
		return
	} else if statErr != nil {
		http.Error(w, "Server error", http.StatusInternalServerError)
		return
	}

	http.FileServer(http.Dir(h.staticPath)).ServeHTTP(w, r)
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

	// DB-backed session store. The legacy in-memory store remains alive
	// only so that tests in this package can keep using it directly.
	sessionStore := session.NewDBStore(dbPool)
	cleanupCtx, cancelSessionCleanup := context.WithCancel(context.Background())
	defer cancelSessionCleanup()
	session.StartCleanup(cleanupCtx, sessionStore, 5*time.Minute)

	corsCfg := middleware.LoadCORSConfig()
	allowlist, err := middleware.NewIPAllowlist(os.Getenv("OPERATOR_IP_ALLOWLIST"))
	if err != nil {
		log.Fatalf("OPERATOR_IP_ALLOWLIST: %v", err)
	}
	loginLimiter := middleware.NewLoginRateLimiter()
	// Periodically drop idle buckets so a long-lived process doesn't accumulate
	// one map entry per distinct source IP that ever hit /login. Idle window
	// is generous — the bucket only matters during an active brute-force burst.
	middleware.StartLoginLimiterCleanup(cleanupCtx, loginLimiter, 10*time.Minute, time.Hour)

	dispatcher := webhook.NewDispatcher()
	sshDialer := sshpkg.NewDialer(dbPool)
	broker := events.NewBroker()
	app := &Application{
		DB:            dbPool,
		TokenStore:    tokenStore,
		Sessions:      sessionStore,
		AuthConfig:    authConfig,
		CORS:          corsCfg,
		IPAllowlist:   allowlist,
		LoginLimiter:  loginLimiter,
		SSHDialer:     sshDialer,
		WebhookSender: dispatcher,
		BulkUpdater:   updater.New(dbPool, sshDialer),
		EventBroker:   broker,
	}

	// Bootstrap an initial admin from ADMIN_USERNAME / ADMIN_PASSWORD env
	// vars. Only takes effect when the users table is empty, so re-deploys
	// don't quietly clobber a manually-created account.
	if created, err := users.EnsureBootstrapAdmin(ctx, dbPool,
		os.Getenv("ADMIN_USERNAME"), os.Getenv("ADMIN_PASSWORD")); err != nil {
		if errors.Is(err, users.ErrPasswordTooShort) {
			log.Fatalf("ADMIN_PASSWORD must be at least 12 characters; cannot bootstrap admin user")
		}
		log.Errorf("bootstrap admin: %v", err)
	} else if created {
		log.Infof("Bootstrapped initial admin user from ADMIN_USERNAME / ADMIN_PASSWORD")
	}

	// Start the LISTEN/NOTIFY pump. Lifetime is tied to listenerCtx so the
	// shutdown path below can stop it cleanly. The goroutine self-recovers
	// from connection drops via internal backoff — no supervisor needed.
	listenerCtx, cancelListener := context.WithCancel(context.Background())
	defer cancelListener()
	go events.NewListener(dbPool, broker).Run(listenerCtx)

	r := mux.NewRouter()
	r.Use(middleware.PrometheusMiddleware) // request metrics (must be first)
	r.Use(middleware.SecurityHeaders)      // defense-in-depth HTTP headers
	r.Use(middleware.ErrorHandler)         // panic recovery + request logging
	r.Use(middleware.CORS(corsCfg))
	if allowlist != nil {
		r.Use(middleware.IPAllowlistMiddleware(allowlist))
	}

	// Enrollment: rate-limit to prevent token brute-force. Shared limiter
	// with login is fine — same 5 req/min per IP budget.
	enrollLimiter := middleware.NewLoginRateLimiter()
	middleware.StartLoginLimiterCleanup(cleanupCtx, enrollLimiter, 10*time.Minute, time.Hour)

	// Prometheus metrics endpoint.
	r.Handle("/metrics", promhttp.Handler()).Methods(http.MethodGet)
	r.HandleFunc("/api/v1/health", app.handleHealth).Methods(http.MethodGet)
	r.Handle("/api/v1/enroll", middleware.RateLimitHandler(enrollLimiter)(http.HandlerFunc(app.handleEnroll))).Methods(http.MethodPost)
	r.HandleFunc("/api/v1/login", app.handleLogin).Methods(http.MethodPost, http.MethodOptions)
	r.HandleFunc("/api/v1/logout", app.handleLogout).Methods(http.MethodPost, http.MethodOptions)

	// Authenticated routes (any role).
	api := r.PathPrefix("/api/v1").Subrouter()
	api.Use(middleware.SessionAuthMiddleware(sessionStore, authConfig))

	// /report is agent-only — we explicitly require RoleAgent rather than
	// relying on a handler-level check. Without this any logged-in viewer
	// could push report payloads.
	reportRouter := api.PathPrefix("").Subrouter()
	reportRouter.Use(middleware.RequireRole(session.RoleAgent))
	reportRouter.HandleFunc("/report", app.handleReport).Methods(http.MethodPost)

	// Read-only — viewer+ can see.
	viewer := api.PathPrefix("").Subrouter()
	viewer.Use(middleware.RequireRole(session.RoleViewer))
	viewer.HandleFunc("/hosts", app.handleListHosts).Methods(http.MethodGet)
	viewer.HandleFunc("/hosts/{id}", app.handleGetHost).Methods(http.MethodGet)
	viewer.HandleFunc("/hosts/{id}/runs", app.handleListRuns).Methods(http.MethodGet)
	viewer.HandleFunc("/runs", app.handleListRunsByGroup).Methods(http.MethodGet)
	viewer.HandleFunc("/runs/{id}", app.handleGetRun).Methods(http.MethodGet)
	viewer.HandleFunc("/events", events.Handler(broker, app.wsUpgrader(), app.Sessions)).Methods(http.MethodGet)
	viewer.HandleFunc("/me", app.handleMe).Methods(http.MethodGet)

	// State-changing operations — operator+.
	op := api.PathPrefix("").Subrouter()
	op.Use(middleware.RequireRole(session.RoleOperator))
	// CSRF defense for cookie-auth POSTs/PATCHes/DELETEs. Bearer-auth bypasses.
	// Disable with CSRF_DISABLED=true if you need to (e.g. CLI-only deployment).
	csrfEnabled := os.Getenv("CSRF_DISABLED") != "true"
	if csrfEnabled {
		op.Use(middleware.CSRFMiddleware(authConfig.CookieName))
	}
	op.HandleFunc("/hosts", app.handleCreateHost).Methods(http.MethodPost)
	op.HandleFunc("/hosts/{id}", app.handleUpdateHost).Methods(http.MethodPatch)
	op.HandleFunc("/hosts/{id}", app.handleDeleteHost).Methods(http.MethodDelete)
	op.HandleFunc("/hosts/{id}/preview-updates", app.handlePreviewUpdates).Methods(http.MethodGet)
	op.HandleFunc("/hosts/{id}/run-update", app.handleRunUpdate).Methods(http.MethodGet)
	op.HandleFunc("/hosts/{id}/execute-script", app.handleExecuteScript).Methods(http.MethodGet)
	op.HandleFunc("/hosts/{id}/ssh-key", app.handleAddSSHKey).Methods(http.MethodPost)
	op.HandleFunc("/hosts/{id}/test-connection", app.handleTestConnection).Methods(http.MethodPost)
	op.HandleFunc("/hosts/{id}/auto-configure", app.handleAutoConfigure).Methods(http.MethodPost)
	op.HandleFunc("/hosts/{id}/rotate-key", app.handleRotateKey).Methods(http.MethodPost)
	op.HandleFunc("/hosts/bulk/enroll", app.handleBulkEnroll).Methods(http.MethodPost)
	op.HandleFunc("/hosts/bulk/run-update", app.handleBulkRunUpdate).Methods(http.MethodPost)
	op.HandleFunc("/webhooks", app.handleAddWebhook).Methods(http.MethodPost)

	// Admin-only — user/audit management. CSRF mirrors the operator subrouter
	// since these endpoints are equally state-changing (and equally cookie-
	// auth-driven from a browser).
	admin := api.PathPrefix("").Subrouter()
	admin.Use(middleware.RequireRole(session.RoleAdmin))
	if csrfEnabled {
		admin.Use(middleware.CSRFMiddleware(authConfig.CookieName))
	}
	admin.HandleFunc("/users", app.handleListUsers).Methods(http.MethodGet)
	admin.HandleFunc("/users", app.handleCreateUser).Methods(http.MethodPost)
	admin.HandleFunc("/users/{id}", app.handleUpdateUser).Methods(http.MethodPatch)
	admin.HandleFunc("/users/{id}", app.handleDeleteUser).Methods(http.MethodDelete)
	admin.HandleFunc("/audit", app.handleListAudit).Methods(http.MethodGet)

	// Fallback to serving the frontend React application
	spa := spaHandler{staticPath: "public", indexPath: "index.html"}
	r.PathPrefix("/").Handler(spa)

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
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.Hostname = strings.TrimSpace(req.Hostname)
	if req.Hostname == "" {
		writeJSONError(w, http.StatusBadRequest, "Hostname cannot be empty")
		return
	}

	enrollmentToken := os.Getenv("ENROLLMENT_TOKEN")
	if enrollmentToken == "" {
		log.Error("ENROLLMENT_TOKEN environment variable not set")
		writeJSONError(w, http.StatusInternalServerError, "Enrollment not configured")
		return
	}
	if subtle.ConstantTimeCompare([]byte(req.EnrollmentToken), []byte(enrollmentToken)) != 1 {
		writeJSONError(w, http.StatusUnauthorized, "Invalid enrollment token")
		return
	}

	// Use the session store when available (production path); fall back to
	// the legacy in-memory store when DB isn't wired (tests).
	var authToken string
	if app.Sessions != nil {
		// Agent sessions: 90 days (was 365). Shorter lifetime limits blast
		// radius if an agent token is compromised. Agents re-enroll on expiry.
		t, err := app.Sessions.Create(r.Context(),
			session.Principal{AgentLabel: req.Hostname, Username: "agent:" + req.Hostname, Role: session.RoleAgent},
			90*24*time.Hour, middleware.ClientIP(r), r.UserAgent())
		if err != nil {
			log.Errorf("Failed to create agent session: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "Failed to generate token")
			return
		}
		authToken = t
	} else {
		t, err := middleware.GenerateSecureToken()
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, "Failed to generate token")
			return
		}
		authToken = t
		app.TokenStore.StoreTokenWithRole(authToken, "agent:"+req.Hostname, session.RoleAgent, 90*24*time.Hour)
	}

	log.Infof("Agent enrolled successfully: %s", req.Hostname)
	app.audit(r, audit.ActionAgentEnroll, "agent", req.Hostname,
		map[string]interface{}{"hostname": req.Hostname})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"token": authToken})
}

func (app *Application) handleLogin(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	// Per-IP rate limit. Cheap and effective at slowing brute-force attempts
	// without affecting legitimate users.
	if app.LoginLimiter != nil {
		if !app.LoginLimiter.Allow(middleware.ClientIP(r)) {
			w.Header().Set("Retry-After", "60")
			writeJSONError(w, http.StatusTooManyRequests, "Too many login attempts; try again shortly")
			return
		}
	}

	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Path A: DB-backed users (production). Falls through to env-based admin
	// only when the DB path isn't available (tests with no DB).
	if app.DB != nil {
		u, err := users.Authenticate(r.Context(), app.DB, req.Username, req.Password)
		if err != nil {
			// Log a single audit entry and return a generic 401.
			app.audit(r, audit.ActionLoginFailure, "user", req.Username,
				map[string]interface{}{"reason": err.Error()})
			writeJSONError(w, http.StatusUnauthorized, "Invalid credentials")
			return
		}

		tok, err := app.Sessions.Create(r.Context(),
			session.Principal{UserID: u.ID, Username: u.Username, Role: u.Role},
			24*time.Hour, middleware.ClientIP(r), r.UserAgent())
		if err != nil {
			log.Errorf("create session: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "Failed to create session")
			return
		}
		middleware.SetAuthCookie(w, app.AuthConfig, tok)
		csrf, _ := middleware.GenerateCSRFToken()
		middleware.SetCSRFCookie(w, csrf)
		app.audit(r, audit.ActionLoginSuccess, "user", strconv.FormatInt(int64(u.ID), 10),
			map[string]interface{}{"username": u.Username})

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"token": tok, "role": u.Role, "csrf_token": csrf})
		return
	}

	// Path B: legacy env-based admin login (preserved for the existing test
	// suite; production should never reach this branch).
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
	app.TokenStore.StoreTokenWithRole(authToken, req.Username, session.RoleAdmin, 24*time.Hour)
	middleware.SetAuthCookie(w, app.AuthConfig, authToken)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"token": authToken})
}

// handleMe returns the current principal so the UI can branch on role
// without a separate config endpoint.
func (app *Application) handleMe(w http.ResponseWriter, r *http.Request) {
	p := middleware.GetPrincipalFromContext(r)
	if p == nil {
		writeJSONError(w, http.StatusUnauthorized, "No principal")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"username": p.Username,
		"role":     p.Role,
		"is_agent": p.IsAgent(),
	})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// handleLogout invalidates the caller's token (if present) and clears the auth cookie.
func (app *Application) handleLogout(w http.ResponseWriter, r *http.Request) {
	tok := ""
	if cookie, err := r.Cookie(app.AuthConfig.CookieName); err == nil && cookie.Value != "" {
		tok = cookie.Value
	} else if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		tok = strings.TrimPrefix(h, "Bearer ")
	}
	if tok != "" {
		if app.Sessions != nil {
			_ = app.Sessions.Revoke(r.Context(), tok)
		}
		// Always remove from the legacy in-memory store too — harmless if
		// missing.
		app.TokenStore.RemoveToken(tok)
	}
	middleware.ClearAuthCookie(w, app.AuthConfig)
	middleware.ClearCSRFCookie(w)
	app.audit(r, audit.ActionLogout, "session", "", nil)
	w.WriteHeader(http.StatusOK)
}

func (app *Application) handleReport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var report models.HostReport
	if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	report.Hostname = strings.TrimSpace(report.Hostname)
	if report.Hostname == "" {
		writeJSONError(w, http.StatusBadRequest, "Hostname cannot be empty")
		return
	}

	log.Infof("Received report from host: %s", report.Hostname)

	host, err := db.UpsertHost(r.Context(), app.DB, report.Hostname, "root", report.UpdateOutput, report.UpgradeOutput, "")
	if err != nil {
		log.Errorf("Failed to upsert host: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to process report")
		return
	}

	log.Infof("Upserted host: %s (ID: %d)", host.Hostname, host.ID)
	w.WriteHeader(http.StatusAccepted)
}

func (app *Application) handleListHosts(w http.ResponseWriter, r *http.Request) {
	hosts, err := db.ListHosts(r.Context(), app.DB)
	if err != nil {
		log.Errorf("Failed to list hosts: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve hosts")
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
		writeJSONError(w, http.StatusBadRequest, "Invalid host ID")
		return
	}

	host, err := db.GetHost(r.Context(), app.DB, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Host not found")
		} else {
			log.Errorf("Failed to get host: %v", err)
			writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve host")
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(host)
}

// handleCreateHost lets an operator create a host record without going
// through agent enrollment. Returns 409 Conflict if the hostname already
// exists.
//
// When the request body includes a `password`, the handler does a one-shot
// enrollment: it password-SSHes into the host, generates a fresh ed25519
// keypair, installs the public key, configures passwordless sudo for
// non-root users, captures the host key, and verifies the new key works.
// On any failure during that flow the host row is rolled back so the UI
// doesn't end up with a half-configured record. The password itself is
// never stored — it lives in memory only for the duration of this call.
func (app *Application) handleCreateHost(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req struct {
		Hostname string `json:"hostname"`
		SshUser  string `json:"ssh_user"`
		Password string `json:"password"` // optional; triggers auto-enrollment
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

	// No password = legacy path: just the row. Operator will paste a key
	// later via the SSH tab.
	if req.Password == "" {
		log.Infof("Operator created host: %s (ID: %d)", host.Hostname, host.ID)
		app.audit(r, audit.ActionHostCreate, "host", strconv.FormatInt(int64(host.ID), 10),
			map[string]interface{}{"hostname": host.Hostname, "ssh_user": host.SshUser})
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(host)
		return
	}

	// Auto-enroll path. Use a longer ctx than the request — the client
	// disconnecting mid-bootstrap shouldn't leave a half-configured host.
	enrollCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	result, bootstrapErr := app.SSHDialer.Bootstrap(enrollCtx, req.Hostname, req.SshUser, req.Password)
	if bootstrapErr != nil {
		// Roll back the host row so the operator can retry from a clean
		// slate. Use enrollCtx (not r.Context()) so client cancellation
		// doesn't leave an orphan.
		if _, delErr := db.DeleteHost(enrollCtx, app.DB, host.ID); delErr != nil {
			log.Errorf("Auto-enroll rollback for host %d failed: %v (original: %v)", host.ID, delErr, bootstrapErr)
		}
		log.Warnf("Auto-enroll failed for %s: %v", req.Hostname, bootstrapErr)
		writeJSONError(w, http.StatusBadGateway, "Auto-enrollment failed. Please check host credentials and network.")
		return
	}

	if err := db.AddSSHKey(enrollCtx, app.DB, host.ID, result.PrivateKeyPEM); err != nil {
		// Same rollback rationale.
		_, _ = db.DeleteHost(enrollCtx, app.DB, host.ID)
		log.Errorf("Auto-enroll: store key failed for host %d: %v", host.ID, err)
		writeJSONError(w, http.StatusInternalServerError, "Enrollment succeeded but storing the key failed; please retry")
		return
	}

	if err := app.SSHDialer.AppendKnownHost(req.Hostname, result.HostKey); err != nil {
		// Non-fatal but log loud — without the known_hosts entry, regular
		// dials will fail until the operator restarts the backend.
		log.Errorf("Auto-enroll: append known_hosts for %s failed: %v", req.Hostname, err)
	}

	log.Infof("Auto-enrolled host: %s (ID: %d, sudo=%v)", host.Hostname, host.ID, result.SudoConfigured)
	app.audit(r, audit.ActionHostBootstrap, "host", strconv.FormatInt(int64(host.ID), 10),
		map[string]interface{}{
			"hostname":    host.Hostname,
			"ssh_user":    host.SshUser,
			"fingerprint": result.HostKeyFingerprint,
			"sudo_scope":  result.SudoScope,
		})
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
	app.audit(r, audit.ActionHostDelete, "host", strconv.FormatInt(int64(id), 10),
		map[string]interface{}{"hostname": host.Hostname})
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
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.URL = strings.TrimSpace(req.URL)
	req.Event = strings.TrimSpace(req.Event)
	if req.URL == "" || req.Event == "" {
		writeJSONError(w, http.StatusBadRequest, "URL and event are required")
		return
	}
	if !strings.HasPrefix(req.URL, "http://") && !strings.HasPrefix(req.URL, "https://") {
		writeJSONError(w, http.StatusBadRequest, "URL must start with http:// or https://")
		return
	}

	if _, err := app.DB.Exec(r.Context(), `INSERT INTO webhooks (url, event) VALUES ($1, $2)`, req.URL, req.Event); err != nil {
		log.Errorf("Failed to add webhook: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to add webhook")
		return
	}
	app.audit(r, audit.ActionWebhookCreate, "webhook", req.URL,
		map[string]interface{}{"event": req.Event})
	w.WriteHeader(http.StatusCreated)
}

// handleAutoConfigure runs the bootstrap flow against an existing host
// (one that the operator added without a password, or that was created
// by an agent enroll but never had a key pasted in). Same flow as the
// inline auto-enroll in handleCreateHost — generate a fresh key, install
// it, configure passwordless sudo for non-root, capture host key — but
// without the rollback, since we're keeping the host row regardless.
func (app *Application) handleAutoConfigure(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	id, err := parseHostID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid host ID")
		return
	}

	var req struct {
		Password string `json:"password"`
		SshUser  string `json:"ssh_user,omitempty"` // optional override
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	req.Password = strings.TrimSpace(req.Password)
	if req.Password == "" {
		writeJSONError(w, http.StatusBadRequest, "Password is required")
		return
	}

	host, err := db.GetHost(r.Context(), app.DB, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Host not found")
			return
		}
		log.Errorf("auto-configure: get host %d: %v", id, err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve host")
		return
	}

	// Allow the caller to override ssh_user in the same request — common
	// when the original record had a placeholder. Persist it before we
	// run bootstrap so the new key matches the user we install it for.
	sshUser := strings.TrimSpace(req.SshUser)
	if sshUser != "" && sshUser != host.SshUser {
		updated, err := db.UpdateHostSSHUser(r.Context(), app.DB, id, sshUser)
		if err != nil {
			log.Errorf("auto-configure: update ssh_user for host %d: %v", id, err)
			writeJSONError(w, http.StatusInternalServerError, "Failed to update ssh_user")
			return
		}
		host = updated
	}

	enrollCtx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	result, bootstrapErr := app.SSHDialer.Bootstrap(enrollCtx, host.Hostname, host.SshUser, req.Password)
	if bootstrapErr != nil {
		log.Warnf("auto-configure failed for %s (id=%d): %v", host.Hostname, host.ID, bootstrapErr)
		writeJSONError(w, http.StatusBadGateway, "Auto-configuration failed: "+bootstrapErr.Error())
		return
	}

	if err := db.AddSSHKey(enrollCtx, app.DB, host.ID, result.PrivateKeyPEM); err != nil {
		log.Errorf("auto-configure: store key for host %d: %v", host.ID, err)
		writeJSONError(w, http.StatusInternalServerError, "Configuration succeeded but storing the key failed; please retry")
		return
	}

	if err := app.SSHDialer.AppendKnownHost(host.Hostname, result.HostKey); err != nil {
		log.Errorf("auto-configure: append known_hosts for %s: %v", host.Hostname, err)
	}

	log.Infof("Auto-configured host: %s (ID: %d, sudo=%v)", host.Hostname, host.ID, result.SudoConfigured)
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":              true,
		"sudo_configured": result.SudoConfigured,
	})
}

// handleTestConnection probes a host's SSH stack and reports back. Used by the
// UI's "Test connection" button so the operator can validate the saved key
// (and passwordless sudo for non-root users) before triggering a real update.
// 7 seconds is plenty for a healthy host and short enough to be UI-friendly.
func (app *Application) handleTestConnection(w http.ResponseWriter, r *http.Request) {
	id, err := parseHostID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid host ID")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 7*time.Second)
	defer cancel()

	result, err := app.SSHDialer.TestConnection(ctx, id)
	if err != nil {
		log.Errorf("test-connection failed for host %d: %v", id, err)
		writeJSONError(w, http.StatusInternalServerError, "Test failed")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

func (app *Application) handleAddSSHKey(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	id, err := parseHostID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid host ID")
		return
	}

	var req struct {
		SshUser    string `json:"ssh_user"`
		PrivateKey string `json:"private_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	req.SshUser = strings.TrimSpace(req.SshUser)
	req.PrivateKey = strings.TrimSpace(req.PrivateKey)
	if req.SshUser == "" || req.PrivateKey == "" {
		writeJSONError(w, http.StatusBadRequest, "ssh_user and private_key are required")
		return
	}

	// Sanity-check the key parses before we put it on disk in any form. Bad
	// PEM blobs are a common operator-paste error and the worst time to find
	// out is when the next SSH dial silently fails.
	if _, parseErr := ssh.ParsePrivateKey([]byte(req.PrivateKey)); parseErr != nil {
		log.Warnf("Failed to parse private key for host %d: %v", id, parseErr)
		writeJSONError(w, http.StatusBadRequest, "private_key does not parse as a valid OpenSSH private key")
		return
	}

	if err := db.SetSSHKeyAndUser(r.Context(), app.DB, id, req.SshUser, req.PrivateKey); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Host not found")
			return
		}
		log.Errorf("Failed to set SSH key for host %d: %v", id, err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to save SSH key")
		return
	}

	app.audit(r, audit.ActionHostKeyInstall, "host", strconv.FormatInt(int64(id), 10),
		map[string]interface{}{"ssh_user": req.SshUser})

	w.WriteHeader(http.StatusCreated)
}

func (app *Application) handleExecuteScript(w http.ResponseWriter, r *http.Request) {
	id, err := parseHostID(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid host ID")
		return
	}

	// C4: Explicitly validate the session token BEFORE upgrading to WebSocket.
	// WebSocket upgrade is a plain GET which bypasses the CSRF middleware.
	// Without this check, the route relies solely on the subrouter's cookie
	// session, which is insufficient for arbitrary command execution.
	token := r.URL.Query().Get("token")
	if app.Sessions != nil {
		if token == "" {
			writeJSONError(w, http.StatusUnauthorized, "Missing token")
			return
		}
		if _, valid, _ := app.Sessions.Validate(r.Context(), token); !valid {
			writeJSONError(w, http.StatusUnauthorized, "Invalid or expired session")
			return
		}
	} else {
		// Legacy in-memory token store path (tests / no-DB mode).
		if token == "" {
			writeJSONError(w, http.StatusUnauthorized, "Missing token")
			return
		}
		if _, valid := app.TokenStore.ValidateToken(token); !valid {
			writeJSONError(w, http.StatusUnauthorized, "Invalid or expired session")
			return
		}
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

	// Audit every script execution. The script body itself can be unbounded;
	// we cap what we record so a paste of a 10MB binary doesn't bloat the log,
	// but we hash the full body for non-repudiation.
	scriptStr := string(script)
	
	// Add a reasonable hard limit so malicious clients can't OOM the server or
	// the SSH session (typically limited to 256KB or so depending on sshd_config).
	const maxScriptBytes = 128 * 1024 // 128 KB
	if len(scriptStr) > maxScriptBytes {
		log.Errorf("Script exceeded maximum size: %d bytes", len(scriptStr))
		_ = conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: Script exceeds maximum size of %d bytes", maxScriptBytes)))
		return
	}

	preview := scriptStr
	const maxAuditedScript = 4096
	if len(preview) > maxAuditedScript {
		preview = preview[:maxAuditedScript] + "…(truncated)"
	}
	
	hash := sha256.Sum256(script)
	hashHex := hex.EncodeToString(hash[:])
	
	app.audit(r, audit.ActionRunScript, "host", strconv.FormatInt(int64(id), 10),
		map[string]interface{}{
			"script_preview": preview, 
			"script_bytes": len(scriptStr),
			"script_sha256": hashHex,
		})

	sshClient, _, err := app.SSHDialer.ConnectToHost(r.Context(), id)
	if err != nil {
		log.Errorf("SSH connect to host %d failed: %v", id, err)
		_ = conn.WriteMessage(websocket.TextMessage, []byte("SSH connect failed: "+err.Error()))
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

	output, err := session.CombinedOutput(scriptStr)
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
		writeJSONError(w, http.StatusBadRequest, "Invalid host ID")
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
		writeJSONError(w, http.StatusBadRequest, "Invalid host ID")
		return
	}
	// Look up the host first so we know which ssh_user is configured; that
	// controls whether the script needs `sudo -n`.
	host, err := db.GetHost(r.Context(), app.DB, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeJSONError(w, http.StatusNotFound, "Host not found")
			return
		}
		log.Errorf("Failed to get host %d: %v", id, err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve host")
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
func pumpReader(ctx context.Context, dbCtx context.Context, conn *websocket.Conn, pool db.DBTX, runID int32, src io.Reader) {
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
		if parsed, err := strconv.Atoi(v); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	// Cap to prevent memory exhaustion on malicious limit values.
	if limit > 200 {
		limit = 200
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

// uuidPattern matches the v4-style UUIDs we generate for run groups. Used to
// reject bogus query params before they hit the DB.
var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// handleListRunsByGroup returns all runs for a bulk run_group_id. Used by the
// BulkUpdate page to render per-host progress without subscribing to N
// websockets.
func (app *Application) handleListRunsByGroup(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group_id")
	if group == "" || !uuidPattern.MatchString(group) {
		writeJSONError(w, http.StatusBadRequest, "group_id query parameter required (UUID format)")
		return
	}
	runs, err := db.ListRunsForGroup(r.Context(), app.DB, group)
	if err != nil {
		log.Errorf("Failed to list runs for group %s: %v", group, err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to retrieve runs")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(runs)
}

// handleBulkRunUpdate fans an apt-get upgrade across many hosts. Bounded by
// MaxConcurrency in the updater package; further constrained by an in-flight
// "one bulk per server" cap so an over-eager operator can't pile up a hundred
// fan-outs in parallel.
func (app *Application) handleBulkRunUpdate(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodySize)

	var req struct {
		HostIDs            []int32 `json:"host_ids"`
		Concurrency        int     `json:"concurrency,omitempty"`
		CanaryCount        int     `json:"canary_count,omitempty"`
		CanaryWaitSeconds  int     `json:"canary_wait_seconds,omitempty"`
		AbortOnFailurePct  int     `json:"abort_on_failure_pct,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if len(req.HostIDs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "host_ids must not be empty")
		return
	}
	if len(req.HostIDs) > 200 {
		writeJSONError(w, http.StatusBadRequest, "host_ids capped at 200 per request")
		return
	}

	// Cheap rate-limit: one bulk group at a time per server. The plan called
	// out per-user, but with single-admin auth today this is equivalent.
	if app.BulkUpdater.InFlightCount() >= 1 {
		writeJSONError(w, http.StatusConflict, "Another bulk update is already running. Try again when it finishes.")
		return
	}

	user := middleware.GetUserFromContext(r)
	triggeredBy := "unknown"
	if user != nil {
		triggeredBy = user.Username
	}

	result, err := app.BulkUpdater.Start(r.Context(), updater.BulkRunOptions{
		HostIDs:            req.HostIDs,
		Concurrency:        req.Concurrency,
		TriggeredBy:        triggeredBy,
		CanaryCount:        req.CanaryCount,
		CanaryWaitSeconds:  req.CanaryWaitSeconds,
		AbortOnFailurePct:  req.AbortOnFailurePct,
	})
	if err != nil {
		log.Errorf("bulk update start failed: %v", err)
		writeJSONError(w, http.StatusInternalServerError, "Failed to start bulk update: "+err.Error())
		return
	}

	log.Infof("Bulk update %s triggered by %s across %d hosts", result.GroupID, triggeredBy, len(req.HostIDs))
	app.audit(r, audit.ActionRunBulkUpdate, "run_group", result.GroupID,
		map[string]interface{}{
			"host_count":         len(req.HostIDs),
			"canary_count":       req.CanaryCount,
			"canary_wait_seconds": req.CanaryWaitSeconds,
			"abort_on_failure_pct": req.AbortOnFailurePct,
		})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(result)
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
