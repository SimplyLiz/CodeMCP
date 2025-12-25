package output

import "testing"

func TestGetImpactKindPriority(t *testing.T) {
	tests := []struct {
		kind string
		want int
	}{
		{"direct-caller", 1},
		{"transitive-caller", 2},
		{"type-dependency", 3},
		{"test-dependency", 4},
		{"unknown", 5},
		// Unknown kinds should get the lowest priority
		{"something-else", 5},
		{"", 5},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			got := GetImpactKindPriority(tt.kind)
			if got != tt.want {
				t.Errorf("GetImpactKindPriority(%q) = %d, want %d", tt.kind, got, tt.want)
			}
		})
	}
}

func TestGetWarningSeverity(t *testing.T) {
	tests := []struct {
		severity string
		want     int
	}{
		{"error", 1},
		{"warning", 2},
		{"info", 3},
		// Unknown severities should get info level (lowest priority)
		{"debug", 3},
		{"trace", 3},
		{"", 3},
	}

	for _, tt := range tests {
		t.Run(tt.severity, func(t *testing.T) {
			got := GetWarningSeverity(tt.severity)
			if got != tt.want {
				t.Errorf("GetWarningSeverity(%q) = %d, want %d", tt.severity, got, tt.want)
			}
		})
	}
}

func TestImpactKindPriorityOrdering(t *testing.T) {
	// Verify that priorities are in expected order
	priorities := []string{"direct-caller", "transitive-caller", "type-dependency", "test-dependency", "unknown"}
	for i := 0; i < len(priorities)-1; i++ {
		current := GetImpactKindPriority(priorities[i])
		next := GetImpactKindPriority(priorities[i+1])
		if current >= next {
			t.Errorf("Priority of %q (%d) should be less than %q (%d)",
				priorities[i], current, priorities[i+1], next)
		}
	}
}

func TestWarningSeverityOrdering(t *testing.T) {
	// Verify that severities are in expected order
	severities := []string{"error", "warning", "info"}
	for i := 0; i < len(severities)-1; i++ {
		current := GetWarningSeverity(severities[i])
		next := GetWarningSeverity(severities[i+1])
		if current >= next {
			t.Errorf("Severity of %q (%d) should be less than %q (%d)",
				severities[i], current, severities[i+1], next)
		}
	}
}
