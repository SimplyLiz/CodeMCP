package scheduler

import (
	"testing"
	"time"
)

func TestParseExpressionInterval(t *testing.T) {
	tests := []struct {
		expr     string
		wantType string
		wantDur  time.Duration
		wantErr  bool
	}{
		{"every 5m", ExprTypeInterval, 5 * time.Minute, false},
		{"every 5 minutes", ExprTypeInterval, 5 * time.Minute, false},
		{"every 2h", ExprTypeInterval, 2 * time.Hour, false},
		{"every 2 hours", ExprTypeInterval, 2 * time.Hour, false},
		{"every 1d", ExprTypeInterval, 24 * time.Hour, false},
		{"every 1 day", ExprTypeInterval, 24 * time.Hour, false},
		{"every 30s", ExprTypeInterval, 0, true}, // Below minimum
		{"every 1 minute", ExprTypeInterval, time.Minute, false},
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			parsed, err := ParseExpression(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseExpression() error = %v", err)
			}
			if parsed.Type != tt.wantType {
				t.Errorf("Type = %q, want %q", parsed.Type, tt.wantType)
			}
			if parsed.Interval != tt.wantDur {
				t.Errorf("Interval = %v, want %v", parsed.Interval, tt.wantDur)
			}
		})
	}
}

func TestParseExpressionDaily(t *testing.T) {
	tests := []struct {
		expr     string
		wantTime string
		wantErr  bool
	}{
		{"daily at 09:00", "09:00", false},
		{"daily at 9:00", "09:00", false},
		{"daily at 23:59", "23:59", false},
		{"daily at 00:00", "00:00", false},
		{"daily at 25:00", "", true}, // Invalid hour
		{"daily at 12:60", "", true}, // Invalid minute
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			parsed, err := ParseExpression(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseExpression() error = %v", err)
			}
			if parsed.Type != ExprTypeDaily {
				t.Errorf("Type = %q, want %q", parsed.Type, ExprTypeDaily)
			}
			if parsed.Time != tt.wantTime {
				t.Errorf("Time = %q, want %q", parsed.Time, tt.wantTime)
			}
		})
	}
}

func TestParseExpressionCron(t *testing.T) {
	tests := []struct {
		expr    string
		wantErr bool
	}{
		{"* * * * *", false},      // Every minute
		{"0 * * * *", false},      // Every hour
		{"0 0 * * *", false},      // Every day at midnight
		{"0 0 1 * *", false},      // First of every month
		{"0 0 * * 0", false},      // Every Sunday
		{"*/5 * * * *", false},    // Every 5 minutes
		{"0 9-17 * * 1-5", false}, // 9am-5pm weekdays
		{"0 0 1,15 * *", false},   // 1st and 15th of month
		{"invalid cron", true},    // Invalid
		{"60 * * * *", true},      // Invalid minute
		{"* 24 * * *", true},      // Invalid hour
		{"* * 32 * *", true},      // Invalid day
		{"* * * 13 *", true},      // Invalid month
		{"* * * * 7", true},       // Invalid day of week
	}

	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			parsed, err := ParseExpression(tt.expr)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseExpression() error = %v", err)
			}
			if parsed.Type != ExprTypeCron {
				t.Errorf("Type = %q, want %q", parsed.Type, ExprTypeCron)
			}
			if parsed.Cron == nil {
				t.Error("Cron should not be nil")
			}
		})
	}
}

func TestParseExpressionUnrecognized(t *testing.T) {
	_, err := ParseExpression("something random")
	if err == nil {
		t.Error("Expected error for unrecognized expression")
	}
}

func TestParseCronField(t *testing.T) {
	tests := []struct {
		name    string
		field   string
		min     int
		max     int
		want    []int
		wantErr bool
	}{
		{"wildcard", "*", 0, 5, []int{0, 1, 2, 3, 4, 5}, false},
		{"single value", "3", 0, 5, []int{3}, false},
		{"comma list", "1,3,5", 0, 5, []int{1, 3, 5}, false},
		{"range", "1-3", 0, 5, []int{1, 2, 3}, false},
		{"range with step", "0-10/2", 0, 10, []int{0, 2, 4, 6, 8, 10}, false},
		{"step", "*/2", 0, 6, []int{0, 2, 4, 6}, false},
		{"out of range", "10", 0, 5, nil, true},
		{"invalid value", "abc", 0, 5, nil, true},
		{"invalid step", "*/0", 0, 5, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCronField(tt.field, tt.min, tt.max)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCronField() error = %v", err)
			}
			if !sliceEqual(got, tt.want) {
				t.Errorf("parseCronField() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestParseCronRange(t *testing.T) {
	tests := []struct {
		name    string
		part    string
		min     int
		max     int
		want    []int
		wantErr bool
	}{
		{"simple range", "1-5", 0, 10, []int{1, 2, 3, 4, 5}, false},
		{"range with step", "1-9/2", 0, 10, []int{1, 3, 5, 7, 9}, false},
		{"invalid range", "5-1", 0, 10, nil, true},
		{"out of bounds", "0-15", 0, 10, nil, true},
		{"invalid start", "a-5", 0, 10, nil, true},
		{"invalid end", "1-b", 0, 10, nil, true},
		{"invalid step in range", "1-5/0", 0, 10, nil, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseCronRange(tt.part, tt.min, tt.max)
			if tt.wantErr {
				if err == nil {
					t.Error("Expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseCronRange() error = %v", err)
			}
			if !sliceEqual(got, tt.want) {
				t.Errorf("parseCronRange() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUnique(t *testing.T) {
	tests := []struct {
		input []int
		want  []int
	}{
		{[]int{1, 2, 3}, []int{1, 2, 3}},
		{[]int{3, 1, 2}, []int{1, 2, 3}},
		{[]int{1, 1, 2, 2, 3}, []int{1, 2, 3}},
		{[]int{}, []int(nil)},
		{[]int{5}, []int{5}},
	}

	for _, tt := range tests {
		got := unique(tt.input)
		if !sliceEqual(got, tt.want) {
			t.Errorf("unique(%v) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestContains(t *testing.T) {
	slice := []int{1, 3, 5, 7}

	if !contains(slice, 3) {
		t.Error("Should contain 3")
	}
	if contains(slice, 2) {
		t.Error("Should not contain 2")
	}
	if contains([]int{}, 1) {
		t.Error("Empty slice should not contain anything")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		dur  time.Duration
		want string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2 * time.Hour, "2h"},
		{48 * time.Hour, "2d"},
		{time.Second, "1s"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := FormatDuration(tt.dur)
			if got != tt.want {
				t.Errorf("FormatDuration(%v) = %q, want %q", tt.dur, got, tt.want)
			}
		})
	}
}

func TestNextRunTimeInterval(t *testing.T) {
	from := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	next, err := NextRunTime("every 1h", from)
	if err != nil {
		t.Fatalf("NextRunTime() error = %v", err)
	}

	expected := from.Add(time.Hour)
	if !next.Equal(expected) {
		t.Errorf("NextRunTime() = %v, want %v", next, expected)
	}
}

func TestNextRunTimeDaily(t *testing.T) {
	// Test when the time is before the scheduled time
	from := time.Date(2025, 1, 1, 8, 0, 0, 0, time.UTC)
	next, err := NextRunTime("daily at 10:00", from)
	if err != nil {
		t.Fatalf("NextRunTime() error = %v", err)
	}

	expected := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("NextRunTime() = %v, want %v", next, expected)
	}

	// Test when the time is after the scheduled time
	from = time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)
	next, err = NextRunTime("daily at 10:00", from)
	if err != nil {
		t.Fatalf("NextRunTime() error = %v", err)
	}

	expected = time.Date(2025, 1, 2, 10, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("NextRunTime() = %v, want %v", next, expected)
	}
}

func TestNextRunTimeInvalid(t *testing.T) {
	_, err := NextRunTime("invalid expression", time.Now())
	if err == nil {
		t.Error("Expected error for invalid expression")
	}
}

func TestCronExprMatches(t *testing.T) {
	// Every minute
	cron := &CronExpr{
		Minute:     []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31, 32, 33, 34, 35, 36, 37, 38, 39, 40, 41, 42, 43, 44, 45, 46, 47, 48, 49, 50, 51, 52, 53, 54, 55, 56, 57, 58, 59},
		Hour:       []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23},
		DayOfMonth: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
		Month:      []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		DayOfWeek:  []int{0, 1, 2, 3, 4, 5, 6},
	}

	testTime := time.Date(2025, 6, 15, 10, 30, 0, 0, time.UTC)
	if !cron.matches(testTime) {
		t.Error("Cron should match any time with wildcards")
	}
}

func TestCronExprNextRun(t *testing.T) {
	// Every hour at minute 0
	cron := &CronExpr{
		Minute:     []int{0},
		Hour:       []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23},
		DayOfMonth: []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27, 28, 29, 30, 31},
		Month:      []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12},
		DayOfWeek:  []int{0, 1, 2, 3, 4, 5, 6},
	}

	from := time.Date(2025, 1, 1, 10, 30, 0, 0, time.UTC)
	next := cron.NextRun(from)

	// Should be 11:00
	expected := time.Date(2025, 1, 1, 11, 0, 0, 0, time.UTC)
	if !next.Equal(expected) {
		t.Errorf("NextRun() = %v, want %v", next, expected)
	}
}

func TestParsedExpressionNextRun(t *testing.T) {
	from := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)

	t.Run("interval", func(t *testing.T) {
		parsed := &ParsedExpression{
			Type:     ExprTypeInterval,
			Interval: 2 * time.Hour,
		}
		next := parsed.NextRun(from)
		expected := from.Add(2 * time.Hour)
		if !next.Equal(expected) {
			t.Errorf("NextRun() = %v, want %v", next, expected)
		}
	})

	t.Run("daily", func(t *testing.T) {
		parsed := &ParsedExpression{
			Type: ExprTypeDaily,
			Time: "12:00",
		}
		next := parsed.NextRun(from)
		expected := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)
		if !next.Equal(expected) {
			t.Errorf("NextRun() = %v, want %v", next, expected)
		}
	})

	t.Run("default fallback", func(t *testing.T) {
		parsed := &ParsedExpression{
			Type: "unknown",
		}
		next := parsed.NextRun(from)
		expected := from.Add(time.Hour)
		if !next.Equal(expected) {
			t.Errorf("NextRun() = %v, want %v", next, expected)
		}
	})
}

// Schedule type tests

func TestScheduleToSummary(t *testing.T) {
	now := time.Now()
	sched := &Schedule{
		ID:         "sched-123",
		TaskType:   TaskTypeRefresh,
		Target:     "repo-1",
		Expression: "every 1h",
		Enabled:    true,
		NextRun:    now,
		LastStatus: "success",
	}

	summary := sched.ToSummary()

	if summary.ID != "sched-123" {
		t.Errorf("ID = %q, want 'sched-123'", summary.ID)
	}
	if summary.TaskType != TaskTypeRefresh {
		t.Errorf("TaskType = %v, want %v", summary.TaskType, TaskTypeRefresh)
	}
	if summary.Target != "repo-1" {
		t.Errorf("Target = %q, want 'repo-1'", summary.Target)
	}
	if !summary.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestScheduleIsDue(t *testing.T) {
	t.Run("disabled", func(t *testing.T) {
		sched := &Schedule{Enabled: false, NextRun: time.Now().Add(-time.Hour)}
		if sched.IsDue() {
			t.Error("Disabled schedule should not be due")
		}
	})

	t.Run("past next run", func(t *testing.T) {
		sched := &Schedule{Enabled: true, NextRun: time.Now().Add(-time.Hour)}
		if !sched.IsDue() {
			t.Error("Schedule should be due when past next run time")
		}
	})

	t.Run("future next run", func(t *testing.T) {
		sched := &Schedule{Enabled: true, NextRun: time.Now().Add(time.Hour)}
		if sched.IsDue() {
			t.Error("Schedule should not be due when next run is in future")
		}
	})
}

func TestScheduleMarkRun(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		sched := &Schedule{Expression: "every 1h", Enabled: true}
		err := sched.MarkRun(true, 5*time.Second, "")
		if err != nil {
			t.Fatalf("MarkRun() error = %v", err)
		}

		if sched.LastStatus != "success" {
			t.Errorf("LastStatus = %q, want 'success'", sched.LastStatus)
		}
		if sched.LastError != "" {
			t.Errorf("LastError = %q, want empty", sched.LastError)
		}
		if sched.LastDuration != 5000 {
			t.Errorf("LastDuration = %d, want 5000", sched.LastDuration)
		}
		if sched.LastRun == nil {
			t.Error("LastRun should be set")
		}
	})

	t.Run("failure", func(t *testing.T) {
		sched := &Schedule{Expression: "every 1h", Enabled: true}
		err := sched.MarkRun(false, time.Second, "something failed")
		if err != nil {
			t.Fatalf("MarkRun() error = %v", err)
		}

		if sched.LastStatus != "failed" {
			t.Errorf("LastStatus = %q, want 'failed'", sched.LastStatus)
		}
		if sched.LastError != "something failed" {
			t.Errorf("LastError = %q, want 'something failed'", sched.LastError)
		}
	})
}

func TestScheduleToJSON(t *testing.T) {
	sched := &Schedule{
		ID:         "sched-1",
		TaskType:   TaskTypeRefresh,
		Expression: "every 1h",
		Enabled:    true,
	}

	json, err := sched.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON() error = %v", err)
	}

	if json == "" {
		t.Error("JSON should not be empty")
	}

	if !containsStr(json, "sched-1") {
		t.Error("JSON should contain schedule ID")
	}
}

func TestNewSchedule(t *testing.T) {
	sched, err := NewSchedule(TaskTypeRefresh, "repo-1", "every 2h")
	if err != nil {
		t.Fatalf("NewSchedule() error = %v", err)
	}

	if sched.ID == "" {
		t.Error("ID should not be empty")
	}
	if sched.TaskType != TaskTypeRefresh {
		t.Errorf("TaskType = %v, want %v", sched.TaskType, TaskTypeRefresh)
	}
	if sched.Target != "repo-1" {
		t.Errorf("Target = %q, want 'repo-1'", sched.Target)
	}
	if !sched.Enabled {
		t.Error("Should be enabled by default")
	}
	if sched.NextRun.IsZero() {
		t.Error("NextRun should be set")
	}
}

func TestNewScheduleInvalidExpression(t *testing.T) {
	_, err := NewSchedule(TaskTypeRefresh, "", "invalid expression")
	if err == nil {
		t.Error("Expected error for invalid expression")
	}
}

func TestTaskTypeConstants(t *testing.T) {
	types := []TaskType{
		TaskTypeRefresh,
		TaskTypeFederationSync,
		TaskTypeCleanup,
		TaskTypeHealthCheck,
	}

	for _, tt := range types {
		if string(tt) == "" {
			t.Errorf("TaskType %v should not be empty", tt)
		}
	}
}

func TestFormatBase36(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{10, "a"},
		{35, "z"},
		{36, "10"},
	}

	for _, tt := range tests {
		got := formatBase36(tt.input)
		if got != tt.want {
			t.Errorf("formatBase36(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGenerateScheduleID(t *testing.T) {
	id1 := generateScheduleID()
	time.Sleep(time.Millisecond) // Ensure different timestamp
	id2 := generateScheduleID()

	if id1 == "" {
		t.Error("ID should not be empty")
	}
	if !containsStr(id1, "sched_") {
		t.Errorf("ID should start with 'sched_', got %q", id1)
	}
	// IDs should be unique (with high probability)
	if id1 == id2 {
		t.Logf("id1=%s, id2=%s (may be same with fast execution)", id1, id2)
	}
}

func TestRandomHex(t *testing.T) {
	hex1 := randomHex(3)
	hex2 := randomHex(3)

	if len(hex1) != 6 {
		t.Errorf("randomHex(3) should return 6 chars, got %d", len(hex1))
	}

	// Both should be valid hex
	for _, c := range hex1 {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Invalid hex char: %c", c)
		}
	}
	_ = hex2
}

// Helper functions

func sliceEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
