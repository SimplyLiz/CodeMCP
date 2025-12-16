# Impact Analysis Package

This package implements Phase 3.3 of the CKB (Codebase Knowledge Backend) design, providing comprehensive impact analysis for code symbols.

## Overview

The impact analyzer determines:
- **Visibility** of symbols (public, internal, private)
- **Impact classification** of references (direct caller, type dependency, etc.)
- **Risk assessment** based on multiple factors
- **Analysis limitations** and confidence scores

## Package Structure

```
internal/impact/
├── types.go           # Core data types (Symbol, Reference, Location)
├── visibility.go      # Visibility derivation logic
├── classification.go  # Impact classification
├── risk.go           # Risk scoring algorithms
├── limits.go         # Analysis limitation tracking
├── analyzer.go       # Main analyzer implementation
├── doc.go            # Package documentation
└── *_test.go         # Comprehensive test suite
```

## Core Types

### Symbol
Represents a code symbol with metadata:
```go
type Symbol struct {
    StableId            string      // Unique identifier
    Name                string      // Symbol name
    Kind                SymbolKind  // class, function, method, etc.
    Signature           string      // Full signature
    SignatureNormalized string      // Normalized signature
    ModuleId            string      // Module identifier
    ModuleName          string      // Module name
    ContainerName       string      // Container (class, namespace)
    Location            *Location   // Source location
    Modifiers           []string    // SCIP modifiers
}
```

### Reference
Represents a reference to a symbol:
```go
type Reference struct {
    Location   *Location     // Where the reference occurs
    Kind       ReferenceKind // call, read, write, type, implements
    FromSymbol string        // Referencing symbol ID
    FromModule string        // Referencing module ID
    IsTest     bool          // Whether from test code
}
```

### ImpactItem
Represents a single impact:
```go
type ImpactItem struct {
    StableId   string          // Impacted symbol ID
    Name       string          // Impacted symbol name
    Kind       ImpactKind      // Impact kind
    Confidence float64         // Confidence score (0.0-1.0)
    ModuleId   string          // Module ID
    ModuleName string          // Module name
    Location   *Location       // Impact location
    Visibility *VisibilityInfo // Visibility info
    Distance   int             // Distance from original (1=direct)
}
```

## Visibility Derivation

The analyzer uses a cascading strategy with three levels:

### 1. SCIP Modifiers (Confidence: 0.95)
Explicit modifiers from static analysis:
- `public`, `private`, `internal`, `protected`, `package`

### 2. Reference Analysis (Confidence: 0.7-0.9)
- External module references → Public
- Same module references only → Internal

### 3. Naming Conventions (Confidence: 0.5-0.7)
Language-specific patterns:
- `_prefix` → Private (Python, TypeScript)
- `#prefix` → Private (Ruby)
- `__prefix` → Private (Python name mangling)
- `Uppercase` → Public (Go)
- `lowercase` → Internal (Go)

## Impact Classification

References are classified into:

| Kind | Description | Confidence |
|------|-------------|------------|
| DirectCaller | Direct calls or property access | 0.90-0.95 |
| TransitiveCaller | Indirect through call chain | 0.85 |
| TypeDependency | Type references | 0.80 |
| TestDependency | References from tests | 0.90 |
| ImplementsInterface | Interface implementation | 0.95 |

## Risk Scoring

Risk is computed using weighted factors:

| Factor | Weight | Description |
|--------|--------|-------------|
| Visibility | 30% | Public = higher risk |
| Direct Callers | 35% | More callers = higher risk |
| Module Spread | 25% | More modules = higher risk |
| Impact Kind | 10% | Breaking impacts = higher risk |

### Risk Levels

- **High** (0.7-1.0): Many callers, public visibility, multiple modules
- **Medium** (0.4-0.69): Moderate usage, internal visibility
- **Low** (0.0-0.39): Few/no callers, private visibility

## Usage Examples

### Basic Analysis

```go
package main

import (
    "fmt"
    "github.com/yourusername/ckb/internal/impact"
)

func main() {
    // Create analyzer with depth 2
    analyzer := impact.NewImpactAnalyzer(2)

    // Define symbol
    symbol := &impact.Symbol{
        StableId:   "com.example.Service.processData",
        Name:       "processData",
        Kind:       impact.KindMethod,
        Signature:  "public Result processData(Input input)",
        ModuleId:   "com.example.core",
        ModuleName: "core-module",
        Modifiers:  []string{"public"},
    }

    // Define references
    refs := []impact.Reference{
        {
            Location: &impact.Location{
                FileId:    "Controller.java",
                StartLine: 45,
            },
            Kind:       impact.RefCall,
            FromSymbol: "com.example.api.Controller.handleRequest",
            FromModule: "com.example.api",
            IsTest:     false,
        },
    }

    // Perform analysis
    result, err := analyzer.Analyze(symbol, refs)
    if err != nil {
        panic(err)
    }

    // Print results
    fmt.Printf("Symbol: %s\n", result.Symbol.Name)
    fmt.Printf("Visibility: %s (%.0f%% confident from %s)\n",
        result.Visibility.Visibility,
        result.Visibility.Confidence*100,
        result.Visibility.Source)
    fmt.Printf("Risk: %s (score: %.2f)\n",
        result.RiskScore.Level,
        result.RiskScore.Score)
    fmt.Printf("Explanation: %s\n", result.RiskScore.Explanation)
    fmt.Printf("Direct impacts: %d\n", len(result.DirectImpact))

    // Show risk factors
    for _, factor := range result.RiskScore.Factors {
        fmt.Printf("  - %s: %.2f (weight: %.0f%%)\n",
            factor.Name, factor.Value, factor.Weight*100)
    }

    // Show module summaries
    for _, module := range result.ModulesAffected {
        fmt.Printf("Module %s: %d impacts (max risk: %s)\n",
            module.Name, module.ImpactCount, module.MaxRisk)
    }
}
```

### Analysis with Options

```go
// Exclude test dependencies
opts := impact.AnalyzeOptions{
    IncludeTests: false,
    MaxDepth:     3,
}
result, err := analyzer.AnalyzeWithOptions(symbol, refs, opts)
```

### Check Breaking Changes

```go
for _, ref := range refs {
    if impact.IsBreakingChange(&ref, symbol, "signature-change") {
        fmt.Printf("Breaking: %s:%d\n",
            ref.Location.FileId,
            ref.Location.StartLine)
    }
}
```

### Direct Visibility Derivation

```go
// Derive visibility independently
visInfo := impact.DeriveVisibility(symbol, refs)
fmt.Printf("Visibility: %s (source: %s, confidence: %.2f)\n",
    visInfo.Visibility, visInfo.Source, visInfo.Confidence)
```

### Impact Classification

```go
// Classify a single reference
kind := impact.ClassifyImpact(&ref, symbol)
kind, confidence := impact.ClassifyImpactWithConfidence(&ref, symbol)
fmt.Printf("Impact: %s (confidence: %.2f)\n", kind, confidence)
```

## Analysis Limitations

The analyzer tracks its own limitations:

```go
if result.AnalysisLimits.HasLimitations() {
    fmt.Printf("Type context: %s\n", result.AnalysisLimits.TypeContext)
    for _, note := range result.AnalysisLimits.Notes {
        fmt.Printf("  - %s\n", note)
    }
}
```

Type context levels:
- **Full**: Complete type information available
- **Partial**: Some type information available
- **None**: No type information available

## Integration Points

This package integrates with:
- `internal/config`: Configuration settings
- `internal/logging`: Structured logging
- Backend packages (SCIP, LSP): Symbol and reference data

## Testing

Comprehensive test suite included:

```bash
go test ./internal/impact/... -v
go test ./internal/impact/... -cover
```

Test coverage:
- Unit tests for all functions
- Integration tests for analyzer
- Edge case handling
- Confidence score validation

## Future Enhancements (v2)

- Transitive impact analysis with actual call graph traversal
- Test coverage factor in risk scoring
- Machine learning-based confidence adjustment
- Historical change impact correlation
- Cross-language type inference improvements
