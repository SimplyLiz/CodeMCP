package paths

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
)

// CanonicalizePath converts an absolute path to a repo-relative canonical path
// - Resolves symlinks to real paths
// - Makes path relative to repo root
// - Converts backslashes to forward slashes
// - Returns repo-relative path with forward slashes
func CanonicalizePath(absolutePath string, repoRoot string) (string, error) {
	// Resolve symlinks
	resolved, err := filepath.EvalSymlinks(absolutePath)
	if err != nil {
		// If the file doesn't exist yet, use the path as-is
		if os.IsNotExist(err) {
			resolved = absolutePath
		} else {
			return "", err
		}
	}

	// Make path relative to repo root
	repoRootResolved, err := filepath.EvalSymlinks(repoRoot)
	if err != nil {
		if os.IsNotExist(err) {
			repoRootResolved = repoRoot
		} else {
			return "", err
		}
	}

	relativePath, err := filepath.Rel(repoRootResolved, resolved)
	if err != nil {
		return "", err
	}

	// Convert to forward slashes (platform independent)
	canonicalPath := filepath.ToSlash(relativePath)

	return canonicalPath, nil
}

// IsWithinRepo checks if a path is within the repository root
func IsWithinRepo(path string, repoRoot string) bool {
	canonical, err := CanonicalizePath(path, repoRoot)
	if err != nil {
		return false
	}

	// Path is outside repo if it starts with ..
	return !strings.HasPrefix(canonical, "..")
}

// NormalizePath normalizes a path by converting backslashes to forward slashes
// This is useful for paths that are already relative but need normalization
func NormalizePath(path string) string {
	return filepath.ToSlash(path)
}

// JoinRepoPath joins a repo root with a canonical path
func JoinRepoPath(repoRoot string, canonicalPath string) string {
	// Ensure we use forward slashes in the canonical path
	normalizedPath := strings.ReplaceAll(canonicalPath, "\\", "/")
	// Convert to OS-specific path separator for joining
	parts := strings.Split(normalizedPath, "/")
	return filepath.Join(append([]string{repoRoot}, parts...)...)
}

// FindRepoRoot finds the repository root directory
// This is a placeholder implementation that returns the current directory
func FindRepoRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return cwd, nil
}

// v6.0 Persistence Paths
// These functions manage the ~/.ckb/repos/ directory structure for persistent architectural data

const (
	// DefaultCKBHome is the default directory for CKB global data
	DefaultCKBHome = ".ckb"

	// ReposSubdir is the subdirectory for per-repo data
	ReposSubdir = "repos"

	// DecisionsSubdir is the subdirectory for ADR files
	DecisionsSubdir = "decisions"

	// CKBHomeEnvVar is the environment variable to override CKB home
	CKBHomeEnvVar = "CKB_HOME"
)

// GetCKBHome returns the CKB home directory
// Prefers $CKB_HOME environment variable, falls back to ~/.ckb
func GetCKBHome() (string, error) {
	// Check environment variable first
	if envHome := os.Getenv(CKBHomeEnvVar); envHome != "" {
		return envHome, nil
	}

	// Fall back to home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	return filepath.Join(homeDir, DefaultCKBHome), nil
}

// ComputeRepoHash generates a stable hash for a repository path
// This is used to create unique per-repo directories
func ComputeRepoHash(repoRoot string) string {
	// Resolve to absolute path
	absPath, err := filepath.Abs(repoRoot)
	if err != nil {
		absPath = repoRoot
	}

	// Resolve symlinks if possible
	resolved, err := filepath.EvalSymlinks(absPath)
	if err == nil {
		absPath = resolved
	}

	// Normalize path
	normalized := filepath.ToSlash(absPath)

	// Compute hash
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:8]) // Use first 8 bytes for shorter hash
}

// GetRepoDataDir returns the data directory for a specific repository
// Path: ~/.ckb/repos/<repo-hash>/
func GetRepoDataDir(repoRoot string) (string, error) {
	ckbHome, err := GetCKBHome()
	if err != nil {
		return "", err
	}

	repoHash := ComputeRepoHash(repoRoot)
	return filepath.Join(ckbHome, ReposSubdir, repoHash), nil
}

// EnsureRepoDataDir creates the repo data directory if it doesn't exist
// Returns the directory path
func EnsureRepoDataDir(repoRoot string) (string, error) {
	dataDir, err := GetRepoDataDir(repoRoot)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return "", err
	}

	return dataDir, nil
}

// GetDecisionsDir returns the ADR decisions directory for a repository
// Path: ~/.ckb/repos/<repo-hash>/decisions/
func GetDecisionsDir(repoRoot string) (string, error) {
	dataDir, err := GetRepoDataDir(repoRoot)
	if err != nil {
		return "", err
	}

	return filepath.Join(dataDir, DecisionsSubdir), nil
}

// EnsureDecisionsDir creates the decisions directory if it doesn't exist
// Returns the directory path
func EnsureDecisionsDir(repoRoot string) (string, error) {
	decisionsDir, err := GetDecisionsDir(repoRoot)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(decisionsDir, 0755); err != nil {
		return "", err
	}

	return decisionsDir, nil
}

// GetRepoDatabasePath returns the path to the repo-specific database
// Path: ~/.ckb/repos/<repo-hash>/ckb.db
func GetRepoDatabasePath(repoRoot string) (string, error) {
	dataDir, err := GetRepoDataDir(repoRoot)
	if err != nil {
		return "", err
	}

	return filepath.Join(dataDir, "ckb.db"), nil
}

// GetLocalDatabasePath returns the path to the local .ckb/ckb.db database
// Path: <repoRoot>/.ckb/ckb.db
func GetLocalDatabasePath(repoRoot string) string {
	return filepath.Join(repoRoot, ".ckb", "ckb.db")
}

// GetSCIPIndexPath returns the path to the SCIP index file
// Path: <repoRoot>/index.scip or configured path
func GetSCIPIndexPath(repoRoot string, configuredPath string) string {
	if configuredPath != "" {
		if filepath.IsAbs(configuredPath) {
			return configuredPath
		}
		return filepath.Join(repoRoot, configuredPath)
	}
	return filepath.Join(repoRoot, "index.scip")
}

// RepoInfo holds information about paths for a repository
type RepoInfo struct {
	// Root is the repository root directory
	Root string

	// Hash is the stable hash of the repository
	Hash string

	// LocalCKBDir is the .ckb directory in the repo root
	LocalCKBDir string

	// GlobalDataDir is the ~/.ckb/repos/<hash>/ directory
	GlobalDataDir string

	// LocalDatabasePath is the path to the local database
	LocalDatabasePath string

	// GlobalDatabasePath is the path to the global database
	GlobalDatabasePath string

	// DecisionsDir is the path to the ADR directory
	DecisionsDir string
}

// GetRepoInfo returns all path information for a repository
func GetRepoInfo(repoRoot string) (*RepoInfo, error) {
	// Resolve to absolute path
	absRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return nil, err
	}

	hash := ComputeRepoHash(absRoot)

	ckbHome, err := GetCKBHome()
	if err != nil {
		return nil, err
	}

	globalDataDir := filepath.Join(ckbHome, ReposSubdir, hash)

	return &RepoInfo{
		Root:               absRoot,
		Hash:               hash,
		LocalCKBDir:        filepath.Join(absRoot, ".ckb"),
		GlobalDataDir:      globalDataDir,
		LocalDatabasePath:  filepath.Join(absRoot, ".ckb", "ckb.db"),
		GlobalDatabasePath: filepath.Join(globalDataDir, "ckb.db"),
		DecisionsDir:       filepath.Join(globalDataDir, DecisionsSubdir),
	}, nil
}
