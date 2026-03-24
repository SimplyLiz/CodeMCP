package euaiact

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// aiIndicators suggest a file is part of an AI/ML component.
var aiIndicators = []string{
	"model", "predict", "inference", "neural", "tensor",
	"sklearn", "pytorch", "tensorflow", "keras", "torch",
	"classifier", "regressor", "embedding", "transformer",
	"openai", "anthropic", "llm", "gpt", "claude",
	"huggingface", "diffusion", "training", "epoch",
	"ml_", "ai_", "deep_learning",
}

func isAIFile(file string, scope *compliance.ScanScope) bool {
	lower := strings.ToLower(file)

	// Check configured AI component paths
	for _, p := range scope.Config.AIComponentPaths {
		if strings.HasPrefix(lower, strings.ToLower(p)) {
			return true
		}
	}

	// Check filename indicators
	for _, ind := range aiIndicators {
		if strings.Contains(lower, ind) {
			return true
		}
	}

	return false
}

func hasAIContent(content string) bool {
	lower := strings.ToLower(content)
	matches := 0
	for _, ind := range aiIndicators {
		if strings.Contains(lower, ind) {
			matches++
		}
		if matches >= 2 {
			return true
		}
	}
	return false
}

// --- missing-model-logging: Art. 12 — ML inference without I/O logging ---

type missingModelLoggingCheck struct{}

func (c *missingModelLoggingCheck) ID() string       { return "missing-model-logging" }
func (c *missingModelLoggingCheck) Name() string     { return "Missing Model I/O Logging" }
func (c *missingModelLoggingCheck) Article() string   { return "Art. 12 EU AI Act" }
func (c *missingModelLoggingCheck) Severity() string  { return "error" }

func (c *missingModelLoggingCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if !isAIFile(file, scope) {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		text := string(content)
		if !hasAIContent(text) {
			continue
		}

		lower := strings.ToLower(text)

		// Check for prediction/inference calls
		hasPrediction := strings.Contains(lower, "predict") || strings.Contains(lower, "inference") ||
			strings.Contains(lower, "generate") || strings.Contains(lower, "completion") ||
			strings.Contains(lower, "forward(")

		if !hasPrediction {
			continue
		}

		// Check for logging of inputs/outputs
		hasIOLogging := strings.Contains(lower, "log_input") || strings.Contains(lower, "log_output") ||
			strings.Contains(lower, "log_prediction") || strings.Contains(lower, "audit_log") ||
			strings.Contains(lower, "log_inference") || strings.Contains(lower, "record_prediction") ||
			(strings.Contains(lower, "log") && strings.Contains(lower, "input") && strings.Contains(lower, "output"))

		if !hasIOLogging {
			findings = append(findings, compliance.Finding{
				Severity:   "error",
				Article:    "Art. 12 EU AI Act",
				File:       file,
				Message:    "AI model inference/prediction without structured input/output logging",
				Suggestion: "Log all model inputs, outputs, and metadata (model version, timestamp) for audit trail compliance",
				Confidence: 0.70,
			})
		}
	}

	return findings, nil
}

// --- no-audit-trail: Art. 12, 19 — Predictions without immutable records ---

type noAuditTrailCheck struct{}

func (c *noAuditTrailCheck) ID() string       { return "no-audit-trail" }
func (c *noAuditTrailCheck) Name() string     { return "Missing AI Audit Trail" }
func (c *noAuditTrailCheck) Article() string   { return "Art. 12, 19 EU AI Act" }
func (c *noAuditTrailCheck) Severity() string  { return "error" }

func (c *noAuditTrailCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	auditPatterns := []string{
		"audit_trail", "audit_log", "prediction_log",
		"inference_log", "model_log", "decision_log",
		"immutable_log", "event_store", "event_log",
	}

	hasAudit := false
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

		for _, p := range auditPatterns {
			if strings.Contains(lower, p) {
				hasAudit = true
				break
			}
		}

		if hasAudit {
			break
		}
	}

	var findings []compliance.Finding
	if hasAICode && !hasAudit {
		findings = append(findings, compliance.Finding{
			Severity:   "error",
			Article:    "Art. 12, 19 EU AI Act",
			Message:    "No audit trail/immutable logging detected for AI system decisions",
			Suggestion: "Implement immutable audit logging for all AI predictions with minimum 6-month retention (Art. 19)",
			Confidence: 0.60,
		})
	}

	return findings, nil
}

// --- missing-confidence-score: Art. 13 — Outputs without confidence ---

type missingConfidenceScoreCheck struct{}

func (c *missingConfidenceScoreCheck) ID() string       { return "missing-confidence-score" }
func (c *missingConfidenceScoreCheck) Name() string     { return "Missing Confidence Scores" }
func (c *missingConfidenceScoreCheck) Article() string   { return "Art. 13 EU AI Act" }
func (c *missingConfidenceScoreCheck) Severity() string  { return "warning" }

func (c *missingConfidenceScoreCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if !isAIFile(file, scope) {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		text := string(content)
		lower := strings.ToLower(text)

		hasPrediction := strings.Contains(lower, "predict") || strings.Contains(lower, "classify") ||
			strings.Contains(lower, "inference")

		if !hasPrediction {
			continue
		}

		hasConfidence := strings.Contains(lower, "confidence") || strings.Contains(lower, "probability") ||
			strings.Contains(lower, "score") || strings.Contains(lower, "certainty") ||
			strings.Contains(lower, "logits") || strings.Contains(lower, "softmax")

		if !hasConfidence {
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "Art. 13 EU AI Act",
				File:       file,
				Message:    "AI prediction without confidence/probability score in output",
				Suggestion: "Include confidence scores with model outputs for transparency",
				Confidence: 0.60,
			})
		}
	}

	return findings, nil
}
