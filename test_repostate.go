package main

import (
	"fmt"
	"os"
	"github.com/ckb/ckb/internal/repostate"
)

func main() {
	cwd, _ := os.Getwd()
	state, err := repostate.ComputeRepoState(cwd)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	
	fmt.Printf("RepoState:\n")
	fmt.Printf("  RepoStateID: %s\n", state.RepoStateID)
	fmt.Printf("  HeadCommit: %s\n", state.HeadCommit)
	fmt.Printf("  StagedDiffHash: %s\n", state.StagedDiffHash)
	fmt.Printf("  WorkingTreeDiffHash: %s\n", state.WorkingTreeDiffHash)
	fmt.Printf("  UntrackedListHash: %s\n", state.UntrackedListHash)
	fmt.Printf("  Dirty: %v\n", state.Dirty)
	fmt.Printf("  ComputedAt: %s\n", state.ComputedAt)
}
