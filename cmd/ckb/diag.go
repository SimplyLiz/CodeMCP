package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/config"
	"ckb/internal/logging"
	"ckb/internal/paths"
	"ckb/internal/query"
	"ckb/internal/version"
)

var (
	diagOut       string
	diagAnonymize bool
)

var diagCmd = &cobra.Command{
	Use:   "diag",
	Short: "Create diagnostic bundle",
	Long: `Create a diagnostic bundle for troubleshooting CKB issues.

The bundle includes:
  - Sanitized configuration
  - Doctor output
  - Backend status
  - System information
  - Recent error logs (if available)

Excludes:
  - Source code
  - Symbol names (when --anonymize is used)
  - Sensitive credentials

Example:
  ckb diag --out ckb-diagnostic.zip
  ckb diag --out ckb-diagnostic.zip --anonymize`,
	Run: runDiag,
}

func init() {
	diagCmd.Flags().StringVar(&diagOut, "out", "ckb-diagnostic.zip", "Output file path")
	diagCmd.Flags().BoolVar(&diagAnonymize, "anonymize", false, "Anonymize symbol names and paths")
	rootCmd.AddCommand(diagCmd)
}

// DiagnosticBundle contains all diagnostic information
type DiagnosticBundle struct {
	GeneratedAt string             `json:"generatedAt"`
	CkbVersion  string             `json:"ckbVersion"`
	System      DiagSystemInfo     `json:"system"`
	RepoState   *query.RepoState   `json:"repoState,omitempty"`
	Config      *config.Config     `json:"config,omitempty"`
	Status      *StatusResponseCLI `json:"status,omitempty"`
	Doctor      *DoctorResponseCLI `json:"doctor,omitempty"`
	Anonymized  bool               `json:"anonymized"`
}

// DiagSystemInfo contains system information
type DiagSystemInfo struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	GoVersion    string `json:"goVersion"`
	WorkingDir   string `json:"workingDir"`
}

func runDiag(cmd *cobra.Command, args []string) {
	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
	})

	fmt.Println("Creating diagnostic bundle...")

	// Get repository root
	repoRoot, err := paths.FindRepoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	// Collect diagnostic information
	bundle := collectDiagnostics(repoRoot, logger)

	// Create zip file
	if err := createDiagnosticZip(bundle, diagOut); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating diagnostic bundle: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Diagnostic bundle created: %s\n", diagOut)
	fmt.Println("\nThis bundle contains sanitized diagnostic information.")
	fmt.Println("Review the contents before sharing.")
}

// collectDiagnostics gathers all diagnostic information
func collectDiagnostics(repoRoot string, logger *logging.Logger) *DiagnosticBundle {
	bundle := &DiagnosticBundle{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		CkbVersion:  version.Version,
		Anonymized:  diagAnonymize,
	}

	// System info
	workingDir := repoRoot
	if diagAnonymize {
		workingDir = "<anonymized>"
	}
	bundle.System = DiagSystemInfo{
		OS:           runtime.GOOS,
		Architecture: runtime.GOARCH,
		GoVersion:    runtime.Version(),
		WorkingDir:   workingDir,
	}

	// Get engine for queries
	engine, err := getEngine(repoRoot, logger)
	if err != nil {
		logger.Warn("Failed to initialize engine", map[string]interface{}{
			"error": err.Error(),
		})
		return bundle
	}

	ctx := newContext()

	// Get repo state
	repoState, err := engine.GetRepoState(ctx, "head")
	if err == nil {
		bundle.RepoState = repoState
	}

	// Config (sanitize sensitive fields)
	if cfg, loadErr := config.LoadConfig(repoRoot); loadErr == nil {
		bundle.Config = sanitizeConfig(cfg)
	}

	// Status
	statusResp, err := engine.GetStatus(ctx)
	if err == nil {
		bundle.Status = convertStatusResponse(statusResp)
	}

	// Doctor checks
	doctorResp, err := engine.Doctor(ctx, "")
	if err == nil {
		bundle.Doctor = convertDoctorResponse(doctorResp)
	}

	return bundle
}

// sanitizeConfig removes sensitive information from config
func sanitizeConfig(cfg *config.Config) *config.Config {
	// Create a copy
	sanitized := *cfg

	// Sanitize repo root if anonymizing
	if diagAnonymize {
		sanitized.RepoRoot = "<anonymized>"
	}

	// Remove any sensitive LSP server configurations
	// (commands might contain tokens or credentials)
	if len(sanitized.Backends.Lsp.Servers) > 0 {
		sanitized.Backends.Lsp.Servers = make(map[string]config.LspServerCfg)
		// Just include language names, not commands
		for lang := range cfg.Backends.Lsp.Servers {
			sanitized.Backends.Lsp.Servers[lang] = config.LspServerCfg{
				Command: "<sanitized>",
				Args:    []string{"<sanitized>"},
			}
		}
	}

	return &sanitized
}

// createDiagnosticZip creates a zip file with diagnostic information
func createDiagnosticZip(bundle *DiagnosticBundle, outPath string) error {
	// Create output file
	outFile, err := os.Create(outPath)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer func() { _ = outFile.Close() }()

	// Create zip writer
	zipWriter := zip.NewWriter(outFile)
	defer func() { _ = zipWriter.Close() }()

	// Add bundle.json
	bundleJSON, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bundle: %w", err)
	}

	if err := addFileToZip(zipWriter, "bundle.json", bundleJSON); err != nil {
		return err
	}

	// Add README
	readme := []byte(`CKB Diagnostic Bundle
====================

This bundle contains diagnostic information for troubleshooting CKB issues.

Contents:
- bundle.json: Complete diagnostic information
  - System information
  - Repository state
  - Configuration (sanitized)
  - Status information
  - Doctor check results

Generated: ` + bundle.GeneratedAt + `
CKB Version: ` + bundle.CkbVersion + `
Anonymized: ` + fmt.Sprintf("%v", bundle.Anonymized) + `

Please review the contents before sharing with others.
`)

	if err := addFileToZip(zipWriter, "README.txt", readme); err != nil {
		return err
	}

	// Add individual sections as separate files for easy reading
	if bundle.Status != nil {
		statusJSON, _ := json.MarshalIndent(bundle.Status, "", "  ")
		if err := addFileToZip(zipWriter, "status.json", statusJSON); err != nil {
			return err
		}
	}

	if bundle.Doctor != nil {
		doctorJSON, _ := json.MarshalIndent(bundle.Doctor, "", "  ")
		if err := addFileToZip(zipWriter, "doctor.json", doctorJSON); err != nil {
			return err
		}
	}

	if bundle.Config != nil {
		configJSON, _ := json.MarshalIndent(bundle.Config, "", "  ")
		if err := addFileToZip(zipWriter, "config.json", configJSON); err != nil {
			return err
		}
	}

	return nil
}

// addFileToZip adds a file to the zip archive
func addFileToZip(zipWriter *zip.Writer, filename string, content []byte) error {
	writer, err := zipWriter.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create zip entry: %w", err)
	}

	if _, err := writer.Write(content); err != nil {
		return fmt.Errorf("failed to write zip entry: %w", err)
	}

	return nil
}
