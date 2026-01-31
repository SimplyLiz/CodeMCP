package suggest

// SuggestionType categorizes the kind of refactoring opportunity.
type SuggestionType string

const (
	SuggestExtractFunction  SuggestionType = "extract_function"
	SuggestSplitFile        SuggestionType = "split_file"
	SuggestReduceCoupling   SuggestionType = "reduce_coupling"
	SuggestRemoveDeadCode   SuggestionType = "remove_dead_code"
	SuggestAddTests         SuggestionType = "add_tests"
	SuggestSimplifyFunction SuggestionType = "simplify_function"
)

// Suggestion describes a single refactoring opportunity.
type Suggestion struct {
	Type        SuggestionType `json:"type"`
	Severity    string         `json:"severity"` // critical/high/medium/low
	Target      string         `json:"target"`   // file or function
	Title       string         `json:"title"`
	Description string         `json:"description"`
	Rationale   []string       `json:"rationale"`
	Effort      string         `json:"effort"` // small/medium/large
	Priority    int            `json:"priority"`
}

// SuggestResult contains all detected refactoring suggestions.
type SuggestResult struct {
	Suggestions []Suggestion    `json:"suggestions"`
	Summary     *SuggestSummary `json:"summary"`
	TotalFound  int             `json:"totalFound"`
}

// SuggestSummary provides aggregate counts by severity and type.
type SuggestSummary struct {
	BySeverity map[string]int `json:"bySeverity"`
	ByType     map[string]int `json:"byType"`
}

// AnalyzeOptions configures suggestion detection.
type AnalyzeOptions struct {
	Scope       string   // directory or file path to analyze
	MinSeverity string   // minimum severity to include (default: "low")
	Types       []string // filter by suggestion type (empty = all)
	Limit       int      // max results (default: 50)
}
