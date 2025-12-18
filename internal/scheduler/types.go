// Package scheduler provides task scheduling for automated operations.
package scheduler

import (
	"encoding/json"
	"time"
)

// TaskType represents the type of scheduled task
type TaskType string

const (
	TaskTypeRefresh        TaskType = "refresh"
	TaskTypeFederationSync TaskType = "federation_sync"
	TaskTypeCleanup        TaskType = "cleanup"
	TaskTypeHealthCheck    TaskType = "health_check"
)

// Schedule represents a scheduled task
type Schedule struct {
	ID           string     `json:"id"`
	TaskType     TaskType   `json:"taskType"`
	Target       string     `json:"target,omitempty"`       // repoId or federationName
	Expression   string     `json:"expression"`             // cron or "every Xh"
	Enabled      bool       `json:"enabled"`
	NextRun      time.Time  `json:"nextRun"`
	LastRun      *time.Time `json:"lastRun,omitempty"`
	LastStatus   string     `json:"lastStatus,omitempty"`   // "success", "failed"
	LastDuration int64      `json:"lastDuration,omitempty"` // milliseconds
	LastError    string     `json:"lastError,omitempty"`
	CreatedAt    time.Time  `json:"createdAt"`
	UpdatedAt    time.Time  `json:"updatedAt"`
}

// ScheduleSummary is a lightweight view for listing
type ScheduleSummary struct {
	ID         string    `json:"id"`
	TaskType   TaskType  `json:"taskType"`
	Target     string    `json:"target,omitempty"`
	Expression string    `json:"expression"`
	Enabled    bool      `json:"enabled"`
	NextRun    time.Time `json:"nextRun"`
	LastStatus string    `json:"lastStatus,omitempty"`
}

// ToSummary creates a summary view
func (s *Schedule) ToSummary() ScheduleSummary {
	return ScheduleSummary{
		ID:         s.ID,
		TaskType:   s.TaskType,
		Target:     s.Target,
		Expression: s.Expression,
		Enabled:    s.Enabled,
		NextRun:    s.NextRun,
		LastStatus: s.LastStatus,
	}
}

// ScheduleConfig defines a schedule in configuration
type ScheduleConfig struct {
	ID         string            `json:"id" mapstructure:"id"`
	TaskType   string            `json:"taskType" mapstructure:"task_type"`
	Target     string            `json:"target,omitempty" mapstructure:"target"`
	Expression string            `json:"expression" mapstructure:"expression"`
	Enabled    bool              `json:"enabled" mapstructure:"enabled"`
	Params     map[string]string `json:"params,omitempty" mapstructure:"params"`
}

// ListSchedulesOptions contains options for listing schedules
type ListSchedulesOptions struct {
	TaskType []TaskType `json:"taskType,omitempty"`
	Enabled  *bool      `json:"enabled,omitempty"`
	Limit    int        `json:"limit,omitempty"`
	Offset   int        `json:"offset,omitempty"`
}

// ListSchedulesResponse contains the result of listing schedules
type ListSchedulesResponse struct {
	Schedules  []ScheduleSummary `json:"schedules"`
	TotalCount int               `json:"totalCount"`
}

// ScheduleRun represents a single execution of a schedule
type ScheduleRun struct {
	ID         string    `json:"id"`
	ScheduleID string    `json:"scheduleId"`
	JobID      string    `json:"jobId,omitempty"`
	StartedAt  time.Time `json:"startedAt"`
	EndedAt    *time.Time `json:"endedAt,omitempty"`
	Status     string    `json:"status"` // "running", "success", "failed"
	Error      string    `json:"error,omitempty"`
	Duration   int64     `json:"duration,omitempty"` // milliseconds
}

// NewSchedule creates a new schedule
func NewSchedule(taskType TaskType, target, expression string) (*Schedule, error) {
	id := generateScheduleID()
	now := time.Now()

	// Calculate next run time
	nextRun, err := NextRunTime(expression, now)
	if err != nil {
		return nil, err
	}

	return &Schedule{
		ID:         id,
		TaskType:   taskType,
		Target:     target,
		Expression: expression,
		Enabled:    true,
		NextRun:    nextRun,
		CreatedAt:  now,
		UpdatedAt:  now,
	}, nil
}

// IsDue returns true if the schedule should run now
func (s *Schedule) IsDue() bool {
	if !s.Enabled {
		return false
	}
	return time.Now().After(s.NextRun) || time.Now().Equal(s.NextRun)
}

// MarkRun updates the schedule after a run
func (s *Schedule) MarkRun(success bool, duration time.Duration, errMsg string) error {
	now := time.Now()
	s.LastRun = &now
	s.LastDuration = duration.Milliseconds()
	s.UpdatedAt = now

	if success {
		s.LastStatus = "success"
		s.LastError = ""
	} else {
		s.LastStatus = "failed"
		s.LastError = errMsg
	}

	// Calculate next run
	nextRun, err := NextRunTime(s.Expression, now)
	if err != nil {
		return err
	}
	s.NextRun = nextRun

	return nil
}

// ToJSON returns JSON representation
func (s *Schedule) ToJSON() (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// generateScheduleID generates a unique schedule ID
func generateScheduleID() string {
	timestamp := time.Now().UnixNano()
	random := randomHex(3)
	return "sched_" + formatBase36(timestamp) + "_" + random
}

func formatBase36(n int64) string {
	const base = "0123456789abcdefghijklmnopqrstuvwxyz"
	if n == 0 {
		return "0"
	}

	result := make([]byte, 0, 13)
	for n > 0 {
		result = append(result, base[n%36])
		n /= 36
	}

	// Reverse
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}

func randomHex(n int) string {
	const hex = "0123456789abcdef"
	result := make([]byte, n*2)

	seed := time.Now().UnixNano()
	for i := range result {
		seed = seed*1103515245 + 12345
		result[i] = hex[(seed>>16)&0xf]
	}

	return string(result)
}
