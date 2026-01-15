package secrets

import "regexp"

// Pattern defines a secret detection pattern.
type Pattern struct {
	Name        string
	Type        SecretType
	Severity    Severity
	Regex       *regexp.Regexp
	MinEntropy  float64  // Minimum entropy (0 = disabled)
	Description string
	Examples    []string // For testing
}

// BuiltinPatterns contains all builtin secret detection patterns.
// These are based on well-known secret formats from various providers.
var BuiltinPatterns = []Pattern{
	// ============ AWS ============
	{
		Name:        "aws_access_key_id",
		Type:        SecretTypeAWSAccessKey,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`(?:^|[^A-Z0-9])((?:A3T[A-Z0-9]|AKIA|ABIA|ACCA|AGPA|AIDA|AIPA|AKIA|ANPA|ANVA|APKA|AROA|ASCA|ASIA)[A-Z0-9]{16})(?:[^A-Z0-9]|$)`),
		Description: "AWS Access Key ID",
		Examples:    []string{"AKIAIOSFODNN7EXAMPLE"},
	},
	{
		Name:        "aws_secret_key",
		Type:        SecretTypeAWSSecretKey,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`(?i)(?:aws[_-]?)?secret[_-]?(?:access[_-]?)?key['":\s=]+['"]?([A-Za-z0-9/+=]{40})['"]?`),
		MinEntropy:  3.5,
		Description: "AWS Secret Access Key",
	},

	// ============ GitHub ============
	{
		Name:        "github_pat",
		Type:        SecretTypeGitHubPAT,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`ghp_[A-Za-z0-9]{36,}`),
		Description: "GitHub Personal Access Token",
		Examples:    []string{"ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"},
	},
	{
		Name:        "github_oauth",
		Type:        SecretTypeGitHubOAuth,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`gho_[A-Za-z0-9]{36,}`),
		Description: "GitHub OAuth Access Token",
	},
	{
		Name:        "github_app",
		Type:        SecretTypeGitHubApp,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`(?:ghu|ghs)_[A-Za-z0-9]{36,}`),
		Description: "GitHub App Token",
	},
	{
		Name:        "github_refresh",
		Type:        SecretTypeGitHubRefresh,
		Severity:    SeverityHigh,
		Regex:       regexp.MustCompile(`ghr_[A-Za-z0-9]{36,}`),
		Description: "GitHub Refresh Token",
	},
	{
		Name:        "github_fine_grained",
		Type:        SecretTypeGitHubPAT,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59}`),
		Description: "GitHub Fine-Grained Personal Access Token",
	},

	// ============ Stripe ============
	{
		Name:        "stripe_live_secret",
		Type:        SecretTypeStripeLiveKey,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`sk_live_[A-Za-z0-9]{24,}`),
		Description: "Stripe Live Secret Key",
	},
	{
		Name:        "stripe_live_restricted",
		Type:        SecretTypeStripeLiveKey,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`rk_live_[A-Za-z0-9]{24,}`),
		Description: "Stripe Live Restricted Key",
	},
	{
		Name:        "stripe_test_secret",
		Type:        SecretTypeStripeTestKey,
		Severity:    SeverityLow,
		Regex:       regexp.MustCompile(`sk_test_[A-Za-z0-9]{24,}`),
		Description: "Stripe Test Secret Key",
	},

	// ============ Slack ============
	{
		Name:        "slack_bot_token",
		Type:        SecretTypeSlackBotToken,
		Severity:    SeverityHigh,
		Regex:       regexp.MustCompile(`xoxb-[0-9]{10,13}-[0-9]{10,13}-[A-Za-z0-9]{24}`),
		Description: "Slack Bot Token",
	},
	{
		Name:        "slack_user_token",
		Type:        SecretTypeSlackUserToken,
		Severity:    SeverityHigh,
		Regex:       regexp.MustCompile(`xoxp-[0-9]{10,13}-[0-9]{10,13}-[0-9]{10,13}-[A-Za-z0-9]{32}`),
		Description: "Slack User Token",
	},
	{
		Name:        "slack_webhook",
		Type:        SecretTypeSlackWebhook,
		Severity:    SeverityMedium,
		Regex:       regexp.MustCompile(`https://hooks\.slack\.com/services/T[A-Z0-9]{8,}/B[A-Z0-9]{8,}/[A-Za-z0-9]{24}`),
		Description: "Slack Webhook URL",
	},
	{
		Name:        "slack_app_token",
		Type:        SecretTypeSlackBotToken,
		Severity:    SeverityHigh,
		Regex:       regexp.MustCompile(`xapp-[0-9]+-[A-Z0-9]+-[0-9]+-[A-Za-z0-9]+`),
		Description: "Slack App Token",
	},

	// ============ Private Keys ============
	{
		Name:        "private_key_rsa",
		Type:        SecretTypePrivateKey,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`-----BEGIN (?:RSA )?PRIVATE KEY-----`),
		Description: "RSA Private Key",
	},
	{
		Name:        "private_key_ec",
		Type:        SecretTypePrivateKey,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`-----BEGIN EC PRIVATE KEY-----`),
		Description: "EC Private Key",
	},
	{
		Name:        "private_key_openssh",
		Type:        SecretTypePrivateKey,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`-----BEGIN OPENSSH PRIVATE KEY-----`),
		Description: "OpenSSH Private Key",
	},
	{
		Name:        "private_key_dsa",
		Type:        SecretTypePrivateKey,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`-----BEGIN DSA PRIVATE KEY-----`),
		Description: "DSA Private Key",
	},
	{
		Name:        "private_key_pgp",
		Type:        SecretTypePrivateKey,
		Severity:    SeverityCritical,
		Regex:       regexp.MustCompile(`-----BEGIN PGP PRIVATE KEY BLOCK-----`),
		Description: "PGP Private Key",
	},

	// ============ JWT ============
	{
		Name:        "jwt_token",
		Type:        SecretTypeJWT,
		Severity:    SeverityMedium,
		Regex:       regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*`),
		MinEntropy:  3.0,
		Description: "JSON Web Token",
	},

	// ============ Google ============
	{
		Name:        "google_api_key",
		Type:        SecretTypeGoogleAPIKey,
		Severity:    SeverityHigh,
		Regex:       regexp.MustCompile(`AIza[A-Za-z0-9_-]{35}`),
		Description: "Google API Key",
	},

	// ============ NPM ============
	{
		Name:        "npm_token",
		Type:        SecretTypeNPMToken,
		Severity:    SeverityHigh,
		Regex:       regexp.MustCompile(`npm_[A-Za-z0-9]{36}`),
		Description: "NPM Access Token",
	},

	// ============ PyPI ============
	{
		Name:        "pypi_token",
		Type:        SecretTypePyPIToken,
		Severity:    SeverityHigh,
		Regex:       regexp.MustCompile(`pypi-[A-Za-z0-9_-]{100,}`),
		Description: "PyPI API Token",
	},

	// ============ Generic Patterns (require entropy check) ============
	{
		Name:        "generic_api_key",
		Type:        SecretTypeGenericAPIKey,
		Severity:    SeverityMedium,
		Regex:       regexp.MustCompile(`(?i)(?:api[_-]?key|apikey)['":\s=]+['"]?([A-Za-z0-9_-]{20,64})['"]?`),
		MinEntropy:  3.5,
		Description: "Generic API Key",
	},
	{
		Name:        "generic_secret",
		Type:        SecretTypeGenericSecret,
		Severity:    SeverityMedium,
		Regex:       regexp.MustCompile(`(?i)(?:secret|password|passwd|pwd|token)['":\s=]+['"]?([A-Za-z0-9!@#$%^&*()_+\-=]{8,64})['"]?`),
		MinEntropy:  3.0,
		Description: "Generic Secret or Password",
	},
	{
		Name:        "password_in_url",
		Type:        SecretTypePasswordInURL,
		Severity:    SeverityHigh,
		Regex:       regexp.MustCompile(`://[^:]+:([^@]{3,})@[^/]+`),
		MinEntropy:  2.5,
		Description: "Password in URL",
	},
	{
		Name:        "basic_auth_header",
		Type:        SecretTypeGenericSecret,
		Severity:    SeverityHigh,
		Regex:       regexp.MustCompile(`(?i)authorization['":\s=]+['"]?basic\s+([A-Za-z0-9+/=]{20,})['"]?`),
		Description: "Basic Auth Header",
	},
	{
		Name:        "bearer_token",
		Type:        SecretTypeGenericSecret,
		Severity:    SeverityMedium,
		Regex:       regexp.MustCompile(`(?i)authorization['":\s=]+['"]?bearer\s+([A-Za-z0-9._-]{20,})['"]?`),
		MinEntropy:  3.0,
		Description: "Bearer Token",
	},
}

// GetPatternByName returns a pattern by name.
func GetPatternByName(name string) *Pattern {
	for i := range BuiltinPatterns {
		if BuiltinPatterns[i].Name == name {
			return &BuiltinPatterns[i]
		}
	}
	return nil
}

// GetPatternsByType returns all patterns for a given secret type.
func GetPatternsByType(t SecretType) []Pattern {
	var result []Pattern
	for _, p := range BuiltinPatterns {
		if p.Type == t {
			result = append(result, p)
		}
	}
	return result
}

// GetPatternsBySeverity returns all patterns at or above a given severity.
func GetPatternsBySeverity(minSeverity Severity) []Pattern {
	minWeight := minSeverity.Weight()
	var result []Pattern
	for _, p := range BuiltinPatterns {
		if p.Severity.Weight() >= minWeight {
			result = append(result, p)
		}
	}
	return result
}
