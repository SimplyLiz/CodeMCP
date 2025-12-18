package telemetry

import (
	"regexp"
	"sync"

	"ckb/internal/config"
)

// ServiceMapper resolves service names to repository IDs
type ServiceMapper struct {
	config   config.TelemetryConfig
	patterns []*compiledPattern
	mu       sync.RWMutex
}

type compiledPattern struct {
	regex   *regexp.Regexp
	replace string
}

// NewServiceMapper creates a new service mapper from config
func NewServiceMapper(cfg config.TelemetryConfig) (*ServiceMapper, error) {
	sm := &ServiceMapper{
		config: cfg,
	}

	// Compile regex patterns
	for _, p := range cfg.ServicePatterns {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return nil, err
		}
		sm.patterns = append(sm.patterns, &compiledPattern{
			regex:   re,
			replace: p.Repo,
		})
	}

	return sm, nil
}

// ServiceMapResult contains the result of a service mapping lookup
type ServiceMapResult struct {
	RepoID     string
	Matched    bool
	MatchType  string // "exact" | "pattern" | "none"
	ModuleHint string // optional module hint for large repos
}

// Resolve resolves a service name to a repository ID
func (sm *ServiceMapper) Resolve(serviceName string) ServiceMapResult {
	sm.mu.RLock()
	defer sm.mu.RUnlock()

	// 1. Try exact match in service_map
	if repoID, ok := sm.config.ServiceMap[serviceName]; ok {
		return ServiceMapResult{
			RepoID:    repoID,
			Matched:   true,
			MatchType: "exact",
		}
	}

	// 2. Try pattern match
	for _, p := range sm.patterns {
		if p.regex.MatchString(serviceName) {
			repoID := p.regex.ReplaceAllString(serviceName, p.replace)
			return ServiceMapResult{
				RepoID:    repoID,
				Matched:   true,
				MatchType: "pattern",
			}
		}
	}

	// 3. No match
	return ServiceMapResult{
		Matched:   false,
		MatchType: "none",
	}
}

// UpdateConfig updates the service mapper configuration
func (sm *ServiceMapper) UpdateConfig(cfg config.TelemetryConfig) error {
	sm.mu.Lock()
	defer sm.mu.Unlock()

	sm.config = cfg
	sm.patterns = nil

	// Recompile patterns
	for _, p := range cfg.ServicePatterns {
		re, err := regexp.Compile(p.Pattern)
		if err != nil {
			return err
		}
		sm.patterns = append(sm.patterns, &compiledPattern{
			regex:   re,
			replace: p.Repo,
		})
	}

	return nil
}

// GetMappedServices returns the count of services that can be mapped
func (sm *ServiceMapper) GetMappedServices() int {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return len(sm.config.ServiceMap)
}

// IsEnabled returns whether telemetry is enabled
func (sm *ServiceMapper) IsEnabled() bool {
	sm.mu.RLock()
	defer sm.mu.RUnlock()
	return sm.config.Enabled
}

// TestMapping tests if a service name would map to a repo (useful for CLI testing)
func (sm *ServiceMapper) TestMapping(serviceName string) (repoID string, matchType string, matched bool) {
	result := sm.Resolve(serviceName)
	return result.RepoID, result.MatchType, result.Matched
}
