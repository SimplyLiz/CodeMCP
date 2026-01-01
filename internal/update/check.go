// Package update provides npm update checking for CKB.
// It checks if a newer version is available on npm and notifies the user.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"ckb/internal/version"
)

const (
	// npmRegistryURL is the npm registry endpoint for package info
	npmRegistryURL = "https://registry.npmjs.org/@tastehub/ckb/latest"

	// checkInterval is how often to check for updates (24 hours)
	checkInterval = 24 * time.Hour

	// httpTimeout is the timeout for the npm registry request
	httpTimeout = 3 * time.Second

	// npmPackageName is the npm package name
	npmPackageName = "@tastehub/ckb"
)

// npmPackageInfo represents the relevant fields from npm registry response
type npmPackageInfo struct {
	Version string `json:"version"`
}

// UpdateInfo contains information about an available update
type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	UpdateCommand  string
}

// Checker handles update checking with caching
type Checker struct {
	cache     *Cache
	isNpmPath bool
}

// NewChecker creates a new update checker.
// It automatically detects if running from an npm installation.
func NewChecker() *Checker {
	return &Checker{
		cache:     NewCache(),
		isNpmPath: detectNpmInstall(),
	}
}

// detectNpmInstall checks if the current executable is running from an npm installation
func detectNpmInstall() bool {
	execPath, err := os.Executable()
	if err != nil {
		return false
	}

	// Resolve symlinks to get the real path
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		realPath = execPath
	}

	// Check if path contains node_modules/@tastehub/ckb
	return strings.Contains(realPath, "node_modules") &&
		strings.Contains(realPath, "@tastehub") &&
		strings.Contains(realPath, "ckb")
}

// IsNpmInstall returns true if running from an npm installation
func (c *Checker) IsNpmInstall() bool {
	return c.isNpmPath
}

// Check checks for available updates.
// Returns nil if no update is available, check is disabled, or any error occurs.
// This method is designed to fail silently - it never returns an error.
func (c *Checker) Check(ctx context.Context) *UpdateInfo {
	// Skip if not installed via npm
	if !c.isNpmPath {
		return nil
	}

	// Skip if disabled via environment variable
	if os.Getenv("CKB_NO_UPDATE_CHECK") != "" {
		return nil
	}

	// Check cache first
	cached, needsRefresh := c.cache.Get()
	if cached != nil && !needsRefresh {
		return c.compareVersions(cached.LatestVersion)
	}

	// Fetch latest version from npm (with timeout)
	latest := c.fetchLatestVersion(ctx)
	if latest == "" {
		return nil
	}

	// Update cache
	c.cache.Set(latest)

	return c.compareVersions(latest)
}

// CheckAsync runs the update check in the background and returns results via channel.
// The channel will receive at most one result and then be closed.
func (c *Checker) CheckAsync(ctx context.Context) <-chan *UpdateInfo {
	ch := make(chan *UpdateInfo, 1)

	go func() {
		defer close(ch)
		if info := c.Check(ctx); info != nil {
			ch <- info
		}
	}()

	return ch
}

// fetchLatestVersion fetches the latest version from npm registry.
// Returns empty string on any error (silent failure).
func (c *Checker) fetchLatestVersion(ctx context.Context) string {
	// Create context with timeout
	ctx, cancel := context.WithTimeout(ctx, httpTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, npmRegistryURL, nil)
	if err != nil {
		return ""
	}

	// Set a reasonable user agent
	req.Header.Set("User-Agent", "ckb/"+version.Version)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return ""
	}

	var pkg npmPackageInfo
	if err := json.NewDecoder(resp.Body).Decode(&pkg); err != nil {
		return ""
	}

	return pkg.Version
}

// compareVersions compares current version with latest and returns UpdateInfo if update available
func (c *Checker) compareVersions(latest string) *UpdateInfo {
	current := version.Version

	// Simple semver comparison - assumes versions are in format X.Y.Z
	if !isNewerVersion(latest, current) {
		return nil
	}

	return &UpdateInfo{
		CurrentVersion: current,
		LatestVersion:  latest,
		UpdateCommand:  "npm update -g " + npmPackageName,
	}
}

// isNewerVersion returns true if version a is newer than version b.
// Handles semver format X.Y.Z with optional pre-release suffixes.
func isNewerVersion(a, b string) bool {
	partsA := parseVersion(a)
	partsB := parseVersion(b)

	for i := 0; i < 3; i++ {
		if partsA[i] > partsB[i] {
			return true
		}
		if partsA[i] < partsB[i] {
			return false
		}
	}

	return false
}

// parseVersion extracts major, minor, patch from a version string
func parseVersion(v string) [3]int {
	var parts [3]int

	// Strip any pre-release suffix (e.g., "-beta.1")
	if idx := strings.Index(v, "-"); idx > 0 {
		v = v[:idx]
	}

	// Parse X.Y.Z
	fmt.Sscanf(v, "%d.%d.%d", &parts[0], &parts[1], &parts[2])

	return parts
}

// FormatUpdateMessage formats the update notification for CLI output
func (u *UpdateInfo) FormatUpdateMessage() string {
	return fmt.Sprintf(
		"\n\033[33m╭─────────────────────────────────────────────────────╮\033[0m\n"+
			"\033[33m│\033[0m  Update available: \033[36m%s\033[0m → \033[32m%s\033[0m%s\033[33m│\033[0m\n"+
			"\033[33m│\033[0m  Run: \033[1m%s\033[0m%s\033[33m│\033[0m\n"+
			"\033[33m╰─────────────────────────────────────────────────────╯\033[0m\n",
		u.CurrentVersion,
		u.LatestVersion,
		strings.Repeat(" ", 21-len(u.CurrentVersion)-len(u.LatestVersion)),
		u.UpdateCommand,
		strings.Repeat(" ", 31-len(u.UpdateCommand)),
	)
}

// FormatUpdateMessagePlain formats the update notification without colors
func (u *UpdateInfo) FormatUpdateMessagePlain() string {
	return fmt.Sprintf(
		"\nUpdate available: %s → %s\nRun: %s\n",
		u.CurrentVersion,
		u.LatestVersion,
		u.UpdateCommand,
	)
}
