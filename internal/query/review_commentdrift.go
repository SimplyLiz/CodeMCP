package query

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// numberRe matches integer and float literals in Go code and comments.
var numberRe = regexp.MustCompile(`\b(\d+(?:\.\d+)?)\b`)

// checkCommentDrift detects numeric mismatches between comments and adjacent constants.
func (e *Engine) checkCommentDrift(ctx context.Context, changedFiles []string) (ReviewCheck, []ReviewFinding) {
	start := time.Now()

	var findings []ReviewFinding
	checked := 0

	for _, file := range changedFiles {
		if ctx.Err() != nil {
			break
		}
		if checked >= 20 {
			break
		}
		// Only check Go files for now
		if !strings.HasSuffix(file, ".go") {
			continue
		}
		checked++

		ff := e.detectCommentDrift(file)
		findings = append(findings, ff...)
	}

	status := "pass"
	summary := "No comment/code drift detected"
	if len(findings) > 0 {
		status = "info" // tier 3, purely informational
		summary = fmt.Sprintf("%d comment/code numeric mismatch(es)", len(findings))
	}

	return ReviewCheck{
		Name:     "comment-drift",
		Status:   status,
		Severity: "info",
		Summary:  summary,
		Duration: time.Since(start).Milliseconds(),
	}, findings
}

// detectCommentDrift scans a single file for numeric mismatches between
// comments and adjacent const assignments inside const blocks.
func (e *Engine) detectCommentDrift(file string) []ReviewFinding {
	absPath := filepath.Join(e.repoRoot, file)
	f, err := os.Open(absPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var findings []ReviewFinding
	scanner := bufio.NewScanner(f)

	inConst := false
	depth := 0
	lineNum := 0
	prevComment := ""
	prevCommentLine := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		// Track const block boundaries.
		if strings.HasPrefix(trimmed, "const (") || trimmed == "const (" {
			inConst = true
			depth = 1
			prevComment = ""
			prevCommentLine = 0
			continue
		}

		if !inConst {
			prevComment = ""
			prevCommentLine = 0
			continue
		}

		// Track nested parens (unlikely in const blocks, but be safe).
		depth += strings.Count(trimmed, "(") - strings.Count(trimmed, ")")
		if depth <= 0 {
			inConst = false
			prevComment = ""
			prevCommentLine = 0
			continue
		}

		// If this line is a comment, remember it.
		if strings.HasPrefix(trimmed, "//") {
			prevComment = trimmed
			prevCommentLine = lineNum
			continue
		}

		// If this line has an assignment and we have a preceding comment,
		// check for numeric drift.
		if prevComment != "" && strings.Contains(trimmed, "=") {
			finding := e.checkConstDrift(file, trimmed, lineNum, prevComment, prevCommentLine)
			if finding != nil {
				findings = append(findings, *finding)
			}
		}

		// Reset comment tracker for non-comment, non-blank lines.
		if trimmed != "" {
			prevComment = ""
			prevCommentLine = 0
		}
	}

	return findings
}

// checkConstDrift compares numbers in a comment to the assigned value of a const.
func (e *Engine) checkConstDrift(file, constLine string, constLineNum int, comment string, _ int) *ReviewFinding {
	// Parse the const assignment: "Name = value" or "Name type = value"
	parts := strings.SplitN(constLine, "=", 2)
	if len(parts) != 2 {
		return nil
	}

	namePart := strings.TrimSpace(parts[0])
	valuePart := strings.TrimSpace(parts[1])

	// Extract the const name (first token of namePart).
	nameTokens := strings.Fields(namePart)
	if len(nameTokens) == 0 {
		return nil
	}
	constName := nameTokens[0]

	// Try to parse the assigned value as a number.
	constVal, err := strconv.ParseFloat(valuePart, 64)
	if err != nil {
		return nil
	}

	// Extract numbers from the comment.
	commentText := strings.TrimPrefix(strings.TrimSpace(comment), "//")
	matches := numberRe.FindAllString(commentText, -1)
	if len(matches) == 0 {
		return nil
	}

	for _, m := range matches {
		commentVal, err := strconv.ParseFloat(m, 64)
		if err != nil {
			continue
		}
		if commentVal != constVal {
			return &ReviewFinding{
				Check:     "comment-drift",
				Severity:  "info",
				File:      file,
				StartLine: constLineNum,
				Message:   fmt.Sprintf("Comment says %q but const %s = %s", m, constName, valuePart),
				Category:  "drift",
				RuleID:    "ckb/comment-drift/numeric-mismatch",
			}
		}
	}

	return nil
}
