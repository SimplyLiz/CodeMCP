package tier

import "testing"

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"shorter than max", "abc", 6, "abc   "},
		{"equal to max", "abcdef", 6, "abcdef"},
		{"longer than max", "abcdefghi", 6, "abcdef"},
		{"empty string", "", 6, "      "},
		{"single char", "a", 3, "a  "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncate(tt.s, tt.maxLen)
			if got != tt.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tt.s, tt.maxLen, got, tt.want)
			}
		})
	}
}

func TestCapitalizeFirst(t *testing.T) {
	tests := []struct {
		name string
		s    string
		want string
	}{
		{"lowercase word", "hello", "Hello"},
		{"already capitalized", "Hello", "Hello"},
		{"uppercase word", "HELLO", "HELLO"},
		{"empty string", "", ""},
		{"single char", "a", "A"},
		{"number prefix", "123abc", "123abc"},
		{"unicode", "über", "Über"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capitalizeFirst(tt.s)
			if got != tt.want {
				t.Errorf("capitalizeFirst(%q) = %q, want %q", tt.s, got, tt.want)
			}
		})
	}
}
