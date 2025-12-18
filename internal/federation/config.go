// Package federation provides cross-repository federation capabilities for CKB v6.2.
// It enables unified visibility across multiple repositories through federated queries.
package federation

import (
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/google/uuid"

	"ckb/internal/paths"
)

// Config represents a federation configuration stored in config.toml
type Config struct {
	// Name is the federation identifier
	Name string `toml:"name"`

	// Description is an optional human-readable description
	Description string `toml:"description,omitempty"`

	// CreatedAt is when the federation was created
	CreatedAt time.Time `toml:"created_at"`

	// UpdatedAt is when the federation was last modified
	UpdatedAt time.Time `toml:"updated_at"`

	// Repos is the list of repositories in this federation
	Repos []RepoConfig `toml:"repos"`
}

// RepoConfig represents a repository entry in the federation config
type RepoConfig struct {
	// RepoUID is the immutable UUID for this repository (never changes)
	RepoUID string `toml:"repo_uid"`

	// RepoID is the mutable human-friendly alias
	RepoID string `toml:"repo_id"`

	// Path is the absolute filesystem path to the repository
	Path string `toml:"path"`

	// Tags are optional labels for categorization
	Tags []string `toml:"tags,omitempty"`

	// AddedAt is when the repo was added to the federation
	AddedAt time.Time `toml:"added_at"`
}

// NewConfig creates a new federation configuration
func NewConfig(name, description string) *Config {
	now := time.Now().UTC()
	return &Config{
		Name:        name,
		Description: description,
		CreatedAt:   now,
		UpdatedAt:   now,
		Repos:       []RepoConfig{},
	}
}

// AddRepo adds a repository to the federation
func (c *Config) AddRepo(repoID, path string, tags []string) (*RepoConfig, error) {
	// Check for duplicate repoID
	for _, r := range c.Repos {
		if r.RepoID == repoID {
			return nil, fmt.Errorf("repository with ID %q already exists", repoID)
		}
		if r.Path == path {
			return nil, fmt.Errorf("repository at path %q already exists (as %q)", path, r.RepoID)
		}
	}

	repo := RepoConfig{
		RepoUID: uuid.New().String(),
		RepoID:  repoID,
		Path:    path,
		Tags:    tags,
		AddedAt: time.Now().UTC(),
	}

	c.Repos = append(c.Repos, repo)
	c.UpdatedAt = time.Now().UTC()

	return &repo, nil
}

// RemoveRepo removes a repository from the federation by repoID
func (c *Config) RemoveRepo(repoID string) error {
	for i, r := range c.Repos {
		if r.RepoID == repoID {
			c.Repos = append(c.Repos[:i], c.Repos[i+1:]...)
			c.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return fmt.Errorf("repository %q not found", repoID)
}

// RenameRepo changes the repoID (alias) of a repository
// The repoUID remains unchanged
func (c *Config) RenameRepo(oldID, newID string) error {
	// Check that newID doesn't already exist
	for _, r := range c.Repos {
		if r.RepoID == newID {
			return fmt.Errorf("repository with ID %q already exists", newID)
		}
	}

	// Find and rename
	for i, r := range c.Repos {
		if r.RepoID == oldID {
			c.Repos[i].RepoID = newID
			c.UpdatedAt = time.Now().UTC()
			return nil
		}
	}
	return fmt.Errorf("repository %q not found", oldID)
}

// GetRepo returns a repository by repoID
func (c *Config) GetRepo(repoID string) *RepoConfig {
	for i := range c.Repos {
		if c.Repos[i].RepoID == repoID {
			return &c.Repos[i]
		}
	}
	return nil
}

// GetRepoByUID returns a repository by repoUID
func (c *Config) GetRepoByUID(repoUID string) *RepoConfig {
	for i := range c.Repos {
		if c.Repos[i].RepoUID == repoUID {
			return &c.Repos[i]
		}
	}
	return nil
}

// LoadConfig loads a federation configuration from disk
func LoadConfig(name string) (*Config, error) {
	configPath, err := paths.GetFederationConfigPath(name)
	if err != nil {
		return nil, fmt.Errorf("failed to get config path: %w", err)
	}

	var config Config
	if _, err := toml.DecodeFile(configPath, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

// SaveConfig saves the federation configuration to disk
func (c *Config) Save() error {
	// Ensure the federation directory exists
	if _, err := paths.EnsureFederationDir(c.Name); err != nil {
		return fmt.Errorf("failed to create federation directory: %w", err)
	}

	configPath, err := paths.GetFederationConfigPath(c.Name)
	if err != nil {
		return fmt.Errorf("failed to get config path: %w", err)
	}

	f, err := os.Create(configPath)
	if err != nil {
		return fmt.Errorf("failed to create config file: %w", err)
	}
	defer f.Close()

	encoder := toml.NewEncoder(f)
	if err := encoder.Encode(c); err != nil {
		return fmt.Errorf("failed to encode config: %w", err)
	}

	return nil
}

// Delete removes the federation configuration and all associated data
func (c *Config) Delete() error {
	return paths.DeleteFederationDir(c.Name)
}
