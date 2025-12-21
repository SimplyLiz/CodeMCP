package tier

import (
	"os"
	"path/filepath"
)

// Prerequisite represents a project requirement for a language.
type Prerequisite struct {
	Name        string   // e.g., "go.mod"
	Description string   // e.g., "Go module file"
	Paths       []string // Relative paths to check
	Required    bool     // True = must exist, False = warning only
}

// PrerequisiteStatus represents the status of a prerequisite check.
type PrerequisiteStatus struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Found       bool   `json:"found"`
	Path        string `json:"path,omitempty"`
	Required    bool   `json:"required"`
	Hint        string `json:"hint,omitempty"`
}

// LanguagePrerequisites maps languages to their project requirements.
var LanguagePrerequisites = map[Language][]Prerequisite{
	LangGo: {
		{
			Name:        "go.mod",
			Description: "Go module file",
			Paths:       []string{"go.mod"},
			Required:    true,
		},
	},
	LangTypeScript: {
		{
			Name:        "package.json",
			Description: "Node.js package manifest",
			Paths:       []string{"package.json"},
			Required:    true,
		},
		{
			Name:        "tsconfig.json",
			Description: "TypeScript configuration",
			Paths:       []string{"tsconfig.json", "tsconfig.base.json"},
			Required:    false, // Warning only - might be in subdirectory
		},
	},
	LangJavaScript: {
		{
			Name:        "package.json",
			Description: "Node.js package manifest",
			Paths:       []string{"package.json"},
			Required:    true,
		},
	},
	LangPython: {
		{
			Name:        "project config",
			Description: "Python project configuration",
			Paths:       []string{"pyproject.toml", "setup.py", "setup.cfg", "requirements.txt"},
			Required:    false, // At least one should exist
		},
	},
	LangRust: {
		{
			Name:        "Cargo.toml",
			Description: "Rust package manifest",
			Paths:       []string{"Cargo.toml"},
			Required:    true,
		},
	},
	LangJava: {
		{
			Name:        "build config",
			Description: "Java build configuration",
			Paths:       []string{"pom.xml", "build.gradle", "build.gradle.kts"},
			Required:    true,
		},
	},
	LangKotlin: {
		{
			Name:        "build config",
			Description: "Kotlin build configuration",
			Paths:       []string{"build.gradle.kts", "build.gradle"},
			Required:    true,
		},
	},
	LangCpp: {
		{
			Name:        "compile_commands.json",
			Description: "Compilation database",
			Paths:       []string{"compile_commands.json", "build/compile_commands.json"},
			Required:    true, // Required for SCIP indexing
		},
	},
	LangCSharp: {
		{
			Name:        "project file",
			Description: "C# project or solution",
			Paths:       []string{"*.csproj", "*.sln"},
			Required:    true,
		},
	},
	LangRuby: {
		{
			Name:        "Gemfile",
			Description: "Ruby dependencies",
			Paths:       []string{"Gemfile"},
			Required:    false,
		},
	},
	LangDart: {
		{
			Name:        "pubspec.yaml",
			Description: "Dart package manifest",
			Paths:       []string{"pubspec.yaml"},
			Required:    true,
		},
	},
	LangPHP: {
		{
			Name:        "composer.json",
			Description: "Composer package manifest",
			Paths:       []string{"composer.json"},
			Required:    true,
		},
	},
}

// PrerequisiteChecker checks project prerequisites.
type PrerequisiteChecker struct {
	workspaceRoot string
}

// NewPrerequisiteChecker creates a new checker for the given workspace.
func NewPrerequisiteChecker(workspaceRoot string) *PrerequisiteChecker {
	return &PrerequisiteChecker{workspaceRoot: workspaceRoot}
}

// CheckPrerequisites checks all prerequisites for a language.
func (c *PrerequisiteChecker) CheckPrerequisites(lang Language) []PrerequisiteStatus {
	prereqs, ok := LanguagePrerequisites[lang]
	if !ok {
		return nil
	}

	var results []PrerequisiteStatus
	for _, prereq := range prereqs {
		status := PrerequisiteStatus{
			Name:        prereq.Name,
			Description: prereq.Description,
			Required:    prereq.Required,
		}

		// Check each possible path
		for _, pathPattern := range prereq.Paths {
			fullPath := filepath.Join(c.workspaceRoot, pathPattern)

			// Handle glob patterns
			if containsGlob(pathPattern) {
				matches, err := filepath.Glob(fullPath)
				if err == nil && len(matches) > 0 {
					status.Found = true
					status.Path = matches[0]
					break
				}
			} else {
				if _, err := os.Stat(fullPath); err == nil {
					status.Found = true
					status.Path = fullPath
					break
				}
			}
		}

		// Add hints for missing prerequisites
		if !status.Found {
			status.Hint = getPrerequisiteHint(lang, prereq)
		}

		results = append(results, status)
	}

	return results
}

// CheckAllLanguages checks prerequisites for multiple languages.
func (c *PrerequisiteChecker) CheckAllLanguages(languages []Language) map[Language][]PrerequisiteStatus {
	results := make(map[Language][]PrerequisiteStatus)
	for _, lang := range languages {
		results[lang] = c.CheckPrerequisites(lang)
	}
	return results
}

// HasRequiredPrerequisites returns true if all required prerequisites are met.
func HasRequiredPrerequisites(statuses []PrerequisiteStatus) bool {
	for _, status := range statuses {
		if status.Required && !status.Found {
			return false
		}
	}
	return true
}

// containsGlob checks if a path contains glob patterns.
func containsGlob(path string) bool {
	return filepath.Base(path) != path && (filepath.Base(path)[0] == '*' ||
		len(path) > 0 && (path[0] == '*' || path[len(path)-1] == '*'))
}

// getPrerequisiteHint returns a hint for a missing prerequisite.
func getPrerequisiteHint(lang Language, prereq Prerequisite) string {
	switch lang {
	case LangGo:
		if prereq.Name == "go.mod" {
			return "Run 'go mod init <module-name>' to initialize"
		}
	case LangTypeScript:
		if prereq.Name == "tsconfig.json" {
			return "Run 'npx tsc --init' to create TypeScript configuration"
		}
		if prereq.Name == "package.json" {
			return "Run 'npm init' to initialize package"
		}
	case LangPython:
		return "Create pyproject.toml or requirements.txt"
	case LangRust:
		if prereq.Name == "Cargo.toml" {
			return "Run 'cargo init' to initialize project"
		}
	case LangCpp:
		if prereq.Name == "compile_commands.json" {
			return "Run 'cmake -DCMAKE_EXPORT_COMPILE_COMMANDS=ON -B build'"
		}
	case LangCSharp:
		return "Create a .csproj file or run 'dotnet new'"
	case LangDart:
		if prereq.Name == "pubspec.yaml" {
			return "Run 'dart create' to initialize project"
		}
	case LangPHP:
		if prereq.Name == "composer.json" {
			return "Run 'composer init' to initialize project"
		}
	}
	return ""
}
