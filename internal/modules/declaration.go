package modules

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	toml "github.com/pelletier/go-toml/v2"

	"ckb/internal/paths"
)

// ModulesDeclarationFile is the default filename for module declarations
const ModulesDeclarationFile = "MODULES.toml"

// ModuleDeclaration represents a declared module in MODULES.toml
type ModuleDeclaration struct {
	// ID is the unique module identifier (optional, will be generated if not provided)
	ID string `toml:"id"`

	// Name is the human-readable name of the module
	Name string `toml:"name"`

	// Path is the repo-relative path to the module root
	Path string `toml:"path"`

	// Responsibility is a one-line description of what this module does
	Responsibility string `toml:"responsibility,omitempty"`

	// Boundaries defines the module's API surface
	Boundaries *BoundaryDeclaration `toml:"boundaries,omitempty"`

	// Owner is the owner reference (e.g., @team-name or user@email.com)
	Owner string `toml:"owner,omitempty"`

	// Tags are classification tags for the module
	Tags []string `toml:"tags,omitempty"`

	// Language is the primary language of the module (optional, will be detected)
	Language string `toml:"language,omitempty"`
}

// BoundaryDeclaration defines module boundaries/API surface
type BoundaryDeclaration struct {
	// Exports are the public APIs exposed by this module
	Exports []string `toml:"exports,omitempty"`

	// Internal are paths that are considered internal/private
	Internal []string `toml:"internal,omitempty"`

	// AllowedDependencies are modules this module is allowed to depend on
	AllowedDependencies []string `toml:"allowed_dependencies,omitempty"`
}

// ModulesFile represents the root structure of MODULES.toml
type ModulesFile struct {
	// Version is the schema version
	Version int `toml:"version"`

	// Modules is the list of declared modules
	Modules []ModuleDeclaration `toml:"module"`
}

// ParseModulesFile parses a MODULES.toml file from the given path
func ParseModulesFile(filePath string) (*ModulesFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read MODULES.toml: %w", err)
	}

	var modulesFile ModulesFile
	if err := toml.Unmarshal(data, &modulesFile); err != nil {
		return nil, fmt.Errorf("failed to parse MODULES.toml: %w", err)
	}

	// Validate version
	if modulesFile.Version < 1 {
		modulesFile.Version = 1 // Default to version 1
	}

	return &modulesFile, nil
}

// LoadDeclaredModules loads declared modules from MODULES.toml if it exists
func LoadDeclaredModules(repoRoot string, declarationFile string, stateId string) ([]*Module, error) {
	if declarationFile == "" {
		declarationFile = ModulesDeclarationFile
	}

	filePath := filepath.Join(repoRoot, declarationFile)

	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, nil // No declared modules file
	}

	modulesFile, err := ParseModulesFile(filePath)
	if err != nil {
		return nil, err
	}

	return convertDeclarationsToModules(repoRoot, modulesFile.Modules, stateId)
}

// convertDeclarationsToModules converts ModuleDeclarations to Module structs
func convertDeclarationsToModules(repoRoot string, declarations []ModuleDeclaration, stateId string) ([]*Module, error) {
	var modules []*Module

	for _, decl := range declarations {
		// Validate required fields
		if decl.Path == "" {
			return nil, fmt.Errorf("module declaration missing required 'path' field")
		}

		// Generate ID if not provided
		moduleID := decl.ID
		if moduleID == "" {
			moduleID = GenerateStableModuleID(repoRoot, decl.Path)
		}

		// Use path as name if name not provided
		name := decl.Name
		if name == "" {
			parts := strings.Split(decl.Path, "/")
			name = parts[len(parts)-1]
		}

		// Detect language if not specified
		language := decl.Language
		if language == "" {
			absPath := filepath.Join(repoRoot, decl.Path)
			language = detectLanguageFromFiles(absPath)
		}

		// Detect manifest if present
		absPath := filepath.Join(repoRoot, decl.Path)
		manifest, _ := detectManifestInDir(absPath)

		module := &Module{
			ID:           moduleID,
			Name:         name,
			RootPath:     decl.Path,
			ManifestType: manifest,
			Language:     language,
			DetectedAt:   time.Now().UTC().Format(time.RFC3339),
			StateId:      stateId,
		}
		modules = append(modules, module)
	}

	return modules, nil
}

// DeclarationMetadata holds the v6.0 metadata from a module declaration
type DeclarationMetadata struct {
	ModuleID       string
	Responsibility string
	Boundaries     *BoundaryDeclaration
	Owner          string
	Tags           []string
}

// ExtractMetadataFromDeclarations extracts v6.0 metadata from declarations
func ExtractMetadataFromDeclarations(filePath string) (map[string]*DeclarationMetadata, error) {
	modulesFile, err := ParseModulesFile(filePath)
	if err != nil {
		return nil, err
	}

	metadata := make(map[string]*DeclarationMetadata)

	for _, decl := range modulesFile.Modules {
		moduleID := decl.ID
		if moduleID == "" {
			// Generate ID from path for lookup
			moduleID = GenerateStableModuleID("", decl.Path)
		}

		metadata[moduleID] = &DeclarationMetadata{
			ModuleID:       moduleID,
			Responsibility: decl.Responsibility,
			Boundaries:     decl.Boundaries,
			Owner:          decl.Owner,
			Tags:           decl.Tags,
		}
	}

	return metadata, nil
}

// WriteModulesFile writes a ModulesFile to the given path
func WriteModulesFile(filePath string, modulesFile *ModulesFile) error {
	data, err := toml.Marshal(modulesFile)
	if err != nil {
		return fmt.Errorf("failed to marshal MODULES.toml: %w", err)
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write MODULES.toml: %w", err)
	}

	return nil
}

// CreateExampleModulesFile creates an example MODULES.toml file
func CreateExampleModulesFile(filePath string) error {
	example := &ModulesFile{
		Version: 1,
		Modules: []ModuleDeclaration{
			{
				Name:           "api",
				Path:           "internal/api",
				Responsibility: "HTTP API handlers and middleware",
				Owner:          "@api-team",
				Tags:           []string{"core", "api"},
				Boundaries: &BoundaryDeclaration{
					Exports:             []string{"Handler", "Middleware"},
					Internal:            []string{"internal/api/internal"},
					AllowedDependencies: []string{"internal/query", "internal/storage"},
				},
			},
			{
				Name:           "query",
				Path:           "internal/query",
				Responsibility: "Query engine for code intelligence",
				Owner:          "@platform-team",
				Tags:           []string{"core", "query"},
			},
		},
	}

	return WriteModulesFile(filePath, example)
}

// GenerateStableModuleID generates a stable module ID that survives renames
// Format: ckb:mod:<hash>
// The hash is based on the normalized path
func GenerateStableModuleID(repoRoot string, modulePath string) string {
	// Normalize path
	normalizedPath := paths.NormalizePath(modulePath)

	// For declared modules, we use a simpler hash based on path
	// This allows the ID to be regenerated deterministically
	hash := sha256.Sum256([]byte(normalizedPath))
	hashStr := hex.EncodeToString(hash[:8]) // Use first 8 bytes for shorter ID

	return fmt.Sprintf("ckb:mod:%s", hashStr)
}

// ModuleIDFromPath generates a module ID for lookup purposes
// This is the same algorithm used in GenerateStableModuleID
func ModuleIDFromPath(modulePath string) string {
	return GenerateStableModuleID("", modulePath)
}

// ParseModuleID extracts components from a module ID
// Returns (prefix, hash, isValid)
func ParseModuleID(moduleID string) (prefix string, hash string, isValid bool) {
	if !strings.HasPrefix(moduleID, "ckb:mod:") {
		return "", "", false
	}

	parts := strings.Split(moduleID, ":")
	if len(parts) != 3 {
		return "", "", false
	}

	// Hash must not be empty
	if parts[2] == "" {
		return "", "", false
	}

	return parts[0] + ":" + parts[1], parts[2], true
}

// IsValidModuleID checks if a string is a valid module ID
func IsValidModuleID(moduleID string) bool {
	_, _, isValid := ParseModuleID(moduleID)
	return isValid
}
