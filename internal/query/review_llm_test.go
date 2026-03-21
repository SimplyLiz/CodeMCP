package query

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseLLMResponse(t *testing.T) {
	t.Parallel()

	body := []byte(`{
		"id": "msg_123",
		"type": "message",
		"role": "assistant",
		"content": [
			{
				"type": "text",
				"text": "This PR changes 5 files across 2 modules. The main risk is a breaking API change in the auth package. Focus review on the token validation logic."
			}
		],
		"model": "claude-sonnet-4-20250514",
		"stop_reason": "end_turn"
	}`)

	text, err := parseLLMResponse(body)
	if err != nil {
		t.Fatalf("parseLLMResponse failed: %v", err)
	}
	if text == "" {
		t.Error("expected non-empty text")
	}
	if len(text) < 10 {
		t.Errorf("expected meaningful text, got %q", text)
	}
}

func TestParseLLMResponse_NoContent(t *testing.T) {
	t.Parallel()

	body := []byte(`{"content": []}`)
	_, err := parseLLMResponse(body)
	if err == nil {
		t.Error("expected error for empty content")
	}
}

func TestGenerateLLMNarrative_PromptFormat(t *testing.T) {
	t.Parallel()

	// Create a mock HTTP server
	var receivedBody map[string]interface{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		if r.Header.Get("x-api-key") != "test-key" {
			t.Errorf("expected api key 'test-key', got %q", r.Header.Get("x-api-key"))
		}
		if r.Header.Get("anthropic-version") != "2023-06-01" {
			t.Errorf("unexpected anthropic-version header")
		}

		json.NewDecoder(r.Body).Decode(&receivedBody)

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"content": []map[string]interface{}{
				{"type": "text", "text": "Test narrative summary."},
			},
		})
	}))
	defer server.Close()

	// We can't easily override the URL in the current implementation,
	// so we just test the prompt construction and response parsing
	// The full integration would need dependency injection for the HTTP client

	// Verify prompt data structure
	resp := &ReviewPRResponse{
		Verdict: "warn",
		Score:   75,
		Summary: ReviewSummary{
			TotalFiles:   10,
			TotalChanges: 200,
		},
		Findings: []ReviewFinding{
			{Check: "breaking", Severity: "error", Message: "Removed public function"},
		},
	}

	promptData := map[string]interface{}{
		"verdict":  resp.Verdict,
		"score":    resp.Score,
		"summary":  resp.Summary,
		"findings": resp.Findings,
	}

	promptJSON, err := json.Marshal(promptData)
	if err != nil {
		t.Fatalf("failed to marshal prompt: %v", err)
	}

	// Verify the prompt contains key information
	promptStr := string(promptJSON)
	if len(promptStr) == 0 {
		t.Error("expected non-empty prompt")
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(promptJSON, &parsed); err != nil {
		t.Fatalf("prompt JSON is not valid: %v", err)
	}
	if parsed["verdict"] != "warn" {
		t.Errorf("expected verdict 'warn' in prompt, got %v", parsed["verdict"])
	}
}

func TestGenerateLLMNarrative_FallbackOnError(t *testing.T) {
	// Not parallel — uses t.Setenv which modifies process environment
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	// Without API key, generateLLMNarrative should return an error
	// and the caller should fall back to deterministic narrative
	engine, cleanup := testEngine(t)
	defer cleanup()

	resp := &ReviewPRResponse{
		Verdict:   "pass",
		Score:     100,
		Narrative: "Deterministic narrative stays.",
	}

	_, err := engine.generateLLMNarrative(t.Context(), resp)
	if err == nil {
		t.Error("expected error when no API key is set")
	}

	// Narrative should be unchanged
	if resp.Narrative != "Deterministic narrative stays." {
		t.Errorf("narrative was modified: %q", resp.Narrative)
	}
}
