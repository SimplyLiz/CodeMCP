package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"ckb/internal/auth"
	"ckb/internal/logging"

	_ "modernc.org/sqlite" // Pure Go SQLite driver
)

var (
	tokenDataDir      string
	tokenName         string
	tokenScopes       []string
	tokenRepos        []string
	tokenExpires      string
	tokenRateLimit    int
	tokenFormat       string
	tokenShowRevoked  bool
)

var tokenCmd = &cobra.Command{
	Use:   "token",
	Short: "Manage API tokens for index server",
	Long: `Create, list, and revoke API tokens for authenticating with the CKB index server.

Tokens are stored in the server data directory (default: ~/.ckb-server).

Examples:
  ckb token create --name "CI Upload" --scopes write
  ckb token create --name "Read-only" --scopes read --repos "myorg/*"
  ckb token list
  ckb token revoke ckb_key_abc123`,
}

var tokenCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new API token",
	Long: `Create a new API token with specified scopes and optional restrictions.

Scopes:
  read   - Can read symbols, files, search (GET requests)
  write  - Can upload indexes, create repos (POST requests)
  admin  - Full access including token management and deletions

Examples:
  ckb token create --name "CI Upload" --scopes write
  ckb token create --name "Read-only" --scopes read
  ckb token create --name "Admin" --scopes admin --expires 30d
  ckb token create --name "Restricted" --scopes write --repos "myorg/*"`,
	Run: runTokenCreate,
}

var tokenListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all API tokens",
	Long: `List all API tokens with their scopes, restrictions, and last used time.

Examples:
  ckb token list
  ckb token list --show-revoked
  ckb token list --format json`,
	Run: runTokenList,
}

var tokenRevokeCmd = &cobra.Command{
	Use:   "revoke <key-id>",
	Short: "Revoke an API token",
	Long: `Revoke an API token, immediately invalidating it.

Examples:
  ckb token revoke ckb_key_abc123`,
	Args: cobra.ExactArgs(1),
	Run:  runTokenRevoke,
}

var tokenRotateCmd = &cobra.Command{
	Use:   "rotate <key-id>",
	Short: "Rotate an API token (generate new secret)",
	Long: `Generate a new secret for an existing API token, invalidating the old one.

The key ID remains the same, but a new token is generated.

Examples:
  ckb token rotate ckb_key_abc123`,
	Args: cobra.ExactArgs(1),
	Run:  runTokenRotate,
}

func init() {
	// Common flags
	tokenCmd.PersistentFlags().StringVar(&tokenDataDir, "data-dir", "~/.ckb-server", "Server data directory")
	tokenCmd.PersistentFlags().StringVar(&tokenFormat, "format", "human", "Output format (json, human)")

	// Create flags
	tokenCreateCmd.Flags().StringVar(&tokenName, "name", "", "Token name (required)")
	tokenCreateCmd.Flags().StringSliceVar(&tokenScopes, "scopes", nil, "Scopes: read, write, admin (required)")
	tokenCreateCmd.Flags().StringSliceVar(&tokenRepos, "repos", nil, "Restrict to repos matching patterns")
	tokenCreateCmd.Flags().StringVar(&tokenExpires, "expires", "", "Expiration (e.g., 30d, 1h, 2024-12-31)")
	tokenCreateCmd.Flags().IntVar(&tokenRateLimit, "rate-limit", 0, "Rate limit (requests per minute, 0=default)")
	_ = tokenCreateCmd.MarkFlagRequired("name")
	_ = tokenCreateCmd.MarkFlagRequired("scopes")

	// List flags
	tokenListCmd.Flags().BoolVar(&tokenShowRevoked, "show-revoked", false, "Include revoked tokens")

	tokenCmd.AddCommand(tokenCreateCmd)
	tokenCmd.AddCommand(tokenListCmd)
	tokenCmd.AddCommand(tokenRevokeCmd)
	tokenCmd.AddCommand(tokenRotateCmd)
	rootCmd.AddCommand(tokenCmd)
}

func runTokenCreate(cmd *cobra.Command, args []string) {
	logger := newLogger(tokenFormat)
	manager := mustGetAuthManager(logger)

	// Parse scopes
	var scopes []auth.Scope
	for _, s := range tokenScopes {
		scope := auth.Scope(strings.ToLower(s))
		if !scope.IsValid() {
			fmt.Fprintf(os.Stderr, "Error: invalid scope '%s' (valid: read, write, admin)\n", s)
			os.Exit(1)
		}
		scopes = append(scopes, scope)
	}

	// Parse expiration
	var expiresAt *time.Time
	if tokenExpires != "" {
		t, err := parseExpiration(tokenExpires)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: invalid expiration '%s': %v\n", tokenExpires, err)
			os.Exit(1)
		}
		expiresAt = &t
	}

	// Parse rate limit
	var rateLimit *int
	if tokenRateLimit > 0 {
		rateLimit = &tokenRateLimit
	}

	opts := auth.CreateKeyOptions{
		Name:         tokenName,
		Scopes:       scopes,
		RepoPatterns: tokenRepos,
		RateLimit:    rateLimit,
		ExpiresAt:    expiresAt,
		CreatedBy:    os.Getenv("USER"),
	}

	key, rawToken, err := manager.CreateKey(opts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating token: %v\n", err)
		os.Exit(1)
	}

	if tokenFormat == "json" {
		resp := map[string]interface{}{
			"key_id":      key.ID,
			"name":        key.Name,
			"scopes":      key.Scopes,
			"token":       rawToken,
			"created_at":  key.CreatedAt.Format(time.RFC3339),
		}
		if len(key.RepoPatterns) > 0 {
			resp["repo_patterns"] = key.RepoPatterns
		}
		if key.RateLimit != nil {
			resp["rate_limit"] = *key.RateLimit
		}
		if key.ExpiresAt != nil {
			resp["expires_at"] = key.ExpiresAt.Format(time.RFC3339)
		}
		printJSON(resp)
	} else {
		fmt.Println("API Token Created:")
		fmt.Println()
		fmt.Printf("  ID:      %s\n", key.ID)
		fmt.Printf("  Name:    %s\n", key.Name)
		fmt.Printf("  Scopes:  %s\n", formatScopes(key.Scopes))
		if len(key.RepoPatterns) > 0 {
			fmt.Printf("  Repos:   %s\n", strings.Join(key.RepoPatterns, ", "))
		}
		if key.RateLimit != nil {
			fmt.Printf("  Rate:    %d/min\n", *key.RateLimit)
		}
		if key.ExpiresAt != nil {
			fmt.Printf("  Expires: %s\n", key.ExpiresAt.Format("2006-01-02"))
		}
		fmt.Printf("  Token:   %s\n", rawToken)
		fmt.Println()
		fmt.Println("  IMPORTANT: Store this token securely. It will not be shown again.")
	}
}

func runTokenList(cmd *cobra.Command, args []string) {
	logger := newLogger(tokenFormat)
	manager := mustGetAuthManager(logger)

	keys, err := manager.ListKeys(tokenShowRevoked)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error listing tokens: %v\n", err)
		os.Exit(1)
	}

	if tokenFormat == "json" {
		printJSON(map[string]interface{}{
			"tokens": keys,
			"count":  len(keys),
		})
	} else {
		if len(keys) == 0 {
			fmt.Println("No API tokens found.")
			return
		}

		fmt.Println("API Tokens:")
		fmt.Println()
		fmt.Printf("  %-26s %-16s %-10s %-12s %-8s %-12s\n",
			"ID", "NAME", "SCOPES", "REPOS", "RATE", "LAST USED")
		fmt.Printf("  %-26s %-16s %-10s %-12s %-8s %-12s\n",
			strings.Repeat("-", 26), strings.Repeat("-", 16), strings.Repeat("-", 10),
			strings.Repeat("-", 12), strings.Repeat("-", 8), strings.Repeat("-", 12))

		for _, key := range keys {
			name := key.Name
			if len(name) > 16 {
				name = name[:13] + "..."
			}

			repos := "*"
			if len(key.RepoPatterns) > 0 {
				repos = strings.Join(key.RepoPatterns, ",")
				if len(repos) > 12 {
					repos = repos[:9] + "..."
				}
			}

			rate := "-"
			if key.RateLimit != nil {
				rate = fmt.Sprintf("%d/m", *key.RateLimit)
			}

			lastUsed := "never"
			if key.LastUsedAt != nil {
				lastUsed = formatTimeAgo(*key.LastUsedAt)
			}

			status := ""
			if key.Revoked {
				status = " [REVOKED]"
			} else if key.IsExpired() {
				status = " [EXPIRED]"
			}

			fmt.Printf("  %-26s %-16s %-10s %-12s %-8s %-12s%s\n",
				key.ID, name, formatScopes(key.Scopes), repos, rate, lastUsed, status)
		}
	}
}

func runTokenRevoke(cmd *cobra.Command, args []string) {
	logger := newLogger(tokenFormat)
	manager := mustGetAuthManager(logger)

	keyID := args[0]

	if err := manager.RevokeKey(keyID); err != nil {
		fmt.Fprintf(os.Stderr, "Error revoking token: %v\n", err)
		os.Exit(1)
	}

	if tokenFormat == "json" {
		printJSON(map[string]interface{}{
			"revoked": keyID,
			"success": true,
		})
	} else {
		fmt.Printf("Token %s revoked successfully.\n", keyID)
	}
}

func runTokenRotate(cmd *cobra.Command, args []string) {
	logger := newLogger(tokenFormat)
	manager := mustGetAuthManager(logger)

	keyID := args[0]

	key, rawToken, err := manager.RotateKey(keyID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error rotating token: %v\n", err)
		os.Exit(1)
	}

	if tokenFormat == "json" {
		printJSON(map[string]interface{}{
			"key_id":     key.ID,
			"name":       key.Name,
			"new_token":  rawToken,
			"rotated_at": time.Now().Format(time.RFC3339),
		})
	} else {
		fmt.Println("Token Rotated:")
		fmt.Println()
		fmt.Printf("  ID:        %s\n", key.ID)
		fmt.Printf("  Name:      %s\n", key.Name)
		fmt.Printf("  New Token: %s\n", rawToken)
		fmt.Println()
		fmt.Println("  IMPORTANT: The old token is now invalid. Store the new token securely.")
	}
}

// mustGetAuthManager creates an auth manager with database connection
func mustGetAuthManager(logger *logging.Logger) *auth.Manager {
	dataDir := expandPath(tokenDataDir)

	// Ensure data directory exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Error creating data directory: %v\n", err)
		os.Exit(1)
	}

	// Open database
	dbPath := filepath.Join(dataDir, "auth.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening database: %v\n", err)
		os.Exit(1)
	}

	// Create manager with database
	config := auth.DefaultManagerConfig()
	config.Enabled = true // Enable for CLI operations

	manager, err := auth.NewManager(config, db, logger)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating auth manager: %v\n", err)
		os.Exit(1)
	}

	return manager
}

// expandPath expands ~ to home directory
func expandPath(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// parseExpiration parses an expiration string like "30d", "1h", or "2024-12-31"
func parseExpiration(s string) (time.Time, error) {
	// Try duration format first (e.g., "30d", "1h")
	if len(s) > 1 {
		unit := s[len(s)-1]
		valueStr := s[:len(s)-1]
		var value int
		if _, err := fmt.Sscanf(valueStr, "%d", &value); err == nil {
			var d time.Duration
			switch unit {
			case 'd':
				d = time.Duration(value) * 24 * time.Hour
			case 'h':
				d = time.Duration(value) * time.Hour
			case 'm':
				d = time.Duration(value) * time.Minute
			default:
				// Fall through to date parsing
			}
			if d > 0 {
				return time.Now().Add(d), nil
			}
		}
	}

	// Try date formats
	formats := []string{
		"2006-01-02",
		"2006-01-02T15:04:05",
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("unrecognized format (use e.g., '30d', '1h', or '2024-12-31')")
}

// formatScopes formats scopes for display
func formatScopes(scopes []auth.Scope) string {
	var strs []string
	for _, s := range scopes {
		strs = append(strs, string(s))
	}
	return strings.Join(strs, ",")
}

// formatTimeAgo formats a time as "Xm ago", "Xh ago", etc.
func formatTimeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	default:
		return t.Format("Jan 2")
	}
}

// printJSON outputs data as formatted JSON
func printJSON(data interface{}) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(data)
}
