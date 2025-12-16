package output

// Module represents a module with impact and symbol counts
type Module struct {
	ModuleId    string  `json:"moduleId"`
	Name        string  `json:"name"`
	ImpactCount int     `json:"impactCount"`
	SymbolCount int     `json:"symbolCount"`
	Confidence  float64 `json:"confidence,omitempty"`
}

// Symbol represents a symbol with confidence and reference count
type Symbol struct {
	StableId   string  `json:"stableId"`
	Name       string  `json:"name"`
	Confidence float64 `json:"confidence"`
	RefCount   int     `json:"refCount"`
	Kind       string  `json:"kind,omitempty"`
}

// Reference represents a code reference location
type Reference struct {
	FileId      string `json:"fileId"`
	StartLine   int    `json:"startLine"`
	StartColumn int    `json:"startColumn"`
	EndLine     int    `json:"endLine"`
	EndColumn   int    `json:"endColumn"`
}

// ImpactItem represents an item affected by a change
type ImpactItem struct {
	StableId   string  `json:"stableId"`
	Name       string  `json:"name,omitempty"`
	Kind       string  `json:"kind"`
	Confidence float64 `json:"confidence"`
}

// Drilldown represents a suggested follow-up query
type Drilldown struct {
	Label          string  `json:"label"`
	Query          string  `json:"query"`
	RelevanceScore float64 `json:"relevanceScore"`
}

// Warning represents a warning message
type Warning struct {
	Severity string `json:"severity"`
	Text     string `json:"text"`
	Code     string `json:"code,omitempty"`
}
