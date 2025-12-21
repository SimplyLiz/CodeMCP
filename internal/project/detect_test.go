package project

import (
	"os"
	"path/filepath"
	"testing"
)

// Helper to create a temp directory with files
func setupTestDir(t *testing.T, files []string) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range files {
		path := filepath.Join(dir, f)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatalf("failed to create dir for %s: %v", f, err)
		}
		if err := os.WriteFile(path, []byte(""), 0644); err != nil {
			t.Fatalf("failed to create file %s: %v", f, err)
		}
	}
	return dir
}

func TestDetectLanguage(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		wantLang Language
		wantOk   bool
	}{
		{
			name:     "Go project",
			files:    []string{"go.mod", "main.go"},
			wantLang: LangGo,
			wantOk:   true,
		},
		{
			name:     "TypeScript project",
			files:    []string{"package.json", "tsconfig.json", "src/index.ts"},
			wantLang: LangTypeScript,
			wantOk:   true,
		},
		{
			name:     "JavaScript project (no tsconfig)",
			files:    []string{"package.json", "src/index.js"},
			wantLang: LangJavaScript,
			wantOk:   true,
		},
		{
			name:     "Python project with pyproject.toml",
			files:    []string{"pyproject.toml", "src/main.py"},
			wantLang: LangPython,
			wantOk:   true,
		},
		{
			name:     "Python project with requirements.txt",
			files:    []string{"requirements.txt", "app.py"},
			wantLang: LangPython,
			wantOk:   true,
		},
		{
			name:     "Rust project",
			files:    []string{"Cargo.toml", "src/main.rs"},
			wantLang: LangRust,
			wantOk:   true,
		},
		{
			name:     "Java Maven project",
			files:    []string{"pom.xml", "src/main/java/App.java"},
			wantLang: LangJava,
			wantOk:   true,
		},
		{
			name:     "Java Gradle project",
			files:    []string{"build.gradle", "src/main/java/App.java"},
			wantLang: LangJava,
			wantOk:   true,
		},
		{
			name:     "Kotlin project",
			files:    []string{"build.gradle.kts", "src/main/kotlin/App.kt"},
			wantLang: LangKotlin,
			wantOk:   true,
		},
		{
			name:     "Dart project",
			files:    []string{"pubspec.yaml", "lib/main.dart"},
			wantLang: LangDart,
			wantOk:   true,
		},
		{
			name:     "PHP project",
			files:    []string{"composer.json", "src/index.php"},
			wantLang: LangPHP,
			wantOk:   true,
		},
		{
			name:     "Ruby project with Gemfile",
			files:    []string{"Gemfile", "lib/app.rb"},
			wantLang: LangRuby,
			wantOk:   true,
		},
		{
			name:     "C++ project with compile_commands.json",
			files:    []string{"compile_commands.json", "src/main.cpp"},
			wantLang: LangCpp,
			wantOk:   true,
		},
		{
			name:     "C++ project with compile_commands in build/",
			files:    []string{"build/compile_commands.json", "src/main.cpp"},
			wantLang: LangCpp,
			wantOk:   true,
		},
		{
			name:     "Unknown project",
			files:    []string{"README.md", "random.txt"},
			wantLang: LangUnknown,
			wantOk:   false,
		},
		{
			name:     "Empty directory",
			files:    []string{},
			wantLang: LangUnknown,
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupTestDir(t, tt.files)
			lang, _, ok := DetectLanguage(dir)
			if lang != tt.wantLang {
				t.Errorf("DetectLanguage() lang = %v, want %v", lang, tt.wantLang)
			}
			if ok != tt.wantOk {
				t.Errorf("DetectLanguage() ok = %v, want %v", ok, tt.wantOk)
			}
		})
	}
}

func TestDetectAllLanguages_MultiLanguage(t *testing.T) {
	tests := []struct {
		name         string
		files        []string
		wantPrimary  Language
		wantAllCount int
	}{
		{
			name:         "Go + TypeScript monorepo",
			files:        []string{"go.mod", "package.json", "tsconfig.json"},
			wantPrimary:  LangGo, // Go has higher priority
			wantAllCount: 2,
		},
		{
			name:         "Python + Ruby",
			files:        []string{"requirements.txt", "Gemfile"},
			wantPrimary:  LangPython, // Python has higher priority
			wantAllCount: 2,
		},
		{
			name:         "Java + Kotlin (same build system)",
			files:        []string{"build.gradle", "build.gradle.kts"},
			wantPrimary:  LangJava, // Java comes first in manifests
			wantAllCount: 2,
		},
		{
			name:         "Single language",
			files:        []string{"go.mod"},
			wantPrimary:  LangGo,
			wantAllCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupTestDir(t, tt.files)
			primary, _, allLangs := DetectAllLanguages(dir)
			if primary != tt.wantPrimary {
				t.Errorf("DetectAllLanguages() primary = %v, want %v", primary, tt.wantPrimary)
			}
			if len(allLangs) != tt.wantAllCount {
				t.Errorf("DetectAllLanguages() allLangs count = %d, want %d (got %v)", len(allLangs), tt.wantAllCount, allLangs)
			}
		})
	}
}

func TestDetectLanguage_GlobPatterns(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		wantLang Language
	}{
		{
			name:     "C# with .csproj at root",
			files:    []string{"MyApp.csproj", "Program.cs"},
			wantLang: LangCSharp,
		},
		{
			name:     "C# with .csproj in subdirectory",
			files:    []string{"src/MyApp/MyApp.csproj", "src/MyApp/Program.cs"},
			wantLang: LangCSharp,
		},
		{
			name:     "C# with .sln file",
			files:    []string{"MySolution.sln", "src/MyApp/MyApp.csproj"},
			wantLang: LangCSharp,
		},
		{
			name:     "Ruby with .gemspec",
			files:    []string{"mylib.gemspec", "lib/mylib.rb"},
			wantLang: LangRuby,
		},
		{
			name:     "Ruby with .gemspec in subdirectory",
			files:    []string{"gems/mylib/mylib.gemspec", "gems/mylib/lib/mylib.rb"},
			wantLang: LangRuby,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupTestDir(t, tt.files)
			lang, _, ok := DetectLanguage(dir)
			if !ok {
				t.Errorf("DetectLanguage() failed to detect language")
				return
			}
			if lang != tt.wantLang {
				t.Errorf("DetectLanguage() = %v, want %v", lang, tt.wantLang)
			}
		})
	}
}

func TestDetectLanguage_SkipsIgnoredDirs(t *testing.T) {
	// Files in node_modules, .git, vendor should be ignored
	files := []string{
		"node_modules/some-pkg/package.json",
		".git/config",
		"vendor/somelib/go.mod",
		"README.md",
	}
	dir := setupTestDir(t, files)

	lang, _, ok := DetectLanguage(dir)
	if ok {
		t.Errorf("DetectLanguage() should not detect language from ignored dirs, got %v", lang)
	}
}

func TestDetectLanguage_DepthLimit(t *testing.T) {
	// File at depth 4 should not be found (maxScanDepth = 3)
	files := []string{
		"a/b/c/d/MyApp.csproj", // depth 4
		"README.md",
	}
	dir := setupTestDir(t, files)

	lang, _, ok := DetectLanguage(dir)
	if ok && lang == LangCSharp {
		t.Errorf("DetectLanguage() should not find .csproj at depth 4")
	}

	// But depth 3 should work
	files2 := []string{
		"a/b/c/MyApp.csproj", // depth 3
	}
	dir2 := setupTestDir(t, files2)

	lang2, _, ok2 := DetectLanguage(dir2)
	if !ok2 || lang2 != LangCSharp {
		t.Errorf("DetectLanguage() should find .csproj at depth 3, got %v, ok=%v", lang2, ok2)
	}
}

func TestFindCompileCommands(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		wantPath string
	}{
		{
			name:     "At root",
			files:    []string{"compile_commands.json"},
			wantPath: "compile_commands.json",
		},
		{
			name:     "In build/",
			files:    []string{"build/compile_commands.json"},
			wantPath: "build/compile_commands.json",
		},
		{
			name:     "In out/",
			files:    []string{"out/compile_commands.json"},
			wantPath: "out/compile_commands.json",
		},
		{
			name:     "In cmake-build-debug/",
			files:    []string{"cmake-build-debug/compile_commands.json"},
			wantPath: "cmake-build-debug/compile_commands.json",
		},
		{
			name:     "In build/subdir/ (glob)",
			files:    []string{"build/Debug/compile_commands.json"},
			wantPath: "build/Debug/compile_commands.json",
		},
		{
			name:     "Not found",
			files:    []string{"src/main.cpp"},
			wantPath: "",
		},
		{
			name:     "Root takes precedence over build/",
			files:    []string{"compile_commands.json", "build/compile_commands.json"},
			wantPath: "compile_commands.json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupTestDir(t, tt.files)
			got := FindCompileCommands(dir)
			if got != tt.wantPath {
				t.Errorf("FindCompileCommands() = %q, want %q", got, tt.wantPath)
			}
		})
	}
}

func TestBuildCppCommand(t *testing.T) {
	tests := []struct {
		name           string
		files          []string
		overrideCompdb string
		wantCmd        string
		wantErr        bool
	}{
		{
			name:    "Auto-detect at root",
			files:   []string{"compile_commands.json"},
			wantCmd: "scip-clang --compdb-path=compile_commands.json",
		},
		{
			name:    "Auto-detect in build/",
			files:   []string{"build/compile_commands.json"},
			wantCmd: "scip-clang --compdb-path=build/compile_commands.json",
		},
		{
			name:           "Override path",
			files:          []string{"custom/path/compile_commands.json"},
			overrideCompdb: "custom/path/compile_commands.json",
			wantCmd:        "scip-clang --compdb-path=custom/path/compile_commands.json",
		},
		{
			name:    "Not found",
			files:   []string{"src/main.cpp"},
			wantCmd: "",
			wantErr: false, // Returns empty string, not error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupTestDir(t, tt.files)
			got, err := BuildCppCommand(dir, tt.overrideCompdb)
			if (err != nil) != tt.wantErr {
				t.Errorf("BuildCppCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantCmd {
				t.Errorf("BuildCppCommand() = %q, want %q", got, tt.wantCmd)
			}
		})
	}
}

func TestBuildRubyCommand(t *testing.T) {
	tests := []struct {
		name    string
		files   []string
		wantCmd string
		wantErr bool
	}{
		{
			name:    "With Gemfile and sorbet/config",
			files:   []string{"Gemfile", "sorbet/config"},
			wantCmd: "bundle exec scip-ruby",
		},
		{
			name:    "With Gemfile, no sorbet",
			files:   []string{"Gemfile"},
			wantCmd: "bundle exec scip-ruby .",
		},
		{
			name:    "No Gemfile",
			files:   []string{"lib/app.rb"},
			wantCmd: "scip-ruby .",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupTestDir(t, tt.files)
			got, err := BuildRubyCommand(dir)

			// Skip bundle check error in tests (bundle may not be installed)
			if err != nil && tt.files[0] == "Gemfile" {
				// This is expected if bundle is not installed
				t.Skipf("Skipping: bundle not installed")
				return
			}

			if (err != nil) != tt.wantErr {
				t.Errorf("BuildRubyCommand() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.wantCmd {
				t.Errorf("BuildRubyCommand() = %q, want %q", got, tt.wantCmd)
			}
		})
	}
}

func TestValidatePHPSetup(t *testing.T) {
	tests := []struct {
		name        string
		files       []string
		wantWarning bool
		wantErr     bool
	}{
		{
			name:        "Full setup",
			files:       []string{"composer.json", "composer.lock", "vendor/bin/scip-php"},
			wantWarning: false,
			wantErr:     false,
		},
		{
			name:        "Missing composer.lock (warning only)",
			files:       []string{"composer.json", "vendor/bin/scip-php"},
			wantWarning: true,
			wantErr:     false,
		},
		{
			name:        "Missing vendor/bin/scip-php (error)",
			files:       []string{"composer.json", "composer.lock"},
			wantWarning: false,
			wantErr:     true,
		},
		{
			name:        "Missing both (warning + error)",
			files:       []string{"composer.json"},
			wantWarning: true,
			wantErr:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupTestDir(t, tt.files)
			warning, err := ValidatePHPSetup(dir)

			gotWarning := warning != ""
			gotErr := err != nil

			if gotWarning != tt.wantWarning {
				t.Errorf("ValidatePHPSetup() warning = %v, want %v", gotWarning, tt.wantWarning)
			}
			if gotErr != tt.wantErr {
				t.Errorf("ValidatePHPSetup() err = %v, want %v", gotErr, tt.wantErr)
			}
		})
	}
}

func TestGetIndexerInfo(t *testing.T) {
	tests := []struct {
		lang      Language
		wantNil   bool
		wantCheck string
	}{
		{LangGo, false, "scip-go"},
		{LangTypeScript, false, "scip-typescript"},
		{LangJavaScript, false, "scip-typescript"},
		{LangPython, false, "scip-python"},
		{LangRust, false, "rust-analyzer"},
		{LangJava, false, "scip-java"},
		{LangKotlin, false, "scip-java"},
		{LangCpp, false, "scip-clang"},
		{LangDart, false, "dart"},
		{LangRuby, false, "scip-ruby"},
		{LangCSharp, false, "scip-dotnet"},
		{LangPHP, false, "vendor/bin/scip-php"},
		{LangUnknown, true, ""},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			info := GetIndexerInfo(tt.lang)
			if tt.wantNil {
				if info != nil {
					t.Errorf("GetIndexerInfo(%v) = %+v, want nil", tt.lang, info)
				}
				return
			}
			if info == nil {
				t.Errorf("GetIndexerInfo(%v) = nil, want non-nil", tt.lang)
				return
			}
			if info.CheckCommand != tt.wantCheck {
				t.Errorf("GetIndexerInfo(%v).CheckCommand = %q, want %q", tt.lang, info.CheckCommand, tt.wantCheck)
			}
		})
	}
}

func TestLanguageDisplayName(t *testing.T) {
	tests := []struct {
		lang Language
		want string
	}{
		{LangGo, "Go"},
		{LangTypeScript, "TypeScript"},
		{LangJavaScript, "JavaScript"},
		{LangPython, "Python"},
		{LangRust, "Rust"},
		{LangJava, "Java"},
		{LangKotlin, "Kotlin"},
		{LangCpp, "C/C++"},
		{LangDart, "Dart"},
		{LangRuby, "Ruby"},
		{LangCSharp, "C#"},
		{LangPHP, "PHP"},
		{LangUnknown, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(string(tt.lang), func(t *testing.T) {
			got := LanguageDisplayName(tt.lang)
			if got != tt.want {
				t.Errorf("LanguageDisplayName(%v) = %q, want %q", tt.lang, got, tt.want)
			}
		})
	}
}

func TestDetectJSorTS(t *testing.T) {
	tests := []struct {
		name     string
		files    []string
		wantLang Language
	}{
		{
			name:     "Has tsconfig.json",
			files:    []string{"package.json", "tsconfig.json"},
			wantLang: LangTypeScript,
		},
		{
			name:     "Has .ts files in root",
			files:    []string{"package.json", "index.ts"},
			wantLang: LangTypeScript,
		},
		{
			name:     "Has .ts files in src/",
			files:    []string{"package.json", "src/index.ts"},
			wantLang: LangTypeScript,
		},
		{
			name:     "Only .js files",
			files:    []string{"package.json", "index.js"},
			wantLang: LangJavaScript,
		},
		{
			name:     "No JS/TS files",
			files:    []string{"package.json"},
			wantLang: LangJavaScript, // Defaults to JS
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := setupTestDir(t, tt.files)
			got := detectJSorTS(dir)
			if got != tt.wantLang {
				t.Errorf("detectJSorTS() = %v, want %v", got, tt.wantLang)
			}
		})
	}
}

func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()

	config := &ProjectConfig{
		Language:     LangGo,
		Indexer:      "scip-go",
		ManifestPath: "go.mod",
	}

	// Save
	if err := SaveConfig(dir, config); err != nil {
		t.Fatalf("SaveConfig() error = %v", err)
	}

	// Verify file exists
	configPath := filepath.Join(dir, ".ckb", "project.json")
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Fatalf("Config file not created at %s", configPath)
	}

	// Load
	loaded, err := LoadConfig(dir)
	if err != nil {
		t.Fatalf("LoadConfig() error = %v", err)
	}

	if loaded.Language != config.Language {
		t.Errorf("LoadConfig().Language = %v, want %v", loaded.Language, config.Language)
	}
	if loaded.Indexer != config.Indexer {
		t.Errorf("LoadConfig().Indexer = %v, want %v", loaded.Indexer, config.Indexer)
	}
	if loaded.ManifestPath != config.ManifestPath {
		t.Errorf("LoadConfig().ManifestPath = %v, want %v", loaded.ManifestPath, config.ManifestPath)
	}
}

func TestLoadConfig_NotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := LoadConfig(dir)
	if err == nil {
		t.Errorf("LoadConfig() should return error for missing config")
	}
}
