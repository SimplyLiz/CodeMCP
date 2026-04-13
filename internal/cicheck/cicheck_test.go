package cicheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// repoRoot walks up from the current directory looking for go.mod to find the
// repository root. This makes the tests work regardless of where `go test` is
// invoked from.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root (no go.mod found in any parent)")
		}
		dir = parent
	}
}

// workflowFiles returns all .yml files under .github/workflows/.
func workflowFiles(t *testing.T) []string {
	t.Helper()
	root := repoRoot(t)
	pattern := filepath.Join(root, ".github", "workflows", "*.yml")
	files, err := filepath.Glob(pattern)
	if err != nil {
		t.Fatalf("filepath.Glob(%q): %v", pattern, err)
	}
	if len(files) == 0 {
		t.Fatalf("no workflow files found at %s", pattern)
	}
	return files
}

// usesLine represents a single `uses:` reference found in a workflow file.
type usesLine struct {
	file    string
	lineNum int
	value   string // the full string after "uses:"
}

// parseUsesLines extracts all `uses:` references from workflow files, excluding
// local (./) and docker:// references.
func parseUsesLines(t *testing.T, files []string) []usesLine {
	t.Helper()
	// Matches lines like:  uses: actions/checkout@abc123 # v6
	usesRe := regexp.MustCompile(`^\s*uses:\s*(.+)$`)
	var results []usesLine
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			m := usesRe.FindStringSubmatch(line)
			if m == nil {
				continue
			}
			val := strings.TrimSpace(m[1])
			// Skip local actions and docker references.
			if strings.HasPrefix(val, "./") || strings.HasPrefix(val, "docker://") {
				continue
			}
			results = append(results, usesLine{
				file:    f,
				lineNum: i + 1,
				value:   val,
			})
		}
	}
	return results
}

// shaRe matches a 40-character hex SHA pinned after @.
var shaRe = regexp.MustCompile(`@([0-9a-f]{40})\b`)

func TestWorkflowActionsPinned(t *testing.T) {
	t.Parallel()
	files := workflowFiles(t)
	uses := parseUsesLines(t, files)
	for _, u := range uses {
		rel := filepath.Base(u.file)
		t.Run(fmt.Sprintf("%s/line%d", rel, u.lineNum), func(t *testing.T) {
			if !shaRe.MatchString(u.value) {
				t.Errorf("unpinned action at %s:%d\n  uses: %s\n  expected a 40-char SHA pin (e.g. @abc123...)", u.file, u.lineNum, u.value)
			}
		})
	}
}

// versionCommentRe matches a version comment like "# v6" or "# 0.33.1" after the SHA.
var versionCommentRe = regexp.MustCompile(`@[0-9a-f]{40}\s+#\s*v?\d`)

func TestWorkflowActionsVersionComments(t *testing.T) {
	t.Parallel()
	files := workflowFiles(t)
	uses := parseUsesLines(t, files)
	for _, u := range uses {
		rel := filepath.Base(u.file)
		t.Run(fmt.Sprintf("%s/line%d", rel, u.lineNum), func(t *testing.T) {
			// Only check if already SHA-pinned.
			if !shaRe.MatchString(u.value) {
				t.Skipf("not SHA-pinned, skipping version comment check")
			}
			if !versionCommentRe.MatchString(u.value) {
				t.Errorf("missing version comment at %s:%d\n  uses: %s\n  expected a comment like '# v6' after the SHA for maintainability", u.file, u.lineNum, u.value)
			}
		})
	}
}

func TestWorkflowJobsHaveTimeout(t *testing.T) {
	t.Parallel()
	files := workflowFiles(t)
	// Simple state-machine parser: track current job name and whether
	// timeout-minutes was seen before the next job or end of file.
	// Jobs that use reusable workflows (job-level `uses:`) are exempt
	// because the called workflow defines its own timeouts.
	jobRe := regexp.MustCompile(`^  (\w[\w-]*):\s*$`)
	timeoutRe := regexp.MustCompile(`^\s+timeout-minutes:`)
	// Job-level uses: (indent of 4 spaces = direct child of job key).
	jobUsesRe := regexp.MustCompile(`^    uses:\s+`)

	for _, f := range files {
		rel := filepath.Base(f)
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		lines := strings.Split(string(data), "\n")

		inJobs := false
		var currentJob string
		hasTimeout := false
		isReusable := false

		checkJob := func() {
			if currentJob != "" && !hasTimeout && !isReusable {
				t.Errorf("%s: job %q is missing timeout-minutes", rel, currentJob)
			}
		}

		for _, line := range lines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "jobs:" {
				inJobs = true
				continue
			}
			if !inJobs {
				continue
			}

			if m := jobRe.FindStringSubmatch(line); m != nil {
				// Found a new job definition; check the previous one.
				checkJob()
				currentJob = m[1]
				hasTimeout = false
				isReusable = false
				continue
			}

			if currentJob != "" {
				if timeoutRe.MatchString(line) {
					hasTimeout = true
				}
				if jobUsesRe.MatchString(line) {
					isReusable = true
				}
			}
		}
		// Check the last job in the file.
		checkJob()
	}
}

func TestWorkflowNoDirectInputInterpolation(t *testing.T) {
	t.Parallel()
	files := workflowFiles(t)
	// Dangerous patterns when used directly in run: shell blocks.
	dangerousPatterns := []string{
		`${{ inputs.`,
		`${{ github.head_ref`,
		`${{ github.base_ref`,
		`${{ github.event.pull_request.title`,
		`${{ github.event.pull_request.body`,
		`${{ github.event.comment.body`,
	}

	// Lines where interpolation is safe (not shell context).
	safeKeyRe := regexp.MustCompile(`^\s*(if|with|uses|env|id|name)\s*:`)

	for _, f := range files {
		rel := filepath.Base(f)
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		lines := strings.Split(string(data), "\n")

		inRun := false
		for i, line := range lines {
			trimmed := strings.TrimSpace(line)

			// Detect `run:` or `run: |` blocks.
			if strings.HasPrefix(trimmed, "run:") {
				inRun = true
				// The run: line itself is a shell context — check it too
				// unless it's just "run: |".
				if trimmed == "run: |" || trimmed == "run: >" {
					continue
				}
			}

			// If the line is a new YAML key at step level, we leave run context.
			if inRun && !strings.HasPrefix(trimmed, "run:") {
				// If indent decreases to step-key level or is a new key, stop.
				if len(line) > 0 && len(line)-len(strings.TrimLeft(line, " ")) <= 8 && strings.Contains(trimmed, ":") && !strings.HasPrefix(trimmed, "#") && !strings.HasPrefix(trimmed, "-") {
					if safeKeyRe.MatchString(line) || regexp.MustCompile(`^\s+\w[\w-]*:`).MatchString(line) {
						// Check if we're at a YAML key that's NOT a continuation of run block
						indent := len(line) - len(strings.TrimLeft(line, " "))
						if indent <= 10 {
							inRun = false
						}
					}
				}
			}

			// Skip safe YAML keys.
			if safeKeyRe.MatchString(line) {
				continue
			}

			// Only flag dangerous patterns inside run: blocks (shell context).
			if !inRun {
				continue
			}

			for _, pattern := range dangerousPatterns {
				if strings.Contains(line, pattern) {
					t.Errorf("potential script injection at %s:%d\n  %s\n  found %q in a run: block — use an env: variable instead",
						rel, i+1, strings.TrimSpace(line), pattern)
				}
			}
		}
	}
}

func TestWorkflowNoLatestDockerTag(t *testing.T) {
	t.Parallel()
	files := workflowFiles(t)
	dockerUsesRe := regexp.MustCompile(`^\s*uses:\s*docker://`)

	for _, f := range files {
		rel := filepath.Base(f)
		data, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("reading %s: %v", f, err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if !dockerUsesRe.MatchString(line) {
				continue
			}
			if strings.Contains(line, ":latest") {
				t.Errorf("%s:%d uses docker :latest tag — pin to a specific version\n  %s",
					rel, i+1, strings.TrimSpace(line))
			}
		}
	}
}

func TestWorkflowConsistentActionVersions(t *testing.T) {
	t.Parallel()
	files := workflowFiles(t)
	uses := parseUsesLines(t, files)

	// actionRef splits "actions/checkout@abc123 # v6" into
	// name="actions/checkout" and version="abc123".
	actionRefRe := regexp.MustCompile(`^([^@]+)@(\S+)`)

	// Map action name -> map[sha] -> list of locations.
	type location struct {
		file string
		line int
	}
	actionVersions := make(map[string]map[string][]location)

	for _, u := range uses {
		m := actionRefRe.FindStringSubmatch(u.value)
		if m == nil {
			continue
		}
		name := m[1]
		version := m[2]
		if actionVersions[name] == nil {
			actionVersions[name] = make(map[string][]location)
		}
		actionVersions[name][version] = append(actionVersions[name][version], location{
			file: filepath.Base(u.file),
			line: u.lineNum,
		})
	}

	for action, versions := range actionVersions {
		if len(versions) <= 1 {
			continue
		}
		t.Run(strings.ReplaceAll(action, "/", "_"), func(t *testing.T) {
			var details []string
			for ver, locs := range versions {
				var locStrs []string
				for _, l := range locs {
					locStrs = append(locStrs, fmt.Sprintf("%s:%d", l.file, l.line))
				}
				details = append(details, fmt.Sprintf("  %s used at: %s", ver, strings.Join(locStrs, ", ")))
			}
			t.Errorf("action %q is used with %d different versions across workflow files:\n%s",
				action, len(versions), strings.Join(details, "\n"))
		})
	}
}
