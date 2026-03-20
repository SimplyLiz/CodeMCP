//go:build cgo

package query

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/SimplyLiz/CodeMCP/internal/complexity"
)

func TestBugPattern_DeferInLoop(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

import "os"

func process() {
	for i := 0; i < 10; i++ {
		f, _ := os.Open("file")
		defer f.Close()
	}
}
`)
	root := mustParse(t, source)
	findings := checkDeferInLoop(root, source, "test.go")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RuleID != "ckb/bug/defer-in-loop" {
		t.Errorf("expected rule ckb/bug/defer-in-loop, got %s", findings[0].RuleID)
	}
}

func TestBugPattern_UnreachableCode(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

func foo() int {
	return 42
	x := 1
	_ = x
}
`)
	root := mustParse(t, source)
	findings := checkUnreachableCode(root, source, "test.go")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RuleID != "ckb/bug/unreachable-code" {
		t.Errorf("expected rule ckb/bug/unreachable-code, got %s", findings[0].RuleID)
	}
}

func TestBugPattern_EmptyErrorBranch(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

func foo() {
	err := doSomething()
	if err != nil {
	}
}

func doSomething() error { return nil }
`)
	root := mustParse(t, source)
	findings := checkEmptyErrorBranch(root, source, "test.go")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RuleID != "ckb/bug/empty-error-branch" {
		t.Errorf("expected rule ckb/bug/empty-error-branch, got %s", findings[0].RuleID)
	}
}

func TestBugPattern_UncheckedTypeAssert(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

func foo(x interface{}) {
	s := x.(string)
	_ = s
}
`)
	root := mustParse(t, source)
	findings := checkUncheckedTypeAssert(root, source, "test.go")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RuleID != "ckb/bug/unchecked-type-assert" {
		t.Errorf("expected rule ckb/bug/unchecked-type-assert, got %s", findings[0].RuleID)
	}
}

func TestBugPattern_UncheckedTypeAssert_TwoValue(t *testing.T) {
	t.Parallel()
	// Two-value form should NOT trigger
	source := []byte(`package main

func foo(x interface{}) {
	s, ok := x.(string)
	_, _ = s, ok
}
`)
	root := mustParse(t, source)
	findings := checkUncheckedTypeAssert(root, source, "test.go")
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings for two-value type assert, got %d", len(findings))
	}
}

func TestBugPattern_SelfAssignment(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

func foo() {
	x := 1
	x = x
}
`)
	root := mustParse(t, source)
	findings := checkSelfAssignment(root, source, "test.go")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RuleID != "ckb/bug/self-assignment" {
		t.Errorf("expected rule ckb/bug/self-assignment, got %s", findings[0].RuleID)
	}
}

func TestBugPattern_IdenticalBranches(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

func foo(x bool) int {
	if x {
		return 1
	} else {
		return 1
	}
}
`)
	root := mustParse(t, source)
	findings := checkIdenticalBranches(root, source, "test.go")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RuleID != "ckb/bug/identical-branches" {
		t.Errorf("expected rule ckb/bug/identical-branches, got %s", findings[0].RuleID)
	}
}

func TestBugPattern_ShadowedErr(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

import "fmt"

func foo() error {
	_, err := fmt.Println("outer")
	if true {
		_, err := fmt.Println("inner")
		_ = err
	}
	return err
}
`)
	root := mustParse(t, source)
	findings := checkShadowedErr(root, source, "test.go")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RuleID != "ckb/bug/shadowed-err" {
		t.Errorf("expected rule ckb/bug/shadowed-err, got %s", findings[0].RuleID)
	}
}

func TestBugPattern_NoFalsePositive(t *testing.T) {
	t.Parallel()
	// Clean code should produce zero findings
	source := []byte(`package main

import "fmt"

func clean() error {
	val, err := fmt.Println("hello")
	if err != nil {
		return err
	}
	_ = val
	return nil
}
`)
	root := mustParse(t, source)
	var allFindings []ReviewFinding
	allFindings = append(allFindings, checkDeferInLoop(root, source, "test.go")...)
	allFindings = append(allFindings, checkUnreachableCode(root, source, "test.go")...)
	allFindings = append(allFindings, checkEmptyErrorBranch(root, source, "test.go")...)
	allFindings = append(allFindings, checkUncheckedTypeAssert(root, source, "test.go")...)
	allFindings = append(allFindings, checkSelfAssignment(root, source, "test.go")...)
	allFindings = append(allFindings, checkIdenticalBranches(root, source, "test.go")...)
	allFindings = append(allFindings, checkShadowedErr(root, source, "test.go")...)
	if len(allFindings) != 0 {
		t.Errorf("expected 0 findings for clean code, got %d:", len(allFindings))
		for _, f := range allFindings {
			t.Logf("  %s:%d %s", f.File, f.StartLine, f.Message)
		}
	}
}

func TestBugPattern_DiscardedError(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

import "os"

func foo() {
	os.Open("file.txt")
}
`)
	root := mustParse(t, source)
	findings := checkDiscardedError(root, source, "test.go")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RuleID != "ckb/bug/discarded-error" {
		t.Errorf("expected rule ckb/bug/discarded-error, got %s", findings[0].RuleID)
	}
}

func TestBugPattern_MissingClose(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

import "os"

func foo() {
	f, _ := os.Open("file.txt")
	_ = f
}
`)
	root := mustParse(t, source)
	findings := checkMissingDeferClose(root, source, "test.go")
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].RuleID != "ckb/bug/missing-defer-close" {
		t.Errorf("expected rule ckb/bug/missing-defer-close, got %s", findings[0].RuleID)
	}
}

func TestBugPattern_MissingClose_WithDefer(t *testing.T) {
	t.Parallel()
	// Should NOT trigger when defer Close() is present
	source := []byte(`package main

import "os"

func foo() {
	f, _ := os.Open("file.txt")
	defer f.Close()
	_ = f
}
`)
	root := mustParse(t, source)
	findings := checkMissingDeferClose(root, source, "test.go")
	if len(findings) != 0 {
		t.Fatalf("expected 0 findings with defer Close, got %d", len(findings))
	}
}

func TestBugPatterns_DiffMode_PreexistingNotReported(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()
	repoRoot := engine.repoRoot

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	git("init", "-b", "main")

	// Base: file with existing defer-in-loop bug
	baseContent := `package main

import "os"

func process() {
	for i := 0; i < 10; i++ {
		f, _ := os.Open("file")
		defer f.Close()
	}
}
`
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte(baseContent), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "initial")

	// Feature: add a NEW defer-in-loop bug in a different function
	git("checkout", "-b", "feature/bugs")
	featureContent := baseContent + `
func processMore() {
	for i := 0; i < 5; i++ {
		g, _ := os.Open("other")
		defer g.Close()
	}
}
`
	if err := os.WriteFile(filepath.Join(repoRoot, "main.go"), []byte(featureContent), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "add more processing")

	reinitEngine(t, engine)

	ctx := context.Background()
	_, findings := engine.checkBugPatternsWithDiff(ctx, []string{"main.go"}, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/bugs",
	})

	// Should only report the NEW defer-in-loop in processMore, not the pre-existing one in process
	if len(findings) != 1 {
		t.Errorf("expected 1 new finding (pre-existing filtered), got %d:", len(findings))
		for _, f := range findings {
			t.Logf("  %s:%d %s", f.File, f.StartLine, f.Message)
		}
	}
}

func TestBugPatterns_DiffMode_NewFile(t *testing.T) {
	t.Parallel()

	engine, cleanup := testEngine(t)
	defer cleanup()
	repoRoot := engine.repoRoot

	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoRoot
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}

	git("init", "-b", "main")
	if err := os.WriteFile(filepath.Join(repoRoot, "README.md"), []byte("# Test\n"), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "initial")

	// Feature: entirely new file with a bug
	git("checkout", "-b", "feature/newfile")
	newContent := `package main

func foo() int {
	return 42
	x := 1
	_ = x
}
`
	if err := os.WriteFile(filepath.Join(repoRoot, "new.go"), []byte(newContent), 0644); err != nil {
		t.Fatal(err)
	}
	git("add", ".")
	git("commit", "-m", "add new file")

	reinitEngine(t, engine)

	ctx := context.Background()
	_, findings := engine.checkBugPatternsWithDiff(ctx, []string{"new.go"}, ReviewPROptions{
		BaseBranch: "main",
		HeadBranch: "feature/newfile",
	})

	// New file — all findings should be reported
	if len(findings) == 0 {
		t.Error("expected findings for new file, got 0")
	}
}

func TestBugPattern_DiscardedError_BuilderNotFlagged(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

import (
	"strings"
)

func foo() string {
	var b strings.Builder
	b.WriteString("hello")
	b.Write([]byte(" world"))
	b.WriteByte('!')
	b.WriteRune('?')
	return b.String()
}
`)
	root := mustParse(t, source)
	findings := checkDiscardedError(root, source, "test.go")
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for strings.Builder, got %d:", len(findings))
		for _, f := range findings {
			t.Logf("  line %d: %s", f.StartLine, f.Message)
		}
	}
}

func TestBugPattern_DiscardedError_BytesBufferNotFlagged(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

import (
	"bytes"
)

func foo() string {
	b := &bytes.Buffer{}
	b.WriteString("hello")
	b.Write([]byte(" world"))
	return b.String()
}
`)
	root := mustParse(t, source)
	findings := checkDiscardedError(root, source, "test.go")
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for bytes.Buffer, got %d:", len(findings))
		for _, f := range findings {
			t.Logf("  line %d: %s", f.StartLine, f.Message)
		}
	}
}

func TestBugPattern_DiscardedError_RealErrorStillFlagged(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

import "os"

func foo() {
	os.Open("file.txt")
	os.Create("out.txt")
}
`)
	root := mustParse(t, source)
	findings := checkDiscardedError(root, source, "test.go")
	if len(findings) != 2 {
		t.Errorf("expected 2 findings for real discarded errors, got %d:", len(findings))
		for _, f := range findings {
			t.Logf("  line %d: %s", f.StartLine, f.Message)
		}
	}
}

func TestBugPattern_DiscardedError_NewBufferNotFlagged(t *testing.T) {
	t.Parallel()
	source := []byte(`package main

import "bytes"

func foo() string {
	b := bytes.NewBufferString("hello")
	b.WriteString(" world")
	return b.String()
}
`)
	root := mustParse(t, source)
	findings := checkDiscardedError(root, source, "test.go")
	if len(findings) != 0 {
		t.Errorf("expected 0 findings for bytes.NewBufferString receiver, got %d:", len(findings))
		for _, f := range findings {
			t.Logf("  line %d: %s", f.StartLine, f.Message)
		}
	}
}

// mustParse is a test helper that parses Go source with tree-sitter.
func mustParse(t *testing.T, source []byte) *sitter.Node {
	t.Helper()
	parser := complexity.NewParser()
	if parser == nil {
		t.Skip("tree-sitter parser not available")
	}
	root, err := parser.Parse(context.Background(), source, complexity.LangGo)
	if err != nil {
		t.Fatalf("failed to parse source: %v", err)
	}
	return root
}

// --- Corpus Tests: realistic known-buggy patterns ---

// TestBugPatternCorpus_KnownBugs exercises all 10 rules against a realistic
// Go file containing one instance of each bug pattern.
func TestBugPatternCorpus_KnownBugs(t *testing.T) {
	t.Parallel()

	source := []byte(`package buggy

import (
	"fmt"
	"io"
	"os"
	"strconv"
)

// Bug 1: defer in loop — resource leak
func processFiles(paths []string) error {
	for _, p := range paths {
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close() // BUG: defer-in-loop
		_ = f
	}
	return nil
}

// Bug 2: unreachable code after return
func validate(x int) string {
	if x < 0 {
		return "negative"
	}
	return "ok"
	fmt.Println("done") // BUG: unreachable
}

// Bug 3: empty error branch — swallowed error
func loadConfig(path string) []byte {
	data, err := os.ReadFile(path)
	if err != nil {
	}
	return data
}

// Bug 4: unchecked type assertion — panic risk
func toString(v interface{}) string {
	return v.(string) // BUG: no comma-ok
}

// Bug 5: self-assignment — probably a typo
func transform(s string) string {
	result := s
	result = result // BUG: self-assignment
	return result
}

// Bug 6: nil check after dereference
func processReader(r io.Reader) {
	data := make([]byte, 100)
	r.Read(data)    // dereference
	if r != nil {   // BUG: nil check AFTER use
		_ = data
	}
}

// Bug 7: identical if/else branches
func classify(n int) string {
	if n > 0 {
		return "positive"
	} else {
		return "positive"
	}
}

// Bug 8: shadowed err
func multiStep() error {
	_, err := fmt.Println("step 1")
	if true {
		_, err := fmt.Println("step 2") // BUG: shadows outer err
		_ = err
	}
	return err
}

// Bug 9: discarded error from function that returns error
func unsafeIO() {
	os.Open("important.dat") // BUG: discarded error
}

// Bug 10: missing defer Close
func leakyReader(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	// missing close — resource leak
	buf := make([]byte, 1024)
	n, err := f.Read(buf)
	return buf[:n], err
}

// Not a bug: strconv.Itoa is intentionally used without error (suppressed)
func ignoreConversion() {
	_ = strconv.Itoa(42) // This is fine — Itoa doesn't return error
}
`)

	root := mustParse(t, source)

	// Run all rules
	allFindings := collectAllRuleFindings(root, source, "corpus_buggy.go")

	// We expect at least one finding per rule category
	expectedRules := map[string]bool{
		"ckb/bug/defer-in-loop":        false,
		"ckb/bug/unreachable-code":     false,
		"ckb/bug/empty-error-branch":   false,
		"ckb/bug/unchecked-type-assert": false,
		"ckb/bug/self-assignment":      false,
		"ckb/bug/nil-after-deref":      false,
		"ckb/bug/identical-branches":   false,
		"ckb/bug/shadowed-err":         false,
		"ckb/bug/discarded-error":      false,
		"ckb/bug/missing-defer-close":  false,
	}

	for _, f := range allFindings {
		if _, ok := expectedRules[f.RuleID]; ok {
			expectedRules[f.RuleID] = true
		}
	}

	for rule, found := range expectedRules {
		if !found {
			t.Errorf("corpus: expected rule %s to fire but it didn't", rule)
		}
	}

	t.Logf("corpus: %d total findings across %d rules", len(allFindings), len(expectedRules))
	for _, f := range allFindings {
		t.Logf("  line %3d  %-35s %s", f.StartLine, f.RuleID, f.Message)
	}
}

// TestBugPatternCorpus_CleanCode verifies zero false positives on idiomatic Go.
func TestBugPatternCorpus_CleanCode(t *testing.T) {
	t.Parallel()

	source := []byte(`package clean

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// Properly closed resource with defer
func readFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open: %w", err)
	}
	defer f.Close()
	return io.ReadAll(f)
}

// Error properly handled
func parseJSON(data []byte) (map[string]interface{}, error) {
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}

// Two-value type assertion (not a bug)
func safeAssert(v interface{}) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// Builder writes (infallible, safe to discard)
func buildString(parts []string) string {
	var b strings.Builder
	for _, p := range parts {
		b.WriteString(p)
		b.WriteString(", ")
	}
	return b.String()
}

// bytes.Buffer writes (infallible, safe to discard)
func buildBytes(parts [][]byte) []byte {
	b := &bytes.Buffer{}
	for _, p := range parts {
		b.Write(p)
	}
	return b.Bytes()
}

// Proper nil check before use
func processOptional(r io.Reader) error {
	if r == nil {
		return fmt.Errorf("reader is nil")
	}
	data := make([]byte, 100)
	_, err := r.Read(data)
	return err
}

// Different branches (not identical)
func sign(n int) string {
	if n > 0 {
		return "positive"
	} else {
		return "non-positive"
	}
}

// Err not shadowed — uses = not :=
func twoSteps() error {
	_, err := fmt.Println("step 1")
	if err == nil {
		_, err = fmt.Println("step 2") // = not :=, no shadow
	}
	return err
}

// Defer outside loop is fine
func closeAfterLoop(paths []string) error {
	f, err := os.Create("output.txt")
	if err != nil {
		return err
	}
	defer f.Close()
	for _, p := range paths {
		_, err = fmt.Fprintln(f, p)
		if err != nil {
			return err
		}
	}
	return nil
}

// No unreachable code
func earlyReturn(x int) string {
	if x < 0 {
		return "negative"
	}
	return "non-negative"
}
`)

	root := mustParse(t, source)
	allFindings := collectAllRuleFindings(root, source, "corpus_clean.go")

	if len(allFindings) != 0 {
		t.Errorf("expected 0 findings for clean code corpus, got %d:", len(allFindings))
		for _, f := range allFindings {
			t.Logf("  line %3d  %-35s %s", f.StartLine, f.RuleID, f.Message)
		}
	}
}

// collectAllRuleFindings runs all 10 bug-pattern rules and returns all findings.
func collectAllRuleFindings(root *sitter.Node, source []byte, file string) []ReviewFinding {
	var all []ReviewFinding
	all = append(all, checkDeferInLoop(root, source, file)...)
	all = append(all, checkUnreachableCode(root, source, file)...)
	all = append(all, checkEmptyErrorBranch(root, source, file)...)
	all = append(all, checkUncheckedTypeAssert(root, source, file)...)
	all = append(all, checkSelfAssignment(root, source, file)...)
	all = append(all, checkNilAfterDeref(root, source, file)...)
	all = append(all, checkIdenticalBranches(root, source, file)...)
	all = append(all, checkShadowedErr(root, source, file)...)
	all = append(all, checkDiscardedError(root, source, file)...)
	all = append(all, checkMissingDeferClose(root, source, file)...)
	return all
}
