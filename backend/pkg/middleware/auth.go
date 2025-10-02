package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	log "github.com/sirupsen/logrus"
)

type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

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
	JWTSecret       string
	TokenExpiry     time.Duration
	CookieName      string
	RequiredRole    string
}

// NewAuthConfig creates auth config from environment
func NewAuthConfig() *AuthConfig {
	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		// Generate a random secret for development
		bytes := make([]byte, 32)
		if _, err := rand.Read(bytes); err != nil {
			panic("Failed to generate JWT secret")
		}
		jwtSecret = hex.EncodeToString(bytes)
		log.Warn("No JWT_SECRET environment variable set, using generated secret")
	}

	return &AuthConfig{
		JWTSecret:   jwtSecret,
		TokenExpiry: 24 * time.Hour,
		CookieName:  "auth_token",
	}
}

// JWTMiddleware validates JWT tokens
func JWTMiddleware(config *AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try to get token from cookie first (for web UI)
			var tokenString string
			if cookie, err := r.Cookie(config.CookieName); err == nil {
				tokenString = cookie.Value
			}

			// If no cookie, try Authorization header (for API clients)
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

			// Parse and validate token
			claims := &Claims{}
			token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
				// Validate signing method
				if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
				}
				return []byte(config.JWTSecret), nil
			})

			if err != nil || !token.Valid {
				log.WithError(err).Warn("Invalid JWT token")
				SendAuthError(w, "Invalid authentication token")
				return
			}

			// Check token expiry
			if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
				SendAuthError(w, "Authentication token expired")
				return
			}

			// Check required role if specified
			if config.RequiredRole != "" && claims.Role != config.RequiredRole && claims.Role != "admin" {
				SendForbiddenError(w, "Insufficient privileges")
				return
			}

			// Add user to request context
			user := &User{
				ID:       claims.UserID,
				Username: claims.Username,
				Role:     claims.Role,
			}
			ctx := context.WithValue(r.Context(), UserContextKey, user)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// CreateJWT creates a new JWT token
func CreateJWT(config *AuthConfig, userID, username, role string) (string, error) {
	claims := Claims{
		UserID:   userID,
		Username: username,
		Role:     role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(config.TokenExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
			Issuer:    "ubuntu-auto-update",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.JWTSecret))
}

// SetAuthCookie sets authentication cookie
func SetAuthCookie(w http.ResponseWriter, config *AuthConfig, tokenString string) {
	cookie := &http.Cookie{
		Name:     config.CookieName,
		Value:    tokenString,
		Path:     "/",
		HttpOnly: true,
		Secure:   os.Getenv("ENVIRONMENT") == "production", // HTTPS in production
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(config.TokenExpiry),
	}
	http.SetCookie(w, cookie)
}

// ClearAuthCookie clears authentication cookie
func ClearAuthCookie(w http.ResponseWriter, config *AuthConfig) {
	cookie := &http.Cookie{
		Name:     config.CookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   os.Getenv("ENVIRONMENT") == "production",
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Now().Add(-time.Hour), // Expire immediately
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