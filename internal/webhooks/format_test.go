package webhooks

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// testManager creates a minimal manager for testing format functions
// The format functions don't require any Manager state, just the methods
func testManager() *Manager {
	return &Manager{}
}

func TestFormatJSON(t *testing.T) {
	m := testManager()

	tests := []struct {
		name      string
		eventType EventType
		source    string
		data      map[string]interface{}
	}{
		{
			name:      "refresh completed",
			eventType: EventRefreshCompleted,
			source:    "my-repo",
			data:      map[string]interface{}{"status": "ok"},
		},
		{
			name:      "job failed",
			eventType: EventJobFailed,
			source:    "worker-1",
			data:      map[string]interface{}{"error": "timeout"},
		},
		{
			name:      "hotspot alert",
			eventType: EventHotspotAlert,
			source:    "main-service",
			data:      map[string]interface{}{"file": "service.go", "churn": 50},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := NewEvent(tt.eventType, tt.source, tt.data)
			if err != nil {
				t.Fatalf("NewEvent failed: %v", err)
			}

			payload, err := m.formatJSON(event)
			if err != nil {
				t.Fatalf("formatJSON failed: %v", err)
			}

			// Parse the payload
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(payload), &result); err != nil {
				t.Fatalf("Failed to parse JSON payload: %v", err)
			}

			// Verify required fields
			if result["event_id"] == nil {
				t.Error("event_id should be present")
			}
			if result["event_type"] != string(tt.eventType) {
				t.Errorf("event_type = %v, want %v", result["event_type"], tt.eventType)
			}
			if result["source"] != tt.source {
				t.Errorf("source = %v, want %v", result["source"], tt.source)
			}
			if result["timestamp"] == nil {
				t.Error("timestamp should be present")
			}
			if result["data"] == nil {
				t.Error("data should be present")
			}
		})
	}
}

func TestFormatSlack(t *testing.T) {
	m := testManager()

	tests := []struct {
		name          string
		eventType     EventType
		source        string
		expectedColor string
		containsText  string
	}{
		{
			name:          "refresh completed - green",
			eventType:     EventRefreshCompleted,
			source:        "my-repo",
			expectedColor: "good",
			containsText:  "refresh completed",
		},
		{
			name:          "refresh failed - red",
			eventType:     EventRefreshFailed,
			source:        "my-repo",
			expectedColor: "danger",
			containsText:  "refresh failed",
		},
		{
			name:          "hotspot alert - warning",
			eventType:     EventHotspotAlert,
			source:        "service",
			expectedColor: "warning",
			containsText:  "Hotspot detected",
		},
		{
			name:          "job completed",
			eventType:     EventJobCompleted,
			source:        "indexer",
			expectedColor: "good",
			containsText:  "Job completed",
		},
		{
			name:          "job failed",
			eventType:     EventJobFailed,
			source:        "indexer",
			expectedColor: "danger",
			containsText:  "Job failed",
		},
		{
			name:          "unknown event type",
			eventType:     EventFederationSync,
			source:        "federation",
			expectedColor: "#36a64f",
			containsText:  "CKB event",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := NewEvent(tt.eventType, tt.source, map[string]interface{}{})
			if err != nil {
				t.Fatalf("NewEvent failed: %v", err)
			}

			payload, err := m.formatSlack(event)
			if err != nil {
				t.Fatalf("formatSlack failed: %v", err)
			}

			// Parse the payload
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(payload), &result); err != nil {
				t.Fatalf("Failed to parse JSON payload: %v", err)
			}

			// Check attachments structure
			attachments, ok := result["attachments"].([]interface{})
			if !ok || len(attachments) == 0 {
				t.Fatal("Expected attachments array")
			}

			attachment, ok := attachments[0].(map[string]interface{})
			if !ok {
				t.Fatal("attachment should be a map")
			}

			// Verify color
			if attachment["color"] != tt.expectedColor {
				t.Errorf("color = %v, want %v", attachment["color"], tt.expectedColor)
			}

			// Verify text contains expected substring
			text, ok := attachment["text"].(string)
			if !ok {
				t.Fatal("text should be a string")
			}
			if !strings.Contains(text, tt.containsText) {
				t.Errorf("text = %q, should contain %q", text, tt.containsText)
			}

			// Verify footer
			if attachment["footer"] != "CKB" {
				t.Errorf("footer = %v, want 'CKB'", attachment["footer"])
			}

			// Verify timestamp is numeric
			if _, ok := attachment["ts"].(float64); !ok {
				t.Error("ts should be a number")
			}
		})
	}
}

func TestFormatPagerDuty(t *testing.T) {
	m := testManager()

	tests := []struct {
		name             string
		eventType        EventType
		expectedSeverity string
	}{
		{
			name:             "refresh failed - error",
			eventType:        EventRefreshFailed,
			expectedSeverity: "error",
		},
		{
			name:             "job failed - error",
			eventType:        EventJobFailed,
			expectedSeverity: "error",
		},
		{
			name:             "hotspot alert - warning",
			eventType:        EventHotspotAlert,
			expectedSeverity: "warning",
		},
		{
			name:             "health degraded - warning",
			eventType:        EventHealthDegraded,
			expectedSeverity: "warning",
		},
		{
			name:             "refresh completed - info",
			eventType:        EventRefreshCompleted,
			expectedSeverity: "info",
		},
		{
			name:             "job completed - info",
			eventType:        EventJobCompleted,
			expectedSeverity: "info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := NewEvent(tt.eventType, "test-source", map[string]interface{}{"key": "value"})
			if err != nil {
				t.Fatalf("NewEvent failed: %v", err)
			}

			payload, err := m.formatPagerDuty(event)
			if err != nil {
				t.Fatalf("formatPagerDuty failed: %v", err)
			}

			// Parse the payload
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(payload), &result); err != nil {
				t.Fatalf("Failed to parse JSON payload: %v", err)
			}

			// Check event_action
			if result["event_action"] != "trigger" {
				t.Errorf("event_action = %v, want 'trigger'", result["event_action"])
			}

			// Check payload structure
			pdPayload, ok := result["payload"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected payload object")
			}

			// Verify severity
			if pdPayload["severity"] != tt.expectedSeverity {
				t.Errorf("severity = %v, want %v", pdPayload["severity"], tt.expectedSeverity)
			}

			// Verify source
			if pdPayload["source"] != "ckb" {
				t.Errorf("source = %v, want 'ckb'", pdPayload["source"])
			}

			// Verify summary contains event type
			summary, ok := pdPayload["summary"].(string)
			if !ok {
				t.Fatal("summary should be a string")
			}
			if !strings.Contains(summary, string(tt.eventType)) {
				t.Errorf("summary = %q, should contain %q", summary, tt.eventType)
			}

			// Verify custom_details exists
			customDetails, ok := pdPayload["custom_details"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected custom_details object")
			}
			if customDetails["event_id"] == nil {
				t.Error("custom_details should have event_id")
			}
		})
	}
}

func TestFormatDiscord(t *testing.T) {
	m := testManager()

	tests := []struct {
		name          string
		eventType     EventType
		expectedColor int
		expectedTitle string
	}{
		{
			name:          "refresh completed - green",
			eventType:     EventRefreshCompleted,
			expectedColor: 0x00FF00,
			expectedTitle: "CKB Success",
		},
		{
			name:          "job completed - green",
			eventType:     EventJobCompleted,
			expectedColor: 0x00FF00,
			expectedTitle: "CKB Success",
		},
		{
			name:          "refresh failed - red",
			eventType:     EventRefreshFailed,
			expectedColor: 0xFF0000,
			expectedTitle: "CKB Error",
		},
		{
			name:          "job failed - red",
			eventType:     EventJobFailed,
			expectedColor: 0xFF0000,
			expectedTitle: "CKB Error",
		},
		{
			name:          "hotspot alert - orange",
			eventType:     EventHotspotAlert,
			expectedColor: 0xFFA500,
			expectedTitle: "CKB Warning",
		},
		{
			name:          "health degraded - orange",
			eventType:     EventHealthDegraded,
			expectedColor: 0xFFA500,
			expectedTitle: "CKB Warning",
		},
		{
			name:          "federation sync - blue",
			eventType:     EventFederationSync,
			expectedColor: 0x0000FF,
			expectedTitle: "CKB Info",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			event, err := NewEvent(tt.eventType, "test-source", map[string]interface{}{})
			if err != nil {
				t.Fatalf("NewEvent failed: %v", err)
			}

			payload, err := m.formatDiscord(event)
			if err != nil {
				t.Fatalf("formatDiscord failed: %v", err)
			}

			// Parse the payload
			var result map[string]interface{}
			if err := json.Unmarshal([]byte(payload), &result); err != nil {
				t.Fatalf("Failed to parse JSON payload: %v", err)
			}

			// Check embeds structure
			embeds, ok := result["embeds"].([]interface{})
			if !ok || len(embeds) == 0 {
				t.Fatal("Expected embeds array")
			}

			embed, ok := embeds[0].(map[string]interface{})
			if !ok {
				t.Fatal("embed should be a map")
			}

			// Verify color (JSON numbers are float64)
			colorVal, ok := embed["color"].(float64)
			if !ok {
				t.Fatalf("color should be a number, got %T", embed["color"])
			}
			if int(colorVal) != tt.expectedColor {
				t.Errorf("color = %x, want %x", int(colorVal), tt.expectedColor)
			}

			// Verify title
			if embed["title"] != tt.expectedTitle {
				t.Errorf("title = %v, want %v", embed["title"], tt.expectedTitle)
			}

			// Verify footer
			footer, ok := embed["footer"].(map[string]interface{})
			if !ok {
				t.Fatal("Expected footer object")
			}
			if footer["text"] != "CKB Daemon" {
				t.Errorf("footer.text = %v, want 'CKB Daemon'", footer["text"])
			}

			// Verify timestamp is present
			if embed["timestamp"] == nil {
				t.Error("timestamp should be present")
			}
		})
	}
}

func TestFormatPayload(t *testing.T) {
	m := testManager()

	event, err := NewEvent(EventRefreshCompleted, "test", map[string]interface{}{"key": "value"})
	if err != nil {
		t.Fatalf("NewEvent failed: %v", err)
	}

	tests := []struct {
		name   string
		format Format
	}{
		{"JSON format", FormatJSON},
		{"Slack format", FormatSlack},
		{"PagerDuty format", FormatPagerDuty},
		{"Discord format", FormatDiscord},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			webhook := &Webhook{Format: tt.format}
			payload, err := m.formatPayload(webhook, event)
			if err != nil {
				t.Fatalf("formatPayload failed: %v", err)
			}

			if payload == "" {
				t.Error("payload should not be empty")
			}

			// Should be valid JSON
			var result interface{}
			if err := json.Unmarshal([]byte(payload), &result); err != nil {
				t.Errorf("payload should be valid JSON: %v", err)
			}
		})
	}

	t.Run("unknown format defaults to JSON", func(t *testing.T) {
		webhook := &Webhook{Format: "unknown"}
		payload, err := m.formatPayload(webhook, event)
		if err != nil {
			t.Fatalf("formatPayload failed: %v", err)
		}

		// Should contain JSON format fields
		var result map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &result); err != nil {
			t.Fatalf("Failed to parse payload: %v", err)
		}

		// JSON format has event_id field
		if result["event_id"] == nil {
			t.Error("unknown format should default to JSON format")
		}
	})
}

func TestSignPayload(t *testing.T) {
	m := testManager()

	tests := []struct {
		name    string
		payload string
		secret  string
	}{
		{
			name:    "simple payload",
			payload: `{"test": true}`,
			secret:  "my-secret",
		},
		{
			name:    "empty payload",
			payload: "",
			secret:  "secret",
		},
		{
			name:    "complex payload",
			payload: `{"event_type": "refresh_completed", "source": "repo", "data": {"key": "value"}}`,
			secret:  "complex-secret-123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			signature := m.signPayload(tt.payload, tt.secret)

			// Signature should be a hex string
			if signature == "" {
				t.Error("signature should not be empty")
			}

			// Should be consistent for same inputs
			signature2 := m.signPayload(tt.payload, tt.secret)
			if signature != signature2 {
				t.Error("signature should be consistent for same inputs")
			}

			// Should differ with different secrets
			differentSig := m.signPayload(tt.payload, tt.secret+"different")
			if signature == differentSig {
				t.Error("signature should differ with different secrets")
			}
		})
	}
}

func TestFormatWithTimestamps(t *testing.T) {
	m := testManager()

	event, err := NewEvent(EventRefreshCompleted, "test", map[string]interface{}{})
	if err != nil {
		t.Fatalf("NewEvent failed: %v", err)
	}

	t.Run("JSON timestamp is RFC3339", func(t *testing.T) {
		payload, err := m.formatJSON(event)
		if err != nil {
			t.Fatalf("formatJSON failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &result); err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}

		ts, ok := result["timestamp"].(string)
		if !ok {
			t.Fatal("timestamp should be a string")
		}

		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("timestamp should be RFC3339 format: %v", err)
		}
	})

	t.Run("Slack timestamp is Unix", func(t *testing.T) {
		payload, err := m.formatSlack(event)
		if err != nil {
			t.Fatalf("formatSlack failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &result); err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}

		attachments, ok := result["attachments"].([]interface{})
		if !ok || len(attachments) == 0 {
			t.Fatal("Expected attachments array")
		}
		attachment, ok := attachments[0].(map[string]interface{})
		if !ok {
			t.Fatal("attachment should be a map")
		}

		_, ok = attachment["ts"].(float64)
		if !ok {
			t.Error("Slack ts should be a Unix timestamp (number)")
		}
	})

	t.Run("Discord timestamp is RFC3339", func(t *testing.T) {
		payload, err := m.formatDiscord(event)
		if err != nil {
			t.Fatalf("formatDiscord failed: %v", err)
		}

		var result map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &result); err != nil {
			t.Fatalf("Failed to parse: %v", err)
		}

		embeds, ok := result["embeds"].([]interface{})
		if !ok || len(embeds) == 0 {
			t.Fatal("Expected embeds array")
		}
		embed, ok := embeds[0].(map[string]interface{})
		if !ok {
			t.Fatal("embed should be a map")
		}

		ts, ok := embed["timestamp"].(string)
		if !ok {
			t.Fatal("timestamp should be a string")
		}

		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			t.Errorf("timestamp should be RFC3339 format: %v", err)
		}
	})
}
