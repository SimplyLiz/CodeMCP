package a2a

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/envelope"
	"github.com/google/uuid"
)

// EnvelopeToArtifacts converts an envelope.Response to A2A artifacts.
func EnvelopeToArtifacts(resp *envelope.Response, skillName string) []Artifact {
	if resp == nil {
		return nil
	}

	var parts []Part

	// Main data as JSON text part
	if resp.Data != nil {
		dataJSON, err := json.Marshal(resp.Data)
		if err == nil {
			parts = append(parts, Part{
				Text:      string(dataJSON),
				MediaType: "application/json",
			})
		}
	}

	// Warnings as additional text parts
	for _, w := range resp.Warnings {
		parts = append(parts, Part{
			Text:      fmt.Sprintf("[warning] %s", w.Message),
			MediaType: "text/plain",
		})
	}

	if len(parts) == 0 {
		return nil
	}

	// Build metadata from envelope meta
	meta := make(map[string]any)
	if resp.Meta != nil {
		if resp.Meta.Confidence != nil {
			meta["confidence"] = resp.Meta.Confidence
		}
		if resp.Meta.Provenance != nil {
			meta["provenance"] = resp.Meta.Provenance
		}
		if resp.Meta.Freshness != nil {
			meta["freshness"] = resp.Meta.Freshness
		}
		if resp.Meta.Truncation != nil {
			meta["truncation"] = resp.Meta.Truncation
		}
	}

	artifact := Artifact{
		ArtifactID: uuid.New().String(),
		Name:       skillName + "-result",
		Parts:      parts,
	}
	if len(meta) > 0 {
		artifact.Metadata = meta
	}

	return []Artifact{artifact}
}

// EnvelopeToMessage converts an envelope.Response to an A2A agent message.
func EnvelopeToMessage(resp *envelope.Response, skillName string) Message {
	var parts []Part

	if resp != nil && resp.Data != nil {
		dataJSON, err := json.Marshal(resp.Data)
		if err == nil {
			parts = append(parts, Part{
				Text:      string(dataJSON),
				MediaType: "application/json",
			})
		}
	}

	if resp != nil && resp.Error != nil {
		parts = append(parts, Part{
			Text:      fmt.Sprintf("error: %s", *resp.Error),
			MediaType: "text/plain",
		})
	}

	if len(parts) == 0 {
		parts = []Part{{Text: "no data returned", MediaType: "text/plain"}}
	}

	return Message{
		MessageID: uuid.New().String(),
		Role:      RoleAgent,
		Parts:     parts,
	}
}

// SkillRequest is the expected JSON structure in a user message's first text part.
type SkillRequest struct {
	Skill  string         `json:"skill"`
	Params map[string]any `json:"params"`
}

// ParseSkillRequest extracts a skill invocation from a user message.
// The first text part should contain JSON: {"skill": "...", "params": {...}}
func ParseSkillRequest(msg Message) (skillID string, params map[string]any, err error) {
	for _, part := range msg.Parts {
		if part.Text == "" {
			continue
		}

		text := strings.TrimSpace(part.Text)
		if !strings.HasPrefix(text, "{") {
			continue
		}

		var req SkillRequest
		if jsonErr := json.Unmarshal([]byte(text), &req); jsonErr != nil {
			continue
		}
		if req.Skill != "" {
			if req.Params == nil {
				req.Params = make(map[string]any)
			}
			return req.Skill, req.Params, nil
		}
	}

	// Fallback: treat entire text as a natural-language query
	for _, part := range msg.Parts {
		if part.Text != "" {
			return "", nil, fmt.Errorf("no skill request found in message; expected JSON {\"skill\": \"...\", \"params\": {...}}")
		}
	}

	return "", nil, fmt.Errorf("empty message: no text parts")
}
