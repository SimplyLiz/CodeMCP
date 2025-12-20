// Package project provides language and indexer detection for repositories.
package project

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Language represents a programming language.
type Language string

const (
	LangGo         Language = "go"
	LangTypeScript Language = "typescript"
	LangJavaScript Language = "javascript"
	LangPython     Language = "python"
	LangRust       Language = "rust"
	LangJava       Language = "java"
	LangKotlin     Language = "kotlin"
	LangUnknown    Language = "unknown"
)

// IndexerInfo contains information about a SCIP indexer.
type IndexerInfo struct {
	Command        string // Command to run the indexer
	InstallCommand string // Command to install the indexer
	CheckCommand   string // Command to check if installed (usually just the binary name)
	OutputFile     string // Expected output file (default: index.scip)
}

// ProjectConfig stores detected project information.
type ProjectConfig struct {
	Language     Language  `json:"language"`
	Indexer      string    `json:"indexer"`
	ManifestPath string    `json:"manifestPath"`
	DetectedAt   time.Time `json:"detectedAt"`
}

// DetectLanguage detects the primary language of a project from manifest files.
// Returns the language, manifest path, and whether detection succeeded.
func DetectLanguage(root string) (Language, string, bool) {
	// Check for manifest files in priority order
	manifests := []struct {
		path string
		lang Language
	}{
		{"go.mod", LangGo},
		{"package.json", LangTypeScript}, // Assume TS for package.json, refine below
		{"Cargo.toml", LangRust},
		{"pyproject.toml", LangPython},
		{"requirements.txt", LangPython},
		{"setup.py", LangPython},
		{"pom.xml", LangJava},
		{"build.gradle", LangJava},
		{"build.gradle.kts", LangKotlin},
	}

	for _, m := range manifests {
		fullPath := filepath.Join(root, m.path)
		if _, err := os.Stat(fullPath); err == nil {
			lang := m.lang
			// Refine TypeScript vs JavaScript check
			if m.path == "package.json" {
				lang = detectJSorTS(root)
			}
			return lang, m.path, true
		}
	}

	return LangUnknown, "", false
}

// detectJSorTS checks if a project is TypeScript or JavaScript.
func detectJSorTS(root string) Language {
	// Check for tsconfig.json
	if _, err := os.Stat(filepath.Join(root, "tsconfig.json")); err == nil {
		return LangTypeScript
	}
	// Check for .ts files
	if hasFileWithExt(root, ".ts") {
		return LangTypeScript
	}
	return LangJavaScript
}

// hasFileWithExt checks if any file with the given extension exists in the root.
func hasFileWithExt(root, ext string) bool {
	entries, err := os.ReadDir(root)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ext {
			return true
		}
	}
	// Check src directory if exists
	srcDir := filepath.Join(root, "src")
	if entries, err := os.ReadDir(srcDir); err == nil {
		for _, e := range entries {
			if !e.IsDir() && filepath.Ext(e.Name()) == ext {
				return true
			}
		}
	}
	return false
}

// GetIndexerInfo returns the SCIP indexer information for a language.
func GetIndexerInfo(lang Language) *IndexerInfo {
	switch lang {
	case LangGo:
		return &IndexerInfo{
			Command:        "scip-go",
			InstallCommand: "go install github.com/sourcegraph/scip-go@latest",
			CheckCommand:   "scip-go",
			OutputFile:     "index.scip",
		}
	case LangTypeScript, LangJavaScript:
		return &IndexerInfo{
			Command:        "scip-typescript index --infer-tsconfig",
			InstallCommand: "npm install -g @sourcegraph/scip-typescript",
			CheckCommand:   "scip-typescript",
			OutputFile:     "index.scip",
		}
	case LangPython:
		return &IndexerInfo{
			Command:        "scip-python index .",
			InstallCommand: "pip install scip-python",
			CheckCommand:   "scip-python",
			OutputFile:     "index.scip",
		}
	case LangRust:
		return &IndexerInfo{
			Command:        "rust-analyzer scip .",
			InstallCommand: "rustup component add rust-analyzer",
			CheckCommand:   "rust-analyzer",
			OutputFile:     "index.scip",
		}
	case LangJava:
		return &IndexerInfo{
			Command:        "scip-java index",
			InstallCommand: "cs install scip-java",
			CheckCommand:   "scip-java",
			OutputFile:     "index.scip",
		}
	case LangKotlin:
		return &IndexerInfo{
			Command:        "scip-kotlin index",
			InstallCommand: "cs install scip-kotlin",
			CheckCommand:   "scip-kotlin",
			OutputFile:     "index.scip",
		}
	default:
		return nil
	}
}

// SaveConfig saves project configuration to .ckb/project.json.
func SaveConfig(root string, config *ProjectConfig) error {
	ckbDir := filepath.Join(root, ".ckb")
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(ckbDir, "project.json"), data, 0644)
}

// LoadConfig loads project configuration from .ckb/project.json.
func LoadConfig(root string) (*ProjectConfig, error) {
	configPath := filepath.Join(root, ".ckb", "project.json")
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var config ProjectConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, err
	}

	return &config, nil
}

// LanguageDisplayName returns a human-readable name for the language.
func LanguageDisplayName(lang Language) string {
	switch lang {
	case LangGo:
		return "Go"
	case LangTypeScript:
		return "TypeScript"
	case LangJavaScript:
		return "JavaScript"
	case LangPython:
		return "Python"
	case LangRust:
		return "Rust"
	case LangJava:
		return "Java"
	case LangKotlin:
		return "Kotlin"
	default:
		return "Unknown"
	}
}
