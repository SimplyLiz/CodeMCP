package tier

// ExitCode represents a CLI exit code.
type ExitCode int

const (
	// ExitSuccess indicates the requested tier was satisfied.
	ExitSuccess ExitCode = 0

	// ExitError indicates a general error.
	ExitError ExitCode = 1

	// ExitDegraded indicates operation completed with a degraded tier.
	// This is NOT an error - it's informational for CI to detect.
	ExitDegraded ExitCode = 2

	// ExitMissingTools indicates requirements are missing for the requested tier.
	ExitMissingTools ExitCode = 3

	// ExitTimeout indicates an LSP timeout or runtime failure.
	ExitTimeout ExitCode = 4
)

// String returns a description of the exit code.
func (e ExitCode) String() string {
	switch e {
	case ExitSuccess:
		return "success"
	case ExitError:
		return "error"
	case ExitDegraded:
		return "degraded"
	case ExitMissingTools:
		return "missing tools"
	case ExitTimeout:
		return "timeout"
	default:
		return "unknown"
	}
}

// ExitResult represents an exit condition that's not an error.
// Use this for ExitDegraded to avoid Cobra printing error messages.
type ExitResult struct {
	Code    ExitCode
	Message string
}

// DetermineExitCode determines the appropriate exit code from validation results.
func DetermineExitCode(result ValidationResult, allowFallback bool) ExitCode {
	if result.AllSatisfied {
		return ExitSuccess
	}

	if !allowFallback && len(result.Errors) > 0 {
		return ExitMissingTools
	}

	if result.Degraded {
		return ExitDegraded
	}

	return ExitSuccess
}

// ShouldContinue returns true if operations should proceed based on validation.
func ShouldContinue(result ValidationResult, allowFallback bool) bool {
	// Always continue if all satisfied
	if result.AllSatisfied {
		return true
	}

	// Continue if fallback is allowed
	if allowFallback {
		return true
	}

	// Don't continue if there are errors and fallback not allowed
	return len(result.Errors) == 0
}
