package compliance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecommendFrameworks_GoProjectWithHTTPAndPII(t *testing.T) {
	tmp := t.TempDir()

	// Create go.mod so hasDependencies triggers
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module example.com/app\n\ngo 1.21\n"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a .go file with net/http import (triggers hasHTTP)
	httpFile := filepath.Join(tmp, "server.go")
	if err := os.WriteFile(httpFile, []byte(`package main

import "net/http"

func main() {
	http.ListenAndServe(":8080", nil)
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a .go file with an email field (triggers hasPII)
	piiFile := filepath.Join(tmp, "user.go")
	if err := os.WriteFile(piiFile, []byte(`package main

type User struct {
	Name  string
	Email string
}
`), 0644); err != nil {
		t.Fatal(err)
	}

	recs, err := RecommendFrameworks(tmp)
	if err != nil {
		t.Fatalf("RecommendFrameworks returned error: %v", err)
	}

	// Build a set of recommended framework IDs for easy lookup
	got := make(map[FrameworkID]bool)
	for _, r := range recs {
		got[r.Framework] = true
	}

	// Universal frameworks -- always recommended
	if !got[FrameworkISO27001] {
		t.Error("expected iso27001 (universal) to be recommended")
	}
	if !got[FrameworkOWASPASVS] {
		t.Error("expected owasp-asvs (universal) to be recommended")
	}

	// HTTP detected
	if !got[FrameworkNIST80053] {
		t.Error("expected nist-800-53 to be recommended (HTTP detected)")
	}

	// PII detected
	if !got[FrameworkGDPR] {
		t.Error("expected gdpr to be recommended (PII detected)")
	}

	// C/C++ safety frameworks should NOT be recommended for a Go project
	if got[FrameworkIEC61508] {
		t.Error("iec61508 should NOT be recommended for a Go project")
	}
	if got[FrameworkDO178C] {
		t.Error("do-178c should NOT be recommended for a Go project")
	}

	// Dependencies detected (go.mod)
	if !got[FrameworkSBOM] {
		t.Error("expected sbom/slsa to be recommended (go.mod present)")
	}
}

func TestRecommendFrameworks_EmptyDirectory(t *testing.T) {
	tmp := t.TempDir()

	recs, err := RecommendFrameworks(tmp)
	if err != nil {
		t.Fatalf("RecommendFrameworks returned error: %v", err)
	}

	got := make(map[FrameworkID]bool)
	for _, r := range recs {
		got[r.Framework] = true
	}

	// Universal frameworks should still be recommended
	if !got[FrameworkISO27001] {
		t.Error("expected iso27001 (universal) even for empty directory")
	}
	if !got[FrameworkOWASPASVS] {
		t.Error("expected owasp-asvs (universal) even for empty directory")
	}

	// Nothing domain-specific should fire
	if got[FrameworkGDPR] {
		t.Error("gdpr should NOT be recommended for empty directory")
	}
	if got[FrameworkNIST80053] {
		t.Error("nist-800-53 should NOT be recommended for empty directory")
	}
	if got[FrameworkIEC61508] {
		t.Error("iec61508 should NOT be recommended for empty directory")
	}
	if got[FrameworkDO178C] {
		t.Error("do-178c should NOT be recommended for empty directory")
	}
}

func TestRecommendFrameworks_NoDuplicates(t *testing.T) {
	tmp := t.TempDir()

	recs, err := RecommendFrameworks(tmp)
	if err != nil {
		t.Fatalf("RecommendFrameworks returned error: %v", err)
	}

	seen := make(map[FrameworkID]int)
	for _, r := range recs {
		seen[r.Framework]++
		if seen[r.Framework] > 1 {
			t.Errorf("duplicate recommendation for framework %s", r.Framework)
		}
	}
}

func TestRecommendFrameworks_RecommendationFields(t *testing.T) {
	tmp := t.TempDir()

	recs, err := RecommendFrameworks(tmp)
	if err != nil {
		t.Fatal(err)
	}

	for _, r := range recs {
		if r.Framework == "" {
			t.Error("recommendation has empty Framework")
		}
		if r.Name == "" {
			t.Errorf("recommendation %s has empty Name", r.Framework)
		}
		if r.Reason == "" {
			t.Errorf("recommendation %s has empty Reason", r.Framework)
		}
		if r.Confidence <= 0 || r.Confidence > 1.0 {
			t.Errorf("recommendation %s has out-of-range Confidence: %f", r.Framework, r.Confidence)
		}
		if r.Category == "" {
			t.Errorf("recommendation %s has empty Category", r.Framework)
		}
	}
}
