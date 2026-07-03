package middleware

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ---------------------------------------------------------------------------
// Prometheus metrics for the HTTP layer.
//
// All counters and histograms use the "uau_" namespace so they group cleanly
// in dashboards and don't collide with third-party exporters.
// ---------------------------------------------------------------------------

var (
	httpRequestsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Namespace: "uau",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests processed, partitioned by method, path, and response code.",
		},
		[]string{"method", "path", "code"},
	)

	httpRequestDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Namespace: "uau",
			Name:      "http_request_duration_seconds",
			Help:      "Histogram of HTTP request latencies in seconds.",
			Buckets:   prometheus.DefBuckets,
		},
		[]string{"method", "path"},
	)

	httpRequestsInFlight = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "uau",
			Name:      "http_requests_in_flight",
			Help:      "Number of HTTP requests currently being served.",
		},
	)

	// Application-level gauges the main package populates.
	HostsTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "uau",
			Name:      "hosts_total",
			Help:      "Total number of enrolled hosts.",
		},
	)

	ActiveSessions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "uau",
			Name:      "active_sessions",
			Help:      "Number of active (non-expired) sessions.",
		},
	)

	DBPoolTotal = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "uau",
			Name:      "db_pool_total_connections",
			Help:      "Total connections in the database pool.",
		},
	)

	DBPoolIdle = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "uau",
			Name:      "db_pool_idle_connections",
			Help:      "Idle connections in the database pool.",
		},
	)

	DBPoolMax = promauto.NewGauge(
		prometheus.GaugeOpts{
			Namespace: "uau",
			Name:      "db_pool_max_connections",
			Help:      "Maximum connections allowed in the database pool.",
		},
	)
)

// PrometheusMiddleware records request count, latency histogram, and
// in-flight gauge for every HTTP request.
func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip metrics endpoint itself to avoid self-referential loops.
		if r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}

		httpRequestsInFlight.Inc()
		defer httpRequestsInFlight.Dec()

		start := time.Now()
		rw := &metricsResponseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		next.ServeHTTP(rw, r)

		duration := time.Since(start).Seconds()
		code := strconv.Itoa(rw.statusCode)

		// Normalize path to prevent cardinality explosion.
		path := normalizePath(r.URL.Path)

		httpRequestsTotal.WithLabelValues(r.Method, path, code).Inc()
		httpRequestDuration.WithLabelValues(r.Method, path).Observe(duration)
	})
}

// metricsResponseWriter captures the status code for Prometheus labelling.
type metricsResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *metricsResponseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack forwards to the underlying writer so WebSocket upgrades work
// through this middleware (see the matching method on responseWriter).
func (rw *metricsResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
	}
	return h.Hijack()
}

// normalizePath collapses dynamic path segments so Prometheus labels stay
// bounded. /api/v1/hosts/42 → /api/v1/hosts/:id, etc.
func normalizePath(path string) string {
	// Only normalize API paths; static assets are behind the SPA handler.
	if len(path) < 8 || path[:8] != "/api/v1/" {
		return "/static"
	}

	parts := splitPath(path)
	for i := range parts {
		if isIDSegment(parts[i]) {
			parts[i] = ":id"
		}
	}
	return joinPath(parts)
}

func splitPath(p string) []string {
	var parts []string
	start := 0
	for i := 0; i < len(p); i++ {
		if p[i] == '/' {
			if i > start {
				parts = append(parts, p[start:i])
			}
			start = i + 1
		}
	}
	if start < len(p) {
		parts = append(parts, p[start:])
	}
	return parts
}

func joinPath(parts []string) string {
	result := ""
	for _, p := range parts {
		result += "/" + p
	}
	if result == "" {
		return "/"
	}
	return result
}

func isIDSegment(s string) bool {
	if len(s) == 0 {
		return false
	}
	// Pure numeric IDs.
	allDigit := true
	for _, c := range s {
		if c < '0' || c > '9' {
			allDigit = false
			break
		}
	}
	if allDigit {
		return true
	}
	// UUID-shaped segments (36 chars with dashes).
	if len(s) == 36 {
		for _, c := range s {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') || c == '-') {
				return false
			}
		}
		return true
	}
	return false
}
