package mcp

import (
	"io"
	"log/slog"

	"ckb/internal/envelope"
	"ckb/internal/errors"
	"ckb/internal/federation"
)

// v6.2 Federation tool implementations

// toolListFederations lists all federations
func (s *MCPServer) toolListFederations(params map[string]interface{}) (*envelope.Response, error) {
	s.logger.Debug("Executing listFederations", map[string]interface{}{
		"params": params,
	})

	names, err := federation.List()
	if err != nil {
		return nil, errors.NewOperationError("list federations", err)
	}

	return NewToolResponse().
		Data(map[string]interface{}{
			"federations": names,
			"count":       len(names),
		}).
		CrossRepo().
		Build(), nil
}

// toolFederationStatus gets federation status
func (s *MCPServer) toolFederationStatus(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	s.logger.Debug("Executing federationStatus", map[string]interface{}{
		"federation": fedName,
	})

	// Check existence
	exists, err := federation.Exists(fedName)
	if err != nil {
		return nil, errors.NewOperationError("check federation", err)
	}
	if !exists {
		return nil, errors.NewResourceNotFoundError("federation", fedName)
	}

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	config := fed.Config()
	repos := fed.ListRepos()

	// Get indexed repos
	indexedRepos, _ := fed.Index().ListRepos()

	// Check compatibility
	var compatible, incompatible int
	checks, err := federation.CheckAllReposCompatibility(fed)
	if err == nil {
		for _, c := range checks {
			if c.Status == federation.CompatibilityOK {
				compatible++
			} else {
				incompatible++
			}
		}
	}

	result := map[string]interface{}{
		"name":        config.Name,
		"description": config.Description,
		"createdAt":   config.CreatedAt,
		"updatedAt":   config.UpdatedAt,
		"repoCount":   len(repos),
		"repos":       repos,
		"compatibility": map[string]int{
			"compatible":   compatible,
			"incompatible": incompatible,
		},
	}

	if len(indexedRepos) > 0 {
		result["indexedRepos"] = indexedRepos
	}

	return NewToolResponse().
		Data(result).
		CrossRepo().
		Build(), nil
}

// toolFederationRepos lists repos in a federation
func (s *MCPServer) toolFederationRepos(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	includeCompat, _ := params["includeCompatibility"].(bool)

	s.logger.Debug("Executing federationRepos", map[string]interface{}{
		"federation":           fedName,
		"includeCompatibility": includeCompat,
	})

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	repos := fed.ListRepos()

	result := map[string]interface{}{
		"repos": repos,
		"count": len(repos),
	}

	if includeCompat {
		checks, compatErr := federation.CheckAllReposCompatibility(fed)
		if compatErr == nil {
			result["compatibility"] = checks
		}
	}

	return NewToolResponse().
		Data(result).
		CrossRepo().
		Build(), nil
}

// toolFederationSearchModules searches modules across federation
func (s *MCPServer) toolFederationSearchModules(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	query, _ := params["query"].(string)
	limit := 50
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	s.logger.Debug("Executing federationSearchModules", map[string]interface{}{
		"federation": fedName,
		"query":      query,
		"limit":      limit,
	})

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	opts := federation.SearchModulesOptions{
		Query: query,
		Limit: limit,
	}

	// Parse repo IDs
	if reposRaw, ok := params["repos"].([]interface{}); ok {
		for _, r := range reposRaw {
			if s, ok := r.(string); ok {
				opts.RepoIDs = append(opts.RepoIDs, s)
			}
		}
	}

	// Parse tags
	if tagsRaw, ok := params["tags"].([]interface{}); ok {
		for _, t := range tagsRaw {
			if s, ok := t.(string); ok {
				opts.Tags = append(opts.Tags, s)
			}
		}
	}

	result, err := fed.SearchModules(opts)
	if err != nil {
		return nil, errors.NewOperationError("search modules", err)
	}

	return NewToolResponse().
		Data(result).
		CrossRepo().
		Build(), nil
}

// toolFederationSearchOwnership searches ownership across federation
func (s *MCPServer) toolFederationSearchOwnership(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	pathGlob, _ := params["path"].(string)
	limit := 50
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	s.logger.Debug("Executing federationSearchOwnership", map[string]interface{}{
		"federation": fedName,
		"path":       pathGlob,
		"limit":      limit,
	})

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	opts := federation.SearchOwnershipOptions{
		PathGlob: pathGlob,
		Limit:    limit,
	}

	// Parse repo IDs
	if reposRaw, ok := params["repos"].([]interface{}); ok {
		for _, r := range reposRaw {
			if s, ok := r.(string); ok {
				opts.RepoIDs = append(opts.RepoIDs, s)
			}
		}
	}

	result, err := fed.SearchOwnership(opts)
	if err != nil {
		return nil, errors.NewOperationError("search ownership", err)
	}

	return NewToolResponse().
		Data(result).
		CrossRepo().
		Build(), nil
}

// toolFederationGetHotspots gets hotspots across federation
func (s *MCPServer) toolFederationGetHotspots(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	top := 20
	if t, ok := params["top"].(float64); ok {
		top = int(t)
	}
	minScore := 0.3
	if m, ok := params["minScore"].(float64); ok {
		minScore = m
	}

	s.logger.Debug("Executing federationGetHotspots", map[string]interface{}{
		"federation": fedName,
		"top":        top,
		"minScore":   minScore,
	})

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	opts := federation.GetHotspotsOptions{
		Top:      top,
		MinScore: minScore,
	}

	// Parse repo IDs
	if reposRaw, ok := params["repos"].([]interface{}); ok {
		for _, r := range reposRaw {
			if s, ok := r.(string); ok {
				opts.RepoIDs = append(opts.RepoIDs, s)
			}
		}
	}

	result, err := fed.GetHotspots(opts)
	if err != nil {
		return nil, errors.NewOperationError("get hotspots", err)
	}

	return NewToolResponse().
		Data(result).
		CrossRepo().
		Build(), nil
}

// toolFederationSearchDecisions searches decisions across federation
func (s *MCPServer) toolFederationSearchDecisions(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	query, _ := params["query"].(string)
	affectedModule, _ := params["module"].(string)
	limit := 50
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	s.logger.Debug("Executing federationSearchDecisions", map[string]interface{}{
		"federation": fedName,
		"query":      query,
		"module":     affectedModule,
		"limit":      limit,
	})

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	opts := federation.SearchDecisionsOptions{
		Query:          query,
		AffectedModule: affectedModule,
		Limit:          limit,
	}

	// Parse repo IDs
	if reposRaw, ok := params["repos"].([]interface{}); ok {
		for _, r := range reposRaw {
			if s, ok := r.(string); ok {
				opts.RepoIDs = append(opts.RepoIDs, s)
			}
		}
	}

	// Parse status filter
	if statusRaw, ok := params["status"].([]interface{}); ok {
		for _, s := range statusRaw {
			if str, ok := s.(string); ok {
				opts.Status = append(opts.Status, str)
			}
		}
	}

	result, err := fed.SearchDecisions(opts)
	if err != nil {
		return nil, errors.NewOperationError("search decisions", err)
	}

	return NewToolResponse().
		Data(result).
		CrossRepo().
		Build(), nil
}

// toolFederationSync syncs federation index
func (s *MCPServer) toolFederationSync(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	force, _ := params["force"].(bool)
	dryRun, _ := params["dryRun"].(bool)

	s.logger.Debug("Executing federationSync", map[string]interface{}{
		"federation": fedName,
		"force":      force,
		"dryRun":     dryRun,
	})

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	opts := federation.SyncOptions{
		Force:  force,
		DryRun: dryRun,
	}

	// Parse repo IDs
	if reposRaw, ok := params["repos"].([]interface{}); ok {
		for _, r := range reposRaw {
			if s, ok := r.(string); ok {
				opts.RepoIDs = append(opts.RepoIDs, s)
			}
		}
	}

	results, err := fed.Sync(opts)
	if err != nil {
		return nil, errors.NewOperationError("sync federation", err)
	}

	// Compute summary
	success := 0
	failed := 0
	skipped := 0
	for _, r := range results {
		switch r.Status {
		case "success":
			success++
		case "failed":
			failed++
		case "skipped", "dry_run":
			skipped++
		}
	}

	return NewToolResponse().
		Data(map[string]interface{}{
			"results": results,
			"summary": map[string]int{
				"success": success,
				"failed":  failed,
				"skipped": skipped,
				"total":   len(results),
			},
		}).
		CrossRepo().
		Build(), nil
}
