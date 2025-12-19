package federation

import (
	"fmt"

	"ckb/internal/logging"
	"ckb/internal/paths"
)

// Federation represents a federation of repositories
type Federation struct {
	config *Config
	index  *Index
	logger *logging.Logger
}

// Open opens an existing federation
func Open(name string, logger *logging.Logger) (*Federation, error) {
	// Check if federation exists
	exists, err := paths.FederationExists(name)
	if err != nil {
		return nil, fmt.Errorf("failed to check federation existence: %w", err)
	}
	if !exists {
		return nil, fmt.Errorf("federation %q does not exist", name)
	}

	// Load config
	config, err := LoadConfig(name)
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Open index
	index, err := OpenIndex(name)
	if err != nil {
		return nil, fmt.Errorf("failed to open index: %w", err)
	}

	return &Federation{
		config: config,
		index:  index,
		logger: logger,
	}, nil
}

// Create creates a new federation
func Create(name, description string, logger *logging.Logger) (*Federation, error) {
	// Check if federation already exists
	exists, err := paths.FederationExists(name)
	if err != nil {
		return nil, fmt.Errorf("failed to check federation existence: %w", err)
	}
	if exists {
		return nil, fmt.Errorf("federation %q already exists", name)
	}

	// Create config
	config := NewConfig(name, description)
	if saveErr := config.Save(); saveErr != nil {
		return nil, fmt.Errorf("failed to save config: %w", saveErr)
	}

	// Create index
	index, err := OpenIndex(name)
	if err != nil {
		// Clean up config on failure
		_ = config.Delete()
		return nil, fmt.Errorf("failed to create index: %w", err)
	}

	if logger != nil {
		logger.Info("Created federation", map[string]interface{}{
			"name": name,
		})
	}

	return &Federation{
		config: config,
		index:  index,
		logger: logger,
	}, nil
}

// Close closes the federation
func (f *Federation) Close() error {
	return f.index.Close()
}

// Name returns the federation name
func (f *Federation) Name() string {
	return f.config.Name
}

// Description returns the federation description
func (f *Federation) Description() string {
	return f.config.Description
}

// Config returns the federation configuration
func (f *Federation) Config() *Config {
	return f.config
}

// Index returns the federation index
func (f *Federation) Index() *Index {
	return f.index
}

// AddRepo adds a repository to the federation
func (f *Federation) AddRepo(repoID, path string, tags []string) (*RepoConfig, error) {
	// Add to config
	repo, err := f.config.AddRepo(repoID, path, tags)
	if err != nil {
		return nil, err
	}

	// Save config
	if err := f.config.Save(); err != nil {
		return nil, fmt.Errorf("failed to save config: %w", err)
	}

	if f.logger != nil {
		f.logger.Info("Added repository to federation", map[string]interface{}{
			"federation": f.config.Name,
			"repoId":     repoID,
			"path":       path,
		})
	}

	return repo, nil
}

// RemoveRepo removes a repository from the federation
func (f *Federation) RemoveRepo(repoID string) error {
	// Get the repo to find its UID
	repo := f.config.GetRepo(repoID)
	if repo == nil {
		return fmt.Errorf("repository %q not found", repoID)
	}

	// Remove from index
	if err := f.index.DeleteRepo(repo.RepoUID); err != nil {
		return fmt.Errorf("failed to remove from index: %w", err)
	}

	// Remove from config
	if err := f.config.RemoveRepo(repoID); err != nil {
		return err
	}

	// Save config
	if err := f.config.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if f.logger != nil {
		f.logger.Info("Removed repository from federation", map[string]interface{}{
			"federation": f.config.Name,
			"repoId":     repoID,
		})
	}

	return nil
}

// RenameRepo renames a repository in the federation
func (f *Federation) RenameRepo(oldID, newID string) error {
	if err := f.config.RenameRepo(oldID, newID); err != nil {
		return err
	}

	// Save config
	if err := f.config.Save(); err != nil {
		return fmt.Errorf("failed to save config: %w", err)
	}

	if f.logger != nil {
		f.logger.Info("Renamed repository in federation", map[string]interface{}{
			"federation": f.config.Name,
			"oldId":      oldID,
			"newId":      newID,
		})
	}

	return nil
}

// ListRepos returns all repositories in the federation
func (f *Federation) ListRepos() []RepoConfig {
	return f.config.Repos
}

// GetRepo returns a repository by repoID
func (f *Federation) GetRepo(repoID string) *RepoConfig {
	return f.config.GetRepo(repoID)
}

// Delete deletes the federation
func (f *Federation) Delete() error {
	// Close the index first
	if err := f.index.Close(); err != nil {
		return fmt.Errorf("failed to close index: %w", err)
	}

	// Delete the federation directory
	if err := f.config.Delete(); err != nil {
		return fmt.Errorf("failed to delete federation: %w", err)
	}

	if f.logger != nil {
		f.logger.Info("Deleted federation", map[string]interface{}{
			"name": f.config.Name,
		})
	}

	return nil
}

// List returns the names of all existing federations
func List() ([]string, error) {
	return paths.ListFederations()
}

// Exists checks if a federation exists
func Exists(name string) (bool, error) {
	return paths.FederationExists(name)
}
