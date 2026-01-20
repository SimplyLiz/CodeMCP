package auth

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	// KeyIDPrefix is the prefix for API key IDs
	KeyIDPrefix = "ckb_key_"

	// TokenPrefix is the prefix for API tokens (secret keys)
	TokenPrefix = "ckb_sk_" // #nosec G101 //nolint:gosec // Not a credential, just a prefix pattern

	// TokenPrefixLength is the number of characters to store as prefix for identification
	TokenPrefixLength = 8

	// KeyIDLength is the length of the random part of key IDs (in bytes, will be hex encoded)
	KeyIDLength = 8

	// TokenLength is the length of the random part of tokens (in bytes, will be hex encoded)
	TokenLength = 32

	// bcryptCost is the cost factor for bcrypt hashing
	bcryptCost = 12
)

// GenerateKeyID generates a new unique key ID
// Format: ckb_key_<16 hex chars>
func GenerateKeyID() (string, error) {
	bytes := make([]byte, KeyIDLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generate key ID: %w", err)
	}
	return KeyIDPrefix + hex.EncodeToString(bytes), nil
}

// GenerateToken generates a new API token
// Returns: raw token, prefix (for storage), error
// Format: ckb_sk_<prefix>_<rest of token>
func GenerateToken() (string, string, error) {
	bytes := make([]byte, TokenLength)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("generate token: %w", err)
	}

	hexToken := hex.EncodeToString(bytes)
	prefix := hexToken[:TokenPrefixLength]
	fullToken := TokenPrefix + hexToken

	return fullToken, prefix, nil
}

// HashToken creates a bcrypt hash of a token
func HashToken(token string) (string, error) {
	// Remove prefix for hashing (hash the actual secret)
	secret := strings.TrimPrefix(token, TokenPrefix)

	hash, err := bcrypt.GenerateFromPassword([]byte(secret), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("hash token: %w", err)
	}
	return string(hash), nil
}

// VerifyToken checks if a token matches a hash
func VerifyToken(token, hash string) bool {
	// Remove prefix for verification
	secret := strings.TrimPrefix(token, TokenPrefix)

	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(secret))
	return err == nil
}

// ExtractTokenPrefix extracts the prefix from a full token for lookup
func ExtractTokenPrefix(token string) string {
	secret := strings.TrimPrefix(token, TokenPrefix)
	if len(secret) < TokenPrefixLength {
		return secret
	}
	return secret[:TokenPrefixLength]
}

// IsValidTokenFormat checks if a token has the correct format
func IsValidTokenFormat(token string) bool {
	if !strings.HasPrefix(token, TokenPrefix) {
		return false
	}
	secret := strings.TrimPrefix(token, TokenPrefix)
	// Should be hex encoded and of expected length
	if len(secret) != TokenLength*2 {
		return false
	}
	_, err := hex.DecodeString(secret)
	return err == nil
}

// IsValidKeyIDFormat checks if a key ID has the correct format
func IsValidKeyIDFormat(keyID string) bool {
	if !strings.HasPrefix(keyID, KeyIDPrefix) {
		return false
	}
	id := strings.TrimPrefix(keyID, KeyIDPrefix)
	if len(id) != KeyIDLength*2 {
		return false
	}
	_, err := hex.DecodeString(id)
	return err == nil
}

// MaskToken returns a masked version of a token for display
// Example: ckb_sk_a1b2****...****
func MaskToken(token string) string {
	if len(token) < len(TokenPrefix)+TokenPrefixLength {
		return "****"
	}
	prefix := token[:len(TokenPrefix)+TokenPrefixLength]
	return prefix + "****...****"
}
