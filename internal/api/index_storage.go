package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
)

// IndexStorage manages the server's data directory for uploaded repos
type IndexStorage struct {
	dataDir   string
	uploadDir string
	reposDir  string
	logger    *slog.Logger
}

// RepoMeta contains metadata for an uploaded repo
type RepoMeta struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
	LastUpload  time.Time `json:"last_upload,omitempty"`
	Source      string    `json:"source"` // "uploaded" or "config"
}

// NewIndexStorage creates a new storage manager
// dataDir can be absolute or "~/.ckb-server" style
func NewIndexStorage(dataDir string, logger *slog.Logger) (*IndexStorage, error) {
	// Expand ~ to home directory
	if strings.HasPrefix(dataDir, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get home directory: %w", err)
		}
		dataDir = filepath.Join(home, dataDir[2:])
	}

	s := &IndexStorage{
		dataDir:   dataDir,
		uploadDir: filepath.Join(dataDir, "uploads"),
		reposDir:  filepath.Join(dataDir, "repos"),
		logger:    logger,
	}

	// Create directories if they don't exist
	if err := os.MkdirAll(s.uploadDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}
	if err := os.MkdirAll(s.reposDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create repos directory: %w", err)
	}

	logger.Info("Index storage initialized", map[string]interface{}{
		"data_dir": dataDir,
	})

	return s, nil
}

// DataDir returns the root data directory
func (s *IndexStorage) DataDir() string {
	return s.dataDir
}

// RepoPath returns the directory path for a repo
// Repo IDs with "/" are converted to "-" for filesystem safety
func (s *IndexStorage) RepoPath(repoID string) string {
	safeID := sanitizeRepoID(repoID)
	return filepath.Join(s.reposDir, safeID)
}

// DBPath returns the database path for a repo
func (s *IndexStorage) DBPath(repoID string) string {
	return filepath.Join(s.RepoPath(repoID), "ckb.db")
}

// MetaPath returns the metadata file path for a repo
func (s *IndexStorage) MetaPath(repoID string) string {
	return filepath.Join(s.RepoPath(repoID), "meta.json")
}

// CreateRepo creates the directory structure for a new repo
func (s *IndexStorage) CreateRepo(repoID, name, description string) error {
	repoPath := s.RepoPath(repoID)

	// Check if already exists
	if _, err := os.Stat(repoPath); err == nil {
		return fmt.Errorf("repo already exists: %s", repoID)
	}

	// Create directory
	if err := os.MkdirAll(repoPath, 0755); err != nil {
		return fmt.Errorf("failed to create repo directory: %w", err)
	}

	// Create metadata file
	meta := RepoMeta{
		ID:          repoID,
		Name:        name,
		Description: description,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		Source:      "uploaded",
	}

	if err := s.SaveMeta(repoID, &meta); err != nil {
		// Clean up on failure
		_ = os.RemoveAll(repoPath)
		return fmt.Errorf("failed to save metadata: %w", err)
	}

	s.logger.Info("Created repo", map[string]interface{}{
		"repo_id": repoID,
		"path":    repoPath,
	})

	return nil
}

// DeleteRepo removes a repo and all its data
func (s *IndexStorage) DeleteRepo(repoID string) error {
	repoPath := s.RepoPath(repoID)

	// Check if exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return fmt.Errorf("repo not found: %s", repoID)
	}

	// Remove entire directory
	if err := os.RemoveAll(repoPath); err != nil {
		return fmt.Errorf("failed to delete repo: %w", err)
	}

	s.logger.Info("Deleted repo", map[string]interface{}{
		"repo_id": repoID,
	})

	return nil
}

// RepoExists checks if a repo exists in storage
func (s *IndexStorage) RepoExists(repoID string) bool {
	_, err := os.Stat(s.RepoPath(repoID))
	return err == nil
}

// ListRepos returns all repo IDs in the data directory
func (s *IndexStorage) ListRepos() ([]string, error) {
	entries, err := os.ReadDir(s.reposDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to read repos directory: %w", err)
	}

	var repos []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		// Read metadata to get actual repo ID
		metaPath := filepath.Join(s.reposDir, entry.Name(), "meta.json")
		meta, err := s.loadMetaFile(metaPath)
		if err != nil {
			// Use directory name as fallback
			repos = append(repos, unsanitizeRepoID(entry.Name()))
			continue
		}
		repos = append(repos, meta.ID)
	}

	return repos, nil
}

// LoadMeta loads metadata for a repo
func (s *IndexStorage) LoadMeta(repoID string) (*RepoMeta, error) {
	return s.loadMetaFile(s.MetaPath(repoID))
}

func (s *IndexStorage) loadMetaFile(path string) (*RepoMeta, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var meta RepoMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("failed to parse metadata: %w", err)
	}

	return &meta, nil
}

// SaveMeta saves metadata for a repo
func (s *IndexStorage) SaveMeta(repoID string, meta *RepoMeta) error {
	meta.UpdatedAt = time.Now()

	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if err := os.WriteFile(s.MetaPath(repoID), data, 0644); err != nil {
		return fmt.Errorf("failed to write metadata: %w", err)
	}

	return nil
}

// UpdateLastUpload updates the last upload timestamp
func (s *IndexStorage) UpdateLastUpload(repoID string) error {
	meta, err := s.LoadMeta(repoID)
	if err != nil {
		// Create minimal meta if missing
		meta = &RepoMeta{
			ID:        repoID,
			Name:      repoID,
			CreatedAt: time.Now(),
			Source:    "uploaded",
		}
	}

	meta.LastUpload = time.Now()
	return s.SaveMeta(repoID, meta)
}

// CreateUploadFile creates a temporary file for an upload
// Returns the file handle, path, and any error
func (s *IndexStorage) CreateUploadFile() (*os.File, string, error) {
	filename := fmt.Sprintf("%s.scip", uuid.New().String())
	path := filepath.Join(s.uploadDir, filename)

	file, err := os.Create(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create upload file: %w", err)
	}

	return file, path, nil
}

// CleanupUpload removes a temporary upload file
func (s *IndexStorage) CleanupUpload(path string) error {
	// Safety check: only delete files in upload directory
	if !strings.HasPrefix(path, s.uploadDir) {
		return fmt.Errorf("invalid upload path: %s", path)
	}
	return os.Remove(path)
}

// CleanupOldUploads removes uploads older than the given duration
func (s *IndexStorage) CleanupOldUploads(maxAge time.Duration) (int, error) {
	entries, err := os.ReadDir(s.uploadDir)
	if err != nil {
		return 0, err
	}

	cutoff := time.Now().Add(-maxAge)
	cleaned := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(s.uploadDir, entry.Name())
			if err := os.Remove(path); err == nil {
				cleaned++
			}
		}
	}

	if cleaned > 0 {
		s.logger.Info("Cleaned up old uploads", map[string]interface{}{
			"count": cleaned,
		})
	}

	return cleaned, nil
}

// sanitizeRepoID converts repo ID to filesystem-safe format
// "company/core-lib" -> "company-core-lib"
func sanitizeRepoID(id string) string {
	return strings.ReplaceAll(id, "/", "-")
}

// unsanitizeRepoID converts filesystem name back to repo ID
// "company-core-lib" -> "company/core-lib"
// Note: This is a heuristic - org/repo format assumed
func unsanitizeRepoID(name string) string {
	// Find first dash that could be org separator
	idx := strings.Index(name, "-")
	if idx > 0 && idx < len(name)-1 {
		return name[:idx] + "/" + name[idx+1:]
	}
	return name
}
