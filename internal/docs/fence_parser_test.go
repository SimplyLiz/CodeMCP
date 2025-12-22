//go:build cgo

package docs

import (
	"testing"
)

func TestFenceParser_ExtractIdentifiers(t *testing.T) {
	parser := NewFenceParser()
	if parser == nil {
		t.Skip("FenceParser not available (CGO disabled)")
	}

	tests := []struct {
		name      string
		fence     Fence
		wantCount int
		wantNames []string
	}{
		{
			name: "Go selector expression",
			fence: Fence{
				Language: "go",
				Content: `func example() {
	result := pkg.DoSomething()
	obj.Method()
}`,
			},
			wantCount: 2,
			wantNames: []string{"pkg.DoSomething", "obj.Method"},
		},
		{
			name: "Go qualified type",
			fence: Fence{
				Language: "go",
				Content:  `var x pkg.Type`,
			},
			wantCount: 1,
			wantNames: []string{"pkg.Type"},
		},
		{
			name: "Python attribute access",
			fence: Fence{
				Language: "python",
				Content: `result = obj.method()
data = module.function()`,
			},
			wantCount: 2,
			wantNames: []string{"obj.method", "module.function"},
		},
		{
			name: "Unknown language",
			fence: Fence{
				Language: "unknown",
				Content:  `foo.bar`,
			},
			wantCount: 0,
		},
		{
			name: "Empty content",
			fence: Fence{
				Language: "go",
				Content:  ``,
			},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identifiers := parser.ExtractIdentifiers(tt.fence)

			if len(identifiers) != tt.wantCount {
				t.Errorf("got %d identifiers, want %d: %v", len(identifiers), tt.wantCount, identifiers)
			}

			if tt.wantNames != nil {
				for i, want := range tt.wantNames {
					if i >= len(identifiers) {
						t.Errorf("missing identifier %d: want %q", i, want)
						continue
					}
					if identifiers[i].Name != want {
						t.Errorf("identifier %d: got %q, want %q", i, identifiers[i].Name, want)
					}
				}
			}
		})
	}
}

func TestFenceLangToComplexity(t *testing.T) {
	tests := []struct {
		input string
		want  bool // true if should map to a valid language
	}{
		{"go", true},
		{"golang", true},
		{"python", true},
		{"py", true},
		{"javascript", true},
		{"js", true},
		{"typescript", true},
		{"ts", true},
		{"tsx", true},
		{"rust", true},
		{"rs", true},
		{"java", true},
		{"kotlin", true},
		{"kt", true},
		{"", false},
		{"unknown", false},
		{"bash", false},
		{"sql", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := fenceLangToComplexity(tt.input)
			if (result != "") != tt.want {
				t.Errorf("fenceLangToComplexity(%q) = %q, want valid=%v", tt.input, result, tt.want)
			}
		})
	}
}

func TestIsQualifiedName(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"Foo.Bar", true},
		{"pkg.Type.Method", true},
		{"crate::module", true},
		{"Class#method", true},
		{"SimpleIdent", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := isQualifiedName(tt.input)
			if got != tt.want {
				t.Errorf("isQualifiedName(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}
