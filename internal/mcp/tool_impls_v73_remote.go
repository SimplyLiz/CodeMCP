package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"ckb/internal/federation"
	"ckb/internal/logging"
)

// v7.3 Remote Federation tool implementations (Phase 5)

// toolFederationAddRemote adds a remote server to a federation
func (s *MCPServer) toolFederationAddRemote(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	serverName, ok := params["name"].(string)
	if !ok || serverName == "" {
		return nil, fmt.Errorf("missing or invalid 'name' parameter")
	}

	serverURL, ok := params["url"].(string)
	if !ok || serverURL == "" {
		return nil, fmt.Errorf("missing or invalid 'url' parameter")
	}

	token, _ := params["token"].(string)
	cacheTTL := federation.DefaultRemoteCacheTTL
	if ttlStr, ok := params["cacheTtl"].(string); ok {
		if d, err := time.ParseDuration(ttlStr); err == nil {
			cacheTTL = d
		}
	}
	timeout := federation.DefaultRemoteTimeout
	if timeoutStr, ok := params["timeout"].(string); ok {
		if d, err := time.ParseDuration(timeoutStr); err == nil {
			timeout = d
		}
	}

	s.logger.Debug("Executing federationAddRemote", map[string]interface{}{
		"federation": fedName,
		"name":       serverName,
		"url":        serverURL,
	})

	// Open federation
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	server := federation.RemoteServer{
		Name:     serverName,
		URL:      serverURL,
		Token:    token,
		CacheTTL: federation.Duration{Duration: cacheTTL},
		Timeout:  federation.Duration{Duration: timeout},
		Enabled:  true,
	}

	if addErr := fed.AddRemoteServer(server); addErr != nil {
		return nil, fmt.Errorf("failed to add remote server: %w", addErr)
	}

	result := map[string]interface{}{
		"name":     serverName,
		"url":      serverURL,
		"cacheTtl": cacheTTL.String(),
		"timeout":  timeout.String(),
		"enabled":  true,
		"message":  fmt.Sprintf("Added remote server %q to federation %q", serverName, fedName),
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolFederationRemoveRemote removes a remote server from a federation
func (s *MCPServer) toolFederationRemoveRemote(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	serverName, ok := params["name"].(string)
	if !ok || serverName == "" {
		return nil, fmt.Errorf("missing or invalid 'name' parameter")
	}

	s.logger.Debug("Executing federationRemoveRemote", map[string]interface{}{
		"federation": fedName,
		"name":       serverName,
	})

	// Open federation
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	if removeErr := fed.RemoveRemoteServer(serverName); removeErr != nil {
		return nil, fmt.Errorf("failed to remove remote server: %w", removeErr)
	}

	result := map[string]interface{}{
		"name":    serverName,
		"message": fmt.Sprintf("Removed remote server %q from federation %q", serverName, fedName),
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolFederationListRemote lists remote servers in a federation
func (s *MCPServer) toolFederationListRemote(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	s.logger.Debug("Executing federationListRemote", map[string]interface{}{
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
	defer func() { _ = fed.Close() }()

	servers := fed.ListRemoteServers()

	// Convert to serializable format
	serversOut := make([]map[string]interface{}, len(servers))
	for i, s := range servers {
		serversOut[i] = map[string]interface{}{
			"name":         s.Name,
			"url":          s.URL,
			"cacheTtl":     s.GetCacheTTL().String(),
			"timeout":      s.GetTimeout().String(),
			"enabled":      s.Enabled,
			"addedAt":      s.AddedAt,
			"lastSyncedAt": s.LastSyncedAt,
			"lastError":    s.LastError,
		}
	}

	result := map[string]interface{}{
		"servers": serversOut,
		"count":   len(servers),
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolFederationSyncRemote syncs metadata from remote servers
func (s *MCPServer) toolFederationSyncRemote(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	serverName, _ := params["name"].(string) // Optional

	s.logger.Debug("Executing federationSyncRemote", map[string]interface{}{
		"federation": fedName,
		"name":       serverName,
	})

	// Open federation
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fed, err := federation.Open(fedName, logger)
	if err != nil {
		return nil, fmt.Errorf("failed to open federation: %w", err)
	}
	defer func() { _ = fed.Close() }()

	// Create hybrid engine
	engine := federation.NewHybridEngine(fed, logger)
	if initErr := engine.InitRemoteClients(); initErr != nil {
		return nil, fmt.Errorf("failed to initialize remote clients: %w", initErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	var result map[string]interface{}

	if serverName != "" {
		// Sync specific server
		if syncErr := engine.SyncRemote(ctx, serverName); syncErr != nil {
			return nil, fmt.Errorf("failed to sync server %q: %w", serverName, syncErr)
		}

		repos, _ := fed.Index().GetRemoteRepos(serverName)
		result = map[string]interface{}{
			"server":    serverName,
			"repoCount": len(repos),
			"message":   fmt.Sprintf("Synced %d repositories from %q", len(repos), serverName),
		}
	} else {
		// Sync all servers
		errors := engine.SyncAllRemotes(ctx)
		servers := fed.GetEnabledRemoteServers()
		success := len(servers) - len(errors)

		result = map[string]interface{}{
			"total":   len(servers),
			"success": success,
			"failed":  len(errors),
			"errors":  errors,
			"message": fmt.Sprintf("Synced %d/%d remote servers", success, len(servers)),
		}
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolFederationStatusRemote gets status of a remote server
func (s *MCPServer) toolFederationStatusRemote(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	serverName, ok := params["name"].(string)
	if !ok || serverName == "" {
		return nil, fmt.Errorf("missing or invalid 'name' parameter")
	}

	s.logger.Debug("Executing federationStatusRemote", map[string]interface{}{
		"federation": fedName,
		"name":       serverName,
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
	defer func() { _ = fed.Close() }()

	// Create hybrid engine
	engine := federation.NewHybridEngine(fed, logger)
	if initErr := engine.InitRemoteClients(); initErr != nil {
		return nil, fmt.Errorf("failed to initialize remote clients: %w", initErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	status, statusErr := engine.GetRemoteStatus(ctx, serverName)
	if statusErr != nil {
		return nil, fmt.Errorf("failed to get status: %w", statusErr)
	}

	result := map[string]interface{}{
		"name":            status.Name,
		"url":             status.URL,
		"enabled":         status.Enabled,
		"online":          status.Online,
		"latency":         status.Latency.String(),
		"pingError":       status.PingError,
		"lastSyncedAt":    status.LastSyncedAt,
		"lastError":       status.LastError,
		"cachedRepoCount": status.CachedRepoCount,
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolFederationSearchSymbolsHybrid searches symbols across local and remote
func (s *MCPServer) toolFederationSearchSymbolsHybrid(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	query, ok := params["query"].(string)
	if !ok || query == "" {
		return nil, fmt.Errorf("missing or invalid 'query' parameter")
	}

	limit := 100
	if l, ok := params["limit"].(float64); ok {
		limit = int(l)
	}

	language, _ := params["language"].(string)
	kind, _ := params["kind"].(string)

	s.logger.Debug("Executing federationSearchSymbolsHybrid", map[string]interface{}{
		"federation": fedName,
		"query":      query,
		"limit":      limit,
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
	defer func() { _ = fed.Close() }()

	// Create hybrid engine
	engine := federation.NewHybridEngine(fed, logger)
	if initErr := engine.InitRemoteClients(); initErr != nil {
		return nil, fmt.Errorf("failed to initialize remote clients: %w", initErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	opts := federation.HybridSearchOptions{
		Query:        query,
		Limit:        limit,
		Language:     language,
		Kind:         kind,
		IncludeLocal: true,
	}

	// Parse specific servers
	if serversRaw, ok := params["servers"].([]interface{}); ok {
		for _, srv := range serversRaw {
			if str, ok := srv.(string); ok {
				opts.Servers = append(opts.Servers, str)
			}
		}
	}

	result, err := engine.SearchSymbols(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}

// toolFederationListAllRepos lists repos from local and remote sources
func (s *MCPServer) toolFederationListAllRepos(params map[string]interface{}) (interface{}, error) {
	fedName, ok := params["federation"].(string)
	if !ok || fedName == "" {
		return nil, fmt.Errorf("missing or invalid 'federation' parameter")
	}

	s.logger.Debug("Executing federationListAllRepos", map[string]interface{}{
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
	defer func() { _ = fed.Close() }()

	// Create hybrid engine
	engine := federation.NewHybridEngine(fed, logger)
	if initErr := engine.InitRemoteClients(); initErr != nil {
		return nil, fmt.Errorf("failed to initialize remote clients: %w", initErr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	result, listErr := engine.ListAllRepos(ctx)
	if listErr != nil {
		return nil, fmt.Errorf("list failed: %w", listErr)
	}

	jsonBytes, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}

	return string(jsonBytes), nil
}
