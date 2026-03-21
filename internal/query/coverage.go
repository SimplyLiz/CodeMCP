package query

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// defaultCoveragePaths lists the standard locations to search for coverage reports.
var defaultCoveragePaths = []string{
	".ckb/coverage.lcov",
	"coverage.lcov",
	"coverage/lcov.info",
	"coverage.xml",
	"coverage/cobertura.xml",
}

// lcovSFRe matches LCOV source file records.
var lcovSFRe = regexp.MustCompile(`^SF:(.+)$`)

// lcovLFRe matches LCOV lines found records.
var lcovLFRe = regexp.MustCompile(`^LF:(\d+)$`)

// lcovLHRe matches LCOV lines hit records.
var lcovLHRe = regexp.MustCompile(`^LH:(\d+)$`)

// coberturaLineRateRe matches Cobertura file-level line-rate attributes.
var coberturaLineRateRe = regexp.MustCompile(`<class[^>]+filename="([^"]+)"[^>]+line-rate="([^"]+)"`)

// loadCoverageReport searches for a coverage file in the repo and returns
// a map of relative file path → coverage percentage (0.0-100.0).
// Returns nil if no coverage file is found.
func loadCoverageReport(repoRoot string, customPaths []string) map[string]float64 {
	paths := append(customPaths, defaultCoveragePaths...)
	for _, p := range paths {
		absPath := filepath.Join(repoRoot, p)
		if _, err := os.Stat(absPath); err != nil {
			continue
		}
		if strings.HasSuffix(p, ".lcov") || strings.HasSuffix(p, "lcov.info") {
			return parseLCOV(absPath, repoRoot)
		}
		if strings.HasSuffix(p, ".xml") {
			return parseCobertura(absPath, repoRoot)
		}
	}
	return nil
}

// parseLCOV parses an LCOV format coverage file.
func parseLCOV(path, repoRoot string) map[string]float64 {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	result := make(map[string]float64)
	scanner := bufio.NewScanner(f)
	var currentFile string
	var linesFound, linesHit int

	for scanner.Scan() {
		line := scanner.Text()

		if m := lcovSFRe.FindStringSubmatch(line); m != nil {
			// Emit previous file if we have one
			if currentFile != "" && linesFound > 0 {
				result[currentFile] = float64(linesHit) / float64(linesFound) * 100
			}
			currentFile = relativizePath(m[1], repoRoot)
			linesFound = 0
			linesHit = 0
			continue
		}

		if m := lcovLFRe.FindStringSubmatch(line); m != nil {
			linesFound, _ = strconv.Atoi(m[1])
			continue
		}

		if m := lcovLHRe.FindStringSubmatch(line); m != nil {
			linesHit, _ = strconv.Atoi(m[1])
			continue
		}

		if line == "end_of_record" {
			if currentFile != "" && linesFound > 0 {
				result[currentFile] = float64(linesHit) / float64(linesFound) * 100
			}
			currentFile = ""
			linesFound = 0
			linesHit = 0
		}
	}

	// Handle last record
	if currentFile != "" && linesFound > 0 {
		result[currentFile] = float64(linesHit) / float64(linesFound) * 100
	}

	return result
}

// parseCobertura parses a Cobertura XML coverage file (simple regex, not full XML).
func parseCobertura(path, repoRoot string) map[string]float64 {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil
	}

	result := make(map[string]float64)
	matches := coberturaLineRateRe.FindAllStringSubmatch(string(content), -1)
	for _, m := range matches {
		file := relativizePath(m[1], repoRoot)
		rate, err := strconv.ParseFloat(m[2], 64)
		if err == nil {
			result[file] = rate * 100
		}
	}
	return result
}

// relativizePath converts an absolute path to a path relative to repoRoot.
func relativizePath(path, repoRoot string) string {
	rel, err := filepath.Rel(repoRoot, path)
	if err != nil {
		return path
	}
	// If the path was already relative, filepath.Rel might produce ../.. paths
	if strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
