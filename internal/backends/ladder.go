package backends

import (
	"context"

	"ckb/internal/logging"
)

// BackendLadder implements the backend selection ladder
// It selects backends based on preference order and availability
type BackendLadder struct {
	policy *QueryPolicy
	logger *logging.Logger
}

// NewBackendLadder creates a new backend ladder
func NewBackendLadder(policy *QueryPolicy, logger *logging.Logger) *BackendLadder {
	return &BackendLadder{
		policy: policy,
		logger: logger,
	}
}

// SelectBackends selects which backends to query based on merge mode and availability
func (l *BackendLadder) SelectBackends(
	ctx context.Context,
	availableBackends map[BackendID]Backend,
	req QueryRequest,
) []BackendID {
	selected := []BackendID{}

	// Add backends that should always be used
	for _, backendID := range l.policy.AlwaysUse {
		if backend, ok := availableBackends[backendID]; ok && backend.IsAvailable() {
			selected = append(selected, backendID)
			l.logger.Debug("Selected always-use backend", map[string]interface{}{
				"backend": backendID,
			})
		}
	}

	// Determine primary backends based on merge mode
	switch l.policy.MergeMode {
	case MergeModePreferFirst:
		// Select first available backend from preference order
		primary := l.selectPrimaryBackend(availableBackends, req)
		if primary != "" && !contains(selected, primary) {
			selected = append(selected, primary)
			l.logger.Debug("Selected primary backend", map[string]interface{}{
				"backend":   primary,
				"mergeMode": l.policy.MergeMode,
			})
		}

	case MergeModeUnion:
		// Select all available backends from preference order
		for _, backendID := range l.policy.BackendPreferenceOrder {
			if backend, ok := availableBackends[backendID]; ok && backend.IsAvailable() {
				if !contains(selected, backendID) {
					selected = append(selected, backendID)
					l.logger.Debug("Selected backend for union", map[string]interface{}{
						"backend": backendID,
					})
				}
			}
		}
	}

	if len(selected) == 0 {
		l.logger.Warn("No backends selected", map[string]interface{}{
			"queryType":      req.Type,
			"availableCount": len(availableBackends),
		})
	}

	return selected
}

// selectPrimaryBackend selects the highest-priority available backend
func (l *BackendLadder) selectPrimaryBackend(
	availableBackends map[BackendID]Backend,
	req QueryRequest,
) BackendID {
	// Try backends in preference order
	for _, backendID := range l.policy.BackendPreferenceOrder {
		backend, ok := availableBackends[backendID]
		if !ok {
			continue
		}

		// Check if backend is available
		if !backend.IsAvailable() {
			l.logger.Debug("Backend not available", map[string]interface{}{
				"backend": backendID,
			})
			continue
		}

		// Check if backend supports the query type
		if !l.supportsQueryType(backend, req.Type) {
			l.logger.Debug("Backend doesn't support query type", map[string]interface{}{
				"backend":   backendID,
				"queryType": req.Type,
			})
			continue
		}

		// Found a suitable backend
		return backendID
	}

	return ""
}

// SelectSupplementBackends selects backends to supplement a primary result
// Only backends with equal or higher precedence than primary are considered
func (l *BackendLadder) SelectSupplementBackends(
	ctx context.Context,
	availableBackends map[BackendID]Backend,
	primaryBackend BackendID,
	primaryCompleteness CompletenessInfo,
	req QueryRequest,
) []BackendID {
	// Don't supplement if completeness is high enough
	if primaryCompleteness.Score >= l.policy.SupplementThreshold {
		return []BackendID{}
	}

	selected := []BackendID{}
	primaryPriority := l.policy.GetBackendPriority(primaryBackend)

	// Consider backends with equal or higher precedence
	for _, backendID := range l.policy.BackendPreferenceOrder {
		// Skip the primary backend itself
		if backendID == primaryBackend {
			continue
		}

		// Only consider equal or higher precedence
		if l.policy.GetBackendPriority(backendID) > primaryPriority {
			break
		}

		backend, ok := availableBackends[backendID]
		if !ok || !backend.IsAvailable() {
			continue
		}

		if !l.supportsQueryType(backend, req.Type) {
			continue
		}

		selected = append(selected, backendID)
		l.logger.Debug("Selected supplement backend", map[string]interface{}{
			"backend":             backendID,
			"primaryBackend":      primaryBackend,
			"primaryCompleteness": primaryCompleteness.Score,
		})
	}

	return selected
}

// FallbackToNext selects the next available backend after a failure
func (l *BackendLadder) FallbackToNext(
	ctx context.Context,
	availableBackends map[BackendID]Backend,
	failedBackends []BackendID,
	req QueryRequest,
) BackendID {
	// Try backends in preference order, skipping failed ones
	for _, backendID := range l.policy.BackendPreferenceOrder {
		// Skip if already failed
		if contains(failedBackends, backendID) {
			continue
		}

		backend, ok := availableBackends[backendID]
		if !ok || !backend.IsAvailable() {
			continue
		}

		if !l.supportsQueryType(backend, req.Type) {
			continue
		}

		l.logger.Info("Falling back to backend", map[string]interface{}{
			"backend":        backendID,
			"failedBackends": failedBackends,
		})

		return backendID
	}

	l.logger.Warn("No fallback backend available", map[string]interface{}{
		"failedBackends": failedBackends,
		"queryType":      req.Type,
	})

	return ""
}

// supportsQueryType checks if a backend supports a given query type
func (l *BackendLadder) supportsQueryType(backend Backend, queryType QueryType) bool {
	capabilities := backend.Capabilities()

	switch queryType {
	case QueryTypeSymbol:
		return containsCapability(capabilities, "symbol-info") ||
			containsCapability(capabilities, "goto-definition")
	case QueryTypeSearch:
		return containsCapability(capabilities, "symbol-search") ||
			containsCapability(capabilities, "workspace-symbols")
	case QueryTypeReferences:
		return containsCapability(capabilities, "find-references")
	default:
		return false
	}
}

// contains checks if a slice contains a backend ID
func contains(slice []BackendID, item BackendID) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// containsCapability checks if a capability list contains a specific capability
func containsCapability(capabilities []string, capability string) bool {
	for _, cap := range capabilities {
		if cap == capability {
			return true
		}
	}
	return false
}
