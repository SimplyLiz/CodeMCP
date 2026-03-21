package mcp

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/repos"
)

// pathParams is the ordered list of tool parameter names that may contain file paths.
// We check these in priority order and return the first path-like value found.
var pathParams = []string{"filePath", "path", "targetPath", "target", "moduleId"}

// extractPathHint extracts a file path hint from tool parameters.
// Returns the first path-like value found, or "" if none.
func extractPathHint(toolParams map[string]interface{}) string {
	for _, key := range pathParams {
		val, ok := toolParams[key].(string)
		if !ok || val == "" {
			continue
		}

		// "target" is overloaded: sometimes a symbol name like "MCPServer.GetEngine".
		// Only treat it as a path if it contains a path separator.
		if key == "target" && !strings.Contains(val, "/") && !strings.Contains(val, "\\") {
			continue
		}

		return val
	}
	return ""
}

// resolveRepoForPath resolves a path hint to a git repo root.
// Returns the repo root path or "" if the hint cannot be resolved.
func (s *MCPServer) resolveRepoForPath(pathHint string) string {
	if pathHint == "" {
		return ""
	}

	// Absolute path: resolve directly
	if filepath.IsAbs(pathHint) {
		return repos.FindGitRoot(pathHint)
	}

	// Relative path: try against each client root
	for _, root := range s.GetRootPaths() {
		candidate := filepath.Join(root, pathHint)
		if _, err := os.Stat(candidate); err == nil {
			if gitRoot := repos.FindGitRoot(candidate); gitRoot != "" {
				return gitRoot
			}
		}
	}

	// Try against current engine's repo root
	if eng := s.engine(); eng != nil {
		candidate := filepath.Join(eng.GetRepoRoot(), pathHint)
		if _, err := os.Stat(candidate); err == nil {
			if gitRoot := repos.FindGitRoot(candidate); gitRoot != "" {
				return gitRoot
			}
		}
	}

	return ""
}
