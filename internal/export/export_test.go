package export

import (
	"strings"
	"testing"

	"ckb/internal/logging"
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

func TestNewExporter(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	exporter := NewExporter("/path/to/repo", logger)

	if exporter == nil {
		t.Fatal("NewExporter returned nil")
	}
	if exporter.repoRoot != "/path/to/repo" {
		t.Errorf("repoRoot = %q, want %q", exporter.repoRoot, "/path/to/repo")
	}
}

func TestParseSymbolsGo(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	exporter := NewExporter("/tmp", logger)

	content := `package main

func main() {
	println("hello")
}

func PublicFunc() error {
	return nil
}

func privateFunc() {
}

type Server struct {
	port int
}

type Handler interface {
	Handle() error
}
`

	symbols, err := exporter.parseSymbols(content, ".go")
	if err != nil {
		t.Fatalf("parseSymbols() error = %v", err)
	}

	// Should find PublicFunc (main is not exported), Server, Handler
	if len(symbols) < 2 {
		t.Errorf("parseSymbols() found %d symbols, want >= 2", len(symbols))
	}

	// Check for specific symbols
	names := make(map[string]bool)
	for _, sym := range symbols {
		names[sym.Name] = true
	}

	if !names["PublicFunc"] {
		t.Error("Should find PublicFunc")
	}
	if !names["Server"] {
		t.Error("Should find Server")
	}
	if !names["Handler"] {
		t.Error("Should find Handler")
	}
	if names["main"] {
		t.Error("Should not include main (not exported)")
	}
	if names["privateFunc"] {
		t.Error("Should not include privateFunc (not exported)")
	}
}

func TestParseSymbolsTS(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	exporter := NewExporter("/tmp", logger)

	content := `
export function handleRequest(req: Request): Response {
	return new Response();
}

export class Server {
	start() {}
}

function privateHelper() {}
`

	symbols, err := exporter.parseSymbols(content, ".ts")
	if err != nil {
		t.Fatalf("parseSymbols() error = %v", err)
	}

	if len(symbols) < 2 {
		t.Errorf("parseSymbols() found %d symbols, want >= 2", len(symbols))
	}

	names := make(map[string]bool)
	for _, sym := range symbols {
		names[sym.Name] = true
	}

	if !names["handleRequest"] {
		t.Error("Should find handleRequest")
	}
	if !names["Server"] {
		t.Error("Should find Server")
	}
}

func TestParseSymbolsPy(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	exporter := NewExporter("/tmp", logger)

	content := `
def public_function():
    pass

def _private_function():
    pass

class MyClass:
    pass

class _PrivateClass:
    pass
`

	symbols, err := exporter.parseSymbols(content, ".py")
	if err != nil {
		t.Fatalf("parseSymbols() error = %v", err)
	}

	names := make(map[string]bool)
	for _, sym := range symbols {
		names[sym.Name] = true
	}

	if !names["public_function"] {
		t.Error("Should find public_function")
	}
	if !names["MyClass"] {
		t.Error("Should find MyClass")
	}
	if names["_private_function"] {
		t.Error("Should not find _private_function")
	}
	if names["_PrivateClass"] {
		t.Error("Should not find _PrivateClass")
	}
}

func TestFilterSymbols(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	exporter := NewExporter("/tmp", logger)

	symbols := []ExportSymbol{
		{Name: "LowComplexity", Complexity: 5, CallsPerDay: 100, IsExported: true},
		{Name: "HighComplexity", Complexity: 30, CallsPerDay: 100, IsExported: true},
		{Name: "HighCalls", Complexity: 10, CallsPerDay: 10000, IsExported: true},
		{Name: "Private", Complexity: 20, CallsPerDay: 5000, IsExported: false},
	}

	tests := []struct {
		name     string
		opts     ExportOptions
		wantLen  int
		wantName string
	}{
		{
			name:    "no filters",
			opts:    ExportOptions{},
			wantLen: 3, // excludes Private
		},
		{
			name:     "filter by complexity",
			opts:     ExportOptions{MinComplexity: 20},
			wantLen:  1,
			wantName: "HighComplexity",
		},
		{
			name:     "filter by calls",
			opts:     ExportOptions{MinCalls: 5000},
			wantLen:  1,
			wantName: "HighCalls",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filtered := exporter.filterSymbols(symbols, tt.opts)

			if len(filtered) != tt.wantLen {
				t.Errorf("filterSymbols() returned %d symbols, want %d", len(filtered), tt.wantLen)
			}

			if tt.wantName != "" && len(filtered) > 0 {
				if filtered[0].Name != tt.wantName {
					t.Errorf("Expected first symbol to be %q, got %q", tt.wantName, filtered[0].Name)
				}
			}
		})
	}
}

func TestFormatSymbolLine(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	exporter := NewExporter("/tmp", logger)

	tests := []struct {
		name     string
		sym      ExportSymbol
		opts     ExportOptions
		wantSubs []string
	}{
		{
			name: "function with complexity",
			sym:  ExportSymbol{Type: SymbolTypeFunction, Name: "MyFunc", Complexity: 15},
			opts: ExportOptions{IncludeComplexity: true},
			wantSubs: []string{"#", "MyFunc()", "c=15"},
		},
		{
			name: "class without complexity",
			sym:  ExportSymbol{Type: SymbolTypeClass, Name: "MyClass"},
			opts: ExportOptions{},
			wantSubs: []string{"$", "MyClass"},
		},
		{
			name: "with calls",
			sym:  ExportSymbol{Type: SymbolTypeFunction, Name: "Func", CallsPerDay: 5000},
			opts: ExportOptions{IncludeUsage: true},
			wantSubs: []string{"calls=5k/day"},
		},
		{
			name: "with importance",
			sym:  ExportSymbol{Type: SymbolTypeFunction, Name: "Func", Importance: 3},
			opts: ExportOptions{},
			wantSubs: []string{"★★★"},
		},
		{
			name: "with contracts",
			sym:  ExportSymbol{Type: SymbolTypeFunction, Name: "Func", Contracts: []string{"API"}},
			opts: ExportOptions{IncludeContracts: true},
			wantSubs: []string{"contract:API"},
		},
		{
			name: "with warnings",
			sym:  ExportSymbol{Type: SymbolTypeFunction, Name: "Func", Warnings: []string{"deprecated"}},
			opts: ExportOptions{},
			wantSubs: []string{"deprecated"},
		},
		{
			name: "interface marker",
			sym:  ExportSymbol{Type: SymbolTypeInterface, Name: "Handler", IsInterface: true},
			opts: ExportOptions{},
			wantSubs: []string{"interface"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			line := exporter.formatSymbolLine(tt.sym, tt.opts)

			for _, sub := range tt.wantSubs {
				if !strings.Contains(line, sub) {
					t.Errorf("formatSymbolLine() = %q, want to contain %q", line, sub)
				}
			}
		})
	}
}

func TestFormatText(t *testing.T) {
	logger := logging.NewLogger(logging.Config{Level: logging.InfoLevel})
	exporter := NewExporter("/tmp", logger)

	export := &LLMExport{
		Metadata: ExportMetadata{
			Repo:        "test-repo",
			Generated:   "2024-01-01T00:00:00Z",
			SymbolCount: 2,
			FileCount:   1,
			ModuleCount: 1,
		},
		Modules: []ExportModule{
			{
				Path:  "src",
				Owner: "team-a",
				Files: []ExportFile{
					{
						Name: "main.go",
						Symbols: []ExportSymbol{
							{Type: SymbolTypeClass, Name: "Server"},
							{Type: SymbolTypeFunction, Name: "Start"},
						},
					},
				},
			},
		},
	}

	opts := ExportOptions{
		IncludeComplexity: true,
		IncludeUsage:      true,
		IncludeContracts:  true,
	}

	output := exporter.FormatText(export, opts)

	// Check header
	if !strings.Contains(output, "Codebase: test-repo") {
		t.Error("Should contain codebase name")
	}

	// Check module
	if !strings.Contains(output, "src/") {
		t.Error("Should contain module path")
	}
	if !strings.Contains(output, "owner: team-a") {
		t.Error("Should contain owner")
	}

	// Check file
	if !strings.Contains(output, "main.go") {
		t.Error("Should contain filename")
	}

	// Check symbols
	if !strings.Contains(output, "Server") {
		t.Error("Should contain Server symbol")
	}
	if !strings.Contains(output, "Start") {
		t.Error("Should contain Start symbol")
	}

	// Check legend
	if !strings.Contains(output, "Legend:") {
		t.Error("Should contain legend")
	}
}

func TestExtractJSClassName(t *testing.T) {
	tests := []struct {
		line     string
		wantName string
	}{
		{"export class MyClass {", "MyClass"},
		{"export class Server extends Base {", "Server"},
		{"class Private {", "Private"},
		{"not a class", ""},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := extractJSClassName(tt.line)
			if got != tt.wantName {
				t.Errorf("extractJSClassName(%q) = %q, want %q", tt.line, got, tt.wantName)
			}
		})
	}
}

func TestExtractPyClassName(t *testing.T) {
	tests := []struct {
		line     string
		wantName string
	}{
		{"class MyClass:", "MyClass"},
		{"class Server(Base):", "Server"},
		{"class Handler:", "Handler"},
	}

	for _, tt := range tests {
		t.Run(tt.line, func(t *testing.T) {
			got := extractPyClassName(tt.line)
			if got != tt.wantName {
				t.Errorf("extractPyClassName(%q) = %q, want %q", tt.line, got, tt.wantName)
			}
		})
	}
}

func TestCalculateImportanceEdgeCases(t *testing.T) {
	tests := []struct {
		name       string
		calls      int
		complexity int
		wantLevel  ImportanceLevel
	}{
		{"zero values", 0, 0, ImportanceLow},
		{"just below medium calls", 99, 0, ImportanceLow},
		{"at medium calls", 100, 0, ImportanceLow},
		{"high calls only", 10000, 0, ImportanceMedium},
		{"high complexity only", 0, 30, ImportanceMedium}, // complexity >= 30 adds 2 points
		{"both high", 10000, 30, ImportanceHigh},
		{"max usage", 10000, 30, ImportanceHigh},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateImportance(tt.calls, tt.complexity)
			if got != tt.wantLevel {
				t.Errorf("CalculateImportance(%d, %d) = %d, want %d",
					tt.calls, tt.complexity, got, tt.wantLevel)
			}
		})
	}
}

func TestExportOptionsStructure(t *testing.T) {
	opts := ExportOptions{
		RepoRoot:          "/path/to/repo",
		Federation:        "my-fed",
		IncludeUsage:      true,
		IncludeOwnership:  true,
		IncludeContracts:  true,
		IncludeComplexity: true,
		MinComplexity:     10,
		MinCalls:          100,
		MaxSymbols:        1000,
		Format:            "json",
	}

	if opts.RepoRoot != "/path/to/repo" {
		t.Errorf("RepoRoot = %q, want /path/to/repo", opts.RepoRoot)
	}
	if opts.MinComplexity != 10 {
		t.Errorf("MinComplexity = %d, want 10", opts.MinComplexity)
	}
	if opts.MaxSymbols != 1000 {
		t.Errorf("MaxSymbols = %d, want 1000", opts.MaxSymbols)
	}
}

func TestImportanceLevelConstants(t *testing.T) {
	if ImportanceLow >= ImportanceMedium {
		t.Error("ImportanceLow should be less than ImportanceMedium")
	}
	if ImportanceMedium >= ImportanceHigh {
		t.Error("ImportanceMedium should be less than ImportanceHigh")
	}
}

func TestLLMExportStructure(t *testing.T) {
	export := LLMExport{
		Metadata: ExportMetadata{Repo: "test"},
		Modules: []ExportModule{
			{Path: "mod1", Files: []ExportFile{}},
		},
	}

	if export.Metadata.Repo != "test" {
		t.Errorf("Metadata.Repo = %q, want test", export.Metadata.Repo)
	}
	if len(export.Modules) != 1 {
		t.Errorf("len(Modules) = %d, want 1", len(export.Modules))
	}
}

func TestExportModuleStructure(t *testing.T) {
	mod := ExportModule{
		Path:  "internal/api",
		Owner: "api-team",
		Files: []ExportFile{
			{Name: "handler.go"},
		},
	}

	if mod.Path != "internal/api" {
		t.Errorf("Path = %q, want internal/api", mod.Path)
	}
	if mod.Owner != "api-team" {
		t.Errorf("Owner = %q, want api-team", mod.Owner)
	}
	if len(mod.Files) != 1 {
		t.Errorf("len(Files) = %d, want 1", len(mod.Files))
	}
}

func TestExportFileStructure(t *testing.T) {
	file := ExportFile{
		Name: "main.go",
		Symbols: []ExportSymbol{
			{Name: "Func1"},
			{Name: "Func2"},
		},
	}

	if file.Name != "main.go" {
		t.Errorf("Name = %q, want main.go", file.Name)
	}
	if len(file.Symbols) != 2 {
		t.Errorf("len(Symbols) = %d, want 2", len(file.Symbols))
	}
}
