package main

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/SimplyLiz/CodeMCP/internal/update"
	"github.com/SimplyLiz/CodeMCP/internal/version"

	"github.com/spf13/cobra"
)

var (
	updateDryRun bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update CKB to the latest version",
	Long: `Update CKB to the latest version using the appropriate package manager.

Automatically detects how CKB was installed and runs the correct update command:
  - npm:  npm update -g @tastehub/ckb
  - brew: brew upgrade ckb
  - go:   go install github.com/SimplyLiz/CodeMCP/cmd/ckb@latest

If the installation method cannot be detected, opens the GitHub releases page.`,
	Run: runUpdate,
}

func init() {
	rootCmd.AddCommand(updateCmd)
	updateCmd.Flags().BoolVar(&updateDryRun, "dry-run", false, "Show the update command without executing it")
}

func runUpdate(cmd *cobra.Command, args []string) {
	checker := update.NewChecker()
	method := checker.InstallMethod()

	fmt.Printf("Current version: %s\n", version.Version)
	fmt.Printf("Install method:  %s\n", formatInstallMethod(method))

	switch method {
	case update.InstallMethodNPM:
		runPackageManagerUpdate("npm", []string{"update", "-g", "@tastehub/ckb"})
	case update.InstallMethodBrew:
		runPackageManagerUpdate("brew", []string{"upgrade", "ckb"})
	case update.InstallMethodGo:
		runPackageManagerUpdate("go", []string{"install", "github.com/SimplyLiz/CodeMCP/cmd/ckb@latest"})
	default:
		openReleasesPage()
	}
}

func formatInstallMethod(method update.InstallMethod) string {
	switch method {
	case update.InstallMethodNPM:
		return "npm"
	case update.InstallMethodBrew:
		return "Homebrew"
	case update.InstallMethodGo:
		return "go install"
	default:
		return "unknown"
	}
}

func runPackageManagerUpdate(command string, args []string) {
	cmdStr := command
	for _, arg := range args {
		cmdStr += " " + arg
	}

	if updateDryRun {
		fmt.Printf("\nWould run: %s\n", cmdStr)
		return
	}

	fmt.Printf("\nRunning: %s\n\n", cmdStr)

	execCmd := exec.Command(command, args...)
	execCmd.Stdout = os.Stdout
	execCmd.Stderr = os.Stderr
	execCmd.Stdin = os.Stdin

	if err := execCmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "\nUpdate failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nUpdate complete!")
}

func openReleasesPage() {
	url := "https://github.com/SimplyLiz/CodeMCP/releases"

	fmt.Printf("\nCould not detect installation method.\n")
	fmt.Printf("Please visit: %s\n", url)

	if updateDryRun {
		fmt.Printf("\nWould open: %s\n", url)
		return
	}

	// Try to open the URL in the default browser
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		return
	}

	_ = cmd.Start()
}
