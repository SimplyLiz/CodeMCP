package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"ckb/internal/config"
	"ckb/internal/errors"
	"ckb/internal/logging"

	"github.com/spf13/cobra"
)

var (
	initForce bool
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize CKB configuration",
	Long:  "Creates a .ckb/ directory with default configuration in the current repository root",
	RunE:  runInit,
}

func init() {
	initCmd.Flags().BoolVarP(&initForce, "force", "f", false, "Force reinitialization (removes existing .ckb directory)")
	rootCmd.AddCommand(initCmd)
}

func runInit(cmd *cobra.Command, args []string) error {
	logger := logging.NewLogger(logging.Config{
		Format: "human",
		Level:  "info",
	})

	// Get current directory
	cwd, err := os.Getwd()
	if err != nil {
		return errors.NewCkbError(errors.InternalError, "Failed to get current directory", err, nil, nil)
	}

	// Check if .ckb already exists
	ckbDir := filepath.Join(cwd, ".ckb")
	if _, statErr := os.Stat(ckbDir); statErr == nil {
		if !initForce {
			// Idempotent behavior: already initialized is success (CI-friendly)
			fmt.Println("CKB already initialized.")
			fmt.Printf("Configuration at: %s\n", filepath.Join(ckbDir, "config.json"))
			fmt.Println("\nRun 'ckb init --force' to reinitialize.")
			return nil
		}
		// Remove existing directory
		if removeErr := os.RemoveAll(ckbDir); removeErr != nil {
			return errors.NewCkbError(errors.InternalError, "Failed to remove existing .ckb directory", removeErr, nil, nil)
		}
		logger.Info("Removed existing .ckb directory", nil)
	}

	// Create .ckb directory
	if mkdirErr := os.MkdirAll(ckbDir, 0755); mkdirErr != nil {
		return errors.NewCkbError(errors.InternalError, "Failed to create .ckb directory", mkdirErr, nil, nil)
	}

	// Create default config
	cfg := config.DefaultConfig()
	cfg.RepoRoot = "."

	// Write config file
	configPath := filepath.Join(ckbDir, "config.json")
	configData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return errors.NewCkbError(errors.InternalError, "Failed to marshal config", err, nil, nil)
	}

	if writeErr := os.WriteFile(configPath, configData, 0644); writeErr != nil {
		return errors.NewCkbError(errors.InternalError, "Failed to write config file", writeErr, nil, nil)
	}

	logger.Info("CKB initialized successfully", map[string]interface{}{
		"config_path": configPath,
	})

	fmt.Println("CKB initialized successfully!")
	fmt.Printf("Configuration written to: %s\n", configPath)
	fmt.Println("\nNext steps:")
	fmt.Println("  1. Run 'ckb doctor' to check your setup")
	fmt.Println("  2. Run 'ckb status' to see system status")

	return nil
}
