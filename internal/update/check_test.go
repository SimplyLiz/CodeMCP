package update

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected [3]int
	}{
		{"7.3.0", [3]int{7, 3, 0}},
		{"1.0.0", [3]int{1, 0, 0}},
		{"10.20.30", [3]int{10, 20, 30}},
		{"7.3.0-beta.1", [3]int{7, 3, 0}},
		{"1.0.0-rc1", [3]int{1, 0, 0}},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseVersion(tt.input)
			if result != tt.expected {
				t.Errorf("parseVersion(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsNewerVersion(t *testing.T) {
	tests := []struct {
		a, b     string
		expected bool
	}{
		{"7.4.0", "7.3.0", true},
		{"7.3.1", "7.3.0", true},
		{"8.0.0", "7.3.0", true},
		{"7.3.0", "7.3.0", false},
		{"7.2.0", "7.3.0", false},
		{"7.3.0", "7.4.0", false},
		{"6.0.0", "7.0.0", false},
	}

	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			result := isNewerVersion(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("isNewerVersion(%q, %q) = %v, want %v", tt.a, tt.b, result, tt.expected)
			}
		})
	}
}

func TestChecker_Check_DisabledByEnv(t *testing.T) {
	// Set the disable env var
	_ = os.Setenv("CKB_NO_UPDATE_CHECK", "1")
	defer func() { _ = os.Unsetenv("CKB_NO_UPDATE_CHECK") }()

	checker := &Checker{
		cache:     NewCache(),
		isNpmPath: true, // Pretend we're an npm install
	}

	result := checker.Check(context.Background())
	if result != nil {
		t.Errorf("expected nil when update check is disabled, got %+v", result)
	}
}

func TestChecker_CheckCached_EmptyCache(t *testing.T) {
	// Create a temp directory for testing
	tmpDir, err := os.MkdirTemp("", "ckb-update-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	checker := &Checker{
		cache: &Cache{
			path: filepath.Join(tmpDir, "update-check.json"),
		},
		isNpmPath: false,
	}

	result := checker.CheckCached()
	if result != nil {
		t.Errorf("expected nil for empty cache, got %+v", result)
	}
}

func TestChecker_CheckCached_WithCachedUpdate(t *testing.T) {
	// Create a temp directory for testing
	tmpDir, err := os.MkdirTemp("", "ckb-update-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cache := &Cache{
		path: filepath.Join(tmpDir, "update-check.json"),
	}
	// Set a version higher than current
	cache.Set("99.0.0")

	checker := &Checker{
		cache:     cache,
		isNpmPath: false,
	}

	result := checker.CheckCached()
	if result == nil {
		t.Fatal("expected update info for newer cached version")
	}
	if result.LatestVersion != "99.0.0" {
		t.Errorf("expected version 99.0.0, got %s", result.LatestVersion)
	}
	// Non-npm install should get GitHub URL
	if result.UpdateCommand != githubReleasesPage {
		t.Errorf("expected GitHub URL, got %s", result.UpdateCommand)
	}
}

func TestChecker_CheckCached_NpmInstall(t *testing.T) {
	// Create a temp directory for testing
	tmpDir, err := os.MkdirTemp("", "ckb-update-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cache := &Cache{
		path: filepath.Join(tmpDir, "update-check.json"),
	}
	cache.Set("99.0.0")

	checker := &Checker{
		cache:     cache,
		isNpmPath: true, // npm install
	}

	result := checker.CheckCached()
	if result == nil {
		t.Fatal("expected update info")
	}
	// npm install should get npm command
	if result.UpdateCommand != "npm update -g @tastehub/ckb" {
		t.Errorf("expected npm command, got %s", result.UpdateCommand)
	}
}

func TestChecker_FetchLatestVersion_Timeout(t *testing.T) {
	checker := &Checker{
		cache:     NewCache(),
		isNpmPath: false,
	}

	// Test with a very short timeout context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()

	// This should return empty string due to timeout
	result := checker.fetchLatestVersion(ctx)
	if result != "" {
		// It's okay if the request completed before timeout
		// The test just ensures we don't panic
		t.Logf("Request completed despite short timeout: %s", result)
	}
}

func TestGitHubReleaseTagParsing(t *testing.T) {
	tests := []struct {
		tag      string
		expected string
	}{
		{"v7.4.0", "7.4.0"},
		{"7.4.0", "7.4.0"},
		{"v1.0.0", "1.0.0"},
		{"v10.20.30", "10.20.30"},
	}

	for _, tt := range tests {
		t.Run(tt.tag, func(t *testing.T) {
			// Simulate the tag stripping logic
			result := tt.tag
			if len(result) > 0 && result[0] == 'v' {
				result = result[1:]
			}
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestCache_GetSet(t *testing.T) {
	// Create a temp directory for testing
	tmpDir, err := os.MkdirTemp("", "ckb-update-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	cache := &Cache{
		path: filepath.Join(tmpDir, "update-check.json"),
	}

	// Initially cache should be empty
	entry, needsRefresh := cache.Get()
	if entry != nil {
		t.Errorf("expected nil entry for empty cache, got %+v", entry)
	}
	if !needsRefresh {
		t.Error("expected needsRefresh=true for empty cache")
	}

	// Set a value
	cache.Set("7.5.0")

	// Now should return the cached value
	entry, needsRefresh = cache.Get()
	if entry == nil {
		t.Fatal("expected non-nil entry after Set")
	}
	if entry.LatestVersion != "7.5.0" {
		t.Errorf("expected version 7.5.0, got %s", entry.LatestVersion)
	}
	if needsRefresh {
		t.Error("expected needsRefresh=false for fresh cache")
	}
}

func TestUpdateInfo_FormatUpdateMessage(t *testing.T) {
	t.Run("npm command", func(t *testing.T) {
		info := &UpdateInfo{
			CurrentVersion: "7.3.0",
			LatestVersion:  "7.4.0",
			UpdateCommand:  "npm update -g @tastehub/ckb",
		}

		msg := info.FormatUpdateMessage()
		if msg == "" {
			t.Error("expected non-empty message")
		}
		// Should include "Run:" prefix for commands
		if !strings.Contains(msg, "Run:") {
			t.Error("expected 'Run:' prefix for npm command")
		}

		plain := info.FormatUpdateMessagePlain()
		if plain == "" {
			t.Error("expected non-empty plain message")
		}
	})

	t.Run("GitHub URL", func(t *testing.T) {
		info := &UpdateInfo{
			CurrentVersion: "7.3.0",
			LatestVersion:  "7.4.0",
			UpdateCommand:  "https://github.com/SimplyLiz/CodeMCP/releases",
		}

		msg := info.FormatUpdateMessage()
		if msg == "" {
			t.Error("expected non-empty message")
		}
		// Should NOT include "Run:" prefix for URLs
		if strings.Contains(msg, "Run:") {
			t.Error("should not have 'Run:' prefix for URL")
		}
		// Should contain the URL
		if !strings.Contains(msg, "github.com") {
			t.Error("expected GitHub URL in message")
		}

		plain := info.FormatUpdateMessagePlain()
		if plain == "" {
			t.Error("expected non-empty plain message")
		}
	})
}
