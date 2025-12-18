package federation

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// OpenAPIDetector detects OpenAPI/Swagger contracts
type OpenAPIDetector struct{}

// NewOpenAPIDetector creates a new OpenAPI detector
func NewOpenAPIDetector() *OpenAPIDetector {
	return &OpenAPIDetector{}
}

// Name returns the detector name
func (d *OpenAPIDetector) Name() string {
	return "openapi"
}

// OpenAPI file patterns
var (
	openapiFileNames = []string{
		"openapi.yaml", "openapi.yml", "openapi.json",
		"swagger.yaml", "swagger.yml", "swagger.json",
		"api.yaml", "api.yml", "api.json",
	}

	openapiPathPattern   = regexp.MustCompile(`/(api|spec|openapi|contracts)/`)
	internalPathPattern  = regexp.MustCompile(`/(internal|test|mock|example)/`)
	generatedMarkerFiles = []string{
		".openapi-generator/FILES",
		".swagger-codegen/",
	}
	generatorConfigFiles = []string{
		"openapitools.json",
		"orval.config.ts",
		"orval.config.js",
		"swagger-codegen-config.json",
	}
)

// Detect scans a repository for OpenAPI contracts
func (d *OpenAPIDetector) Detect(repoPath string, repoUID string, repoID string) (*DetectorResult, error) {
	result := &DetectorResult{
		Contracts:  []Contract{},
		References: []OutgoingReference{},
	}

	// Find OpenAPI files by known names
	for _, name := range openapiFileNames {
		matches, err := findFilesNamed(repoPath, name)
		if err != nil {
			continue
		}

		for _, match := range matches {
			relPath, err := filepath.Rel(repoPath, match)
			if err != nil {
				continue
			}

			contract, err := d.parseOpenAPIFile(match, relPath, repoUID, repoID)
			if err != nil {
				continue
			}

			if contract != nil {
				result.Contracts = append(result.Contracts, *contract)
			}
		}
	}

	// Find generator configs (consumers)
	generatorRefs, err := d.findGeneratorConfigs(repoPath, repoUID, repoID)
	if err == nil {
		result.References = append(result.References, generatorRefs...)
	}

	// Find generated client markers
	markerRefs, err := d.findGeneratedMarkers(repoPath, repoUID, repoID)
	if err == nil {
		result.References = append(result.References, markerRefs...)
	}

	return result, nil
}

// parseOpenAPIFile parses an OpenAPI file and extracts metadata
func (d *OpenAPIDetector) parseOpenAPIFile(filePath, relPath, repoUID, repoID string) (*Contract, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var doc map[string]interface{}

	// Try YAML first, then JSON
	if strings.HasSuffix(filePath, ".yaml") || strings.HasSuffix(filePath, ".yml") {
		if err := yaml.Unmarshal(content, &doc); err != nil {
			return nil, err
		}
	} else {
		if err := json.Unmarshal(content, &doc); err != nil {
			return nil, err
		}
	}

	// Detect OpenAPI version
	var metadata OpenAPIMetadata

	if openapi, ok := doc["openapi"].(string); ok && strings.HasPrefix(openapi, "3.") {
		// OpenAPI 3.x
		if strings.HasPrefix(openapi, "3.1") {
			metadata.Version = "3.1"
		} else {
			metadata.Version = "3.0"
		}

		// Extract info
		if info, ok := doc["info"].(map[string]interface{}); ok {
			if title, ok := info["title"].(string); ok {
				metadata.Title = title
			}
			if version, ok := info["version"].(string); ok {
				metadata.APIVersion = version
			}
		}

		// Extract servers
		if servers, ok := doc["servers"].([]interface{}); ok {
			for _, s := range servers {
				if server, ok := s.(map[string]interface{}); ok {
					if url, ok := server["url"].(string); ok {
						metadata.Servers = append(metadata.Servers, url)
					}
				}
			}
		}
	} else if swagger, ok := doc["swagger"].(string); ok && swagger == "2.0" {
		// Swagger 2.0
		metadata.Version = "2.0"

		// Extract info
		if info, ok := doc["info"].(map[string]interface{}); ok {
			if title, ok := info["title"].(string); ok {
				metadata.Title = title
			}
			if version, ok := info["version"].(string); ok {
				metadata.APIVersion = version
			}
		}

		// Construct server URL
		if host, ok := doc["host"].(string); ok {
			basePath, _ := doc["basePath"].(string)
			metadata.Servers = append(metadata.Servers, fmt.Sprintf("https://%s%s", host, basePath))
		}
	} else {
		// Not a recognized OpenAPI/Swagger file
		return nil, fmt.Errorf("not an OpenAPI file")
	}

	// Classify visibility
	visibility, visibilityBasis, confidence := d.classifyVisibility(relPath, metadata)

	metadataJSON, _ := json.Marshal(metadata)

	contract := &Contract{
		ID:              fmt.Sprintf("%s:%s", repoUID, relPath),
		RepoUID:         repoUID,
		RepoID:          repoID,
		Path:            relPath,
		ContractType:    ContractTypeOpenAPI,
		Metadata:        metadataJSON,
		Visibility:      visibility,
		VisibilityBasis: visibilityBasis,
		Confidence:      confidence,
		ImportKeys:      []string{relPath, metadata.Title}, // Use path and title as keys
		IndexedAt:       time.Now(),
	}

	return contract, nil
}

// classifyVisibility classifies the visibility of an OpenAPI file
func (d *OpenAPIDetector) classifyVisibility(path string, metadata OpenAPIMetadata) (Visibility, string, float64) {
	// Check for internal patterns first
	if internalPathPattern.MatchString(path) {
		return VisibilityInternal, "path_pattern_internal", 0.9
	}

	// Check for public patterns
	if openapiPathPattern.MatchString(path) {
		return VisibilityPublic, "path_pattern_api", 1.0
	}

	// Server URLs indicate public
	for _, server := range metadata.Servers {
		if !strings.Contains(server, "localhost") && !strings.Contains(server, "127.0.0.1") {
			return VisibilityPublic, "public_server_url", 0.9
		}
	}

	return VisibilityUnknown, "no_clear_pattern", 0.5
}

// findGeneratorConfigs finds OpenAPI generator configuration files
func (d *OpenAPIDetector) findGeneratorConfigs(repoPath, repoUID, repoID string) ([]OutgoingReference, error) {
	var refs []OutgoingReference

	for _, configFile := range generatorConfigFiles {
		configPath := filepath.Join(repoPath, configFile)
		if _, err := os.Stat(configPath); os.IsNotExist(err) {
			continue
		}

		inputSpec, err := d.extractInputSpec(configPath)
		if err != nil || inputSpec == "" {
			continue
		}

		refs = append(refs, OutgoingReference{
			ConsumerRepoUID: repoUID,
			ConsumerRepoID:  repoID,
			ConsumerPath:    configFile,
			ImportKey:       inputSpec,
			Tier:            TierDeclared,
			EvidenceType:    "generator_config",
			Confidence:      1.0,
			ConfidenceBasis: "explicit_input_spec",
			DetectorName:    d.Name(),
		})
	}

	return refs, nil
}

// extractInputSpec extracts the input spec path from a generator config
func (d *OpenAPIDetector) extractInputSpec(configPath string) (string, error) {
	content, err := os.ReadFile(configPath)
	if err != nil {
		return "", err
	}

	// Try JSON first
	var jsonConfig map[string]interface{}
	if err := json.Unmarshal(content, &jsonConfig); err == nil {
		// openapi-generator config
		if generatorConfig, ok := jsonConfig["generator-cli"].(map[string]interface{}); ok {
			if generators, ok := generatorConfig["generators"].(map[string]interface{}); ok {
				for _, gen := range generators {
					if genMap, ok := gen.(map[string]interface{}); ok {
						if inputSpec, ok := genMap["inputSpec"].(string); ok {
							return inputSpec, nil
						}
					}
				}
			}
		}

		// Direct inputSpec
		if inputSpec, ok := jsonConfig["inputSpec"].(string); ok {
			return inputSpec, nil
		}
	}

	// For TypeScript configs, we'd need more complex parsing
	// Just check if the file exists for now
	return "", nil
}

// findGeneratedMarkers finds OpenAPI/Swagger generated code markers
func (d *OpenAPIDetector) findGeneratedMarkers(repoPath, repoUID, repoID string) ([]OutgoingReference, error) {
	var refs []OutgoingReference

	for _, marker := range generatedMarkerFiles {
		markerPath := filepath.Join(repoPath, marker)
		if info, err := os.Stat(markerPath); err == nil {
			relPath := marker

			if info.IsDir() {
				// For directories, try to find metadata inside
				metaPath := filepath.Join(markerPath, "FILES")
				if _, err := os.Stat(metaPath); err == nil {
					// Read first line which often contains spec reference
					content, err := os.ReadFile(metaPath)
					if err == nil {
						lines := strings.Split(string(content), "\n")
						if len(lines) > 0 {
							// First line is often a comment with the spec URL
							firstLine := strings.TrimPrefix(lines[0], "#")
							firstLine = strings.TrimSpace(firstLine)
							if strings.HasPrefix(firstLine, "http") || strings.HasSuffix(firstLine, ".yaml") || strings.HasSuffix(firstLine, ".json") {
								refs = append(refs, OutgoingReference{
									ConsumerRepoUID: repoUID,
									ConsumerRepoID:  repoID,
									ConsumerPath:    relPath,
									ImportKey:       firstLine,
									Tier:            TierDerived,
									EvidenceType:    "generator_marker",
									Confidence:      0.8,
									ConfidenceBasis: "generator_marker_dir",
									DetectorName:    d.Name(),
								})
								continue
							}
						}
					}
				}

				// Generic marker
				refs = append(refs, OutgoingReference{
					ConsumerRepoUID: repoUID,
					ConsumerRepoID:  repoID,
					ConsumerPath:    relPath,
					ImportKey:       "",
					Tier:            TierDerived,
					EvidenceType:    "generator_marker",
					Confidence:      0.6,
					ConfidenceBasis: "generator_marker_dir_only",
					DetectorName:    d.Name(),
				})
			}
		}
	}

	return refs, nil
}

// findFilesNamed finds files with a specific name in a directory tree
func findFilesNamed(root, name string) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}

		// Skip hidden directories and common non-source directories
		if info.IsDir() {
			dirName := info.Name()
			if strings.HasPrefix(dirName, ".") || dirName == "node_modules" || dirName == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		if info.Name() == name {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}
