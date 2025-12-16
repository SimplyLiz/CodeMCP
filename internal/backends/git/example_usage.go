//go:build ignore
// +build ignore

// This file demonstrates example usage of the Git adapter.
// It is not built as part of the package (build ignore tag).

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"

	"ckb/internal/backends/git"
	"ckb/internal/config"
	"ckb/internal/logging"
)

func main() {
	// Example 1: Initialize the Git adapter
	fmt.Println("=== Example 1: Initializing Git Adapter ===")

	cfg := &config.Config{
		RepoRoot: ".",
		Backends: config.BackendsConfig{
			Git: config.GitConfig{
				Enabled: true,
			},
		},
		QueryPolicy: config.QueryPolicyConfig{
			TimeoutMs: map[string]int{
				"git": 5000, // 5 second timeout
			},
		},
	}

	logger := logging.NewLogger(logging.Config{
		Format: logging.HumanFormat,
		Level:  logging.InfoLevel,
		Output: os.Stdout,
	})

	adapter, err := git.NewGitAdapter(cfg, logger)
	if err != nil {
		log.Fatalf("Failed to create Git adapter: %v", err)
	}

	fmt.Printf("Backend ID: %s\n", adapter.ID())
	fmt.Printf("Available: %v\n", adapter.IsAvailable())
	fmt.Printf("Capabilities: %v\n\n", adapter.Capabilities())

	// Example 2: Get repository state for cache keys
	fmt.Println("=== Example 2: Repository State ===")

	state, err := adapter.GetRepoState()
	if err != nil {
		log.Fatalf("Failed to get repo state: %v", err)
	}

	fmt.Printf("Repo State ID: %s\n", state.RepoStateID)
	fmt.Printf("HEAD Commit: %s\n", state.HeadCommit)
	fmt.Printf("Dirty: %v\n", state.Dirty)
	fmt.Printf("Computed At: %s\n\n", state.ComputedAt)

	// Example 3: Get file history
	fmt.Println("=== Example 3: File History ===")

	history, err := adapter.GetFileHistory("internal/config/config.go", 5)
	if err != nil {
		log.Printf("Warning: Could not get file history: %v", err)
	} else {
		fmt.Printf("File: %s\n", history.FilePath)
		fmt.Printf("Total Commits: %d\n", history.CommitCount)
		fmt.Printf("Last Modified: %s\n", history.LastModified)
		fmt.Println("Recent commits:")
		for i, commit := range history.Commits {
			if i >= 3 { // Show only first 3
				break
			}
			fmt.Printf("  [%s] %s - %s\n", commit.Hash[:8], commit.Message, commit.Author)
		}
		fmt.Println()
	}

	// Example 4: Get churn hotspots
	fmt.Println("=== Example 4: Churn Hotspots ===")

	hotspots, err := adapter.GetHotspots(10, "3 months ago")
	if err != nil {
		log.Printf("Warning: Could not get hotspots: %v", err)
	} else {
		fmt.Println("Top 10 files by churn (last 3 months):")
		for i, churn := range hotspots {
			if i >= 5 { // Show only top 5
				break
			}
			fmt.Printf("%2d. %s\n", i+1, churn.FilePath)
			fmt.Printf("    Changes: %d | Authors: %d | Avg Lines: %.1f | Score: %.2f\n",
				churn.ChangeCount, churn.AuthorCount, churn.AverageChanges, churn.HotspotScore)
		}
		fmt.Println()
	}

	// Example 5: Get diff summary
	fmt.Println("=== Example 5: Diff Summary ===")

	summary, err := adapter.GetDiffSummary()
	if err != nil {
		log.Fatalf("Failed to get diff summary: %v", err)
	}

	summaryJSON, _ := json.MarshalIndent(summary, "", "  ")
	fmt.Println(string(summaryJSON))
	fmt.Println()

	// Example 6: Get repository information
	fmt.Println("=== Example 6: Repository Information ===")

	info, err := adapter.GetRepositoryInfo()
	if err != nil {
		log.Fatalf("Failed to get repository info: %v", err)
	}

	fmt.Printf("Repository Root: %s\n", info["repoRoot"])
	fmt.Printf("Current Branch: %s\n", info["branch"])
	fmt.Printf("Remote URL: %s\n", info["remoteURL"])
	fmt.Printf("Staged Files: %v\n", info["stagedFiles"])
	fmt.Printf("Modified Files: %v\n", info["modifiedFiles"])
	fmt.Printf("Untracked Files: %v\n\n", info["untrackedFiles"])

	// Example 7: Get recent commits
	fmt.Println("=== Example 7: Recent Commits ===")

	commits, err := adapter.GetRecentCommits(10)
	if err != nil {
		log.Fatalf("Failed to get recent commits: %v", err)
	}

	fmt.Printf("Last %d commits:\n", len(commits))
	for i, commit := range commits {
		if i >= 5 { // Show only first 5
			break
		}
		fmt.Printf("%2d. [%s] %s\n", i+1, commit.Hash[:8], commit.Message)
		fmt.Printf("    By: %s | At: %s\n", commit.Author, commit.Timestamp)
	}
	fmt.Println()

	// Example 8: Check file status
	fmt.Println("=== Example 8: File Status ===")

	testFile := "internal/config/config.go"
	status, err := adapter.GetFileStatus(testFile)
	if err != nil {
		log.Printf("Warning: Could not get file status: %v", err)
	} else {
		fmt.Printf("Status of %s: %s\n", testFile, status)
	}

	tracked, err := adapter.IsFileTracked(testFile)
	if err != nil {
		log.Printf("Warning: Could not check if file is tracked: %v", err)
	} else {
		fmt.Printf("Is tracked: %v\n", tracked)
	}
	fmt.Println()

	// Example 9: Get file churn metrics
	fmt.Println("=== Example 9: File Churn Metrics ===")

	churn, err := adapter.GetFileChurn(testFile, "6 months ago")
	if err != nil {
		log.Printf("Warning: Could not get file churn: %v", err)
	} else {
		fmt.Printf("Churn metrics for %s (last 6 months):\n", testFile)
		fmt.Printf("  Change Count: %d\n", churn.ChangeCount)
		fmt.Printf("  Author Count: %d\n", churn.AuthorCount)
		fmt.Printf("  Average Changes: %.2f lines/commit\n", churn.AverageChanges)
		fmt.Printf("  Hotspot Score: %.2f\n", churn.HotspotScore)
		fmt.Printf("  Last Modified: %s\n", churn.LastModified)
	}
	fmt.Println()

	// Example 10: Validate repo state ID
	fmt.Println("=== Example 10: Cache Key Validation ===")

	repoStateID, err := adapter.GetRepoStateID()
	if err != nil {
		log.Fatalf("Failed to get repo state ID: %v", err)
	}

	fmt.Printf("Current Repo State ID: %s\n", repoStateID)

	// Simulate cache validation
	isValid, err := adapter.ValidateRepoStateID(repoStateID)
	if err != nil {
		log.Fatalf("Failed to validate repo state ID: %v", err)
	}
	fmt.Printf("State ID is valid: %v\n", isValid)

	fmt.Println("\n=== All Examples Complete ===")
}
