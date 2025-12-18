package jobs

import (
	"encoding/json"
)

// RefreshScope defines what to refresh during a refresh_architecture job.
type RefreshScope struct {
	Scope string `json:"scope"` // "all", "modules", "ownership", "hotspots", "responsibilities"
	Force bool   `json:"force"`
}

// ParseRefreshScope parses the scope JSON from a job.
func ParseRefreshScope(scopeJSON string) (*RefreshScope, error) {
	if scopeJSON == "" {
		return &RefreshScope{Scope: "all"}, nil
	}

	var scope RefreshScope
	if err := json.Unmarshal([]byte(scopeJSON), &scope); err != nil {
		return nil, err
	}

	if scope.Scope == "" {
		scope.Scope = "all"
	}

	return &scope, nil
}

// RefreshResult contains the result of a refresh_architecture job.
type RefreshResult struct {
	Status           string                 `json:"status"` // "completed", "partial"
	ModulesDetected  int                    `json:"modulesDetected,omitempty"`
	ModulesChanged   int                    `json:"modulesChanged,omitempty"`
	OwnershipUpdated int                    `json:"ownershipUpdated,omitempty"`
	HotspotsUpdated  int                    `json:"hotspotsUpdated,omitempty"`
	Duration         string                 `json:"duration"`
	Warnings         []string               `json:"warnings,omitempty"`
	Details          map[string]interface{} `json:"details,omitempty"`
}

// AnalyzeImpactScope defines the scope for impact analysis jobs.
type AnalyzeImpactScope struct {
	SymbolID string `json:"symbolId"`
	Depth    int    `json:"depth"`
}

// ParseAnalyzeImpactScope parses the scope JSON for impact analysis.
func ParseAnalyzeImpactScope(scopeJSON string) (*AnalyzeImpactScope, error) {
	if scopeJSON == "" {
		return nil, nil
	}

	var scope AnalyzeImpactScope
	if err := json.Unmarshal([]byte(scopeJSON), &scope); err != nil {
		return nil, err
	}

	if scope.Depth == 0 {
		scope.Depth = 2
	}

	return &scope, nil
}

// ExportScope defines the scope for export jobs.
type ExportScope struct {
	Format string `json:"format"` // "json", "markdown", "html"
	Target string `json:"target"` // Output path
}

// ParseExportScope parses the scope JSON for export jobs.
func ParseExportScope(scopeJSON string) (*ExportScope, error) {
	if scopeJSON == "" {
		return &ExportScope{Format: "json"}, nil
	}

	var scope ExportScope
	if err := json.Unmarshal([]byte(scopeJSON), &scope); err != nil {
		return nil, err
	}

	if scope.Format == "" {
		scope.Format = "json"
	}

	return &scope, nil
}

// FederationSyncScope defines the scope for federation sync jobs.
type FederationSyncScope struct {
	FederationName string `json:"federationName"`
	Direction      string `json:"direction"` // "pull" or "push"
	RepoID         string `json:"repoId,omitempty"`
}

// ParseFederationSyncScope parses the scope JSON for federation sync jobs.
func ParseFederationSyncScope(scopeJSON string) (*FederationSyncScope, error) {
	if scopeJSON == "" {
		return nil, nil
	}

	var scope FederationSyncScope
	if err := json.Unmarshal([]byte(scopeJSON), &scope); err != nil {
		return nil, err
	}

	if scope.Direction == "" {
		scope.Direction = "pull"
	}

	return &scope, nil
}

// WebhookDispatchScope defines the scope for webhook dispatch jobs.
type WebhookDispatchScope struct {
	WebhookID string `json:"webhookId"`
	EventID   string `json:"eventId"`
	EventType string `json:"eventType"`
	Payload   string `json:"payload"` // JSON-encoded payload
}

// ParseWebhookDispatchScope parses the scope JSON for webhook dispatch jobs.
func ParseWebhookDispatchScope(scopeJSON string) (*WebhookDispatchScope, error) {
	if scopeJSON == "" {
		return nil, nil
	}

	var scope WebhookDispatchScope
	if err := json.Unmarshal([]byte(scopeJSON), &scope); err != nil {
		return nil, err
	}

	return &scope, nil
}

// ScheduledTaskScope defines the scope for scheduled task jobs.
type ScheduledTaskScope struct {
	ScheduleID string `json:"scheduleId"`
	TaskType   string `json:"taskType"`
	Target     string `json:"target,omitempty"`
}

// ParseScheduledTaskScope parses the scope JSON for scheduled task jobs.
func ParseScheduledTaskScope(scopeJSON string) (*ScheduledTaskScope, error) {
	if scopeJSON == "" {
		return nil, nil
	}

	var scope ScheduledTaskScope
	if err := json.Unmarshal([]byte(scopeJSON), &scope); err != nil {
		return nil, err
	}

	return &scope, nil
}
