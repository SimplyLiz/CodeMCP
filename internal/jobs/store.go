package jobs

import (
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store provides persistence for jobs in a separate SQLite database.
type Store struct {
	conn   *sql.DB
	logger *slog.Logger
	dbPath string
}

// OpenStore opens or creates the jobs database at .ckb/jobs.db
func OpenStore(ckbDir string, logger *slog.Logger) (*Store, error) {
	// Ensure .ckb directory exists
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .ckb directory: %w", err)
	}

	dbPath := filepath.Join(ckbDir, "jobs.db")
	dbExists := fileExists(dbPath)

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open jobs database: %w", err)
	}

	// Set pragmas for performance
	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA cache_size=-16000", // 16MB cache
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
		logger.Info("Creating jobs database", map[string]interface{}{
			"path": dbPath,
		})
		if err := store.initializeSchema(); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("failed to initialize jobs schema: %w", err)
		}
	}

	return store, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// initializeSchema creates the jobs tables.
func (s *Store) initializeSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS jobs (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			scope TEXT,
			status TEXT NOT NULL DEFAULT 'queued',
			progress INTEGER DEFAULT 0,
			created_at TEXT NOT NULL,
			started_at TEXT,
			completed_at TEXT,
			error TEXT,
			result TEXT
		);
		CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
		CREATE INDEX IF NOT EXISTS idx_jobs_created_at ON jobs(created_at DESC);
		CREATE INDEX IF NOT EXISTS idx_jobs_type ON jobs(type);

		CREATE TABLE IF NOT EXISTS file_checksums (
			path TEXT PRIMARY KEY,
			checksum TEXT NOT NULL,
			last_indexed TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_checksums_indexed ON file_checksums(last_indexed);

		CREATE TABLE IF NOT EXISTS schema_version (
			version INTEGER PRIMARY KEY
		);
		INSERT OR REPLACE INTO schema_version (version) VALUES (1);
	`

	_, err := s.conn.Exec(schema)
	return err
}

// Close closes the database connection.
func (s *Store) Close() error {
	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

// CreateJob inserts a new job into the database.
func (s *Store) CreateJob(job *Job) error {
	query := `
		INSERT INTO jobs (id, type, scope, status, progress, created_at, started_at, completed_at, error, result)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.conn.Exec(query,
		job.ID,
		job.Type,
		nullString(job.Scope),
		job.Status,
		job.Progress,
		job.CreatedAt.Format(time.RFC3339),
		nullTime(job.StartedAt),
		nullTime(job.CompletedAt),
		nullString(job.Error),
		nullString(job.Result),
	)

	if err != nil {
		return fmt.Errorf("failed to create job: %w", err)
	}

	s.logger.Debug("Created job", map[string]interface{}{
		"jobId": job.ID,
		"type":  job.Type,
	})

	return nil
}

// GetJob retrieves a job by ID.
func (s *Store) GetJob(id string) (*Job, error) {
	query := `
		SELECT id, type, scope, status, progress, created_at, started_at, completed_at, error, result
		FROM jobs WHERE id = ?
	`

	row := s.conn.QueryRow(query, id)
	return s.scanJob(row)
}

// UpdateJob updates an existing job.
func (s *Store) UpdateJob(job *Job) error {
	query := `
		UPDATE jobs SET
			status = ?,
			progress = ?,
			started_at = ?,
			completed_at = ?,
			error = ?,
			result = ?
		WHERE id = ?
	`

	result, err := s.conn.Exec(query,
		job.Status,
		job.Progress,
		nullTime(job.StartedAt),
		nullTime(job.CompletedAt),
		nullString(job.Error),
		nullString(job.Result),
		job.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update job: %w", err)
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("job not found: %s", job.ID)
	}

	return nil
}

// ListJobs retrieves jobs matching the given options.
func (s *Store) ListJobs(opts ListJobsOptions) (*ListJobsResponse, error) {
	// Build query with filters
	var conditions []string
	var args []interface{}

	if len(opts.Status) > 0 {
		placeholders := make([]string, len(opts.Status))
		for i, status := range opts.Status {
			placeholders[i] = "?"
			args = append(args, status)
		}
		conditions = append(conditions, fmt.Sprintf("status IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(opts.Type) > 0 {
		placeholders := make([]string, len(opts.Type))
		for i, t := range opts.Type {
			placeholders[i] = "?"
			args = append(args, t)
		}
		conditions = append(conditions, fmt.Sprintf("type IN (%s)", strings.Join(placeholders, ",")))
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM jobs %s", whereClause)
	var totalCount int
	if err := s.conn.QueryRow(countQuery, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("failed to count jobs: %w", err)
	}

	// Apply limit/offset
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	query := fmt.Sprintf(`
		SELECT id, type, scope, status, progress, created_at, started_at, completed_at, error, result
		FROM jobs %s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	args = append(args, limit, opts.Offset)

	rows, err := s.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []JobSummary
	for rows.Next() {
		job, err := s.scanJobFromRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job.ToSummary())
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating jobs: %w", err)
	}

	return &ListJobsResponse{
		Jobs:       jobs,
		TotalCount: totalCount,
	}, nil
}

// GetPendingJobs retrieves all queued jobs ordered by creation time.
func (s *Store) GetPendingJobs() ([]*Job, error) {
	query := `
		SELECT id, type, scope, status, progress, created_at, started_at, completed_at, error, result
		FROM jobs WHERE status = 'queued'
		ORDER BY created_at ASC
	`

	rows, err := s.conn.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get pending jobs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var jobs []*Job
	for rows.Next() {
		job, err := s.scanJobFromRows(rows)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}

	return jobs, rows.Err()
}

// CleanupOldJobs removes completed jobs older than the given duration.
func (s *Store) CleanupOldJobs(retention time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-retention).Format(time.RFC3339)

	result, err := s.conn.Exec(`
		DELETE FROM jobs
		WHERE status IN ('completed', 'failed', 'cancelled')
		AND completed_at < ?
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup old jobs: %w", err)
	}

	return result.RowsAffected()
}

// scanJob scans a single row into a Job.
func (s *Store) scanJob(row *sql.Row) (*Job, error) {
	var job Job
	var scope, startedAt, completedAt, errMsg, result sql.NullString
	var createdAt string

	err := row.Scan(
		&job.ID,
		&job.Type,
		&scope,
		&job.Status,
		&job.Progress,
		&createdAt,
		&startedAt,
		&completedAt,
		&errMsg,
		&result,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan job: %w", err)
	}

	job.Scope = scope.String
	job.Error = errMsg.String
	job.Result = result.String

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		job.CreatedAt = t
	}
	if startedAt.Valid {
		if t, err := time.Parse(time.RFC3339, startedAt.String); err == nil {
			job.StartedAt = &t
		}
	}
	if completedAt.Valid {
		if t, err := time.Parse(time.RFC3339, completedAt.String); err == nil {
			job.CompletedAt = &t
		}
	}

	return &job, nil
}

// scanJobFromRows scans a row from a Rows result into a Job.
func (s *Store) scanJobFromRows(rows *sql.Rows) (*Job, error) {
	var job Job
	var scope, startedAt, completedAt, errMsg, result sql.NullString
	var createdAt string

	err := rows.Scan(
		&job.ID,
		&job.Type,
		&scope,
		&job.Status,
		&job.Progress,
		&createdAt,
		&startedAt,
		&completedAt,
		&errMsg,
		&result,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan job row: %w", err)
	}

	job.Scope = scope.String
	job.Error = errMsg.String
	job.Result = result.String

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		job.CreatedAt = t
	}
	if startedAt.Valid {
		if t, err := time.Parse(time.RFC3339, startedAt.String); err == nil {
			job.StartedAt = &t
		}
	}
	if completedAt.Valid {
		if t, err := time.Parse(time.RFC3339, completedAt.String); err == nil {
			job.CompletedAt = &t
		}
	}

	return &job, nil
}

// Helper functions for nullable fields
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

// File checksum operations for incremental refresh

// FileChecksum represents a file's checksum for change detection.
type FileChecksum struct {
	Path        string
	Checksum    string
	LastIndexed time.Time
}

// GetFileChecksum retrieves a file's checksum.
func (s *Store) GetFileChecksum(path string) (*FileChecksum, error) {
	var checksum FileChecksum
	var lastIndexed string

	err := s.conn.QueryRow(`
		SELECT path, checksum, last_indexed FROM file_checksums WHERE path = ?
	`, path).Scan(&checksum.Path, &checksum.Checksum, &lastIndexed)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if t, err := time.Parse(time.RFC3339, lastIndexed); err == nil {
		checksum.LastIndexed = t
	}

	return &checksum, nil
}

// SaveFileChecksum saves or updates a file's checksum.
func (s *Store) SaveFileChecksum(checksum *FileChecksum) error {
	_, err := s.conn.Exec(`
		INSERT OR REPLACE INTO file_checksums (path, checksum, last_indexed)
		VALUES (?, ?, ?)
	`, checksum.Path, checksum.Checksum, checksum.LastIndexed.Format(time.RFC3339))
	return err
}

// GetAllChecksums retrieves all file checksums.
func (s *Store) GetAllChecksums() (map[string]string, error) {
	rows, err := s.conn.Query("SELECT path, checksum FROM file_checksums")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	checksums := make(map[string]string)
	for rows.Next() {
		var path, checksum string
		if err := rows.Scan(&path, &checksum); err != nil {
			return nil, err
		}
		checksums[path] = checksum
	}

	return checksums, rows.Err()
}

// ClearChecksums removes all file checksums.
func (s *Store) ClearChecksums() error {
	_, err := s.conn.Exec("DELETE FROM file_checksums")
	return err
}
