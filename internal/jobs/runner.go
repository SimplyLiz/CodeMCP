package jobs

import (
	"context"
	"fmt"
	"sync"
	"time"

	"ckb/internal/logging"
)

// JobHandler executes a specific type of job.
type JobHandler func(ctx context.Context, job *Job, progress func(int)) (interface{}, error)

// Runner manages background job execution.
type Runner struct {
	store    *Store
	logger   *logging.Logger
	handlers map[JobType]JobHandler

	queue       chan *Job
	queueSize   int
	workerCount int

	// Control channels
	done   chan struct{}
	cancel map[string]context.CancelFunc

	mu sync.RWMutex
	wg sync.WaitGroup

	// Metrics
	processedCount int64
	failedCount    int64

	// Recovery settings
	recoveryInterval time.Duration
}

// RunnerConfig contains configuration for the job runner.
type RunnerConfig struct {
	QueueSize        int
	WorkerCount      int
	RecoveryInterval time.Duration // How often to check for orphaned jobs
}

// DefaultRunnerConfig returns the default runner configuration.
func DefaultRunnerConfig() RunnerConfig {
	return RunnerConfig{
		QueueSize:        100,
		WorkerCount:      1, // Single worker for v6.1
		RecoveryInterval: 30 * time.Second,
	}
}

// NewRunner creates a new job runner.
func NewRunner(store *Store, logger *logging.Logger, config RunnerConfig) *Runner {
	if config.QueueSize <= 0 {
		config.QueueSize = 100
	}
	if config.WorkerCount <= 0 {
		config.WorkerCount = 1
	}
	if config.RecoveryInterval <= 0 {
		config.RecoveryInterval = 30 * time.Second
	}

	return &Runner{
		store:            store,
		logger:           logger,
		handlers:         make(map[JobType]JobHandler),
		queue:            make(chan *Job, config.QueueSize),
		queueSize:        config.QueueSize,
		workerCount:      config.WorkerCount,
		done:             make(chan struct{}),
		cancel:           make(map[string]context.CancelFunc),
		recoveryInterval: config.RecoveryInterval,
	}
}

// RegisterHandler registers a handler for a job type.
func (r *Runner) RegisterHandler(jobType JobType, handler JobHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.handlers[jobType] = handler
	r.logger.Debug("Registered job handler", map[string]interface{}{
		"type": jobType,
	})
}

// Start begins processing jobs.
func (r *Runner) Start() error {
	r.logger.Info("Starting job runner", map[string]interface{}{
		"workers":          r.workerCount,
		"queueSize":        r.queueSize,
		"recoveryInterval": r.recoveryInterval.String(),
	})

	// Start workers
	for i := 0; i < r.workerCount; i++ {
		r.wg.Add(1)
		go r.worker(i)
	}

	// Start recovery goroutine to periodically check for orphaned jobs
	r.wg.Add(1)
	go r.recoveryLoop()

	// Recover pending jobs from database on startup
	r.recoverPendingJobs()

	return nil
}

// recoveryLoop periodically checks for orphaned jobs in the database
func (r *Runner) recoveryLoop() {
	defer r.wg.Done()

	ticker := time.NewTicker(r.recoveryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			r.recoverPendingJobs()
		case <-r.done:
			r.logger.Debug("Recovery loop stopping", nil)
			return
		}
	}
}

// recoverPendingJobs loads pending jobs from the database and enqueues them
func (r *Runner) recoverPendingJobs() {
	pending, err := r.store.GetPendingJobs()
	if err != nil {
		r.logger.Warn("Failed to recover pending jobs", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	if len(pending) == 0 {
		return
	}

	recovered := 0
	for _, job := range pending {
		select {
		case r.queue <- job:
			recovered++
		default:
			// Queue still full, will retry on next interval
			break
		}
	}

	if recovered > 0 {
		r.logger.Info("Recovered pending jobs", map[string]interface{}{
			"recovered": recovered,
			"remaining": len(pending) - recovered,
		})
	}
}

// Stop gracefully shuts down the runner.
func (r *Runner) Stop(timeout time.Duration) error {
	r.logger.Info("Stopping job runner", nil)

	// Signal workers to stop
	close(r.done)

	// Cancel all running jobs
	r.mu.Lock()
	for id, cancel := range r.cancel {
		r.logger.Debug("Cancelling running job", map[string]interface{}{
			"jobId": id,
		})
		cancel()
	}
	r.mu.Unlock()

	// Wait for workers with timeout
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		r.logger.Info("Job runner stopped cleanly", nil)
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("job runner shutdown timed out after %v", timeout)
	}
}

// Submit adds a job to the queue.
func (r *Runner) Submit(job *Job) error {
	// Save to database first
	if err := r.store.CreateJob(job); err != nil {
		return fmt.Errorf("failed to persist job: %w", err)
	}

	// Try to enqueue
	select {
	case r.queue <- job:
		r.logger.Debug("Job queued", map[string]interface{}{
			"jobId": job.ID,
			"type":  job.Type,
		})
		return nil
	case <-time.After(100 * time.Millisecond):
		// Queue is full, job remains in database and will be picked up later
		r.logger.Warn("Job queue full, job will be processed later", map[string]interface{}{
			"jobId": job.ID,
		})
		return nil
	case <-r.done:
		return fmt.Errorf("runner is shutting down")
	}
}

// Cancel attempts to cancel a job.
func (r *Runner) Cancel(jobID string) error {
	// Check if job exists and can be cancelled
	job, err := r.store.GetJob(jobID)
	if err != nil {
		return err
	}
	if job == nil {
		return fmt.Errorf("job not found: %s", jobID)
	}

	if !job.CanCancel() {
		return fmt.Errorf("job cannot be cancelled in state: %s", job.Status)
	}

	// If running, cancel context
	r.mu.Lock()
	if cancel, ok := r.cancel[jobID]; ok {
		cancel()
	}
	r.mu.Unlock()

	// Update status
	job.MarkCancelled()
	return r.store.UpdateJob(job)
}

// GetJob retrieves a job by ID.
func (r *Runner) GetJob(jobID string) (*Job, error) {
	return r.store.GetJob(jobID)
}

// ListJobs lists jobs with filters.
func (r *Runner) ListJobs(opts ListJobsOptions) (*ListJobsResponse, error) {
	return r.store.ListJobs(opts)
}

// worker processes jobs from the queue.
func (r *Runner) worker(id int) {
	defer r.wg.Done()

	r.logger.Debug("Job worker started", map[string]interface{}{
		"workerId": id,
	})

	for {
		select {
		case job, ok := <-r.queue:
			if !ok {
				return
			}
			r.processJob(job)

		case <-r.done:
			r.logger.Debug("Job worker stopping", map[string]interface{}{
				"workerId": id,
			})
			return
		}
	}
}

// processJob executes a single job.
func (r *Runner) processJob(job *Job) {
	// Get handler
	r.mu.RLock()
	handler, ok := r.handlers[job.Type]
	r.mu.RUnlock()

	if !ok {
		r.logger.Error("No handler for job type", map[string]interface{}{
			"jobId": job.ID,
			"type":  job.Type,
		})
		job.MarkFailed(fmt.Errorf("no handler for job type: %s", job.Type))
		_ = r.store.UpdateJob(job)
		return
	}

	// Create cancellable context
	ctx, cancel := context.WithCancel(context.Background())
	r.mu.Lock()
	r.cancel[job.ID] = cancel
	r.mu.Unlock()

	defer func() {
		r.mu.Lock()
		delete(r.cancel, job.ID)
		r.mu.Unlock()
		cancel()
	}()

	// Mark as running
	job.MarkStarted()
	if err := r.store.UpdateJob(job); err != nil {
		r.logger.Error("Failed to update job status", map[string]interface{}{
			"jobId": job.ID,
			"error": err.Error(),
		})
	}

	r.logger.Info("Processing job", map[string]interface{}{
		"jobId": job.ID,
		"type":  job.Type,
	})

	// Progress callback
	progress := func(pct int) {
		job.SetProgress(pct)
		if err := r.store.UpdateJob(job); err != nil {
			r.logger.Warn("Failed to update job progress", map[string]interface{}{
				"jobId": job.ID,
				"error": err.Error(),
			})
		}
	}

	// Execute handler
	startTime := time.Now()
	result, err := handler(ctx, job, progress)
	duration := time.Since(startTime)

	if err != nil {
		// Check if cancelled
		if ctx.Err() == context.Canceled {
			job.MarkCancelled()
			r.logger.Info("Job cancelled", map[string]interface{}{
				"jobId":    job.ID,
				"duration": duration.String(),
			})
		} else {
			job.MarkFailed(err)
			r.failedCount++
			r.logger.Error("Job failed", map[string]interface{}{
				"jobId":    job.ID,
				"error":    err.Error(),
				"duration": duration.String(),
			})
		}
	} else {
		if err := job.MarkCompleted(result); err != nil {
			r.logger.Error("Failed to serialize job result", map[string]interface{}{
				"jobId": job.ID,
				"error": err.Error(),
			})
			job.MarkFailed(err)
		} else {
			r.processedCount++
			r.logger.Info("Job completed", map[string]interface{}{
				"jobId":    job.ID,
				"duration": duration.String(),
			})
		}
	}

	// Save final state
	if err := r.store.UpdateJob(job); err != nil {
		r.logger.Error("Failed to save job final state", map[string]interface{}{
			"jobId": job.ID,
			"error": err.Error(),
		})
	}
}

// Stats returns runner statistics.
func (r *Runner) Stats() map[string]interface{} {
	r.mu.RLock()
	runningCount := len(r.cancel)
	r.mu.RUnlock()

	return map[string]interface{}{
		"queueLength":    len(r.queue),
		"queueCapacity":  r.queueSize,
		"runningJobs":    runningCount,
		"processedTotal": r.processedCount,
		"failedTotal":    r.failedCount,
		"workerCount":    r.workerCount,
	}
}

// QueueLength returns the current queue length.
func (r *Runner) QueueLength() int {
	return len(r.queue)
}

// IsRunning returns true if the runner is active.
func (r *Runner) IsRunning() bool {
	select {
	case <-r.done:
		return false
	default:
		return true
	}
}
