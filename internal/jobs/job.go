// Package jobs provides background job processing for long-running operations.
package jobs

import (
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// JobStatus represents the current state of a job.
type JobStatus string

const (
	JobQueued    JobStatus = "queued"
	JobRunning   JobStatus = "running"
	JobCompleted JobStatus = "completed"
	JobFailed    JobStatus = "failed"
	JobCancelled JobStatus = "cancelled"
)

// JobType identifies the kind of work a job performs.
type JobType string

const (
	JobTypeRefreshArchitecture JobType = "refresh_architecture"
	JobTypeAnalyzeImpact       JobType = "analyze_impact"
	JobTypeExport              JobType = "export"
	JobTypeFederationSync      JobType = "federation_sync"
	JobTypeWebhookDispatch     JobType = "webhook_dispatch"
	JobTypeScheduledTask       JobType = "scheduled_task"
)

// Job represents a background task with its state and metadata.
type Job struct {
	ID          string     `json:"id"`
	Type        JobType    `json:"type"`
	Scope       string     `json:"scope,omitempty"` // JSON-encoded scope parameters
	Status      JobStatus  `json:"status"`
	Progress    int        `json:"progress"` // 0-100
	CreatedAt   time.Time  `json:"createdAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Error       string     `json:"error,omitempty"`
	Result      string     `json:"result,omitempty"` // JSON-encoded result
}

// NewJob creates a new job with the given type and scope.
func NewJob(jobType JobType, scope interface{}) (*Job, error) {
	var scopeJSON string
	if scope != nil {
		data, err := json.Marshal(scope)
		if err != nil {
			return nil, err
		}
		scopeJSON = string(data)
	}

	return &Job{
		ID:        uuid.New().String(),
		Type:      jobType,
		Scope:     scopeJSON,
		Status:    JobQueued,
		Progress:  0,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// IsTerminal returns true if the job is in a terminal state.
func (j *Job) IsTerminal() bool {
	return j.Status == JobCompleted || j.Status == JobFailed || j.Status == JobCancelled
}

// CanCancel returns true if the job can be cancelled.
func (j *Job) CanCancel() bool {
	return j.Status == JobQueued || j.Status == JobRunning
}

// MarkStarted transitions the job to running state.
func (j *Job) MarkStarted() {
	now := time.Now().UTC()
	j.Status = JobRunning
	j.StartedAt = &now
}

// MarkCompleted transitions the job to completed state with result.
func (j *Job) MarkCompleted(result interface{}) error {
	now := time.Now().UTC()
	j.Status = JobCompleted
	j.Progress = 100
	j.CompletedAt = &now

	if result != nil {
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		j.Result = string(data)
	}
	return nil
}

// MarkFailed transitions the job to failed state with error.
func (j *Job) MarkFailed(err error) {
	now := time.Now().UTC()
	j.Status = JobFailed
	j.CompletedAt = &now
	if err != nil {
		j.Error = err.Error()
	}
}

// MarkCancelled transitions the job to cancelled state.
func (j *Job) MarkCancelled() {
	now := time.Now().UTC()
	j.Status = JobCancelled
	j.CompletedAt = &now
}

// SetProgress updates the job's progress (0-100).
func (j *Job) SetProgress(progress int) {
	if progress < 0 {
		progress = 0
	}
	if progress > 100 {
		progress = 100
	}
	j.Progress = progress
}

// Duration returns how long the job took (or has been running).
func (j *Job) Duration() time.Duration {
	if j.StartedAt == nil {
		return 0
	}
	endTime := time.Now().UTC()
	if j.CompletedAt != nil {
		endTime = *j.CompletedAt
	}
	return endTime.Sub(*j.StartedAt)
}

// JobSummary is a lightweight view of a job for listing.
type JobSummary struct {
	ID          string     `json:"id"`
	Type        JobType    `json:"type"`
	Status      JobStatus  `json:"status"`
	Progress    int        `json:"progress"`
	CreatedAt   time.Time  `json:"createdAt"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
	Error       string     `json:"error,omitempty"`
}

// ToSummary creates a summary view of the job.
func (j *Job) ToSummary() JobSummary {
	return JobSummary{
		ID:          j.ID,
		Type:        j.Type,
		Status:      j.Status,
		Progress:    j.Progress,
		CreatedAt:   j.CreatedAt,
		CompletedAt: j.CompletedAt,
		Error:       j.Error,
	}
}

// ListJobsOptions contains options for listing jobs.
type ListJobsOptions struct {
	Status []JobStatus
	Type   []JobType
	Limit  int
	Offset int
}

// ListJobsResponse contains the result of listing jobs.
type ListJobsResponse struct {
	Jobs       []JobSummary `json:"jobs"`
	TotalCount int          `json:"totalCount"`
}
