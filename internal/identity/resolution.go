package identity

import (
	"fmt"

	"ckb/internal/errors"
	"ckb/internal/logging"
	"ckb/internal/storage"
)

// AliasChainMaxDepth is the maximum depth for alias chain resolution
// Section 4.5: Alias resolution
const AliasChainMaxDepth = 3

// ResolvedSymbol represents the result of resolving a symbol ID through alias chains
type ResolvedSymbol struct {
	Symbol             *SymbolMapping `json:"symbol,omitempty"`             // The resolved symbol (nil if not found/deleted)
	Redirected         bool           `json:"redirected,omitempty"`         // True if we followed an alias
	RedirectedFrom     string         `json:"redirectedFrom,omitempty"`     // Original requested ID
	RedirectReason     AliasReason    `json:"redirectReason,omitempty"`     // Why the redirect occurred
	RedirectConfidence float64        `json:"redirectConfidence,omitempty"` // Confidence in the redirect
	Deleted            bool           `json:"deleted,omitempty"`            // True if the symbol was deleted
	DeletedAt          string         `json:"deletedAt,omitempty"`          // When it was deleted
	Error              string         `json:"error,omitempty"`              // Error message if resolution failed
}

// IdentityResolver handles symbol ID resolution with alias following
type IdentityResolver struct {
	db     *storage.DB
	logger *logging.Logger
}

// NewIdentityResolver creates a new identity resolver
func NewIdentityResolver(db *storage.DB, logger *logging.Logger) *IdentityResolver {
	return &IdentityResolver{
		db:     db,
		logger: logger,
	}
}

// ResolveSymbolId follows alias chains to find the current symbol
// Returns the resolved symbol or an error/tombstone response
func (r *IdentityResolver) ResolveSymbolId(requestedId string) (*ResolvedSymbol, error) {
	return r.resolveWithDepth(requestedId, 0, make(map[string]bool))
}

// resolveWithDepth is the internal recursive resolution function
func (r *IdentityResolver) resolveWithDepth(
	requestedId string,
	depth int,
	visited map[string]bool,
) (*ResolvedSymbol, error) {
	// Cycle detection
	if visited[requestedId] {
		r.logger.Error("alias cycle detected", map[string]interface{}{
			"requested_id": requestedId,
			"visited":      visited,
		})
		return &ResolvedSymbol{
			Error: string(errors.AliasCycle),
		}, errors.NewCkbError(
			errors.AliasCycle,
			fmt.Sprintf("circular alias chain detected for symbol %s", requestedId),
			nil,
			nil,
			nil,
		)
	}
	visited[requestedId] = true

	// Max depth check
	if depth > AliasChainMaxDepth {
		r.logger.Warn("alias chain too deep", map[string]interface{}{
			"requested_id": requestedId,
			"depth":        depth,
			"max_depth":    AliasChainMaxDepth,
		})
		return &ResolvedSymbol{
			Error: string(errors.AliasChainTooDeep),
		}, errors.NewCkbError(
			errors.AliasChainTooDeep,
			fmt.Sprintf("alias chain exceeds maximum depth of %d for symbol %s", AliasChainMaxDepth, requestedId),
			nil,
			nil,
			nil,
		)
	}

	// Try direct lookup first
	repo := NewSymbolRepository(r.db, r.logger)
	symbol, err := repo.Get(requestedId)
	if err == nil && symbol != nil {
		// Active symbol found - return directly
		if symbol.IsActive() {
			return &ResolvedSymbol{
				Symbol: symbol,
			}, nil
		}

		// Check if deleted - but first check for aliases (renamed symbols have aliases)
		if symbol.IsDeleted() {
			// Check for alias before returning deleted status
			alias, aliasErr := r.getAlias(requestedId)
			if aliasErr == nil && alias != nil {
				// Follow the alias
				r.logger.Debug("following alias from deleted symbol", map[string]interface{}{
					"from":       requestedId,
					"to":         alias.NewStableId,
					"reason":     alias.Reason,
					"confidence": alias.Confidence,
					"depth":      depth,
				})

				resolved, resolveErr := r.resolveWithDepth(alias.NewStableId, depth+1, visited)
				if resolveErr != nil {
					return resolved, resolveErr
				}

				// Mark as redirected
				if resolved.Symbol != nil || resolved.Deleted {
					if !resolved.Redirected {
						resolved.Redirected = true
						resolved.RedirectedFrom = requestedId
						resolved.RedirectReason = alias.Reason
						resolved.RedirectConfidence = alias.Confidence
					}
				}
				return resolved, nil
			}

			// No alias - symbol is truly deleted
			r.logger.Debug("symbol is deleted", map[string]interface{}{
				"stable_id":  requestedId,
				"deleted_at": symbol.DeletedAt,
			})
			return &ResolvedSymbol{
				Deleted:   true,
				DeletedAt: symbol.DeletedAt,
			}, nil
		}
	}

	// No direct match - check for aliases
	alias, err := r.getAlias(requestedId)
	if err != nil {
		// No alias found either
		r.logger.Debug("symbol not found", map[string]interface{}{
			"stable_id": requestedId,
		})
		return &ResolvedSymbol{
			Error: string(errors.SymbolNotFound),
		}, errors.NewCkbError(
			errors.SymbolNotFound,
			fmt.Sprintf("symbol not found: %s", requestedId),
			nil,
			nil,
			nil,
		)
	}

	// Follow the alias recursively
	r.logger.Debug("following alias", map[string]interface{}{
		"from":       requestedId,
		"to":         alias.NewStableId,
		"reason":     alias.Reason,
		"confidence": alias.Confidence,
		"depth":      depth,
	})

	resolved, err := r.resolveWithDepth(alias.NewStableId, depth+1, visited)
	if err != nil {
		return resolved, err
	}

	// If we successfully resolved through an alias, mark it as redirected
	if resolved.Symbol != nil || resolved.Deleted {
		if !resolved.Redirected {
			// First redirect in the chain
			resolved.Redirected = true
			resolved.RedirectedFrom = requestedId
			resolved.RedirectReason = alias.Reason
			resolved.RedirectConfidence = alias.Confidence
		}
	}

	return resolved, nil
}

// getAlias retrieves an alias by old stable ID
func (r *IdentityResolver) getAlias(oldStableId string) (*SymbolAlias, error) {
	query := `
		SELECT old_stable_id, new_stable_id, reason, confidence, created_at, created_state_id
		FROM symbol_aliases
		WHERE old_stable_id = ?
		LIMIT 1
	`

	row := r.db.QueryRow(query, oldStableId)

	var alias SymbolAlias
	err := row.Scan(
		&alias.OldStableId,
		&alias.NewStableId,
		&alias.Reason,
		&alias.Confidence,
		&alias.CreatedAt,
		&alias.CreatedStateId,
	)

	if err != nil {
		return nil, fmt.Errorf("alias not found: %w", err)
	}

	return &alias, nil
}
