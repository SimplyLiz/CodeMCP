package query

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// --- Code Health Tests ---

func TestHealthGrade(t *testing.T) {
	tests := []struct {
		score int
		want  string
	}{
		{95, "A"},
		{90, "A"},
		{89, "B"},
		{70, "B"},
		{69, "C"},
		{50, "C"},
		{49, "D"},
		{30, "D"},
		{29, "F"},
		{0, "F"},
	}

	for _, tt := range tests {
		got := healthGrade(tt.score)
		if got != tt.want {
			t.Errorf("healthGrade(%d) = %q, want %q", tt.score, got, tt.want)
		}
	}
}

func TestComplexityToScore(t *testing.T) {
	tests := []struct {
		complexity int
		want       float64
	}{
		{3, 100},
		{5, 100},
		{7, 85},
		{10, 85},
		{15, 65},
		{25, 40},
		{35, 20},
	}

	for _, tt := range tests {
		got := complexityToScore(tt.complexity)
		if got != tt.want {
			t.Errorf("complexityToScore(%d) = %.0f, want %.0f", tt.complexity, got, tt.want)
		}
	}
}

func TestFileSizeToScore(t *testing.T) {
	tests := []struct {
		loc  int
		want float64
	}{
		{50, 100},
		{100, 100},
		{200, 85},
		{400, 70},
		{700, 50},
		{1500, 30},
	}

	for _, tt := range tests {
		got := fileSizeToScore(tt.loc)
		if got != tt.want {
			t.Errorf("fileSizeToScore(%d) = %.0f, want %.0f", tt.loc, got, tt.want)
		}
	}
}

func TestCountLines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.go")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	got := countLines(path)
	if got != 3 {
		t.Errorf("countLines() = %d, want 3", got)
	}
}

func TestCountLines_Missing(t *testing.T) {
	got := countLines("/nonexistent/path")
	if got != 0 {
		t.Errorf("countLines(missing) = %d, want 0", got)
	}
}

func TestCodeHealthReport_Fields(t *testing.T) {
	report := &CodeHealthReport{
		Deltas: []CodeHealthDelta{
			{File: "a.go", HealthBefore: 80, HealthAfter: 70, Delta: -10, Grade: "B", GradeBefore: "B"},
			{File: "b.go", HealthBefore: 60, HealthAfter: 65, Delta: 5, Grade: "C", GradeBefore: "C"},
			{File: "c.go", HealthBefore: 90, HealthAfter: 90, Delta: 0, Grade: "A", GradeBefore: "A"},
		},
	}

	// Count degraded/improved
	for _, d := range report.Deltas {
		if d.Delta < 0 {
			report.Degraded++
		}
		if d.Delta > 0 {
			report.Improved++
		}
	}

	if report.Degraded != 1 {
		t.Errorf("Degraded = %d, want 1", report.Degraded)
	}
	if report.Improved != 1 {
		t.Errorf("Improved = %d, want 1", report.Improved)
	}
}

func TestHealthWeights(t *testing.T) {
	const epsilon = 0.001
	sum := weightCyclomatic + weightCognitive + weightFileSize + weightChurn + weightCoupling + weightBusFactor + weightAge
	if diff := sum - 1.0; diff > epsilon || diff < -epsilon {
		t.Errorf("health weights sum to %.3f, want 1.0", sum)
	}

	// Cognitive complexity should weigh more than cyclomatic (design intent:
	// cognitive is a better proxy for readability than raw branch count).
	if weightCognitive <= weightCyclomatic {
		t.Errorf("weightCognitive (%.2f) should be > weightCyclomatic (%.2f)", weightCognitive, weightCyclomatic)
	}
}

func TestCheckCodeHealth_NoFiles(t *testing.T) {
	e := &Engine{repoRoot: t.TempDir()}
	ctx := context.Background()

	check, findings, report := e.checkCodeHealth(ctx, nil, ReviewPROptions{})

	if check.Name != "health" {
		t.Errorf("check.Name = %q, want %q", check.Name, "health")
	}
	if check.Status != "pass" {
		t.Errorf("check.Status = %q, want %q", check.Status, "pass")
	}
	if len(findings) != 0 {
		t.Errorf("len(findings) = %d, want 0", len(findings))
	}
	if len(report.Deltas) != 0 {
		t.Errorf("len(report.Deltas) = %d, want 0", len(report.Deltas))
	}
}

// --- Baseline Tests ---

func TestFingerprintFinding(t *testing.T) {
	f1 := ReviewFinding{RuleID: "ckb/secrets/api-key", File: "config.go", Message: "API key detected"}
	f2 := ReviewFinding{RuleID: "ckb/secrets/api-key", File: "config.go", Message: "API key detected"}
	f3 := ReviewFinding{RuleID: "ckb/secrets/api-key", File: "other.go", Message: "API key detected"}

	fp1 := fingerprintFinding(f1)
	fp2 := fingerprintFinding(f2)
	fp3 := fingerprintFinding(f3)

	if fp1 != fp2 {
		t.Errorf("identical findings should have same fingerprint: %s != %s", fp1, fp2)
	}
	if fp1 == fp3 {
		t.Error("different files should have different fingerprints")
	}
	if len(fp1) != 16 {
		t.Errorf("fingerprint length = %d, want 16", len(fp1))
	}
}

func TestSaveAndLoadBaseline(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{repoRoot: dir}

	findings := []ReviewFinding{
		{RuleID: "rule1", File: "a.go", Message: "msg1", Severity: "error"},
		{RuleID: "rule2", File: "b.go", Message: "msg2", Severity: "warning"},
	}

	err := e.SaveBaseline(findings, "test-tag", "main", "feature")
	if err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	// Verify file exists
	path := filepath.Join(dir, ".ckb", "baselines", "test-tag.json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Fatal("baseline file not created")
	}

	// Load it back
	baseline, err := e.LoadBaseline("test-tag")
	if err != nil {
		t.Fatalf("LoadBaseline: %v", err)
	}

	if baseline.Tag != "test-tag" {
		t.Errorf("Tag = %q, want %q", baseline.Tag, "test-tag")
	}
	if baseline.FindingCount != 2 {
		t.Errorf("FindingCount = %d, want 2", baseline.FindingCount)
	}
	if baseline.BaseBranch != "main" {
		t.Errorf("BaseBranch = %q, want %q", baseline.BaseBranch, "main")
	}
	if len(baseline.Fingerprints) != 2 {
		t.Errorf("len(Fingerprints) = %d, want 2", len(baseline.Fingerprints))
	}
}

func TestSaveBaseline_AutoTag(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{repoRoot: dir}

	err := e.SaveBaseline(nil, "", "main", "HEAD")
	if err != nil {
		t.Fatalf("SaveBaseline with auto-tag: %v", err)
	}

	// Should create a file with timestamp-based name
	baselines, err := e.ListBaselines()
	if err != nil {
		t.Fatalf("ListBaselines: %v", err)
	}
	if len(baselines) != 1 {
		t.Fatalf("expected 1 baseline, got %d", len(baselines))
	}
}

func TestSaveBaseline_LatestCopy(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{repoRoot: dir}

	err := e.SaveBaseline(nil, "v1", "main", "HEAD")
	if err != nil {
		t.Fatalf("SaveBaseline: %v", err)
	}

	// latest.json should also exist
	latest, err := e.LoadBaseline("latest")
	if err != nil {
		t.Fatalf("LoadBaseline(latest): %v", err)
	}
	if latest.Tag != "v1" {
		t.Errorf("latest.Tag = %q, want %q", latest.Tag, "v1")
	}
}

func TestListBaselines_Empty(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{repoRoot: dir}

	baselines, err := e.ListBaselines()
	if err != nil {
		t.Fatalf("ListBaselines: %v", err)
	}
	if baselines != nil {
		t.Errorf("expected nil, got %v", baselines)
	}
}

func TestListBaselines_Sorted(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{repoRoot: dir}

	// Save two baselines with some time gap
	_ = e.SaveBaseline(nil, "older", "main", "HEAD")
	time.Sleep(10 * time.Millisecond)
	_ = e.SaveBaseline([]ReviewFinding{{RuleID: "r1", File: "a.go", Message: "m"}}, "newer", "main", "HEAD")

	baselines, err := e.ListBaselines()
	if err != nil {
		t.Fatalf("ListBaselines: %v", err)
	}
	if len(baselines) != 2 {
		t.Fatalf("expected 2, got %d", len(baselines))
	}
	// Should be sorted newest first
	if baselines[0].Tag != "newer" {
		t.Errorf("first baseline tag = %q, want %q", baselines[0].Tag, "newer")
	}
}

func TestLoadBaseline_NotFound(t *testing.T) {
	dir := t.TempDir()
	e := &Engine{repoRoot: dir}

	_, err := e.LoadBaseline("nonexistent")
	if err == nil {
		t.Error("expected error for missing baseline")
	}
}

func TestCompareWithBaseline(t *testing.T) {
	// Create baseline with 3 findings
	baseline := &ReviewBaseline{
		Tag:          "test",
		FindingCount: 3,
		Fingerprints: make(map[string]BaselineFinding),
	}

	baselineFindings := []ReviewFinding{
		{RuleID: "rule1", File: "a.go", Message: "issue A", Severity: "error"},
		{RuleID: "rule2", File: "b.go", Message: "issue B", Severity: "warning"},
		{RuleID: "rule3", File: "c.go", Message: "issue C", Severity: "info"},
	}

	for _, f := range baselineFindings {
		fp := fingerprintFinding(f)
		baseline.Fingerprints[fp] = BaselineFinding{
			Fingerprint: fp,
			RuleID:      f.RuleID,
			File:        f.File,
			Message:     f.Message,
			Severity:    f.Severity,
		}
	}

	// Current: keep A, remove B, add D
	current := []ReviewFinding{
		{RuleID: "rule1", File: "a.go", Message: "issue A", Severity: "error"},   // unchanged
		{RuleID: "rule4", File: "d.go", Message: "issue D", Severity: "warning"}, // new
	}

	newF, unchanged, resolved := CompareWithBaseline(current, baseline)

	if len(newF) != 1 {
		t.Errorf("new findings = %d, want 1", len(newF))
	}
	if len(unchanged) != 1 {
		t.Errorf("unchanged findings = %d, want 1", len(unchanged))
	}
	if len(resolved) != 2 {
		t.Errorf("resolved findings = %d, want 2", len(resolved))
	}

	// Verify the new finding is D
	if len(newF) > 0 && newF[0].RuleID != "rule4" {
		t.Errorf("new finding ruleID = %q, want %q", newF[0].RuleID, "rule4")
	}
}

func TestCompareWithBaseline_EmptyBaseline(t *testing.T) {
	baseline := &ReviewBaseline{
		Fingerprints: make(map[string]BaselineFinding),
	}

	current := []ReviewFinding{
		{RuleID: "rule1", File: "a.go", Message: "issue"},
	}

	newF, unchanged, resolved := CompareWithBaseline(current, baseline)

	if len(newF) != 1 {
		t.Errorf("new = %d, want 1", len(newF))
	}
	if len(unchanged) != 0 {
		t.Errorf("unchanged = %d, want 0", len(unchanged))
	}
	if len(resolved) != 0 {
		t.Errorf("resolved = %d, want 0", len(resolved))
	}
}

func TestCompareWithBaseline_AllResolved(t *testing.T) {
	baseline := &ReviewBaseline{
		FindingCount: 2,
		Fingerprints: make(map[string]BaselineFinding),
	}

	for _, f := range []ReviewFinding{
		{RuleID: "rule1", File: "a.go", Message: "issue A"},
		{RuleID: "rule2", File: "b.go", Message: "issue B"},
	} {
		fp := fingerprintFinding(f)
		baseline.Fingerprints[fp] = BaselineFinding{
			Fingerprint: fp, RuleID: f.RuleID, File: f.File, Message: f.Message,
		}
	}

	newF, unchanged, resolved := CompareWithBaseline(nil, baseline)

	if len(newF) != 0 {
		t.Errorf("new = %d, want 0", len(newF))
	}
	if len(unchanged) != 0 {
		t.Errorf("unchanged = %d, want 0", len(unchanged))
	}
	if len(resolved) != 2 {
		t.Errorf("resolved = %d, want 2", len(resolved))
	}
}
