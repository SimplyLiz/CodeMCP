package a2a

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/index"
)

// registerRoutes registers all A2A protocol routes.
func (s *Server) registerRoutes() {
	// Agent discovery (no auth)
	s.router.HandleFunc("GET /.well-known/agent-card.json", s.handleAgentCard)
	s.router.HandleFunc("GET /extendedAgentCard", s.handleExtendedAgentCard)

	// JSON-RPC endpoint (handles all A2A methods)
	s.router.HandleFunc("POST /", s.handleJSONRPC)

	// HTTP+JSON binding
	s.router.HandleFunc("POST /message:send", s.handleHTTPMessageSend)
	s.router.HandleFunc("POST /message:stream", s.handleHTTPMessageStream)
	s.router.HandleFunc("GET /tasks/{id}", s.handleHTTPGetTask)
	s.router.HandleFunc("GET /tasks", s.handleHTTPListTasks)
	// Go 1.22 ServeMux rejects "{id}:action" patterns (colon after wildcard segment).
	// Use a prefix catch-all for POST /tasks/ and dispatch by action suffix.
	s.router.HandleFunc("POST /tasks/", s.handleHTTPTaskActions)

	// Push notification config CRUD
	s.router.HandleFunc("POST /tasks/{id}/pushNotificationConfigs", s.handleHTTPCreatePushConfig)
	s.router.HandleFunc("GET /tasks/{id}/pushNotificationConfigs/{configId}", s.handleHTTPGetPushConfig)
	s.router.HandleFunc("GET /tasks/{id}/pushNotificationConfigs", s.handleHTTPListPushConfigs)
	s.router.HandleFunc("DELETE /tasks/{id}/pushNotificationConfigs/{configId}", s.handleHTTPDeletePushConfig)

	// Health
	s.router.HandleFunc("GET /health", s.handleHealth)
}

// handleHTTPTaskActions dispatches POST /tasks/<id>:cancel and POST /tasks/<id>:subscribe.
// Go 1.22's net/http.ServeMux rejects "{id}:action" patterns (literal colon after a
// wildcard segment is not allowed). We register a prefix catch-all and parse manually.
func (s *Server) handleHTTPTaskActions(w http.ResponseWriter, r *http.Request) {
	// path is everything after "/tasks/"
	path := strings.TrimPrefix(r.URL.Path, "/tasks/")

	var taskID, action string
	if idx := strings.LastIndex(path, ":"); idx >= 0 {
		taskID = path[:idx]
		action = path[idx+1:]
	} else {
		taskID = path
	}

	if taskID == "" {
		writeA2AError(w, NewInvalidParamsError("task ID required"))
		return
	}

	// Inject the task ID so existing handlers can read it via r.PathValue("id").
	r.SetPathValue("id", taskID)

	switch action {
	case "cancel":
		s.handleHTTPCancelTask(w, r)
	case "subscribe":
		s.handleHTTPSubscribeTask(w, r)
	default:
		http.NotFound(w, r)
	}
}

// handleHealth returns health with repo init status, index freshness, and suggestions.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]any{
		"protocol": "a2a",
		"version":  ProtocolVersion,
	}

	// Index freshness
	indexStatus := s.getIndexStatus()
	health["index"] = indexStatus

	// Backend health from query engine
	ctx := context.Background()
	statusResp, err := s.engine.GetStatus(ctx)
	if err != nil {
		health["status"] = "degraded"
		health["error"] = err.Error()
		health["suggestions"] = []string{"Run 'ckb init' to initialize the repository, then 'ckb index' to create the index."}
		writeJSON(w, http.StatusOK, health)
		return
	}

	health["status"] = "healthy"
	if !statusResp.Healthy {
		health["status"] = "degraded"
	}

	// Backend summary
	backends := make([]map[string]any, 0, len(statusResp.Backends))
	for _, b := range statusResp.Backends {
		tier := "available"
		if !b.Available {
			tier = "unavailable"
		} else if !b.Healthy {
			tier = "degraded"
		}
		backends = append(backends, map[string]any{
			"id":         b.Id,
			"healthTier": tier,
			"available":  b.Available,
		})
	}
	health["backends"] = backends

	// Actionable suggestions
	suggestions := s.buildSuggestions(indexStatus, statusResp)
	if len(suggestions) > 0 {
		health["suggestions"] = suggestions
	}

	writeJSON(w, http.StatusOK, health)
}

// getIndexStatus returns index freshness information.
func (s *Server) getIndexStatus() map[string]any {
	repoRoot := s.engine.GetRepoRoot()
	if repoRoot == "" {
		return map[string]any{
			"initialized": false,
			"exists":      false,
			"fresh":       false,
			"reason":      "no repository configured",
		}
	}

	ckbDir := filepath.Join(repoRoot, ".ckb")
	meta, err := index.LoadMeta(ckbDir)
	if err != nil || meta == nil {
		return map[string]any{
			"initialized": false,
			"exists":      false,
			"fresh":       false,
			"reason":      "no index found — run 'ckb init && ckb index'",
		}
	}

	staleness := meta.GetStaleness(repoRoot)
	return map[string]any{
		"initialized":   true,
		"exists":        true,
		"fresh":         !staleness.IsStale,
		"reason":        staleness.Reason,
		"indexAge":      staleness.IndexAge,
		"commitsBehind": staleness.CommitsBehind,
	}
}

// buildSuggestions generates actionable hints based on repo/index state.
func (s *Server) buildSuggestions(indexStatus map[string]any, statusResp interface{}) []string {
	var suggestions []string

	initialized, _ := indexStatus["initialized"].(bool)
	fresh, _ := indexStatus["fresh"].(bool)
	commitsBehind, _ := indexStatus["commitsBehind"].(int)

	if !initialized {
		suggestions = append(suggestions,
			"Repository is not indexed. Run 'ckb init && ckb index' to enable full code intelligence.",
			"Without an index, only git-based features will be available.",
		)
	} else if !fresh {
		if commitsBehind > 0 {
			suggestions = append(suggestions,
				fmt.Sprintf("Index is %d commit(s) behind. Run 'ckb index' to refresh, or use the 'reindex' skill.", commitsBehind),
			)
		} else if reason, ok := indexStatus["reason"].(string); ok && reason != "" {
			suggestions = append(suggestions,
				fmt.Sprintf("Index may be stale: %s. Run 'ckb index' to refresh, or use the 'reindex' skill.", reason),
			)
		}
	}

	return suggestions
}
