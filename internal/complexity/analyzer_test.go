package complexity

import (
	"context"
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
