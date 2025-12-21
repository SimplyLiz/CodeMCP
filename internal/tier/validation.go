package tier

import (
	"context"
	"fmt"
	"sort"
)

// ValidationConfig configures the validation behavior.
type ValidationConfig struct {
	// RequestedTier is the global default tier to validate against.
	RequestedTier AnalysisTier

	// AllowFallback allows graceful degradation if requirements aren't met.
	AllowFallback bool

	// PerLanguageTier overrides the requested tier for specific languages.
	PerLanguageTier map[Language]AnalysisTier

	// CheckPrerequisites enables project prerequisite checks.
	CheckPrerequisites bool

	// WorkspaceRoot is the project root for prerequisite checks.
	WorkspaceRoot string
}

// ValidationResult contains the complete validation results.
type ValidationResult struct {
	// Languages maps each language to its status.
	Languages map[Language]LanguageValidation `json:"languages"`

	// AllSatisfied is true if all languages meet their requested tier.
	AllSatisfied bool `json:"allSatisfied"`

	// Degraded is true if any language is running at a lower tier.
	Degraded bool `json:"degraded"`

	// Errors contains validation error messages (when AllowFallback is false).
	Errors []string `json:"errors,omitempty"`

	// Warnings contains non-fatal warnings.
	Warnings []string `json:"warnings,omitempty"`
}

// LanguageValidation contains validation results for a single language.
type LanguageValidation struct {
	Language      Language              `json:"language"`
	DisplayName   string                `json:"displayName"`
	RequestedTier AnalysisTier          `json:"requestedTier"`
	ToolTier      AnalysisTier          `json:"toolTier"`
	RuntimeTier   AnalysisTier          `json:"runtimeTier"`
	Satisfied     bool                  `json:"satisfied"`
	Tools         []ToolStatus          `json:"tools"`
	Missing       []ToolStatus          `json:"missing,omitempty"`
	Prerequisites []PrerequisiteStatus  `json:"prerequisites,omitempty"`
	Capabilities  map[string]bool       `json:"capabilities"`
	CapabilityProviders map[string]string `json:"capabilityProviders,omitempty"`
}

// Validator validates tier requirements.
type Validator struct {
	detector *ToolDetector
	config   ValidationConfig
}

// NewValidator creates a new validator with the given configuration.
func NewValidator(detector *ToolDetector, config ValidationConfig) *Validator {
	return &Validator{
		detector: detector,
		config:   config,
	}
}

// Validate validates tier requirements for the given languages.
func (v *Validator) Validate(ctx context.Context, languages []Language) ValidationResult {
	result := ValidationResult{
		Languages:    make(map[Language]LanguageValidation),
		AllSatisfied: true,
	}

	// Check prerequisites if enabled
	var prereqChecker *PrerequisiteChecker
	if v.config.CheckPrerequisites && v.config.WorkspaceRoot != "" {
		prereqChecker = NewPrerequisiteChecker(v.config.WorkspaceRoot)
	}

	// Detect tool status for all languages concurrently
	toolStatuses := v.detector.DetectAllLanguages(ctx, languages)

	for _, lang := range languages {
		toolStatus := toolStatuses[lang]

		// Determine requested tier for this language
		requestedTier := v.config.RequestedTier
		if override, ok := v.config.PerLanguageTier[lang]; ok {
			requestedTier = override
		}

		validation := LanguageValidation{
			Language:           lang,
			DisplayName:        lang.DisplayName(),
			RequestedTier:      requestedTier,
			ToolTier:           toolStatus.ToolTier,
			RuntimeTier:        toolStatus.RuntimeTier,
			Tools:              toolStatus.Tools,
			Missing:            toolStatus.Missing,
			Capabilities:       toolStatus.Capabilities,
			CapabilityProviders: make(map[string]string),
		}

		// Check prerequisites
		if prereqChecker != nil {
			validation.Prerequisites = prereqChecker.CheckPrerequisites(lang)

			// Check for missing required prerequisites
			for _, prereq := range validation.Prerequisites {
				if prereq.Required && !prereq.Found {
					result.Warnings = append(result.Warnings,
						fmt.Sprintf("%s: missing %s", lang.DisplayName(), prereq.Name))
				}
			}
		}

		// Check if request is satisfied
		validation.Satisfied = validation.ToolTier >= requestedTier

		if !validation.Satisfied {
			result.AllSatisfied = false
			result.Degraded = true

			if !v.config.AllowFallback {
				result.Errors = append(result.Errors,
					fmt.Sprintf("%s: requested %s tier, got %s",
						lang.DisplayName(),
						tierDisplayName(requestedTier),
						tierDisplayName(validation.ToolTier)))
			}
		}

		// Track which provider enables each capability
		for _, tool := range validation.Tools {
			if tool.Found {
				for _, cap := range tool.Capabilities {
					if _, exists := validation.CapabilityProviders[cap]; !exists {
						validation.CapabilityProviders[cap] = tool.Name
					}
				}
			}
		}

		result.Languages[lang] = validation
	}

	return result
}

// ValidateSingle validates a single language.
func (v *Validator) ValidateSingle(ctx context.Context, lang Language) LanguageValidation {
	result := v.Validate(ctx, []Language{lang})
	return result.Languages[lang]
}

// tierDisplayName returns a display name for a tier.
func tierDisplayName(tier AnalysisTier) string {
	switch tier {
	case TierBasic:
		return "basic"
	case TierEnhanced:
		return "enhanced"
	case TierFull:
		return "full"
	default:
		return "unknown"
	}
}

// SortedLanguageValidations returns validations in deterministic order.
func SortedLanguageValidations(result ValidationResult) []LanguageValidation {
	languages := make([]Language, 0, len(result.Languages))
	for lang := range result.Languages {
		languages = append(languages, lang)
	}
	sort.Slice(languages, func(i, j int) bool {
		return string(languages[i]) < string(languages[j])
	})

	validations := make([]LanguageValidation, 0, len(languages))
	for _, lang := range languages {
		validations = append(validations, result.Languages[lang])
	}
	return validations
}

// GetEffectiveTier returns the effective tier for a language based on validation.
func GetEffectiveTier(validation LanguageValidation) AnalysisTier {
	// Use the minimum of tool tier and runtime tier
	if validation.RuntimeTier < validation.ToolTier {
		return validation.RuntimeTier
	}
	return validation.ToolTier
}

// CapabilitySummary returns a summary of available capabilities.
type CapabilitySummary struct {
	Available   []Capability
	Unavailable []Capability
	Providers   map[Capability]string
}

// GetCapabilitySummary returns a summary of capabilities for a validation.
func GetCapabilitySummary(validation LanguageValidation) CapabilitySummary {
	summary := CapabilitySummary{
		Providers: make(map[Capability]string),
	}

	for _, cap := range AllCapabilities() {
		if validation.Capabilities[string(cap)] {
			summary.Available = append(summary.Available, cap)
			if provider, ok := validation.CapabilityProviders[string(cap)]; ok {
				summary.Providers[cap] = provider
			}
		} else {
			summary.Unavailable = append(summary.Unavailable, cap)
		}
	}

	return summary
}
