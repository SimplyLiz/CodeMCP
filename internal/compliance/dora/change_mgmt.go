package dora

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- missing-rollback: Art. 15 DORA — Database migrations without rollback ---

type missingRollbackCheck struct{}

func (c *missingRollbackCheck) ID() string       { return "missing-rollback" }
func (c *missingRollbackCheck) Name() string     { return "Missing Migration Rollback" }
func (c *missingRollbackCheck) Article() string   { return "Art. 15 DORA" }
func (c *missingRollbackCheck) Severity() string  { return "warning" }

var migrationDirs = []string{
	"migrations", "migration", "db/migrations", "db/migrate",
	"database/migrations", "sql/migrations", "schema",
}

func (c *missingRollbackCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	// Collect migration files grouped by directory
	upFiles := make(map[string][]string) // dir -> list of "up" migration files
	downFiles := make(map[string]bool)   // set of "down" migration file basenames

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		lower := strings.ToLower(file)

		// Check if file is in a migration directory
		isMigration := false
		for _, dir := range migrationDirs {
			if strings.Contains(lower, dir+"/") || strings.Contains(lower, dir+"\\") {
				isMigration = true
				break
			}
		}

		// Also detect numbered migration files
		if !isMigration && (strings.Contains(lower, ".up.") || strings.Contains(lower, ".down.") ||
			strings.Contains(lower, "_up.") || strings.Contains(lower, "_down.")) {
			isMigration = true
		}

		if !isMigration {
			continue
		}

		dir := filepath.Dir(file)

		if strings.Contains(lower, ".down.") || strings.Contains(lower, "_down.") ||
			strings.Contains(lower, "rollback") || strings.Contains(lower, "revert") {
			downFiles[file] = true
		} else if strings.Contains(lower, ".up.") || strings.Contains(lower, "_up.") ||
			strings.HasSuffix(lower, ".sql") || strings.HasSuffix(lower, ".rb") ||
			strings.HasSuffix(lower, ".py") || strings.HasSuffix(lower, ".ts") ||
			strings.HasSuffix(lower, ".js") {
			upFiles[dir] = append(upFiles[dir], file)
		}
	}

	// Check for up migrations without corresponding down migrations
	for dir, ups := range upFiles {
		hasAnyDown := false
		for downFile := range downFiles {
			if strings.HasPrefix(downFile, dir) {
				hasAnyDown = true
				break
			}
		}

		if !hasAnyDown && len(ups) > 0 {
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "Art. 15 DORA",
				File:       ups[0],
				Message:    "Database migration directory has up/forward migrations but no corresponding rollback/down migrations",
				Suggestion: "Add rollback (down) migrations for each forward migration to enable safe change reversal per DORA ICT change management",
				Confidence: 0.70,
			})
		}
	}

	// If no migration files found, check if project has DB usage without any migration structure
	if len(upFiles) == 0 && len(downFiles) == 0 {
		hasDatabaseUsage := false
		for _, file := range scope.Files {
			if strings.Contains(file, "_test.") {
				continue
			}

			fullPath := filepath.Join(scope.RepoRoot, file)
			content, err := os.ReadFile(fullPath)
			if err != nil {
				continue
			}

			contentLower := strings.ToLower(string(content))
			if strings.Contains(contentLower, "create table") || strings.Contains(contentLower, "alter table") ||
				strings.Contains(contentLower, "sql.open") || strings.Contains(contentLower, "database_url") {
				hasDatabaseUsage = true
				break
			}
		}

		if hasDatabaseUsage {
			findings = append(findings, compliance.Finding{
				Severity:   "warning",
				Article:    "Art. 15 DORA",
				File:       "",
				Message:    "Database usage detected but no structured migration directory found",
				Suggestion: "Implement a structured database migration system with up/down migrations to support change rollback",
				Confidence: 0.55,
			})
		}
	}

	return findings, nil
}
