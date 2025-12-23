package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
)

// CursorData contains pagination state
type CursorData struct {
	Entity string `json:"e"`            // "symbol", "ref", "callgraph", "file"
	LastPK string `json:"pk,omitempty"` // Last seen primary key (for keyset pagination)
	Offset int64  `json:"o,omitempty"`  // Offset (for search results that can't use keyset)
}

// CursorManager handles cursor encoding/decoding with HMAC signing
type CursorManager struct {
	secret []byte
}

// NewCursorManager creates a new cursor manager with the given secret
func NewCursorManager(secret string) *CursorManager {
	return &CursorManager{
		secret: []byte(secret),
	}
}

// Encode encodes cursor data to a URL-safe string with HMAC signature
func (m *CursorManager) Encode(data CursorData) (string, error) {
	// Marshal to JSON
	payload, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal cursor: %w", err)
	}

	// Sign with HMAC
	signature := m.sign(payload)

	// Combine: base64(payload) + "." + base64(signature)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payload)
	sigB64 := base64.RawURLEncoding.EncodeToString(signature)

	return payloadB64 + "." + sigB64, nil
}

// Decode decodes and verifies a cursor string
func (m *CursorManager) Decode(cursor string) (*CursorData, error) {
	if cursor == "" {
		return nil, nil // Empty cursor is valid (first page)
	}

	// Split into payload and signature
	parts := strings.SplitN(cursor, ".", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid cursor format")
	}

	// Decode payload
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, fmt.Errorf("decode cursor payload: %w", err)
	}

	// Decode signature
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode cursor signature: %w", err)
	}

	// Verify signature
	expectedSig := m.sign(payload)
	if !hmac.Equal(signature, expectedSig) {
		return nil, fmt.Errorf("invalid cursor signature")
	}

	// Unmarshal payload
	var data CursorData
	if err := json.Unmarshal(payload, &data); err != nil {
		return nil, fmt.Errorf("unmarshal cursor: %w", err)
	}

	return &data, nil
}

// sign computes HMAC-SHA256 signature
func (m *CursorManager) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, m.secret)
	mac.Write(payload)
	return mac.Sum(nil)
}

// NewSymbolCursor creates a cursor for symbol pagination
func NewSymbolCursor(lastPK string) CursorData {
	return CursorData{
		Entity: "symbol",
		LastPK: lastPK,
	}
}

// NewRefCursor creates a cursor for reference pagination
func NewRefCursor(lastPK string) CursorData {
	return CursorData{
		Entity: "ref",
		LastPK: lastPK,
	}
}

// NewCallgraphCursor creates a cursor for callgraph pagination
func NewCallgraphCursor(lastPK string) CursorData {
	return CursorData{
		Entity: "callgraph",
		LastPK: lastPK,
	}
}

// NewFileCursor creates a cursor for file pagination
func NewFileCursor(lastPK string) CursorData {
	return CursorData{
		Entity: "file",
		LastPK: lastPK,
	}
}

// NewSearchCursor creates a cursor for search result pagination (offset-based)
func NewSearchCursor(entity string, offset int64) CursorData {
	return CursorData{
		Entity: entity,
		Offset: offset,
	}
}

// ValidateEntity checks if the cursor entity matches the expected type
func (c *CursorData) ValidateEntity(expected string) error {
	if c == nil {
		return nil // Nil cursor is valid (first page)
	}
	if c.Entity != expected {
		return fmt.Errorf("cursor entity mismatch: got %s, expected %s", c.Entity, expected)
	}
	return nil
}
