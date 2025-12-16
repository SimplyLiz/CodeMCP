package main

import (
	"os"

	"github.com/ckb/ckb/internal/logging"
)

func main() {
	logger := logging.NewLogger(logging.Config{
		Format: "human",
		Level:  "info",
	})

	if err := rootCmd.Execute(); err != nil {
		logger.Error("Command execution failed", map[string]interface{}{
			"error": err.Error(),
		})
		os.Exit(1)
	}
}
