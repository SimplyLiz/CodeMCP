package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ckb/ckb/internal/config"
	"github.com/ckb/ckb/internal/errors"
	"github.com/ckb/ckb/internal/logging"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Initialize CKB configuration",
	Long:  "Creates a .ckb/ directory with default configuration in the current repository root",
	RunE:  runInit,
}

func init() {
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
	if _, err := os.Stat(ckbDir); err == nil {
		return errors.NewCkbError(
			errors.InternalError,
			fmt.Sprintf(".ckb directory already exists at %s", ckbDir),
			nil,
			[]errors.FixAction{
				{
					Type:        "run-command",
					Command:     "rm -rf .ckb && ckb init",
					Safe:        false,
					Description: "Remove existing .ckb directory and reinitialize",
				},
			},
			nil,
		)
	}

	// Create .ckb directory
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		return errors.NewCkbError(errors.InternalError, "Failed to create .ckb directory", err, nil, nil)
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

	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return errors.NewCkbError(errors.InternalError, "Failed to write config file", err, nil, nil)
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
