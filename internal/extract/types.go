package extract

// FlowAnalysis describes the variable flow for a code extraction region.
type FlowAnalysis struct {
	Parameters []FlowVariable `json:"parameters"`
	Returns    []FlowVariable `json:"returns"`
	Locals     []FlowVariable `json:"locals"`
}

// FlowVariable describes a variable's role in the extraction region.
type FlowVariable struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"`
	DefinedAt   int    `json:"definedAt"`
	FirstUsedAt int    `json:"firstUsedAt"`
	UsageCount  int    `json:"usageCount"`
	IsModified  bool   `json:"isModified"`
}

// AnalyzeOptions configures variable flow analysis for an extraction region.
type AnalyzeOptions struct {
	Source    []byte
	Language  string // from complexity.Language
	StartLine int
	EndLine   int
}
