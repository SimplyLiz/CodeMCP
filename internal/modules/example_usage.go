package modules

// This file provides example usage of the modules package.
// It demonstrates how to use the module detection and import scanning functionality.

/*
Example 1: Basic Module Detection

	import (
		"ckb/internal/config"
		"ckb/internal/slogutil"
		"ckb/internal/modules"
		"log/slog"
		"os"
	)

	func detectModulesExample() {
		// Create logger
		logger := slogutil.NewLogger(os.Stderr, slog.LevelInfo)

		// Load config
		cfg, err := config.LoadConfig("/path/to/repo")
		if err != nil {
			panic(err)
		}

		// Create scanner
		scanner := modules.NewScanner(cfg, logger)

		// Scan modules
		modules, err := scanner.ScanModules("/path/to/repo", "state-id-123")
		if err != nil {
			panic(err)
		}

		// Print detected modules
		for _, module := range modules {
			logger.Info("Detected module", map[string]interface{}{
				"id":           module.ID,
				"name":         module.Name,
				"rootPath":     module.RootPath,
				"manifestType": module.ManifestType,
				"language":     module.Language,
			})
		}
	}

Example 2: Import Scanning

	func scanImportsExample() {
		logger := slogutil.NewLogger(os.Stderr, slog.LevelInfo)

		cfg, err := config.LoadConfig("/path/to/repo")
		if err != nil {
			panic(err)
		}

		scanner := modules.NewScanner(cfg, logger)

		// First, detect modules
		modules, err := scanner.ScanModules("/path/to/repo", "state-id-123")
		if err != nil {
			panic(err)
		}

		// Scan imports for a specific module
		if len(modules) > 0 {
			module := modules[0]
			edges, err := scanner.ScanImports("/path/to/repo", module, modules)
			if err != nil {
				panic(err)
			}

			// Print import statistics
			stats := modules.GetImportStatistics(edges)
			logger.Info("Import statistics", stats)

			// Filter by kind
			localImports := modules.FilterImportsByKind(edges, modules.LocalFile)
			externalImports := modules.FilterImportsByKind(edges, modules.ExternalDependency)

			logger.Info("Import breakdown", map[string]interface{}{
				"local":    len(localImports),
				"external": len(externalImports),
			})
		}
	}

Example 3: Manual Import Classification

	func classifyImportsExample() {
		// Build module context
		repoRoot := "/path/to/repo"
		modules := []*modules.Module{
			modules.NewModule("mod-1", "my-app", ".", "package.json", modules.LanguageTypeScript, "state-123"),
		}

		ctx := modules.BuildModuleContext(repoRoot, modules, modules.LanguageTypeScript)

		// Create classifier
		classifier := modules.NewImportClassifier(ctx)

		// Classify various imports
		imports := []string{
			"./utils/helper",          // Local file
			"react",                   // External dependency
			"@my-org/shared-lib",      // Workspace package (if configured)
			"node:fs",                 // Stdlib
		}

		for _, imp := range imports {
			kind := classifier.ClassifyImport(imp, "src/index.ts")
			// kind will be: LocalFile, ExternalDependency, WorkspacePackage, or Stdlib
		}
	}

Example 4: Complete Workflow

	func completeWorkflowExample() {
		logger := slogutil.NewLogger(os.Stderr, slog.LevelInfo)

		cfg, err := config.LoadConfig("/path/to/repo")
		if err != nil {
			panic(err)
		}

		scanner := modules.NewScanner(cfg, logger)
		repoRoot := "/path/to/repo"
		stateId := "state-123"

		// 1. Detect modules
		modules, err := scanner.ScanModules(repoRoot, stateId)
		if err != nil {
			panic(err)
		}

		logger.Info("Detected modules", map[string]interface{}{
			"count": len(modules),
		})

		// 2. Scan imports for all modules
		allImports, err := scanner.ScanAllImports(repoRoot, modules)
		if err != nil {
			panic(err)
		}

		// 3. Process results
		for moduleID, edges := range allImports {
			module := scanner.GetModuleByID(modules, moduleID)
			if module == nil {
				continue
			}

			// Get statistics
			stats := modules.GetImportStatistics(edges)
			logger.Info("Module import analysis", map[string]interface{}{
				"moduleId":           module.ID,
				"moduleName":         module.Name,
				"totalImports":       stats["total"],
				"externalDeps":       stats["externalDependency"],
				"localFiles":         stats["localFile"],
				"stdlib":             stats["stdlib"],
			})

			// Group by kind
			grouped := modules.GroupImportsByKind(edges)

			// Print external dependencies
			if extDeps, ok := grouped[modules.ExternalDependency]; ok {
				logger.Info("External dependencies", map[string]interface{}{
					"count": len(extDeps),
				})
				for _, edge := range extDeps {
					logger.Debug("External dependency", map[string]interface{}{
						"from": edge.From,
						"to":   edge.To,
					})
				}
			}
		}
	}

Example 5: Custom Module Detection

	func customModuleDetection() {
		logger := slogutil.NewLogger(os.Stderr, slog.LevelInfo)

		repoRoot := "/path/to/repo"
		stateId := "state-123"

		// Explicit module roots
		explicitRoots := []string{"frontend", "backend", "shared"}
		ignoreDirs := []string{"node_modules", "build", ".git"}

		result, err := modules.DetectModules(repoRoot, explicitRoots, ignoreDirs, stateId, logger)
		if err != nil {
			panic(err)
		}

		logger.Info("Module detection", map[string]interface{}{
			"method":      result.DetectionMethod,
			"moduleCount": len(result.Modules),
		})

		for _, module := range result.Modules {
			logger.Info("Module", map[string]interface{}{
				"id":           module.ID,
				"name":         module.Name,
				"rootPath":     module.RootPath,
				"manifestType": module.ManifestType,
				"language":     module.Language,
			})
		}
	}
*/
