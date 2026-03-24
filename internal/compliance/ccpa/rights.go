package ccpa

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- missing-data-access: §1798.110 CCPA — Right to access/export ---

type missingDataAccessCheck struct{}

func (c *missingDataAccessCheck) ID() string       { return "missing-data-access" }
func (c *missingDataAccessCheck) Name() string     { return "Missing Data Access/Export Capability" }
func (c *missingDataAccessCheck) Article() string   { return "§1798.110 CCPA" }
func (c *missingDataAccessCheck) Severity() string  { return "warning" }

var dataAccessPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)data[_\-]?export`),
	regexp.MustCompile(`(?i)data[_\-]?download`),
	regexp.MustCompile(`(?i)data[_\-]?portability`),
	regexp.MustCompile(`(?i)export[_\-]?data`),
	regexp.MustCompile(`(?i)download[_\-]?data`),
	regexp.MustCompile(`(?i)user[_\-]?data[_\-]?request`),
	regexp.MustCompile(`(?i)data[_\-]?access[_\-]?request`),
	regexp.MustCompile(`(?i)subject[_\-]?access[_\-]?request`),
	regexp.MustCompile(`(?i)dsar\b`),
	regexp.MustCompile(`(?i)/api/.*(export|download|data-request)`),
	regexp.MustCompile(`(?i)get[_\-]?my[_\-]?data`),
	regexp.MustCompile(`(?i)right[_\-]?to[_\-]?access`),
}

var userDataPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\buser[_\-]?profile\b`),
	regexp.MustCompile(`(?i)\buser[_\-]?account\b`),
	regexp.MustCompile(`(?i)\bpersonal[_\-]?data\b`),
	regexp.MustCompile(`(?i)\buser[_\-]?data\b`),
}

func (c *missingDataAccessCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	hasUserData := false
	hasDataAccess := false
	var userDataFile string

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
			continue
		}

		f, err := os.Open(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()

			for _, p := range dataAccessPatterns {
				if p.MatchString(line) {
					hasDataAccess = true
				}
			}

			if !hasUserData {
				for _, p := range userDataPatterns {
					if p.MatchString(line) {
						hasUserData = true
						userDataFile = file
					}
				}
			}
		}
		f.Close()
	}

	if hasUserData && !hasDataAccess {
		return []compliance.Finding{
			{
				Severity:   "warning",
				Article:    "§1798.110 CCPA",
				File:       userDataFile,
				Message:    "User/personal data handling detected without data access/export capability",
				Suggestion: "Implement a data access/export endpoint so consumers can request their personal information per CCPA §1798.110",
				Confidence: 0.60,
			},
		}, nil
	}

	return nil, nil
}

// --- missing-deletion: §1798.105 CCPA — Right to delete ---

type missingDeletionCheck struct{}

func (c *missingDeletionCheck) ID() string       { return "missing-deletion" }
func (c *missingDeletionCheck) Name() string     { return "Missing Data Deletion Capability" }
func (c *missingDeletionCheck) Article() string   { return "§1798.105 CCPA" }
func (c *missingDeletionCheck) Severity() string  { return "warning" }

var dataDeletionPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)delete[_\-]?account`),
	regexp.MustCompile(`(?i)delete[_\-]?user`),
	regexp.MustCompile(`(?i)remove[_\-]?account`),
	regexp.MustCompile(`(?i)data[_\-]?deletion`),
	regexp.MustCompile(`(?i)erase[_\-]?data`),
	regexp.MustCompile(`(?i)purge[_\-]?data`),
	regexp.MustCompile(`(?i)right[_\-]?to[_\-]?delete`),
	regexp.MustCompile(`(?i)right[_\-]?to[_\-]?erasure`),
	regexp.MustCompile(`(?i)deletion[_\-]?request`),
	regexp.MustCompile(`(?i)anonymize[_\-]?user`),
	regexp.MustCompile(`(?i)gdpr[_\-]?delete`),
}

func (c *missingDeletionCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	hasUserData := false
	hasDeletion := false
	var userDataFile string

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, ".test.") {
			continue
		}

		f, err := os.Open(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			line := scanner.Text()

			for _, p := range dataDeletionPatterns {
				if p.MatchString(line) {
					hasDeletion = true
				}
			}

			if !hasUserData {
				for _, p := range userDataPatterns {
					if p.MatchString(line) {
						hasUserData = true
						userDataFile = file
					}
				}
			}
		}
		f.Close()
	}

	if hasUserData && !hasDeletion {
		return []compliance.Finding{
			{
				Severity:   "warning",
				Article:    "§1798.105 CCPA",
				File:       userDataFile,
				Message:    "User/personal data handling detected without data deletion capability",
				Suggestion: "Implement data deletion functionality so consumers can request deletion of their personal information per CCPA §1798.105",
				Confidence: 0.60,
			},
		}, nil
	}

	return nil, nil
}
