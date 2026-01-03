package identity

import (
	"fmt"
	"log/slog"
	"strings"
	"time"

	"ckb/internal/storage"
)

// FuzzyMatchResult represents the result of a fuzzy match attempt
type FuzzyMatchResult struct {
	Mapping    *SymbolMapping
	Confidence float64
}

// AliasCreator handles creating aliases and tombstones during refresh
type AliasCreator struct {
	db     *storage.DB
	logger *slog.Logger
}

// NewAliasCreator creates a new alias creator
func NewAliasCreator(db *storage.DB, logger *slog.Logger) *AliasCreator {
	return &AliasCreator{
		db:     db,
		logger: logger,
	}
}

// CreateAliasesOnRefresh compares old and new mappings and creates aliases or tombstones
// Section 4.6: Alias creation strategy
func (c *AliasCreator) CreateAliasesOnRefresh(
	oldMappings []*SymbolMapping,
	newMappings []*SymbolMapping,
	repoStateId string,
) error {
	// Index new mappings by stable ID and backend ID for fast lookup
	newByStableId := make(map[string]*SymbolMapping)
	newByBackendId := make(map[string]*SymbolMapping)

	for _, m := range newMappings {
		newByStableId[m.StableId] = m
		if m.BackendStableId != "" {
			newByBackendId[m.BackendStableId] = m
		}
	}

	c.logger.Debug("analyzing symbol changes", map[string]interface{}{
		"old_count":  len(oldMappings),
		"new_count":  len(newMappings),
		"repo_state": repoStateId,
	})

	aliasCount := 0
	tombstoneCount := 0
	now := time.Now().UTC().Format(time.RFC3339)

	for _, old := range oldMappings {
		// Skip if symbol still exists with same stable ID
		if _, exists := newByStableId[old.StableId]; exists {
			continue
		}

		// Strategy 1: Backend ID match (high confidence)
		// If the backend ID still exists but with a different stable ID,
		// this is likely a rename or signature change
		if old.BackendStableId != "" {
			if newSymbol, exists := newByBackendId[old.BackendStableId]; exists {
				if newSymbol.StableId != old.StableId {
					// Backend ID exists but stable ID changed - create high-confidence alias
					alias := &SymbolAlias{
						OldStableId:    old.StableId,
						NewStableId:    newSymbol.StableId,
						Reason:         ReasonRenamed,
						Confidence:     0.95,
						CreatedAt:      now,
						CreatedStateId: repoStateId,
					}

					if err := c.createAlias(alias); err != nil {
						c.logger.Error("failed to create alias", map[string]interface{}{
							"old_id": old.StableId,
							"new_id": newSymbol.StableId,
							"error":  err.Error(),
						})
						return fmt.Errorf("failed to create alias: %w", err)
					}

					// Mark old symbol as deleted since it was renamed
					repo := NewSymbolRepository(c.db, c.logger)
					if err := repo.MarkDeleted(old.StableId, repoStateId); err != nil {
						c.logger.Warn("failed to mark renamed symbol as deleted", map[string]interface{}{
							"stable_id": old.StableId,
							"error":     err.Error(),
						})
					}

					c.logger.Info("created alias (backend ID match)", map[string]interface{}{
						"old_id":     old.StableId,
						"new_id":     newSymbol.StableId,
						"backend_id": old.BackendStableId,
					})

					aliasCount++
					continue
				}
			}
		}

		// Strategy 2: Fuzzy match (lower confidence)
		// Try to match based on name, kind, and location similarity
		fuzzyMatch, err := c.FindFuzzyMatch(old, newMappings)
		if err == nil && fuzzyMatch != nil && fuzzyMatch.Confidence >= 0.6 {
			alias := &SymbolAlias{
				OldStableId:    old.StableId,
				NewStableId:    fuzzyMatch.Mapping.StableId,
				Reason:         ReasonFuzzyMatch,
				Confidence:     fuzzyMatch.Confidence,
				CreatedAt:      now,
				CreatedStateId: repoStateId,
			}

			if err := c.createAlias(alias); err != nil {
				c.logger.Error("failed to create fuzzy alias", map[string]interface{}{
					"old_id":     old.StableId,
					"new_id":     fuzzyMatch.Mapping.StableId,
					"confidence": fuzzyMatch.Confidence,
					"error":      err.Error(),
				})
				return fmt.Errorf("failed to create fuzzy alias: %w", err)
			}

			// Mark old symbol as deleted since it was matched
			repo := NewSymbolRepository(c.db, c.logger)
			if err := repo.MarkDeleted(old.StableId, repoStateId); err != nil {
				c.logger.Warn("failed to mark fuzzy-matched symbol as deleted", map[string]interface{}{
					"stable_id": old.StableId,
					"error":     err.Error(),
				})
			}

			c.logger.Info("created alias (fuzzy match)", map[string]interface{}{
				"old_id":     old.StableId,
				"new_id":     fuzzyMatch.Mapping.StableId,
				"confidence": fuzzyMatch.Confidence,
			})

			aliasCount++
			continue
		}

		// No match found - mark as deleted (tombstone)
		repo := NewSymbolRepository(c.db, c.logger)
		if err := repo.MarkDeleted(old.StableId, repoStateId); err != nil {
			c.logger.Error("failed to mark symbol as deleted", map[string]interface{}{
				"stable_id": old.StableId,
				"error":     err.Error(),
			})
			return fmt.Errorf("failed to mark symbol as deleted: %w", err)
		}

		c.logger.Debug("marked symbol as deleted", map[string]interface{}{
			"stable_id": old.StableId,
		})

		tombstoneCount++
	}

	c.logger.Info("alias refresh complete", map[string]interface{}{
		"aliases_created":    aliasCount,
		"tombstones_created": tombstoneCount,
		"repo_state":         repoStateId,
	})

	return nil
}

// FindFuzzyMatch attempts to match a disappeared symbol to a new one
func (c *AliasCreator) FindFuzzyMatch(
	old *SymbolMapping,
	newMappings []*SymbolMapping,
) (*FuzzyMatchResult, error) {
	if old == nil || old.Fingerprint == nil {
		return nil, fmt.Errorf("invalid old mapping")
	}

	var bestMatch *SymbolMapping
	var bestConfidence float64

	for _, newMapping := range newMappings {
		if newMapping.Fingerprint == nil {
			continue
		}

		confidence := c.computeSimilarity(old, newMapping)
		if confidence > bestConfidence {
			bestConfidence = confidence
			bestMatch = newMapping
		}
	}

	if bestMatch == nil || bestConfidence < 0.6 {
		return nil, nil
	}

	return &FuzzyMatchResult{
		Mapping:    bestMatch,
		Confidence: bestConfidence,
	}, nil
}

// computeSimilarity computes a similarity score between two symbol mappings
func (c *AliasCreator) computeSimilarity(old, new *SymbolMapping) float64 {
	if old.Fingerprint == nil || new.Fingerprint == nil {
		return 0.0
	}

	score := 0.0
	weights := 0.0

	// Same kind (weight: 0.3)
	if old.Fingerprint.Kind == new.Fingerprint.Kind {
		score += 0.3
	}
	weights += 0.3

	// Same name (weight: 0.4)
	if old.Fingerprint.Name == new.Fingerprint.Name {
		score += 0.4
	} else if c.namesSimilar(old.Fingerprint.Name, new.Fingerprint.Name) {
		score += 0.2 // Partial credit for similar names
	}
	weights += 0.4

	// Same or similar container (weight: 0.2)
	if old.Fingerprint.QualifiedContainer == new.Fingerprint.QualifiedContainer {
		score += 0.2
	} else if c.containersSimilar(old.Fingerprint.QualifiedContainer, new.Fingerprint.QualifiedContainer) {
		score += 0.1
	}
	weights += 0.2

	// Similar location (weight: 0.1)
	if old.Location != nil && new.Location != nil {
		if old.Location.Path == new.Location.Path {
			score += 0.1
		} else if c.pathsSimilar(old.Location.Path, new.Location.Path) {
			score += 0.05
		}
	}
	weights += 0.1

	// Normalize by total weights
	if weights > 0 {
		return score / weights
	}

	return 0.0
}

// namesSimilar checks if two names are similar (case-insensitive, ignoring underscores)
func (c *AliasCreator) namesSimilar(name1, name2 string) bool {
	normalize := func(s string) string {
		return strings.ToLower(strings.ReplaceAll(s, "_", ""))
	}
	return normalize(name1) == normalize(name2)
}

// containersSimilar checks if two containers are similar
func (c *AliasCreator) containersSimilar(container1, container2 string) bool {
	// Simple prefix match (same parent namespace)
	if container1 == "" || container2 == "" {
		return false
	}

	parts1 := strings.Split(container1, ".")
	parts2 := strings.Split(container2, ".")

	// Check if they share at least one parent level
	minLen := len(parts1)
	if len(parts2) < minLen {
		minLen = len(parts2)
	}

	if minLen == 0 {
		return false
	}

	// Check if first part matches (root namespace)
	return parts1[0] == parts2[0]
}

// pathsSimilar checks if two file paths are similar
func (c *AliasCreator) pathsSimilar(path1, path2 string) bool {
	// Same directory
	dir1 := strings.TrimSuffix(path1, getFileName(path1))
	dir2 := strings.TrimSuffix(path2, getFileName(path2))
	return dir1 == dir2
}

// getFileName extracts the file name from a path
func getFileName(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

// createAlias inserts an alias into the database
func (c *AliasCreator) createAlias(alias *SymbolAlias) error {
	if err := alias.Validate(); err != nil {
		return fmt.Errorf("invalid alias: %w", err)
	}

	query := `
		INSERT INTO symbol_aliases (
			old_stable_id, new_stable_id, reason, confidence,
			created_at, created_state_id
		) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (old_stable_id, new_stable_id) DO NOTHING
	`

	_, err := c.db.Exec(
		query,
		alias.OldStableId,
		alias.NewStableId,
		alias.Reason,
		alias.Confidence,
		alias.CreatedAt,
		alias.CreatedStateId,
	)

	if err != nil {
		return fmt.Errorf("failed to insert alias: %w", err)
	}

	return nil
}
