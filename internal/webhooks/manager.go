package webhooks

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	_ "modernc.org/sqlite"

	"ckb/internal/logging"
)

// Manager manages webhooks and their deliveries
type Manager struct {
	store   *Store
	logger  *logging.Logger
	client  *http.Client

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.RWMutex

	// Config
	workerCount   int
	retryInterval time.Duration
}

// Config contains webhook manager configuration
type Config struct {
	WorkerCount   int           // Number of delivery workers
	RetryInterval time.Duration // How often to retry failed deliveries
	Timeout       time.Duration // HTTP request timeout
}

// DefaultConfig returns the default webhook manager configuration
func DefaultConfig() Config {
	return Config{
		WorkerCount:   2,
		RetryInterval: time.Minute,
		Timeout:       30 * time.Second,
	}
}

// NewManager creates a new webhook manager
func NewManager(ckbDir string, logger *logging.Logger, config Config) (*Manager, error) {
	if config.WorkerCount <= 0 {
		config.WorkerCount = 2
	}
	if config.RetryInterval <= 0 {
		config.RetryInterval = time.Minute
	}
	if config.Timeout <= 0 {
		config.Timeout = 30 * time.Second
	}

	store, err := OpenStore(ckbDir, logger)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		store:  store,
		logger: logger,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		ctx:           ctx,
		cancel:        cancel,
		workerCount:   config.WorkerCount,
		retryInterval: config.RetryInterval,
	}, nil
}

// Start begins the webhook delivery workers
func (m *Manager) Start() error {
	m.logger.Info("Starting webhook manager", map[string]interface{}{
		"workers":       m.workerCount,
		"retryInterval": m.retryInterval.String(),
	})

	// Start retry worker
	m.wg.Add(1)
	go m.retryWorker()

	return nil
}

// Stop gracefully stops the webhook manager
func (m *Manager) Stop(timeout time.Duration) error {
	m.logger.Info("Stopping webhook manager", nil)
	m.cancel()

	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("Webhook manager stopped", nil)
		return m.store.Close()
	case <-time.After(timeout):
		return fmt.Errorf("webhook manager shutdown timed out")
	}
}

// retryWorker periodically retries failed deliveries
func (m *Manager) retryWorker() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.retryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			m.processRetries()
		case <-m.ctx.Done():
			return
		}
	}
}

// processRetries processes deliveries that are due for retry
func (m *Manager) processRetries() {
	deliveries, err := m.store.GetPendingRetries()
	if err != nil {
		m.logger.Error("Failed to get pending retries", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	for _, delivery := range deliveries {
		m.deliver(delivery)
	}
}

// Emit emits an event to all matching webhooks
func (m *Manager) Emit(event *Event) error {
	webhooks, err := m.store.GetWebhooksForEvent(event.Type)
	if err != nil {
		return fmt.Errorf("failed to get webhooks: %w", err)
	}

	if len(webhooks) == 0 {
		return nil
	}

	m.logger.Info("Emitting event to webhooks", map[string]interface{}{
		"eventId":      event.ID,
		"eventType":    event.Type,
		"webhookCount": len(webhooks),
	})

	for _, webhook := range webhooks {
		if err := m.queueDelivery(webhook, event); err != nil {
			m.logger.Error("Failed to queue delivery", map[string]interface{}{
				"eventId":   event.ID,
				"webhookId": webhook.ID,
				"error":     err.Error(),
			})
		}
	}

	return nil
}

// queueDelivery creates a delivery record and attempts immediate delivery
func (m *Manager) queueDelivery(webhook *Webhook, event *Event) error {
	// Format payload
	payload, err := m.formatPayload(webhook, event)
	if err != nil {
		return fmt.Errorf("failed to format payload: %w", err)
	}

	delivery := &Delivery{
		ID:        generateDeliveryID(),
		WebhookID: webhook.ID,
		EventID:   event.ID,
		EventType: event.Type,
		Payload:   payload,
		Status:    DeliveryQueued,
		CreatedAt: time.Now(),
	}

	if err := m.store.CreateDelivery(delivery); err != nil {
		return fmt.Errorf("failed to create delivery: %w", err)
	}

	// Attempt immediate delivery
	go m.deliver(delivery)

	return nil
}

// deliver attempts to deliver a webhook
func (m *Manager) deliver(delivery *Delivery) {
	webhook, err := m.store.GetWebhook(delivery.WebhookID)
	if err != nil || webhook == nil {
		m.logger.Error("Webhook not found for delivery", map[string]interface{}{
			"deliveryId": delivery.ID,
			"webhookId":  delivery.WebhookID,
		})
		return
	}

	m.logger.Debug("Delivering webhook", map[string]interface{}{
		"deliveryId": delivery.ID,
		"webhookId":  webhook.ID,
		"url":        webhook.URL,
		"attempt":    delivery.Attempts + 1,
	})

	// Create request
	req, err := http.NewRequestWithContext(m.ctx, "POST", webhook.URL, bytes.NewBufferString(delivery.Payload))
	if err != nil {
		m.handleDeliveryError(delivery, webhook, err)
		return
	}

	// Set headers
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "CKB-Webhook/1.0")
	req.Header.Set("X-CKB-Event-ID", delivery.EventID)
	req.Header.Set("X-CKB-Event-Type", string(delivery.EventType))
	req.Header.Set("X-CKB-Delivery-ID", delivery.ID)

	// Add custom headers
	for k, v := range webhook.Headers {
		req.Header.Set(k, v)
	}

	// Add signature if secret is configured
	if webhook.Secret != "" {
		signature := m.signPayload(delivery.Payload, webhook.Secret)
		req.Header.Set("X-CKB-Signature-256", "sha256="+signature)
	}

	// Send request
	resp, err := m.client.Do(req)
	if err != nil {
		m.handleDeliveryError(delivery, webhook, err)
		return
	}
	defer resp.Body.Close()

	// Read response body (limited)
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 10*1024))

	// Check response
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		delivery.MarkDelivered(resp.StatusCode)
		m.logger.Info("Webhook delivered successfully", map[string]interface{}{
			"deliveryId":   delivery.ID,
			"webhookId":    webhook.ID,
			"responseCode": resp.StatusCode,
		})
	} else {
		err := fmt.Errorf("HTTP %d: %s", resp.StatusCode, resp.Status)
		m.handleDeliveryError(delivery, webhook, err)
		return
	}

	if err := m.store.UpdateDelivery(delivery); err != nil {
		m.logger.Error("Failed to update delivery", map[string]interface{}{
			"deliveryId": delivery.ID,
			"error":      err.Error(),
		})
	}
}

// handleDeliveryError handles a delivery failure
func (m *Manager) handleDeliveryError(delivery *Delivery, webhook *Webhook, err error) {
	delivery.MarkFailed(err, webhook.MaxRetries, webhook.RetryDelay)

	m.logger.Warn("Webhook delivery failed", map[string]interface{}{
		"deliveryId": delivery.ID,
		"webhookId":  webhook.ID,
		"error":      err.Error(),
		"attempts":   delivery.Attempts,
		"status":     delivery.Status,
	})

	if delivery.Status == DeliveryDead {
		// Move to dead letter queue
		if err := m.store.MoveToDeadLetter(delivery); err != nil {
			m.logger.Error("Failed to move to dead letter", map[string]interface{}{
				"deliveryId": delivery.ID,
				"error":      err.Error(),
			})
		}
	}

	if err := m.store.UpdateDelivery(delivery); err != nil {
		m.logger.Error("Failed to update delivery", map[string]interface{}{
			"deliveryId": delivery.ID,
			"error":      err.Error(),
		})
	}
}

// signPayload creates an HMAC-SHA256 signature
func (m *Manager) signPayload(payload, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

// formatPayload formats the event payload according to webhook format
func (m *Manager) formatPayload(webhook *Webhook, event *Event) (string, error) {
	switch webhook.Format {
	case FormatSlack:
		return m.formatSlack(event)
	case FormatPagerDuty:
		return m.formatPagerDuty(event)
	case FormatDiscord:
		return m.formatDiscord(event)
	default:
		return m.formatJSON(event)
	}
}

// formatJSON formats as standard JSON
func (m *Manager) formatJSON(event *Event) (string, error) {
	payload := map[string]interface{}{
		"event_id":   event.ID,
		"event_type": event.Type,
		"timestamp":  event.Timestamp.Format(time.RFC3339),
		"source":     event.Source,
		"data":       json.RawMessage(event.Data),
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// formatSlack formats for Slack incoming webhooks
func (m *Manager) formatSlack(event *Event) (string, error) {
	var text string
	var color string

	switch event.Type {
	case EventRefreshCompleted:
		text = fmt.Sprintf(":white_check_mark: CKB refresh completed for %s", event.Source)
		color = "good"
	case EventRefreshFailed:
		text = fmt.Sprintf(":x: CKB refresh failed for %s", event.Source)
		color = "danger"
	case EventHotspotAlert:
		text = fmt.Sprintf(":fire: Hotspot detected in %s", event.Source)
		color = "warning"
	case EventJobCompleted:
		text = fmt.Sprintf(":heavy_check_mark: Job completed: %s", event.Source)
		color = "good"
	case EventJobFailed:
		text = fmt.Sprintf(":warning: Job failed: %s", event.Source)
		color = "danger"
	default:
		text = fmt.Sprintf("CKB event: %s - %s", event.Type, event.Source)
		color = "#36a64f"
	}

	payload := map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":   color,
				"text":    text,
				"ts":      event.Timestamp.Unix(),
				"footer":  "CKB",
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// formatPagerDuty formats for PagerDuty Events API v2
func (m *Manager) formatPagerDuty(event *Event) (string, error) {
	var severity string
	switch event.Type {
	case EventRefreshFailed, EventJobFailed:
		severity = "error"
	case EventHotspotAlert, EventHealthDegraded:
		severity = "warning"
	default:
		severity = "info"
	}

	payload := map[string]interface{}{
		"routing_key":  "", // To be filled by webhook URL or header
		"event_action": "trigger",
		"payload": map[string]interface{}{
			"summary":   fmt.Sprintf("CKB %s: %s", event.Type, event.Source),
			"source":    "ckb",
			"severity":  severity,
			"timestamp": event.Timestamp.Format(time.RFC3339),
			"custom_details": map[string]interface{}{
				"event_id": event.ID,
				"source":   event.Source,
				"data":     json.RawMessage(event.Data),
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// formatDiscord formats for Discord webhooks
func (m *Manager) formatDiscord(event *Event) (string, error) {
	var color int
	var title string

	switch event.Type {
	case EventRefreshCompleted, EventJobCompleted:
		color = 0x00FF00 // Green
		title = "Success"
	case EventRefreshFailed, EventJobFailed:
		color = 0xFF0000 // Red
		title = "Error"
	case EventHotspotAlert, EventHealthDegraded:
		color = 0xFFA500 // Orange
		title = "Warning"
	default:
		color = 0x0000FF // Blue
		title = "Info"
	}

	payload := map[string]interface{}{
		"embeds": []map[string]interface{}{
			{
				"title":       fmt.Sprintf("CKB %s", title),
				"description": fmt.Sprintf("%s: %s", event.Type, event.Source),
				"color":       color,
				"timestamp":   event.Timestamp.Format(time.RFC3339),
				"footer": map[string]interface{}{
					"text": "CKB Daemon",
				},
			},
		},
	}

	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// Webhook management methods

// RegisterWebhook adds a new webhook
func (m *Manager) RegisterWebhook(webhook *Webhook) error {
	return m.store.CreateWebhook(webhook)
}

// GetWebhook retrieves a webhook by ID
func (m *Manager) GetWebhook(id string) (*Webhook, error) {
	return m.store.GetWebhook(id)
}

// UpdateWebhook updates an existing webhook
func (m *Manager) UpdateWebhook(webhook *Webhook) error {
	webhook.UpdatedAt = time.Now()
	return m.store.UpdateWebhook(webhook)
}

// DeleteWebhook removes a webhook
func (m *Manager) DeleteWebhook(id string) error {
	return m.store.DeleteWebhook(id)
}

// ListWebhooks lists all webhooks
func (m *Manager) ListWebhooks() ([]*Webhook, error) {
	return m.store.ListWebhooks()
}

// TestWebhook sends a test event to a webhook
func (m *Manager) TestWebhook(id string) error {
	webhook, err := m.store.GetWebhook(id)
	if err != nil {
		return err
	}
	if webhook == nil {
		return fmt.Errorf("webhook not found: %s", id)
	}

	event, err := NewEvent(EventRefreshCompleted, "test", map[string]interface{}{
		"message": "This is a test webhook delivery",
		"time":    time.Now().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}

	return m.queueDelivery(webhook, event)
}

// ListDeliveries lists deliveries with filters
func (m *Manager) ListDeliveries(opts ListDeliveriesOptions) (*ListDeliveriesResponse, error) {
	return m.store.ListDeliveries(opts)
}

// GetDeadLetters retrieves dead letter queue entries
func (m *Manager) GetDeadLetters(webhookID string, limit int) ([]DeadLetter, error) {
	return m.store.GetDeadLetters(webhookID, limit)
}

// RetryDeadLetter moves a dead letter back to the delivery queue
func (m *Manager) RetryDeadLetter(deadLetterID string) error {
	return m.store.RetryDeadLetter(deadLetterID)
}

// Store provides persistence for webhooks

// Store manages webhook persistence
type Store struct {
	conn   *sql.DB
	logger *logging.Logger
	dbPath string
}

// OpenStore opens or creates the webhooks database
func OpenStore(ckbDir string, logger *logging.Logger) (*Store, error) {
	if err := os.MkdirAll(ckbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	dbPath := filepath.Join(ckbDir, "webhooks.db")
	dbExists := fileExists(dbPath)

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open webhooks database: %w", err)
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
		logger.Info("Creating webhooks database", map[string]interface{}{
			"path": dbPath,
		})
		if err := store.initializeSchema(); err != nil {
			_ = conn.Close()
			return nil, fmt.Errorf("failed to initialize webhooks schema: %w", err)
		}
	}

	return store, nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// initializeSchema creates the webhook tables
func (s *Store) initializeSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS webhooks (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			secret TEXT,
			events TEXT NOT NULL,
			format TEXT NOT NULL DEFAULT 'json',
			enabled INTEGER DEFAULT 1,
			headers TEXT,
			max_retries INTEGER DEFAULT 3,
			retry_delay INTEGER DEFAULT 60,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);

		CREATE TABLE IF NOT EXISTS deliveries (
			id TEXT PRIMARY KEY,
			webhook_id TEXT NOT NULL,
			event_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			payload TEXT NOT NULL,
			status TEXT NOT NULL,
			attempts INTEGER DEFAULT 0,
			last_attempt_at TEXT,
			last_error TEXT,
			response_code INTEGER,
			next_retry_at TEXT,
			created_at TEXT NOT NULL,
			completed_at TEXT,
			FOREIGN KEY (webhook_id) REFERENCES webhooks(id)
		);
		CREATE INDEX IF NOT EXISTS idx_deliveries_webhook ON deliveries(webhook_id);
		CREATE INDEX IF NOT EXISTS idx_deliveries_status ON deliveries(status);
		CREATE INDEX IF NOT EXISTS idx_deliveries_next_retry ON deliveries(next_retry_at);

		CREATE TABLE IF NOT EXISTS dead_letters (
			id TEXT PRIMARY KEY,
			webhook_id TEXT NOT NULL,
			event_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			payload TEXT NOT NULL,
			last_error TEXT,
			attempts INTEGER,
			dead_at TEXT NOT NULL,
			FOREIGN KEY (webhook_id) REFERENCES webhooks(id)
		);
		CREATE INDEX IF NOT EXISTS idx_dead_letters_webhook ON dead_letters(webhook_id);
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

// CreateWebhook inserts a new webhook
func (s *Store) CreateWebhook(webhook *Webhook) error {
	eventsJSON, _ := json.Marshal(webhook.Events)
	headersJSON, _ := json.Marshal(webhook.Headers)

	query := `
		INSERT INTO webhooks (id, name, url, secret, events, format, enabled, headers, max_retries, retry_delay, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.conn.Exec(query,
		webhook.ID,
		webhook.Name,
		webhook.URL,
		webhook.Secret,
		string(eventsJSON),
		webhook.Format,
		webhook.Enabled,
		string(headersJSON),
		webhook.MaxRetries,
		webhook.RetryDelay,
		webhook.CreatedAt.Format(time.RFC3339),
		webhook.UpdatedAt.Format(time.RFC3339),
	)

	return err
}

// GetWebhook retrieves a webhook by ID
func (s *Store) GetWebhook(id string) (*Webhook, error) {
	query := `
		SELECT id, name, url, secret, events, format, enabled, headers, max_retries, retry_delay, created_at, updated_at
		FROM webhooks WHERE id = ?
	`

	row := s.conn.QueryRow(query, id)
	return s.scanWebhook(row)
}

// UpdateWebhook updates an existing webhook
func (s *Store) UpdateWebhook(webhook *Webhook) error {
	eventsJSON, _ := json.Marshal(webhook.Events)
	headersJSON, _ := json.Marshal(webhook.Headers)

	query := `
		UPDATE webhooks SET
			name = ?,
			url = ?,
			secret = ?,
			events = ?,
			format = ?,
			enabled = ?,
			headers = ?,
			max_retries = ?,
			retry_delay = ?,
			updated_at = ?
		WHERE id = ?
	`

	result, err := s.conn.Exec(query,
		webhook.Name,
		webhook.URL,
		webhook.Secret,
		string(eventsJSON),
		webhook.Format,
		webhook.Enabled,
		string(headersJSON),
		webhook.MaxRetries,
		webhook.RetryDelay,
		webhook.UpdatedAt.Format(time.RFC3339),
		webhook.ID,
	)
	if err != nil {
		return err
	}

	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("webhook not found: %s", webhook.ID)
	}

	return nil
}

// DeleteWebhook removes a webhook
func (s *Store) DeleteWebhook(id string) error {
	_, err := s.conn.Exec("DELETE FROM webhooks WHERE id = ?", id)
	return err
}

// ListWebhooks lists all webhooks
func (s *Store) ListWebhooks() ([]*Webhook, error) {
	query := `
		SELECT id, name, url, secret, events, format, enabled, headers, max_retries, retry_delay, created_at, updated_at
		FROM webhooks ORDER BY created_at DESC
	`

	rows, err := s.conn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var webhooks []*Webhook
	for rows.Next() {
		webhook, err := s.scanWebhookFromRows(rows)
		if err != nil {
			return nil, err
		}
		webhooks = append(webhooks, webhook)
	}

	return webhooks, rows.Err()
}

// GetWebhooksForEvent retrieves enabled webhooks that listen for an event type
func (s *Store) GetWebhooksForEvent(eventType EventType) ([]*Webhook, error) {
	webhooks, err := s.ListWebhooks()
	if err != nil {
		return nil, err
	}

	var matching []*Webhook
	for _, w := range webhooks {
		if !w.Enabled {
			continue
		}
		for _, e := range w.Events {
			if e == eventType {
				matching = append(matching, w)
				break
			}
		}
	}

	return matching, nil
}

// CreateDelivery inserts a new delivery
func (s *Store) CreateDelivery(delivery *Delivery) error {
	query := `
		INSERT INTO deliveries (id, webhook_id, event_id, event_type, payload, status, attempts, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.conn.Exec(query,
		delivery.ID,
		delivery.WebhookID,
		delivery.EventID,
		delivery.EventType,
		delivery.Payload,
		delivery.Status,
		delivery.Attempts,
		delivery.CreatedAt.Format(time.RFC3339),
	)

	return err
}

// UpdateDelivery updates an existing delivery
func (s *Store) UpdateDelivery(delivery *Delivery) error {
	query := `
		UPDATE deliveries SET
			status = ?,
			attempts = ?,
			last_attempt_at = ?,
			last_error = ?,
			response_code = ?,
			next_retry_at = ?,
			completed_at = ?
		WHERE id = ?
	`

	_, err := s.conn.Exec(query,
		delivery.Status,
		delivery.Attempts,
		nullTime(delivery.LastAttemptAt),
		nullString(delivery.LastError),
		delivery.ResponseCode,
		nullTime(delivery.NextRetryAt),
		nullTime(delivery.CompletedAt),
		delivery.ID,
	)

	return err
}

// GetPendingRetries retrieves deliveries ready for retry
func (s *Store) GetPendingRetries() ([]*Delivery, error) {
	query := `
		SELECT id, webhook_id, event_id, event_type, payload, status, attempts, last_attempt_at, last_error, response_code, next_retry_at, created_at, completed_at
		FROM deliveries
		WHERE status = 'pending' AND next_retry_at <= ?
		ORDER BY next_retry_at ASC
		LIMIT 50
	`

	rows, err := s.conn.Query(query, time.Now().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []*Delivery
	for rows.Next() {
		delivery, err := s.scanDeliveryFromRows(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, delivery)
	}

	return deliveries, rows.Err()
}

// ListDeliveries lists deliveries with filters
func (s *Store) ListDeliveries(opts ListDeliveriesOptions) (*ListDeliveriesResponse, error) {
	var conditions []string
	var args []interface{}

	if opts.WebhookID != "" {
		conditions = append(conditions, "webhook_id = ?")
		args = append(args, opts.WebhookID)
	}

	if len(opts.Status) > 0 {
		placeholders := make([]string, len(opts.Status))
		for i, s := range opts.Status {
			placeholders[i] = "?"
			args = append(args, s)
		}
		conditions = append(conditions, fmt.Sprintf("status IN (%s)", joinStrings(placeholders, ",")))
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + joinStrings(conditions, " AND ")
	}

	// Count total
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM deliveries %s", whereClause)
	var totalCount int
	if err := s.conn.QueryRow(countQuery, args...).Scan(&totalCount); err != nil {
		return nil, err
	}

	// Apply limit
	limit := opts.Limit
	if limit <= 0 {
		limit = 20
	}

	query := fmt.Sprintf(`
		SELECT id, webhook_id, event_id, event_type, payload, status, attempts, last_attempt_at, last_error, response_code, next_retry_at, created_at, completed_at
		FROM deliveries %s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, whereClause)

	args = append(args, limit, opts.Offset)

	rows, err := s.conn.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deliveries []Delivery
	for rows.Next() {
		delivery, err := s.scanDeliveryFromRows(rows)
		if err != nil {
			return nil, err
		}
		deliveries = append(deliveries, *delivery)
	}

	return &ListDeliveriesResponse{
		Deliveries: deliveries,
		TotalCount: totalCount,
	}, rows.Err()
}

// MoveToDeadLetter moves a failed delivery to the dead letter queue
func (s *Store) MoveToDeadLetter(delivery *Delivery) error {
	deadLetter := &DeadLetter{
		ID:        delivery.ID,
		WebhookID: delivery.WebhookID,
		EventID:   delivery.EventID,
		EventType: delivery.EventType,
		Payload:   delivery.Payload,
		LastError: delivery.LastError,
		Attempts:  delivery.Attempts,
		DeadAt:    time.Now(),
	}

	query := `
		INSERT INTO dead_letters (id, webhook_id, event_id, event_type, payload, last_error, attempts, dead_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err := s.conn.Exec(query,
		deadLetter.ID,
		deadLetter.WebhookID,
		deadLetter.EventID,
		deadLetter.EventType,
		deadLetter.Payload,
		deadLetter.LastError,
		deadLetter.Attempts,
		deadLetter.DeadAt.Format(time.RFC3339),
	)

	return err
}

// GetDeadLetters retrieves dead letter entries
func (s *Store) GetDeadLetters(webhookID string, limit int) ([]DeadLetter, error) {
	query := `
		SELECT id, webhook_id, event_id, event_type, payload, last_error, attempts, dead_at
		FROM dead_letters
		WHERE webhook_id = ?
		ORDER BY dead_at DESC
		LIMIT ?
	`

	if limit <= 0 {
		limit = 50
	}

	rows, err := s.conn.Query(query, webhookID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deadLetters []DeadLetter
	for rows.Next() {
		var dl DeadLetter
		var deadAt string

		err := rows.Scan(
			&dl.ID,
			&dl.WebhookID,
			&dl.EventID,
			&dl.EventType,
			&dl.Payload,
			&dl.LastError,
			&dl.Attempts,
			&deadAt,
		)
		if err != nil {
			return nil, err
		}

		if t, err := time.Parse(time.RFC3339, deadAt); err == nil {
			dl.DeadAt = t
		}

		deadLetters = append(deadLetters, dl)
	}

	return deadLetters, rows.Err()
}

// RetryDeadLetter moves a dead letter back to the delivery queue
func (s *Store) RetryDeadLetter(deadLetterID string) error {
	// Get dead letter
	var dl DeadLetter
	var deadAt string

	err := s.conn.QueryRow(`
		SELECT id, webhook_id, event_id, event_type, payload, last_error, attempts, dead_at
		FROM dead_letters WHERE id = ?
	`, deadLetterID).Scan(
		&dl.ID,
		&dl.WebhookID,
		&dl.EventID,
		&dl.EventType,
		&dl.Payload,
		&dl.LastError,
		&dl.Attempts,
		&deadAt,
	)
	if err != nil {
		return err
	}

	// Create new delivery
	delivery := &Delivery{
		ID:        generateDeliveryID(),
		WebhookID: dl.WebhookID,
		EventID:   dl.EventID,
		EventType: dl.EventType,
		Payload:   dl.Payload,
		Status:    DeliveryQueued,
		CreatedAt: time.Now(),
	}

	if err := s.CreateDelivery(delivery); err != nil {
		return err
	}

	// Delete dead letter
	_, err = s.conn.Exec("DELETE FROM dead_letters WHERE id = ?", deadLetterID)
	return err
}

// Scan helpers

func (s *Store) scanWebhook(row *sql.Row) (*Webhook, error) {
	var webhook Webhook
	var secret, headersJSON sql.NullString
	var eventsJSON, createdAt, updatedAt string
	var enabled int

	err := row.Scan(
		&webhook.ID,
		&webhook.Name,
		&webhook.URL,
		&secret,
		&eventsJSON,
		&webhook.Format,
		&enabled,
		&headersJSON,
		&webhook.MaxRetries,
		&webhook.RetryDelay,
		&createdAt,
		&updatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	webhook.Secret = secret.String
	webhook.Enabled = enabled != 0

	_ = json.Unmarshal([]byte(eventsJSON), &webhook.Events)
	if headersJSON.Valid {
		_ = json.Unmarshal([]byte(headersJSON.String), &webhook.Headers)
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		webhook.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		webhook.UpdatedAt = t
	}

	return &webhook, nil
}

func (s *Store) scanWebhookFromRows(rows *sql.Rows) (*Webhook, error) {
	var webhook Webhook
	var secret, headersJSON sql.NullString
	var eventsJSON, createdAt, updatedAt string
	var enabled int

	err := rows.Scan(
		&webhook.ID,
		&webhook.Name,
		&webhook.URL,
		&secret,
		&eventsJSON,
		&webhook.Format,
		&enabled,
		&headersJSON,
		&webhook.MaxRetries,
		&webhook.RetryDelay,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	webhook.Secret = secret.String
	webhook.Enabled = enabled != 0

	_ = json.Unmarshal([]byte(eventsJSON), &webhook.Events)
	if headersJSON.Valid {
		_ = json.Unmarshal([]byte(headersJSON.String), &webhook.Headers)
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		webhook.CreatedAt = t
	}
	if t, err := time.Parse(time.RFC3339, updatedAt); err == nil {
		webhook.UpdatedAt = t
	}

	return &webhook, nil
}

func (s *Store) scanDeliveryFromRows(rows *sql.Rows) (*Delivery, error) {
	var delivery Delivery
	var lastAttemptAt, lastError, nextRetryAt, completedAt sql.NullString
	var createdAt string
	var responseCode sql.NullInt64

	err := rows.Scan(
		&delivery.ID,
		&delivery.WebhookID,
		&delivery.EventID,
		&delivery.EventType,
		&delivery.Payload,
		&delivery.Status,
		&delivery.Attempts,
		&lastAttemptAt,
		&lastError,
		&responseCode,
		&nextRetryAt,
		&createdAt,
		&completedAt,
	)
	if err != nil {
		return nil, err
	}

	delivery.LastError = lastError.String
	if responseCode.Valid {
		delivery.ResponseCode = int(responseCode.Int64)
	}

	if t, err := time.Parse(time.RFC3339, createdAt); err == nil {
		delivery.CreatedAt = t
	}
	if lastAttemptAt.Valid {
		if t, err := time.Parse(time.RFC3339, lastAttemptAt.String); err == nil {
			delivery.LastAttemptAt = &t
		}
	}
	if nextRetryAt.Valid {
		if t, err := time.Parse(time.RFC3339, nextRetryAt.String); err == nil {
			delivery.NextRetryAt = &t
		}
	}
	if completedAt.Valid {
		if t, err := time.Parse(time.RFC3339, completedAt.String); err == nil {
			delivery.CompletedAt = &t
		}
	}

	return &delivery, nil
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
