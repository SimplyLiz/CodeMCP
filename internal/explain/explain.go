package explain

import (
	"context"
	"log/slog"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"ckb/internal/coupling"
)

// Explainer provides symbol explanation functionality
type Explainer struct {
	repoRoot         string
	logger           *slog.Logger
	couplingAnalyzer *coupling.Analyzer
}

// NewExplainer creates a new symbol explainer
func NewExplainer(repoRoot string, logger *slog.Logger) *Explainer {
	return &Explainer{
		repoRoot:         repoRoot,
		logger:           logger,
		couplingAnalyzer: coupling.NewAnalyzer(repoRoot, logger),
	}
}

// Explain generates a full explanation for a symbol
func (e *Explainer) Explain(ctx context.Context, opts ExplainOptions) (*SymbolExplanation, error) {
	// Set defaults
	if opts.RepoRoot == "" {
		opts.RepoRoot = e.repoRoot
	}
	if opts.HistoryLimit <= 0 {
		opts.HistoryLimit = 10
	}

	// Parse the symbol query (could be file:line or just a file path for now)
	filePath, line := e.parseSymbolQuery(opts.Symbol)

	e.logger.Debug("Starting symbol explanation",
		"symbol", opts.Symbol,
		"filePath", filePath,
		"line", line)

	// Get file history
	commits, err := e.getFileHistory(ctx, filePath)
	if err != nil {
		return nil, err
	}

	if len(commits) == 0 {
		return &SymbolExplanation{
			Symbol: filepath.Base(filePath),
			File:   filePath,
			Origin: Origin{
				CommitMessage: "No history found",
			},
			Warnings: []Warning{
				{
					Type:     WarningStale,
					Message:  "No commit history found for this file",
					Severity: SeverityWarning,
				},
			},
		}, nil
	}

	// Find origin (first commit that introduced the file)
	origin := e.findOrigin(commits)

	// Build evolution timeline
	evolution := e.buildEvolution(commits, opts.HistoryLimit)

	// Extract references from commit messages
	references := e.extractReferences(commits)

	// Get co-change patterns (if enabled)
	var coChanges []CoChange
	if opts.IncludeCoChange {
		couplingResult, err := e.couplingAnalyzer.Analyze(ctx, coupling.AnalyzeOptions{
			RepoRoot:       opts.RepoRoot,
			Target:         filePath,
			MinCorrelation: 0.3,
			WindowDays:     365,
			Limit:          10,
		})
		if err == nil && couplingResult != nil {
			for _, c := range couplingResult.Correlations {
				coChanges = append(coChanges, CoChange{
					File:          c.File,
					Correlation:   c.Correlation,
					CoChangeCount: c.CoChangeCount,
					TotalChanges:  c.TotalChanges,
				})
			}
		}
	}

	// Generate warnings
	warnings := e.analyzeWarnings(origin, evolution, coChanges)

	// Build ownership info from evolution
	ownership := e.buildOwnership(evolution)

	return &SymbolExplanation{
		Symbol:           filepath.Base(filePath),
		File:             filePath,
		Line:             line,
		Origin:           origin,
		Evolution:        evolution,
		Ownership:        ownership,
		CoChangePatterns: coChanges,
		References:       references,
		Warnings:         warnings,
	}, nil
}

// parseSymbolQuery parses a symbol query which could be file:line or just a file path
func (e *Explainer) parseSymbolQuery(query string) (filePath string, line int) {
	// Check for file:line format
	parts := strings.Split(query, ":")
	if len(parts) >= 2 {
		filePath = parts[0]
		// Try to parse line number
		if lineNum, err := parseInt(parts[1]); err == nil && lineNum > 0 {
			line = lineNum
		}
	} else {
		filePath = query
	}
	return
}

// getFileHistory gets the commit history for a file
func (e *Explainer) getFileHistory(ctx context.Context, filePath string) ([]commitInfo, error) {
	// Format: hash|author|email|timestamp|subject
	args := []string{
		"log",
		"--format=%H|%an|%ae|%aI|%s",
		"--follow",
		"--",
		filePath,
	}

	output, err := e.executeGit(ctx, args...)
	if err != nil {
		return nil, err
	}

	if output == "" {
		return []commitInfo{}, nil
	}

	lines := strings.Split(strings.TrimSpace(output), "\n")
	commits := make([]commitInfo, 0, len(lines))

	for _, line := range lines {
		parts := strings.SplitN(line, "|", 5)
		if len(parts) != 5 {
			continue
		}

		timestamp, _ := time.Parse(time.RFC3339, parts[3])
		commits = append(commits, commitInfo{
			Hash:      parts[0],
			Author:    parts[1],
			Email:     parts[2],
			Timestamp: timestamp,
			Message:   parts[4],
		})
	}

	return commits, nil
}

// commitInfo holds information about a single commit
type commitInfo struct {
	Hash      string
	Author    string
	Email     string
	Timestamp time.Time
	Message   string
}

// findOrigin finds the origin commit (first commit that introduced the file)
func (e *Explainer) findOrigin(commits []commitInfo) Origin {
	if len(commits) == 0 {
		return Origin{}
	}

	// The last commit in the list is the oldest (origin)
	origin := commits[len(commits)-1]

	return Origin{
		Author:        origin.Author,
		AuthorEmail:   origin.Email,
		Date:          origin.Timestamp,
		CommitSha:     origin.Hash,
		CommitMessage: origin.Message,
	}
}

// buildEvolution builds the evolution timeline from commits
func (e *Explainer) buildEvolution(commits []commitInfo, limit int) Evolution {
	if len(commits) == 0 {
		return Evolution{}
	}

	// Build contributor map
	contributorMap := make(map[string]*Contributor)
	for _, c := range commits {
		key := c.Author
		if contrib, exists := contributorMap[key]; exists {
			contrib.CommitCount++
			if c.Timestamp.Before(contrib.FirstCommit) {
				contrib.FirstCommit = c.Timestamp
			}
			if c.Timestamp.After(contrib.LastCommit) {
				contrib.LastCommit = c.Timestamp
			}
		} else {
			contributorMap[key] = &Contributor{
				Name:        c.Author,
				Email:       c.Email,
				CommitCount: 1,
				FirstCommit: c.Timestamp,
				LastCommit:  c.Timestamp,
			}
		}
	}

	// Convert to slice and sort by commit count
	contributors := make([]Contributor, 0, len(contributorMap))
	for _, contrib := range contributorMap {
		contributors = append(contributors, *contrib)
	}
	sort.Slice(contributors, func(i, j int) bool {
		return contributors[i].CommitCount > contributors[j].CommitCount
	})

	// Build timeline (most recent first)
	timeline := make([]TimelineEntry, 0, limit)
	for i, c := range commits {
		if i >= limit {
			break
		}
		timeline = append(timeline, TimelineEntry{
			Date:    c.Timestamp,
			Author:  c.Author,
			Message: c.Message,
			Sha:     c.Hash[:8], // short sha
		})
	}

	return Evolution{
		TotalCommits:  len(commits),
		Contributors:  contributors,
		LastTouched:   commits[0].Timestamp,
		LastTouchedBy: commits[0].Author,
		Timeline:      timeline,
	}
}

// extractReferences extracts issue/PR references from commit messages
func (e *Explainer) extractReferences(commits []commitInfo) References {
	refs := References{}

	issuePattern := regexp.MustCompile(`#(\d+)`)
	prPattern := regexp.MustCompile(`(?i)PR\s*#?(\d+)`)
	jiraPattern := regexp.MustCompile(`[A-Z][A-Z0-9]+-\d+`)

	issueSet := make(map[string]bool)
	prSet := make(map[string]bool)
	jiraSet := make(map[string]bool)

	for _, c := range commits {
		// Extract issues
		matches := issuePattern.FindAllStringSubmatch(c.Message, -1)
		for _, m := range matches {
			issueSet["#"+m[1]] = true
		}

		// Extract PRs
		prMatches := prPattern.FindAllStringSubmatch(c.Message, -1)
		for _, m := range prMatches {
			prSet["#"+m[1]] = true
		}

		// Extract JIRA tickets
		jiraMatches := jiraPattern.FindAllString(c.Message, -1)
		for _, m := range jiraMatches {
			jiraSet[m] = true
		}
	}

	// Convert sets to slices
	for issue := range issueSet {
		refs.Issues = append(refs.Issues, issue)
	}
	for pr := range prSet {
		refs.PRs = append(refs.PRs, pr)
	}
	for jira := range jiraSet {
		refs.JiraTickets = append(refs.JiraTickets, jira)
	}

	// Sort for consistent output
	sort.Strings(refs.Issues)
	sort.Strings(refs.PRs)
	sort.Strings(refs.JiraTickets)

	return refs
}

// analyzeWarnings generates warnings based on the analysis
func (e *Explainer) analyzeWarnings(origin Origin, evolution Evolution, coChanges []CoChange) []Warning {
	var warnings []Warning

	// Check for "temporary" code that stuck around
	tempKeywords := []string{"temp", "temporary", "hack", "fixme", "todo", "remove after", "workaround"}
	msgLower := strings.ToLower(origin.CommitMessage)
	for _, kw := range tempKeywords {
		if strings.Contains(msgLower, kw) {
			ageMonths := monthsSince(origin.Date)
			if ageMonths > 3 {
				warnings = append(warnings, Warning{
					Type:     WarningTemporaryCode,
					Message:  "Original intent was \"" + kw + "\" but code is " + formatAge(origin.Date) + " old",
					Severity: SeverityWarning,
				})
			}
			break
		}
	}

	// Check bus factor
	oneYearAgo := time.Now().AddDate(-1, 0, 0)
	recentContributors := 0
	var activeContributor string
	for _, c := range evolution.Contributors {
		if c.LastCommit.After(oneYearAgo) {
			recentContributors++
			if activeContributor == "" {
				activeContributor = c.Name
			}
		}
	}
	if recentContributors <= 1 {
		msg := "Bus factor: " + itoa(recentContributors)
		if activeContributor != "" {
			msg += " (only " + activeContributor + " active in past year)"
		} else {
			msg += " (nobody active in past year)"
		}
		warnings = append(warnings, Warning{
			Type:     WarningBusFactor,
			Message:  msg,
			Severity: SeverityWarning,
		})
	}

	// Check high coupling
	highCorrelation := 0
	for _, c := range coChanges {
		if c.Correlation >= 0.7 {
			highCorrelation++
		}
	}
	if highCorrelation >= 3 {
		warnings = append(warnings, Warning{
			Type:     WarningHighCoupling,
			Message:  "High co-change coupling with " + itoa(highCorrelation) + " other files",
			Severity: SeverityInfo,
		})
	}

	// Check staleness
	if !evolution.LastTouched.IsZero() {
		monthsStale := monthsSince(evolution.LastTouched)
		if monthsStale >= 12 {
			warnings = append(warnings, Warning{
				Type:     WarningStale,
				Message:  "Not touched in " + itoa(monthsStale) + " months",
				Severity: SeverityInfo,
			})
		}
	}

	return warnings
}

// buildOwnership builds ownership info from evolution data
func (e *Explainer) buildOwnership(evolution Evolution) OwnershipInfo {
	info := OwnershipInfo{}

	if len(evolution.Contributors) > 0 {
		// Primary contact is the most active contributor
		info.PrimaryContact = evolution.Contributors[0].Name
	}

	// Most recent contributor
	if evolution.LastTouchedBy != "" {
		info.CurrentOwner = evolution.LastTouchedBy
	}

	return info
}

// executeGit executes a git command and returns the output
func (e *Explainer) executeGit(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = e.repoRoot

	output, err := cmd.Output()
	if err != nil {
		return "", err
	}

	return string(output), nil
}

// Helper functions

func monthsSince(t time.Time) int {
	now := time.Now()
	years := now.Year() - t.Year()
	months := int(now.Month()) - int(t.Month())
	return years*12 + months
}

func formatAge(t time.Time) string {
	months := monthsSince(t)
	if months < 1 {
		return "less than a month"
	} else if months == 1 {
		return "1 month"
	} else if months < 12 {
		return itoa(months) + " months"
	} else if months < 24 {
		return "1 year"
	} else {
		return itoa(months/12) + " years"
	}
}

func itoa(i int) string {
	if i < 0 {
		return "-" + uitoa(uint(-i))
	}
	return uitoa(uint(i))
}

func uitoa(u uint) string {
	if u == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for u > 0 {
		i--
		buf[i] = byte('0' + u%10)
		u /= 10
	}
	return string(buf[i:])
}

func parseInt(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}

	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, nil
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}
