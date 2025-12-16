package backends

import (
	"ckb/internal/config"
)

// QueryPolicy defines how backends should be queried and results merged
type QueryPolicy struct {
	// BackendPreferenceOrder defines the order in which backends are preferred
	// Example: [scip, glean, lsp] means try SCIP first, then Glean, then LSP
	BackendPreferenceOrder []BackendID

	// AlwaysUse lists backends that should always be queried regardless of merge mode
	// Example: [git] means always query git for blame/history information
	AlwaysUse []BackendID

	// MaxInFlightPerBackend limits concurrent requests to each backend
	MaxInFlightPerBackend map[BackendID]int

	// CoalesceWindowMs is the time window for coalescing duplicate requests
	CoalesceWindowMs int

	// MergeMode specifies how to merge results from multiple backends
	MergeMode MergeMode

	// SupplementThreshold is the minimum completeness score to trigger supplementation
	// If primary backend score < threshold, supplement from other backends
	SupplementThreshold float64

	// TimeoutMs defines timeout per backend
	TimeoutMs map[BackendID]int
}

// DefaultQueryPolicy returns the default query policy
func DefaultQueryPolicy() *QueryPolicy {
	return &QueryPolicy{
		BackendPreferenceOrder: []BackendID{BackendSCIP, BackendGlean, BackendLSP},
		AlwaysUse:              []BackendID{BackendGit},
		MaxInFlightPerBackend: map[BackendID]int{
			BackendSCIP:  10,
			BackendGlean: 10,
			BackendLSP:   3,
			BackendGit:   5,
		},
		CoalesceWindowMs:    50,
		MergeMode:           MergeModePreferFirst,
		SupplementThreshold: 0.8,
		TimeoutMs: map[BackendID]int{
			BackendSCIP:  5000,
			BackendGlean: 5000,
			BackendLSP:   15000,
			BackendGit:   5000,
		},
	}
}

// LoadQueryPolicy creates a QueryPolicy from config
func LoadQueryPolicy(cfg *config.Config) *QueryPolicy {
	policy := DefaultQueryPolicy()

	// Convert string backend IDs to BackendID type
	if len(cfg.QueryPolicy.BackendPreferenceOrder) > 0 {
		policy.BackendPreferenceOrder = make([]BackendID, 0, len(cfg.QueryPolicy.BackendPreferenceOrder))
		for _, id := range cfg.QueryPolicy.BackendPreferenceOrder {
			policy.BackendPreferenceOrder = append(policy.BackendPreferenceOrder, BackendID(id))
		}
	}

	if len(cfg.QueryPolicy.AlwaysUse) > 0 {
		policy.AlwaysUse = make([]BackendID, 0, len(cfg.QueryPolicy.AlwaysUse))
		for _, id := range cfg.QueryPolicy.AlwaysUse {
			policy.AlwaysUse = append(policy.AlwaysUse, BackendID(id))
		}
	}

	// Convert string-keyed maps to BackendID-keyed maps
	if len(cfg.QueryPolicy.MaxInFlightPerBackend) > 0 {
		policy.MaxInFlightPerBackend = make(map[BackendID]int)
		for k, v := range cfg.QueryPolicy.MaxInFlightPerBackend {
			policy.MaxInFlightPerBackend[BackendID(k)] = v
		}
	}

	if len(cfg.QueryPolicy.TimeoutMs) > 0 {
		policy.TimeoutMs = make(map[BackendID]int)
		for k, v := range cfg.QueryPolicy.TimeoutMs {
			policy.TimeoutMs[BackendID(k)] = v
		}
	}

	if cfg.QueryPolicy.CoalesceWindowMs > 0 {
		policy.CoalesceWindowMs = cfg.QueryPolicy.CoalesceWindowMs
	}

	if cfg.QueryPolicy.MergeMode != "" {
		policy.MergeMode = MergeMode(cfg.QueryPolicy.MergeMode)
	}

	if cfg.QueryPolicy.SupplementThreshold > 0 {
		policy.SupplementThreshold = cfg.QueryPolicy.SupplementThreshold
	}

	return policy
}

// GetMaxInFlight returns the max in-flight requests for a backend
func (p *QueryPolicy) GetMaxInFlight(backendID BackendID) int {
	if max, ok := p.MaxInFlightPerBackend[backendID]; ok {
		return max
	}
	// Default to 5 if not specified
	return 5
}

// GetTimeout returns the timeout for a backend in milliseconds
func (p *QueryPolicy) GetTimeout(backendID BackendID) int {
	if timeout, ok := p.TimeoutMs[backendID]; ok {
		return timeout
	}
	// Default to 10 seconds if not specified
	return 10000
}

// ShouldAlwaysUse checks if a backend should always be queried
func (p *QueryPolicy) ShouldAlwaysUse(backendID BackendID) bool {
	for _, id := range p.AlwaysUse {
		if id == backendID {
			return true
		}
	}
	return false
}

// GetBackendPriority returns the priority of a backend based on preference order
// Lower number = higher priority (first in list = 1)
func (p *QueryPolicy) GetBackendPriority(backendID BackendID) int {
	for i, id := range p.BackendPreferenceOrder {
		if id == backendID {
			return i + 1
		}
	}
	// Backends not in preference order get low priority
	return 100
}
