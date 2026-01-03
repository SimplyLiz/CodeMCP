package repos

import (
	"os"
	"path/filepath"
)

// ResolutionSource indicates how the active repo was determined.
type ResolutionSource string

const (
	// ResolvedFromEnv indicates the repo was set via CKB_REPO env var.
	ResolvedFromEnv ResolutionSource = "env"

	// ResolvedFromFlag indicates the repo was set via --repo flag.
	ResolvedFromFlag ResolutionSource = "flag"

	// ResolvedFromCWD indicates the CWD matches a registered repo.
	ResolvedFromCWD ResolutionSource = "cwd"

	// ResolvedFromDefault indicates the default repo from registry was used.
	ResolvedFromDefault ResolutionSource = "default"

	// ResolvedNone indicates no active repo could be determined.
	ResolvedNone ResolutionSource = ""
)

// ResolvedRepo contains the resolved active repository and how it was determined.
type ResolvedRepo struct {
	// Entry is the resolved repository entry, nil if none.
	Entry *RepoEntry

	// Source indicates how the repo was resolved.
	Source ResolutionSource

	// State is the current state of the resolved repo.
	State RepoState
}

// ResolveActiveRepo determines the active repository using the resolution order:
// 1. CKB_REPO environment variable
// 2. flagValue (--repo flag, if provided)
// 3. Current working directory matches a registered repo
// 4. Default repo from registry
// 5. No active repo
func ResolveActiveRepo(flagValue string) (*ResolvedRepo, error) {
	registry, err := LoadRegistry()
	if err != nil {
		return nil, err
	}

	return ResolveActiveRepoWithRegistry(registry, flagValue)
}

// ResolveActiveRepoWithRegistry resolves using an already-loaded registry.
func ResolveActiveRepoWithRegistry(registry *Registry, flagValue string) (*ResolvedRepo, error) {
	// 1. Check CKB_REPO environment variable
	if envRepo := os.Getenv("CKB_REPO"); envRepo != "" {
		entry, state, err := registry.Get(envRepo)
		if err == nil {
			return &ResolvedRepo{
				Entry:  entry,
				Source: ResolvedFromEnv,
				State:  state,
			}, nil
		}
		// If env var is set but repo doesn't exist, we still report it
		// so the user knows their env var is misconfigured
	}

	// 2. Check --repo flag
	if flagValue != "" {
		entry, state, err := registry.Get(flagValue)
		if err == nil {
			return &ResolvedRepo{
				Entry:  entry,
				Source: ResolvedFromFlag,
				State:  state,
			}, nil
		}
		// If flag is set but repo doesn't exist, we still report it
	}

	// 3. Check if CWD is within a registered repo
	cwd, err := os.Getwd()
	if err == nil {
		if entry := findRepoContainingPath(registry, cwd); entry != nil {
			state := registry.ValidateState(entry.Name)
			return &ResolvedRepo{
				Entry:  entry,
				Source: ResolvedFromCWD,
				State:  state,
			}, nil
		}
	}

	// 4. Use default from registry
	if registry.Default != "" {
		entry, state, err := registry.Get(registry.Default)
		if err == nil {
			return &ResolvedRepo{
				Entry:  entry,
				Source: ResolvedFromDefault,
				State:  state,
			}, nil
		}
	}

	// 5. No active repo
	return &ResolvedRepo{
		Entry:  nil,
		Source: ResolvedNone,
		State:  "",
	}, nil
}

// findRepoContainingPath finds a registered repo whose path contains the given path.
// This handles the case where the user is in a subdirectory of a registered repo.
// When multiple repos match, returns the most specific one (longest path).
func findRepoContainingPath(registry *Registry, path string) *RepoEntry {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return nil
	}
	absPath = filepath.Clean(absPath)

	// Resolve symlinks for comparison (handles macOS /var -> /private/var)
	resolvedPath, err := filepath.EvalSymlinks(absPath)
	if err != nil {
		resolvedPath = absPath
	}

	var bestMatch *RepoEntry
	bestMatchLen := -1

	for _, entry := range registry.Repos {
		entryPath := entry.Path
		// Resolve symlinks in entry path too
		resolvedEntryPath, err := filepath.EvalSymlinks(entryPath)
		if err != nil {
			resolvedEntryPath = entryPath
		}

		// Check if this repo contains our path
		rel, err := filepath.Rel(resolvedEntryPath, resolvedPath)
		if err != nil {
			continue
		}

		// Path is inside repo if:
		// - rel == "." (exact match)
		// - rel doesn't start with ".." (is a subdirectory)
		isInside := rel == "." || (len(rel) > 0 && rel[0] != '.')
		if !isInside {
			continue
		}

		// Pick the most specific match (longest path wins)
		if len(resolvedEntryPath) > bestMatchLen {
			bestMatchLen = len(resolvedEntryPath)
			entryCopy := entry
			bestMatch = &entryCopy
		}
	}

	return bestMatch
}

// IsInRegisteredRepo checks if the given path is within a registered repo.
func IsInRegisteredRepo(path string) (bool, *RepoEntry, error) {
	registry, err := LoadRegistry()
	if err != nil {
		return false, nil, err
	}

	entry := findRepoContainingPath(registry, path)
	return entry != nil, entry, nil
}

// GetRepoRoot returns the root path for the active repo, or empty string if none.
// This is a convenience function for commands that need just the path.
func GetRepoRoot(flagValue string) (string, error) {
	resolved, err := ResolveActiveRepo(flagValue)
	if err != nil {
		return "", err
	}

	if resolved.Entry == nil {
		return "", nil
	}

	return resolved.Entry.Path, nil
}
