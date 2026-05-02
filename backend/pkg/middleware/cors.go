package middleware

import (
	"net/http"
	"os"
	"strings"
)

// CORSConfig caches the parsed allowed origins so we don't re-split the env var
// on every request.
type CORSConfig struct {
	AllowedOrigins []string
	AllowAll       bool
}

// LoadCORSConfig reads CORS_ALLOWED_ORIGINS once at startup. Defaults to the
// usual local dev origins (Vite + CRA) when the env var is unset.
func LoadCORSConfig() *CORSConfig {
	raw := os.Getenv("CORS_ALLOWED_ORIGINS")
	if raw == "" {
		raw = "http://localhost:5173,http://localhost:3000"
	}
	cfg := &CORSConfig{}
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o == "*" {
			cfg.AllowAll = true
		}
		if o != "" {
			cfg.AllowedOrigins = append(cfg.AllowedOrigins, o)
		}
	}
	return cfg
}

// IsAllowed returns true if the given origin matches the configured allow list.
func (c *CORSConfig) IsAllowed(origin string) bool {
	if origin == "" {
		return false
	}
	if c.AllowAll {
		return true
	}
	for _, allowed := range c.AllowedOrigins {
		if allowed == origin {
			return true
		}
	}
	return false
}

// CORS returns a middleware that applies the cached CORS configuration.
func CORS(cfg *CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if cfg.IsAllowed(origin) {
				w.Header().Set("Access-Control-Allow-Origin", origin)
			}
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-CSRF-Token, X-Confirm-Hostname")
			w.Header().Set("Access-Control-Allow-Credentials", "true")

			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
