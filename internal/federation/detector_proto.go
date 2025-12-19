package federation

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ProtoDetector detects protobuf contracts and their consumers
type ProtoDetector struct{}

// NewProtoDetector creates a new proto detector
func NewProtoDetector() *ProtoDetector {
	return &ProtoDetector{}
}

// Name returns the detector name
func (d *ProtoDetector) Name() string {
	return "proto"
}

// Proto file patterns
var (
	protoPackageRegex = regexp.MustCompile(`^package\s+([\w.]+)\s*;`)
	protoImportRegex  = regexp.MustCompile(`^import\s+(?:public\s+)?"([^"]+)"\s*;`)
	protoServiceRegex = regexp.MustCompile(`^service\s+(\w+)\s*\{`)
	protoOptionRegex  = regexp.MustCompile(`^option\s+(\w+)\s*=`)

	// Visibility classification patterns
	publicPathRoots    = []string{"proto/", "protos/", "api/", "idl/", "schemas/", "contracts/"}
	internalPathParts  = []string{"internal/", "testdata/", "examples/", "tmp/", "vendor/"}
	versionedPkgRegex  = regexp.MustCompile(`\.(v\d+|v\d+alpha\d*|v\d+beta\d*)$`)
	internalPkgRegex   = regexp.MustCompile(`\.internal\.|\.private\.|\.test\.`)
	testProtoFileRegex = regexp.MustCompile(`(?:_test\.proto|test_.*\.proto)$`)

	// Generated code patterns for consumer detection
	goGeneratedSourceRegex = regexp.MustCompile(`//\s*source:\s*(.+\.proto)`)
	tsGeneratedSourceRegex = regexp.MustCompile(`@generated from (.+\.proto)`)
)

// Detect scans a repository for proto contracts and consumers
func (d *ProtoDetector) Detect(repoPath string, repoUID string, repoID string) (*DetectorResult, error) {
	result := &DetectorResult{
		Contracts:    []Contract{},
		References:   []OutgoingReference{},
		ProtoImports: []ProtoImport{},
	}

	// Find all .proto files
	protoFiles, err := findFiles(repoPath, "*.proto")
	if err != nil {
		return result, err
	}

	// Parse each proto file
	for _, protoPath := range protoFiles {
		relPath, relErr := filepath.Rel(repoPath, protoPath)
		if relErr != nil {
			continue
		}

		contract, imports, parseErr := d.parseProtoFile(protoPath, relPath, repoUID, repoID)
		if parseErr != nil {
			continue
		}

		result.Contracts = append(result.Contracts, *contract)

		// Record imports as outgoing references
		for _, imp := range imports {
			result.References = append(result.References, OutgoingReference{
				ConsumerRepoUID: repoUID,
				ConsumerRepoID:  repoID,
				ConsumerPath:    relPath,
				ImportKey:       imp,
				Tier:            TierDeclared,
				EvidenceType:    "proto_import",
				Confidence:      1.0,
				ConfidenceBasis: "explicit_import",
				DetectorName:    d.Name(),
			})
		}
	}

	// Find generated code (consumers)
	generatedRefs, err := d.findGeneratedCodeConsumers(repoPath, repoUID, repoID)
	if err == nil {
		result.References = append(result.References, generatedRefs...)
	}

	// Find buf.yaml dependencies
	bufRefs, err := d.findBufDependencies(repoPath, repoUID, repoID)
	if err == nil {
		result.References = append(result.References, bufRefs...)
	}

	return result, nil
}

// parseProtoFile parses a proto file and extracts metadata
func (d *ProtoDetector) parseProtoFile(filePath, relPath, repoUID, repoID string) (*Contract, []string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = file.Close() }()

	metadata := ProtoMetadata{
		Imports:  []string{},
		Services: []string{},
		Options:  []string{},
	}

	var imports []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments
		if strings.HasPrefix(line, "//") || strings.HasPrefix(line, "/*") {
			continue
		}

		// Package
		if match := protoPackageRegex.FindStringSubmatch(line); match != nil {
			metadata.PackageName = match[1]
		}

		// Imports
		if match := protoImportRegex.FindStringSubmatch(line); match != nil {
			imports = append(imports, match[1])
			metadata.Imports = append(metadata.Imports, match[1])
		}

		// Services
		if match := protoServiceRegex.FindStringSubmatch(line); match != nil {
			metadata.Services = append(metadata.Services, match[1])
		}

		// Options
		if match := protoOptionRegex.FindStringSubmatch(line); match != nil {
			metadata.Options = append(metadata.Options, match[1])
		}
	}

	// Classify visibility
	visibility, visibilityBasis, confidence := d.classifyVisibility(relPath, metadata)

	// Generate import keys
	importKeys := d.generateImportKeys(relPath, metadata)

	metadataJSON, _ := json.Marshal(metadata)

	contract := &Contract{
		ID:              fmt.Sprintf("%s:%s", repoUID, relPath),
		RepoUID:         repoUID,
		RepoID:          repoID,
		Path:            relPath,
		ContractType:    ContractTypeProto,
		Metadata:        metadataJSON,
		Visibility:      visibility,
		VisibilityBasis: visibilityBasis,
		Confidence:      confidence,
		ImportKeys:      importKeys,
		IndexedAt:       time.Now(),
	}

	return contract, imports, nil
}

// classifyVisibility classifies the visibility of a proto file
func (d *ProtoDetector) classifyVisibility(path string, metadata ProtoMetadata) (Visibility, string, float64) {
	// Check for internal patterns first
	for _, internal := range internalPathParts {
		if strings.Contains(path, internal) {
			return VisibilityInternal, "path_contains_" + strings.TrimSuffix(internal, "/"), 0.9
		}
	}

	if testProtoFileRegex.MatchString(path) {
		return VisibilityInternal, "test_file_pattern", 0.9
	}

	if internalPkgRegex.MatchString(metadata.PackageName) {
		return VisibilityInternal, "internal_package", 0.9
	}

	// Check for public patterns
	for _, root := range publicPathRoots {
		if strings.HasPrefix(path, root) {
			return VisibilityPublic, "path_root_" + strings.TrimSuffix(root, "/"), 1.0
		}
	}

	if versionedPkgRegex.MatchString(metadata.PackageName) {
		return VisibilityPublic, "versioned_package", 0.9
	}

	if len(metadata.Services) > 0 {
		return VisibilityPublic, "has_service_definition", 0.8
	}

	return VisibilityUnknown, "no_clear_pattern", 0.5
}

// generateImportKeys generates import keys for a proto file
func (d *ProtoDetector) generateImportKeys(path string, metadata ProtoMetadata) []string {
	keys := []string{}

	// Exact relative path
	keys = append(keys, path)

	// Without common roots
	for _, root := range publicPathRoots {
		if strings.HasPrefix(path, root) {
			keys = append(keys, strings.TrimPrefix(path, root))
		}
	}

	// Package-based key
	if metadata.PackageName != "" {
		pkgPath := strings.ReplaceAll(metadata.PackageName, ".", "/") + ".proto"
		keys = append(keys, pkgPath)

		// Also add the path with the filename from the original
		base := filepath.Base(path)
		pkgDir := strings.ReplaceAll(metadata.PackageName, ".", "/")
		keys = append(keys, pkgDir+"/"+base)
	}

	return keys
}

// findGeneratedCodeConsumers finds generated proto code in a repo
func (d *ProtoDetector) findGeneratedCodeConsumers(repoPath, repoUID, repoID string) ([]OutgoingReference, error) {
	var refs []OutgoingReference

	// Find Go generated files (*.pb.go)
	goFiles, goErr := findFiles(repoPath, "*.pb.go")
	if goErr == nil {
		for _, goFile := range goFiles {
			ref, parseErr := d.parseGoGeneratedFile(goFile, repoPath, repoUID, repoID)
			if parseErr == nil && ref != nil {
				refs = append(refs, *ref)
			}
		}
	}

	// Find TypeScript generated files (*.pb.ts, *_pb.js)
	tsFiles, tsErr := findFiles(repoPath, "*.pb.ts")
	if tsErr == nil {
		for _, tsFile := range tsFiles {
			ref, parseErr := d.parseTSGeneratedFile(tsFile, repoPath, repoUID, repoID)
			if parseErr == nil && ref != nil {
				refs = append(refs, *ref)
			}
		}
	}

	jsFiles, err := findFiles(repoPath, "*_pb.js")
	if err == nil {
		for _, jsFile := range jsFiles {
			ref, err := d.parseTSGeneratedFile(jsFile, repoPath, repoUID, repoID)
			if err == nil && ref != nil {
				refs = append(refs, *ref)
			}
		}
	}

	return refs, nil
}

// parseGoGeneratedFile extracts proto source from Go generated file
func (d *ProtoDetector) parseGoGeneratedFile(filePath, repoPath, repoUID, repoID string) (*OutgoingReference, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	relPath, _ := filepath.Rel(repoPath, filePath)

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() && lineCount < 50 { // Only check first 50 lines
		line := scanner.Text()
		lineCount++

		if match := goGeneratedSourceRegex.FindStringSubmatch(line); match != nil {
			return &OutgoingReference{
				ConsumerRepoUID: repoUID,
				ConsumerRepoID:  repoID,
				ConsumerPath:    relPath,
				ImportKey:       match[1],
				Tier:            TierDerived,
				EvidenceType:    "generated_code",
				Confidence:      0.85,
				ConfidenceBasis: "go_proto_source_comment",
				DetectorName:    d.Name(),
			}, nil
		}
	}

	return nil, nil
}

// parseTSGeneratedFile extracts proto source from TypeScript generated file
func (d *ProtoDetector) parseTSGeneratedFile(filePath, repoPath, repoUID, repoID string) (*OutgoingReference, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()

	relPath, _ := filepath.Rel(repoPath, filePath)

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() && lineCount < 50 {
		line := scanner.Text()
		lineCount++

		if match := tsGeneratedSourceRegex.FindStringSubmatch(line); match != nil {
			return &OutgoingReference{
				ConsumerRepoUID: repoUID,
				ConsumerRepoID:  repoID,
				ConsumerPath:    relPath,
				ImportKey:       match[1],
				Tier:            TierDerived,
				EvidenceType:    "generated_code",
				Confidence:      0.85,
				ConfidenceBasis: "ts_proto_source_comment",
				DetectorName:    d.Name(),
			}, nil
		}
	}

	return nil, nil
}

// findBufDependencies finds dependencies from buf.yaml
func (d *ProtoDetector) findBufDependencies(repoPath, repoUID, repoID string) ([]OutgoingReference, error) {
	var refs []OutgoingReference

	bufYamlPath := filepath.Join(repoPath, "buf.yaml")
	if _, err := os.Stat(bufYamlPath); os.IsNotExist(err) {
		return refs, nil
	}

	// Simple YAML parsing for deps field
	file, err := os.Open(bufYamlPath)
	if err != nil {
		return refs, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	inDeps := false
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "deps:") {
			inDeps = true
			continue
		}

		if inDeps {
			// Check for continuation
			if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "-") {
				inDeps = false
				continue
			}

			if strings.HasPrefix(trimmed, "-") {
				dep := strings.TrimPrefix(trimmed, "-")
				dep = strings.TrimSpace(dep)
				if dep != "" {
					refs = append(refs, OutgoingReference{
						ConsumerRepoUID: repoUID,
						ConsumerRepoID:  repoID,
						ConsumerPath:    "buf.yaml",
						ImportKey:       dep,
						Tier:            TierDeclared,
						EvidenceType:    "buf_dependency",
						Confidence:      1.0,
						ConfidenceBasis: "buf_yaml_deps",
						DetectorName:    d.Name(),
					})
				}
			}
		}
	}

	return refs, nil
}

// findFiles finds files matching a pattern in a directory
func findFiles(root, pattern string) ([]string, error) {
	var files []string

	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil //nolint:nilerr // skip inaccessible files
		}

		// Skip hidden directories and common non-source directories
		if info.IsDir() {
			name := info.Name()
			if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}

		matched, err := filepath.Match(pattern, info.Name())
		if err != nil {
			return nil //nolint:nilerr // skip invalid pattern matches
		}

		if matched {
			files = append(files, path)
		}

		return nil
	})

	return files, err
}
