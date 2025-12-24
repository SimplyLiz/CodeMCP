package jobs

import (
	"errors"
	"testing"
	"time"
)

func TestNewJob(t *testing.T) {
	t.Run("with nil scope", func(t *testing.T) {
		job, err := NewJob(JobTypeRefreshArchitecture, nil)
		if err != nil {
			t.Fatalf("NewJob() error = %v", err)
		}
		if job == nil {
			t.Fatal("NewJob() returned nil")
		}
		if job.ID == "" {
			t.Error("Job ID should not be empty")
		}
		if job.Type != JobTypeRefreshArchitecture {
			t.Errorf("Type = %v, want %v", job.Type, JobTypeRefreshArchitecture)
		}
		if job.Status != JobQueued {
			t.Errorf("Status = %v, want %v", job.Status, JobQueued)
		}
		if job.Progress != 0 {
			t.Errorf("Progress = %d, want 0", job.Progress)
		}
		if job.Scope != "" {
			t.Errorf("Scope = %q, want empty", job.Scope)
		}
	})

	t.Run("with scope", func(t *testing.T) {
		scope := RefreshScope{Scope: "modules", Force: true}
		job, err := NewJob(JobTypeRefreshArchitecture, scope)
		if err != nil {
			t.Fatalf("NewJob() error = %v", err)
		}
		if job.Scope == "" {
			t.Error("Scope should be serialized")
		}
	})

	t.Run("different job types", func(t *testing.T) {
		types := []JobType{
			JobTypeRefreshArchitecture,
			JobTypeAnalyzeImpact,
			JobTypeExport,
			JobTypeFederationSync,
			JobTypeWebhookDispatch,
			JobTypeScheduledTask,
		}
		for _, jt := range types {
			job, err := NewJob(jt, nil)
			if err != nil {
				t.Errorf("NewJob(%v) error = %v", jt, err)
			}
			if job.Type != jt {
				t.Errorf("Type = %v, want %v", job.Type, jt)
			}
		}
	})
}

func TestJobIsTerminal(t *testing.T) {
	tests := []struct {
		status   JobStatus
		terminal bool
	}{
		{JobQueued, false},
		{JobRunning, false},
		{JobCompleted, true},
		{JobFailed, true},
		{JobCancelled, true},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			job := &Job{Status: tt.status}
			if got := job.IsTerminal(); got != tt.terminal {
				t.Errorf("IsTerminal() = %v, want %v", got, tt.terminal)
			}
		})
	}
}

func TestJobCanCancel(t *testing.T) {
	tests := []struct {
		status    JobStatus
		canCancel bool
	}{
		{JobQueued, true},
		{JobRunning, true},
		{JobCompleted, false},
		{JobFailed, false},
		{JobCancelled, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			job := &Job{Status: tt.status}
			if got := job.CanCancel(); got != tt.canCancel {
				t.Errorf("CanCancel() = %v, want %v", got, tt.canCancel)
			}
		})
	}
}

func TestJobMarkStarted(t *testing.T) {
	job := &Job{Status: JobQueued}
	job.MarkStarted()

	if job.Status != JobRunning {
		t.Errorf("Status = %v, want %v", job.Status, JobRunning)
	}
	if job.StartedAt == nil {
		t.Error("StartedAt should be set")
	}
}

func TestJobMarkCompleted(t *testing.T) {
	t.Run("with result", func(t *testing.T) {
		job := &Job{Status: JobRunning}
		result := RefreshResult{Status: "completed", ModulesDetected: 5}

		err := job.MarkCompleted(result)
		if err != nil {
			t.Fatalf("MarkCompleted() error = %v", err)
		}

		if job.Status != JobCompleted {
			t.Errorf("Status = %v, want %v", job.Status, JobCompleted)
		}
		if job.Progress != 100 {
			t.Errorf("Progress = %d, want 100", job.Progress)
		}
		if job.CompletedAt == nil {
			t.Error("CompletedAt should be set")
		}
		if job.Result == "" {
			t.Error("Result should be serialized")
		}
	})

	t.Run("with nil result", func(t *testing.T) {
		job := &Job{Status: JobRunning}
		err := job.MarkCompleted(nil)
		if err != nil {
			t.Fatalf("MarkCompleted() error = %v", err)
		}
		if job.Result != "" {
			t.Errorf("Result = %q, want empty", job.Result)
		}
	})
}

func TestJobMarkFailed(t *testing.T) {
	t.Run("with error", func(t *testing.T) {
		job := &Job{Status: JobRunning}
		job.MarkFailed(errors.New("something went wrong"))

		if job.Status != JobFailed {
			t.Errorf("Status = %v, want %v", job.Status, JobFailed)
		}
		if job.CompletedAt == nil {
			t.Error("CompletedAt should be set")
		}
		if job.Error != "something went wrong" {
			t.Errorf("Error = %q, want 'something went wrong'", job.Error)
		}
	})

	t.Run("with nil error", func(t *testing.T) {
		job := &Job{Status: JobRunning}
		job.MarkFailed(nil)

		if job.Status != JobFailed {
			t.Errorf("Status = %v, want %v", job.Status, JobFailed)
		}
		if job.Error != "" {
			t.Errorf("Error = %q, want empty", job.Error)
		}
	})
}

func TestJobMarkCancelled(t *testing.T) {
	job := &Job{Status: JobRunning}
	job.MarkCancelled()

	if job.Status != JobCancelled {
		t.Errorf("Status = %v, want %v", job.Status, JobCancelled)
	}
	if job.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
}

func TestJobSetProgress(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{0, 0},
		{50, 50},
		{100, 100},
		{-10, 0},   // Clamp to 0
		{150, 100}, // Clamp to 100
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			job := &Job{}
			job.SetProgress(tt.input)
			if job.Progress != tt.expected {
				t.Errorf("SetProgress(%d) = %d, want %d", tt.input, job.Progress, tt.expected)
			}
		})
	}
}

func TestJobDuration(t *testing.T) {
	t.Run("not started", func(t *testing.T) {
		job := &Job{}
		if d := job.Duration(); d != 0 {
			t.Errorf("Duration() = %v, want 0", d)
		}
	})

	t.Run("running", func(t *testing.T) {
		now := time.Now().UTC()
		past := now.Add(-5 * time.Second)
		job := &Job{StartedAt: &past}
		d := job.Duration()
		if d < 5*time.Second {
			t.Errorf("Duration() = %v, want >= 5s", d)
		}
	})

	t.Run("completed", func(t *testing.T) {
		start := time.Now().UTC().Add(-10 * time.Second)
		end := time.Now().UTC().Add(-5 * time.Second)
		job := &Job{StartedAt: &start, CompletedAt: &end}
		d := job.Duration()
		expected := 5 * time.Second
		if d < expected-time.Millisecond || d > expected+time.Millisecond {
			t.Errorf("Duration() = %v, want ~%v", d, expected)
		}
	})
}

func TestJobToSummary(t *testing.T) {
	now := time.Now().UTC()
	job := &Job{
		ID:          "job-123",
		Type:        JobTypeExport,
		Status:      JobCompleted,
		Progress:    100,
		CreatedAt:   now,
		CompletedAt: &now,
		Error:       "",
	}

	summary := job.ToSummary()

	if summary.ID != "job-123" {
		t.Errorf("ID = %q, want 'job-123'", summary.ID)
	}
	if summary.Type != JobTypeExport {
		t.Errorf("Type = %v, want %v", summary.Type, JobTypeExport)
	}
	if summary.Status != JobCompleted {
		t.Errorf("Status = %v, want %v", summary.Status, JobCompleted)
	}
	if summary.Progress != 100 {
		t.Errorf("Progress = %d, want 100", summary.Progress)
	}
}

// Test scope parsing functions

func TestParseRefreshScope(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		scope, err := ParseRefreshScope("")
		if err != nil {
			t.Fatalf("ParseRefreshScope() error = %v", err)
		}
		if scope.Scope != "all" {
			t.Errorf("Scope = %q, want 'all'", scope.Scope)
		}
	})

	t.Run("valid JSON", func(t *testing.T) {
		scope, err := ParseRefreshScope(`{"scope":"modules","force":true}`)
		if err != nil {
			t.Fatalf("ParseRefreshScope() error = %v", err)
		}
		if scope.Scope != "modules" {
			t.Errorf("Scope = %q, want 'modules'", scope.Scope)
		}
		if !scope.Force {
			t.Error("Force should be true")
		}
	})

	t.Run("empty scope in JSON", func(t *testing.T) {
		scope, err := ParseRefreshScope(`{"force":true}`)
		if err != nil {
			t.Fatalf("ParseRefreshScope() error = %v", err)
		}
		if scope.Scope != "all" {
			t.Errorf("Scope = %q, want 'all' (default)", scope.Scope)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		_, err := ParseRefreshScope(`{invalid}`)
		if err == nil {
			t.Error("Expected error for invalid JSON")
		}
	})
}

func TestParseAnalyzeImpactScope(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		scope, err := ParseAnalyzeImpactScope("")
		if err != nil {
			t.Fatalf("ParseAnalyzeImpactScope() error = %v", err)
		}
		if scope != nil {
			t.Errorf("Expected nil for empty string")
		}
	})

	t.Run("valid JSON", func(t *testing.T) {
		scope, err := ParseAnalyzeImpactScope(`{"symbolId":"sym:123","depth":3}`)
		if err != nil {
			t.Fatalf("ParseAnalyzeImpactScope() error = %v", err)
		}
		if scope.SymbolID != "sym:123" {
			t.Errorf("SymbolID = %q, want 'sym:123'", scope.SymbolID)
		}
		if scope.Depth != 3 {
			t.Errorf("Depth = %d, want 3", scope.Depth)
		}
	})

	t.Run("default depth", func(t *testing.T) {
		scope, err := ParseAnalyzeImpactScope(`{"symbolId":"sym:123"}`)
		if err != nil {
			t.Fatalf("ParseAnalyzeImpactScope() error = %v", err)
		}
		if scope.Depth != 2 {
			t.Errorf("Depth = %d, want 2 (default)", scope.Depth)
		}
	})
}

func TestParseExportScope(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		scope, err := ParseExportScope("")
		if err != nil {
			t.Fatalf("ParseExportScope() error = %v", err)
		}
		if scope.Format != "json" {
			t.Errorf("Format = %q, want 'json' (default)", scope.Format)
		}
	})

	t.Run("valid JSON", func(t *testing.T) {
		scope, err := ParseExportScope(`{"format":"markdown","target":"output.md"}`)
		if err != nil {
			t.Fatalf("ParseExportScope() error = %v", err)
		}
		if scope.Format != "markdown" {
			t.Errorf("Format = %q, want 'markdown'", scope.Format)
		}
		if scope.Target != "output.md" {
			t.Errorf("Target = %q, want 'output.md'", scope.Target)
		}
	})

	t.Run("default format", func(t *testing.T) {
		scope, err := ParseExportScope(`{"target":"output.json"}`)
		if err != nil {
			t.Fatalf("ParseExportScope() error = %v", err)
		}
		if scope.Format != "json" {
			t.Errorf("Format = %q, want 'json' (default)", scope.Format)
		}
	})
}

func TestParseFederationSyncScope(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		scope, err := ParseFederationSyncScope("")
		if err != nil {
			t.Fatalf("ParseFederationSyncScope() error = %v", err)
		}
		if scope != nil {
			t.Error("Expected nil for empty string")
		}
	})

	t.Run("valid JSON", func(t *testing.T) {
		scope, err := ParseFederationSyncScope(`{"federationName":"my-fed","direction":"push","repoId":"repo-1"}`)
		if err != nil {
			t.Fatalf("ParseFederationSyncScope() error = %v", err)
		}
		if scope.FederationName != "my-fed" {
			t.Errorf("FederationName = %q, want 'my-fed'", scope.FederationName)
		}
		if scope.Direction != "push" {
			t.Errorf("Direction = %q, want 'push'", scope.Direction)
		}
		if scope.RepoID != "repo-1" {
			t.Errorf("RepoID = %q, want 'repo-1'", scope.RepoID)
		}
	})

	t.Run("default direction", func(t *testing.T) {
		scope, err := ParseFederationSyncScope(`{"federationName":"my-fed"}`)
		if err != nil {
			t.Fatalf("ParseFederationSyncScope() error = %v", err)
		}
		if scope.Direction != "pull" {
			t.Errorf("Direction = %q, want 'pull' (default)", scope.Direction)
		}
	})
}

func TestParseWebhookDispatchScope(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		scope, err := ParseWebhookDispatchScope("")
		if err != nil {
			t.Fatalf("ParseWebhookDispatchScope() error = %v", err)
		}
		if scope != nil {
			t.Error("Expected nil for empty string")
		}
	})

	t.Run("valid JSON", func(t *testing.T) {
		scope, err := ParseWebhookDispatchScope(`{"webhookId":"wh-1","eventId":"ev-1","eventType":"push","payload":"{}"}`)
		if err != nil {
			t.Fatalf("ParseWebhookDispatchScope() error = %v", err)
		}
		if scope.WebhookID != "wh-1" {
			t.Errorf("WebhookID = %q, want 'wh-1'", scope.WebhookID)
		}
		if scope.EventType != "push" {
			t.Errorf("EventType = %q, want 'push'", scope.EventType)
		}
	})
}

func TestParseScheduledTaskScope(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		scope, err := ParseScheduledTaskScope("")
		if err != nil {
			t.Fatalf("ParseScheduledTaskScope() error = %v", err)
		}
		if scope != nil {
			t.Error("Expected nil for empty string")
		}
	})

	t.Run("valid JSON", func(t *testing.T) {
		scope, err := ParseScheduledTaskScope(`{"scheduleId":"sched-1","taskType":"refresh","target":"repo-1"}`)
		if err != nil {
			t.Fatalf("ParseScheduledTaskScope() error = %v", err)
		}
		if scope.ScheduleID != "sched-1" {
			t.Errorf("ScheduleID = %q, want 'sched-1'", scope.ScheduleID)
		}
		if scope.TaskType != "refresh" {
			t.Errorf("TaskType = %q, want 'refresh'", scope.TaskType)
		}
		if scope.Target != "repo-1" {
			t.Errorf("Target = %q, want 'repo-1'", scope.Target)
		}
	})
}

func TestJobStatusConstants(t *testing.T) {
	statuses := []JobStatus{JobQueued, JobRunning, JobCompleted, JobFailed, JobCancelled}
	for _, s := range statuses {
		if string(s) == "" {
			t.Errorf("JobStatus %v should not be empty", s)
		}
	}
}

func TestJobTypeConstants(t *testing.T) {
	types := []JobType{
		JobTypeRefreshArchitecture,
		JobTypeAnalyzeImpact,
		JobTypeExport,
		JobTypeFederationSync,
		JobTypeWebhookDispatch,
		JobTypeScheduledTask,
	}
	for _, jt := range types {
		if string(jt) == "" {
			t.Errorf("JobType %v should not be empty", jt)
		}
	}
}

func TestListJobsOptions(t *testing.T) {
	opts := ListJobsOptions{
		Status: []JobStatus{JobQueued, JobRunning},
		Type:   []JobType{JobTypeExport},
		Limit:  10,
		Offset: 20,
	}

	if len(opts.Status) != 2 {
		t.Errorf("Status len = %d, want 2", len(opts.Status))
	}
	if opts.Limit != 10 {
		t.Errorf("Limit = %d, want 10", opts.Limit)
	}
}

func TestListJobsResponse(t *testing.T) {
	resp := ListJobsResponse{
		Jobs: []JobSummary{
			{ID: "job-1", Status: JobQueued},
			{ID: "job-2", Status: JobRunning},
		},
		TotalCount: 100,
	}

	if len(resp.Jobs) != 2 {
		t.Errorf("Jobs len = %d, want 2", len(resp.Jobs))
	}
	if resp.TotalCount != 100 {
		t.Errorf("TotalCount = %d, want 100", resp.TotalCount)
	}
}
