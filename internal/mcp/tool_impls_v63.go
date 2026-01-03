package mcp

import (
	"io"
	"log/slog"

	"ckb/internal/envelope"
	"ckb/internal/errors"
	"ckb/internal/federation"
)

// v6.3 Contract-Aware Impact Analysis tool implementations

// toolListContracts lists contracts in a federation
func (s *MCPServer) toolListContracts(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	s.logger.Debug("Executing listContracts", map[string]interface{}{
		"federation": fedName,
	})

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	opts := federation.ListContractsOptions{}

	if repoID, ok := params["repoId"].(string); ok {
		opts.RepoID = repoID
	}
	if contractType, ok := params["contractType"].(string); ok {
		opts.ContractType = contractType
	}
	if visibility, ok := params["visibility"].(string); ok {
		opts.Visibility = visibility
	}
	if limit, ok := params["limit"].(float64); ok {
		opts.Limit = int(limit)
	}

	result, err := fed.ListContracts(opts)
	if err != nil {
		return nil, errors.NewOperationError("list contracts", err)
	}

	return NewToolResponse().
		Data(result).
		CrossRepo().
		Build(), nil
}

// toolAnalyzeContractImpact analyzes impact of changing a contract
func (s *MCPServer) toolAnalyzeContractImpact(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	repoID, ok := params["repoId"].(string)
	if !ok || repoID == "" {
		return nil, errors.NewInvalidParameterError("repoId", "")
	}

	path, ok := params["path"].(string)
	if !ok || path == "" {
		return nil, errors.NewInvalidParameterError("path", "")
	}

	s.logger.Debug("Executing analyzeContractImpact", map[string]interface{}{
		"federation": fedName,
		"repoId":     repoID,
		"path":       path,
	})

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	opts := federation.AnalyzeContractImpactOptions{
		Federation: fedName,
		RepoID:     repoID,
		Path:       path,
	}

	if includeHeuristic, ok := params["includeHeuristic"].(bool); ok {
		opts.IncludeHeuristic = includeHeuristic
	}
	if includeTransitive, ok := params["includeTransitive"].(bool); ok {
		opts.IncludeTransitive = includeTransitive
	}
	if maxDepth, ok := params["maxDepth"].(float64); ok {
		opts.MaxDepth = int(maxDepth)
	}

	result, err := fed.AnalyzeContractImpact(opts)
	if err != nil {
		return nil, errors.NewOperationError("analyze contract impact", err)
	}

	return NewToolResponse().
		Data(result).
		CrossRepo().
		Build(), nil
}

// toolGetContractDependencies gets dependencies for a repo
func (s *MCPServer) toolGetContractDependencies(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	repoID, ok := params["repoId"].(string)
	if !ok || repoID == "" {
		return nil, errors.NewInvalidParameterError("repoId", "")
	}

	direction := "both"
	if d, ok := params["direction"].(string); ok {
		direction = d
	}

	s.logger.Debug("Executing getContractDependencies", map[string]interface{}{
		"federation": fedName,
		"repoId":     repoID,
		"direction":  direction,
	})

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	opts := federation.GetDependenciesOptions{
		Federation: fedName,
		RepoID:     repoID,
		Direction:  direction,
	}

	if moduleID, ok := params["moduleId"].(string); ok {
		opts.ModuleID = moduleID
	}
	if includeHeuristic, ok := params["includeHeuristic"].(bool); ok {
		opts.IncludeHeuristic = includeHeuristic
	}

	result, err := fed.GetDependencies(opts)
	if err != nil {
		return nil, errors.NewOperationError("get contract dependencies", err)
	}

	return NewToolResponse().
		Data(result).
		CrossRepo().
		Build(), nil
}

// toolSuppressContractEdge suppresses a contract edge
func (s *MCPServer) toolSuppressContractEdge(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	edgeID, ok := params["edgeId"].(float64)
	if !ok {
		return nil, errors.NewInvalidParameterError("edgeId", "")
	}

	reason, _ := params["reason"].(string)

	s.logger.Debug("Executing suppressContractEdge", map[string]interface{}{
		"federation": fedName,
		"edgeId":     edgeID,
		"reason":     reason,
	})

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	if err := fed.SuppressContractEdge(int64(edgeID), "user", reason); err != nil {
		return nil, errors.NewOperationError("suppress contract edge", err)
	}

	return NewToolResponse().
		Data(map[string]interface{}{
			"success": true,
			"edgeId":  int64(edgeID),
			"message": "Edge suppressed successfully",
		}).
		CrossRepo().
		Build(), nil
}

// toolVerifyContractEdge verifies a contract edge
func (s *MCPServer) toolVerifyContractEdge(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	edgeID, ok := params["edgeId"].(float64)
	if !ok {
		return nil, errors.NewInvalidParameterError("edgeId", "")
	}

	s.logger.Debug("Executing verifyContractEdge", map[string]interface{}{
		"federation": fedName,
		"edgeId":     edgeID,
	})

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	if err := fed.VerifyContractEdge(int64(edgeID), "user"); err != nil {
		return nil, errors.NewOperationError("verify contract edge", err)
	}

	return NewToolResponse().
		Data(map[string]interface{}{
			"success": true,
			"edgeId":  int64(edgeID),
			"message": "Edge verified successfully",
		}).
		CrossRepo().
		Build(), nil
}

// toolGetContractStats gets contract statistics for a federation
func (s *MCPServer) toolGetContractStats(params map[string]interface{}) (*envelope.Response, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, errors.NewInvalidParameterError("federation", "")
	}

	s.logger.Debug("Executing getContractStats", map[string]interface{}{
		"federation": fedName,
	})

	// Open federation
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, errors.NewOperationError("open federation", err)
	}
	defer func() { _ = fed.Close() }()

	stats, err := fed.GetContractStats()
	if err != nil {
		return nil, errors.NewOperationError("get contract stats", err)
	}

	return NewToolResponse().
		Data(stats).
		CrossRepo().
		Build(), nil
}
