package modules

import (
	"testing"
)

func TestNewModule(t *testing.T) {
	module := NewModule(
		"mod-123",
		"my-module",
		"internal/mymodule",
		ManifestGoMod,
		LanguageGo,
		"state-abc",
	)

	if module == nil {
		t.Fatal("NewModule returned nil")
	}
	if module.ID != "mod-123" {
		t.Errorf("ID = %q, want %q", module.ID, "mod-123")
	}
	if module.Name != "my-module" {
		t.Errorf("Name = %q, want %q", module.Name, "my-module")
	}
	if module.RootPath != "internal/mymodule" {
		t.Errorf("RootPath = %q, want %q", module.RootPath, "internal/mymodule")
	}
	if module.ManifestType != ManifestGoMod {
		t.Errorf("ManifestType = %q, want %q", module.ManifestType, ManifestGoMod)
	}
	if module.Language != LanguageGo {
		t.Errorf("Language = %q, want %q", module.Language, LanguageGo)
	}
	if module.StateId != "state-abc" {
		t.Errorf("StateId = %q, want %q", module.StateId, "state-abc")
	}
	if module.DetectedAt == "" {
		t.Error("DetectedAt should be set")
	}
}

func TestModule_IsManifestBased(t *testing.T) {
	tests := []struct {
		name         string
		manifestType string
		expected     bool
	}{
		{"go.mod", ManifestGoMod, true},
		{"package.json", ManifestPackageJSON, true},
		{"Cargo.toml", ManifestCargoToml, true},
		{"no manifest", ManifestNone, false},
		{"empty string", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &Module{ManifestType: tt.manifestType}
			result := m.IsManifestBased()
			if result != tt.expected {
				t.Errorf("IsManifestBased() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestManifestTypeConstants(t *testing.T) {
	if ManifestPackageJSON != "package.json" {
		t.Errorf("ManifestPackageJSON = %q, want %q", ManifestPackageJSON, "package.json")
	}
	if ManifestGoMod != "go.mod" {
		t.Errorf("ManifestGoMod = %q, want %q", ManifestGoMod, "go.mod")
	}
	if ManifestCargoToml != "Cargo.toml" {
		t.Errorf("ManifestCargoToml = %q, want %q", ManifestCargoToml, "Cargo.toml")
	}
	if ManifestPubspecYaml != "pubspec.yaml" {
		t.Errorf("ManifestPubspecYaml = %q, want %q", ManifestPubspecYaml, "pubspec.yaml")
	}
	if ManifestPyprojectToml != "pyproject.toml" {
		t.Errorf("ManifestPyprojectToml = %q, want %q", ManifestPyprojectToml, "pyproject.toml")
	}
	if ManifestSetupPy != "setup.py" {
		t.Errorf("ManifestSetupPy = %q, want %q", ManifestSetupPy, "setup.py")
	}
	if ManifestPomXML != "pom.xml" {
		t.Errorf("ManifestPomXML = %q, want %q", ManifestPomXML, "pom.xml")
	}
	if ManifestBuildGradle != "build.gradle" {
		t.Errorf("ManifestBuildGradle = %q, want %q", ManifestBuildGradle, "build.gradle")
	}
	if ManifestBuildGradleKts != "build.gradle.kts" {
		t.Errorf("ManifestBuildGradleKts = %q, want %q", ManifestBuildGradleKts, "build.gradle.kts")
	}
	if ManifestNone != "" {
		t.Errorf("ManifestNone = %q, want empty string", ManifestNone)
	}
}

func TestLanguageConstants(t *testing.T) {
	if LanguageTypeScript != "typescript" {
		t.Errorf("LanguageTypeScript = %q, want %q", LanguageTypeScript, "typescript")
	}
	if LanguageJavaScript != "javascript" {
		t.Errorf("LanguageJavaScript = %q, want %q", LanguageJavaScript, "javascript")
	}
	if LanguageDart != "dart" {
		t.Errorf("LanguageDart = %q, want %q", LanguageDart, "dart")
	}
	if LanguageGo != "go" {
		t.Errorf("LanguageGo = %q, want %q", LanguageGo, "go")
	}
	if LanguageRust != "rust" {
		t.Errorf("LanguageRust = %q, want %q", LanguageRust, "rust")
	}
	if LanguagePython != "python" {
		t.Errorf("LanguagePython = %q, want %q", LanguagePython, "python")
	}
	if LanguageJava != "java" {
		t.Errorf("LanguageJava = %q, want %q", LanguageJava, "java")
	}
	if LanguageKotlin != "kotlin" {
		t.Errorf("LanguageKotlin = %q, want %q", LanguageKotlin, "kotlin")
	}
	if LanguageUnknown != "unknown" {
		t.Errorf("LanguageUnknown = %q, want %q", LanguageUnknown, "unknown")
	}
}

func TestModuleStruct(t *testing.T) {
	module := Module{
		ID:           "test-id",
		Name:         "test-name",
		RootPath:     "src/module",
		ManifestType: ManifestPackageJSON,
		Language:     LanguageTypeScript,
		DetectedAt:   "2024-01-15T10:00:00Z",
		StateId:      "state-123",
	}

	if module.ID != "test-id" {
		t.Errorf("ID = %q, want %q", module.ID, "test-id")
	}
	if module.RootPath != "src/module" {
		t.Errorf("RootPath = %q, want %q", module.RootPath, "src/module")
	}
}
