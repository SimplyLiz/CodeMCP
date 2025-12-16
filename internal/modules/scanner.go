package modules

import (
	"path/filepath"

	"ckb/internal/config"
	"ckb/internal/logging"
)

// Scanner provides high-level module and import scanning functionality
type Scanner struct {
	config         *config.Config
	logger         *logging.Logger
	importScanner  *ImportScanner
}

// NewScanner creates a new module scanner
func NewScanner(cfg *config.Config, logger *logging.Logger) *Scanner {
	return &Scanner{
		config:        cfg,
		logger:        logger,
		importScanner: NewImportScanner(&cfg.ImportScan, logger),
	}
}

// ScanModules scans the repository and returns all detected modules
func (s *Scanner) ScanModules(repoRoot string, stateId string) ([]*Module, error) {
	s.logger.Info("Scanning modules", map[string]interface{}{
		"repoRoot": repoRoot,
		"stateId":  stateId,
	})

	result, err := DetectModules(
		repoRoot,
		s.config.Modules.Roots,
		s.config.Modules.Ignore,
		stateId,
		s.logger,
	)

	if err != nil {
		return nil, err
	}

	s.logger.Info("Module detection completed", map[string]interface{}{
		"method":      result.DetectionMethod,
		"moduleCount": len(result.Modules),
	})

	return result.Modules, nil
}

// ScanImports scans a module for imports and classifies them
func (s *Scanner) ScanImports(repoRoot string, module *Module, allModules []*Module) ([]*ImportEdge, error) {
	if !s.config.ImportScan.Enabled {
		s.logger.Debug("Import scanning disabled", nil)
		return nil, nil
	}

	s.logger.Info("Scanning imports for module", map[string]interface{}{
		"moduleId":   module.ID,
		"moduleName": module.Name,
		"rootPath":   module.RootPath,
	})

	// Scan directory for imports
	modulePath := filepath.Join(repoRoot, module.RootPath)
	edges, err := s.importScanner.ScanDirectory(modulePath, repoRoot, s.config.Modules.Ignore)
	if err != nil {
		return nil, err
	}

	// Build context for classification
	ctx := BuildModuleContext(repoRoot, allModules, module.Language)

	// Classify each import edge
	classifier := NewImportClassifier(ctx)
	for _, edge := range edges {
		classifier.ClassifyEdge(edge)
	}

	s.logger.Info("Import scanning completed", map[string]interface{}{
		"moduleId":    module.ID,
		"importsFound": len(edges),
	})

	return edges, nil
}

// ScanAllImports scans all modules and returns aggregated import edges
func (s *Scanner) ScanAllImports(repoRoot string, modules []*Module) (map[string][]*ImportEdge, error) {
	if !s.config.ImportScan.Enabled {
		s.logger.Debug("Import scanning disabled", nil)
		return nil, nil
	}

	s.logger.Info("Scanning imports for all modules", map[string]interface{}{
		"moduleCount": len(modules),
	})

	result := make(map[string][]*ImportEdge)

	for _, module := range modules {
		edges, err := s.ScanImports(repoRoot, module, modules)
		if err != nil {
			s.logger.Warn("Failed to scan imports for module", map[string]interface{}{
				"moduleId": module.ID,
				"error":    err.Error(),
			})
			continue
		}

		result[module.ID] = edges
	}

	s.logger.Info("Import scanning completed for all modules", map[string]interface{}{
		"modulesScanned": len(result),
	})

	return result, nil
}

// GetModuleByID returns a module by its ID
func (s *Scanner) GetModuleByID(modules []*Module, moduleID string) *Module {
	for _, module := range modules {
		if module.ID == moduleID {
			return module
		}
	}
	return nil
}

// GetModuleByPath returns a module by its root path
func (s *Scanner) GetModuleByPath(modules []*Module, rootPath string) *Module {
	for _, module := range modules {
		if module.RootPath == rootPath {
			return module
		}
	}
	return nil
}

// FilterImportsByKind filters import edges by their kind
func FilterImportsByKind(edges []*ImportEdge, kinds ...ImportEdgeKind) []*ImportEdge {
	var filtered []*ImportEdge
	kindMap := make(map[ImportEdgeKind]bool)
	for _, kind := range kinds {
		kindMap[kind] = true
	}

	for _, edge := range edges {
		if kindMap[edge.Kind] {
			filtered = append(filtered, edge)
		}
	}

	return filtered
}

// GroupImportsByKind groups import edges by their kind
func GroupImportsByKind(edges []*ImportEdge) map[ImportEdgeKind][]*ImportEdge {
	grouped := make(map[ImportEdgeKind][]*ImportEdge)

	for _, edge := range edges {
		grouped[edge.Kind] = append(grouped[edge.Kind], edge)
	}

	return grouped
}

// GetImportStatistics computes statistics for import edges
func GetImportStatistics(edges []*ImportEdge) map[string]interface{} {
	stats := make(map[string]interface{})

	grouped := GroupImportsByKind(edges)

	stats["total"] = len(edges)
	stats["localFile"] = len(grouped[LocalFile])
	stats["localModule"] = len(grouped[LocalModule])
	stats["workspacePackage"] = len(grouped[WorkspacePackage])
	stats["externalDependency"] = len(grouped[ExternalDependency])
	stats["stdlib"] = len(grouped[Stdlib])
	stats["unknown"] = len(grouped[Unknown])

	return stats
}

// DeduplicateImports removes duplicate import edges
func DeduplicateImports(edges []*ImportEdge) []*ImportEdge {
	seen := make(map[string]bool)
	var unique []*ImportEdge

	for _, edge := range edges {
		// Create a key from from+to+raw
		key := edge.From + "|" + edge.To + "|" + edge.RawImport
		if !seen[key] {
			seen[key] = true
			unique = append(unique, edge)
		}
	}

	return unique
}
