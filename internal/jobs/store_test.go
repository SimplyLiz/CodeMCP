package jobs

import (
	"context"
	"testing"
	"time"

	"ckb/internal/logging"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	tmpDir := t.TempDir()
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})

	store, err := OpenStore(tmpDir, logger)
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestOpenStore(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})

	store, err := OpenStore(tmpDir, logger)
	if err != nil {
		t.Fatalf("OpenStore failed: %v", err)
	}
	defer store.Close()

	if store.conn == nil {
		t.Error("conn should not be nil")
	}
}

func TestOpenStore_Twice(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})

	// First open creates database
	store1, err := OpenStore(tmpDir, logger)
	if err != nil {
		t.Fatalf("First OpenStore failed: %v", err)
	}
	store1.Close()

	// Second open uses existing database
	store2, err := OpenStore(tmpDir, logger)
	if err != nil {
		t.Fatalf("Second OpenStore failed: %v", err)
	}
	defer store2.Close()
}

func TestStore_CreateAndGetJob(t *testing.T) {
	store := newTestStore(t)

	job, err := NewJob(JobTypeExport, nil)
	if err != nil {
		t.Fatalf("NewJob failed: %v", err)
	}

	if err := store.CreateJob(job); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	// Get the job back
	retrieved, err := store.GetJob(job.ID)
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}

	if retrieved.ID != job.ID {
		t.Errorf("ID = %q, want %q", retrieved.ID, job.ID)
	}
	if retrieved.Type != job.Type {
		t.Errorf("Type = %v, want %v", retrieved.Type, job.Type)
	}
	if retrieved.Status != JobQueued {
		t.Errorf("Status = %v, want %v", retrieved.Status, JobQueued)
	}
}

func TestStore_GetJob_NotFound(t *testing.T) {
	store := newTestStore(t)

	// GetJob returns (nil, nil) for not found, not an error
	job, err := store.GetJob("nonexistent-id")
	if err != nil {
		t.Errorf("GetJob error = %v, want nil", err)
	}
	if job != nil {
		t.Errorf("GetJob = %v, want nil for nonexistent", job)
	}
}

func TestStore_UpdateJob(t *testing.T) {
	store := newTestStore(t)

	job, _ := NewJob(JobTypeExport, nil)
	if err := store.CreateJob(job); err != nil {
		t.Fatal(err)
	}

	// Update the job
	job.MarkStarted()
	job.SetProgress(50)
	if err := store.UpdateJob(job); err != nil {
		t.Fatalf("UpdateJob failed: %v", err)
	}

	// Verify update
	retrieved, err := store.GetJob(job.ID)
	if err != nil {
		t.Fatal(err)
	}

	if retrieved.Status != JobRunning {
		t.Errorf("Status = %v, want %v", retrieved.Status, JobRunning)
	}
	if retrieved.Progress != 50 {
		t.Errorf("Progress = %d, want 50", retrieved.Progress)
	}
	if retrieved.StartedAt == nil {
		t.Error("StartedAt should be set")
	}
}

func TestStore_UpdateJob_Complete(t *testing.T) {
	store := newTestStore(t)

	job, _ := NewJob(JobTypeExport, nil)
	if err := store.CreateJob(job); err != nil {
		t.Fatal(err)
	}

	job.MarkStarted()
	_ = job.MarkCompleted(RefreshResult{Status: "done"})
	if err := store.UpdateJob(job); err != nil {
		t.Fatalf("UpdateJob failed: %v", err)
	}

	retrieved, _ := store.GetJob(job.ID)
	if retrieved.Status != JobCompleted {
		t.Errorf("Status = %v, want %v", retrieved.Status, JobCompleted)
	}
	if retrieved.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
	if retrieved.Result == "" {
		t.Error("Result should be set")
	}
}

func TestStore_UpdateJob_Failed(t *testing.T) {
	store := newTestStore(t)

	job, _ := NewJob(JobTypeExport, nil)
	if err := store.CreateJob(job); err != nil {
		t.Fatal(err)
	}

	job.MarkStarted()
	job.MarkFailed(context.DeadlineExceeded)
	if err := store.UpdateJob(job); err != nil {
		t.Fatalf("UpdateJob failed: %v", err)
	}

	retrieved, _ := store.GetJob(job.ID)
	if retrieved.Status != JobFailed {
		t.Errorf("Status = %v, want %v", retrieved.Status, JobFailed)
	}
	if retrieved.Error == "" {
		t.Error("Error should be set")
	}
}

func TestStore_ListJobs_Empty(t *testing.T) {
	store := newTestStore(t)

	resp, err := store.ListJobs(ListJobsOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}

	if len(resp.Jobs) != 0 {
		t.Errorf("Jobs len = %d, want 0", len(resp.Jobs))
	}
	if resp.TotalCount != 0 {
		t.Errorf("TotalCount = %d, want 0", resp.TotalCount)
	}
}

func TestStore_ListJobs_Multiple(t *testing.T) {
	store := newTestStore(t)

	// Create several jobs
	for i := 0; i < 5; i++ {
		job, _ := NewJob(JobTypeExport, nil)
		if err := store.CreateJob(job); err != nil {
			t.Fatal(err)
		}
	}

	resp, err := store.ListJobs(ListJobsOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListJobs failed: %v", err)
	}

	if len(resp.Jobs) != 5 {
		t.Errorf("Jobs len = %d, want 5", len(resp.Jobs))
	}
	if resp.TotalCount != 5 {
		t.Errorf("TotalCount = %d, want 5", resp.TotalCount)
	}
}

func TestStore_ListJobs_FilterByStatus(t *testing.T) {
	store := newTestStore(t)

	// Create jobs with different statuses
	queued, _ := NewJob(JobTypeExport, nil)
	_ = store.CreateJob(queued)

	running, _ := NewJob(JobTypeExport, nil)
	running.MarkStarted()
	_ = store.CreateJob(running)
	_ = store.UpdateJob(running)

	completed, _ := NewJob(JobTypeExport, nil)
	completed.MarkStarted()
	_ = completed.MarkCompleted(nil)
	_ = store.CreateJob(completed)
	_ = store.UpdateJob(completed)

	// Filter by queued
	resp, err := store.ListJobs(ListJobsOptions{
		Status: []JobStatus{JobQueued},
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Jobs) != 1 {
		t.Errorf("Queued jobs = %d, want 1", len(resp.Jobs))
	}

	// Filter by running
	resp, err = store.ListJobs(ListJobsOptions{
		Status: []JobStatus{JobRunning},
		Limit:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Jobs) != 1 {
		t.Errorf("Running jobs = %d, want 1", len(resp.Jobs))
	}
}

func TestStore_ListJobs_FilterByType(t *testing.T) {
	store := newTestStore(t)

	export, _ := NewJob(JobTypeExport, nil)
	_ = store.CreateJob(export)

	refresh, _ := NewJob(JobTypeRefreshArchitecture, nil)
	_ = store.CreateJob(refresh)

	resp, err := store.ListJobs(ListJobsOptions{
		Type:  []JobType{JobTypeExport},
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Jobs) != 1 {
		t.Errorf("Export jobs = %d, want 1", len(resp.Jobs))
	}
}

func TestStore_ListJobs_Pagination(t *testing.T) {
	store := newTestStore(t)

	// Create 10 jobs
	for i := 0; i < 10; i++ {
		job, _ := NewJob(JobTypeExport, nil)
		_ = store.CreateJob(job)
	}

	// Get first page
	resp, _ := store.ListJobs(ListJobsOptions{Limit: 3, Offset: 0})
	if len(resp.Jobs) != 3 {
		t.Errorf("First page = %d jobs, want 3", len(resp.Jobs))
	}
	if resp.TotalCount != 10 {
		t.Errorf("TotalCount = %d, want 10", resp.TotalCount)
	}

	// Get second page
	resp, _ = store.ListJobs(ListJobsOptions{Limit: 3, Offset: 3})
	if len(resp.Jobs) != 3 {
		t.Errorf("Second page = %d jobs, want 3", len(resp.Jobs))
	}
}

func TestStore_GetPendingJobs(t *testing.T) {
	store := newTestStore(t)

	// Create queued, running, and completed jobs
	queued, _ := NewJob(JobTypeExport, nil)
	_ = store.CreateJob(queued)

	running, _ := NewJob(JobTypeExport, nil)
	running.MarkStarted()
	_ = store.CreateJob(running)
	_ = store.UpdateJob(running)

	completed, _ := NewJob(JobTypeExport, nil)
	completed.MarkStarted()
	_ = completed.MarkCompleted(nil)
	_ = store.CreateJob(completed)
	_ = store.UpdateJob(completed)

	pending, err := store.GetPendingJobs()
	if err != nil {
		t.Fatalf("GetPendingJobs failed: %v", err)
	}

	// GetPendingJobs only returns queued jobs (not running)
	if len(pending) != 1 {
		t.Errorf("Pending jobs = %d, want 1 (only queued)", len(pending))
	}
	if len(pending) > 0 && pending[0].ID != queued.ID {
		t.Errorf("Expected queued job, got %v", pending[0].ID)
	}
}

func TestStore_CleanupOldJobs(t *testing.T) {
	store := newTestStore(t)

	// Create a completed job (completed_at = now)
	job, _ := NewJob(JobTypeExport, nil)
	job.MarkStarted()
	_ = job.MarkCompleted(nil)
	_ = store.CreateJob(job)
	_ = store.UpdateJob(job)

	// Cleanup with 0 duration won't delete the job since its completed_at
	// is equal to now (not less than cutoff). This tests the function runs correctly.
	deleted, err := store.CleanupOldJobs(0)
	if err != nil {
		t.Fatalf("CleanupOldJobs failed: %v", err)
	}
	// Job's completed_at == now, cutoff == now, so completed_at < cutoff is false
	// The job won't be deleted since it's not strictly older than the cutoff
	if deleted != 0 {
		t.Errorf("Deleted = %d, want 0 (job not old enough)", deleted)
	}

	// Verify job still exists
	retrieved, _ := store.GetJob(job.ID)
	if retrieved == nil {
		t.Error("Job should still exist (not old enough)")
	}
}

func TestStore_FileChecksums(t *testing.T) {
	store := newTestStore(t)

	path := "/test/file.go"
	checksum := "abc123"

	// Save checksum
	fc := &FileChecksum{
		Path:        path,
		Checksum:    checksum,
		LastIndexed: time.Now(),
	}
	if err := store.SaveFileChecksum(fc); err != nil {
		t.Fatalf("SaveFileChecksum failed: %v", err)
	}

	// Get checksum
	retrieved, err := store.GetFileChecksum(path)
	if err != nil {
		t.Fatalf("GetFileChecksum failed: %v", err)
	}
	if retrieved == nil {
		t.Fatal("GetFileChecksum returned nil")
	}
	if retrieved.Checksum != checksum {
		t.Errorf("Checksum = %q, want %q", retrieved.Checksum, checksum)
	}
	if retrieved.LastIndexed.IsZero() {
		t.Error("LastIndexed should not be zero")
	}
}

func TestStore_FileChecksums_NotFound(t *testing.T) {
	store := newTestStore(t)

	fc, err := store.GetFileChecksum("/nonexistent")
	if err != nil {
		t.Fatalf("GetFileChecksum failed: %v", err)
	}
	if fc != nil {
		t.Errorf("FileChecksum = %v, want nil for nonexistent", fc)
	}
}

func TestStore_FileChecksums_Update(t *testing.T) {
	store := newTestStore(t)

	path := "/test/file.go"

	// Save first checksum
	_ = store.SaveFileChecksum(&FileChecksum{
		Path:        path,
		Checksum:    "first",
		LastIndexed: time.Now(),
	})

	// Update with new checksum
	_ = store.SaveFileChecksum(&FileChecksum{
		Path:        path,
		Checksum:    "second",
		LastIndexed: time.Now(),
	})

	// Verify update
	fc, _ := store.GetFileChecksum(path)
	if fc == nil || fc.Checksum != "second" {
		t.Errorf("Checksum = %v, want 'second'", fc)
	}
}

func TestStore_Close(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})

	store, _ := OpenStore(tmpDir, logger)

	if err := store.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}

	// Close again should not error
	if err := store.Close(); err != nil {
		t.Errorf("Second Close failed: %v", err)
	}
}

func TestStore_CreateJob_WithScope(t *testing.T) {
	store := newTestStore(t)

	scope := RefreshScope{Scope: "modules", Force: true}
	job, err := NewJob(JobTypeRefreshArchitecture, scope)
	if err != nil {
		t.Fatal(err)
	}

	if err := store.CreateJob(job); err != nil {
		t.Fatalf("CreateJob failed: %v", err)
	}

	retrieved, _ := store.GetJob(job.ID)
	if retrieved.Scope == "" {
		t.Error("Scope should be preserved")
	}
}

func TestStore_ListJobs_DefaultLimit(t *testing.T) {
	store := newTestStore(t)

	// Create 100 jobs
	for i := 0; i < 100; i++ {
		job, _ := NewJob(JobTypeExport, nil)
		_ = store.CreateJob(job)
	}

	// List with no limit should use default (50)
	resp, _ := store.ListJobs(ListJobsOptions{})
	if len(resp.Jobs) > 50 {
		t.Errorf("Jobs len = %d, should respect default limit", len(resp.Jobs))
	}
}

func TestNullString(t *testing.T) {
	tests := []struct {
		input    string
		expected bool // Valid
	}{
		{"", false},
		{"test", true},
	}

	for _, tt := range tests {
		ns := nullString(tt.input)
		if ns.Valid != tt.expected {
			t.Errorf("nullString(%q).Valid = %v, want %v", tt.input, ns.Valid, tt.expected)
		}
		if tt.expected && ns.String != tt.input {
			t.Errorf("nullString(%q).String = %q, want %q", tt.input, ns.String, tt.input)
		}
	}
}

func TestNullTime(t *testing.T) {
	// Nil time
	ns := nullTime(nil)
	if ns.Valid {
		t.Error("nullTime(nil).Valid = true, want false")
	}

	// Non-nil time
	now := time.Now()
	ns = nullTime(&now)
	if !ns.Valid {
		t.Error("nullTime(&now).Valid = false, want true")
	}
	if ns.String == "" {
		t.Error("nullTime(&now).String should not be empty")
	}
}
