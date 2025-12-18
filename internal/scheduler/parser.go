package scheduler

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Expression types
const (
	ExprTypeInterval = "interval"
	ExprTypeCron     = "cron"
	ExprTypeDaily    = "daily"
)

// ParsedExpression represents a parsed schedule expression
type ParsedExpression struct {
	Type     string
	Interval time.Duration // for interval type
	Cron     *CronExpr     // for cron type
	Time     string        // for daily type (HH:MM)
}

// CronExpr represents a parsed cron expression (simplified)
type CronExpr struct {
	Minute     []int // 0-59
	Hour       []int // 0-23
	DayOfMonth []int // 1-31
	Month      []int // 1-12
	DayOfWeek  []int // 0-6 (Sunday = 0)
}

var (
	intervalRegex = regexp.MustCompile(`^every\s+(\d+)\s*(s|m|h|d|seconds?|minutes?|hours?|days?)$`)
	dailyRegex    = regexp.MustCompile(`^daily\s+at\s+(\d{1,2}):(\d{2})$`)
	cronRegex     = regexp.MustCompile(`^(\S+)\s+(\S+)\s+(\S+)\s+(\S+)\s+(\S+)$`)
)

// ParseExpression parses a schedule expression
func ParseExpression(expr string) (*ParsedExpression, error) {
	expr = strings.TrimSpace(strings.ToLower(expr))

	// Try interval format: "every Xh", "every 30m", etc.
	if matches := intervalRegex.FindStringSubmatch(expr); matches != nil {
		value, _ := strconv.Atoi(matches[1])
		unit := matches[2]

		var duration time.Duration
		switch {
		case strings.HasPrefix(unit, "s"):
			duration = time.Duration(value) * time.Second
		case strings.HasPrefix(unit, "m"):
			duration = time.Duration(value) * time.Minute
		case strings.HasPrefix(unit, "h"):
			duration = time.Duration(value) * time.Hour
		case strings.HasPrefix(unit, "d"):
			duration = time.Duration(value) * 24 * time.Hour
		}

		if duration < time.Minute {
			return nil, fmt.Errorf("minimum interval is 1 minute")
		}

		return &ParsedExpression{
			Type:     ExprTypeInterval,
			Interval: duration,
		}, nil
	}

	// Try daily format: "daily at HH:MM"
	if matches := dailyRegex.FindStringSubmatch(expr); matches != nil {
		hour, _ := strconv.Atoi(matches[1])
		minute, _ := strconv.Atoi(matches[2])

		if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
			return nil, fmt.Errorf("invalid time: %s:%s", matches[1], matches[2])
		}

		return &ParsedExpression{
			Type: ExprTypeDaily,
			Time: fmt.Sprintf("%02d:%02d", hour, minute),
		}, nil
	}

	// Try cron format: "* * * * *"
	if matches := cronRegex.FindStringSubmatch(expr); matches != nil {
		cron, err := parseCronExpr(matches[1], matches[2], matches[3], matches[4], matches[5])
		if err != nil {
			return nil, fmt.Errorf("invalid cron expression: %w", err)
		}

		return &ParsedExpression{
			Type: ExprTypeCron,
			Cron: cron,
		}, nil
	}

	return nil, fmt.Errorf("unrecognized schedule expression: %s", expr)
}

// NextRunTime calculates the next run time for an expression
func NextRunTime(expr string, from time.Time) (time.Time, error) {
	parsed, err := ParseExpression(expr)
	if err != nil {
		return time.Time{}, err
	}

	return parsed.NextRun(from), nil
}

// NextRun calculates the next run time
func (p *ParsedExpression) NextRun(from time.Time) time.Time {
	switch p.Type {
	case ExprTypeInterval:
		return from.Add(p.Interval)

	case ExprTypeDaily:
		parts := strings.Split(p.Time, ":")
		hour, _ := strconv.Atoi(parts[0])
		minute, _ := strconv.Atoi(parts[1])

		next := time.Date(from.Year(), from.Month(), from.Day(), hour, minute, 0, 0, from.Location())
		if !next.After(from) {
			next = next.AddDate(0, 0, 1)
		}
		return next

	case ExprTypeCron:
		return p.Cron.NextRun(from)

	default:
		return from.Add(time.Hour)
	}
}

// parseCronExpr parses the 5 cron fields
func parseCronExpr(minute, hour, dom, month, dow string) (*CronExpr, error) {
	var err error
	cron := &CronExpr{}

	cron.Minute, err = parseCronField(minute, 0, 59)
	if err != nil {
		return nil, fmt.Errorf("minute field: %w", err)
	}

	cron.Hour, err = parseCronField(hour, 0, 23)
	if err != nil {
		return nil, fmt.Errorf("hour field: %w", err)
	}

	cron.DayOfMonth, err = parseCronField(dom, 1, 31)
	if err != nil {
		return nil, fmt.Errorf("day-of-month field: %w", err)
	}

	cron.Month, err = parseCronField(month, 1, 12)
	if err != nil {
		return nil, fmt.Errorf("month field: %w", err)
	}

	cron.DayOfWeek, err = parseCronField(dow, 0, 6)
	if err != nil {
		return nil, fmt.Errorf("day-of-week field: %w", err)
	}

	return cron, nil
}

// parseCronField parses a single cron field
func parseCronField(field string, min, max int) ([]int, error) {
	if field == "*" {
		// All values
		values := make([]int, max-min+1)
		for i := range values {
			values[i] = min + i
		}
		return values, nil
	}

	var values []int

	// Handle comma-separated values
	parts := strings.Split(field, ",")
	for _, part := range parts {
		// Handle range with optional step: "1-5" or "1-5/2"
		if strings.Contains(part, "-") {
			rangeVals, err := parseCronRange(part, min, max)
			if err != nil {
				return nil, err
			}
			values = append(values, rangeVals...)
			continue
		}

		// Handle step: "*/5"
		if strings.HasPrefix(part, "*/") {
			stepStr := strings.TrimPrefix(part, "*/")
			step, err := strconv.Atoi(stepStr)
			if err != nil || step <= 0 {
				return nil, fmt.Errorf("invalid step: %s", stepStr)
			}
			for i := min; i <= max; i += step {
				values = append(values, i)
			}
			continue
		}

		// Single value
		val, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid value: %s", part)
		}
		if val < min || val > max {
			return nil, fmt.Errorf("value %d out of range [%d-%d]", val, min, max)
		}
		values = append(values, val)
	}

	if len(values) == 0 {
		return nil, fmt.Errorf("no valid values")
	}

	return unique(values), nil
}

// parseCronRange parses a range like "1-5" or "1-5/2"
func parseCronRange(part string, min, max int) ([]int, error) {
	step := 1
	rangePart := part

	if strings.Contains(part, "/") {
		parts := strings.Split(part, "/")
		rangePart = parts[0]
		var err error
		step, err = strconv.Atoi(parts[1])
		if err != nil || step <= 0 {
			return nil, fmt.Errorf("invalid step in range: %s", part)
		}
	}

	rangeParts := strings.Split(rangePart, "-")
	if len(rangeParts) != 2 {
		return nil, fmt.Errorf("invalid range: %s", rangePart)
	}

	start, err := strconv.Atoi(rangeParts[0])
	if err != nil {
		return nil, fmt.Errorf("invalid range start: %s", rangeParts[0])
	}

	end, err := strconv.Atoi(rangeParts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid range end: %s", rangeParts[1])
	}

	if start < min || end > max || start > end {
		return nil, fmt.Errorf("range %d-%d out of bounds [%d-%d]", start, end, min, max)
	}

	var values []int
	for i := start; i <= end; i += step {
		values = append(values, i)
	}

	return values, nil
}

// unique removes duplicates and sorts
func unique(values []int) []int {
	seen := make(map[int]bool)
	var result []int
	for _, v := range values {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	// Simple sort
	for i := 0; i < len(result)-1; i++ {
		for j := i + 1; j < len(result); j++ {
			if result[j] < result[i] {
				result[i], result[j] = result[j], result[i]
			}
		}
	}
	return result
}

// NextRun finds the next run time for a cron expression
func (c *CronExpr) NextRun(from time.Time) time.Time {
	// Start from the next minute
	t := from.Truncate(time.Minute).Add(time.Minute)

	// Search for the next matching time (max 1 year ahead)
	maxTime := from.AddDate(1, 0, 0)

	for t.Before(maxTime) {
		if c.matches(t) {
			return t
		}

		// Optimize: jump to next valid minute/hour/day if possible
		if !contains(c.Month, int(t.Month())) {
			// Jump to next month
			t = time.Date(t.Year(), t.Month()+1, 1, 0, 0, 0, 0, t.Location())
			continue
		}

		if !contains(c.DayOfMonth, t.Day()) && !contains(c.DayOfWeek, int(t.Weekday())) {
			// Jump to next day
			t = time.Date(t.Year(), t.Month(), t.Day()+1, 0, 0, 0, 0, t.Location())
			continue
		}

		if !contains(c.Hour, t.Hour()) {
			// Jump to next hour
			t = time.Date(t.Year(), t.Month(), t.Day(), t.Hour()+1, 0, 0, 0, t.Location())
			continue
		}

		// Increment by minute
		t = t.Add(time.Minute)
	}

	// Fallback: 1 hour from now
	return from.Add(time.Hour)
}

// matches checks if a time matches the cron expression
func (c *CronExpr) matches(t time.Time) bool {
	return contains(c.Minute, t.Minute()) &&
		contains(c.Hour, t.Hour()) &&
		(contains(c.DayOfMonth, t.Day()) || contains(c.DayOfWeek, int(t.Weekday()))) &&
		contains(c.Month, int(t.Month()))
}

// contains checks if a slice contains a value
func contains(slice []int, val int) bool {
	for _, v := range slice {
		if v == val {
			return true
		}
	}
	return false
}

// FormatDuration formats a duration for display
func FormatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
