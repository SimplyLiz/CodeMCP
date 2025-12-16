package impact_test

import (
	"fmt"

	"ckb/internal/impact"
)

// ExampleImpactAnalyzer demonstrates basic usage of the impact analyzer
func ExampleImpactAnalyzer() {
	// Create an analyzer with max depth of 2
	analyzer := impact.NewImpactAnalyzer(2)

	// Define a symbol to analyze
	symbol := &impact.Symbol{
		StableId:            "com.example.UserService.authenticate",
		Name:                "authenticate",
		Kind:                impact.KindMethod,
		Signature:           "public User authenticate(String username, String password)",
		SignatureNormalized: "authenticate(String,String):User",
		ModuleId:            "com.example.auth",
		ModuleName:          "auth-service",
		ContainerName:       "UserService",
		Location: &impact.Location{
			FileId:      "UserService.java",
			StartLine:   45,
			StartColumn: 5,
			EndLine:     52,
			EndColumn:   6,
		},
		Modifiers: []string{"public"},
	}

	// Define references to the symbol
	refs := []impact.Reference{
		{
			Location: &impact.Location{
				FileId:      "LoginController.java",
				StartLine:   28,
				StartColumn: 20,
				EndLine:     28,
				EndColumn:   32,
			},
			Kind:       impact.RefCall,
			FromSymbol: "com.example.web.LoginController.handleLogin",
			FromModule: "com.example.web",
			IsTest:     false,
		},
		{
			Location: &impact.Location{
				FileId:      "ApiController.java",
				StartLine:   15,
				StartColumn: 16,
				EndLine:     15,
				EndColumn:   28,
			},
			Kind:       impact.RefCall,
			FromSymbol: "com.example.api.ApiController.authenticate",
			FromModule: "com.example.api",
			IsTest:     false,
		},
		{
			Location: &impact.Location{
				FileId:      "UserServiceTest.java",
				StartLine:   42,
				StartColumn: 12,
				EndLine:     42,
				EndColumn:   24,
			},
			Kind:       impact.RefCall,
			FromSymbol: "com.example.auth.UserServiceTest.testAuthenticate",
			FromModule: "com.example.auth",
			IsTest:     true,
		},
	}

	// Perform the analysis
	result, err := analyzer.Analyze(symbol, refs)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Print visibility information
	fmt.Printf("Symbol: %s\n", result.Symbol.Name)
	fmt.Printf("Visibility: %s (%.0f%% confident from %s)\n",
		result.Visibility.Visibility,
		result.Visibility.Confidence*100,
		result.Visibility.Source)

	// Print risk assessment
	fmt.Printf("\nRisk Assessment:\n")
	fmt.Printf("  Level: %s\n", result.RiskScore.Level)
	fmt.Printf("  Score: %.2f\n", result.RiskScore.Score)
	fmt.Printf("  Explanation: %s\n", result.RiskScore.Explanation)

	// Print risk factors
	fmt.Printf("\nRisk Factors:\n")
	for _, factor := range result.RiskScore.Factors {
		fmt.Printf("  - %s: %.2f (weight: %.0f%%)\n",
			factor.Name, factor.Value, factor.Weight*100)
	}

	// Print impact summary
	fmt.Printf("\nImpact Summary:\n")
	fmt.Printf("  Direct impacts: %d\n", len(result.DirectImpact))
	fmt.Printf("  Transitive impacts: %d\n", len(result.TransitiveImpact))
	fmt.Printf("  Modules affected: %d\n", len(result.ModulesAffected))

	// Output:
	// Symbol: authenticate
	// Visibility: public (95% confident from scip-modifiers)
	//
	// Risk Assessment:
	//   Level: medium
	//   Score: 0.59
	//   Explanation: Medium risk: 2 direct caller(s) across 3 module(s). Changes require careful testing.
	//
	// Risk Factors:
	//   - visibility: 0.90 (weight: 30%)
	//   - direct-callers: 0.36 (weight: 35%)
	//   - module-spread: 0.48 (weight: 25%)
	//   - impact-kind: 0.70 (weight: 10%)
	//
	// Impact Summary:
	//   Direct impacts: 3
	//   Transitive impacts: 0
	//   Modules affected: 3
}

// ExampleDeriveVisibility demonstrates visibility derivation
func ExampleDeriveVisibility() {
	// Public symbol with SCIP modifiers
	publicSymbol := &impact.Symbol{
		Name:      "PublicFunction",
		ModuleId:  "module1",
		Modifiers: []string{"public"},
	}

	visInfo := impact.DeriveVisibility(publicSymbol, nil)
	fmt.Printf("Public symbol: %s (confidence: %.2f, source: %s)\n",
		visInfo.Visibility, visInfo.Confidence, visInfo.Source)

	// Private symbol with naming convention
	privateSymbol := &impact.Symbol{
		Name:      "_privateFunction",
		ModuleId:  "module1",
		Modifiers: []string{},
	}

	visInfo = impact.DeriveVisibility(privateSymbol, nil)
	fmt.Printf("Private symbol: %s (confidence: %.2f, source: %s)\n",
		visInfo.Visibility, visInfo.Confidence, visInfo.Source)

	// Symbol with external references
	symbolWithRefs := &impact.Symbol{
		Name:      "sharedFunction",
		ModuleId:  "module1",
		Modifiers: []string{},
	}

	refs := []impact.Reference{
		{FromModule: "module2"},
		{FromModule: "module3"},
	}

	visInfo = impact.DeriveVisibility(symbolWithRefs, refs)
	fmt.Printf("Symbol with external refs: %s (confidence: %.2f, source: %s)\n",
		visInfo.Visibility, visInfo.Confidence, visInfo.Source)

	// Output:
	// Public symbol: public (confidence: 0.95, source: scip-modifiers)
	// Private symbol: private (confidence: 0.60, source: naming-convention)
	// Symbol with external refs: public (confidence: 0.90, source: ref-analysis)
}

// ExampleClassifyImpact demonstrates impact classification
func ExampleClassifyImpact() {
	symbol := &impact.Symbol{
		Kind: impact.KindFunction,
	}

	// Direct call reference
	callRef := &impact.Reference{
		Kind:   impact.RefCall,
		IsTest: false,
	}
	kind, confidence := impact.ClassifyImpactWithConfidence(callRef, symbol)
	fmt.Printf("Call reference: %s (confidence: %.2f)\n", kind, confidence)

	// Test reference
	testRef := &impact.Reference{
		Kind:   impact.RefCall,
		IsTest: true,
	}
	kind, confidence = impact.ClassifyImpactWithConfidence(testRef, symbol)
	fmt.Printf("Test reference: %s (confidence: %.2f)\n", kind, confidence)

	// Type reference
	typeRef := &impact.Reference{
		Kind:   impact.RefType,
		IsTest: false,
	}
	kind, confidence = impact.ClassifyImpactWithConfidence(typeRef, symbol)
	fmt.Printf("Type reference: %s (confidence: %.2f)\n", kind, confidence)

	// Output:
	// Call reference: direct-caller (confidence: 0.95)
	// Test reference: test-dependency (confidence: 0.90)
	// Type reference: type-dependency (confidence: 0.80)
}

// ExampleIsBreakingChange demonstrates breaking change detection
func ExampleIsBreakingChange() {
	symbol := &impact.Symbol{
		ModuleId: "module1",
	}

	externalRef := &impact.Reference{
		Kind:       impact.RefCall,
		FromModule: "module2",
	}

	internalRef := &impact.Reference{
		Kind:       impact.RefCall,
		FromModule: "module1",
	}

	// Signature changes affect all callers
	fmt.Printf("Signature change (external): %v\n",
		impact.IsBreakingChange(externalRef, symbol, "signature-change"))

	// Visibility changes only affect external references
	fmt.Printf("Visibility change (external): %v\n",
		impact.IsBreakingChange(externalRef, symbol, "visibility-change"))
	fmt.Printf("Visibility change (internal): %v\n",
		impact.IsBreakingChange(internalRef, symbol, "visibility-change"))

	// Rename affects everyone
	fmt.Printf("Rename (external): %v\n",
		impact.IsBreakingChange(externalRef, symbol, "rename"))
	fmt.Printf("Rename (internal): %v\n",
		impact.IsBreakingChange(internalRef, symbol, "rename"))

	// Output:
	// Signature change (external): true
	// Visibility change (external): true
	// Visibility change (internal): false
	// Rename (external): true
	// Rename (internal): true
}

// ExampleImpactAnalyzer_AnalyzeWithOptions demonstrates custom analysis options
func ExampleImpactAnalyzer_AnalyzeWithOptions() {
	analyzer := impact.NewImpactAnalyzer(2)

	symbol := &impact.Symbol{
		StableId:  "test.function",
		Name:      "myFunction",
		Kind:      impact.KindFunction,
		ModuleId:  "module1",
		Modifiers: []string{"public"},
	}

	refs := []impact.Reference{
		{
			Location:   &impact.Location{FileId: "file1.go", StartLine: 10},
			Kind:       impact.RefCall,
			FromSymbol: "caller1",
			FromModule: "module2",
			IsTest:     false,
		},
		{
			Location:   &impact.Location{FileId: "test.go", StartLine: 20},
			Kind:       impact.RefCall,
			FromSymbol: "testFunc",
			FromModule: "module1",
			IsTest:     true,
		},
	}

	// Exclude test dependencies
	opts := impact.AnalyzeOptions{
		IncludeTests: false,
		MaxDepth:     3,
	}

	result, err := analyzer.AnalyzeWithOptions(symbol, refs, opts)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Impacts (excluding tests): %d\n", len(result.DirectImpact))
	fmt.Printf("Risk level: %s\n", result.RiskScore.Level)

	// Output:
	// Impacts (excluding tests): 1
	// Risk level: medium
}
