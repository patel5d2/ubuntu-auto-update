package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"ubuntu-auto-update/backend/pkg/session"
)

type contextKey string

const (
	UserContextKey      contextKey = "user"
	PrincipalContextKey contextKey = "principal"
)

// User is the legacy actor representation surfaced via GetUserFromContext. New
// code should reach for the richer Principal via GetPrincipalFromContext.
type User struct {
	ID       string
	Username string
	Role     string
}

type AuthConfig struct {
	CookieName   string
	RequiredRole string
}

func NewAuthConfig() *AuthConfig {
	return &AuthConfig{CookieName: "auth_token"}
}

// ---------------------------------------------------------------------------
// Legacy in-memory token store. Retained because:
//   - the existing main_test.go and middleware_test.go construct it directly,
//   - it remains a valid choice for single-process dev environments.
// New production code should use pkg/session.NewDBStore via SessionAuthMiddleware.
// ---------------------------------------------------------------------------

type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]TokenEntry
}

type TokenEntry struct {
	Username  string
	Role      string
	ExpiresAt time.Time
}

var globalTokenStore = &TokenStore{tokens: make(map[string]TokenEntry)}

func GetTokenStore() *TokenStore { return globalTokenStore }

// StoreToken adds an admin-role token. Kept for backwards compatibility with
// callers that don't yet supply a role.
func (ts *TokenStore) StoreToken(token, username string, expiry time.Duration) {
	ts.StoreTokenWithRole(token, username, "admin", expiry)
}

// StoreTokenWithRole is the role-aware variant; new code should prefer this.
func (ts *TokenStore) StoreTokenWithRole(token, username, role string, expiry time.Duration) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.tokens[token] = TokenEntry{
		Username:  username,
		Role:      role,
		ExpiresAt: time.Now().Add(expiry),
	}
}

func (ts *TokenStore) ValidateToken(token string) (string, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	entry, exists := ts.tokens[token]
	if !exists || time.Now().After(entry.ExpiresAt) {
		return "", false
	}
	return entry.Username, true
}

func (ts *TokenStore) RemoveToken(token string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.tokens, token)
}

func (ts *TokenStore) CleanExpiredTokens() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	now := time.Now()
	for token, entry := range ts.tokens {
		if now.After(entry.ExpiresAt) {
			delete(ts.tokens, token)
		}
	}
}

// GenerateSecureToken — kept for callers that still need a raw token
// independent of session storage (e.g. legacy enrollment).
func GenerateSecureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// ---------------------------------------------------------------------------
// Cookie helpers, principal context, RBAC.
// ---------------------------------------------------------------------------

func SetAuthCookie(w http.ResponseWriter, config *AuthConfig, tokenString string) {
	isProduction := os.Getenv("ENVIRONMENT") == "production"
	http.SetCookie(w, &http.Cookie{
		Name:     config.CookieName,
		Value:    tokenString,
		Path:     "/",
		HttpOnly: true,
		Secure:   isProduction,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
}

func ClearAuthCookie(w http.ResponseWriter, config *AuthConfig) {
	isProduction := os.Getenv("ENVIRONMENT") == "production"
	http.SetCookie(w, &http.Cookie{
		Name:     config.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isProduction,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func GetUserFromContext(r *http.Request) *User {
	if user, ok := r.Context().Value(UserContextKey).(*User); ok {
		return user
	}
	return nil
}

// GetPrincipalFromContext returns the rich Principal for handlers that need
// role/agent details. Returns nil if no principal was attached (i.e. the
// route is not behind auth middleware).
func GetPrincipalFromContext(r *http.Request) *session.Principal {
	if p, ok := r.Context().Value(PrincipalContextKey).(*session.Principal); ok {
		return p
	}
	return nil
}

// RequireRole gates a handler on a minimum role. Admin always passes. Used as
// `r.Use(middleware.RequireRole(session.RoleOperator))` on a subrouter.
func RequireRole(required string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			p := GetPrincipalFromContext(r)
			if p == nil {
				SendAuthError(w, "No authenticated principal")
				return
			}
			if !p.HasRole(required) {
				SendForbiddenError(w, fmt.Sprintf("Role '%s' required", required))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RoleMiddleware is the legacy variant kept for compatibility. Delegates to
// the principal-aware path when one is available, otherwise falls back to the
// User context.
func RoleMiddleware(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if p := GetPrincipalFromContext(r); p != nil {
				if !p.HasRole(requiredRole) {
					SendForbiddenError(w, fmt.Sprintf("Role %s required", requiredRole))
					return
				}
				next.ServeHTTP(w, r)
				return
			}
			user := GetUserFromContext(r)
			if user == nil {
				SendAuthError(w, "User context not found")
				return
			}
			if user.Role != "admin" && user.Role != requiredRole {
				SendForbiddenError(w, fmt.Sprintf("Role %s required", requiredRole))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ---------------------------------------------------------------------------
// Auth middlewares.
// ---------------------------------------------------------------------------

// extractToken pulls a token out of either the auth cookie or an
// Authorization: Bearer header. Returns "" if neither is present.
func extractToken(r *http.Request, cookieName string) string {
	if cookie, err := r.Cookie(cookieName); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	if h := r.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		return strings.TrimPrefix(h, "Bearer ")
	}
	return ""
}

// TokenAuthMiddleware validates against the legacy TokenStore. Preserved for
// tests and dev. New deployments should use SessionAuthMiddleware.
func TokenAuthMiddleware(store *TokenStore, config *AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := extractToken(r, config.CookieName)
			if tok == "" {
				SendAuthError(w, "No authentication token provided")
				return
			}

			username, valid := store.ValidateToken(tok)
			if !valid {
				SendAuthError(w, "Invalid or expired authentication token")
				return
			}

			user := &User{Username: username, Role: "admin"}
			ctx := context.WithValue(r.Context(), UserContextKey, user)

			// Also attach a Principal so downstream handlers using the new API
			// see something sensible. The role is read from the in-memory entry.
			store.mu.RLock()
			role := "admin"
			if entry, ok := store.tokens[tok]; ok && entry.Role != "" {
				role = entry.Role
			}
			store.mu.RUnlock()
			p := &session.Principal{Username: username, Role: role}
			ctx = context.WithValue(ctx, PrincipalContextKey, p)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// SessionAuthMiddleware validates against a pkg/session.Store. This is the
// production path: it gives us shared state across replicas, agent vs. user
// distinction, and richer principal data (role, user id, session id).
func SessionAuthMiddleware(store session.Store, config *AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tok := extractToken(r, config.CookieName)
			if tok == "" {
				SendAuthError(w, "No authentication token provided")
				return
			}
			p, ok, err := store.Validate(r.Context(), tok)
			if err != nil {
				log.Errorf("session validate: %v", err)
				SendAuthError(w, "Authentication temporarily unavailable")
				return
			}
			if !ok {
				SendAuthError(w, "Invalid or expired authentication token")
				return
			}

			ctx := context.WithValue(r.Context(), PrincipalContextKey, &p)
			// Legacy compatibility for handlers still reading User.
			user := &User{Username: p.Username, Role: p.Role}
			ctx = context.WithValue(ctx, UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// StartTokenCleanup is the legacy ticker for the in-memory store. Production
// code uses session.StartCleanup with a session.Store.
func StartTokenCleanup(store *TokenStore, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			store.CleanExpiredTokens()
		}
	}()
}
