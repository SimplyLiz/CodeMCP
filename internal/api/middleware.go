package api

import (
	"context"
	"fmt"
	"net/http"
	"runtime/debug"
	"time"

	"ckb/internal/logging"

	"github.com/google/uuid"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const (
	requestIDKey contextKey = "requestID"
)

// LoggingMiddleware logs HTTP requests and responses
func LoggingMiddleware(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Wrap response writer to capture status code
			wrapped := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

			// Get request ID from context
			reqID := GetRequestID(r.Context())

			// Log request
			logger.Info("HTTP request", map[string]interface{}{
				"method":     r.Method,
				"path":       r.URL.Path,
				"query":      r.URL.RawQuery,
				"remoteAddr": r.RemoteAddr,
				"requestID":  reqID,
			})

			// Call next handler
			next.ServeHTTP(wrapped, r)

			// Log response
			duration := time.Since(start)
			logger.Info("HTTP response", map[string]interface{}{
				"method":     r.Method,
				"path":       r.URL.Path,
				"status":     wrapped.statusCode,
				"duration":   duration.String(),
				"durationMs": duration.Milliseconds(),
				"requestID":  reqID,
			})
		})
	}
}

// RecoveryMiddleware recovers from panics and logs them
func RecoveryMiddleware(logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					reqID := GetRequestID(r.Context())
					logger.Error("Panic recovered", map[string]interface{}{
						"error":     fmt.Sprintf("%v", err),
						"stack":     string(debug.Stack()),
						"requestID": reqID,
					})

					// Return 500 error
					InternalError(w, "Internal server error", fmt.Errorf("%v", err))
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// CORSMiddleware adds CORS headers for local development
func CORSMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set CORS headers
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
			w.Header().Set("Access-Control-Max-Age", "86400")

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// RequestIDMiddleware adds a unique request ID to each request
func RequestIDMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if request ID already exists in header
			reqID := r.Header.Get("X-Request-ID")
			if reqID == "" {
				// Generate new request ID
				reqID = uuid.New().String()
			}

			// Add request ID to context
			ctx := context.WithValue(r.Context(), requestIDKey, reqID)
			r = r.WithContext(ctx)

			// Add request ID to response header
			w.Header().Set("X-Request-ID", reqID)

			next.ServeHTTP(w, r)
		})
	}
}

// GetRequestID retrieves the request ID from context
func GetRequestID(ctx context.Context) string {
	if reqID, ok := ctx.Value(requestIDKey).(string); ok {
		return reqID
	}
	return ""
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code before writing it
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write ensures status code is set if WriteHeader wasn't called
func (rw *responseWriter) Write(data []byte) (int, error) {
	if rw.statusCode == 0 {
		rw.statusCode = http.StatusOK
	}
	return rw.ResponseWriter.Write(data)
}
