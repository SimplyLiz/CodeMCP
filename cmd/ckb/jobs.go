package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/jobs"
)

var (
	jobsFormat string
	jobsLimit  int
	jobsStatus string
	jobsType   string
)

var jobsCmd = &cobra.Command{
	Use:   "jobs",
	Short: "Manage background jobs",
	Long: `List, check status, and manage background jobs.

Background jobs are used for async operations like architecture refresh,
federation sync, and impact analysis.

Examples:
  ckb jobs list
  ckb jobs status <job-id>
  ckb jobs cancel <job-id>`,
}

var jobsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List recent background jobs",
	Long: `List recent background jobs with optional filtering.

Examples:
  ckb jobs list
  ckb jobs list --status=running
  ckb jobs list --type=refresh_architecture
  ckb jobs list --limit=50`,
	Run: runJobsList,
}

var jobsStatusCmd = &cobra.Command{
	Use:   "status <job-id>",
	Short: "Get status of a specific job",
	Long: `Get detailed status and result of a background job.

Examples:
  ckb jobs status job_abc123`,
	Args: cobra.ExactArgs(1),
	Run:  runJobsStatus,
}

var jobsCancelCmd = &cobra.Command{
	Use:   "cancel <job-id>",
	Short: "Cancel a queued or running job",
	Long: `Cancel a background job that is queued or running.

Examples:
  ckb jobs cancel job_abc123`,
	Args: cobra.ExactArgs(1),
	Run:  runJobsCancel,
}

func init() {
	// List flags
	jobsListCmd.Flags().StringVar(&jobsFormat, "format", "json", "Output format (json, human)")
	jobsListCmd.Flags().IntVar(&jobsLimit, "limit", 20, "Maximum jobs to return")
	jobsListCmd.Flags().StringVar(&jobsStatus, "status", "", "Filter by status (queued, running, completed, failed, cancelled)")
	jobsListCmd.Flags().StringVar(&jobsType, "type", "", "Filter by type (refresh_architecture, analyze_impact, export)")

	// Status flags
	jobsStatusCmd.Flags().StringVar(&jobsFormat, "format", "json", "Output format (json, human)")

	// Cancel flags
	jobsCancelCmd.Flags().StringVar(&jobsFormat, "format", "json", "Output format (json, human)")

	jobsCmd.AddCommand(jobsListCmd)
	jobsCmd.AddCommand(jobsStatusCmd)
	jobsCmd.AddCommand(jobsCancelCmd)
	rootCmd.AddCommand(jobsCmd)
}

func runJobsList(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(jobsFormat)

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	opts := jobs.ListJobsOptions{
		Limit: jobsLimit,
	}

	if jobsStatus != "" {
		opts.Status = []jobs.JobStatus{jobs.JobStatus(jobsStatus)}
	}
	if jobsType != "" {
		opts.Type = []jobs.JobType{jobs.JobType(jobsType)}
	}

	response, err := engine.ListJobs(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing jobs: %v\n", err)
		os.Exit(1)
	}

	cliResponse := convertJobsListResponse(response)

	output, err := FormatResponse(cliResponse, OutputFormat(jobsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Jobs list completed", map[string]interface{}{
		"count":    len(response.Jobs),
		"duration": time.Since(start).Milliseconds(),
	})
}

func runJobsStatus(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(jobsFormat)
	jobId := args[0]

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	job, err := engine.GetJob(jobId)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting job status: %v\n", err)
		os.Exit(1)
	}

	cliResponse := convertJobResponse(job)

	output, err := FormatResponse(cliResponse, OutputFormat(jobsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Job status completed", map[string]interface{}{
		"jobId":    jobId,
		"status":   job.Status,
		"duration": time.Since(start).Milliseconds(),
	})
}

func runJobsCancel(cmd *cobra.Command, args []string) {
	start := time.Now()
	logger := newLogger(jobsFormat)
	jobId := args[0]

	repoRoot := mustGetRepoRoot()
	engine := mustGetEngine(repoRoot, logger)

	err := engine.CancelJob(jobId)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error cancelling job: %v\n", err)
		os.Exit(1)
	}

	cliResponse := &JobCancelResponseCLI{
		JobId:     jobId,
		Cancelled: true,
		Message:   "Job cancellation requested",
	}

	output, err := FormatResponse(cliResponse, OutputFormat(jobsFormat))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error formatting output: %v\n", err)
		os.Exit(1)
	}

	fmt.Println(output)

	logger.Debug("Job cancel completed", map[string]interface{}{
		"jobId":    jobId,
		"duration": time.Since(start).Milliseconds(),
	})
}

// JobsListResponseCLI contains jobs list for CLI output
type JobsListResponseCLI struct {
	Jobs       []JobSummaryCLI `json:"jobs"`
	TotalCount int             `json:"totalCount"`
}

type JobSummaryCLI struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Status      string  `json:"status"`
	Progress    int     `json:"progress"`
	CreatedAt   string  `json:"createdAt"`
	CompletedAt *string `json:"completedAt,omitempty"`
	Error       string  `json:"error,omitempty"`
}

// JobResponseCLI contains full job details for CLI output
type JobResponseCLI struct {
	ID          string  `json:"id"`
	Type        string  `json:"type"`
	Scope       string  `json:"scope,omitempty"`
	Status      string  `json:"status"`
	Progress    int     `json:"progress"`
	CreatedAt   string  `json:"createdAt"`
	StartedAt   *string `json:"startedAt,omitempty"`
	CompletedAt *string `json:"completedAt,omitempty"`
	Error       string  `json:"error,omitempty"`
	Result      string  `json:"result,omitempty"`
}

// JobCancelResponseCLI contains cancel result for CLI output
type JobCancelResponseCLI struct {
	JobId     string `json:"jobId"`
	Cancelled bool   `json:"cancelled"`
	Message   string `json:"message"`
}

func convertJobsListResponse(resp *jobs.ListJobsResponse) *JobsListResponseCLI {
	jobsList := make([]JobSummaryCLI, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		job := JobSummaryCLI{
			ID:        j.ID,
			Type:      string(j.Type),
			Status:    string(j.Status),
			Progress:  j.Progress,
			CreatedAt: j.CreatedAt.Format(time.RFC3339),
			Error:     j.Error,
		}
		if j.CompletedAt != nil {
			s := j.CompletedAt.Format(time.RFC3339)
			job.CompletedAt = &s
		}
		jobsList = append(jobsList, job)
	}

	return &JobsListResponseCLI{
		Jobs:       jobsList,
		TotalCount: resp.TotalCount,
	}
}

func convertJobResponse(job *jobs.Job) *JobResponseCLI {
	result := &JobResponseCLI{
		ID:        job.ID,
		Type:      string(job.Type),
		Scope:     truncateString(job.Scope, 200),
		Status:    string(job.Status),
		Progress:  job.Progress,
		CreatedAt: job.CreatedAt.Format(time.RFC3339),
		Error:     job.Error,
		Result:    truncateString(job.Result, 500),
	}

	if job.StartedAt != nil {
		s := job.StartedAt.Format(time.RFC3339)
		result.StartedAt = &s
	}
	if job.CompletedAt != nil {
		s := job.CompletedAt.Format(time.RFC3339)
		result.CompletedAt = &s
	}

	return result
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return strings.TrimSpace(s[:maxLen]) + "..."
}
