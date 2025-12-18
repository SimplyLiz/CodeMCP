package federation

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
)

// ModuleResult represents a module from a federated query
type ModuleResult struct {
	RepoUID        string   `json:"repoUid"`
	RepoID         string   `json:"repoId"`
	ModuleID       string   `json:"moduleId"`
	Name           string   `json:"name"`
	Responsibility string   `json:"responsibility,omitempty"`
	OwnerRef       string   `json:"ownerRef,omitempty"`
	Tags           []string `json:"tags,omitempty"`
	Source         string   `json:"source,omitempty"`
	Confidence     float64  `json:"confidence,omitempty"`
}

// SearchModulesOptions contains options for module search
type SearchModulesOptions struct {
	// Query is the search query (FTS)
	Query string

	// RepoIDs filters to specific repos (empty = all)
	RepoIDs []string

	// Tags filters to modules with these tags
	Tags []string

	// Limit is the max number of results (default: 50)
	Limit int
}

// SearchModulesResult contains the results of a module search
type SearchModulesResult struct {
	Modules   []ModuleResult      `json:"modules"`
	Total     int                 `json:"total"`
	Staleness FederationStaleness `json:"staleness"`
}

// SearchModules searches for modules across the federation
func (f *Federation) SearchModules(opts SearchModulesOptions) (*SearchModulesResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	var args []interface{}
	var whereClauses []string

	// Build query
	baseQuery := `
		SELECT m.repo_uid, m.repo_id, m.module_id, m.name, m.responsibility,
		       m.owner_ref, m.tags, m.source, m.confidence
		FROM federated_modules m
	`

	// FTS search
	if opts.Query != "" {
		baseQuery = `
			SELECT m.repo_uid, m.repo_id, m.module_id, m.name, m.responsibility,
			       m.owner_ref, m.tags, m.source, m.confidence
			FROM federated_modules m
			JOIN federated_modules_fts fts ON m.id = fts.rowid
			WHERE federated_modules_fts MATCH ?
		`
		args = append(args, opts.Query)
	}

	// Filter by repos
	if len(opts.RepoIDs) > 0 {
		placeholders := make([]string, len(opts.RepoIDs))
		for i, id := range opts.RepoIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("m.repo_id IN (%s)", strings.Join(placeholders, ",")))
	}

	// Build WHERE clause
	query := baseQuery
	if len(whereClauses) > 0 {
		if opts.Query != "" {
			query += " AND " + strings.Join(whereClauses, " AND ")
		} else {
			query += " WHERE " + strings.Join(whereClauses, " AND ")
		}
	}

	query += fmt.Sprintf(" LIMIT %d", opts.Limit)

	rows, err := f.index.DB().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var modules []ModuleResult
	for rows.Next() {
		var m ModuleResult
		var responsibility, ownerRef, tags, source sql.NullString
		var confidence sql.NullFloat64

		if err := rows.Scan(&m.RepoUID, &m.RepoID, &m.ModuleID, &m.Name,
			&responsibility, &ownerRef, &tags, &source, &confidence); err != nil {
			continue
		}

		m.Responsibility = responsibility.String
		m.OwnerRef = ownerRef.String
		m.Source = source.String
		if confidence.Valid {
			m.Confidence = confidence.Float64
		}
		if tags.Valid && tags.String != "" {
			_ = json.Unmarshal([]byte(tags.String), &m.Tags)
		}

		modules = append(modules, m)
	}

	// Get staleness info
	staleness := f.computeStaleness()

	return &SearchModulesResult{
		Modules:   modules,
		Total:     len(modules),
		Staleness: staleness,
	}, nil
}

// OwnershipResult represents ownership from a federated query
type OwnershipResult struct {
	RepoUID    string   `json:"repoUid"`
	RepoID     string   `json:"repoId"`
	Pattern    string   `json:"pattern"`
	Owners     []Owner  `json:"owners"`
	Scope      string   `json:"scope"`
	Source     string   `json:"source"`
	Confidence float64  `json:"confidence"`
}

// Owner represents an owner entry
type Owner struct {
	Type   string  `json:"type"`   // user, team, email
	ID     string  `json:"id"`     // @username, @org/team, email
	Weight float64 `json:"weight"` // 0.0-1.0
}

// SearchOwnershipOptions contains options for ownership search
type SearchOwnershipOptions struct {
	// PathGlob is the glob pattern to match (e.g., "**/auth/**")
	PathGlob string

	// RepoIDs filters to specific repos (empty = all)
	RepoIDs []string

	// Limit is the max number of results (default: 50)
	Limit int
}

// SearchOwnershipResult contains the results of an ownership search
type SearchOwnershipResult struct {
	Matches   []OwnershipResult   `json:"matches"`
	Total     int                 `json:"total"`
	Staleness FederationStaleness `json:"staleness"`
}

// SearchOwnership searches for ownership across the federation
func (f *Federation) SearchOwnership(opts SearchOwnershipOptions) (*SearchOwnershipResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	var args []interface{}
	var whereClauses []string

	// Convert glob to SQL LIKE pattern
	if opts.PathGlob != "" {
		// Convert glob patterns to SQL LIKE
		// ** -> %
		// * -> %
		pattern := opts.PathGlob
		pattern = strings.ReplaceAll(pattern, "**", "%")
		pattern = strings.ReplaceAll(pattern, "*", "%")
		whereClauses = append(whereClauses, "pattern LIKE ?")
		args = append(args, pattern)
	}

	// Filter by repos
	if len(opts.RepoIDs) > 0 {
		placeholders := make([]string, len(opts.RepoIDs))
		for i, id := range opts.RepoIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("repo_id IN (%s)", strings.Join(placeholders, ",")))
	}

	query := "SELECT repo_uid, repo_id, pattern, owners, scope, source, confidence FROM federated_ownership"
	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	query += fmt.Sprintf(" LIMIT %d", opts.Limit)

	rows, err := f.index.DB().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var matches []OwnershipResult
	for rows.Next() {
		var o OwnershipResult
		var ownersJSON string

		if err := rows.Scan(&o.RepoUID, &o.RepoID, &o.Pattern, &ownersJSON, &o.Scope, &o.Source, &o.Confidence); err != nil {
			continue
		}

		if ownersJSON != "" {
			_ = json.Unmarshal([]byte(ownersJSON), &o.Owners)
		}

		matches = append(matches, o)
	}

	staleness := f.computeStaleness()

	return &SearchOwnershipResult{
		Matches:   matches,
		Total:     len(matches),
		Staleness: staleness,
	}, nil
}

// HotspotResult represents a hotspot from a federated query
type HotspotResult struct {
	RepoUID              string  `json:"repoUid"`
	RepoID               string  `json:"repoId"`
	TargetID             string  `json:"targetId"`
	TargetType           string  `json:"targetType"`
	Score                float64 `json:"score"`
	ChurnCommits30d      int     `json:"churnCommits30d,omitempty"`
	ComplexityCyclomatic float64 `json:"complexityCyclomatic,omitempty"`
	CouplingInstability  float64 `json:"couplingInstability,omitempty"`
}

// GetHotspotsOptions contains options for hotspot query
type GetHotspotsOptions struct {
	// Top is the number of top hotspots to return (default: 20)
	Top int

	// RepoIDs filters to specific repos (empty = all)
	RepoIDs []string

	// MinScore filters to hotspots above this score (default: 0.3)
	MinScore float64
}

// GetHotspotsResult contains the results of a hotspot query
type GetHotspotsResult struct {
	Hotspots  []HotspotResult     `json:"hotspots"`
	Total     int                 `json:"total"`
	Staleness FederationStaleness `json:"staleness"`
}

// GetHotspots returns merged hotspots across the federation
func (f *Federation) GetHotspots(opts GetHotspotsOptions) (*GetHotspotsResult, error) {
	if opts.Top <= 0 {
		opts.Top = 20
	}
	if opts.MinScore <= 0 {
		opts.MinScore = 0.3
	}

	var args []interface{}
	var whereClauses []string

	whereClauses = append(whereClauses, "score >= ?")
	args = append(args, opts.MinScore)

	// Filter by repos
	if len(opts.RepoIDs) > 0 {
		placeholders := make([]string, len(opts.RepoIDs))
		for i, id := range opts.RepoIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("repo_id IN (%s)", strings.Join(placeholders, ",")))
	}

	query := `
		SELECT repo_uid, repo_id, target_id, target_type, score,
		       churn_commits_30d, complexity_cyclomatic, coupling_instability
		FROM federated_hotspots
		WHERE ` + strings.Join(whereClauses, " AND ") + `
		ORDER BY score DESC
		LIMIT ?
	`
	args = append(args, opts.Top)

	rows, err := f.index.DB().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var hotspots []HotspotResult
	for rows.Next() {
		var h HotspotResult
		var churn sql.NullInt64
		var complexity, coupling sql.NullFloat64

		if err := rows.Scan(&h.RepoUID, &h.RepoID, &h.TargetID, &h.TargetType, &h.Score,
			&churn, &complexity, &coupling); err != nil {
			continue
		}

		if churn.Valid {
			h.ChurnCommits30d = int(churn.Int64)
		}
		if complexity.Valid {
			h.ComplexityCyclomatic = complexity.Float64
		}
		if coupling.Valid {
			h.CouplingInstability = coupling.Float64
		}

		hotspots = append(hotspots, h)
	}

	staleness := f.computeStaleness()

	return &GetHotspotsResult{
		Hotspots:  hotspots,
		Total:     len(hotspots),
		Staleness: staleness,
	}, nil
}

// DecisionResult represents a decision from a federated query
type DecisionResult struct {
	RepoUID         string   `json:"repoUid"`
	RepoID          string   `json:"repoId"`
	DecisionID      string   `json:"decisionId"`
	Title           string   `json:"title"`
	Status          string   `json:"status"`
	AffectedModules []string `json:"affectedModules,omitempty"`
	Author          string   `json:"author,omitempty"`
	CreatedAt       string   `json:"createdAt,omitempty"`
}

// SearchDecisionsOptions contains options for decision search
type SearchDecisionsOptions struct {
	// Query is the search query (FTS)
	Query string

	// Status filters by status (empty = all)
	Status []string

	// RepoIDs filters to specific repos (empty = all)
	RepoIDs []string

	// AffectedModule filters to decisions affecting this module
	AffectedModule string

	// Limit is the max number of results (default: 50)
	Limit int
}

// SearchDecisionsResult contains the results of a decision search
type SearchDecisionsResult struct {
	Decisions []DecisionResult    `json:"decisions"`
	Total     int                 `json:"total"`
	Staleness FederationStaleness `json:"staleness"`
}

// SearchDecisions searches for decisions across the federation
func (f *Federation) SearchDecisions(opts SearchDecisionsOptions) (*SearchDecisionsResult, error) {
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	var args []interface{}
	var whereClauses []string

	// Build query
	baseQuery := `
		SELECT d.repo_uid, d.repo_id, d.decision_id, d.title, d.status,
		       d.affected_modules, d.author, d.created_at
		FROM federated_decisions d
	`

	// FTS search
	if opts.Query != "" {
		baseQuery = `
			SELECT d.repo_uid, d.repo_id, d.decision_id, d.title, d.status,
			       d.affected_modules, d.author, d.created_at
			FROM federated_decisions d
			JOIN federated_decisions_fts fts ON d.id = fts.rowid
			WHERE federated_decisions_fts MATCH ?
		`
		args = append(args, opts.Query)
	}

	// Filter by status
	if len(opts.Status) > 0 {
		placeholders := make([]string, len(opts.Status))
		for i, s := range opts.Status {
			placeholders[i] = "?"
			args = append(args, s)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("d.status IN (%s)", strings.Join(placeholders, ",")))
	}

	// Filter by repos
	if len(opts.RepoIDs) > 0 {
		placeholders := make([]string, len(opts.RepoIDs))
		for i, id := range opts.RepoIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("d.repo_id IN (%s)", strings.Join(placeholders, ",")))
	}

	// Filter by affected module
	if opts.AffectedModule != "" {
		whereClauses = append(whereClauses, "d.affected_modules LIKE ?")
		args = append(args, "%"+opts.AffectedModule+"%")
	}

	// Build WHERE clause
	query := baseQuery
	if len(whereClauses) > 0 {
		if opts.Query != "" {
			query += " AND " + strings.Join(whereClauses, " AND ")
		} else {
			query += " WHERE " + strings.Join(whereClauses, " AND ")
		}
	}

	query += fmt.Sprintf(" ORDER BY d.created_at DESC LIMIT %d", opts.Limit)

	rows, err := f.index.DB().Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	var decisions []DecisionResult
	for rows.Next() {
		var d DecisionResult
		var affectedModules, author, createdAt sql.NullString

		if err := rows.Scan(&d.RepoUID, &d.RepoID, &d.DecisionID, &d.Title, &d.Status,
			&affectedModules, &author, &createdAt); err != nil {
			continue
		}

		d.Author = author.String
		d.CreatedAt = createdAt.String
		if affectedModules.Valid && affectedModules.String != "" {
			_ = json.Unmarshal([]byte(affectedModules.String), &d.AffectedModules)
		}

		decisions = append(decisions, d)
	}

	staleness := f.computeStaleness()

	return &SearchDecisionsResult{
		Decisions: decisions,
		Total:     len(decisions),
		Staleness: staleness,
	}, nil
}

// computeStaleness computes federation-wide staleness
func (f *Federation) computeStaleness() FederationStaleness {
	repos, err := f.index.ListRepos()
	if err != nil {
		return FederationStaleness{
			OverallStaleness:   StalenessLevelObsolete,
			RefreshRecommended: true,
		}
	}

	var repoStaleness []RepoStaleness
	for _, repo := range repos {
		// For now, compute staleness based on sync time only (no commit counting)
		info := ComputeStaleness(repo.LastSyncedAt, 0)
		repoStaleness = append(repoStaleness, RepoStaleness{
			RepoID:    repo.RepoID,
			Staleness: info,
		})
	}

	return ComputeFederationStaleness(repoStaleness)
}
