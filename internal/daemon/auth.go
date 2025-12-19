package daemon

import (
	"net/http"
	"os"
	"strings"
)

const (
	// AuthHeader is the header name for bearer token authentication
	AuthHeader = "Authorization"

	// AuthScheme is the authentication scheme prefix
	AuthScheme = "Bearer "

	// DaemonTokenEnvVar is the environment variable for the daemon token
	DaemonTokenEnvVar = "CKB_DAEMON_TOKEN"
)

// withAuth wraps a handler with authentication middleware
func (d *Daemon) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth if disabled
		if !d.config.Auth.Enabled {
			next.ServeHTTP(w, r)
			return
		}

		// Get the expected token
		expectedToken := d.getAuthToken()
		if expectedToken == "" {
			// No token configured, allow all requests but log warning
			d.logger.Println("WARNING: Auth enabled but no token configured")
			next.ServeHTTP(w, r)
			return
		}

		// Get the provided token
		authHeader := r.Header.Get(AuthHeader)
		if authHeader == "" {
			d.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Missing Authorization header")
			return
		}

		if !strings.HasPrefix(authHeader, AuthScheme) {
			d.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid Authorization scheme, expected Bearer")
			return
		}

		providedToken := strings.TrimPrefix(authHeader, AuthScheme)
		if providedToken != expectedToken {
			d.writeError(w, http.StatusUnauthorized, "UNAUTHORIZED", "Invalid token")
			return
		}

		// Token valid, proceed
		next.ServeHTTP(w, r)
	})
}

// getAuthToken returns the authentication token from config or environment
func (d *Daemon) getAuthToken() string {
	// Check config token first
	if d.config.Auth.Token != "" {
		// Expand environment variable if it starts with $
		token := d.config.Auth.Token
		if strings.HasPrefix(token, "${") && strings.HasSuffix(token, "}") {
			envVar := token[2 : len(token)-1]
			return os.Getenv(envVar)
		}
		return token
	}

	// Check token file
	if d.config.Auth.TokenFile != "" {
		path := d.config.Auth.TokenFile
		// Expand ~ to home directory
		if strings.HasPrefix(path, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				path = home + path[1:]
			}
		}

		data, err := os.ReadFile(path)
		if err == nil {
			return strings.TrimSpace(string(data))
		}
		d.logger.Printf("Failed to read token file %s: %v", path, err)
	}

	// Fall back to environment variable
	return os.Getenv(DaemonTokenEnvVar)
}

// GenerateToken generates a random token for daemon authentication
// This is a utility function for the CLI to generate tokens
func GenerateToken() string {
	// Use crypto/rand to generate a secure random token
	b := make([]byte, 32)
	if _, err := randomRead(b); err != nil {
		// Fall back to less secure but still usable token
		return generateFallbackToken()
	}
	return encodeHex(b)
}

// randomRead reads random bytes
func randomRead(b []byte) (int, error) {
	f, err := os.Open("/dev/urandom")
	if err != nil {
		return 0, err
	}
	defer func() { _ = f.Close() }()
	return f.Read(b)
}

// encodeHex encodes bytes as hex
func encodeHex(b []byte) string {
	const hex = "0123456789abcdef"
	result := make([]byte, len(b)*2)
	for i, v := range b {
		result[i*2] = hex[v>>4]
		result[i*2+1] = hex[v&0x0f]
	}
	return string(result)
}

// generateFallbackToken generates a token using time-based seed
func generateFallbackToken() string {
	// This is less secure but works as a fallback
	seed := int64(os.Getpid()) * 1000000
	const chars = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	result := make([]byte, 64)
	for i := range result {
		seed = seed*1103515245 + 12345
		result[i] = chars[int(seed>>16)%len(chars)]
	}
	return string(result)
}
