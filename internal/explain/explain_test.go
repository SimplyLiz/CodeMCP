package explain

import (
	"testing"
	"time"

	"ckb/internal/logging"
)

func TestParseSymbolQuery(t *testing.T) {
	explainer := &Explainer{}

	tests := []struct {
		query    string
		wantFile string
		wantLine int
	}{
		{"src/main.go", "src/main.go", 0},
		{"src/main.go:42", "src/main.go", 42},
		{"src/main.go:10", "src/main.go", 10},
		{"path/to/file.ts:100", "path/to/file.ts", 100},
		{"path/to/file.ts", "path/to/file.ts", 0},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			gotFile, gotLine := explainer.parseSymbolQuery(tt.query)
			if gotFile != tt.wantFile {
				t.Errorf("parseSymbolQuery(%q) file = %q, want %q", tt.query, gotFile, tt.wantFile)
			}
			if gotLine != tt.wantLine {
				t.Errorf("parseSymbolQuery(%q) line = %d, want %d", tt.query, gotLine, tt.wantLine)
			}
		})
	}
}

func TestWarningSeverityConstants(t *testing.T) {
	// Ensure severity constants are defined
	if SeverityInfo == "" {
		t.Error("SeverityInfo should not be empty")
	}
	if SeverityWarning == "" {
		t.Error("SeverityWarning should not be empty")
	}
	if SeverityCritical == "" {
		t.Error("SeverityCritical should not be empty")
	}
}

func TestWarningTypeConstants(t *testing.T) {
	tests := []struct {
		warningType string
	}{
		{WarningTemporaryCode},
		{WarningBusFactor},
		{WarningHighCoupling},
		{WarningStale},
		{WarningComplex},
	}

	for _, tt := range tests {
		t.Run(tt.warningType, func(t *testing.T) {
			if tt.warningType == "" {
				t.Error("Warning type should not be empty")
			}
		})
	}
}

func TestOriginStructure(t *testing.T) {
	// Test Origin struct can be created properly
	origin := Origin{
		CommitSha:     "abc123",
		Author:        "Test User",
		Date:          time.Now(),
		CommitMessage: "Initial commit",
	}

	if origin.CommitSha != "abc123" {
		t.Errorf("Origin.CommitSha = %q, want %q", origin.CommitSha, "abc123")
	}
	if origin.Author != "Test User" {
		t.Errorf("Origin.Author = %q, want %q", origin.Author, "Test User")
	}
	if origin.Date.IsZero() {
		t.Error("Origin.Date should not be zero")
	}
	if origin.CommitMessage != "Initial commit" {
		t.Errorf("Origin.CommitMessage = %q, want %q", origin.CommitMessage, "Initial commit")
	}
}

func TestEvolutionCalculation(t *testing.T) {
	// Test evolution structure
	evolution := Evolution{
		TotalCommits: 10,
		Timeline:     make([]TimelineEntry, 0),
	}

	if evolution.TotalCommits != 10 {
		t.Errorf("Evolution.TotalCommits = %d, want %d", evolution.TotalCommits, 10)
	}
	if len(evolution.Timeline) != 0 {
		t.Errorf("Evolution.Timeline length = %d, want 0", len(evolution.Timeline))
	}
}

func TestWarningStructure(t *testing.T) {
	warning := Warning{
		Type:     WarningBusFactor,
		Message:  "Only one contributor",
		Severity: SeverityWarning,
	}

	if warning.Type != WarningBusFactor {
		t.Errorf("Warning.Type = %q, want %q", warning.Type, WarningBusFactor)
	}
	if warning.Severity != SeverityWarning {
		t.Errorf("Warning.Severity = %q, want %q", warning.Severity, SeverityWarning)
	}
	if warning.Message != "Only one contributor" {
		t.Errorf("Warning.Message = %q, want %q", warning.Message, "Only one contributor")
	}
}

func TestReferencesStructure(t *testing.T) {
	refs := References{
		Issues:      []string{"#123", "#456"},
		PRs:         []string{"#789"},
		JiraTickets: []string{"PROJ-123"},
	}

	if len(refs.Issues) != 2 {
		t.Errorf("len(Issues) = %d, want %d", len(refs.Issues), 2)
	}
	if len(refs.PRs) != 1 {
		t.Errorf("len(PRs) = %d, want %d", len(refs.PRs), 1)
	}
	if len(refs.JiraTickets) != 1 || refs.JiraTickets[0] != "PROJ-123" {
		t.Errorf("JiraTickets = %v, want [PROJ-123]", refs.JiraTickets)
	}
}

func TestSymbolExplanationStructure(t *testing.T) {
	exp := SymbolExplanation{
		Symbol: "TestFunc",
		File:   "test.go",
		Origin: Origin{
			Author:        "Test Author",
			CommitMessage: "Add test function",
		},
		Warnings: []Warning{
			{Type: WarningStale, Message: "Not touched in 12 months", Severity: SeverityWarning},
		},
	}

	if exp.Symbol != "TestFunc" {
		t.Errorf("SymbolExplanation.Symbol = %q, want %q", exp.Symbol, "TestFunc")
	}
	if exp.File != "test.go" {
		t.Errorf("SymbolExplanation.File = %q, want %q", exp.File, "test.go")
	}
	if len(exp.Warnings) != 1 {
		t.Errorf("len(Warnings) = %d, want %d", len(exp.Warnings), 1)
	}
}

func TestNewExplainer(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	explainer := NewExplainer("/path/to/repo", logger)

	if explainer == nil {
		t.Fatal("NewExplainer returned nil")
	}
	if explainer.repoRoot != "/path/to/repo" {
		t.Errorf("repoRoot = %q, want %q", explainer.repoRoot, "/path/to/repo")
	}
}

func TestFindOrigin(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	explainer := NewExplainer("/tmp", logger)

	now := time.Now()

	tests := []struct {
		name     string
		commits  []commitInfo
		wantSha  string
		wantAuth string
	}{
		{
			name:    "empty commits",
			commits: []commitInfo{},
		},
		{
			name: "single commit",
			commits: []commitInfo{
				{Hash: "abc123", Author: "Alice", Timestamp: now, Message: "Initial"},
			},
			wantSha:  "abc123",
			wantAuth: "Alice",
		},
		{
			name: "multiple commits - oldest is last",
			commits: []commitInfo{
				{Hash: "ccc333", Author: "Charlie", Timestamp: now, Message: "Latest"},
				{Hash: "bbb222", Author: "Bob", Timestamp: now.AddDate(0, -1, 0), Message: "Middle"},
				{Hash: "aaa111", Author: "Alice", Timestamp: now.AddDate(-1, 0, 0), Message: "First"},
			},
			wantSha:  "aaa111",
			wantAuth: "Alice",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			origin := explainer.findOrigin(tt.commits)

			if tt.wantSha != "" && origin.CommitSha != tt.wantSha {
				t.Errorf("findOrigin() sha = %q, want %q", origin.CommitSha, tt.wantSha)
			}
			if tt.wantAuth != "" && origin.Author != tt.wantAuth {
				t.Errorf("findOrigin() author = %q, want %q", origin.Author, tt.wantAuth)
			}
		})
	}
}

func TestBuildEvolution(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	explainer := NewExplainer("/tmp", logger)

	now := time.Now()
	commits := []commitInfo{
		{Hash: "abc123456", Author: "Alice", Email: "alice@example.com", Timestamp: now, Message: "Latest"},
		{Hash: "def789012", Author: "Alice", Email: "alice@example.com", Timestamp: now.AddDate(0, -1, 0), Message: "Earlier"},
		{Hash: "ghi345678", Author: "Bob", Email: "bob@example.com", Timestamp: now.AddDate(0, -2, 0), Message: "Oldest"},
	}

	evolution := explainer.buildEvolution(commits, 10)

	if evolution.TotalCommits != 3 {
		t.Errorf("TotalCommits = %d, want 3", evolution.TotalCommits)
	}

	if len(evolution.Contributors) != 2 {
		t.Errorf("len(Contributors) = %d, want 2", len(evolution.Contributors))
	}

	// Alice should be first (2 commits vs Bob's 1)
	if len(evolution.Contributors) > 0 && evolution.Contributors[0].Name != "Alice" {
		t.Errorf("First contributor = %q, want Alice", evolution.Contributors[0].Name)
	}

	if evolution.LastTouchedBy != "Alice" {
		t.Errorf("LastTouchedBy = %q, want Alice", evolution.LastTouchedBy)
	}

	// Check timeline length respects limit
	if len(evolution.Timeline) > 10 {
		t.Errorf("Timeline length = %d, should be <= 10", len(evolution.Timeline))
	}

	// Check short sha format
	if len(evolution.Timeline) > 0 && len(evolution.Timeline[0].Sha) != 8 {
		t.Errorf("Timeline sha length = %d, want 8", len(evolution.Timeline[0].Sha))
	}
}

func TestBuildEvolutionEmpty(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	explainer := NewExplainer("/tmp", logger)

	evolution := explainer.buildEvolution([]commitInfo{}, 10)

	if evolution.TotalCommits != 0 {
		t.Errorf("TotalCommits = %d, want 0", evolution.TotalCommits)
	}
}

func TestExtractReferences(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	explainer := NewExplainer("/tmp", logger)

	commits := []commitInfo{
		{Message: "Fix bug #123 and #456"},
		{Message: "PR #789 merged"},
		{Message: "Implements PROJ-100 and FEAT-200"},
	}

	refs := explainer.extractReferences(commits)

	// Check issues
	if len(refs.Issues) < 2 {
		t.Errorf("Issues count = %d, want >= 2", len(refs.Issues))
	}

	// Check PRs
	if len(refs.PRs) < 1 {
		t.Errorf("PRs count = %d, want >= 1", len(refs.PRs))
	}

	// Check JIRA tickets
	if len(refs.JiraTickets) < 2 {
		t.Errorf("JiraTickets count = %d, want >= 2", len(refs.JiraTickets))
	}
}

func TestAnalyzeWarnings(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	explainer := NewExplainer("/tmp", logger)

	tests := []struct {
		name           string
		origin         Origin
		evolution      Evolution
		coChanges      []CoChange
		wantWarningLen int
		wantType       string
	}{
		{
			name: "temporary code warning",
			origin: Origin{
				CommitMessage: "Add temporary hack for API",
				Date:          time.Now().AddDate(0, -6, 0), // 6 months ago
			},
			evolution:      Evolution{},
			wantWarningLen: 1,
			wantType:       WarningTemporaryCode,
		},
		{
			name:   "bus factor warning - single contributor",
			origin: Origin{Date: time.Now()},
			evolution: Evolution{
				Contributors: []Contributor{
					{Name: "Alice", LastCommit: time.Now()},
				},
			},
			wantWarningLen: 1,
			wantType:       WarningBusFactor,
		},
		{
			name:   "high coupling warning",
			origin: Origin{Date: time.Now()},
			evolution: Evolution{
				Contributors: []Contributor{
					{Name: "Alice", LastCommit: time.Now()},
					{Name: "Bob", LastCommit: time.Now()},
				},
			},
			coChanges: []CoChange{
				{Correlation: 0.8}, {Correlation: 0.9}, {Correlation: 0.75},
			},
			wantWarningLen: 1,
			wantType:       WarningHighCoupling,
		},
		{
			name:   "stale code warning",
			origin: Origin{Date: time.Now()},
			evolution: Evolution{
				LastTouched: time.Now().AddDate(-2, 0, 0), // 2 years ago
				Contributors: []Contributor{
					{Name: "Alice", LastCommit: time.Now()},
					{Name: "Bob", LastCommit: time.Now()},
				},
			},
			wantWarningLen: 1,
			wantType:       WarningStale,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings := explainer.analyzeWarnings(tt.origin, tt.evolution, tt.coChanges)

			if len(warnings) < tt.wantWarningLen {
				t.Errorf("len(warnings) = %d, want >= %d", len(warnings), tt.wantWarningLen)
				return
			}

			// Check for expected warning type
			if tt.wantType != "" {
				found := false
				for _, w := range warnings {
					if w.Type == tt.wantType {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Expected warning type %q not found in %v", tt.wantType, warnings)
				}
			}
		})
	}
}

func TestBuildOwnership(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	explainer := NewExplainer("/tmp", logger)

	evolution := Evolution{
		Contributors: []Contributor{
			{Name: "Alice", CommitCount: 10},
			{Name: "Bob", CommitCount: 5},
		},
		LastTouchedBy: "Bob",
	}

	ownership := explainer.buildOwnership(evolution)

	if ownership.PrimaryContact != "Alice" {
		t.Errorf("PrimaryContact = %q, want Alice", ownership.PrimaryContact)
	}
	if ownership.CurrentOwner != "Bob" {
		t.Errorf("CurrentOwner = %q, want Bob", ownership.CurrentOwner)
	}
}

func TestItoa(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{0, "0"},
		{1, "1"},
		{42, "42"},
		{-1, "-1"},
		{-42, "-42"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := itoa(tt.input)
			if got != tt.want {
				t.Errorf("itoa(%d) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestParseInt(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{"42", 42},
		{"0", 0},
		{"", 0},
		{"  10  ", 10},
		{"abc", 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, _ := parseInt(tt.input)
			if got != tt.want {
				t.Errorf("parseInt(%q) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}

func TestMonthsSince(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		time    time.Time
		wantMin int
		wantMax int
	}{
		{"now", now, 0, 0},
		{"1 month ago", now.AddDate(0, -1, 0), 1, 1},
		{"1 year ago", now.AddDate(-1, 0, 0), 11, 13},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := monthsSince(tt.time)
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("monthsSince() = %d, want between %d and %d", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestFormatAge(t *testing.T) {
	now := time.Now()

	tests := []struct {
		name    string
		time    time.Time
		wantSub string
	}{
		{"recent", now, "less than a month"},
		{"1 month", now.AddDate(0, -1, 0), "1 month"},
		{"6 months", now.AddDate(0, -6, 0), "months"},
		{"1 year", now.AddDate(-1, 0, 0), "year"},
		{"2 years", now.AddDate(-2, 0, 0), "years"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatAge(tt.time)
			if !containsStr(got, tt.wantSub) {
				t.Errorf("formatAge() = %q, want to contain %q", got, tt.wantSub)
			}
		})
	}
}

func TestCoChangeStructure(t *testing.T) {
	coChange := CoChange{
		File:          "related.go",
		Correlation:   0.85,
		CoChangeCount: 10,
		TotalChanges:  12,
	}

	if coChange.File != "related.go" {
		t.Errorf("CoChange.File = %q, want %q", coChange.File, "related.go")
	}
	if coChange.Correlation != 0.85 {
		t.Errorf("CoChange.Correlation = %v, want 0.85", coChange.Correlation)
	}
}

func TestContributorStructure(t *testing.T) {
	now := time.Now()
	contrib := Contributor{
		Name:        "Alice",
		Email:       "alice@example.com",
		CommitCount: 10,
		FirstCommit: now.AddDate(-1, 0, 0),
		LastCommit:  now,
	}

	if contrib.Name != "Alice" {
		t.Errorf("Contributor.Name = %q, want Alice", contrib.Name)
	}
	if contrib.CommitCount != 10 {
		t.Errorf("Contributor.CommitCount = %d, want 10", contrib.CommitCount)
	}
}

func TestOwnershipInfoStructure(t *testing.T) {
	info := OwnershipInfo{
		CurrentOwner:   "Alice",
		PrimaryContact: "Bob",
	}

	if info.CurrentOwner != "Alice" {
		t.Errorf("CurrentOwner = %q, want Alice", info.CurrentOwner)
	}
	if info.PrimaryContact != "Bob" {
		t.Errorf("PrimaryContact = %q, want Bob", info.PrimaryContact)
	}
}

func TestExplainOptionsStructure(t *testing.T) {
	opts := ExplainOptions{
		RepoRoot:        "/path/to/repo",
		Symbol:          "main.go:42",
		HistoryLimit:    20,
		IncludeCoChange: true,
	}

	if opts.RepoRoot != "/path/to/repo" {
		t.Errorf("RepoRoot = %q, want /path/to/repo", opts.RepoRoot)
	}
	if opts.HistoryLimit != 20 {
		t.Errorf("HistoryLimit = %d, want 20", opts.HistoryLimit)
	}
	if !opts.IncludeCoChange {
		t.Error("IncludeCoChange should be true")
	}
}

// Helper function
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
