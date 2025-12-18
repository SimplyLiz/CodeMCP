// Package webhooks provides webhook management and delivery.
package webhooks

import (
	"encoding/json"
	"time"
)

// EventType represents the type of webhook event
type EventType string

const (
	EventRefreshCompleted EventType = "refresh_completed"
	EventRefreshFailed    EventType = "refresh_failed"
	EventHotspotAlert     EventType = "hotspot_alert"
	EventFederationSync   EventType = "federation_sync"
	EventJobCompleted     EventType = "job_completed"
	EventJobFailed        EventType = "job_failed"
	EventHealthDegraded   EventType = "health_degraded"
)

// DeliveryStatus represents the status of a webhook delivery
type DeliveryStatus string

const (
	DeliveryQueued    DeliveryStatus = "queued"
	DeliveryPending   DeliveryStatus = "pending"
	DeliveryDelivered DeliveryStatus = "delivered"
	DeliveryFailed    DeliveryStatus = "failed"
	DeliveryDead      DeliveryStatus = "dead" // moved to dead letter queue
)

// Format represents the payload format
type Format string

const (
	FormatJSON      Format = "json"
	FormatSlack     Format = "slack"
	FormatPagerDuty Format = "pagerduty"
	FormatDiscord   Format = "discord"
)

// Webhook represents a configured webhook endpoint
type Webhook struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	Secret     string            `json:"-"` // Never serialize
	Events     []EventType       `json:"events"`
	Format     Format            `json:"format"`
	Enabled    bool              `json:"enabled"`
	Headers    map[string]string `json:"headers,omitempty"`
	MaxRetries int               `json:"maxRetries"`
	RetryDelay int               `json:"retryDelay"` // seconds
	CreatedAt  time.Time         `json:"createdAt"`
	UpdatedAt  time.Time         `json:"updatedAt"`
}

// WebhookSummary is a lightweight view for listing
type WebhookSummary struct {
	ID      string      `json:"id"`
	Name    string      `json:"name"`
	URL     string      `json:"url"`
	Events  []EventType `json:"events"`
	Format  Format      `json:"format"`
	Enabled bool        `json:"enabled"`
}

// ToSummary creates a summary view
func (w *Webhook) ToSummary() WebhookSummary {
	return WebhookSummary{
		ID:      w.ID,
		Name:    w.Name,
		URL:     w.URL,
		Events:  w.Events,
		Format:  w.Format,
		Enabled: w.Enabled,
	}
}

// Event represents a webhook event
type Event struct {
	ID        string          `json:"id"`
	Type      EventType       `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Source    string          `json:"source"` // repo or federation name
	Data      json.RawMessage `json:"data"`
}

// Delivery represents a webhook delivery attempt
type Delivery struct {
	ID            string         `json:"id"`
	WebhookID     string         `json:"webhookId"`
	EventID       string         `json:"eventId"`
	EventType     EventType      `json:"eventType"`
	Payload       string         `json:"payload"`
	Status        DeliveryStatus `json:"status"`
	Attempts      int            `json:"attempts"`
	LastAttemptAt *time.Time     `json:"lastAttemptAt,omitempty"`
	LastError     string         `json:"lastError,omitempty"`
	ResponseCode  int            `json:"responseCode,omitempty"`
	NextRetryAt   *time.Time     `json:"nextRetryAt,omitempty"`
	CreatedAt     time.Time      `json:"createdAt"`
	CompletedAt   *time.Time     `json:"completedAt,omitempty"`
}

// DeadLetter represents a failed delivery in the dead letter queue
type DeadLetter struct {
	ID        string    `json:"id"`
	WebhookID string    `json:"webhookId"`
	EventID   string    `json:"eventId"`
	EventType EventType `json:"eventType"`
	Payload   string    `json:"payload"`
	LastError string    `json:"lastError"`
	Attempts  int       `json:"attempts"`
	DeadAt    time.Time `json:"deadAt"`
}

// WebhookConfig defines a webhook in configuration
type WebhookConfig struct {
	ID         string            `json:"id" mapstructure:"id"`
	Name       string            `json:"name" mapstructure:"name"`
	URL        string            `json:"url" mapstructure:"url"`
	Secret     string            `json:"secret,omitempty" mapstructure:"secret"`
	Events     []string          `json:"events" mapstructure:"events"`
	Format     string            `json:"format" mapstructure:"format"`
	Headers    map[string]string `json:"headers,omitempty" mapstructure:"headers"`
	MaxRetries int               `json:"maxRetries,omitempty" mapstructure:"max_retries"`
	RetryDelay int               `json:"retryDelay,omitempty" mapstructure:"retry_delay"`
}

// ListDeliveriesOptions contains options for listing deliveries
type ListDeliveriesOptions struct {
	WebhookID string           `json:"webhookId,omitempty"`
	Status    []DeliveryStatus `json:"status,omitempty"`
	Limit     int              `json:"limit,omitempty"`
	Offset    int              `json:"offset,omitempty"`
}

// ListDeliveriesResponse contains the result of listing deliveries
type ListDeliveriesResponse struct {
	Deliveries []Delivery `json:"deliveries"`
	TotalCount int        `json:"totalCount"`
}

// NewWebhook creates a new webhook
func NewWebhook(name, url string, events []EventType, format Format) *Webhook {
	now := time.Now()
	return &Webhook{
		ID:         generateWebhookID(),
		Name:       name,
		URL:        url,
		Events:     events,
		Format:     format,
		Enabled:    true,
		MaxRetries: 3,
		RetryDelay: 60, // 1 minute
		CreatedAt:  now,
		UpdatedAt:  now,
	}
}

// NewEvent creates a new webhook event
func NewEvent(eventType EventType, source string, data interface{}) (*Event, error) {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	return &Event{
		ID:        generateEventID(),
		Type:      eventType,
		Timestamp: time.Now(),
		Source:    source,
		Data:      dataJSON,
	}, nil
}

// CanRetry returns true if the delivery can be retried
func (d *Delivery) CanRetry(maxRetries int) bool {
	return d.Attempts < maxRetries && d.Status != DeliveryDelivered && d.Status != DeliveryDead
}

// MarkDelivered marks the delivery as successful
func (d *Delivery) MarkDelivered(responseCode int) {
	now := time.Now()
	d.Status = DeliveryDelivered
	d.ResponseCode = responseCode
	d.CompletedAt = &now
	d.LastAttemptAt = &now
}

// MarkFailed marks the delivery as failed with error
func (d *Delivery) MarkFailed(err error, maxRetries int, retryDelay int) {
	now := time.Now()
	d.LastAttemptAt = &now
	d.LastError = err.Error()
	d.Attempts++

	if d.Attempts >= maxRetries {
		d.Status = DeliveryDead
		d.CompletedAt = &now
	} else {
		d.Status = DeliveryPending
		nextRetry := now.Add(time.Duration(retryDelay) * time.Second * time.Duration(d.Attempts)) // exponential backoff
		d.NextRetryAt = &nextRetry
	}
}

// generateWebhookID generates a unique webhook ID
func generateWebhookID() string {
	timestamp := time.Now().UnixNano()
	random := randomHex(3)
	return "whk_" + formatBase36(timestamp) + "_" + random
}

// generateEventID generates a unique event ID
func generateEventID() string {
	timestamp := time.Now().UnixNano()
	random := randomHex(3)
	return "evt_" + formatBase36(timestamp) + "_" + random
}

// generateDeliveryID generates a unique delivery ID
func generateDeliveryID() string {
	timestamp := time.Now().UnixNano()
	random := randomHex(3)
	return "dlv_" + formatBase36(timestamp) + "_" + random
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
