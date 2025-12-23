// Package testdata provides synthetic fixtures for deterministic NFR testing.
// These fixtures allow token budget tests to run in CI without a SCIP index.
package testdata

import (
	"fmt"
	"strings"
)

// Fixture size tiers
const (
	TierSmall  = "small"
	TierMedium = "medium"
	TierLarge  = "large"
)

// SymbolFixture represents a synthetic symbol for testing.
type SymbolFixture struct {
	StableID    string
	Name        string
	Kind        string
	FilePath    string
	Line        int
	Description string
}

// ReferenceFixture represents a synthetic reference.
type ReferenceFixture struct {
	FilePath string
	Line     int
	Column   int
	Kind     string
}

// HotspotFixture represents a synthetic hotspot.
type HotspotFixture struct {
	FilePath string
	Score    float64
	Churn    int
	Recency  string
}

// CallGraphNodeFixture represents a node in a call graph.
type CallGraphNodeFixture struct {
	SymbolID string
	Name     string
	Kind     string
	Callers  []string
	Callees  []string
}

// GenerateSymbols creates n synthetic symbols.
func GenerateSymbols(n int) []SymbolFixture {
	symbols := make([]SymbolFixture, n)
	kinds := []string{"function", "class", "method", "interface", "variable"}

	for i := 0; i < n; i++ {
		symbols[i] = SymbolFixture{
			StableID:    fmt.Sprintf("ckb:test:sym:%08x", i),
			Name:        fmt.Sprintf("Symbol%d", i),
			Kind:        kinds[i%len(kinds)],
			FilePath:    fmt.Sprintf("internal/module%d/file%d.go", i/10, i%10),
			Line:        (i % 500) + 1,
			Description: fmt.Sprintf("Test symbol %d for NFR testing", i),
		}
	}
	return symbols
}

// GenerateReferences creates n synthetic references.
func GenerateReferences(n int) []ReferenceFixture {
	refs := make([]ReferenceFixture, n)
	kinds := []string{"read", "write", "call"}

	for i := 0; i < n; i++ {
		refs[i] = ReferenceFixture{
			FilePath: fmt.Sprintf("internal/module%d/file%d.go", i/20, i%20),
			Line:     (i % 1000) + 1,
			Column:   (i % 80) + 1,
			Kind:     kinds[i%len(kinds)],
		}
	}
	return refs
}

// GenerateHotspots creates n synthetic hotspots.
func GenerateHotspots(n int) []HotspotFixture {
	hotspots := make([]HotspotFixture, n)

	for i := 0; i < n; i++ {
		hotspots[i] = HotspotFixture{
			FilePath: fmt.Sprintf("internal/module%d/file%d.go", i/10, i%10),
			Score:    1.0 - float64(i)/float64(n),
			Churn:    100 - i,
			Recency:  fmt.Sprintf("%dd ago", i+1),
		}
	}
	return hotspots
}

// GenerateCallGraph creates a synthetic call graph with the given depth.
func GenerateCallGraph(rootSymbol string, depth int, branching int) []CallGraphNodeFixture {
	nodes := make([]CallGraphNodeFixture, 0)
	generateCallGraphLevel(rootSymbol, depth, branching, &nodes, 0)
	return nodes
}

func generateCallGraphLevel(symbolID string, depth int, branching int, nodes *[]CallGraphNodeFixture, level int) {
	if level >= depth {
		return
	}

	callers := make([]string, 0, branching)
	callees := make([]string, 0, branching)

	for i := 0; i < branching; i++ {
		callerID := fmt.Sprintf("%s_caller%d_L%d", symbolID, i, level)
		calleeID := fmt.Sprintf("%s_callee%d_L%d", symbolID, i, level)
		callers = append(callers, callerID)
		callees = append(callees, calleeID)
	}

	*nodes = append(*nodes, CallGraphNodeFixture{
		SymbolID: symbolID,
		Name:     fmt.Sprintf("Function_%d", len(*nodes)),
		Kind:     "function",
		Callers:  callers,
		Callees:  callees,
	})

	// Recurse for callees
	for _, callee := range callees {
		generateCallGraphLevel(callee, depth, branching/2+1, nodes, level+1)
	}
}

// FixtureSet contains fixtures for a specific size tier.
type FixtureSet struct {
	Tier       string
	Symbols    []SymbolFixture
	References []ReferenceFixture
	Hotspots   []HotspotFixture
	CallGraph  []CallGraphNodeFixture
}

// SmallFixtures returns fixtures for small result sets.
func SmallFixtures() *FixtureSet {
	return &FixtureSet{
		Tier:       TierSmall,
		Symbols:    GenerateSymbols(20),
		References: GenerateReferences(50),
		Hotspots:   GenerateHotspots(10),
		CallGraph:  GenerateCallGraph("root", 2, 3),
	}
}

// MediumFixtures returns fixtures for medium result sets.
func MediumFixtures() *FixtureSet {
	return &FixtureSet{
		Tier:       TierMedium,
		Symbols:    GenerateSymbols(100),
		References: GenerateReferences(500),
		Hotspots:   GenerateHotspots(50),
		CallGraph:  GenerateCallGraph("root", 3, 4),
	}
}

// LargeFixtures returns fixtures for large result sets (stress test).
func LargeFixtures() *FixtureSet {
	return &FixtureSet{
		Tier:       TierLarge,
		Symbols:    GenerateSymbols(500),
		References: GenerateReferences(5000),
		Hotspots:   GenerateHotspots(200),
		CallGraph:  GenerateCallGraph("root", 4, 5),
	}
}

// ToSearchSymbolsJSON converts symbols to searchSymbols response JSON.
func (f *FixtureSet) ToSearchSymbolsJSON() string {
	var sb strings.Builder
	sb.WriteString(`{"schemaVersion":"1.0","data":{"symbols":[`)

	for i, sym := range f.Symbols {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(
			`{"stableId":"%s","name":"%s","kind":"%s","location":{"path":"%s","line":%d},"description":"%s"}`,
			sym.StableID, sym.Name, sym.Kind, sym.FilePath, sym.Line, sym.Description,
		))
	}

	sb.WriteString(`],"truncated":false,"total":`)
	sb.WriteString(fmt.Sprintf("%d", len(f.Symbols)))
	sb.WriteString(`}}`)
	return sb.String()
}

// ToFindReferencesJSON converts references to findReferences response JSON.
func (f *FixtureSet) ToFindReferencesJSON() string {
	var sb strings.Builder
	sb.WriteString(`{"schemaVersion":"1.0","data":{"references":[`)

	for i, ref := range f.References {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(
			`{"location":{"path":"%s","line":%d,"column":%d},"kind":"%s"}`,
			ref.FilePath, ref.Line, ref.Column, ref.Kind,
		))
	}

	sb.WriteString(`],"truncated":false,"total":`)
	sb.WriteString(fmt.Sprintf("%d", len(f.References)))
	sb.WriteString(`}}`)
	return sb.String()
}

// ToGetHotspotsJSON converts hotspots to getHotspots response JSON.
func (f *FixtureSet) ToGetHotspotsJSON() string {
	var sb strings.Builder
	sb.WriteString(`{"schemaVersion":"1.0","data":{"hotspots":[`)

	for i, h := range f.Hotspots {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(
			`{"path":"%s","score":%.3f,"churn":%d,"recency":"%s"}`,
			h.FilePath, h.Score, h.Churn, h.Recency,
		))
	}

	sb.WriteString(`],"truncated":false,"total":`)
	sb.WriteString(fmt.Sprintf("%d", len(f.Hotspots)))
	sb.WriteString(`}}`)
	return sb.String()
}

// ToGetCallGraphJSON converts call graph to getCallGraph response JSON.
func (f *FixtureSet) ToGetCallGraphJSON() string {
	var sb strings.Builder
	sb.WriteString(`{"schemaVersion":"1.0","data":{"nodes":[`)

	for i, node := range f.CallGraph {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(fmt.Sprintf(
			`{"symbolId":"%s","name":"%s","kind":"%s","callers":[`,
			node.SymbolID, node.Name, node.Kind,
		))
		for j, caller := range node.Callers {
			if j > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(fmt.Sprintf(`"%s"`, caller))
		}
		sb.WriteString(`],"callees":[`)
		for j, callee := range node.Callees {
			if j > 0 {
				sb.WriteString(",")
			}
			sb.WriteString(fmt.Sprintf(`"%s"`, callee))
		}
		sb.WriteString(`]}`)
	}

	sb.WriteString(`],"depth":2,"truncated":false}}`)
	return sb.String()
}
