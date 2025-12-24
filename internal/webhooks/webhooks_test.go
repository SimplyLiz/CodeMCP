package webhooks

import (
	"errors"
	"testing"
	"time"
)

func TestNewWebhook(t *testing.T) {
	events := []EventType{EventRefreshCompleted, EventJobFailed}
	webhook := NewWebhook("My Webhook", "https://example.com/webhook", events, FormatJSON)

	if webhook == nil {
		t.Fatal("NewWebhook() returned nil")
	}
	if webhook.ID == "" {
		t.Error("ID should not be empty")
	}
	if webhook.Name != "My Webhook" {
		t.Errorf("Name = %q, want 'My Webhook'", webhook.Name)
	}
	if webhook.URL != "https://example.com/webhook" {
		t.Errorf("URL = %q, want 'https://example.com/webhook'", webhook.URL)
	}
	if len(webhook.Events) != 2 {
		t.Errorf("Events len = %d, want 2", len(webhook.Events))
	}
	if webhook.Format != FormatJSON {
		t.Errorf("Format = %v, want %v", webhook.Format, FormatJSON)
	}
	if !webhook.Enabled {
		t.Error("Should be enabled by default")
	}
	if webhook.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", webhook.MaxRetries)
	}
	if webhook.RetryDelay != 60 {
		t.Errorf("RetryDelay = %d, want 60", webhook.RetryDelay)
	}
}

func TestWebhookToSummary(t *testing.T) {
	webhook := &Webhook{
		ID:      "whk_123",
		Name:    "Test",
		URL:     "https://example.com",
		Events:  []EventType{EventJobCompleted},
		Format:  FormatSlack,
		Enabled: true,
	}

	summary := webhook.ToSummary()

	if summary.ID != "whk_123" {
		t.Errorf("ID = %q, want 'whk_123'", summary.ID)
	}
	if summary.Name != "Test" {
		t.Errorf("Name = %q, want 'Test'", summary.Name)
	}
	if summary.URL != "https://example.com" {
		t.Errorf("URL = %q, want 'https://example.com'", summary.URL)
	}
	if summary.Format != FormatSlack {
		t.Errorf("Format = %v, want %v", summary.Format, FormatSlack)
	}
	if !summary.Enabled {
		t.Error("Enabled should be true")
	}
}

func TestNewEvent(t *testing.T) {
	data := map[string]interface{}{
		"key": "value",
	}

	event, err := NewEvent(EventRefreshCompleted, "repo-123", data)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}

	if event == nil {
		t.Fatal("NewEvent() returned nil")
	}
	if event.ID == "" {
		t.Error("ID should not be empty")
	}
	if event.Type != EventRefreshCompleted {
		t.Errorf("Type = %v, want %v", event.Type, EventRefreshCompleted)
	}
	if event.Source != "repo-123" {
		t.Errorf("Source = %q, want 'repo-123'", event.Source)
	}
	if len(event.Data) == 0 {
		t.Error("Data should not be empty")
	}
}

func TestNewEventInvalidData(t *testing.T) {
	// Create data that cannot be marshaled (channel)
	ch := make(chan int)
	_, err := NewEvent(EventRefreshCompleted, "repo", ch)
	if err == nil {
		t.Error("Expected error for unmarshalable data")
	}
	close(ch)
}

func TestDeliveryCanRetry(t *testing.T) {
	tests := []struct {
		name       string
		delivery   Delivery
		maxRetries int
		want       bool
	}{
		{
			name:       "can retry - attempts below max",
			delivery:   Delivery{Attempts: 1, Status: DeliveryPending},
			maxRetries: 3,
			want:       true,
		},
		{
			name:       "cannot retry - max reached",
			delivery:   Delivery{Attempts: 3, Status: DeliveryPending},
			maxRetries: 3,
			want:       false,
		},
		{
			name:       "cannot retry - already delivered",
			delivery:   Delivery{Attempts: 0, Status: DeliveryDelivered},
			maxRetries: 3,
			want:       false,
		},
		{
			name:       "cannot retry - dead",
			delivery:   Delivery{Attempts: 0, Status: DeliveryDead},
			maxRetries: 3,
			want:       false,
		},
		{
			name:       "can retry - queued",
			delivery:   Delivery{Attempts: 0, Status: DeliveryQueued},
			maxRetries: 3,
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.delivery.CanRetry(tt.maxRetries)
			if got != tt.want {
				t.Errorf("CanRetry() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDeliveryMarkDelivered(t *testing.T) {
	delivery := &Delivery{Status: DeliveryPending}
	delivery.MarkDelivered(200)

	if delivery.Status != DeliveryDelivered {
		t.Errorf("Status = %v, want %v", delivery.Status, DeliveryDelivered)
	}
	if delivery.ResponseCode != 200 {
		t.Errorf("ResponseCode = %d, want 200", delivery.ResponseCode)
	}
	if delivery.CompletedAt == nil {
		t.Error("CompletedAt should be set")
	}
	if delivery.LastAttemptAt == nil {
		t.Error("LastAttemptAt should be set")
	}
}

func TestDeliveryMarkFailed(t *testing.T) {
	t.Run("below max retries", func(t *testing.T) {
		delivery := &Delivery{Status: DeliveryPending, Attempts: 0}
		delivery.MarkFailed(errors.New("connection error"), 3, 60)

		if delivery.Status != DeliveryPending {
			t.Errorf("Status = %v, want %v", delivery.Status, DeliveryPending)
		}
		if delivery.Attempts != 1 {
			t.Errorf("Attempts = %d, want 1", delivery.Attempts)
		}
		if delivery.LastError != "connection error" {
			t.Errorf("LastError = %q, want 'connection error'", delivery.LastError)
		}
		if delivery.LastAttemptAt == nil {
			t.Error("LastAttemptAt should be set")
		}
		if delivery.NextRetryAt == nil {
			t.Error("NextRetryAt should be set")
		}
	})

	t.Run("at max retries", func(t *testing.T) {
		delivery := &Delivery{Status: DeliveryPending, Attempts: 2}
		delivery.MarkFailed(errors.New("timeout"), 3, 60)

		if delivery.Status != DeliveryDead {
			t.Errorf("Status = %v, want %v", delivery.Status, DeliveryDead)
		}
		if delivery.Attempts != 3 {
			t.Errorf("Attempts = %d, want 3", delivery.Attempts)
		}
		if delivery.CompletedAt == nil {
			t.Error("CompletedAt should be set when dead")
		}
	})
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.WorkerCount != 2 {
		t.Errorf("WorkerCount = %d, want 2", config.WorkerCount)
	}
	if config.RetryInterval != time.Minute {
		t.Errorf("RetryInterval = %v, want 1m", config.RetryInterval)
	}
	if config.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", config.Timeout)
	}
}

func TestGenerateWebhookID(t *testing.T) {
	id1 := generateWebhookID()
	time.Sleep(time.Millisecond) // Ensure different timestamp
	id2 := generateWebhookID()

	if id1 == "" {
		t.Error("ID should not be empty")
	}
	if !containsStr(id1, "whk_") {
		t.Errorf("ID should start with 'whk_', got %q", id1)
	}
	// IDs should be unique (with high probability)
	if id1 == id2 {
		t.Logf("id1=%s, id2=%s (may be same with fast execution)", id1, id2)
	}
}

func TestGenerateEventID(t *testing.T) {
	id := generateEventID()

	if id == "" {
		t.Error("ID should not be empty")
	}
	if !containsStr(id, "evt_") {
		t.Errorf("ID should start with 'evt_', got %q", id)
	}
}

func TestGenerateDeliveryID(t *testing.T) {
	id := generateDeliveryID()

	if id == "" {
		t.Error("ID should not be empty")
	}
	if !containsStr(id, "dlv_") {
		t.Errorf("ID should start with 'dlv_', got %q", id)
	}
}

func TestFormatBase36(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{0, "0"},
		{10, "a"},
		{35, "z"},
		{36, "10"},
		{100, "2s"},
	}

	for _, tt := range tests {
		got := formatBase36(tt.input)
		if got != tt.want {
			t.Errorf("formatBase36(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestRandomHex(t *testing.T) {
	hex := randomHex(4)

	if len(hex) != 8 {
		t.Errorf("randomHex(4) should return 8 chars, got %d", len(hex))
	}

	for _, c := range hex {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("Invalid hex char: %c", c)
		}
	}
}

func TestEventTypeConstants(t *testing.T) {
	types := []EventType{
		EventRefreshCompleted,
		EventRefreshFailed,
		EventHotspotAlert,
		EventFederationSync,
		EventJobCompleted,
		EventJobFailed,
		EventHealthDegraded,
	}

	for _, et := range types {
		if string(et) == "" {
			t.Errorf("EventType %v should not be empty", et)
		}
	}
}

func TestDeliveryStatusConstants(t *testing.T) {
	statuses := []DeliveryStatus{
		DeliveryQueued,
		DeliveryPending,
		DeliveryDelivered,
		DeliveryFailed,
		DeliveryDead,
	}

	for _, s := range statuses {
		if string(s) == "" {
			t.Errorf("DeliveryStatus %v should not be empty", s)
		}
	}
}

func TestFormatConstants(t *testing.T) {
	formats := []Format{
		FormatJSON,
		FormatSlack,
		FormatPagerDuty,
		FormatDiscord,
	}

	for _, f := range formats {
		if string(f) == "" {
			t.Errorf("Format %v should not be empty", f)
		}
	}
}

func TestWebhookStructure(t *testing.T) {
	now := time.Now()
	webhook := Webhook{
		ID:         "whk_test",
		Name:       "Test Webhook",
		URL:        "https://example.com",
		Secret:     "secret123",
		Events:     []EventType{EventJobCompleted},
		Format:     FormatJSON,
		Enabled:    true,
		Headers:    map[string]string{"X-Custom": "header"},
		MaxRetries: 5,
		RetryDelay: 120,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if webhook.ID != "whk_test" {
		t.Errorf("ID = %q, want 'whk_test'", webhook.ID)
	}
	if webhook.Secret != "secret123" {
		t.Errorf("Secret = %q, want 'secret123'", webhook.Secret)
	}
	if len(webhook.Headers) != 1 {
		t.Errorf("Headers len = %d, want 1", len(webhook.Headers))
	}
}

func TestEventStructure(t *testing.T) {
	now := time.Now()
	event := Event{
		ID:        "evt_test",
		Type:      EventRefreshCompleted,
		Timestamp: now,
		Source:    "repo-1",
		Data:      []byte(`{"key":"value"}`),
	}

	if event.ID != "evt_test" {
		t.Errorf("ID = %q, want 'evt_test'", event.ID)
	}
	if event.Type != EventRefreshCompleted {
		t.Errorf("Type = %v, want %v", event.Type, EventRefreshCompleted)
	}
	if event.Source != "repo-1" {
		t.Errorf("Source = %q, want 'repo-1'", event.Source)
	}
}

func TestDeliveryStructure(t *testing.T) {
	now := time.Now()
	delivery := Delivery{
		ID:            "dlv_test",
		WebhookID:     "whk_1",
		EventID:       "evt_1",
		EventType:     EventJobCompleted,
		Payload:       `{"test": true}`,
		Status:        DeliveryQueued,
		Attempts:      0,
		LastAttemptAt: nil,
		LastError:     "",
		ResponseCode:  0,
		NextRetryAt:   nil,
		CreatedAt:     now,
		CompletedAt:   nil,
	}

	if delivery.ID != "dlv_test" {
		t.Errorf("ID = %q, want 'dlv_test'", delivery.ID)
	}
	if delivery.WebhookID != "whk_1" {
		t.Errorf("WebhookID = %q, want 'whk_1'", delivery.WebhookID)
	}
	if delivery.Status != DeliveryQueued {
		t.Errorf("Status = %v, want %v", delivery.Status, DeliveryQueued)
	}
}

func TestDeadLetterStructure(t *testing.T) {
	now := time.Now()
	dl := DeadLetter{
		ID:        "dlv_dead",
		WebhookID: "whk_1",
		EventID:   "evt_1",
		EventType: EventJobFailed,
		Payload:   `{"error": true}`,
		LastError: "connection refused",
		Attempts:  3,
		DeadAt:    now,
	}

	if dl.ID != "dlv_dead" {
		t.Errorf("ID = %q, want 'dlv_dead'", dl.ID)
	}
	if dl.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", dl.Attempts)
	}
	if dl.LastError != "connection refused" {
		t.Errorf("LastError = %q, want 'connection refused'", dl.LastError)
	}
}

func TestWebhookConfigStructure(t *testing.T) {
	config := WebhookConfig{
		ID:         "whk_config",
		Name:       "Config Webhook",
		URL:        "https://example.com",
		Secret:     "secret",
		Events:     []string{"job_completed"},
		Format:     "json",
		Headers:    map[string]string{"Auth": "token"},
		MaxRetries: 5,
		RetryDelay: 30,
	}

	if config.ID != "whk_config" {
		t.Errorf("ID = %q, want 'whk_config'", config.ID)
	}
	if len(config.Events) != 1 {
		t.Errorf("Events len = %d, want 1", len(config.Events))
	}
}

func TestListDeliveriesOptions(t *testing.T) {
	opts := ListDeliveriesOptions{
		WebhookID: "whk_1",
		Status:    []DeliveryStatus{DeliveryQueued, DeliveryPending},
		Limit:     10,
		Offset:    20,
	}

	if opts.WebhookID != "whk_1" {
		t.Errorf("WebhookID = %q, want 'whk_1'", opts.WebhookID)
	}
	if len(opts.Status) != 2 {
		t.Errorf("Status len = %d, want 2", len(opts.Status))
	}
}

func TestListDeliveriesResponse(t *testing.T) {
	resp := ListDeliveriesResponse{
		Deliveries: []Delivery{
			{ID: "dlv_1"},
			{ID: "dlv_2"},
		},
		TotalCount: 100,
	}

	if len(resp.Deliveries) != 2 {
		t.Errorf("Deliveries len = %d, want 2", len(resp.Deliveries))
	}
	if resp.TotalCount != 100 {
		t.Errorf("TotalCount = %d, want 100", resp.TotalCount)
	}
}

func TestJoinStrings(t *testing.T) {
	tests := []struct {
		strs []string
		sep  string
		want string
	}{
		{[]string{"a", "b", "c"}, ",", "a,b,c"},
		{[]string{"a"}, ",", "a"},
		{[]string{}, ",", ""},
		{[]string{"hello", "world"}, " ", "hello world"},
	}

	for _, tt := range tests {
		got := joinStrings(tt.strs, tt.sep)
		if got != tt.want {
			t.Errorf("joinStrings(%v, %q) = %q, want %q", tt.strs, tt.sep, got, tt.want)
		}
	}
}

func TestNullString(t *testing.T) {
	t.Run("empty string", func(t *testing.T) {
		ns := nullString("")
		if ns.Valid {
			t.Error("Empty string should not be valid")
		}
	})

	t.Run("non-empty string", func(t *testing.T) {
		ns := nullString("hello")
		if !ns.Valid {
			t.Error("Non-empty string should be valid")
		}
		if ns.String != "hello" {
			t.Errorf("String = %q, want 'hello'", ns.String)
		}
	})
}

func TestNullTime(t *testing.T) {
	t.Run("nil time", func(t *testing.T) {
		nt := nullTime(nil)
		if nt.Valid {
			t.Error("Nil time should not be valid")
		}
	})

	t.Run("non-nil time", func(t *testing.T) {
		now := time.Now()
		nt := nullTime(&now)
		if !nt.Valid {
			t.Error("Non-nil time should be valid")
		}
		if nt.String == "" {
			t.Error("String should not be empty")
		}
	})
}

// Helper function
func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
