package query

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	defaultLLMModel     = "claude-sonnet-4-20250514"
	anthropicAPIURL     = "https://api.anthropic.com/v1/messages"
	anthropicAPIVersion = "2023-06-01"
)

// generateLLMNarrative calls the Anthropic API to produce a narrative summary.
func (e *Engine) generateLLMNarrative(ctx context.Context, resp *ReviewPRResponse) (string, error) {
	apiKey := ""
	if e.config != nil && e.config.LLM.APIKey != "" {
		apiKey = e.config.LLM.APIKey
	}
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	if apiKey == "" {
		return "", fmt.Errorf("no API key: set ANTHROPIC_API_KEY or config.llm.apiKey")
	}

	model := defaultLLMModel
	if e.config != nil && e.config.LLM.Model != "" {
		model = e.config.LLM.Model
	}

	// Build prompt with top findings
	topFindings := resp.Findings
	if len(topFindings) > 10 {
		topFindings = topFindings[:10]
	}

	promptData := map[string]interface{}{
		"verdict":  resp.Verdict,
		"score":    resp.Score,
		"summary":  resp.Summary,
		"findings": topFindings,
	}
	if resp.HealthReport != nil {
		promptData["healthReport"] = map[string]interface{}{
			"degraded":     resp.HealthReport.Degraded,
			"improved":     resp.HealthReport.Improved,
			"averageDelta": resp.HealthReport.AverageDelta,
		}
	}

	promptJSON, err := json.Marshal(promptData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal prompt data: %w", err)
	}

	reqBody := map[string]interface{}{
		"model":      model,
		"max_tokens": 256,
		"system":     "You are CKB, a code review tool. Write a concise 2-3 sentence narrative summary of a PR review. Focus on what matters most: blocking issues, key risks, and where reviewers should focus. Be direct and specific. Do not use markdown formatting.",
		"messages": []map[string]interface{}{
			{
				"role":    "user",
				"content": "Summarize this PR review:\n\n" + string(promptJSON),
			},
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	httpCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(httpCtx, http.MethodPost, anthropicAPIURL, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicAPIVersion)

	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API request failed: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	if httpResp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned %d: %s", httpResp.StatusCode, string(respBody))
	}

	return parseLLMResponse(respBody)
}

// parseLLMResponse extracts the text content from an Anthropic API response.
func parseLLMResponse(body []byte) (string, error) {
	var result struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("failed to parse response: %w", err)
	}

	for _, block := range result.Content {
		if block.Type == "text" {
			return block.Text, nil
		}
	}

	return "", fmt.Errorf("no text content in response")
}
