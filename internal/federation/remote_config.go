// Package federation provides cross-repository federation capabilities for CKB.
// This file manages remote index server configuration (Phase 5).
package federation

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// RemoteServer represents a remote CKB index server configuration.
type RemoteServer struct {
	// Name is the unique identifier for this server
	Name string `toml:"name"`

	// URL is the base URL of the index server (e.g., https://ckb.example.com)
	URL string `toml:"url"`

	// Token is the authentication token. Supports ${ENV_VAR} expansion.
	Token string `toml:"token,omitempty"`

	// CacheTTL is how long to cache responses from this server
	CacheTTL Duration `toml:"cache_ttl,omitempty"`

	// Timeout is the request timeout for this server
	Timeout Duration `toml:"timeout,omitempty"`

	// Enabled controls whether this server is included in queries
	Enabled bool `toml:"enabled"`

	// AddedAt is when this server was added
	AddedAt time.Time `toml:"added_at"`

	// LastSyncedAt is when we last synced metadata from this server
	LastSyncedAt *time.Time `toml:"last_synced_at,omitempty"`

	// LastError is the last error encountered when connecting to this server
	LastError string `toml:"last_error,omitempty"`
}

// Duration is a wrapper around time.Duration for TOML serialization.
type Duration struct {
	time.Duration
}

// UnmarshalText implements encoding.TextUnmarshaler for Duration.
func (d *Duration) UnmarshalText(text []byte) error {
	var err error
	d.Duration, err = time.ParseDuration(string(text))
	return err
}

// MarshalText implements encoding.TextMarshaler for Duration.
func (d Duration) MarshalText() ([]byte, error) {
	return []byte(d.Duration.String()), nil
}

// Default values for remote server configuration
const (
	DefaultRemoteCacheTTL = time.Hour
	DefaultRemoteTimeout  = 30 * time.Second
	DefaultMaxRetries     = 3
	DefaultRetryBaseDelay = 500 * time.Millisecond
	DefaultMaxBodySize    = 10 * 1024 * 1024 // 10MB
)

// envVarPattern matches ${ENV_VAR} patterns for expansion.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// ExpandEnvVars expands ${ENV_VAR} patterns in a string.
func ExpandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name from ${VAR_NAME}
		varName := match[2 : len(match)-1]
		if value, ok := os.LookupEnv(varName); ok {
			return value
		}
		// If not found, return empty string (don't leak the variable name)
		return ""
	})
}

// GetToken returns the expanded token value.
func (rs *RemoteServer) GetToken() string {
	return ExpandEnvVars(rs.Token)
}

// GetCacheTTL returns the cache TTL, using default if not set.
func (rs *RemoteServer) GetCacheTTL() time.Duration {
	if rs.CacheTTL.Duration > 0 {
		return rs.CacheTTL.Duration
	}
	return DefaultRemoteCacheTTL
}

// GetTimeout returns the timeout, using default if not set.
func (rs *RemoteServer) GetTimeout() time.Duration {
	if rs.Timeout.Duration > 0 {
		return rs.Timeout.Duration
	}
	return DefaultRemoteTimeout
}

// Validate checks if the remote server configuration is valid.
func (rs *RemoteServer) Validate() error {
	if rs.Name == "" {
		return fmt.Errorf("server name is required")
	}
	if rs.URL == "" {
		return fmt.Errorf("server URL is required")
	}
	// URL must start with http:// or https://
	if !strings.HasPrefix(rs.URL, "http://") && !strings.HasPrefix(rs.URL, "https://") {
		return fmt.Errorf("server URL must start with http:// or https://")
	}
	// Remove trailing slash from URL
	rs.URL = strings.TrimSuffix(rs.URL, "/")
	return nil
}

// RemoteServerUpdate contains fields that can be updated on a remote server.
type RemoteServerUpdate struct {
	URL      *string
	Token    *string
	CacheTTL *time.Duration
	Timeout  *time.Duration
	Enabled  *bool
}

// AddRemoteServer adds a remote index server to the federation.
func (f *Federation) AddRemoteServer(server RemoteServer) error {
	if err := server.Validate(); err != nil {
		return fmt.Errorf("invalid server configuration: %w", err)
	}

	// Check for duplicate name
	for _, s := range f.config.RemoteServers {
		if s.Name == server.Name {
			return fmt.Errorf("remote server %q already exists", server.Name)
		}
	}

	// Set defaults
	if server.AddedAt.IsZero() {
		server.AddedAt = time.Now().UTC()
	}
	if server.CacheTTL.Duration == 0 {
		server.CacheTTL.Duration = DefaultRemoteCacheTTL
	}
	if server.Timeout.Duration == 0 {
		server.Timeout.Duration = DefaultRemoteTimeout
	}

	f.config.RemoteServers = append(f.config.RemoteServers, server)
	f.config.UpdatedAt = time.Now().UTC()

	if err := f.config.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if f.logger != nil {
		f.logger.Info("Added remote server to federation", map[string]interface{}{
			"federation": f.config.Name,
			"server":     server.Name,
			"url":        server.URL,
		})
	}

	return nil
}

// RemoveRemoteServer removes a remote index server from the federation.
func (f *Federation) RemoveRemoteServer(name string) error {
	found := false
	for i, s := range f.config.RemoteServers {
		if s.Name == name {
			f.config.RemoteServers = append(
				f.config.RemoteServers[:i],
				f.config.RemoteServers[i+1:]...,
			)
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("remote server %q not found", name)
	}

	f.config.UpdatedAt = time.Now().UTC()

	if err := f.config.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	// Clear cached data for this server
	if err := f.index.ClearRemoteCache(name); err != nil {
		if f.logger != nil {
			f.logger.Warn("Failed to clear remote cache", map[string]interface{}{
				"server": name,
				"error":  err.Error(),
			})
		}
	}

	if f.logger != nil {
		f.logger.Info("Removed remote server from federation", map[string]interface{}{
			"federation": f.config.Name,
			"server":     name,
		})
	}

	return nil
}

// ListRemoteServers returns all configured remote servers.
func (f *Federation) ListRemoteServers() []RemoteServer {
	return f.config.RemoteServers
}

// GetRemoteServer returns a remote server by name.
func (f *Federation) GetRemoteServer(name string) *RemoteServer {
	for i := range f.config.RemoteServers {
		if f.config.RemoteServers[i].Name == name {
			return &f.config.RemoteServers[i]
		}
	}
	return nil
}

// UpdateRemoteServer updates a remote server's configuration.
func (f *Federation) UpdateRemoteServer(name string, updates RemoteServerUpdate) error {
	var server *RemoteServer
	for i := range f.config.RemoteServers {
		if f.config.RemoteServers[i].Name == name {
			server = &f.config.RemoteServers[i]
			break
		}
	}

	if server == nil {
		return fmt.Errorf("remote server %q not found", name)
	}

	if updates.URL != nil {
		server.URL = *updates.URL
	}
	if updates.Token != nil {
		server.Token = *updates.Token
	}
	if updates.CacheTTL != nil {
		server.CacheTTL.Duration = *updates.CacheTTL
	}
	if updates.Timeout != nil {
		server.Timeout.Duration = *updates.Timeout
	}
	if updates.Enabled != nil {
		server.Enabled = *updates.Enabled
	}

	if err := server.Validate(); err != nil {
		return fmt.Errorf("invalid server configuration: %w", err)
	}

	f.config.UpdatedAt = time.Now().UTC()

	if err := f.config.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if f.logger != nil {
		f.logger.Info("Updated remote server in federation", map[string]interface{}{
			"federation": f.config.Name,
			"server":     name,
		})
	}

	return nil
}

// SetRemoteServerError records an error for a remote server.
func (f *Federation) SetRemoteServerError(name, errorMsg string) error {
	for i := range f.config.RemoteServers {
		if f.config.RemoteServers[i].Name == name {
			f.config.RemoteServers[i].LastError = errorMsg
			return f.config.Save()
		}
	}
	return fmt.Errorf("remote server %q not found", name)
}

// SetRemoteServerSynced records that a remote server was successfully synced.
func (f *Federation) SetRemoteServerSynced(name string) error {
	for i := range f.config.RemoteServers {
		if f.config.RemoteServers[i].Name == name {
			now := time.Now().UTC()
			f.config.RemoteServers[i].LastSyncedAt = &now
			f.config.RemoteServers[i].LastError = "" // Clear any previous error
			return f.config.Save()
		}
	}
	return fmt.Errorf("remote server %q not found", name)
}

// GetEnabledRemoteServers returns only enabled remote servers.
func (f *Federation) GetEnabledRemoteServers() []RemoteServer {
	var enabled []RemoteServer
	for _, s := range f.config.RemoteServers {
		if s.Enabled {
			enabled = append(enabled, s)
		}
	}
	return enabled
}
