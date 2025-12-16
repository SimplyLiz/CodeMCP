package modules

import (
	"time"
)

// Module represents a detected module/package in the repository
type Module struct {
	// ID is the unique identifier for this module (ModuleId)
	ID string `json:"id"`

	// Name is the human-readable name of the module
	Name string `json:"name"`

	// RootPath is the repo-relative path to the module root
	RootPath string `json:"rootPath"`

	// ManifestType indicates which manifest file was used to detect this module
	// Examples: "package.json", "go.mod", "pubspec.yaml", "Cargo.toml", etc.
	ManifestType string `json:"manifestType"`

	// Language is the detected primary language of the module
	Language string `json:"language"`

	// DetectedAt is the timestamp when this module was detected
	DetectedAt string `json:"detectedAt"`

	// StateId is the repoStateId at which this module was detected
	StateId string `json:"stateId"`
}

// ManifestType constants for well-known manifest files
const (
	ManifestPackageJSON    = "package.json"
	ManifestPubspecYaml    = "pubspec.yaml"
	ManifestGoMod          = "go.mod"
	ManifestCargoToml      = "Cargo.toml"
	ManifestPyprojectToml  = "pyproject.toml"
	ManifestSetupPy        = "setup.py"
	ManifestPomXML         = "pom.xml"
	ManifestBuildGradle    = "build.gradle"
	ManifestBuildGradleKts = "build.gradle.kts"
	ManifestNone           = "" // For convention-based or directory fallback detection
)

// Language constants
const (
	LanguageTypeScript = "typescript"
	LanguageJavaScript = "javascript"
	LanguageDart       = "dart"
	LanguageGo         = "go"
	LanguageRust       = "rust"
	LanguagePython     = "python"
	LanguageJava       = "java"
	LanguageKotlin     = "kotlin"
	LanguageUnknown    = "unknown"
)

// NewModule creates a new Module with the current timestamp
func NewModule(id, name, rootPath, manifestType, language, stateId string) *Module {
	return &Module{
		ID:           id,
		Name:         name,
		RootPath:     rootPath,
		ManifestType: manifestType,
		Language:     language,
		DetectedAt:   time.Now().UTC().Format(time.RFC3339),
		StateId:      stateId,
	}
}

// IsManifestBased returns true if the module was detected via a manifest file
func (m *Module) IsManifestBased() bool {
	return m.ManifestType != ManifestNone
}
