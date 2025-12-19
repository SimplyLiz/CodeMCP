package export

import (
	"testing"
)

func TestSymbolTypeConstants(t *testing.T) {
	tests := []struct {
		symbolType string
	}{
		{SymbolTypeFunction},
		{SymbolTypeMethod},
		{SymbolTypeClass},
		{SymbolTypeInterface},
		{SymbolTypeConstant},
		{SymbolTypeVariable},
	}

	for _, tt := range tests {
		t.Run(tt.symbolType, func(t *testing.T) {
			if tt.symbolType == "" {
				t.Error("Symbol type should not be empty")
			}
		})
	}
}

func TestIsSourceFile(t *testing.T) {
	tests := []struct {
		ext      string
		wantTrue bool
	}{
		{".go", true},
		{".ts", true},
		{".tsx", true},
		{".js", true},
		{".jsx", true},
		{".py", true},
		{".java", true},
		{".kt", true},
		{".rs", true},
		{".rb", true},
		{".c", true},
		{".cpp", true},
		{".h", true},
		{".hpp", true},
		{".txt", false},
		{".md", false},
		{".json", false},
		{".yaml", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.ext, func(t *testing.T) {
			got := isSourceFile(tt.ext)
			if got != tt.wantTrue {
				t.Errorf("isSourceFile(%q) = %v, want %v", tt.ext, got, tt.wantTrue)
			}
		})
	}
}

func TestIsExportedGo(t *testing.T) {
	tests := []struct {
		name     string
		wantTrue bool
	}{
		{"Exported", true},
		{"AnotherExported", true},
		{"unexported", false},
		{"anotherUnexported", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isExportedGo(tt.name)
			if got != tt.wantTrue {
				t.Errorf("isExportedGo(%q) = %v, want %v", tt.name, got, tt.wantTrue)
			}
		})
	}
}

func TestExtractGoFuncName(t *testing.T) {
	tests := []struct {
		line     string
		wantName string
	}{
		{"func Hello() {", "Hello"},
		{"func hello() {", "hello"},
		{"func (s *Server) Start() {", "Start"},
		{"func (r *Receiver) handle(ctx context.Context) error {", "handle"},
		{"func main() {", "main"},
		{"not a function", ""},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := extractGoFuncName(tt.line)
			if got != tt.wantName {
				t.Errorf("extractGoFuncName(%q) = %q, want %q", tt.line, got, tt.wantName)
			}
		})
	}
}

func TestExtractGoTypeName(t *testing.T) {
	tests := []struct {
		line          string
		wantName      string
		wantInterface bool
	}{
		{"type Server struct {", "Server", false},
		{"type Handler interface {", "Handler", true},
		{"type Config struct {", "Config", false},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			gotName, gotInterface := extractGoTypeName(tt.line)
			if gotName != tt.wantName {
				t.Errorf("extractGoTypeName(%q) name = %q, want %q", tt.line, gotName, tt.wantName)
			}
			if gotInterface != tt.wantInterface {
				t.Errorf("extractGoTypeName(%q) interface = %v, want %v", tt.line, gotInterface, tt.wantInterface)
			}
		})
	}
}

func TestExtractJSFuncName(t *testing.T) {
	tests := []struct {
		line     string
		wantName string
	}{
		{"export function handleRequest() {", "handleRequest"},
		{"function helper() {", "helper"},
		{"export async function fetchData() {", "fetchData"},
		{"not a function", ""},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := extractJSFuncName(tt.line)
			if got != tt.wantName {
				t.Errorf("extractJSFuncName(%q) = %q, want %q", tt.line, got, tt.wantName)
			}
		})
	}
}

func TestExtractPyFuncName(t *testing.T) {
	tests := []struct {
		line     string
		wantName string
	}{
		{"def hello():", "hello"},
		{"def process_data(data):", "process_data"},
		{"def _private():", "_private"},
		{"not a function", ""},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := extractPyFuncName(tt.line)
			if got != tt.wantName {
				t.Errorf("extractPyFuncName(%q) = %q, want %q", tt.line, got, tt.wantName)
			}
		})
	}
}

func TestFormatCalls(t *testing.T) {
	tests := []struct {
		calls   int
		wantStr string
	}{
		{100, "100/day"},
		{1000, "1k/day"},
		{5000, "5k/day"},
		{1000000, "1M/day"},
		{5000000, "5M/day"},
	}

	for _, tt := range tests {
		t.Run(tt.wantStr, func(t *testing.T) {
			got := formatCalls(tt.calls)
			if got != tt.wantStr {
				t.Errorf("formatCalls(%d) = %q, want %q", tt.calls, got, tt.wantStr)
			}
		})
	}
}

func TestCalculateImportance(t *testing.T) {
	// Test that CalculateImportance returns valid importance levels
	tests := []struct {
		name       string
		calls      int
		complexity int
	}{
		{"low usage, low complexity", 10, 5},
		{"medium usage, low complexity", 1000, 5},
		{"high usage, low complexity", 10000, 5},
		{"low usage, high complexity", 10, 30},
		{"high usage, high complexity", 10000, 30},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateImportance(tt.calls, tt.complexity)
			// Verify the result is a valid ImportanceLevel
			if got < ImportanceLow || got > ImportanceHigh {
				t.Errorf("CalculateImportance(%d, %d) = %d, should be between %d and %d",
					tt.calls, tt.complexity, got, ImportanceLow, ImportanceHigh)
			}
		})
	}
}

func TestExportSymbolStructure(t *testing.T) {
	sym := ExportSymbol{
		Type:        SymbolTypeFunction,
		Name:        "TestFunc",
		Complexity:  15,
		CallsPerDay: 1000,
		Importance:  2,
		IsExported:  true,
	}

	if sym.Type != SymbolTypeFunction {
		t.Errorf("ExportSymbol.Type = %q, want %q", sym.Type, SymbolTypeFunction)
	}
	if sym.Name != "TestFunc" {
		t.Errorf("ExportSymbol.Name = %q, want %q", sym.Name, "TestFunc")
	}
	if sym.Complexity != 15 {
		t.Errorf("ExportSymbol.Complexity = %d, want %d", sym.Complexity, 15)
	}
	if sym.CallsPerDay != 1000 {
		t.Errorf("ExportSymbol.CallsPerDay = %d, want %d", sym.CallsPerDay, 1000)
	}
	if sym.Importance != 2 {
		t.Errorf("ExportSymbol.Importance = %d, want %d", sym.Importance, 2)
	}
	if sym.IsExported != true {
		t.Error("ExportSymbol.IsExported should be true")
	}
}

func TestExportMetadataStructure(t *testing.T) {
	meta := ExportMetadata{
		Repo:        "test-repo",
		Generated:   "2024-01-01T00:00:00Z",
		SymbolCount: 100,
		FileCount:   20,
		ModuleCount: 5,
	}

	if meta.Repo != "test-repo" {
		t.Errorf("ExportMetadata.Repo = %q, want %q", meta.Repo, "test-repo")
	}
	if meta.Generated != "2024-01-01T00:00:00Z" {
		t.Errorf("ExportMetadata.Generated = %q, want %q", meta.Generated, "2024-01-01T00:00:00Z")
	}
	if meta.SymbolCount != 100 {
		t.Errorf("ExportMetadata.SymbolCount = %d, want %d", meta.SymbolCount, 100)
	}
	if meta.FileCount != 20 {
		t.Errorf("ExportMetadata.FileCount = %d, want %d", meta.FileCount, 20)
	}
	if meta.ModuleCount != 5 {
		t.Errorf("ExportMetadata.ModuleCount = %d, want %d", meta.ModuleCount, 5)
	}
}
