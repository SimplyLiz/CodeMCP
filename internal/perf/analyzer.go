package perf

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/SimplyLiz/CodeMCP/internal/config"
	"github.com/SimplyLiz/CodeMCP/internal/modules"
)

// importCouldReferTo returns true if any raw import string in imports looks
// like it is referencing targetFile. ScanFile returns unclassified import
// strings (e.g. "github.com/org/repo/internal/foo"), so we use heuristics:
// the import ends with the target's directory path or its path without
// extension. This handles module-style imports (Go, Java, Kotlin) and
// relative imports (TypeScript, Python).
func importCouldReferTo(imports []string, targetFile string) bool {
	dir := filepath.ToSlash(filepath.Dir(targetFile))
	noExt := strings.TrimSuffix(filepath.ToSlash(targetFile), filepath.Ext(targetFile))

	for _, imp := range imports {
		imp = filepath.ToSlash(imp)
		// Relative import resolves to the exact file (without extension).
		if strings.HasSuffix(imp, noExt) {
			return true
		}
		// Module-path import addresses the directory/package containing the file.
		if strings.HasSuffix(imp, "/"+dir) || imp == dir {
			return true
		}
		// Relative import like "./foo" or "../foo/bar" that resolves to the dir.
		if strings.HasSuffix(imp, "/"+filepath.Base(dir)) && strings.HasPrefix(imp, ".") {
			return true
		}
	}
	return false
}

// Analyzer detects hidden coupling and other structural performance issues.
type Analyzer struct {
	repoRoot      string
	importScanner *modules.ImportScanner
	logger        *slog.Logger
}

// NewAnalyzer creates an Analyzer using the default import scan config.
func NewAnalyzer(repoRoot string, logger *slog.Logger) *Analyzer {
	cfg := &config.ImportScanConfig{
		Enabled:          true,
		MaxFileSizeBytes: 1_000_000,
	}
	return &Analyzer{
		repoRoot:      repoRoot,
		importScanner: modules.NewImportScanner(cfg, logger),
		logger:        logger,
	}
}

// Scan runs the performance scan and returns findings.
func (a *Analyzer) Scan(ctx context.Context, opts ScanOptions) (*PerfScanResult, error) {
	if opts.MinCorrelation <= 0 {
		opts.MinCorrelation = 0.3
	}
	if opts.MinCoChanges <= 0 {
		opts.MinCoChanges = 3
	}
	if opts.WindowDays <= 0 {
		opts.WindowDays = 365
	}
	if opts.Limit <= 0 {
		opts.Limit = 50
	}

	since := time.Now().AddDate(0, 0, -opts.WindowDays)

	a.logger.Debug("Starting perf scan",
		"scope", opts.Scope,
		"minCorrelation", opts.MinCorrelation,
		"minCoChanges", opts.MinCoChanges,
		"windowDays", opts.WindowDays,
	)

	// Step 1: collect co-change pairs from git history.
	pairCounts, fileTotals, err := a.buildCoChangePairs(ctx, since, opts.Scope)
	if err != nil {
		return nil, fmt.Errorf("building co-change pairs: %w", err)
	}

	// Step 2: filter by threshold and compute correlation.
	type candidate struct {
		a, b          string
		coChangeCount int
		correlation   float64
	}
	var candidates []candidate

	for pair, count := range pairCounts {
		if count < opts.MinCoChanges {
			continue
		}
		totalA := fileTotals[pair.a]
		totalB := fileTotals[pair.b]
		minTotal := totalA
		if totalB < minTotal {
			minTotal = totalB
		}
		if minTotal == 0 {
			continue
		}
		corr := float64(count) / float64(minTotal)
		if corr < opts.MinCorrelation {
			continue
		}
		candidates = append(candidates, candidate{
			a:             pair.a,
			b:             pair.b,
			coChangeCount: count,
			correlation:   corr,
		})
	}

	// Sort by correlation descending before the import-edge check (expensive).
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].correlation > candidates[j].correlation
	})

	// Step 3: filter out pairs that have a static import edge.
	// ScanFile returns raw import strings (not classified paths), so we use a
	// path-fragment heuristic: an import "references" a file if the import
	// string ends with the file's directory path or its path without extension.
	importEdgeCache := make(map[string][]string) // file -> raw import strings

	getRawImports := func(file string) []string {
		if cached, ok := importEdgeCache[file]; ok {
			return cached
		}
		absPath := filepath.Join(a.repoRoot, file)
		edges, err := a.importScanner.ScanFile(absPath, a.repoRoot)
		var raw []string
		if err == nil {
			for _, e := range edges {
				raw = append(raw, e.To)
			}
		}
		importEdgeCache[file] = raw
		return raw
	}

	var hidden []HiddenCouplingPair
	pairsChecked := 0

	for _, c := range candidates {
		if len(hidden) >= opts.Limit {
			break
		}
		pairsChecked++

		// Check A→B or B→A using path-fragment matching on raw import strings.
		if importCouldReferTo(getRawImports(c.a), c.b) ||
			importCouldReferTo(getRawImports(c.b), c.a) {
			continue // explained by static import — not hidden
		}

		level := correlationLevel(c.correlation)
		explanation := fmt.Sprintf(
			"%s and %s changed together in %d commits (%.0f%% of the time) "+
				"but neither file imports the other — likely sharing state or behavior through a third party",
			filepath.Base(c.a), filepath.Base(c.b),
			c.coChangeCount, c.correlation*100,
		)

		hidden = append(hidden, HiddenCouplingPair{
			FileA:         c.a,
			FileB:         c.b,
			Correlation:   c.correlation,
			CoChangeCount: c.coChangeCount,
			Level:         level,
			Explanation:   explanation,
		})
	}

	return &PerfScanResult{
		HiddenCoupling: hidden,
		Summary: PerfScanSummary{
			FilesObserved:    len(fileTotals),
			PairsChecked:     pairsChecked,
			HiddenPairsFound: len(hidden),
			AnalysisFrom:     since,
			AnalysisTo:       time.Now(),
		},
	}, nil
}

// filePair is an ordered (a <= b) pair used as a map key.
type filePair struct{ a, b string }

// defaultIgnorePrefixes are path prefixes that generate noise in hidden-coupling
// analysis. They change in sweeps (fixture updates, vendoring) that have nothing
// to do with behavioral coupling.
var defaultIgnorePrefixes = []string{
	"testdata/",
	"vendor/",
	"node_modules/",
	".ckb/",
}

func shouldIgnore(file string) bool {
	for _, prefix := range defaultIgnorePrefixes {
		if strings.HasPrefix(file, prefix) {
			return true
		}
	}
	return false
}

// buildCoChangePairs runs a single git log pass and builds co-change counts
// for all file pairs. Returns pairCounts and per-file commit totals.
func (a *Analyzer) buildCoChangePairs(ctx context.Context, since time.Time, scope []string) (map[filePair]int, map[string]int, error) {
	sinceStr := since.Format("2006-01-02")

	args := []string{
		"log",
		"--format=COMMIT %H",
		"--name-only",
		"--since=" + sinceStr,
		"--diff-filter=d", // exclude deleted files
		"--no-merges",
	}
	if len(scope) > 0 {
		args = append(args, "--")
		args = append(args, scope...)
	}

	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = a.repoRoot

	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, fmt.Errorf("git log pipe: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, fmt.Errorf("git log: %w", err)
	}

	pairCounts := make(map[filePair]int)
	fileTotals := make(map[string]int)

	// Reusable buffers — allocated once, cleared between commits.
	seen := make(map[string]bool, 32)
	var currentFiles []string

	// Stream git output line by line — avoids loading the full log into memory
	// and copying it to a string before splitting.
	commitPrefix := []byte("COMMIT ")
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		b := bytes.TrimRight(scanner.Bytes(), "\r")
		if bytes.HasPrefix(b, commitPrefix) {
			a.recordCommit(currentFiles, pairCounts, fileTotals, seen)
			currentFiles = currentFiles[:0]
			continue
		}
		if len(b) == 0 {
			continue
		}
		currentFiles = append(currentFiles, string(b))
	}
	if scanErr := scanner.Err(); scanErr != nil {
		_ = cmd.Wait()
		return nil, nil, fmt.Errorf("reading git log: %w", scanErr)
	}
	a.recordCommit(currentFiles, pairCounts, fileTotals, seen)

	if err := cmd.Wait(); err != nil {
		if s := stderrBuf.String(); s != "" {
			return nil, nil, fmt.Errorf("git log: %s", s)
		}
		return nil, nil, fmt.Errorf("git log: %w", err)
	}

	return pairCounts, fileTotals, nil
}

func (a *Analyzer) recordCommit(files []string, pairCounts map[filePair]int, fileTotals map[string]int, seen map[string]bool) {
	if len(files) == 0 {
		return
	}
	// Clear the reusable seen map from the previous commit.
	for k := range seen {
		delete(seen, k)
	}
	// Deduplicate and filter in a single pass, writing unique entries
	// back into the files slice (safe: we only write index j where j <= i).
	unique := files[:0]
	for _, f := range files {
		if shouldIgnore(f) {
			continue
		}
		if !seen[f] {
			seen[f] = true
			unique = append(unique, f)
			fileTotals[f]++
		}
	}
	// Build all pairs (order-independent key).
	for i := 0; i < len(unique); i++ {
		for j := i + 1; j < len(unique); j++ {
			fa, fb := unique[i], unique[j]
			if fa > fb {
				fa, fb = fb, fa
			}
			pairCounts[filePair{fa, fb}]++
		}
	}
}

func correlationLevel(c float64) string {
	switch {
	case c >= 0.8:
		return "high"
	case c >= 0.5:
		return "medium"
	default:
		return "low"
	}
}
