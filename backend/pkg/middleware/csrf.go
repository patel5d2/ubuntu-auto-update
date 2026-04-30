package middleware

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"os"
	"strings"
)

// CSRF protection via the double-submit-cookie pattern.
//
//   - On login the server sets a `csrf_token` cookie (NOT HttpOnly so JS can
//     read it) along with the auth_token cookie.
//   - The browser is expected to echo that value back in the X-CSRF-Token
//     header on every state-changing cookie-authenticated request.
//   - Requests that authenticate via Authorization: Bearer don't go through
//     the browser cookie path, so they bypass CSRF (no cross-origin auth ride).
//
// Methods exempted: GET, HEAD, OPTIONS — the spec already considers these
// safe.

const (
	CSRFCookieName = "csrf_token"
	CSRFHeader     = "X-CSRF-Token"
)

// GenerateCSRFToken returns a fresh random CSRF token.
func GenerateCSRFToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// SetCSRFCookie writes the CSRF token cookie. NOT HttpOnly: the browser JS
// reads this and echoes it as a header. Secure follows ENVIRONMENT=production
// to match the auth cookie's policy.
func SetCSRFCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: false,
		Secure:   isProduction(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   86400,
	})
}

// ClearCSRFCookie wipes the cookie on logout.
func ClearCSRFCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: false,
		Secure:   isProduction(),
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
	})
}

func isProduction() bool {
	return os.Getenv("ENVIRONMENT") == "production"
}

// CSRFMiddleware enforces the double-submit pattern for state-changing
// cookie-authenticated requests. Bearer-auth requests pass through unchecked.
func CSRFMiddleware(authCookieName string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if isSafeMethod(r.Method) {
				next.ServeHTTP(w, r)
				return
			}

			// Bearer auth bypasses CSRF — no cookie ride means no CSRF surface.
			if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
				next.ServeHTTP(w, r)
				return
			}

			// Cookie auth: validate the double-submit token.
			cookie, err := r.Cookie(CSRFCookieName)
			if err != nil || cookie.Value == "" {
				SendForbiddenError(w, "CSRF token missing")
				return
			}
			header := r.Header.Get(CSRFHeader)
			if header == "" {
				SendForbiddenError(w, "CSRF header missing")
				return
			}
			if subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(header)) != 1 {
				SendForbiddenError(w, "CSRF token mismatch")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func isSafeMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
		return true
	}
	return false
}
