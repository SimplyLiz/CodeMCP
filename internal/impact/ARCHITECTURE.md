# Impact Analyzer Architecture

## Package Structure

```
internal/impact/
│
├─── Core Data Types (types.go)
│    ├─ Symbol
│    ├─ Reference
│    ├─ Location
│    ├─ SymbolKind
│    └─ ReferenceKind
│
├─── Visibility Module (visibility.go)
│    ├─ Visibility enum
│    ├─ VisibilityInfo
│    └─ DeriveVisibility()
│         ├─ deriveFromModifiers()      [Confidence: 0.95]
│         ├─ deriveFromReferences()     [Confidence: 0.7-0.9]
│         └─ deriveFromNaming()         [Confidence: 0.5-0.7]
│
├─── Classification Module (classification.go)
│    ├─ ImpactKind enum
│    ├─ ImpactItem
│    └─ Classification Functions
│         ├─ ClassifyImpact()
│         ├─ ClassifyImpactWithConfidence()
│         └─ IsBreakingChange()
│
├─── Risk Scoring Module (risk.go)
│    ├─ RiskLevel enum
│    ├─ RiskScore
│    ├─ RiskFactor
│    └─ ComputeRiskScore()
│         ├─ calculateVisibilityRisk()      [Weight: 0.30]
│         ├─ calculateDirectCallerRisk()    [Weight: 0.35]
│         ├─ calculateModuleSpreadRisk()    [Weight: 0.25]
│         └─ calculateImpactKindRisk()      [Weight: 0.10]
│
├─── Limits Module (limits.go)
│    ├─ TypeContextLevel enum
│    ├─ AnalysisLimits
│    └─ DetermineTypeContext()
│
└─── Main Analyzer (analyzer.go)
     ├─ ImpactAnalyzer
     ├─ ImpactAnalysisResult
     ├─ ModuleSummary
     ├─ AnalyzeOptions
     └─ Analysis Functions
          ├─ NewImpactAnalyzer()
          ├─ Analyze()
          ├─ AnalyzeWithOptions()
          ├─ processDirectReferences()
          └─ generateModuleSummaries()
```

## Data Flow

```
Input: Symbol + References
         │
         ├──────────────────────────────────────┐
         │                                      │
         ▼                                      ▼
  [Visibility Derivation]            [Direct Reference Processing]
         │                                      │
         │ VisibilityInfo                       │ []ImpactItem
         │ (with confidence)                    │
         │                                      │
         └───────────┬──────────────────────────┘
                     │
                     ▼
            [Impact Classification]
                     │
                     │ Classified ImpactItems
                     │ (with confidence)
                     │
                     ▼
              [Risk Scoring]
                     │
                     │ RiskScore
                     │ (level, score, factors)
                     │
                     ▼
           [Module Summarization]
                     │
                     │ []ModuleSummary
                     │
                     ▼
           [Limits Assessment]
                     │
                     │ AnalysisLimits
                     │
                     ▼
         ImpactAnalysisResult
```

## Visibility Derivation Flow

```
Symbol + References
         │
         ▼
    Has SCIP Modifiers? ──Yes──▶ [Extract Modifiers]
         │                             │
         No                            │ public/private/internal
         │                             │ Confidence: 0.95
         ▼                             │
    Has References? ──Yes──▶ [Analyze References]
         │                             │
         No                            │ External → Public (0.9)
         │                             │ Internal → Internal (0.7)
         ▼                             │
    Has Naming Pattern? ──Yes──▶ [Check Naming]
         │                             │
         No                            │ _prefix → Private (0.6)
         │                             │ Uppercase → Public (0.5)
         ▼                             │
    Return Unknown                     │
    Confidence: 0.0                    │
         │                             │
         └──────────┬──────────────────┘
                    │
                    ▼
              VisibilityInfo
```

## Risk Calculation Flow

```
Symbol + ImpactItems
         │
         ├─────────────┬─────────────┬──────────────┐
         │             │             │              │
         ▼             ▼             ▼              ▼
    Visibility    Direct Callers  Module Spread  Impact Kind
    Analysis      Analysis        Analysis       Analysis
         │             │             │              │
         │ 0.0-1.0     │ 0.0-1.0     │ 0.0-1.0      │ 0.0-1.0
         │ × 0.30      │ × 0.35      │ × 0.25       │ × 0.10
         │             │             │              │
         └─────────────┴─────────────┴──────────────┘
                       │
                       ▼
                Weighted Sum ──▶ Total Score (0.0-1.0)
                       │
                       ▼
              Determine Risk Level
                       │
         ┌─────────────┼─────────────┐
         │             │             │
         ▼             ▼             ▼
    < 0.4         0.4-0.69       >= 0.7
       │             │             │
       ▼             ▼             ▼
      Low         Medium         High
```

## Classification Decision Tree

```
Reference
    │
    ▼
Is Test? ──Yes──▶ TestDependency (0.90)
    │
    No
    │
    ▼
RefKind?
    │
    ├─ RefCall ──────────────▶ DirectCaller (0.95)
    │
    ├─ RefImplements ────────▶ ImplementsInterface (0.95)
    │
    ├─ RefExtends ───────────▶ DirectCaller (0.95)
    │
    ├─ RefType ──────────────▶ TypeDependency (0.80)
    │
    └─ RefRead/RefWrite
            │
            ▼
      Symbol Kind?
            │
            ├─ Property/Variable/Constant ──▶ DirectCaller (0.90)
            │
            └─ Other ────────────────────────▶ TypeDependency (0.80)
```

## Module Summary Aggregation

```
ImpactItems
    │
    ▼
Group by ModuleId
    │
    ├─ Module1 ──▶ Count: 5  ──▶ MaxRisk: High
    │                 │
    │                 └─ [DirectCaller, DirectCaller, TypeDep, ...]
    │
    ├─ Module2 ──▶ Count: 2  ──▶ MaxRisk: Medium
    │                 │
    │                 └─ [TypeDep, TypeDep]
    │
    └─ Module3 ──▶ Count: 1  ──▶ MaxRisk: Low
                      │
                      └─ [TestDependency]
    │
    ▼
Sort by ImpactCount (descending)
    │
    ▼
[]ModuleSummary
```

## Confidence Score Distribution

```
Source                    Confidence Range    Typical Use Case
─────────────────────────────────────────────────────────────
SCIP Modifiers           0.95                Statically analyzed code
External References      0.90                Cross-module calls
Direct Caller (Call)     0.95                Function/method calls
Interface Implementation 0.95                Interface relationships
Internal References      0.70                Same-module references
Direct Caller (Access)   0.90                Property/field access
Test Dependencies        0.90                Test code references
Transitive Callers       0.85                Indirect callers
Type Dependencies        0.80                Type references
Naming (Double _)        0.70                Python name mangling
Naming (Single _)        0.60                Private prefix convention
Naming (Go uppercase)    0.50                Go exported symbols
Unknown                  0.50                Low confidence fallback
```

## Type Context Determination

```
Symbol + References
         │
         ▼
    Has Signature? ────────┐
         │                 │
         │                 Yes
         ▼                 │
    Has Typed Refs? ───────┤
         │                 │
         │                 Yes
         │                 │
    ┌────┴────┬────────────┘
    │         │
    No        Yes
    │         │
    ▼         ▼
  TypeContextNone   ┌──▶ Both? ──Yes──▶ TypeContextFull
                    │      │
                    └──────┘
                           No
                           │
                           ▼
                    TypeContextPartial
```

## Integration Points

```
┌─────────────────────────────────────────────────────┐
│                  CKB System                         │
│                                                     │
│  ┌───────────────┐         ┌──────────────────┐   │
│  │ SCIP Backend  │────────▶│ Impact Analyzer  │   │
│  │ - Modifiers   │         │                  │   │
│  │ - References  │         │  Visibility      │   │
│  └───────────────┘         │  Classification  │   │
│                            │  Risk Scoring    │   │
│  ┌───────────────┐         │  Limits          │   │
│  │  LSP Backend  │────────▶│                  │   │
│  │ - Real-time   │         └────────┬─────────┘   │
│  │ - Symbols     │                  │             │
│  └───────────────┘                  │             │
│                                     │             │
│  ┌───────────────┐                  │             │
│  │  Git Backend  │                  │             │
│  │ - History     │                  ▼             │
│  └───────────────┘         ┌──────────────────┐   │
│                            │  Query Engine    │   │
│  ┌───────────────┐         │  - Symbol Lookup │   │
│  │    Config     │────────▶│  - Impact Query  │   │
│  │    Logging    │         │  - Risk Reports  │   │
│  └───────────────┘         └──────────────────┘   │
│                                                     │
└─────────────────────────────────────────────────────┘
```

## Performance Characteristics

```
Operation                    Complexity    Notes
─────────────────────────────────────────────────────────────
Visibility Derivation        O(n)          n = number of references
Impact Classification        O(n)          n = number of references
Risk Scoring                 O(n)          n = number of impact items
Module Summarization         O(n log n)    n = impacts, sorted
Full Analysis                O(n log n)    Dominated by sorting

Memory Usage:
- Per Symbol:                ~200 bytes
- Per Reference:             ~150 bytes
- Per ImpactItem:            ~250 bytes
- Analysis Result:           ~1-10 KB (typical)
```

## Error Handling Strategy

```
Input Validation
    │
    ├─ Nil Symbol ──────────▶ Return Error
    │
    ├─ Empty References ─────▶ Continue (valid case)
    │
    └─ Invalid Data ─────────▶ Log Warning, Use Defaults
         │
         ▼
Core Analysis (Never Fails)
    │
    ├─ Visibility Derivation ──▶ Falls back to Unknown
    │
    ├─ Classification ──────────▶ Falls back to Unknown
    │
    └─ Risk Scoring ────────────▶ Returns Low (safe default)
         │
         ▼
Result Construction (Always Succeeds)
    │
    └─ Includes AnalysisLimits to document any issues
```

## Extension Points

```
Future Enhancements:
│
├─ Transitive Analysis
│  └─ Requires: Call graph traversal
│     Implementation: recursive Analyze() with depth tracking
│
├─ Test Coverage Factor
│  └─ Requires: Test execution data
│     Integration: New risk factor (weight 0.05-0.10)
│
├─ ML-based Confidence
│  └─ Requires: Historical accuracy data
│     Implementation: Confidence adjuster post-processor
│
└─ Cross-Language Type Inference
   └─ Requires: Type system mappings
      Implementation: Enhanced DetermineTypeContext()
```
