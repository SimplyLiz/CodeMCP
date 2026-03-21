package query

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// DismissedFinding records a user-dismissed finding.
type DismissedFinding struct {
	RuleID      string    `json:"ruleId"`
	File        string    `json:"file,omitempty"` // empty = dismiss rule globally
	Reason      string    `json:"reason,omitempty"`
	DismissedAt time.Time `json:"dismissedAt"`
}

// DismissalStore persists dismissed findings to .ckb/review-dismissals.json
type DismissalStore struct {
	Dismissals []DismissedFinding `json:"dismissals"`
	path       string
}

// LoadDismissals loads the dismissal store from disk.
func LoadDismissals(repoRoot string) *DismissalStore {
	store := &DismissalStore{
		path: filepath.Join(repoRoot, ".ckb", "review-dismissals.json"),
	}
	data, err := os.ReadFile(store.path)
	if err != nil {
		return store // empty store
	}
	_ = json.Unmarshal(data, store)
	return store
}

// Save writes the dismissal store to disk.
func (s *DismissalStore) Save() error {
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0750); err != nil { // #nosec G301 -- .ckb directory, user-scoped
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0600) // #nosec G306 -- user config file
}

// Dismiss adds a finding to the dismissed list.
func (s *DismissalStore) Dismiss(ruleID, file, reason string) {
	s.Dismissals = append(s.Dismissals, DismissedFinding{
		RuleID:      ruleID,
		File:        file,
		Reason:      reason,
		DismissedAt: time.Now(),
	})
}

// IsDismissed checks if a finding matches a dismissed rule+file.
func (s *DismissalStore) IsDismissed(ruleID, file string) bool {
	for _, d := range s.Dismissals {
		// Global dismissal (no file specified) matches any file
		if d.RuleID == ruleID && d.File == "" {
			return true
		}
		// File-specific dismissal
		if d.RuleID == ruleID && d.File == file {
			return true
		}
	}
	return false
}

// FilterDismissed removes dismissed findings from a list.
func (s *DismissalStore) FilterDismissed(findings []ReviewFinding) (filtered []ReviewFinding, dismissed int) {
	for _, f := range findings {
		if s.IsDismissed(f.RuleID, f.File) {
			dismissed++
			continue
		}
		filtered = append(filtered, f)
	}
	return
}
