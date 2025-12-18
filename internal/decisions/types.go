package decisions

import (
	"time"
)

// ArchitecturalDecision represents an ADR (Architectural Decision Record)
type ArchitecturalDecision struct {
	ID              string    `json:"id"`              // "ADR-001" style
	Title           string    `json:"title"`
	Status          string    `json:"status"`          // "proposed" | "accepted" | "deprecated" | "superseded"
	Context         string    `json:"context"`
	Decision        string    `json:"decision"`
	Consequences    []string  `json:"consequences"`
	AffectedModules []string  `json:"affectedModules"`
	Alternatives    []string  `json:"alternatives,omitempty"`
	SupersededBy    string    `json:"supersededBy,omitempty"`
	Author          string    `json:"author,omitempty"`
	Date            time.Time `json:"date"`
	LastReviewed    *time.Time `json:"lastReviewed,omitempty"`
	FilePath        string    `json:"filePath"`        // relative path to .md file
}

// ADRStatus represents valid ADR statuses
type ADRStatus string

const (
	StatusProposed   ADRStatus = "proposed"
	StatusAccepted   ADRStatus = "accepted"
	StatusDeprecated ADRStatus = "deprecated"
	StatusSuperseded ADRStatus = "superseded"
)

// IsValidStatus checks if a status string is valid
func IsValidStatus(status string) bool {
	switch ADRStatus(status) {
	case StatusProposed, StatusAccepted, StatusDeprecated, StatusSuperseded:
		return true
	default:
		return false
	}
}

// ADRTemplate is the template for generating new ADRs
const ADRTemplate = `# {{.ID}}: {{.Title}}

**Status:** {{.Status}}

**Date:** {{.Date}}
{{if .Author}}
**Author:** {{.Author}}
{{end}}
## Context

{{.Context}}

## Decision

{{.Decision}}

## Consequences

{{range .Consequences}}- {{.}}
{{end}}
{{if .AffectedModules}}
## Affected Modules

{{range .AffectedModules}}- {{.}}
{{end}}{{end}}
{{if .Alternatives}}
## Alternatives Considered

{{range .Alternatives}}- {{.}}
{{end}}{{end}}
`
