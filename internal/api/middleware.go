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

// CORSConfig contains CORS configuration
type CORSConfig struct {
	AllowedOrigins []string // Empty or ["*"] means allow all
	AllowedMethods []string
	AllowedHeaders []string
}

// DefaultCORSConfig returns a restrictive default CORS configuration
func DefaultCORSConfig() CORSConfig {
	return CORSConfig{
		AllowedOrigins: []string{}, // Empty = no CORS (same-origin only)
		AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders: []string{"Content-Type", "Authorization", "X-Request-ID"},
	}
}

// CORSMiddleware adds CORS headers based on configuration
func CORSMiddleware(config CORSConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")

			// Determine allowed origin
			allowedOrigin := ""
			if len(config.AllowedOrigins) == 0 {
				// No CORS configured - don't set any CORS headers (same-origin only)
			} else if len(config.AllowedOrigins) == 1 && config.AllowedOrigins[0] == "*" {
				// Allow all origins
				allowedOrigin = "*"
			} else {
				// Check if origin is in allowed list
				for _, allowed := range config.AllowedOrigins {
					if allowed == origin {
						allowedOrigin = origin
						break
					}
				}
			}

			// Set CORS headers only if origin is allowed
			if allowedOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				if allowedOrigin != "*" {
					w.Header().Set("Vary", "Origin")
				}

				methods := "GET, POST, PUT, DELETE, OPTIONS"
				if len(config.AllowedMethods) > 0 {
					methods = ""
					for i, m := range config.AllowedMethods {
						if i > 0 {
							methods += ", "
						}
						methods += m
					}
				}
				w.Header().Set("Access-Control-Allow-Methods", methods)

				headers := "Content-Type, Authorization, X-Request-ID"
				if len(config.AllowedHeaders) > 0 {
					headers = ""
					for i, h := range config.AllowedHeaders {
						if i > 0 {
							headers += ", "
						}
						headers += h
					}
				}
				w.Header().Set("Access-Control-Allow-Headers", headers)
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			// Handle preflight requests
			if r.Method == "OPTIONS" {
				if allowedOrigin != "" {
					w.WriteHeader(http.StatusOK)
				} else {
					w.WriteHeader(http.StatusForbidden)
				}
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// AuthConfig contains authentication configuration
type AuthConfig struct {
	Enabled bool
	Token   string
}

// AuthMiddleware enforces token-based authentication for mutating requests
func AuthMiddleware(config AuthConfig, logger *logging.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth if disabled
			if !config.Enabled {
				next.ServeHTTP(w, r)
				return
			}

			// Allow read-only methods without auth
			if r.Method == "GET" || r.Method == "HEAD" || r.Method == "OPTIONS" {
				next.ServeHTTP(w, r)
				return
			}

			// Check Authorization header
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				logger.Warn("Missing authorization header", map[string]interface{}{
					"path":      r.URL.Path,
					"method":    r.Method,
					"requestID": GetRequestID(r.Context()),
				})
				http.Error(w, `{"error":"unauthorized","message":"missing Authorization header"}`, http.StatusUnauthorized)
				return
			}

			// Expect "Bearer <token>" format
			const bearerPrefix = "Bearer "
			if len(authHeader) < len(bearerPrefix) || authHeader[:len(bearerPrefix)] != bearerPrefix {
				logger.Warn("Invalid authorization format", map[string]interface{}{
					"path":      r.URL.Path,
					"method":    r.Method,
					"requestID": GetRequestID(r.Context()),
				})
				http.Error(w, `{"error":"unauthorized","message":"invalid Authorization format, expected 'Bearer <token>'"}`, http.StatusUnauthorized)
				return
			}

			token := authHeader[len(bearerPrefix):]
			if token != config.Token {
				logger.Warn("Invalid auth token", map[string]interface{}{
					"path":      r.URL.Path,
					"method":    r.Method,
					"requestID": GetRequestID(r.Context()),
				})
				http.Error(w, `{"error":"forbidden","message":"invalid token"}`, http.StatusForbidden)
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
