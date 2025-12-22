package api

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// IndexServerConfig configures remote index serving for federation
type IndexServerConfig struct {
	Enabled        bool                `toml:"enabled"`
	Repos          []IndexRepoConfig   `toml:"repos"`
	DefaultPrivacy IndexPrivacyConfig  `toml:"default_privacy"`
	MaxPageSize    int                 `toml:"max_page_size"`    // Default 10000
	CursorSecret   string              `toml:"cursor_secret"`    // HMAC key for cursors
}

// IndexRepoConfig configures a single repository for index serving
type IndexRepoConfig struct {
	ID          string              `toml:"id"`          // "company/core-lib"
	Name        string              `toml:"name"`        // Display name
	Path        string              `toml:"path"`        // Path to repo with .ckb/
	Description string              `toml:"description"` // Optional description
	Privacy     *IndexPrivacyConfig `toml:"privacy"`     // Per-repo override (nil = use default)
}

// IndexPrivacyConfig controls field redaction in API responses
type IndexPrivacyConfig struct {
	ExposePaths      bool   `toml:"expose_paths"`       // Default true - expose full file paths
	ExposeDocs       bool   `toml:"expose_docs"`        // Default true - expose documentation strings
	ExposeSignatures bool   `toml:"expose_signatures"`  // Default true - expose function signatures
	PathPrefixStrip  string `toml:"path_prefix_strip"`  // Remove this prefix from paths
}

// DefaultIndexServerConfig returns default configuration for index server
func DefaultIndexServerConfig() *IndexServerConfig {
	return &IndexServerConfig{
		Enabled:     false,
		Repos:       []IndexRepoConfig{},
		MaxPageSize: 10000,
		DefaultPrivacy: IndexPrivacyConfig{
			ExposePaths:      true,
			ExposeDocs:       true,
			ExposeSignatures: true,
		},
		CursorSecret: generateDefaultSecret(),
	}
}

// DefaultIndexPrivacyConfig returns default privacy settings (all exposed)
func DefaultIndexPrivacyConfig() IndexPrivacyConfig {
	return IndexPrivacyConfig{
		ExposePaths:      true,
		ExposeDocs:       true,
		ExposeSignatures: true,
	}
}

// generateDefaultSecret generates a random secret for cursor signing
func generateDefaultSecret() string {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a less secure but working default
		return "ckb-default-cursor-secret-change-me"
	}
	return hex.EncodeToString(bytes)
}

// LoadIndexServerConfig loads configuration from a TOML file
func LoadIndexServerConfig(path string) (*IndexServerConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	// Start with defaults
	config := DefaultIndexServerConfig()

	// Parse TOML
	if _, err := toml.Decode(string(data), config); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	// Validate configuration
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return config, nil
}

// Validate checks that the configuration is valid
func (c *IndexServerConfig) Validate() error {
	if !c.Enabled {
		return nil // Nothing to validate if disabled
	}

	if len(c.Repos) == 0 {
		return fmt.Errorf("at least one repo must be configured when index server is enabled")
	}

	// Check for duplicate repo IDs
	seen := make(map[string]bool)
	for i, repo := range c.Repos {
		if repo.ID == "" {
			return fmt.Errorf("repo[%d]: id is required", i)
		}
		if repo.Path == "" {
			return fmt.Errorf("repo[%d] (%s): path is required", i, repo.ID)
		}
		if seen[repo.ID] {
			return fmt.Errorf("duplicate repo id: %s", repo.ID)
		}
		seen[repo.ID] = true

		// Check if path exists
		if _, err := os.Stat(repo.Path); os.IsNotExist(err) {
			return fmt.Errorf("repo[%d] (%s): path does not exist: %s", i, repo.ID, repo.Path)
		}
	}

	if c.MaxPageSize <= 0 {
		return fmt.Errorf("max_page_size must be positive")
	}

	if c.MaxPageSize > 100000 {
		return fmt.Errorf("max_page_size cannot exceed 100000")
	}

	return nil
}

// GetRepoPrivacy returns the effective privacy config for a repo
// (per-repo override or default)
func (c *IndexServerConfig) GetRepoPrivacy(repoID string) IndexPrivacyConfig {
	for _, repo := range c.Repos {
		if repo.ID == repoID {
			if repo.Privacy != nil {
				return *repo.Privacy
			}
			break
		}
	}
	return c.DefaultPrivacy
}

// GetRepoConfig returns the configuration for a specific repo, or nil if not found
func (c *IndexServerConfig) GetRepoConfig(repoID string) *IndexRepoConfig {
	for i := range c.Repos {
		if c.Repos[i].ID == repoID {
			return &c.Repos[i]
		}
	}
	return nil
}
