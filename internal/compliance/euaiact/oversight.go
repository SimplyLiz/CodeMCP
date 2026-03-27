package euaiact

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- no-human-override: Art. 14 — No human intervention mechanism ---

type noHumanOverrideCheck struct{}

func (c *noHumanOverrideCheck) ID() string       { return "no-human-override" }
func (c *noHumanOverrideCheck) Name() string     { return "Missing Human Override" }
func (c *noHumanOverrideCheck) Article() string  { return "Art. 14 EU AI Act" }
func (c *noHumanOverrideCheck) Severity() string { return "error" }

func (c *noHumanOverrideCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	overridePatterns := []string{
		"human_review", "human_override", "manual_review",
		"human_in_the_loop", "hitl", "approval_required",
		"manual_approval", "human_decision", "escalate",
		"review_queue", "pending_review", "needs_approval",
	}

	hasOverride := false
	hasAICode := false

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lower := strings.ToLower(string(content))

		if hasAIContent(lower) {
			hasAICode = true
		}

		for _, p := range overridePatterns {
			if strings.Contains(lower, p) {
				hasOverride = true
				break
			}
		}

		if hasOverride {
			break
		}
	}

	var findings []compliance.Finding
	if hasAICode && !hasOverride {
		findings = append(findings, compliance.Finding{
			Severity:   "error",
			Article:    "Art. 14 EU AI Act",
			Message:    "No human oversight/override mechanism detected for AI system",
			Suggestion: "Implement human-in-the-loop: approval gates, override mechanisms, or escalation paths for AI decisions",
			Confidence: 0.60,
		})
	}

	return findings, nil
}

// --- no-kill-switch: Art. 14 — No shutdown mechanism ---

type noKillSwitchCheck struct{}

func (c *noKillSwitchCheck) ID() string       { return "no-kill-switch" }
func (c *noKillSwitchCheck) Name() string     { return "Missing Kill Switch" }
func (c *noKillSwitchCheck) Article() string  { return "Art. 14 EU AI Act" }
func (c *noKillSwitchCheck) Severity() string { return "error" }

func (c *noKillSwitchCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	killPatterns := []string{
		"kill_switch", "emergency_stop", "shutdown",
		"disable_model", "disable_ai", "feature_flag",
		"circuit_breaker", "fallback", "safe_mode",
		"model_enabled", "ai_enabled", "enable_model",
	}

	hasKillSwitch := false
	hasAICode := false

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lower := strings.ToLower(string(content))

		if hasAIContent(lower) {
			hasAICode = true
		}

		for _, p := range killPatterns {
			if strings.Contains(lower, p) {
				hasKillSwitch = true
				break
			}
		}

		if hasKillSwitch {
			break
		}
	}

	var findings []compliance.Finding
	if hasAICode && !hasKillSwitch {
		findings = append(findings, compliance.Finding{
			Severity:   "error",
			Article:    "Art. 14 EU AI Act",
			Message:    "No kill switch/disable mechanism detected for AI system",
			Suggestion: "Implement a feature flag, circuit breaker, or emergency shutdown mechanism for the AI system",
			Confidence: 0.60,
		})
	}

	return findings, nil
}

// --- missing-bias-testing: Art. 10 — No fairness evaluation ---

type missingBiasTestingCheck struct{}

func (c *missingBiasTestingCheck) ID() string       { return "missing-bias-testing" }
func (c *missingBiasTestingCheck) Name() string     { return "Missing Bias Testing" }
func (c *missingBiasTestingCheck) Article() string  { return "Art. 10 EU AI Act" }
func (c *missingBiasTestingCheck) Severity() string { return "warning" }

func (c *missingBiasTestingCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	biasPatterns := []string{
		"bias", "fairness", "fair_", "demographic_parity",
		"equalized_odds", "disparate_impact", "discrimination",
		"protected_attribute", "sensitive_attribute",
		"aif360", "fairlearn", "what_if_tool",
	}

	hasBiasTesting := false
	hasAICode := false

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lower := strings.ToLower(string(content))

		if hasAIContent(lower) {
			hasAICode = true
		}

		for _, p := range biasPatterns {
			if strings.Contains(lower, p) {
				hasBiasTesting = true
				break
			}
		}

		if hasBiasTesting {
			break
		}
	}

	var findings []compliance.Finding
	if hasAICode && !hasBiasTesting {
		findings = append(findings, compliance.Finding{
			Severity:   "warning",
			Article:    "Art. 10 EU AI Act",
			Message:    "No bias detection or fairness evaluation detected for AI system",
			Suggestion: "Implement bias testing: measure demographic parity, equalized odds, or disparate impact on protected attributes",
			Confidence: 0.55,
		})
	}

	return findings, nil
}

// --- no-data-provenance: Art. 10 — Training data without lineage ---

type noDataProvenanceCheck struct{}

func (c *noDataProvenanceCheck) ID() string       { return "no-data-provenance" }
func (c *noDataProvenanceCheck) Name() string     { return "Missing Data Provenance" }
func (c *noDataProvenanceCheck) Article() string  { return "Art. 10 EU AI Act" }
func (c *noDataProvenanceCheck) Severity() string { return "warning" }

func (c *noDataProvenanceCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	provenancePatterns := []string{
		"provenance", "lineage", "data_source", "dataset_version",
		"data_card", "model_card", "data_sheet",
		"training_data", "data_manifest", "data_catalog",
	}

	hasProvenance := false
	hasTrainingCode := false

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lower := strings.ToLower(string(content))

		if strings.Contains(lower, "train") && hasAIContent(lower) {
			hasTrainingCode = true
		}

		for _, p := range provenancePatterns {
			if strings.Contains(lower, p) {
				hasProvenance = true
				break
			}
		}

		if hasProvenance {
			break
		}
	}

	var findings []compliance.Finding
	if hasTrainingCode && !hasProvenance {
		findings = append(findings, compliance.Finding{
			Severity:   "warning",
			Article:    "Art. 10 EU AI Act",
			Message:    "No data provenance/lineage tracking detected for training pipeline",
			Suggestion: "Track training data sources, versions, and transformations for data governance compliance",
			Confidence: 0.55,
		})
	}

	return findings, nil
}

// --- missing-version-tracking: Art. 12 — Model without version ---

type missingVersionTrackingCheck struct{}

func (c *missingVersionTrackingCheck) ID() string       { return "missing-version-tracking" }
func (c *missingVersionTrackingCheck) Name() string     { return "Missing Model Version Tracking" }
func (c *missingVersionTrackingCheck) Article() string  { return "Art. 12 EU AI Act" }
func (c *missingVersionTrackingCheck) Severity() string { return "warning" }

func (c *missingVersionTrackingCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	versionPatterns := []string{
		"model_version", "model_id", "model_name",
		"model_registry", "mlflow", "wandb",
		"model_checkpoint", "model_hash", "model_sha",
	}

	hasVersioning := false
	hasAICode := false

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			break
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		lower := strings.ToLower(string(content))

		if hasAIContent(lower) {
			hasAICode = true
		}

		for _, p := range versionPatterns {
			if strings.Contains(lower, p) {
				hasVersioning = true
				break
			}
		}

		if hasVersioning {
			break
		}
	}

	var findings []compliance.Finding
	if hasAICode && !hasVersioning {
		findings = append(findings, compliance.Finding{
			Severity:   "warning",
			Article:    "Art. 12 EU AI Act",
			Message:    "No model version tracking detected for AI system",
			Suggestion: "Include model version/ID in all predictions and responses for traceability",
			Confidence: 0.55,
		})
	}

	return findings, nil
}
