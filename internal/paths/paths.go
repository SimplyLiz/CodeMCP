package paths

import (
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
