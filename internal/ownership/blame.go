package ownership

import (
	"bufio"
	"bytes"
	"math"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"
)

// BlameConfig contains configuration for git-blame ownership computation
type BlameConfig struct {
	// TimeDecayHalfLife is the number of days for time decay (default: 90)
	TimeDecayHalfLife int

	// ExcludeBots indicates whether to exclude bot commits
	ExcludeBots bool

	// BotPatterns are regex patterns to detect bot authors
	BotPatterns []string

	// MinContribution is the minimum percentage to be considered a contributor
	MinContribution float64
}

// DefaultBlameConfig returns the default blame configuration
func DefaultBlameConfig() BlameConfig {
	return BlameConfig{
		TimeDecayHalfLife: 90,
		ExcludeBots:       true,
		BotPatterns: []string{
			`\[bot\]$`,
			`^dependabot`,
			`^renovate`,
			`^github-actions`,
		},
		MinContribution: 0.05, // 5%
	}
}

// BlameEntry represents a single line's blame information
type BlameEntry struct {
	CommitHash string
	Author     string
	AuthorMail string
	Timestamp  time.Time
	LineNumber int
}

// BlameResult contains the parsed blame information for a file
type BlameResult struct {
	FilePath string
	Entries  []BlameEntry
}

// AuthorContribution represents an author's contribution to a file
type AuthorContribution struct {
	Author        string    `json:"author"`
	Email         string    `json:"email"`
	LineCount     int       `json:"lineCount"`
	WeightedLines float64   `json:"weightedLines"`
	Percentage    float64   `json:"percentage"`
	LastCommit    time.Time `json:"lastCommit"`
}

// BlameOwnership represents ownership derived from git blame
type BlameOwnership struct {
	FilePath     string               `json:"filePath"`
	TotalLines   int                  `json:"totalLines"`
	Contributors []AuthorContribution `json:"contributors"`
	ComputedAt   time.Time            `json:"computedAt"`
	Confidence   float64              `json:"confidence"`
}

// RunGitBlame runs git blame on a file and parses the output
func RunGitBlame(repoRoot, filePath string) (*BlameResult, error) {
	// Run git blame with porcelain format for easy parsing
	cmd := exec.Command("git", "blame", "--porcelain", filePath)
	cmd.Dir = repoRoot

	output, err := cmd.Output()
	if err != nil {
		return nil, err
	}

	entries, err := parseBlameOutput(output)
	if err != nil {
		return nil, err
	}

	return &BlameResult{
		FilePath: filePath,
		Entries:  entries,
	}, nil
}

// parseBlameOutput parses git blame porcelain output
func parseBlameOutput(output []byte) ([]BlameEntry, error) {
	var entries []BlameEntry
	scanner := bufio.NewScanner(bytes.NewReader(output))

	var currentEntry BlameEntry
	lineNumber := 0

	for scanner.Scan() {
		line := scanner.Text()

		// New commit line starts with 40 hex chars
		if len(line) >= 40 && isHexString(line[:40]) {
			// Save previous entry if exists
			if currentEntry.CommitHash != "" {
				entries = append(entries, currentEntry)
			}

			parts := strings.Fields(line)
			currentEntry = BlameEntry{
				CommitHash: parts[0],
			}
			if len(parts) >= 3 {
				lineNumber++
				currentEntry.LineNumber = lineNumber
			}
		} else if strings.HasPrefix(line, "author ") {
			currentEntry.Author = strings.TrimPrefix(line, "author ")
		} else if strings.HasPrefix(line, "author-mail ") {
			mail := strings.TrimPrefix(line, "author-mail ")
			// Remove < and >
			mail = strings.Trim(mail, "<>")
			currentEntry.AuthorMail = mail
		} else if strings.HasPrefix(line, "author-time ") {
			timeStr := strings.TrimPrefix(line, "author-time ")
			var timestamp int64
			if _, err := parseTimestamp(timeStr, &timestamp); err == nil {
				currentEntry.Timestamp = time.Unix(timestamp, 0)
			}
		}
	}

	// Don't forget the last entry
	if currentEntry.CommitHash != "" {
		entries = append(entries, currentEntry)
	}

	return entries, scanner.Err()
}

func parseTimestamp(s string, timestamp *int64) (int, error) {
	n, err := exec.Command("sh", "-c", "echo "+s).Output()
	if err != nil {
		return 0, err
	}
	// Simple parsing - just convert string to int64
	var val int64
	for _, c := range strings.TrimSpace(string(n)) {
		if c >= '0' && c <= '9' {
			val = val*10 + int64(c-'0')
		}
	}
	*timestamp = val
	return len(s), nil
}

func isHexString(s string) bool {
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')) {
			return false
		}
	}
	return true
}

// ComputeBlameOwnership computes ownership from git blame with time decay
func ComputeBlameOwnership(result *BlameResult, config BlameConfig) *BlameOwnership {
	if len(result.Entries) == 0 {
		return &BlameOwnership{
			FilePath:     result.FilePath,
			TotalLines:   0,
			Contributors: []AuthorContribution{},
			ComputedAt:   time.Now(),
			Confidence:   0.79,
		}
	}

	// Compile bot patterns
	var botPatterns []*regexp.Regexp
	if config.ExcludeBots {
		for _, pattern := range config.BotPatterns {
			if re, err := regexp.Compile(pattern); err == nil {
				botPatterns = append(botPatterns, re)
			}
		}
	}

	now := time.Now()
	halfLife := float64(config.TimeDecayHalfLife) * 24 * float64(time.Hour)

	// Aggregate by author
	authorStats := make(map[string]*struct {
		email       string
		lineCount   int
		weightedSum float64
		lastCommit  time.Time
	})

	totalWeighted := 0.0

	for _, entry := range result.Entries {
		// Skip bots
		if isBot(entry.Author, entry.AuthorMail, botPatterns) {
			continue
		}

		// Compute time decay weight
		age := now.Sub(entry.Timestamp)
		weight := math.Pow(0.5, float64(age)/halfLife)

		// Use email as key for uniqueness
		key := normalizeAuthorKey(entry.Author, entry.AuthorMail)

		stats, exists := authorStats[key]
		if !exists {
			stats = &struct {
				email       string
				lineCount   int
				weightedSum float64
				lastCommit  time.Time
			}{
				email: entry.AuthorMail,
			}
			authorStats[key] = stats
		}

		stats.lineCount++
		stats.weightedSum += weight
		totalWeighted += weight

		if entry.Timestamp.After(stats.lastCommit) {
			stats.lastCommit = entry.Timestamp
		}
	}

	// Convert to sorted list
	var contributions []AuthorContribution
	for author, stats := range authorStats {
		percentage := 0.0
		if totalWeighted > 0 {
			percentage = stats.weightedSum / totalWeighted
		}

		// Skip if below minimum contribution threshold
		if percentage < config.MinContribution {
			continue
		}

		contributions = append(contributions, AuthorContribution{
			Author:        author,
			Email:         stats.email,
			LineCount:     stats.lineCount,
			WeightedLines: stats.weightedSum,
			Percentage:    percentage,
			LastCommit:    stats.lastCommit,
		})
	}

	// Sort by percentage descending
	sort.Slice(contributions, func(i, j int) bool {
		return contributions[i].Percentage > contributions[j].Percentage
	})

	return &BlameOwnership{
		FilePath:     result.FilePath,
		TotalLines:   len(result.Entries),
		Contributors: contributions,
		ComputedAt:   now,
		Confidence:   0.79,
	}
}

// normalizeAuthorKey creates a consistent key for an author
func normalizeAuthorKey(author, email string) string {
	// Prefer email for uniqueness, fall back to author name
	if email != "" && email != "noreply@github.com" {
		return strings.ToLower(email)
	}
	return strings.ToLower(author)
}

// isBot checks if an author is a bot
func isBot(author, email string, patterns []*regexp.Regexp) bool {
	for _, pattern := range patterns {
		if pattern.MatchString(author) || pattern.MatchString(email) {
			return true
		}
	}
	return false
}

// BlameOwnershipToOwners converts blame ownership to Owner structs
func BlameOwnershipToOwners(ownership *BlameOwnership) []Owner {
	owners := make([]Owner, 0, len(ownership.Contributors))

	for _, contrib := range ownership.Contributors {
		scope := "contributor"
		if contrib.Percentage >= 0.50 {
			scope = "maintainer"
		} else if contrib.Percentage >= 0.20 {
			scope = "reviewer"
		}

		owners = append(owners, Owner{
			ID:         contrib.Email,
			Type:       "email",
			Scope:      scope,
			Source:     "git-blame",
			Confidence: ownership.Confidence,
		})
	}

	return owners
}

// GetFileOwnership computes complete ownership for a file
// combining CODEOWNERS and git-blame
func GetFileOwnership(repoRoot, filePath string, codeowners *CodeownersFile, blameConfig BlameConfig) ([]Owner, error) {
	var owners []Owner

	// First, check CODEOWNERS (highest priority)
	if codeowners != nil {
		codeownersList := codeowners.GetOwnersForPath(filePath)
		if len(codeownersList) > 0 {
			owners = append(owners, CodeownersToOwners(codeownersList)...)
		}
	}

	// Then, get git-blame ownership
	blameResult, err := RunGitBlame(repoRoot, filePath)
	if err == nil {
		blameOwnership := ComputeBlameOwnership(blameResult, blameConfig)
		blameOwners := BlameOwnershipToOwners(blameOwnership)
		owners = append(owners, blameOwners...)
	}

	return owners, nil
}
