# Impact Analyzer Quick Reference

## Quick Start

```go
import "github.com/ckb/ckb/internal/impact"

// 1. Create analyzer
analyzer := impact.NewImpactAnalyzer(2)

// 2. Define symbol
symbol := &impact.Symbol{
    StableId:  "package.Class.method",
    Name:      "method",
    Kind:      impact.KindMethod,
    Modifiers: []string{"public"},
    /* ... */
}

// 3. Define references
refs := []impact.Reference{
    {
        Kind:       impact.RefCall,
        FromSymbol: "caller.id",
        FromModule: "other.module",
    },
}

// 4. Analyze
result, _ := analyzer.Analyze(symbol, refs)

// 5. Use results
fmt.Printf("Risk: %s\n", result.RiskScore.Level)
```

## Core Types Cheat Sheet

### Symbol
```go
&impact.Symbol{
    StableId:            string      // Required: Unique ID
    Name:                string      // Required: Display name
    Kind:                SymbolKind  // Required: class/function/method/etc
    Signature:           string      // Optional: Full signature
    SignatureNormalized: string      // Optional: Normalized form
    ModuleId:            string      // Required: Module ID
    ModuleName:          string      // Optional: Display name
    ContainerName:       string      // Optional: Parent container
    Location:            *Location   // Optional: Source location
    Modifiers:           []string    // Optional: SCIP modifiers
}
```

### Reference
```go
&impact.Reference{
    Location:   *Location     // Required: Reference location
    Kind:       ReferenceKind // Required: call/read/write/type/implements/extends
    FromSymbol: string        // Required: Referencing symbol ID
    FromModule: string        // Required: Referencing module ID
    IsTest:     bool          // Required: Test code flag
}
```

## Enums

### SymbolKind
```go
KindClass      // "class"
KindInterface  // "interface"
KindFunction   // "function"
KindMethod     // "method"
KindProperty   // "property"
KindVariable   // "variable"
KindConstant   // "constant"
KindType       // "type"
```

### ReferenceKind
```go
RefCall       // "call"       - Function/method invocation
RefRead       // "read"       - Read access
RefWrite      // "write"      - Write access
RefType       // "type"       - Type reference
RefImplements // "implements" - Interface implementation
RefExtends    // "extends"    - Class inheritance
```

### Visibility
```go
VisibilityPublic   // "public"   - Externally visible
VisibilityInternal // "internal" - Package/module visible
VisibilityPrivate  // "private"  - Class/file visible
VisibilityUnknown  // "unknown"  - Cannot determine
```

### ImpactKind
```go
DirectCaller        // "direct-caller"
TransitiveCaller    // "transitive-caller"
TypeDependency      // "type-dependency"
TestDependency      // "test-dependency"
ImplementsInterface // "implements-interface"
Unknown             // "unknown"
```

### RiskLevel
```go
RiskHigh   // "high"   - Score >= 0.7
RiskMedium // "medium" - Score 0.4-0.69
RiskLow    // "low"    - Score < 0.4
```

## Function Quick Reference

### Analysis
| Function | Input | Output | Purpose |
|----------|-------|--------|---------|
| `NewImpactAnalyzer(depth)` | int | `*ImpactAnalyzer` | Create analyzer |
| `Analyze(symbol, refs)` | `*Symbol`, `[]Reference` | `*ImpactAnalysisResult`, error | Full analysis |
| `AnalyzeWithOptions(symbol, refs, opts)` | `*Symbol`, `[]Reference`, `AnalyzeOptions` | `*ImpactAnalysisResult`, error | Custom analysis |

### Visibility
| Function | Input | Output | Purpose |
|----------|-------|--------|---------|
| `DeriveVisibility(symbol, refs)` | `*Symbol`, `[]Reference` | `*VisibilityInfo` | Determine visibility |

### Classification
| Function | Input | Output | Purpose |
|----------|-------|--------|---------|
| `ClassifyImpact(ref, symbol)` | `*Reference`, `*Symbol` | `ImpactKind` | Classify reference |
| `ClassifyImpactWithConfidence(ref, symbol)` | `*Reference`, `*Symbol` | `ImpactKind`, float64 | Classify + confidence |
| `IsBreakingChange(ref, symbol, changeType)` | `*Reference`, `*Symbol`, string | bool | Check breaking change |

### Risk Scoring
| Function | Input | Output | Purpose |
|----------|-------|--------|---------|
| `ComputeRiskScore(symbol, impact)` | `*Symbol`, `[]ImpactItem` | `*RiskScore` | Calculate risk |

### Limits
| Function | Input | Output | Purpose |
|----------|-------|--------|---------|
| `NewAnalysisLimits()` | - | `*AnalysisLimits` | Create limits tracker |
| `DetermineTypeContext(symbol, refs)` | `*Symbol`, `[]Reference` | `TypeContextLevel` | Evaluate type context |

## Result Access Patterns

### Basic Info
```go
result.Symbol.Name                    // Symbol name
result.Visibility.Visibility          // public/internal/private
result.Visibility.Confidence          // 0.0-1.0
result.Visibility.Source              // scip-modifiers/ref-analysis/naming-convention
```

### Risk Assessment
```go
result.RiskScore.Level                // high/medium/low
result.RiskScore.Score                // 0.0-1.0
result.RiskScore.Explanation          // Human-readable text
result.RiskScore.Factors              // []RiskFactor

for _, factor := range result.RiskScore.Factors {
    factor.Name                       // visibility/direct-callers/module-spread/impact-kind
    factor.Weight                     // 0.0-1.0
    factor.Value                      // 0.0-1.0
}
```

### Impact Items
```go
len(result.DirectImpact)              // Count of direct impacts
len(result.TransitiveImpact)          // Count of transitive impacts

for _, item := range result.DirectImpact {
    item.StableId                     // Impacted symbol ID
    item.Name                         // Impacted symbol name
    item.Kind                         // Impact kind
    item.Confidence                   // 0.0-1.0
    item.ModuleId                     // Module ID
    item.Location                     // Source location
    item.Distance                     // 1=direct, 2+=transitive
}
```

### Module Summary
```go
len(result.ModulesAffected)           // Count of affected modules

for _, module := range result.ModulesAffected {
    module.ModuleId                   // Module ID
    module.Name                       // Module name
    module.ImpactCount                // Number of impacts
    module.MaxRisk                    // Highest risk in module
}
```

### Limitations
```go
result.AnalysisLimits.TypeContext     // full/partial/none
result.AnalysisLimits.Notes           // []string
result.AnalysisLimits.HasLimitations() // bool
```

## Common Patterns

### Filter Test Impacts
```go
opts := impact.AnalyzeOptions{IncludeTests: false}
result, _ := analyzer.AnalyzeWithOptions(symbol, refs, opts)
```

### Check High Risk Only
```go
if result.RiskScore.Level == impact.RiskHigh {
    // Handle high risk
}
```

### Find Breaking Changes
```go
for _, ref := range refs {
    if impact.IsBreakingChange(&ref, symbol, "signature-change") {
        fmt.Printf("Breaking: %s\n", ref.Location.FileId)
    }
}
```

### Count External Impacts
```go
externalCount := 0
for _, item := range result.DirectImpact {
    if item.ModuleId != symbol.ModuleId {
        externalCount++
    }
}
```

### Get Top Risk Modules
```go
// Already sorted by impact count
topModules := result.ModulesAffected[:min(5, len(result.ModulesAffected))]
```

## Confidence Score Guide

| Source | Confidence | When to Use |
|--------|-----------|-------------|
| SCIP Modifiers | 0.95 | Statically analyzed code |
| External Refs | 0.90 | Cross-module references |
| Direct Call | 0.95 | Function/method calls |
| Property Access | 0.90 | Field/property access |
| Internal Refs | 0.70 | Same-module references |
| Type Refs | 0.80 | Type dependencies |
| Naming (strong) | 0.70 | `__private` (Python) |
| Naming (weak) | 0.60 | `_private` |
| Naming (weak) | 0.50 | Go capitalization |

## Risk Factor Weights

| Factor | Weight | Notes |
|--------|--------|-------|
| Visibility | 30% | Public > Internal > Private |
| Direct Callers | 35% | Logarithmic: 1→0.3, 5→0.6, 20+→1.0 |
| Module Spread | 25% | Logarithmic: 1→0.2, 3→0.5, 10+→1.0 |
| Impact Kind | 10% | Interface>Direct>Transitive>Type |

## Change Type Reference

| Change Type | Affects | Example |
|-------------|---------|---------|
| `signature-change` | Calls, Types | Adding parameter |
| `rename` | All | Changing symbol name |
| `remove` | All | Deleting symbol |
| `visibility-change` | External only | public→private |
| `behavioral-change` | Callers | Logic changes |

## Troubleshooting

### Low Confidence Visibility
**Problem:** Visibility confidence < 0.7
**Solution:** Add SCIP modifiers or improve reference data

### Unknown Impact Kind
**Problem:** ImpactKind = Unknown
**Solution:** Check Reference.Kind is valid enum value

### Missing Module Info
**Problem:** ModuleSummary empty
**Solution:** Ensure Reference.FromModule is populated

### Zero Risk Score
**Problem:** RiskScore.Score = 0.0
**Solution:** Verify impact items have valid data

## Performance Tips

1. **Batch Analysis**: Analyze multiple symbols in parallel
2. **Limit Depth**: Use depth=1 for quick analysis
3. **Filter Early**: Use AnalyzeOptions to reduce processing
4. **Reuse Analyzer**: Create once, use for multiple analyses
5. **Cache Results**: Results are immutable, safe to cache

## Error Handling

```go
result, err := analyzer.Analyze(symbol, refs)
if err != nil {
    // Only error: nil symbol
    log.Fatal("Symbol is required")
}

// Check for limitations
if result.AnalysisLimits.HasLimitations() {
    for _, note := range result.AnalysisLimits.Notes {
        log.Printf("Limitation: %s", note)
    }
}
```

## Integration Examples

### With Config
```go
import "github.com/ckb/ckb/internal/config"

cfg, _ := config.LoadConfig(".")
analyzer := impact.NewImpactAnalyzer(2)

// Respect budget limits
maxImpacts := cfg.Budget.MaxImpactItems
impacts := result.DirectImpact[:min(maxImpacts, len(result.DirectImpact))]
```

### With Logging
```go
import "github.com/ckb/ckb/internal/logging"

logger := logging.NewLogger(logging.Config{
    Level:  logging.InfoLevel,
    Format: logging.HumanFormat,
})

result, _ := analyzer.Analyze(symbol, refs)
logger.Info("Impact analysis complete", map[string]interface{}{
    "symbol":          result.Symbol.Name,
    "risk":           result.RiskScore.Level,
    "direct_impacts": len(result.DirectImpact),
})
```

## Version Info

- Package: `internal/impact`
- Spec: Design Document Section 8
- Version: 1.0 (Phase 3.3)
- Last Updated: 2025-12-16
