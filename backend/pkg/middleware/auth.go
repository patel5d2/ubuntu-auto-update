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
)

type contextKey string

const (
	UserContextKey contextKey = "user"
)

type User struct {
	ID       string
	Username string
	Role     string
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	CookieName   string
	RequiredRole string
}

// NewAuthConfig creates auth config from environment
func NewAuthConfig() *AuthConfig {
	return &AuthConfig{
		CookieName: "auth_token",
	}
}

// TokenStore provides in-memory token storage with expiry
type TokenStore struct {
	mu     sync.RWMutex
	tokens map[string]TokenEntry
}

type TokenEntry struct {
	Username  string
	ExpiresAt time.Time
}

var globalTokenStore = &TokenStore{
	tokens: make(map[string]TokenEntry),
}

// GetTokenStore returns the global token store
func GetTokenStore() *TokenStore {
	return globalTokenStore
}

// StoreToken adds a token to the store
func (ts *TokenStore) StoreToken(token, username string, expiry time.Duration) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	ts.tokens[token] = TokenEntry{
		Username:  username,
		ExpiresAt: time.Now().Add(expiry),
	}
}

// ValidateToken checks if a token is valid and not expired
func (ts *TokenStore) ValidateToken(token string) (string, bool) {
	ts.mu.RLock()
	defer ts.mu.RUnlock()
	entry, exists := ts.tokens[token]
	if !exists {
		return "", false
	}
	if time.Now().After(entry.ExpiresAt) {
		return "", false
	}
	return entry.Username, true
}

// RemoveToken removes a token from the store
func (ts *TokenStore) RemoveToken(token string) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	delete(ts.tokens, token)
}

// CleanExpiredTokens removes all expired tokens
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

// GenerateSecureToken creates a cryptographically random token
func GenerateSecureToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate secure token: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

// SetAuthCookie sets authentication cookie with secure defaults
func SetAuthCookie(w http.ResponseWriter, config *AuthConfig, tokenString string) {
	isProduction := os.Getenv("ENVIRONMENT") == "production"
	cookie := &http.Cookie{
		Name:     config.CookieName,
		Value:    tokenString,
		Path:     "/",
		HttpOnly: true,
		Secure:   isProduction,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400, // 24 hours
	}
	http.SetCookie(w, cookie)
}

// ClearAuthCookie clears authentication cookie
func ClearAuthCookie(w http.ResponseWriter, config *AuthConfig) {
	isProduction := os.Getenv("ENVIRONMENT") == "production"
	cookie := &http.Cookie{
		Name:     config.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isProduction,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	}
	http.SetCookie(w, cookie)
}

// GetUserFromContext extracts user from request context
func GetUserFromContext(r *http.Request) *User {
	if user, ok := r.Context().Value(UserContextKey).(*User); ok {
		return user
	}
	return nil
}

// RoleMiddleware checks for specific role
func RoleMiddleware(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := GetUserFromContext(r)
			if user == nil {
				SendAuthError(w, "User context not found")
				return
			}

			// Admin role bypasses all role checks
			if user.Role != "admin" && user.Role != requiredRole {
				SendForbiddenError(w, fmt.Sprintf("Role %s required", requiredRole))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TokenAuthMiddleware validates tokens from the token store
func TokenAuthMiddleware(store *TokenStore, config *AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var tokenString string

			// Try to get token from cookie first (for web UI)
			if cookie, err := r.Cookie(config.CookieName); err == nil {
				tokenString = cookie.Value
			}

			// If no cookie, try Authorization header (for API clients / agents)
			if tokenString == "" {
				authHeader := r.Header.Get("Authorization")
				if strings.HasPrefix(authHeader, "Bearer ") {
					tokenString = strings.TrimPrefix(authHeader, "Bearer ")
				}
			}

			if tokenString == "" {
				SendAuthError(w, "No authentication token provided")
				return
			}

			// Validate token
			username, valid := store.ValidateToken(tokenString)
			if !valid {
				SendAuthError(w, "Invalid or expired authentication token")
				return
			}

			// Add user to request context
			user := &User{
				Username: username,
				Role:     "admin", // For now, all authenticated users are admin
			}
			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// StartTokenCleanup starts a background goroutine that cleans expired tokens
func StartTokenCleanup(store *TokenStore, interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			store.CleanExpiredTokens()
			log.Debug("Expired tokens cleaned up")
		}
	}()
}