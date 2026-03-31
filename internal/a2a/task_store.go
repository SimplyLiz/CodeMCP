package a2a

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

// TaskStore provides SQLite-backed persistence for A2A tasks.
type TaskStore struct {
	conn   *sql.DB
	logger *slog.Logger
	dbPath string
}

// OpenTaskStore opens or creates the A2A task database at .ckb/a2a_tasks.db.
func OpenTaskStore(ckbDir string, logger *slog.Logger) (*TaskStore, error) {
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .ckb directory: %w", err)
	}

	dbPath := filepath.Join(ckbDir, "a2a_tasks.db")
	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open A2A task database: %w", err)
	}

	pragmas := []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=NORMAL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
		"PRAGMA cache_size=-16000",
	}
	for _, p := range pragmas {
		if _, err = conn.Exec(p); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("failed to set pragma: %w", err)
		}
	}

	store := &TaskStore{conn: conn, logger: logger, dbPath: dbPath}
	if err = store.initSchema(); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return store, nil
}

// Close closes the database connection.
func (s *TaskStore) Close() error {
	return s.conn.Close()
}

func (s *TaskStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS tasks (
		id TEXT PRIMARY KEY,
		context_id TEXT NOT NULL DEFAULT '',
		state TEXT NOT NULL DEFAULT 'TASK_STATE_SUBMITTED',
		status_message TEXT,
		metadata TEXT,
		created_at TEXT NOT NULL,
		updated_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_tasks_state ON tasks(state);
	CREATE INDEX IF NOT EXISTS idx_tasks_context ON tasks(context_id);
	CREATE INDEX IF NOT EXISTS idx_tasks_updated ON tasks(updated_at DESC);

	CREATE TABLE IF NOT EXISTS task_messages (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		message_id TEXT NOT NULL DEFAULT '',
		role TEXT NOT NULL,
		parts TEXT NOT NULL,
		metadata TEXT,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_messages_task ON task_messages(task_id);

	CREATE TABLE IF NOT EXISTS task_artifacts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		artifact_id TEXT NOT NULL,
		name TEXT NOT NULL DEFAULT '',
		description TEXT NOT NULL DEFAULT '',
		parts TEXT NOT NULL,
		metadata TEXT,
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_artifacts_task ON task_artifacts(task_id);

	CREATE TABLE IF NOT EXISTS push_notification_configs (
		id TEXT PRIMARY KEY,
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		url TEXT NOT NULL,
		auth_schemes TEXT,
		auth_token TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_push_task ON push_notification_configs(task_id);

	CREATE TABLE IF NOT EXISTS task_status_history (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		task_id TEXT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
		state TEXT NOT NULL,
		message TEXT,
		timestamp TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_status_history_task ON task_status_history(task_id);
	`
	_, err := s.conn.Exec(schema)
	return err
}

// --- Task CRUD ---

// CreateTask creates a new task and returns it.
func (s *TaskStore) CreateTask(contextID string, metadata map[string]any) (*Task, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	if contextID == "" {
		contextID = uuid.New().String()
	}

	var metaJSON sql.NullString
	if metadata != nil {
		b, err := json.Marshal(metadata)
		if err != nil {
			return nil, fmt.Errorf("marshal metadata: %w", err)
		}
		metaJSON = sql.NullString{String: string(b), Valid: true}
	}

	_, err := s.conn.Exec(
		`INSERT INTO tasks (id, context_id, state, created_at, updated_at, metadata) VALUES (?, ?, ?, ?, ?, ?)`,
		id, contextID, string(TaskStateSubmitted), now, now, metaJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}

	// Record initial status
	_, _ = s.conn.Exec(
		`INSERT INTO task_status_history (task_id, state, timestamp) VALUES (?, ?, ?)`,
		id, string(TaskStateSubmitted), now,
	)

	return &Task{
		ID:        id,
		ContextID: contextID,
		Status: TaskStatus{
			State:     TaskStateSubmitted,
			Timestamp: now,
		},
		Metadata: metadata,
	}, nil
}

// GetTask retrieves a task by ID with optional history length.
func (s *TaskStore) GetTask(taskID string, historyLength *int) (*Task, error) {
	var (
		state         string
		contextID     string
		statusMessage sql.NullString
		metadataJSON  sql.NullString
		updatedAt     string
	)

	err := s.conn.QueryRow(
		`SELECT state, context_id, status_message, metadata, updated_at FROM tasks WHERE id = ?`,
		taskID,
	).Scan(&state, &contextID, &statusMessage, &metadataJSON, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query task: %w", err)
	}

	task := &Task{
		ID:        taskID,
		ContextID: contextID,
		Status: TaskStatus{
			State:     TaskState(state),
			Timestamp: updatedAt,
		},
	}

	if statusMessage.Valid {
		var msg Message
		if json.Unmarshal([]byte(statusMessage.String), &msg) == nil {
			task.Status.Message = &msg
		}
	}

	if metadataJSON.Valid {
		var meta map[string]any
		if json.Unmarshal([]byte(metadataJSON.String), &meta) == nil {
			task.Metadata = meta
		}
	}

	// Load history
	if historyLength == nil || *historyLength != 0 {
		var messages []Message
		messages, err = s.getMessages(taskID, historyLength)
		if err != nil {
			return nil, err
		}
		task.History = messages
	}

	// Load artifacts
	var artifacts []Artifact
	artifacts, err = s.getArtifacts(taskID)
	if err != nil {
		return nil, err
	}
	task.Artifacts = artifacts

	return task, nil
}

// UpdateTaskState atomically transitions a task to a new state.
// Uses a conditional UPDATE to prevent TOCTOU races — the UPDATE only succeeds
// if the current state is one that allows the requested transition.
func (s *TaskStore) UpdateTaskState(taskID string, newState TaskState, statusMsg *Message) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	// Build the list of states that can transition to newState
	validFrom := validSourceStates(newState)
	if len(validFrom) == 0 {
		return fmt.Errorf("no valid source states for target state: %s", newState)
	}

	var statusMsgJSON sql.NullString
	if statusMsg != nil {
		b, err := json.Marshal(statusMsg)
		if err != nil {
			return fmt.Errorf("marshal status message: %w", err)
		}
		statusMsgJSON = sql.NullString{String: string(b), Valid: true}
	}

	// Atomic conditional update: only succeeds if current state allows this transition
	placeholders := make([]string, len(validFrom))
	args := []any{string(newState), statusMsgJSON, now, taskID}
	for i, s := range validFrom {
		placeholders[i] = "?"
		args = append(args, string(s))
	}

	query := fmt.Sprintf(
		`UPDATE tasks SET state = ?, status_message = ?, updated_at = ? WHERE id = ? AND state IN (%s)`,
		joinComma(placeholders),
	)
	result, err := s.conn.Exec(query, args...)
	if err != nil {
		return fmt.Errorf("update task state: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		// Distinguish "not found" from "invalid transition"
		var currentState string
		scanErr := s.conn.QueryRow(`SELECT state FROM tasks WHERE id = ?`, taskID).Scan(&currentState)
		if scanErr == sql.ErrNoRows {
			return NewTaskNotFoundError(taskID)
		}
		return fmt.Errorf("invalid state transition: %s -> %s", currentState, string(newState))
	}

	// Record state transition in history
	var historyMsg sql.NullString
	if statusMsg != nil {
		historyMsg = statusMsgJSON
	}
	_, _ = s.conn.Exec(
		`INSERT INTO task_status_history (task_id, state, message, timestamp) VALUES (?, ?, ?, ?)`,
		taskID, string(newState), historyMsg, now,
	)

	return nil
}

// AddMessage adds a message to a task's history.
func (s *TaskStore) AddMessage(taskID string, msg Message) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	partsJSON, err := json.Marshal(msg.Parts)
	if err != nil {
		return fmt.Errorf("marshal parts: %w", err)
	}

	var metaJSON sql.NullString
	if msg.Metadata != nil {
		var b []byte
		b, err = json.Marshal(msg.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		metaJSON = sql.NullString{String: string(b), Valid: true}
	}

	msgID := msg.MessageID
	if msgID == "" {
		msgID = uuid.New().String()
	}

	_, err = s.conn.Exec(
		`INSERT INTO task_messages (task_id, message_id, role, parts, metadata, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		taskID, msgID, string(msg.Role), string(partsJSON), metaJSON, now,
	)
	if err != nil {
		return fmt.Errorf("insert message: %w", err)
	}
	return nil
}

// AddArtifact adds an artifact to a task.
func (s *TaskStore) AddArtifact(taskID string, artifact Artifact) error {
	now := time.Now().UTC().Format(time.RFC3339Nano)

	partsJSON, err := json.Marshal(artifact.Parts)
	if err != nil {
		return fmt.Errorf("marshal artifact parts: %w", err)
	}

	var metaJSON sql.NullString
	if artifact.Metadata != nil {
		var b []byte
		b, err = json.Marshal(artifact.Metadata)
		if err != nil {
			return fmt.Errorf("marshal metadata: %w", err)
		}
		metaJSON = sql.NullString{String: string(b), Valid: true}
	}

	artifactID := artifact.ArtifactID
	if artifactID == "" {
		artifactID = uuid.New().String()
	}

	_, err = s.conn.Exec(
		`INSERT INTO task_artifacts (task_id, artifact_id, name, description, parts, metadata, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		taskID, artifactID, artifact.Name, artifact.Name, string(partsJSON), metaJSON, now,
	)
	if err != nil {
		return fmt.Errorf("insert artifact: %w", err)
	}
	return nil
}

// ListTasks returns tasks with cursor-based pagination.
func (s *TaskStore) ListTasks(req ListTasksRequest) (*ListTasksResponse, error) {
	pageSize := req.PageSize
	if pageSize <= 0 {
		pageSize = DefaultPageSize
	}
	if pageSize > MaxPageSize {
		pageSize = MaxPageSize
	}

	// Build query
	where := []string{"1=1"}
	args := []any{}

	if req.ContextID != "" {
		where = append(where, "context_id = ?")
		args = append(args, req.ContextID)
	}
	if req.Status != nil {
		where = append(where, "state = ?")
		args = append(args, string(*req.Status))
	}

	// Cursor: base64-encoded offset
	offset := 0
	if req.PageToken != "" {
		decoded, err := base64.StdEncoding.DecodeString(req.PageToken)
		if err == nil {
			offset, _ = strconv.Atoi(string(decoded))
		}
	}

	// Count total
	var totalSize int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM tasks WHERE %s", joinAnd(where))
	_ = s.conn.QueryRow(countQuery, args...).Scan(&totalSize)

	// Fetch page
	query := fmt.Sprintf(
		"SELECT id FROM tasks WHERE %s ORDER BY updated_at DESC LIMIT ? OFFSET ?",
		joinAnd(where),
	)
	args = append(args, pageSize, offset)
	rows, err := s.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list tasks: %w", err)
	}
	defer rows.Close()

	var tasks []Task
	for rows.Next() {
		var taskID string
		if err = rows.Scan(&taskID); err != nil {
			continue
		}
		var task *Task
		task, err = s.GetTask(taskID, req.HistoryLength)
		if err != nil || task == nil {
			continue
		}
		if !req.IncludeArtifacts {
			task.Artifacts = nil
		}
		tasks = append(tasks, *task)
	}

	resp := &ListTasksResponse{
		Tasks:     tasks,
		TotalSize: totalSize,
		PageSize:  pageSize,
	}

	if offset+pageSize < totalSize {
		nextOffset := offset + pageSize
		resp.NextPageToken = base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(nextOffset)))
	}

	return resp, nil
}

// --- Push Notification Config ---

// CreatePushConfig creates a push notification config for a task.
func (s *TaskStore) CreatePushConfig(taskID string, cfg PushNotificationConfig) (*PushNotificationConfig, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339Nano)

	var authSchemes sql.NullString
	var authToken string
	if cfg.Authentication != nil {
		if len(cfg.Authentication.Schemes) > 0 {
			b, _ := json.Marshal(cfg.Authentication.Schemes)
			authSchemes = sql.NullString{String: string(b), Valid: true}
		}
		authToken = cfg.Authentication.Token
	}

	_, err := s.conn.Exec(
		`INSERT INTO push_notification_configs (id, task_id, url, auth_schemes, auth_token, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		id, taskID, cfg.URL, authSchemes, authToken, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert push config: %w", err)
	}

	return &PushNotificationConfig{
		ID:             id,
		URL:            cfg.URL,
		TaskID:         taskID,
		Authentication: cfg.Authentication,
	}, nil
}

// GetPushConfig retrieves a push notification config by ID.
func (s *TaskStore) GetPushConfig(configID string) (*PushNotificationConfig, error) {
	var (
		taskID      string
		url         string
		authSchemes sql.NullString
		authToken   string
	)
	err := s.conn.QueryRow(
		`SELECT task_id, url, auth_schemes, auth_token FROM push_notification_configs WHERE id = ?`,
		configID,
	).Scan(&taskID, &url, &authSchemes, &authToken)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("query push config: %w", err)
	}

	cfg := &PushNotificationConfig{
		ID:     configID,
		URL:    url,
		TaskID: taskID,
	}
	if authToken != "" || authSchemes.Valid {
		cfg.Authentication = &AuthenticationInfo{Token: authToken}
		if authSchemes.Valid {
			_ = json.Unmarshal([]byte(authSchemes.String), &cfg.Authentication.Schemes)
		}
	}
	return cfg, nil
}

// ListPushConfigs lists push notification configs for a task.
func (s *TaskStore) ListPushConfigs(taskID string) ([]PushNotificationConfig, error) {
	rows, err := s.conn.Query(
		`SELECT id, url, auth_schemes, auth_token FROM push_notification_configs WHERE task_id = ?`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("list push configs: %w", err)
	}
	defer rows.Close()

	var configs []PushNotificationConfig
	for rows.Next() {
		var (
			id          string
			url         string
			authSchemes sql.NullString
			authToken   string
		)
		if err = rows.Scan(&id, &url, &authSchemes, &authToken); err != nil {
			continue
		}
		cfg := PushNotificationConfig{ID: id, URL: url, TaskID: taskID}
		if authToken != "" || authSchemes.Valid {
			cfg.Authentication = &AuthenticationInfo{Token: authToken}
			if authSchemes.Valid {
				_ = json.Unmarshal([]byte(authSchemes.String), &cfg.Authentication.Schemes)
			}
		}
		configs = append(configs, cfg)
	}
	return configs, nil
}

// DeletePushConfig deletes a push notification config.
func (s *TaskStore) DeletePushConfig(configID string) error {
	result, err := s.conn.Exec(`DELETE FROM push_notification_configs WHERE id = ?`, configID)
	if err != nil {
		return fmt.Errorf("delete push config: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("push config not found: %s", configID)
	}
	return nil
}

// CleanupOldTasks removes completed/failed/canceled tasks older than the given duration.
func (s *TaskStore) CleanupOldTasks(retention time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-retention).Format(time.RFC3339Nano)
	result, err := s.conn.Exec(
		`DELETE FROM tasks WHERE state IN (?, ?, ?, ?) AND updated_at < ?`,
		string(TaskStateCompleted), string(TaskStateFailed), string(TaskStateCanceled), string(TaskStateRejected), cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("cleanup old tasks: %w", err)
	}
	return result.RowsAffected()
}

// --- Internal helpers ---

func (s *TaskStore) getMessages(taskID string, limit *int) ([]Message, error) {
	query := `SELECT message_id, role, parts, metadata FROM task_messages WHERE task_id = ? ORDER BY id ASC`
	args := []any{taskID}
	if limit != nil && *limit > 0 {
		query = `SELECT message_id, role, parts, metadata FROM task_messages WHERE task_id = ? ORDER BY id DESC LIMIT ?`
		args = append(args, *limit)
	}

	rows, err := s.conn.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query messages: %w", err)
	}
	defer rows.Close()

	var messages []Message
	for rows.Next() {
		var (
			msgID     string
			role      string
			partsJSON string
			metaJSON  sql.NullString
		)
		if err = rows.Scan(&msgID, &role, &partsJSON, &metaJSON); err != nil {
			continue
		}
		msg := Message{MessageID: msgID, Role: Role(role)}
		_ = json.Unmarshal([]byte(partsJSON), &msg.Parts)
		if metaJSON.Valid {
			_ = json.Unmarshal([]byte(metaJSON.String), &msg.Metadata)
		}
		messages = append(messages, msg)
	}

	// If we used DESC LIMIT, reverse to get chronological order
	if limit != nil && *limit > 0 {
		for i, j := 0, len(messages)-1; i < j; i, j = i+1, j-1 {
			messages[i], messages[j] = messages[j], messages[i]
		}
	}

	return messages, nil
}

func (s *TaskStore) getArtifacts(taskID string) ([]Artifact, error) {
	rows, err := s.conn.Query(
		`SELECT artifact_id, name, description, parts, metadata FROM task_artifacts WHERE task_id = ? ORDER BY id ASC`,
		taskID,
	)
	if err != nil {
		return nil, fmt.Errorf("query artifacts: %w", err)
	}
	defer rows.Close()

	var artifacts []Artifact
	for rows.Next() {
		var (
			artID       string
			name        string
			description string
			partsJSON   string
			metaJSON    sql.NullString
		)
		if err = rows.Scan(&artID, &name, &description, &partsJSON, &metaJSON); err != nil {
			continue
		}
		art := Artifact{ArtifactID: artID, Name: name}
		_ = json.Unmarshal([]byte(partsJSON), &art.Parts)
		if metaJSON.Valid {
			_ = json.Unmarshal([]byte(metaJSON.String), &art.Metadata)
		}
		artifacts = append(artifacts, art)
	}

	return artifacts, nil
}

func joinAnd(clauses []string) string {
	result := clauses[0]
	for i := 1; i < len(clauses); i++ {
		result += " AND " + clauses[i]
	}
	return result
}

func joinComma(parts []string) string {
	result := parts[0]
	for i := 1; i < len(parts); i++ {
		result += ", " + parts[i]
	}
	return result
}

// validSourceStates returns all states that can transition to the given target state.
func validSourceStates(target TaskState) []TaskState {
	var sources []TaskState
	for from, tos := range validTransitions {
		for _, to := range tos {
			if to == target {
				sources = append(sources, from)
				break
			}
		}
	}
	return sources
}
