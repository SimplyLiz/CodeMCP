package a2a

import (
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/google/uuid"
)

// applyMiddleware wraps the router with middleware in the correct order.
func (s *Server) applyMiddleware(handler http.Handler) http.Handler {
	handler = recoveryMiddleware(s.logger)(handler)
	handler = loggingMiddleware(s.logger)(handler)
	handler = a2aAuthMiddleware(s.config.AuthToken, s.logger)(handler)
	handler = a2aVersionMiddleware()(handler)
	handler = requestIDMiddleware()(handler)
	handler = corsMiddleware(s.config.CORSAllow)(handler)
	return handler
}

// a2aVersionMiddleware validates the A2A-Version header.
func a2aVersionMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ver := r.Header.Get("A2A-Version")
			if ver != "" && ver != ProtocolVersion {
				// Check major version compatibility
				parts := strings.SplitN(ver, ".", 2)
				protoParts := strings.SplitN(ProtocolVersion, ".", 2)
				if len(parts) > 0 && len(protoParts) > 0 && parts[0] != protoParts[0] {
					writeA2AError(w, NewVersionNotSupportedError(ver))
					return
				}
			}
			// Set response header
			w.Header().Set("A2A-Version", ProtocolVersion)
			next.ServeHTTP(w, r)
		})
	}
}

// a2aAuthMiddleware validates bearer token auth.
func a2aAuthMiddleware(token string, logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// No auth configured — allow all
			if token == "" {
				next.ServeHTTP(w, r)
				return
			}

			// Agent card is always public
			if r.URL.Path == WellKnownPath || r.URL.Path == "/health" {
				next.ServeHTTP(w, r)
				return
			}

			// GET requests on tasks are read-only, allow without auth
			if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/tasks") {
				next.ServeHTTP(w, r)
				return
			}

			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				logger.Warn("Missing authorization header", "path", r.URL.Path, "method", r.Method)
				http.Error(w, `{"error":"unauthorized"}`, http.StatusUnauthorized)
				return
			}

			const bearerPrefix = "Bearer "
			if len(authHeader) < len(bearerPrefix) || !strings.EqualFold(authHeader[:len(bearerPrefix)], bearerPrefix) {
				http.Error(w, `{"error":"invalid authorization format"}`, http.StatusUnauthorized)
				return
			}

			if authHeader[len(bearerPrefix):] != token {
				logger.Warn("Invalid auth token", "path", r.URL.Path)
				http.Error(w, `{"error":"invalid token"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// corsMiddleware handles CORS headers.
func corsMiddleware(allowedOrigins []string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if len(allowedOrigins) == 0 {
				next.ServeHTTP(w, r)
				return
			}

			origin := r.Header.Get("Origin")
			allowed := ""
			for _, o := range allowedOrigins {
				if o == "*" || o == origin {
					allowed = o
					break
				}
			}

			if allowed != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowed)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, A2A-Version, A2A-Extensions, X-Request-ID")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == "OPTIONS" {
				if allowed != "" {
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

// requestIDMiddleware adds a request ID to each request.
func requestIDMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			reqID := r.Header.Get("X-Request-ID")
			if reqID == "" {
				reqID = uuid.New().String()
			}
			w.Header().Set("X-Request-ID", reqID)
			next.ServeHTTP(w, r)
		})
	}
}

// loggingMiddleware logs HTTP requests.
func loggingMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			wrapped := &statusWriter{ResponseWriter: w, status: http.StatusOK}

			next.ServeHTTP(wrapped, r)

			logger.Info("A2A request",
				"method", r.Method,
				"path", r.URL.Path,
				"status", wrapped.status,
				"duration", time.Since(start).String(),
				"remoteAddr", r.RemoteAddr,
			)
		})
	}
}

// recoveryMiddleware catches panics.
func recoveryMiddleware(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if err := recover(); err != nil {
					logger.Error("Panic recovered",
						"error", fmt.Sprintf("%v", err),
						"stack", string(debug.Stack()),
						"path", r.URL.Path,
					)
					writeA2AError(w, NewInternalError("internal server error"))
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}

// statusWriter wraps http.ResponseWriter to capture the status code.
type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}
