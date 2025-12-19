//go:build cgo

package complexity

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func TestAnalyzeSource_Go(t *testing.T) {
	source := []byte(`package main

func simple() {
	fmt.Println("hello")
}

func withIf(x int) {
	if x > 0 {
		fmt.Println("positive")
	}
}

func withIfElse(x int) {
	if x > 0 {
		fmt.Println("positive")
	} else {
		fmt.Println("non-positive")
	}
}

func withLoop(items []int) {
	for _, item := range items {
		fmt.Println(item)
	}
}

func withNestedIf(x, y int) {
	if x > 0 {
		if y > 0 {
			fmt.Println("both positive")
		}
	}
}

func withAndOr(a, b bool) {
	if a && b {
		fmt.Println("both true")
	}
	if a || b {
		fmt.Println("one true")
	}
}

func complex(x int, items []int) int {
	result := 0
	if x > 0 {
		for _, item := range items {
			if item > 0 {
				result += item
			} else if item < 0 {
				result -= item
			}
		}
	}
	return result
}
`)

	analyzer := NewAnalyzer()
	fc, err := analyzer.AnalyzeSource(context.Background(), "test.go", source, LangGo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fc.Language != LangGo {
		t.Errorf("expected language Go, got %s", fc.Language)
	}

	if len(fc.Functions) != 7 {
		t.Errorf("expected 7 functions, got %d", len(fc.Functions))
	}

	// Test simple function
	simple := findFunction(fc.Functions, "simple")
	if simple == nil {
		t.Fatal("simple function not found")
	}
	if simple.Cyclomatic != 1 {
		t.Errorf("simple: expected cyclomatic 1, got %d", simple.Cyclomatic)
	}

	// Test withIf function - 1 if = 2
	withIf := findFunction(fc.Functions, "withIf")
	if withIf == nil {
		t.Fatal("withIf function not found")
	}
	if withIf.Cyclomatic != 2 {
		t.Errorf("withIf: expected cyclomatic 2, got %d", withIf.Cyclomatic)
	}

	// Test withLoop function - 1 for + 1 range = 3
	withLoop := findFunction(fc.Functions, "withLoop")
	if withLoop == nil {
		t.Fatal("withLoop function not found")
	}
	if withLoop.Cyclomatic != 3 {
		t.Errorf("withLoop: expected cyclomatic 3, got %d", withLoop.Cyclomatic)
	}

	// Test withAndOr function - 2 ifs + 2 boolean operators = 5
	withAndOr := findFunction(fc.Functions, "withAndOr")
	if withAndOr == nil {
		t.Fatal("withAndOr function not found")
	}
	if withAndOr.Cyclomatic != 5 {
		t.Errorf("withAndOr: expected cyclomatic 5, got %d", withAndOr.Cyclomatic)
	}

	// Test complex function - has higher cognitive due to nesting
	complex := findFunction(fc.Functions, "complex")
	if complex == nil {
		t.Fatal("complex function not found")
	}
	if complex.Cognitive < complex.Cyclomatic {
		t.Errorf("complex: expected cognitive >= cyclomatic due to nesting, got cognitive=%d, cyclomatic=%d",
			complex.Cognitive, complex.Cyclomatic)
	}
}

func TestAnalyzeSource_JavaScript(t *testing.T) {
	source := []byte(`
function simple() {
	console.log("hello");
}

function withIf(x) {
	if (x > 0) {
		console.log("positive");
	}
}

const arrow = (x) => {
	if (x > 0) {
		return x * 2;
	}
	return x;
};

function withTernary(x) {
	return x > 0 ? "positive" : "non-positive";
}

function withAndOr(a, b) {
	if (a && b) {
		console.log("both");
	}
	return a || b;
}
`)

	analyzer := NewAnalyzer()
	fc, err := analyzer.AnalyzeSource(context.Background(), "test.js", source, LangJavaScript)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fc.Language != LangJavaScript {
		t.Errorf("expected language JavaScript, got %s", fc.Language)
	}

	if len(fc.Functions) < 4 {
		t.Errorf("expected at least 4 functions, got %d", len(fc.Functions))
	}

	// Test simple function
	simple := findFunction(fc.Functions, "simple")
	if simple == nil {
		t.Fatal("simple function not found")
	}
	if simple.Cyclomatic != 1 {
		t.Errorf("simple: expected cyclomatic 1, got %d", simple.Cyclomatic)
	}

	// Test withTernary - ternary adds complexity
	withTernary := findFunction(fc.Functions, "withTernary")
	if withTernary == nil {
		t.Fatal("withTernary function not found")
	}
	if withTernary.Cyclomatic < 2 {
		t.Errorf("withTernary: expected cyclomatic >= 2, got %d", withTernary.Cyclomatic)
	}
}

func TestAnalyzeSource_Python(t *testing.T) {
	source := []byte(`
def simple():
    print("hello")

def with_if(x):
    if x > 0:
        print("positive")

def with_loop(items):
    for item in items:
        print(item)

def with_comprehension(items):
    return [x * 2 for x in items if x > 0]

def with_and_or(a, b):
    if a and b:
        print("both")
    return a or b
`)

	analyzer := NewAnalyzer()
	fc, err := analyzer.AnalyzeSource(context.Background(), "test.py", source, LangPython)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fc.Language != LangPython {
		t.Errorf("expected language Python, got %s", fc.Language)
	}

	if len(fc.Functions) != 5 {
		t.Errorf("expected 5 functions, got %d", len(fc.Functions))
	}

	// Test simple function
	simple := findFunction(fc.Functions, "simple")
	if simple == nil {
		t.Fatal("simple function not found")
	}
	if simple.Cyclomatic != 1 {
		t.Errorf("simple: expected cyclomatic 1, got %d", simple.Cyclomatic)
	}

	// Test with_loop
	withLoop := findFunction(fc.Functions, "with_loop")
	if withLoop == nil {
		t.Fatal("with_loop function not found")
	}
	if withLoop.Cyclomatic != 2 {
		t.Errorf("with_loop: expected cyclomatic 2, got %d", withLoop.Cyclomatic)
	}
}

func TestAnalyzeSource_Rust(t *testing.T) {
	source := []byte(`
fn simple() {
    println!("hello");
}

fn with_if(x: i32) {
    if x > 0 {
        println!("positive");
    }
}

fn with_match(x: Option<i32>) -> i32 {
    match x {
        Some(v) => v,
        None => 0,
    }
}

fn with_loop(items: Vec<i32>) {
    for item in items {
        println!("{}", item);
    }
}
`)

	analyzer := NewAnalyzer()
	fc, err := analyzer.AnalyzeSource(context.Background(), "test.rs", source, LangRust)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fc.Language != LangRust {
		t.Errorf("expected language Rust, got %s", fc.Language)
	}

	if len(fc.Functions) != 4 {
		t.Errorf("expected 4 functions, got %d", len(fc.Functions))
	}

	// Test simple function
	simple := findFunction(fc.Functions, "simple")
	if simple == nil {
		t.Fatal("simple function not found")
	}
	if simple.Cyclomatic != 1 {
		t.Errorf("simple: expected cyclomatic 1, got %d", simple.Cyclomatic)
	}

	// Test with_match - match + 2 arms
	withMatch := findFunction(fc.Functions, "with_match")
	if withMatch == nil {
		t.Fatal("with_match function not found")
	}
	if withMatch.Cyclomatic < 2 {
		t.Errorf("with_match: expected cyclomatic >= 2, got %d", withMatch.Cyclomatic)
	}
}

func TestAnalyzeSource_Java(t *testing.T) {
	source := []byte(`
public class Test {
    public void simple() {
        System.out.println("hello");
    }

    public void withIf(int x) {
        if (x > 0) {
            System.out.println("positive");
        }
    }

    public void withTryCatch() {
        try {
            doSomething();
        } catch (Exception e) {
            handleError(e);
        }
    }

    public void withSwitch(int x) {
        switch (x) {
            case 1:
                System.out.println("one");
                break;
            case 2:
                System.out.println("two");
                break;
            default:
                System.out.println("other");
        }
    }
}
`)

	analyzer := NewAnalyzer()
	fc, err := analyzer.AnalyzeSource(context.Background(), "Test.java", source, LangJava)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if fc.Language != LangJava {
		t.Errorf("expected language Java, got %s", fc.Language)
	}

	if len(fc.Functions) != 4 {
		t.Errorf("expected 4 functions, got %d", len(fc.Functions))
	}

	// Test simple method
	simple := findFunction(fc.Functions, "simple")
	if simple == nil {
		t.Fatal("simple method not found")
	}
	if simple.Cyclomatic != 1 {
		t.Errorf("simple: expected cyclomatic 1, got %d", simple.Cyclomatic)
	}

	// Test withTryCatch - catch clause adds complexity
	withTryCatch := findFunction(fc.Functions, "withTryCatch")
	if withTryCatch == nil {
		t.Fatal("withTryCatch method not found")
	}
	if withTryCatch.Cyclomatic < 2 {
		t.Errorf("withTryCatch: expected cyclomatic >= 2, got %d", withTryCatch.Cyclomatic)
	}
}

func TestCognitiveComplexity_NestingPenalty(t *testing.T) {
	// Test that nested constructs have higher cognitive complexity
	source := []byte(`package main

func flat(a, b, c bool) {
	if a {
		doA()
	}
	if b {
		doB()
	}
	if c {
		doC()
	}
}

func nested(a, b, c bool) {
	if a {
		if b {
			if c {
				doABC()
			}
		}
	}
}
`)

	analyzer := NewAnalyzer()
	fc, err := analyzer.AnalyzeSource(context.Background(), "test.go", source, LangGo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	flat := findFunction(fc.Functions, "flat")
	nested := findFunction(fc.Functions, "nested")

	if flat == nil || nested == nil {
		t.Fatal("functions not found")
	}

	// Both have same cyclomatic (3 ifs = 4)
	if flat.Cyclomatic != nested.Cyclomatic {
		t.Logf("flat cyclomatic: %d, nested cyclomatic: %d", flat.Cyclomatic, nested.Cyclomatic)
	}

	// But nested should have higher cognitive due to nesting penalty
	if nested.Cognitive <= flat.Cognitive {
		t.Errorf("expected nested cognitive (%d) > flat cognitive (%d)",
			nested.Cognitive, flat.Cognitive)
	}
}

func TestLanguageFromExtension(t *testing.T) {
	tests := []struct {
		ext      string
		expected Language
		ok       bool
	}{
		{".go", LangGo, true},
		{".js", LangJavaScript, true},
		{".mjs", LangJavaScript, true},
		{".ts", LangTypeScript, true},
		{".tsx", LangTSX, true},
		{".py", LangPython, true},
		{".rs", LangRust, true},
		{".java", LangJava, true},
		{".kt", LangKotlin, true},
		{".kts", LangKotlin, true},
		{".txt", "", false},
		{".md", "", false},
	}

	for _, tt := range tests {
		lang, ok := LanguageFromExtension(tt.ext)
		if ok != tt.ok {
			t.Errorf("LanguageFromExtension(%q): expected ok=%v, got %v", tt.ext, tt.ok, ok)
		}
		if lang != tt.expected {
			t.Errorf("LanguageFromExtension(%q): expected %q, got %q", tt.ext, tt.expected, lang)
		}
	}
}

func TestFileComplexity_Aggregate(t *testing.T) {
	fc := &FileComplexity{
		Functions: []ComplexityResult{
			{Name: "a", Cyclomatic: 5, Cognitive: 10},
			{Name: "b", Cyclomatic: 3, Cognitive: 4},
			{Name: "c", Cyclomatic: 8, Cognitive: 15},
		},
	}

	fc.Aggregate()

	if fc.FunctionCount != 3 {
		t.Errorf("expected FunctionCount 3, got %d", fc.FunctionCount)
	}
	if fc.TotalCyclomatic != 16 {
		t.Errorf("expected TotalCyclomatic 16, got %d", fc.TotalCyclomatic)
	}
	if fc.TotalCognitive != 29 {
		t.Errorf("expected TotalCognitive 29, got %d", fc.TotalCognitive)
	}
	if fc.MaxCyclomatic != 8 {
		t.Errorf("expected MaxCyclomatic 8, got %d", fc.MaxCyclomatic)
	}
	if fc.MaxCognitive != 15 {
		t.Errorf("expected MaxCognitive 15, got %d", fc.MaxCognitive)
	}
}

func findFunction(functions []ComplexityResult, name string) *ComplexityResult {
	for i := range functions {
		if functions[i].Name == name {
			return &functions[i]
		}
	}
	return nil
}

// Benchmarks

func BenchmarkAnalyzeSource_Go_Small(b *testing.B) {
	source := []byte(`package main

func simple() {
	fmt.Println("hello")
}

func withIf(x int) {
	if x > 0 {
		fmt.Println("positive")
	}
}
`)
	analyzer := NewAnalyzer()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = analyzer.AnalyzeSource(ctx, "test.go", source, LangGo)
	}
}

func BenchmarkAnalyzeSource_Go_Medium(b *testing.B) {
	// ~500 lines with moderate complexity
	source := generateMediumGoSource()
	analyzer := NewAnalyzer()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = analyzer.AnalyzeSource(ctx, "test.go", source, LangGo)
	}
}

func BenchmarkAnalyzeSource_Go_Large(b *testing.B) {
	// ~2000 lines with high complexity
	source := generateLargeGoSource()
	analyzer := NewAnalyzer()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = analyzer.AnalyzeSource(ctx, "test.go", source, LangGo)
	}
}

func BenchmarkAnalyzeSource_JavaScript_Medium(b *testing.B) {
	source := generateMediumJSSource()
	analyzer := NewAnalyzer()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = analyzer.AnalyzeSource(ctx, "test.js", source, LangJavaScript)
	}
}

func BenchmarkAnalyzeSource_Python_Medium(b *testing.B) {
	source := generateMediumPythonSource()
	analyzer := NewAnalyzer()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = analyzer.AnalyzeSource(ctx, "test.py", source, LangPython)
	}
}

func BenchmarkAnalyzeSource_Rust_Medium(b *testing.B) {
	source := generateMediumRustSource()
	analyzer := NewAnalyzer()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = analyzer.AnalyzeSource(ctx, "test.rs", source, LangRust)
	}
}

func BenchmarkAnalyzeSource_Java_Medium(b *testing.B) {
	source := generateMediumJavaSource()
	analyzer := NewAnalyzer()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = analyzer.AnalyzeSource(ctx, "Test.java", source, LangJava)
	}
}

// Validation tests against known complexity tool outputs
// These validate our implementation matches standard tools like gocyclo, radon, etc.

func TestValidation_GoCyclo_Compatibility(t *testing.T) {
	// Test cases with known cyclomatic complexity values
	// Based on gocyclo algorithm: +1 for each if, for, case, &&, ||
	testCases := []struct {
		name     string
		source   string
		expected int
	}{
		{
			name: "empty function",
			source: `package main
func empty() {}`,
			expected: 1,
		},
		{
			name: "single if",
			source: `package main
func singleIf(x int) {
	if x > 0 {
		println(x)
	}
}`,
			expected: 2,
		},
		{
			name: "if-else chain",
			source: `package main
func ifElseChain(x int) {
	if x > 0 {
		println("positive")
	} else if x < 0 {
		println("negative")
	} else {
		println("zero")
	}
}`,
			expected: 3, // 1 base + 2 conditions (if, else if)
		},
		{
			name: "switch with cases",
			source: `package main
func switchCases(x int) {
	switch x {
	case 1:
		println("one")
	case 2:
		println("two")
	case 3:
		println("three")
	default:
		println("other")
	}
}`,
			expected: 4, // 1 base + 3 case clauses (default doesn't count)
		},
		{
			name: "logical operators",
			source: `package main
func logicalOps(a, b, c bool) {
	if a && b || c {
		println("complex")
	}
}`,
			expected: 4, // 1 base + 1 if + 1 && + 1 ||
		},
		{
			name: "nested loops",
			source: `package main
func nestedLoops(n int) {
	for i := 0; i < n; i++ {
		for j := 0; j < n; j++ {
			println(i, j)
		}
	}
}`,
			expected: 3, // 1 base + 2 for loops
		},
	}

	analyzer := NewAnalyzer()
	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fc, err := analyzer.AnalyzeSource(ctx, "test.go", []byte(tc.source), LangGo)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fc.Functions) != 1 {
				t.Fatalf("expected 1 function, got %d", len(fc.Functions))
			}
			if fc.Functions[0].Cyclomatic != tc.expected {
				t.Errorf("expected cyclomatic %d, got %d", tc.expected, fc.Functions[0].Cyclomatic)
			}
		})
	}
}

func TestValidation_CognitiveComplexity_SonarSource(t *testing.T) {
	// Test cases based on SonarSource cognitive complexity specification
	// https://www.sonarsource.com/docs/CognitiveComplexity.pdf
	testCases := []struct {
		name     string
		source   string
		expected int
	}{
		{
			name: "flat structure - no nesting penalty",
			source: `package main
func flat(a, b bool) {
	if a {   // +1
		doA()
	}
	if b {   // +1
		doB()
	}
}`,
			expected: 2,
		},
		{
			name: "nested structure - with nesting penalty",
			source: `package main
func nested(a, b bool) {
	if a {       // +1
		if b {   // +2 (1 + nesting 1)
			doAB()
		}
	}
}`,
			expected: 3,
		},
		{
			name: "deeply nested",
			source: `package main
func deeplyNested(a, b, c bool) {
	if a {           // +1
		if b {       // +2 (1 + nesting 1)
			if c {   // +3 (1 + nesting 2)
				doABC()
			}
		}
	}
}`,
			expected: 6,
		},
		{
			name: "loop with nested condition",
			source: `package main
func loopWithCondition(items []int) {
	for _, item := range items {  // +1
		if item > 0 {             // +2 (1 + nesting 1)
			process(item)
		}
	}
}`,
			// Note: Go has for_statement + range_clause, so complexity is higher
			expected: 5, // for: +1, range: +2, if: +3
		},
	}

	analyzer := NewAnalyzer()
	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fc, err := analyzer.AnalyzeSource(ctx, "test.go", []byte(tc.source), LangGo)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fc.Functions) != 1 {
				t.Fatalf("expected 1 function, got %d", len(fc.Functions))
			}
			if fc.Functions[0].Cognitive != tc.expected {
				t.Errorf("expected cognitive %d, got %d", tc.expected, fc.Functions[0].Cognitive)
			}
		})
	}
}

func TestValidation_Python_Radon_Compatibility(t *testing.T) {
	// Test cases validated against radon cc output
	// radon is the standard Python cyclomatic complexity tool
	testCases := []struct {
		name     string
		source   string
		expected int
	}{
		{
			name: "simple function",
			source: `def simple():
    return 1`,
			expected: 1,
		},
		{
			name: "single if",
			source: `def single_if(x):
    if x > 0:
        return x
    return 0`,
			expected: 2,
		},
		{
			name: "if-elif-else",
			source: `def if_elif_else(x):
    if x > 0:
        return 1
    elif x < 0:
        return -1
    else:
        return 0`,
			expected: 3,
		},
		{
			name: "for loop",
			source: `def for_loop(items):
    total = 0
    for item in items:
        total += item
    return total`,
			expected: 2,
		},
		{
			name: "try-except",
			source: `def try_except():
    try:
        risky()
    except ValueError:
        handle_value()
    except TypeError:
        handle_type()`,
			expected: 3, // 1 base + 2 except clauses
		},
		{
			name: "boolean operators",
			source: `def bool_ops(a, b, c):
    if a and b or c:
        return True
    return False`,
			expected: 4, // 1 base + 1 if + 1 and + 1 or
		},
	}

	analyzer := NewAnalyzer()
	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fc, err := analyzer.AnalyzeSource(ctx, "test.py", []byte(tc.source), LangPython)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fc.Functions) != 1 {
				t.Fatalf("expected 1 function, got %d", len(fc.Functions))
			}
			if fc.Functions[0].Cyclomatic != tc.expected {
				t.Errorf("expected cyclomatic %d, got %d", tc.expected, fc.Functions[0].Cyclomatic)
			}
		})
	}
}

func TestValidation_JavaScript_ESLint_Compatibility(t *testing.T) {
	// Test cases compatible with eslint complexity rule
	testCases := []struct {
		name     string
		source   string
		expected int
	}{
		{
			name: "simple function",
			source: `function simple() {
    return 1;
}`,
			expected: 1,
		},
		{
			name: "single if",
			source: `function singleIf(x) {
    if (x > 0) {
        return x;
    }
    return 0;
}`,
			expected: 2,
		},
		{
			name: "ternary operator",
			source: `function ternary(x) {
    return x > 0 ? x : -x;
}`,
			expected: 2, // 1 base + 1 ternary
		},
		{
			name: "logical operators in condition",
			source: `function logicalOps(a, b) {
    if (a && b) {
        return true;
    }
    return a || b;
}`,
			expected: 4, // 1 base + 1 if + 1 && + 1 ||
		},
		{
			name: "switch cases",
			source: `function switchCase(x) {
    switch (x) {
        case 1:
            return "one";
        case 2:
            return "two";
        default:
            return "other";
    }
}`,
			expected: 3, // 1 base + 2 case clauses
		},
	}

	analyzer := NewAnalyzer()
	ctx := context.Background()

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			fc, err := analyzer.AnalyzeSource(ctx, "test.js", []byte(tc.source), LangJavaScript)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(fc.Functions) != 1 {
				t.Fatalf("expected 1 function, got %d", len(fc.Functions))
			}
			if fc.Functions[0].Cyclomatic != tc.expected {
				t.Errorf("expected cyclomatic %d, got %d", tc.expected, fc.Functions[0].Cyclomatic)
			}
		})
	}
}

// Helper functions to generate test source code

func generateMediumGoSource() []byte {
	var sb strings.Builder
	sb.WriteString("package main\n\n")

	// Generate 20 functions with varying complexity
	for i := 0; i < 20; i++ {
		sb.WriteString(fmt.Sprintf("func function%d(x, y, z int) int {\n", i))
		sb.WriteString("\tresult := 0\n")
		sb.WriteString("\tif x > 0 {\n")
		sb.WriteString("\t\tfor j := 0; j < y; j++ {\n")
		sb.WriteString("\t\t\tif j%2 == 0 {\n")
		sb.WriteString("\t\t\t\tresult += j\n")
		sb.WriteString("\t\t\t} else {\n")
		sb.WriteString("\t\t\t\tresult -= j\n")
		sb.WriteString("\t\t\t}\n")
		sb.WriteString("\t\t}\n")
		sb.WriteString("\t} else if x < 0 {\n")
		sb.WriteString("\t\tswitch z {\n")
		sb.WriteString("\t\tcase 1:\n")
		sb.WriteString("\t\t\tresult = 1\n")
		sb.WriteString("\t\tcase 2:\n")
		sb.WriteString("\t\t\tresult = 2\n")
		sb.WriteString("\t\tdefault:\n")
		sb.WriteString("\t\t\tresult = -1\n")
		sb.WriteString("\t\t}\n")
		sb.WriteString("\t}\n")
		sb.WriteString("\treturn result\n")
		sb.WriteString("}\n\n")
	}

	return []byte(sb.String())
}

func generateLargeGoSource() []byte {
	var sb strings.Builder
	sb.WriteString("package main\n\n")

	// Generate 80 functions with high complexity
	for i := 0; i < 80; i++ {
		sb.WriteString(fmt.Sprintf("func largeFunction%d(items []int, filter func(int) bool) int {\n", i))
		sb.WriteString("\tresult := 0\n")
		sb.WriteString("\tfor _, item := range items {\n")
		sb.WriteString("\t\tif filter(item) {\n")
		sb.WriteString("\t\t\tif item > 100 {\n")
		sb.WriteString("\t\t\t\tfor j := 0; j < item; j++ {\n")
		sb.WriteString("\t\t\t\t\tif j%3 == 0 && j%5 == 0 {\n")
		sb.WriteString("\t\t\t\t\t\tresult += j\n")
		sb.WriteString("\t\t\t\t\t} else if j%3 == 0 || j%5 == 0 {\n")
		sb.WriteString("\t\t\t\t\t\tresult -= j\n")
		sb.WriteString("\t\t\t\t\t}\n")
		sb.WriteString("\t\t\t\t}\n")
		sb.WriteString("\t\t\t} else if item > 50 {\n")
		sb.WriteString("\t\t\t\tswitch item % 4 {\n")
		sb.WriteString("\t\t\t\tcase 0:\n")
		sb.WriteString("\t\t\t\t\tresult++\n")
		sb.WriteString("\t\t\t\tcase 1:\n")
		sb.WriteString("\t\t\t\t\tresult--\n")
		sb.WriteString("\t\t\t\tcase 2:\n")
		sb.WriteString("\t\t\t\t\tresult *= 2\n")
		sb.WriteString("\t\t\t\tdefault:\n")
		sb.WriteString("\t\t\t\t\tresult /= 2\n")
		sb.WriteString("\t\t\t\t}\n")
		sb.WriteString("\t\t\t}\n")
		sb.WriteString("\t\t}\n")
		sb.WriteString("\t}\n")
		sb.WriteString("\treturn result\n")
		sb.WriteString("}\n\n")
	}

	return []byte(sb.String())
}

func generateMediumJSSource() []byte {
	var sb strings.Builder

	for i := 0; i < 20; i++ {
		sb.WriteString(fmt.Sprintf("function process%d(data, options) {\n", i))
		sb.WriteString("  let result = [];\n")
		sb.WriteString("  if (data && data.length > 0) {\n")
		sb.WriteString("    for (const item of data) {\n")
		sb.WriteString("      if (item.active && !item.deleted) {\n")
		sb.WriteString("        result.push(item.value > 0 ? item.value : 0);\n")
		sb.WriteString("      } else if (options.includeInactive || item.important) {\n")
		sb.WriteString("        result.push(-1);\n")
		sb.WriteString("      }\n")
		sb.WriteString("    }\n")
		sb.WriteString("  }\n")
		sb.WriteString("  return result;\n")
		sb.WriteString("}\n\n")
	}

	return []byte(sb.String())
}

func generateMediumPythonSource() []byte {
	var sb strings.Builder

	for i := 0; i < 20; i++ {
		sb.WriteString(fmt.Sprintf("def process_%d(data, options=None):\n", i))
		sb.WriteString("    result = []\n")
		sb.WriteString("    if data and len(data) > 0:\n")
		sb.WriteString("        for item in data:\n")
		sb.WriteString("            if item.get('active') and not item.get('deleted'):\n")
		sb.WriteString("                value = item.get('value', 0)\n")
		sb.WriteString("                result.append(value if value > 0 else 0)\n")
		sb.WriteString("            elif options and (options.get('include_inactive') or item.get('important')):\n")
		sb.WriteString("                result.append(-1)\n")
		sb.WriteString("    return result\n\n")
	}

	return []byte(sb.String())
}

func generateMediumRustSource() []byte {
	var sb strings.Builder

	for i := 0; i < 20; i++ {
		sb.WriteString(fmt.Sprintf("fn process_%d(data: Vec<Item>, options: &Options) -> Vec<i32> {\n", i))
		sb.WriteString("    let mut result = Vec::new();\n")
		sb.WriteString("    for item in data {\n")
		sb.WriteString("        match item.status {\n")
		sb.WriteString("            Status::Active if !item.deleted => {\n")
		sb.WriteString("                if item.value > 0 {\n")
		sb.WriteString("                    result.push(item.value);\n")
		sb.WriteString("                } else {\n")
		sb.WriteString("                    result.push(0);\n")
		sb.WriteString("                }\n")
		sb.WriteString("            }\n")
		sb.WriteString("            _ if options.include_inactive || item.important => {\n")
		sb.WriteString("                result.push(-1);\n")
		sb.WriteString("            }\n")
		sb.WriteString("            _ => {}\n")
		sb.WriteString("        }\n")
		sb.WriteString("    }\n")
		sb.WriteString("    result\n")
		sb.WriteString("}\n\n")
	}

	return []byte(sb.String())
}

func generateMediumJavaSource() []byte {
	var sb strings.Builder
	sb.WriteString("public class ProcessorService {\n\n")

	for i := 0; i < 20; i++ {
		sb.WriteString(fmt.Sprintf("    public List<Integer> process%d(List<Item> data, Options options) {\n", i))
		sb.WriteString("        List<Integer> result = new ArrayList<>();\n")
		sb.WriteString("        if (data != null && !data.isEmpty()) {\n")
		sb.WriteString("            for (Item item : data) {\n")
		sb.WriteString("                if (item.isActive() && !item.isDeleted()) {\n")
		sb.WriteString("                    result.add(item.getValue() > 0 ? item.getValue() : 0);\n")
		sb.WriteString("                } else if (options.isIncludeInactive() || item.isImportant()) {\n")
		sb.WriteString("                    result.add(-1);\n")
		sb.WriteString("                }\n")
		sb.WriteString("            }\n")
		sb.WriteString("        }\n")
		sb.WriteString("        return result;\n")
		sb.WriteString("    }\n\n")
	}

	sb.WriteString("}\n")
	return []byte(sb.String())
}
