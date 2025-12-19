package scheduler

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"ckb/internal/logging"
)

// TaskHandler executes a scheduled task
type TaskHandler func(ctx context.Context, schedule *Schedule) error

// Scheduler manages scheduled task execution
type Scheduler struct {
	store    *Store
	logger   *logging.Logger
	handlers map[TaskType]TaskHandler

	// Control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex

	// Config
	checkInterval time.Duration
}

// Config contains scheduler configuration
type Config struct {
	CheckInterval time.Duration // How often to check for due schedules
	DBPath        string        // Path to scheduler database
}

// DefaultConfig returns the default scheduler configuration
func DefaultConfig() Config {
	return Config{
		CheckInterval: time.Minute,
	}
}

// New creates a new scheduler
func New(ckbDir string, logger *logging.Logger, config Config) (*Scheduler, error) {
	if config.CheckInterval <= 0 {
		config.CheckInterval = time.Minute
	}

	store, err := OpenStore(ckbDir, logger)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Scheduler{
		store:         store,
		logger:        logger,
		handlers:      make(map[TaskType]TaskHandler),
		ctx:           ctx,
		cancel:        cancel,
		checkInterval: config.CheckInterval,
	}, nil
}

// RegisterHandler registers a handler for a task type
func (s *Scheduler) RegisterHandler(taskType TaskType, handler TaskHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[taskType] = handler
	s.logger.Debug("Registered scheduler handler", map[string]interface{}{
		"taskType": taskType,
	})
}

// Start begins the scheduler loop
func (s *Scheduler) Start() error {
	s.logger.Info("Starting scheduler", map[string]interface{}{
		"checkInterval": s.checkInterval.String(),
	})

	s.wg.Add(1)
	go s.run()

	return nil
}

// Stop gracefully stops the scheduler
func (s *Scheduler) Stop(timeout time.Duration) error {
	s.logger.Info("Stopping scheduler", nil)
	s.cancel()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		s.logger.Info("Scheduler stopped", nil)
		return s.store.Close()
	case <-time.After(timeout):
		return fmt.Errorf("scheduler shutdown timed out")
	}
}

// run is the main scheduler loop
func (s *Scheduler) run() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.checkInterval)
	defer ticker.Stop()

	// Run immediately on start
	s.checkDueSchedules()

	for {
		select {
		case <-ticker.C:
			s.checkDueSchedules()
		case <-s.ctx.Done():
			return
		}
	}
}

// checkDueSchedules checks for and executes due schedules
func (s *Scheduler) checkDueSchedules() {
	schedules, err := s.store.GetDueSchedules()
	if err != nil {
		s.logger.Error("Failed to get due schedules", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	for _, schedule := range schedules {
		s.executeSchedule(schedule)
	}
}

// executeSchedule executes a single schedule
func (s *Scheduler) executeSchedule(schedule *Schedule) {
	s.mu.RLock()
	handler, ok := s.handlers[schedule.TaskType]
	s.mu.RUnlock()

	if !ok {
		s.logger.Warn("No handler for task type", map[string]interface{}{
			"scheduleId": schedule.ID,
			"taskType":   schedule.TaskType,
		})
		return
	}

	s.logger.Info("Executing scheduled task", map[string]interface{}{
		"scheduleId": schedule.ID,
		"taskType":   schedule.TaskType,
		"target":     schedule.Target,
	})

	startTime := time.Now()
	err := handler(s.ctx, schedule)
	duration := time.Since(startTime)

	var errMsg string
	if err != nil {
		errMsg = err.Error()
		s.logger.Error("Scheduled task failed", map[string]interface{}{
			"scheduleId": schedule.ID,
			"error":      errMsg,
			"duration":   duration.String(),
		})
	} else {
		s.logger.Info("Scheduled task completed", map[string]interface{}{
			"scheduleId": schedule.ID,
			"duration":   duration.String(),
		})
	}

	// Update schedule
	success := err == nil
	if markErr := schedule.MarkRun(success, duration, errMsg); markErr != nil {
		s.logger.Error("Failed to calculate next run time", map[string]interface{}{
			"scheduleId": schedule.ID,
			"error":      markErr.Error(),
		})
	}

	if updateErr := s.store.UpdateSchedule(schedule); updateErr != nil {
		s.logger.Error("Failed to update schedule", map[string]interface{}{
			"scheduleId": schedule.ID,
			"error":      updateErr.Error(),
		})
	}
}

// AddSchedule adds a new schedule
func (s *Scheduler) AddSchedule(schedule *Schedule) error {
	return s.store.CreateSchedule(schedule)
}

// GetSchedule retrieves a schedule by ID
func (s *Scheduler) GetSchedule(id string) (*Schedule, error) {
	return s.store.GetSchedule(id)
}

// UpdateSchedule updates an existing schedule
func (s *Scheduler) UpdateSchedule(schedule *Schedule) error {
	return s.store.UpdateSchedule(schedule)
}

// DeleteSchedule removes a schedule
func (s *Scheduler) DeleteSchedule(id string) error {
	return s.store.DeleteSchedule(id)
}

// ListSchedules lists schedules with filters
func (s *Scheduler) ListSchedules(opts ListSchedulesOptions) (*ListSchedulesResponse, error) {
	return s.store.ListSchedules(opts)
}

// EnableSchedule enables a schedule
func (s *Scheduler) EnableSchedule(id string) error {
	schedule, err := s.store.GetSchedule(id)
	if err != nil {
		return err
	}
	if schedule == nil {
		return fmt.Errorf("schedule not found: %s", id)
	}

	schedule.Enabled = true
	schedule.UpdatedAt = time.Now()

	// Recalculate next run
	nextRun, err := NextRunTime(schedule.Expression, time.Now())
	if err != nil {
		return err
	}
	schedule.NextRun = nextRun

	return s.store.UpdateSchedule(schedule)
}

// DisableSchedule disables a schedule
func (s *Scheduler) DisableSchedule(id string) error {
	schedule, err := s.store.GetSchedule(id)
	if err != nil {
		return err
	}
	if schedule == nil {
		return fmt.Errorf("schedule not found: %s", id)
	}

	schedule.Enabled = false
	schedule.UpdatedAt = time.Now()

	return s.store.UpdateSchedule(schedule)
}

// RunNow immediately executes a schedule
func (s *Scheduler) RunNow(id string) error {
	schedule, err := s.store.GetSchedule(id)
	if err != nil {
		return err
	}
	if schedule == nil {
		return fmt.Errorf("schedule not found: %s", id)
	}

	s.executeSchedule(schedule)
	return nil
}

// Store provides persistence for schedules
type Store struct {
	conn   *sql.DB
	logger *logging.Logger
	dbPath string
}

// OpenStore opens or creates the scheduler database
func OpenStore(ckbDir string, logger *logging.Logger) (*Store, error) {
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	dbPath := filepath.Join(ckbDir, "scheduler.db")
	dbExists := fileExists(dbPath)

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open scheduler database: %w", err)
	}

	// Set pragmas
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	}

	for _, pragma := range pragmas {
		if _, err := conn.Exec(pragma); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("failed to set pragma: %w", err)
		}
	}

	store := &Store{
		conn:   conn,
		logger: logger,
		dbPath: dbPath,
	}

	if !dbExists {
		logger.Info("Creating scheduler database", map[string]interface{}{
			"path": dbPath,
		})
		if err := store.initializeSchema(); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("failed to initialize scheduler schema: %w", err)
		}
	}

	return store, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// initializeSchema creates the scheduler tables
func (s *Store) initializeSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS schedules (
			id TEXT PRIMARY KEY,
			task_type TEXT NOT NULL,
			target TEXT,
			expression TEXT NOT NULL,
			enabled INTEGER DEFAULT 1,
			next_run TEXT NOT NULL,
			last_run TEXT,
			last_status TEXT,
			last_duration INTEGER,
			last_error TEXT,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_schedules_next_run ON schedules(next_run);
		CREATE INDEX IF NOT EXISTS idx_schedules_enabled ON schedules(enabled);
		CREATE INDEX IF NOT EXISTS idx_schedules_task_type ON schedules(task_type);

		CREATE TABLE IF NOT EXISTS schedule_runs (
			id TEXT PRIMARY KEY,
			schedule_id TEXT NOT NULL,
			job_id TEXT,
			started_at TEXT NOT NULL,
			ended_at TEXT,
			status TEXT NOT NULL,
			error TEXT,
			duration INTEGER,
			FOREIGN KEY (schedule_id) REFERENCES schedules(id)
		);
		CREATE INDEX IF NOT EXISTS idx_runs_schedule ON schedule_runs(schedule_id);
		CREATE INDEX IF NOT EXISTS idx_runs_started ON schedule_runs(started_at DESC);
	`

	_, err := s.conn.Exec(schema)
	return err
}

// Close closes the database connection
func (s *Store) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// CreateSchedule inserts a new schedule
func (s *Store) CreateSchedule(schedule *Schedule) error {
	query := `
		INSERT INTO schedules (id, task_type, target, expression, enabled, next_run, last_run, last_status, last_duration, last_error, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.conn.Exec(query,
		schedule.ID,
		schedule.TaskType,
		nullString(schedule.Target),
		schedule.Expression,
		schedule.Enabled,
		schedule.NextRun.Format(time.RFC3339),
		nullTime(schedule.LastRun),
		nullString(schedule.LastStatus),
		schedule.LastDuration,
		nullString(schedule.LastError),
		schedule.CreatedAt.Format(time.RFC3339),
		schedule.UpdatedAt.Format(time.RFC3339),
	)

	return err
}

// GetSchedule retrieves a schedule by ID
func (s *Store) GetSchedule(id string) (*Schedule, error) {
	query := `
		SELECT id, task_type, target, expression, enabled, next_run, last_run, last_status, last_duration, last_error, created_at, updated_at
		FROM schedules WHERE id = ?
	`

	row := s.conn.QueryRow(query, id)
	return s.scanSchedule(row)
}

// UpdateSchedule updates an existing schedule
func (s *Store) UpdateSchedule(schedule *Schedule) error {
	query := `
		UPDATE schedules SET
			task_type = ?,
			target = ?,
			expression = ?,
			enabled = ?,
			next_run = ?,
			last_run = ?,
			last_status = ?,
			last_duration = ?,
			last_error = ?,
			updated_at = ?
		WHERE id = ?
	`

	result, err := s.conn.Exec(query,
		schedule.TaskType,
		nullString(schedule.Target),
		schedule.Expression,
		schedule.Enabled,
		schedule.NextRun.Format(time.RFC3339),
		nullTime(schedule.LastRun),
		nullString(schedule.LastStatus),
		schedule.LastDuration,
		nullString(schedule.LastError),
		schedule.UpdatedAt.Format(time.RFC3339),
		schedule.ID,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("schedule not found: %s", schedule.ID)
	}

	return nil
}

// DeleteSchedule removes a schedule
func (s *Store) DeleteSchedule(id string) error {
	_, err := s.conn.Exec("DELETE FROM schedules WHERE id = ?", id)
	return err
}

// ListSchedules lists schedules with filters
func (s *Store) ListSchedules(opts ListSchedulesOptions) (*ListSchedulesResponse, error) {
	var conditions []string
	var args []interface{}

	if len(opts.TaskType) > 0 {
		placeholders := make([]string, len(opts.TaskType))
		for i, t := range opts.TaskType {
			placeholders[i] = "?"
			args = append(args, t)
		}
		conditions = append(conditions, fmt.Sprintf("task_type IN (%s)", joinStrings(placeholders, ",")))
	}

	if opts.Enabled != nil {
		conditions = append(conditions, "enabled = ?")
		args = append(args, *opts.Enabled)
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + joinStrings(conditions, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM schedules %s", whereClause)
	var totalCount int
	if err := s.conn.QueryRow(countQuery, args...).Scan(&totalCount); err != nil {
		return nil, err
	}

	// Apply limit/offset
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	query := fmt.Sprintf(`
		SELECT id, task_type, target, expression, enabled, next_run, last_run, last_status, last_duration, last_error, created_at, updated_at
		FROM schedules %s
		ORDER BY next_run ASC
		LIMIT ? OFFSET ?
	`, whereClause)

	args = append(args, limit, opts.Offset)

	rows, err := s.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var schedules []ScheduleSummary
	for rows.Next() {
		schedule, err := s.scanScheduleFromRows(rows)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, schedule.ToSummary())
	}

	return &ListSchedulesResponse{
		Schedules:  schedules,
		TotalCount: totalCount,
	}, rows.Err()
}

// GetDueSchedules retrieves schedules that are due to run
func (s *Store) GetDueSchedules() ([]*Schedule, error) {
	query := `
		SELECT id, task_type, target, expression, enabled, next_run, last_run, last_status, last_duration, last_error, created_at, updated_at
		FROM schedules
		WHERE enabled = 1 AND next_run <= ?
		ORDER BY next_run ASC
	`

	rows, err := s.conn.Query(query, time.Now().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var schedules []*Schedule
	for rows.Next() {
		schedule, err := s.scanScheduleFromRows(rows)
		if err != nil {
			return nil, err
		}
		schedules = append(schedules, schedule)
	}

	return schedules, rows.Err()
}

func (s *Store) scanSchedule(row *sql.Row) (*Schedule, error) {
	var schedule Schedule
	var target, lastRun, lastStatus, lastError sql.NullString
	var nextRun, createdAt, updatedAt string
	var enabled int

	err := row.Scan(
		&schedule.ID,
		&schedule.TaskType,
		&target,
		&schedule.Expression,
		&enabled,
		&nextRun,
		&lastRun,
		&lastStatus,
		&schedule.LastDuration,
		&lastError,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	schedule.Target = target.String
	schedule.Enabled = enabled != 0
	schedule.LastStatus = lastStatus.String
	schedule.LastError = lastError.String

	if t, err := time.Parse(time.RFC3339, nextRun); err == nil {
		schedule.NextRun = t
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		schedule.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		schedule.UpdatedAt = t
	}
	if lastRun.Valid {
		if t, err := time.Parse(time.RFC3339, lastRun.String); err == nil {
			schedule.LastRun = &t
		}
	}

	return &schedule, nil
}

func (s *Store) scanScheduleFromRows(rows *sql.Rows) (*Schedule, error) {
	var schedule Schedule
	var target, lastRun, lastStatus, lastError sql.NullString
	var nextRun, createdAt, updatedAt string
	var enabled int

	err := rows.Scan(
		&schedule.ID,
		&schedule.TaskType,
		&target,
		&schedule.Expression,
		&enabled,
		&nextRun,
		&lastRun,
		&lastStatus,
		&schedule.LastDuration,
		&lastError,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	schedule.Target = target.String
	schedule.Enabled = enabled != 0
	schedule.LastStatus = lastStatus.String
	schedule.LastError = lastError.String

	if t, err := time.Parse(time.RFC3339, nextRun); err == nil {
		schedule.NextRun = t
	}
	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		schedule.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		schedule.UpdatedAt = t
	}
	if lastRun.Valid {
		if t, err := time.Parse(time.RFC3339, lastRun.String); err == nil {
			schedule.LastRun = &t
		}
	}

	return &schedule, nil
}

// Helper functions
func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func nullTime(t *time.Time) sql.NullString {
	if t == nil {
		return sql.NullString{}
	}
	return sql.NullString{String: t.Format(time.RFC3339), Valid: true}
}

func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
