package mcp

import (
	"encoding/json"
	"fmt"

	"ckb/internal/federation"
	"ckb/internal/logging"
)

// v6.3 Contract-Aware Impact Analysis tool implementations

// toolListContracts lists contracts in a federation
func (s *MCPServer) toolListContracts(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	s.logger.Debug("Executing listContracts", map[string]interface{}{
		"federation": fedName,
	})

	// Open federation
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.WarnLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open federation: %w", err)
	}
	defer fed.Close()

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
		return nil, fmt.Errorf("failed to list contracts: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolAnalyzeContractImpact analyzes impact of changing a contract
func (s *MCPServer) toolAnalyzeContractImpact(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	repoID, ok := params["repoId"].(string)
	if !ok || repoID == "" {
		return nil, fmt.Errorf("missing or invalid 'repoId' parameter")
	}

	path, ok := params["path"].(string)
	if !ok || path == "" {
		return nil, fmt.Errorf("missing or invalid 'path' parameter")
	}

	s.logger.Debug("Executing analyzeContractImpact", map[string]interface{}{
		"federation": fedName,
		"repoId":     repoID,
		"path":       path,
	})

	// Open federation
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.WarnLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open federation: %w", err)
	}
	defer fed.Close()

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
		return nil, fmt.Errorf("failed to analyze impact: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolGetContractDependencies gets dependencies for a repo
func (s *MCPServer) toolGetContractDependencies(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	repoID, ok := params["repoId"].(string)
	if !ok || repoID == "" {
		return nil, fmt.Errorf("missing or invalid 'repoId' parameter")
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
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.WarnLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open federation: %w", err)
	}
	defer fed.Close()

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
		return nil, fmt.Errorf("failed to get dependencies: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolSuppressContractEdge suppresses a contract edge
func (s *MCPServer) toolSuppressContractEdge(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	edgeID, ok := params["edgeId"].(float64)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'edgeId' parameter")
	}

	reason, _ := params["reason"].(string)

	s.logger.Debug("Executing suppressContractEdge", map[string]interface{}{
		"federation": fedName,
		"edgeId":     edgeID,
		"reason":     reason,
	})

	// Open federation
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.WarnLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open federation: %w", err)
	}
	defer fed.Close()

	if err := fed.SuppressContractEdge(int64(edgeID), "user", reason); err != nil {
		return nil, fmt.Errorf("failed to suppress edge: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"edgeId":  int64(edgeID),
		"message": "Edge suppressed successfully",
	}, nil
}

// toolVerifyContractEdge verifies a contract edge
func (s *MCPServer) toolVerifyContractEdge(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	edgeID, ok := params["edgeId"].(float64)
	if !ok {
		return nil, fmt.Errorf("missing or invalid 'edgeId' parameter")
	}

	s.logger.Debug("Executing verifyContractEdge", map[string]interface{}{
		"federation": fedName,
		"edgeId":     edgeID,
	})

	// Open federation
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.WarnLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open federation: %w", err)
	}
	defer fed.Close()

	if err := fed.VerifyContractEdge(int64(edgeID), "user"); err != nil {
		return nil, fmt.Errorf("failed to verify edge: %w", err)
	}

	return map[string]interface{}{
		"success": true,
		"edgeId":  int64(edgeID),
		"message": "Edge verified successfully",
	}, nil
}

// toolGetContractStats gets contract statistics for a federation
func (s *MCPServer) toolGetContractStats(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	s.logger.Debug("Executing getContractStats", map[string]interface{}{
		"federation": fedName,
	})

	// Open federation
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.WarnLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open federation: %w", err)
	}
	defer fed.Close()

	stats, err := fed.GetContractStats()
	if err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(stats, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}
