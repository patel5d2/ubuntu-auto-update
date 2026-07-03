package middleware

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"time"

	log "github.com/sirupsen/logrus"
)

type ErrorResponse struct {
	Error      string                 `json:"error"`
	Message    string                 `json:"message"`
	StatusCode int                    `json:"status_code"`
	RequestID  string                 `json:"request_id,omitempty"`
	Details    map[string]interface{} `json:"details,omitempty"`
	Timestamp  string                 `json:"timestamp"`
}

// ErrorHandler middleware for centralized error handling
func ErrorHandler(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Log the panic with stack trace
				log.WithFields(log.Fields{
					"panic":  err,
					"stack":  string(debug.Stack()),
					"method": r.Method,
					"path":   r.URL.Path,
					"remote": r.RemoteAddr,
				}).Error("HTTP handler panic recovered")

				// Return internal server error
				SendErrorResponse(w, http.StatusInternalServerError, "Internal server error", "A server error occurred", nil)
			}
		}()

		// Create a custom ResponseWriter to capture status codes
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)

		// Log request details for monitoring
		log.WithFields(log.Fields{
			"method":      r.Method,
			"path":        r.URL.Path,
			"status_code": rw.statusCode,
			"remote":      r.RemoteAddr,
			"user_agent":  r.UserAgent(),
		}).Info("HTTP request completed")
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// Hijack forwards to the underlying writer so WebSocket upgrades work
// through this middleware. Embedding only forwards http.ResponseWriter's
// method set — without this, gorilla/websocket fails with "response does
// not implement http.Hijacker".
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter does not support hijacking")
	}
	return h.Hijack()
}

// SendErrorResponse sends a standardized error response
func SendErrorResponse(w http.ResponseWriter, statusCode int, error string, message string, details map[string]interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResp := ErrorResponse{
		Error:      error,
		Message:    message,
		StatusCode: statusCode,
		Details:    details,
		Timestamp:  getCurrentTimestamp(),
	}

	if err := json.NewEncoder(w).Encode(errorResp); err != nil {
		log.WithError(err).Error("Failed to encode error response")
	}
}

// SendValidationError sends a validation error response
func SendValidationError(w http.ResponseWriter, field string, message string) {
	details := map[string]interface{}{
		"field": field,
	}
	SendErrorResponse(w, http.StatusBadRequest, "validation_error", message, details)
}

// SendAuthError sends an authentication error response
func SendAuthError(w http.ResponseWriter, message string) {
	SendErrorResponse(w, http.StatusUnauthorized, "authentication_error", message, nil)
}

// SendForbiddenError sends a forbidden error response
func SendForbiddenError(w http.ResponseWriter, message string) {
	SendErrorResponse(w, http.StatusForbidden, "forbidden", message, nil)
}

// SendNotFoundError sends a not found error response
func SendNotFoundError(w http.ResponseWriter, resource string) {
	message := "Resource not found"
	if resource != "" {
		message = resource + " not found"
	}
	SendErrorResponse(w, http.StatusNotFound, "not_found", message, nil)
}

func getCurrentTimestamp() string {
	return time.Now().UTC().Format(time.RFC3339)
}
