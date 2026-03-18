package query

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// ReviewBaseline stores a snapshot of findings for comparison.
type ReviewBaseline struct {
	Tag          string                     `json:"tag"`
	CreatedAt    time.Time                  `json:"createdAt"`
	BaseBranch   string                     `json:"baseBranch"`
	HeadBranch   string                     `json:"headBranch"`
	FindingCount int                        `json:"findingCount"`
	Fingerprints map[string]BaselineFinding `json:"fingerprints"` // fingerprint → finding
}

// BaselineFinding stores a finding with its fingerprint for matching.
type BaselineFinding struct {
	Fingerprint string `json:"fingerprint"`
	RuleID      string `json:"ruleId"`
	File        string `json:"file"`
	Message     string `json:"message"`
	Severity    string `json:"severity"`
	FirstSeen   string `json:"firstSeen"` // ISO8601
}

// FindingLifecycle classifies a finding relative to a baseline.
type FindingLifecycle struct {
	Status      string `json:"status"`      // "new", "unchanged", "resolved"
	BaselineTag string `json:"baselineTag"` // Which baseline it's compared against
	FirstSeen   string `json:"firstSeen"`   // When this finding was first detected
}

// BaselineInfo provides metadata about a stored baseline.
type BaselineInfo struct {
	Tag          string    `json:"tag"`
	CreatedAt    time.Time `json:"createdAt"`
	FindingCount int       `json:"findingCount"`
	Path         string    `json:"path"`
}

// baselineDir returns the directory for baseline storage.
func baselineDir(repoRoot string) string {
	return filepath.Join(repoRoot, ".ckb", "baselines")
}

// SaveBaseline saves the current findings as a baseline snapshot.
func (e *Engine) SaveBaseline(findings []ReviewFinding, tag string, baseBranch, headBranch string) error {
	dir := baselineDir(e.repoRoot)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create baseline dir: %w", err)
	}

	if tag == "" {
		tag = time.Now().Format("20060102-150405")
	}

	baseline := ReviewBaseline{
		Tag:          tag,
		CreatedAt:    time.Now(),
		BaseBranch:   baseBranch,
		HeadBranch:   headBranch,
		FindingCount: len(findings),
		Fingerprints: make(map[string]BaselineFinding),
	}

	now := time.Now().Format(time.RFC3339)
	for _, f := range findings {
		fp := fingerprintFinding(f)
		baseline.Fingerprints[fp] = BaselineFinding{
			Fingerprint: fp,
			RuleID:      f.RuleID,
			File:        f.File,
			Message:     f.Message,
			Severity:    f.Severity,
			FirstSeen:   now,
		}
	}

	data, err := json.MarshalIndent(baseline, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal baseline: %w", err)
	}

	path := filepath.Join(dir, tag+".json")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("write baseline: %w", err)
	}

	// Update "latest" symlink
	latestPath := filepath.Join(dir, "latest.json")
	_ = os.Remove(latestPath) // ignore error if doesn't exist
	if err := os.WriteFile(latestPath, data, 0644); err != nil {
		return fmt.Errorf("write latest baseline: %w", err)
	}

	return nil
}

// LoadBaseline loads a baseline by tag (or "latest").
func (e *Engine) LoadBaseline(tag string) (*ReviewBaseline, error) {
	dir := baselineDir(e.repoRoot)
	path := filepath.Join(dir, tag+".json")

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read baseline %q: %w", tag, err)
	}

	var baseline ReviewBaseline
	if err := json.Unmarshal(data, &baseline); err != nil {
		return nil, fmt.Errorf("parse baseline: %w", err)
	}

	return &baseline, nil
}

// ListBaselines returns available baselines sorted by creation time.
func (e *Engine) ListBaselines() ([]BaselineInfo, error) {
	dir := baselineDir(e.repoRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("list baselines: %w", err)
	}

	var infos []BaselineInfo
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		name := entry.Name()
		if name == "latest.json" {
			continue
		}
		tag := name[:len(name)-5] // strip .json

		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var baseline ReviewBaseline
		if err := json.Unmarshal(data, &baseline); err != nil {
			continue
		}

		infos = append(infos, BaselineInfo{
			Tag:          tag,
			CreatedAt:    baseline.CreatedAt,
			FindingCount: baseline.FindingCount,
			Path:         path,
		})
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].CreatedAt.After(infos[j].CreatedAt)
	})

	return infos, nil
}

// CompareWithBaseline classifies current findings against a baseline.
func CompareWithBaseline(current []ReviewFinding, baseline *ReviewBaseline) (newFindings, unchanged, resolved []ReviewFinding) {
	currentFPs := make(map[string]ReviewFinding)
	for _, f := range current {
		fp := fingerprintFinding(f)
		currentFPs[fp] = f
	}

	// Check which baseline findings are still present
	for fp, bf := range baseline.Fingerprints {
		if _, exists := currentFPs[fp]; exists {
			unchanged = append(unchanged, currentFPs[fp])
			delete(currentFPs, fp)
		} else {
			// Finding was resolved
			resolved = append(resolved, ReviewFinding{
				Check:    bf.RuleID,
				Severity: bf.Severity,
				File:     bf.File,
				Message:  bf.Message,
				RuleID:   bf.RuleID,
			})
		}
	}

	// Remaining current findings are new
	for _, f := range currentFPs {
		newFindings = append(newFindings, f)
	}

	return newFindings, unchanged, resolved
}

// fingerprintFinding creates a stable fingerprint for a finding.
// Uses ruleId + file + message hash to survive line shifts.
func fingerprintFinding(f ReviewFinding) string {
	h := sha256.New()
	h.Write([]byte(f.RuleID))
	h.Write([]byte{0})
	h.Write([]byte(f.File))
	h.Write([]byte{0})
	h.Write([]byte(f.Message))
	return hex.EncodeToString(h.Sum(nil))[:16]
}
