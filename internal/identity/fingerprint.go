package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

// SymbolFingerprint contains the components used to generate a stable ID
type SymbolFingerprint struct {
	QualifiedContainer  string     `json:"qualifiedContainer"`            // e.g., "mypackage.MyClass"
	Name                string     `json:"name"`                          // Symbol name
	Kind                SymbolKind `json:"kind"`                          // Symbol kind
	Arity               int        `json:"arity,omitempty"`               // Number of parameters (for overloading)
	SignatureNormalized string     `json:"signatureNormalized,omitempty"` // Normalized signature
}

// ComputeStableFingerprint creates a deterministic hash from fingerprint components
// This hash is stable across refactorings that preserve the symbol's identity
func ComputeStableFingerprint(fp *SymbolFingerprint) string {
	if fp == nil {
		return ""
	}

	// Build a canonical string representation
	parts := []string{
		"container:" + fp.QualifiedContainer,
		"name:" + fp.Name,
		"kind:" + string(fp.Kind),
	}

	if fp.Arity > 0 {
		parts = append(parts, fmt.Sprintf("arity:%d", fp.Arity))
	}

	if fp.SignatureNormalized != "" {
		parts = append(parts, "sig:"+fp.SignatureNormalized)
	}

	// Sort to ensure deterministic ordering
	sort.Strings(parts)

	// Join and hash
	canonical := strings.Join(parts, "|")
	hash := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(hash[:])
}

// GenerateStableId creates the full stable ID from repo name and fingerprint
// Format: ckb:<repo>:sym:<fingerprint-hash>
func GenerateStableId(repoName string, fingerprint *SymbolFingerprint) string {
	if fingerprint == nil {
		return ""
	}

	// Sanitize repo name (remove special chars, lowercase)
	sanitized := sanitizeRepoName(repoName)
	fingerprintHash := ComputeStableFingerprint(fingerprint)

	return fmt.Sprintf("ckb:%s:sym:%s", sanitized, fingerprintHash)
}

// sanitizeRepoName converts a repo name to a safe, deterministic format
func sanitizeRepoName(repoName string) string {
	// Remove path separators and special characters
	sanitized := strings.ReplaceAll(repoName, "/", "-")
	sanitized = strings.ReplaceAll(sanitized, "\\", "-")
	sanitized = strings.ReplaceAll(sanitized, ":", "-")
	sanitized = strings.ToLower(sanitized)

	// Trim leading/trailing dashes
	sanitized = strings.Trim(sanitized, "-")

	// If empty, use default
	if sanitized == "" {
		sanitized = "unknown"
	}

	return sanitized
}

// NormalizeSignature attempts to create a normalized signature for comparison
// This removes whitespace, standardizes formatting, etc.
func NormalizeSignature(signature string) string {
	// Remove all whitespace
	normalized := strings.Map(func(r rune) rune {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			return -1
		}
		return r
	}, signature)

	return normalized
}

// ExtractArity extracts the parameter count from a signature
// This is a simple implementation - backends should provide accurate arity
func ExtractArity(signature string) int {
	// Count commas in parameter list (simple heuristic)
	// Real implementation would parse the signature properly
	if !strings.Contains(signature, "(") {
		return 0
	}

	start := strings.Index(signature, "(")
	end := strings.LastIndex(signature, ")")
	if start == -1 || end == -1 || start >= end {
		return 0
	}

	params := signature[start+1 : end]
	params = strings.TrimSpace(params)

	if params == "" {
		return 0
	}

	// Count commas + 1 (rough estimate)
	return strings.Count(params, ",") + 1
}

// ComputeDefinitionVersionId computes a hash from the full signature
// This changes whenever the definition changes
func ComputeDefinitionVersionId(signature string) string {
	if signature == "" {
		return ""
	}

	normalized := NormalizeSignature(signature)
	hash := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(hash[:])
}
