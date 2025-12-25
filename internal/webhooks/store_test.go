package webhooks

import (
	"testing"
	"time"

	"ckb/internal/logging"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	tmpDir := t.TempDir()
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})

	mgr, err := NewManager(tmpDir, logger, Config{
		WorkerCount:   1,
		RetryInterval: time.Second,
		Timeout:       time.Second,
	})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	t.Cleanup(func() { _ = mgr.Stop(time.Second) })
	return mgr
}

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})

	mgr, err := NewManager(tmpDir, logger, Config{
		WorkerCount:   2,
		RetryInterval: time.Minute,
		Timeout:       30 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.Stop(time.Second)

	if mgr.store == nil {
		t.Error("store should not be nil")
	}
	if mgr.logger == nil {
		t.Error("logger should not be nil")
	}
}

func TestNewManager_DefaultConfig(t *testing.T) {
	tmpDir := t.TempDir()
	logger := logging.NewLogger(logging.Config{
		Level:  logging.ErrorLevel,
		Format: logging.HumanFormat,
	})

	// Empty config should use defaults
	mgr, err := NewManager(tmpDir, logger, Config{})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	defer mgr.Stop(time.Second)

	// Verify defaults are applied
	if mgr.workerCount != 2 {
		t.Errorf("workerCount = %d, want 2 (default)", mgr.workerCount)
	}
}

func TestManager_RegisterAndGetWebhook(t *testing.T) {
	mgr := newTestManager(t)

	webhook := &Webhook{
		ID:      "wh-1",
		Name:    "Test Hook",
		URL:     "https://example.com/webhook",
		Events:  []EventType{"index.updated"},
		Enabled: true,
		Format:  FormatJSON,
		Secret:  "secret123",
		Headers: map[string]string{"env": "test"},
	}

	if err := mgr.RegisterWebhook(webhook); err != nil {
		t.Fatalf("RegisterWebhook failed: %v", err)
	}

	// Get it back
	retrieved, err := mgr.GetWebhook("wh-1")
	if err != nil {
		t.Fatalf("GetWebhook failed: %v", err)
	}

	if retrieved.ID != webhook.ID {
		t.Errorf("ID = %q, want %q", retrieved.ID, webhook.ID)
	}
	if retrieved.Name != webhook.Name {
		t.Errorf("Name = %q, want %q", retrieved.Name, webhook.Name)
	}
	if retrieved.URL != webhook.URL {
		t.Errorf("URL = %q, want %q", retrieved.URL, webhook.URL)
	}
	if !retrieved.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestManager_GetWebhook_NotFound(t *testing.T) {
	mgr := newTestManager(t)

	// Manager returns (nil, nil) for not found, not an error
	webhook, err := mgr.GetWebhook("nonexistent")
	if err != nil {
		t.Errorf("GetWebhook error = %v, want nil", err)
	}
	if webhook != nil {
		t.Errorf("GetWebhook = %v, want nil for nonexistent", webhook)
	}
}

func TestManager_UpdateWebhook(t *testing.T) {
	mgr := newTestManager(t)

	webhook := &Webhook{
		ID:      "wh-1",
		Name:    "Original",
		URL:     "https://example.com/webhook",
		Events:  []EventType{"index.updated"},
		Enabled: true,
		Format:  FormatJSON,
	}
	mgr.RegisterWebhook(webhook)

	// Update
	webhook.Name = "Updated"
	webhook.Enabled = false
	if err := mgr.UpdateWebhook(webhook); err != nil {
		t.Fatalf("UpdateWebhook failed: %v", err)
	}

	// Verify
	retrieved, _ := mgr.GetWebhook("wh-1")
	if retrieved.Name != "Updated" {
		t.Errorf("Name = %q, want 'Updated'", retrieved.Name)
	}
	if retrieved.Enabled {
		t.Error("Enabled should be false")
	}
}

func TestManager_DeleteWebhook(t *testing.T) {
	mgr := newTestManager(t)

	webhook := &Webhook{
		ID:      "wh-1",
		Name:    "Test",
		URL:     "https://example.com/webhook",
		Events:  []EventType{"index.updated"},
		Enabled: true,
		Format:  FormatJSON,
	}
	mgr.RegisterWebhook(webhook)

	if err := mgr.DeleteWebhook("wh-1"); err != nil {
		t.Fatalf("DeleteWebhook failed: %v", err)
	}

	// GetWebhook returns (nil, nil) for deleted webhook
	webhook, err := mgr.GetWebhook("wh-1")
	if err != nil {
		t.Errorf("GetWebhook error = %v, want nil", err)
	}
	if webhook != nil {
		t.Error("Webhook should be deleted (nil)")
	}
}

func TestManager_DeleteWebhook_NotFound(t *testing.T) {
	mgr := newTestManager(t)

	// DeleteWebhook does not error for nonexistent webhook (idempotent delete)
	err := mgr.DeleteWebhook("nonexistent")
	if err != nil {
		t.Errorf("DeleteWebhook error = %v, want nil (idempotent)", err)
	}
}

func TestManager_ListWebhooks(t *testing.T) {
	mgr := newTestManager(t)

	// Empty list
	webhooks, err := mgr.ListWebhooks()
	if err != nil {
		t.Fatalf("ListWebhooks failed: %v", err)
	}
	if len(webhooks) != 0 {
		t.Errorf("Webhooks = %d, want 0", len(webhooks))
	}

	// Add some webhooks
	for i := 1; i <= 3; i++ {
		webhook := &Webhook{
			ID:      "wh-" + string(rune('0'+i)),
			Name:    "Test",
			URL:     "https://example.com/webhook",
			Events:  []EventType{"index.updated"},
			Enabled: true,
			Format:  FormatJSON,
		}
		mgr.RegisterWebhook(webhook)
	}

	webhooks, err = mgr.ListWebhooks()
	if err != nil {
		t.Fatal(err)
	}
	if len(webhooks) != 3 {
		t.Errorf("Webhooks = %d, want 3", len(webhooks))
	}
}

func TestManager_Emit_NoWebhooks(t *testing.T) {
	mgr := newTestManager(t)

	event := &Event{
		ID:        "evt-1",
		Type:      "index.updated",
		Source:    "/test/repo",
		Timestamp: time.Now(),
		Data:      []byte(`{"key":"value"}`),
	}

	// Should not error when no webhooks registered
	err := mgr.Emit(event)
	if err != nil {
		t.Errorf("Emit with no webhooks should not error: %v", err)
	}
}

func TestManager_ListDeliveries(t *testing.T) {
	mgr := newTestManager(t)

	// Empty list
	resp, err := mgr.ListDeliveries(ListDeliveriesOptions{Limit: 10})
	if err != nil {
		t.Fatalf("ListDeliveries failed: %v", err)
	}
	if len(resp.Deliveries) != 0 {
		t.Errorf("Deliveries = %d, want 0", len(resp.Deliveries))
	}
}

func TestManager_GetDeadLetters(t *testing.T) {
	mgr := newTestManager(t)

	// Empty list
	deadLetters, err := mgr.GetDeadLetters("wh-1", 10)
	if err != nil {
		t.Fatalf("GetDeadLetters failed: %v", err)
	}
	if len(deadLetters) != 0 {
		t.Errorf("DeadLetters = %d, want 0", len(deadLetters))
	}
}

func TestManager_SignPayload(t *testing.T) {
	mgr := newTestManager(t)

	signature := mgr.signPayload(`{"test":"data"}`, "secret123")
	if signature == "" {
		t.Error("Signature should not be empty")
	}

	// Same input should produce same signature
	signature2 := mgr.signPayload(`{"test":"data"}`, "secret123")
	if signature != signature2 {
		t.Error("Same input should produce same signature")
	}

	// Different input should produce different signature
	signature3 := mgr.signPayload(`{"test":"different"}`, "secret123")
	if signature == signature3 {
		t.Error("Different input should produce different signature")
	}
}

func TestManager_FormatJSON(t *testing.T) {
	mgr := newTestManager(t)

	event := &Event{
		ID:        "evt-1",
		Type:      "index.updated",
		Source:    "/test/repo",
		Timestamp: time.Now(),
		Data:      []byte(`{"key":"value"}`),
	}

	payload, err := mgr.formatJSON(event)
	if err != nil {
		t.Fatalf("formatJSON failed: %v", err)
	}
	if payload == "" {
		t.Error("Payload should not be empty")
	}
}

func TestManager_FormatSlack(t *testing.T) {
	mgr := newTestManager(t)

	event := &Event{
		ID:        "evt-1",
		Type:      "index.updated",
		Source:    "/test/repo",
		Timestamp: time.Now(),
		Data:      []byte(`{"key":"value"}`),
	}

	payload, err := mgr.formatSlack(event)
	if err != nil {
		t.Fatalf("formatSlack failed: %v", err)
	}
	if payload == "" {
		t.Error("Payload should not be empty")
	}
}

func TestManager_FormatDiscord(t *testing.T) {
	mgr := newTestManager(t)

	event := &Event{
		ID:        "evt-1",
		Type:      "index.updated",
		Source:    "/test/repo",
		Timestamp: time.Now(),
		Data:      []byte(`{"key":"value"}`),
	}

	payload, err := mgr.formatDiscord(event)
	if err != nil {
		t.Fatalf("formatDiscord failed: %v", err)
	}
	if payload == "" {
		t.Error("Payload should not be empty")
	}
}

func TestManager_FormatPagerDuty(t *testing.T) {
	mgr := newTestManager(t)

	event := &Event{
		ID:        "evt-1",
		Type:      "index.updated",
		Source:    "/test/repo",
		Timestamp: time.Now(),
		Data:      []byte(`{"key":"value"}`),
	}

	payload, err := mgr.formatPagerDuty(event)
	if err != nil {
		t.Fatalf("formatPagerDuty failed: %v", err)
	}
	if payload == "" {
		t.Error("Payload should not be empty")
	}
}

func TestManager_FormatPayload(t *testing.T) {
	mgr := newTestManager(t)

	event := &Event{
		ID:        "evt-1",
		Type:      "index.updated",
		Source:    "/test/repo",
		Timestamp: time.Now(),
		Data:      []byte(`{"key":"value"}`),
	}

	tests := []struct {
		format Format
	}{
		{FormatJSON},
		{FormatSlack},
		{FormatDiscord},
		{FormatPagerDuty},
	}

	for _, tt := range tests {
		t.Run(string(tt.format), func(t *testing.T) {
			webhook := &Webhook{Format: tt.format}
			payload, err := mgr.formatPayload(webhook, event)
			if err != nil {
				t.Fatalf("formatPayload(%s) failed: %v", tt.format, err)
			}
			if payload == "" {
				t.Error("Payload should not be empty")
			}
		})
	}
}

func TestWebhookFormatConstants_Store(t *testing.T) {
	formats := []Format{FormatJSON, FormatSlack, FormatDiscord, FormatPagerDuty}
	for _, f := range formats {
		if string(f) == "" {
			t.Errorf("Format %v should not be empty", f)
		}
	}
}

func TestEventTypeConstants_Store(t *testing.T) {
	// Verify event types are strings
	eventTypes := []EventType{"index.updated", "index.created", "error.occurred"}
	for _, et := range eventTypes {
		if string(et) == "" {
			t.Errorf("EventType %v should not be empty", et)
		}
	}
}

func TestWebhookStruct_Store(t *testing.T) {
	webhook := &Webhook{
		ID:         "wh-1",
		Name:       "Test Webhook",
		URL:        "https://example.com/hook",
		Events:     []EventType{"index.updated", "error.occurred"},
		Enabled:    true,
		Format:     FormatJSON,
		Secret:     "secret123",
		Headers:    map[string]string{"env": "prod"},
		MaxRetries: 3,
	}

	if webhook.ID != "wh-1" {
		t.Errorf("ID = %q, want 'wh-1'", webhook.ID)
	}
	if len(webhook.Events) != 2 {
		t.Errorf("Events len = %d, want 2", len(webhook.Events))
	}
	if webhook.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", webhook.MaxRetries)
	}
}

func TestEventStruct(t *testing.T) {
	now := time.Now()
	event := &Event{
		ID:        "evt-123",
		Type:      "index.updated",
		Source:    "/path/to/repo",
		Timestamp: now,
		Data:      []byte(`{"files": 10}`),
	}

	if event.ID != "evt-123" {
		t.Errorf("ID = %q, want 'evt-123'", event.ID)
	}
	if event.Type != "index.updated" {
		t.Errorf("Type = %q, want 'index.updated'", event.Type)
	}
	if !event.Timestamp.Equal(now) {
		t.Errorf("Timestamp mismatch")
	}
}

func TestDeliveryStruct_Store(t *testing.T) {
	now := time.Now()
	delivery := &Delivery{
		ID:            "del-1",
		WebhookID:     "wh-1",
		EventID:       "evt-1",
		Status:        DeliveryPending,
		Attempts:      1,
		LastAttemptAt: &now,
		NextRetryAt:   &now,
		ResponseCode:  200,
	}

	if delivery.ID != "del-1" {
		t.Errorf("ID = %q, want 'del-1'", delivery.ID)
	}
	if delivery.Status != DeliveryPending {
		t.Errorf("Status = %v, want %v", delivery.Status, DeliveryPending)
	}
	if delivery.ResponseCode != 200 {
		t.Errorf("ResponseCode = %d, want 200", delivery.ResponseCode)
	}
}

func TestListDeliveriesOptions_Store(t *testing.T) {
	opts := ListDeliveriesOptions{
		WebhookID: "wh-1",
		Status:    []DeliveryStatus{DeliveryFailed},
		Limit:     50,
		Offset:    10,
	}

	if opts.WebhookID != "wh-1" {
		t.Errorf("WebhookID = %q, want 'wh-1'", opts.WebhookID)
	}
	if opts.Limit != 50 {
		t.Errorf("Limit = %d, want 50", opts.Limit)
	}
}

func TestConfig(t *testing.T) {
	cfg := Config{
		WorkerCount:   4,
		RetryInterval: 5 * time.Minute,
		Timeout:       30 * time.Second,
	}

	if cfg.WorkerCount != 4 {
		t.Errorf("WorkerCount = %d, want 4", cfg.WorkerCount)
	}
	if cfg.RetryInterval != 5*time.Minute {
		t.Errorf("RetryInterval = %v, want 5m", cfg.RetryInterval)
	}
}
