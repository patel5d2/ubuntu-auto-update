package middleware

import (
	"net/http"
)

// SecurityHeaders adds defense-in-depth HTTP headers to every response.
// These mitigate entire classes of attack (clickjacking, MIME-sniffing,
// content injection) at near-zero cost.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME-type sniffing — forces the browser to honor Content-Type.
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Clickjacking protection — no framing allowed.
		w.Header().Set("X-Frame-Options", "DENY")

		// XSS filter: some browsers still support this legacy header.
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Referrer policy: don't leak full URLs on navigation.
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content-Security-Policy: restrict script and style sources to self.
		// 'unsafe-inline' is needed for Vite's injected styles in dev; a nonce-based
		// CSP is better but requires wiring through the template layer.
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"script-src 'self'; "+
				"style-src 'self' 'unsafe-inline' https://fonts.googleapis.com; "+
				"font-src 'self' https://fonts.gstatic.com; "+
				"img-src 'self' data:; "+
				"connect-src 'self' ws: wss:; "+
				"frame-ancestors 'none'")

		// Permissions-Policy: disable browser features we don't use.
		w.Header().Set("Permissions-Policy",
			"camera=(), microphone=(), geolocation=(), payment=()")

		// HSTS: only set in production where TLS is terminated.
		if isProduction() {
			w.Header().Set("Strict-Transport-Security",
				"max-age=63072000; includeSubDomains; preload")
		}

		next.ServeHTTP(w, r)
	})
}
