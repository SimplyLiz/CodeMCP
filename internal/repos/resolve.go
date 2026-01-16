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

	// ResolvedFromCWDGit indicates the CWD is in a git repo that's not registered.
	// This enables auto-detection of unregistered repositories.
	ResolvedFromCWDGit ResolutionSource = "cwd_git"

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

	// DetectedGitRoot is set when CWD is in a git repo (registered or not).
	// Used for UX warnings when falling back to default.
	DetectedGitRoot string

	// SkippedDefault is set when a default repo exists but was not used
	// because CWD is in a different git repo. Used for UX warnings.
	SkippedDefault string
}

// ResolveActiveRepo determines the active repository using the resolution order:
// 1. CKB_REPO environment variable
// 2. flagValue (--repo flag, if provided)
// 3. Current working directory matches a registered repo
// 3.5. Current working directory is in a git repo (auto-detect unregistered)
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
	// Detect git root early for UX purposes
	cwd, _ := os.Getwd()
	detectedGitRoot := ""
	if cwd != "" {
		detectedGitRoot = FindGitRoot(cwd)
	}

	// 1. Check CKB_REPO environment variable
	if envRepo := os.Getenv("CKB_REPO"); envRepo != "" {
		entry, state, err := registry.Get(envRepo)
		if err == nil {
			return &ResolvedRepo{
				Entry:           entry,
				Source:          ResolvedFromEnv,
				State:           state,
				DetectedGitRoot: detectedGitRoot,
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
				Entry:           entry,
				Source:          ResolvedFromFlag,
				State:           state,
				DetectedGitRoot: detectedGitRoot,
			}, nil
		}
		// If flag is set but repo doesn't exist, we still report it
	}

	// 3. Check if CWD is within a registered repo
	if cwd != "" {
		if entry := findRepoContainingPath(registry, cwd); entry != nil {
			state := registry.ValidateState(entry.Name)
			return &ResolvedRepo{
				Entry:           entry,
				Source:          ResolvedFromCWD,
				State:           state,
				DetectedGitRoot: detectedGitRoot,
			}, nil
		}
	}

	// 3.5. Auto-detect: CWD is in a git repo but not registered
	// This takes precedence over the default registry entry
	if detectedGitRoot != "" {
		// Determine state based on whether .ckb exists
		state := RepoStateUninitialized
		if checkCkbInitialized(detectedGitRoot) {
			state = RepoStateValid
		}

		// Generate a friendly name from the directory
		name := filepath.Base(detectedGitRoot)

		// Track if we're skipping a default
		skippedDefault := ""
		if registry.Default != "" {
			skippedDefault = registry.Default
		}

		return &ResolvedRepo{
			Entry: &RepoEntry{
				Name: name,
				Path: detectedGitRoot,
			},
			Source:          ResolvedFromCWDGit,
			State:           state,
			DetectedGitRoot: detectedGitRoot,
			SkippedDefault:  skippedDefault,
		}, nil
	}

	// 4. Use default from registry
	if registry.Default != "" {
		entry, state, err := registry.Get(registry.Default)
		if err == nil {
			return &ResolvedRepo{
				Entry:           entry,
				Source:          ResolvedFromDefault,
				State:           state,
				DetectedGitRoot: detectedGitRoot,
			}, nil
		}
	}

	// 5. No active repo
	return &ResolvedRepo{
		Entry:           nil,
		Source:          ResolvedNone,
		State:           "",
		DetectedGitRoot: detectedGitRoot,
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

// FindGitRoot walks up the directory tree from the given path to find the git root.
// Returns the path containing .git, or empty string if not in a git repo.
func FindGitRoot(path string) string {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return ""
	}

	// Resolve symlinks (handles macOS /var -> /private/var)
	absPath, err = filepath.EvalSymlinks(absPath)
	if err != nil {
		// Fall back to original path
		absPath, _ = filepath.Abs(path)
	}

	current := absPath
	for {
		gitPath := filepath.Join(current, ".git")
		if info, err := os.Stat(gitPath); err == nil {
			// .git can be a directory (normal repo) or file (worktree/submodule)
			if info.IsDir() || info.Mode().IsRegular() {
				return current
			}
		}

		parent := filepath.Dir(current)
		if parent == current {
			// Reached root
			return ""
		}
		current = parent
	}
}

// checkCkbInitialized checks if a directory has been initialized with ckb init.
func checkCkbInitialized(path string) bool {
	ckbDir := filepath.Join(path, ".ckb")
	info, err := os.Stat(ckbDir)
	return err == nil && info.IsDir()
}
