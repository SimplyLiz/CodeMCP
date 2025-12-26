package telemetry

import (
	"testing"

	"ckb/internal/config"
)

func TestNewServiceMapper(t *testing.T) {
	t.Run("creates mapper with empty config", func(t *testing.T) {
		cfg := config.TelemetryConfig{}
		sm, err := NewServiceMapper(cfg)
		if err != nil {
			t.Fatalf("NewServiceMapper failed: %v", err)
		}
		if sm == nil {
			t.Fatal("expected non-nil mapper")
		}
	})

	t.Run("creates mapper with service map", func(t *testing.T) {
		cfg := config.TelemetryConfig{
			ServiceMap: map[string]string{
				"my-service": "repo1",
			},
		}
		sm, err := NewServiceMapper(cfg)
		if err != nil {
			t.Fatalf("NewServiceMapper failed: %v", err)
		}
		if sm.GetMappedServices() != 1 {
			t.Errorf("expected 1 mapped service, got %d", sm.GetMappedServices())
		}
	})

	t.Run("creates mapper with patterns", func(t *testing.T) {
		cfg := config.TelemetryConfig{
			ServicePatterns: []config.TelemetryServicePattern{
				{Pattern: "^(.+)-prod$", Repo: "$1"},
			},
		}
		sm, err := NewServiceMapper(cfg)
		if err != nil {
			t.Fatalf("NewServiceMapper failed: %v", err)
		}
		if sm == nil {
			t.Fatal("expected non-nil mapper")
		}
	})

	t.Run("fails with invalid regex", func(t *testing.T) {
		cfg := config.TelemetryConfig{
			ServicePatterns: []config.TelemetryServicePattern{
				{Pattern: "[invalid", Repo: "test"},
			},
		}
		_, err := NewServiceMapper(cfg)
		if err == nil {
			t.Error("expected error for invalid regex")
		}
	})
}

func TestServiceMapperResolve(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		cfg := config.TelemetryConfig{
			ServiceMap: map[string]string{
				"user-service": "user-repo",
				"api-gateway":  "gateway-repo",
			},
		}
		sm, _ := NewServiceMapper(cfg)

		result := sm.Resolve("user-service")
		if !result.Matched {
			t.Error("expected match")
		}
		if result.MatchType != "exact" {
			t.Errorf("expected 'exact' match type, got %q", result.MatchType)
		}
		if result.RepoID != "user-repo" {
			t.Errorf("expected 'user-repo', got %q", result.RepoID)
		}
	})

	t.Run("pattern match", func(t *testing.T) {
		cfg := config.TelemetryConfig{
			ServicePatterns: []config.TelemetryServicePattern{
				{Pattern: "^(.+)-prod$", Repo: "$1"},
			},
		}
		sm, _ := NewServiceMapper(cfg)

		result := sm.Resolve("api-prod")
		if !result.Matched {
			t.Error("expected match")
		}
		if result.MatchType != "pattern" {
			t.Errorf("expected 'pattern' match type, got %q", result.MatchType)
		}
		if result.RepoID != "api" {
			t.Errorf("expected 'api', got %q", result.RepoID)
		}
	})

	t.Run("exact takes priority over pattern", func(t *testing.T) {
		cfg := config.TelemetryConfig{
			ServiceMap: map[string]string{
				"api-prod": "exact-repo",
			},
			ServicePatterns: []config.TelemetryServicePattern{
				{Pattern: "^(.+)-prod$", Repo: "$1"},
			},
		}
		sm, _ := NewServiceMapper(cfg)

		result := sm.Resolve("api-prod")
		if result.MatchType != "exact" {
			t.Errorf("expected 'exact' (priority), got %q", result.MatchType)
		}
		if result.RepoID != "exact-repo" {
			t.Errorf("expected 'exact-repo', got %q", result.RepoID)
		}
	})

	t.Run("no match", func(t *testing.T) {
		cfg := config.TelemetryConfig{
			ServiceMap: map[string]string{
				"known-service": "repo1",
			},
		}
		sm, _ := NewServiceMapper(cfg)

		result := sm.Resolve("unknown-service")
		if result.Matched {
			t.Error("expected no match")
		}
		if result.MatchType != "none" {
			t.Errorf("expected 'none' match type, got %q", result.MatchType)
		}
	})
}

func TestServiceMapperUpdateConfig(t *testing.T) {
	cfg1 := config.TelemetryConfig{
		ServiceMap: map[string]string{
			"old-service": "old-repo",
		},
	}
	sm, _ := NewServiceMapper(cfg1)

	// Verify initial config
	result := sm.Resolve("old-service")
	if !result.Matched {
		t.Error("expected old-service to match initially")
	}

	// Update config
	cfg2 := config.TelemetryConfig{
		ServiceMap: map[string]string{
			"new-service": "new-repo",
		},
	}
	err := sm.UpdateConfig(cfg2)
	if err != nil {
		t.Fatalf("UpdateConfig failed: %v", err)
	}

	// Old service should no longer match
	result = sm.Resolve("old-service")
	if result.Matched {
		t.Error("old-service should no longer match after update")
	}

	// New service should match
	result = sm.Resolve("new-service")
	if !result.Matched {
		t.Error("new-service should match after update")
	}
}

func TestServiceMapperUpdateConfigInvalidRegex(t *testing.T) {
	cfg1 := config.TelemetryConfig{}
	sm, _ := NewServiceMapper(cfg1)

	cfg2 := config.TelemetryConfig{
		ServicePatterns: []config.TelemetryServicePattern{
			{Pattern: "[invalid", Repo: "test"},
		},
	}
	err := sm.UpdateConfig(cfg2)
	if err == nil {
		t.Error("expected error for invalid regex in update")
	}
}

func TestServiceMapperGetMappedServices(t *testing.T) {
	cfg := config.TelemetryConfig{
		ServiceMap: map[string]string{
			"svc1": "repo1",
			"svc2": "repo2",
			"svc3": "repo3",
		},
	}
	sm, _ := NewServiceMapper(cfg)

	count := sm.GetMappedServices()
	if count != 3 {
		t.Errorf("expected 3 mapped services, got %d", count)
	}
}

func TestServiceMapperIsEnabled(t *testing.T) {
	t.Run("enabled", func(t *testing.T) {
		cfg := config.TelemetryConfig{Enabled: true}
		sm, _ := NewServiceMapper(cfg)

		if !sm.IsEnabled() {
			t.Error("expected IsEnabled to return true")
		}
	})

	t.Run("disabled", func(t *testing.T) {
		cfg := config.TelemetryConfig{Enabled: false}
		sm, _ := NewServiceMapper(cfg)

		if sm.IsEnabled() {
			t.Error("expected IsEnabled to return false")
		}
	})
}

func TestServiceMapperTestMapping(t *testing.T) {
	cfg := config.TelemetryConfig{
		ServiceMap: map[string]string{
			"test-service": "test-repo",
		},
	}
	sm, _ := NewServiceMapper(cfg)

	repoID, matchType, matched := sm.TestMapping("test-service")
	if !matched {
		t.Error("expected match")
	}
	if matchType != "exact" {
		t.Errorf("expected 'exact', got %q", matchType)
	}
	if repoID != "test-repo" {
		t.Errorf("expected 'test-repo', got %q", repoID)
	}

	_, matchType, matched = sm.TestMapping("unknown")
	if matched {
		t.Error("expected no match")
	}
	if matchType != "none" {
		t.Errorf("expected 'none', got %q", matchType)
	}
}
