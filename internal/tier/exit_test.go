package tier

import "testing"

func TestExitCodeString(t *testing.T) {
	tests := []struct {
		code ExitCode
		want string
	}{
		{ExitSuccess, "success"},
		{ExitError, "error"},
		{ExitDegraded, "degraded"},
		{ExitMissingTools, "missing tools"},
		{ExitTimeout, "timeout"},
		{ExitCode(99), "unknown"},
		{ExitCode(-1), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.code.String()
			if got != tt.want {
				t.Errorf("ExitCode(%d).String() = %q, want %q", tt.code, got, tt.want)
			}
		})
	}
}

func TestDetermineExitCode(t *testing.T) {
	tests := []struct {
		name          string
		result        ValidationResult
		allowFallback bool
		want          ExitCode
	}{
		{
			name:          "all satisfied",
			result:        ValidationResult{AllSatisfied: true},
			allowFallback: false,
			want:          ExitSuccess,
		},
		{
			name:          "not satisfied with errors, no fallback",
			result:        ValidationResult{AllSatisfied: false, Errors: []string{"missing tool"}},
			allowFallback: false,
			want:          ExitMissingTools,
		},
		{
			name:          "not satisfied with errors, fallback allowed",
			result:        ValidationResult{AllSatisfied: false, Errors: []string{"missing tool"}, Degraded: true},
			allowFallback: true,
			want:          ExitDegraded,
		},
		{
			name:          "degraded without errors",
			result:        ValidationResult{AllSatisfied: false, Degraded: true},
			allowFallback: false,
			want:          ExitDegraded,
		},
		{
			name:          "not satisfied, no errors, not degraded",
			result:        ValidationResult{AllSatisfied: false},
			allowFallback: false,
			want:          ExitSuccess,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetermineExitCode(tt.result, tt.allowFallback)
			if got != tt.want {
				t.Errorf("DetermineExitCode() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldContinue(t *testing.T) {
	tests := []struct {
		name          string
		result        ValidationResult
		allowFallback bool
		want          bool
	}{
		{
			name:          "all satisfied, continue",
			result:        ValidationResult{AllSatisfied: true},
			allowFallback: false,
			want:          true,
		},
		{
			name:          "fallback allowed, continue",
			result:        ValidationResult{AllSatisfied: false, Errors: []string{"error"}},
			allowFallback: true,
			want:          true,
		},
		{
			name:          "errors without fallback, don't continue",
			result:        ValidationResult{AllSatisfied: false, Errors: []string{"error"}},
			allowFallback: false,
			want:          false,
		},
		{
			name:          "no errors without fallback, continue",
			result:        ValidationResult{AllSatisfied: false, Errors: []string{}},
			allowFallback: false,
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ShouldContinue(tt.result, tt.allowFallback)
			if got != tt.want {
				t.Errorf("ShouldContinue() = %v, want %v", got, tt.want)
			}
		})
	}
}
