// Package impact provides impact analysis capabilities for code symbols.
//
// The impact analyzer can:
//   - Derive symbol visibility from SCIP modifiers, reference analysis, or naming conventions
//   - Classify references into impact kinds (direct caller, transitive caller, type dependency, etc.)
//   - Compute risk scores based on visibility, caller count, module spread, and impact types
//   - Generate comprehensive impact analysis results with limitations noted
//
// Basic usage:
//
//	// Create an analyzer with max depth of 2 for transitive analysis
//	analyzer := impact.NewImpactAnalyzer(2)
//
//	// Define the symbol to analyze
//	symbol := &impact.Symbol{
//	    StableId:   "com.example.MyClass.myMethod",
//	    Name:       "myMethod",
//	    Kind:       impact.KindMethod,
//	    Signature:  "public void myMethod(String arg)",
//	    ModuleId:   "com.example",
//	    ModuleName: "example-module",
//	    Modifiers:  []string{"public"},
//	}
//
//	// Define references to the symbol
//	refs := []impact.Reference{
//	    {
//	        Location:   &impact.Location{FileId: "Caller.java", StartLine: 42, StartColumn: 10},
//	        Kind:       impact.RefCall,
//	        FromSymbol: "com.example.Caller.callMethod",
//	        FromModule: "com.example.other",
//	        IsTest:     false,
//	    },
//	}
//
//	// Perform analysis
//	result, err := analyzer.Analyze(symbol, refs)
//	if err != nil {
//	    log.Fatal(err)
//	}
//
//	// Access results
//	fmt.Printf("Visibility: %s (confidence: %.2f)\n",
//	    result.Visibility.Visibility, result.Visibility.Confidence)
//	fmt.Printf("Risk: %s (score: %.2f)\n",
//	    result.RiskScore.Level, result.RiskScore.Score)
//	fmt.Printf("Direct impacts: %d\n", len(result.DirectImpact))
//	fmt.Printf("Modules affected: %d\n", len(result.ModulesAffected))
//
// Visibility Derivation:
//
// The analyzer uses a cascading fallback strategy to determine visibility:
//
//  1. SCIP/Glean modifiers (confidence 0.95)
//     - Explicit public/private/internal modifiers from static analysis
//
//  2. Reference analysis (confidence 0.9 for public, 0.7 for internal)
//     - If referenced from external modules -> public
//     - If only referenced within same module -> internal
//
//  3. Naming conventions (confidence 0.5-0.7)
//     - Underscore prefix (_) -> private (Python, TypeScript)
//     - Hash prefix (#) -> private (Ruby)
//     - Double underscore (__) -> private (Python name mangling)
//     - Uppercase first letter -> public (Go)
//     - Lowercase first letter -> internal (Go)
//
// Impact Classification:
//
// References are classified into impact kinds:
//
//   - DirectCaller: Direct function/method calls or property access
//   - TransitiveCaller: Indirect callers through call chain
//   - TypeDependency: Type references in signatures, parameters, etc.
//   - TestDependency: References from test code
//   - ImplementsInterface: Interface implementation relationships
//
// Risk Scoring:
//
// Risk is calculated using weighted factors:
//
//   - Visibility (weight 0.3): Public symbols have higher risk
//   - Direct callers (weight 0.35): More callers = higher risk
//   - Module spread (weight 0.25): More affected modules = higher risk
//   - Impact kind (weight 0.1): Breaking impacts = higher risk
//
// The final score (0.0-1.0) is mapped to risk levels:
//   - Low: 0.0 - 0.39
//   - Medium: 0.4 - 0.69
//   - High: 0.7 - 1.0
//
// Advanced Usage:
//
// Use AnalyzeWithOptions for custom analysis:
//
//	opts := impact.AnalyzeOptions{
//	    MaxDepth:            3,     // Override default depth
//	    IncludeTests:        false, // Exclude test dependencies
//	    OnlyBreakingChanges: true,  // Only show breaking changes
//	}
//	result, err := analyzer.AnalyzeWithOptions(symbol, refs, opts)
//
// Check for breaking changes:
//
//	for _, ref := range refs {
//	    if impact.IsBreakingChange(&ref, symbol, "signature-change") {
//	        fmt.Printf("Breaking change at %s:%d\n", ref.Location.FileId, ref.Location.StartLine)
//	    }
//	}
//
package impact
