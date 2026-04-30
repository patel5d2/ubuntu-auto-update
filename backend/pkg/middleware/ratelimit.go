package middleware

import (
	"context"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// LoginRateLimiter is a small token-bucket per IP, sized for the login
// endpoint specifically. Generous enough to not block humans (5 attempts
// per minute) and tight enough to make automated guessing painful.
type LoginRateLimiter struct {
	mu       sync.Mutex
	buckets  map[string]*bucket
	rate     float64 // tokens per second
	capacity float64
}

type bucket struct {
	tokens float64
	last   time.Time
}

// NewLoginRateLimiter returns a limiter sized for human login traffic.
func NewLoginRateLimiter() *LoginRateLimiter {
	return &LoginRateLimiter{
		buckets:  make(map[string]*bucket),
		rate:     5.0 / 60.0, // 5 per minute
		capacity: 5,
	}
}

// Allow returns true iff the request from `key` should be processed.
// Increments the bucket as a side-effect.
func (l *LoginRateLimiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.capacity, last: now}
		l.buckets[key] = b
	}
	elapsed := now.Sub(b.last).Seconds()
	b.tokens += elapsed * l.rate
	if b.tokens > l.capacity {
		b.tokens = l.capacity
	}
	b.last = now

	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// CleanIdle drops buckets that haven't been touched in `older` so the map
// doesn't grow unboundedly when many distinct IPs hit /login once.
func (l *LoginRateLimiter) CleanIdle(older time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	cutoff := time.Now().Add(-older)
	for k, b := range l.buckets {
		if b.last.Before(cutoff) {
			delete(l.buckets, k)
		}
	}
}

// StartLoginLimiterCleanup runs CleanIdle on `interval`, dropping buckets
// idle for `idle`. Stops when ctx is cancelled. Without this the bucket map
// grows unboundedly across the lifetime of the process — one entry per
// distinct source IP that ever hit /login.
func StartLoginLimiterCleanup(ctx context.Context, l *LoginRateLimiter, interval, idle time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				l.CleanIdle(idle)
			}
		}
	}()
}

// ---------------------------------------------------------------------------
// IP allowlist + helpers.
// ---------------------------------------------------------------------------

// IPAllowlist is a simple CIDR-based allow list. Empty means "allow all".
type IPAllowlist struct {
	nets    []*net.IPNet
	enabled bool
}

// NewIPAllowlist parses comma-separated CIDRs (also accepts bare IPs).
func NewIPAllowlist(commaSeparated string) (*IPAllowlist, error) {
	commaSeparated = strings.TrimSpace(commaSeparated)
	if commaSeparated == "" {
		return &IPAllowlist{enabled: false}, nil
	}
	a := &IPAllowlist{enabled: true}
	for _, raw := range strings.Split(commaSeparated, ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if !strings.Contains(raw, "/") {
			// Bare IP — promote to /32 or /128 as appropriate.
			ip := net.ParseIP(raw)
			if ip == nil {
				return nil, &net.ParseError{Type: "IP", Text: raw}
			}
			if ip.To4() != nil {
				raw += "/32"
			} else {
				raw += "/128"
			}
		}
		_, n, err := net.ParseCIDR(raw)
		if err != nil {
			return nil, err
		}
		a.nets = append(a.nets, n)
	}
	return a, nil
}

// Allow returns true if the given remote IP matches a registered CIDR. When
// the allowlist is disabled (no entries configured) Allow always returns true.
func (a *IPAllowlist) Allow(remoteAddr string) bool {
	if a == nil || !a.enabled {
		return true
	}
	host := remoteAddr
	if h, _, err := net.SplitHostPort(remoteAddr); err == nil {
		host = h
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	for _, n := range a.nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

// IPAllowlistMiddleware blocks requests whose remote IP isn't on the list.
// Disabled lists pass everything through. Uses ClientIP so X-Forwarded-For
// is honored when TRUST_FORWARDED_FOR is set — required behind any load
// balancer or reverse proxy.
func IPAllowlistMiddleware(a *IPAllowlist) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !a.Allow(ClientIP(r)) {
				SendForbiddenError(w, "Source IP is not allowed")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// ClientIP returns the best-guess origin IP for a request. Trusts the first
// X-Forwarded-For entry only when TRUST_FORWARDED_FOR is enabled (opt-in,
// because anyone can set the header).
func ClientIP(r *http.Request) string {
	if envIsTrue("TRUST_FORWARDED_FOR") {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.Split(xff, ",")
			if len(parts) > 0 {
				return strings.TrimSpace(parts[0])
			}
		}
	}
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		return h
	}
	return r.RemoteAddr
}

func envIsTrue(env string) bool {
	v := strings.ToLower(os.Getenv(env))
	return v == "1" || v == "true" || v == "yes"
}
