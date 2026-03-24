package compliance

// ComplianceConfig configures compliance audit behavior.
// Stored in .ckb/config.json under the "compliance" key.
type ComplianceConfig struct {
	// Additional PII field patterns beyond defaults (merged, not replaced)
	PIIFieldPatterns []string `json:"piiFieldPatterns,omitempty" mapstructure:"piiFieldPatterns"`

	// Glob patterns identifying AI/ML component paths (for EU AI Act)
	AIComponentPaths []string `json:"aiComponentPaths,omitempty" mapstructure:"aiComponentPaths"`

	// SIL level for IEC 61508 (1-4, determines thresholds)
	SILLevel int `json:"silLevel,omitempty" mapstructure:"silLevel"`

	// Glob patterns for GDPR Art. 9 special category data paths
	SpecialCategoryPaths []string `json:"specialCategoryPaths,omitempty" mapstructure:"specialCategoryPaths"`

	// Frameworks to enable by default when --framework is omitted
	DefaultFrameworks []string `json:"defaultFrameworks,omitempty" mapstructure:"defaultFrameworks"`
}

// DefaultPIIPatterns returns the built-in PII field name patterns.
// These cover direct identifiers, quasi-identifiers, and sensitive categories.
// Includes German equivalents for DSGVO compliance.
func DefaultPIIPatterns() []PIIPattern {
	return []PIIPattern{
		// Direct identifiers — "name" alone is too broad, require prefix/context
		{Pattern: "person_name", Category: "direct-identifier", PIIType: "name"},
		{Pattern: "real_name", Category: "direct-identifier", PIIType: "name"},
		{Pattern: "legal_name", Category: "direct-identifier", PIIType: "name"},
		{Pattern: "first_name", Category: "direct-identifier", PIIType: "name"},
		{Pattern: "last_name", Category: "direct-identifier", PIIType: "name"},
		{Pattern: "full_name", Category: "direct-identifier", PIIType: "name"},
		{Pattern: "vorname", Category: "direct-identifier", PIIType: "name"},
		{Pattern: "nachname", Category: "direct-identifier", PIIType: "name"},
		{Pattern: "username", Category: "direct-identifier", PIIType: "name"},
		{Pattern: "user_name", Category: "direct-identifier", PIIType: "name"},
		{Pattern: "display_name", Category: "direct-identifier", PIIType: "name"},

		// Contact information
		{Pattern: "email", Category: "direct-identifier", PIIType: "contact"},
		{Pattern: "e_mail", Category: "direct-identifier", PIIType: "contact"},
		{Pattern: "email_address", Category: "direct-identifier", PIIType: "contact"},
		{Pattern: "phone", Category: "direct-identifier", PIIType: "contact"},
		{Pattern: "phone_number", Category: "direct-identifier", PIIType: "contact"},
		{Pattern: "telephone", Category: "direct-identifier", PIIType: "contact"},
		{Pattern: "mobile", Category: "direct-identifier", PIIType: "contact"},
		{Pattern: "telefon", Category: "direct-identifier", PIIType: "contact"},

		// Address
		{Pattern: "address", Category: "direct-identifier", PIIType: "address"},
		{Pattern: "street", Category: "direct-identifier", PIIType: "address"},
		{Pattern: "city", Category: "quasi-identifier", PIIType: "address"},
		{Pattern: "zip_code", Category: "quasi-identifier", PIIType: "address"},
		{Pattern: "postal_code", Category: "quasi-identifier", PIIType: "address"},
		{Pattern: "anschrift", Category: "direct-identifier", PIIType: "address"},
		{Pattern: "strasse", Category: "direct-identifier", PIIType: "address"},
		{Pattern: "plz", Category: "quasi-identifier", PIIType: "address"},

		// Government IDs
		{Pattern: "ssn", Category: "direct-identifier", PIIType: "government-id"},
		{Pattern: "social_security", Category: "direct-identifier", PIIType: "government-id"},
		{Pattern: "sozialversicherung", Category: "direct-identifier", PIIType: "government-id"},
		{Pattern: "passport", Category: "direct-identifier", PIIType: "government-id"},
		{Pattern: "id_card", Category: "direct-identifier", PIIType: "government-id"},
		{Pattern: "personalausweis", Category: "direct-identifier", PIIType: "government-id"},
		{Pattern: "national_id", Category: "direct-identifier", PIIType: "government-id"},
		{Pattern: "tax_id", Category: "direct-identifier", PIIType: "government-id"},
		{Pattern: "steuer_id", Category: "direct-identifier", PIIType: "government-id"},

		// Date of birth
		{Pattern: "date_of_birth", Category: "direct-identifier", PIIType: "dob"},
		{Pattern: "dob", Category: "direct-identifier", PIIType: "dob"},
		{Pattern: "birthday", Category: "direct-identifier", PIIType: "dob"},
		{Pattern: "birth_date", Category: "direct-identifier", PIIType: "dob"},
		{Pattern: "geburtsdatum", Category: "direct-identifier", PIIType: "dob"},

		// Network identifiers
		{Pattern: "ip_address", Category: "quasi-identifier", PIIType: "network"},
		{Pattern: "ip_addr", Category: "quasi-identifier", PIIType: "network"},
		{Pattern: "user_agent", Category: "quasi-identifier", PIIType: "network"},
		{Pattern: "mac_address", Category: "quasi-identifier", PIIType: "network"},
		{Pattern: "device_id", Category: "quasi-identifier", PIIType: "network"},

		// Financial
		{Pattern: "iban", Category: "direct-identifier", PIIType: "financial"},
		{Pattern: "bank_account", Category: "direct-identifier", PIIType: "financial"},
		{Pattern: "kontonummer", Category: "direct-identifier", PIIType: "financial"},
		{Pattern: "credit_card", Category: "direct-identifier", PIIType: "financial"},
		{Pattern: "card_number", Category: "direct-identifier", PIIType: "financial"},
		{Pattern: "kartennummer", Category: "direct-identifier", PIIType: "financial"},
		{Pattern: "account_number", Category: "direct-identifier", PIIType: "financial"},
		{Pattern: "routing_number", Category: "direct-identifier", PIIType: "financial"},

		// Special categories (GDPR Art. 9)
		{Pattern: "gender", Category: "sensitive", PIIType: "demographics"},
		{Pattern: "geschlecht", Category: "sensitive", PIIType: "demographics"},
		{Pattern: "nationality", Category: "sensitive", PIIType: "demographics"},
		{Pattern: "staatsangehoerigkeit", Category: "sensitive", PIIType: "demographics"},
		{Pattern: "ethnicity", Category: "sensitive", PIIType: "demographics"},
		{Pattern: "race", Category: "sensitive", PIIType: "demographics"},
		{Pattern: "religion", Category: "sensitive", PIIType: "demographics"},
		{Pattern: "sexual_orientation", Category: "sensitive", PIIType: "demographics"},
		{Pattern: "health_data", Category: "sensitive", PIIType: "health"},
		{Pattern: "gesundheitsdaten", Category: "sensitive", PIIType: "health"},
		{Pattern: "medical_record", Category: "sensitive", PIIType: "health"},
		{Pattern: "diagnosis", Category: "sensitive", PIIType: "health"},
		{Pattern: "biometric", Category: "sensitive", PIIType: "biometric"},
		{Pattern: "fingerprint", Category: "sensitive", PIIType: "biometric"},
		{Pattern: "face_id", Category: "sensitive", PIIType: "biometric"},

		// Credentials (overlap with security, but also PII)
		{Pattern: "password", Category: "direct-identifier", PIIType: "credential"},
		{Pattern: "passwort", Category: "direct-identifier", PIIType: "credential"},
	}
}

// PIIPattern describes a PII field name pattern with classification.
type PIIPattern struct {
	Pattern  string // Normalized pattern (snake_case)
	Category string // "direct-identifier", "quasi-identifier", "sensitive"
	PIIType  string // "name", "contact", "address", "government-id", "dob", "financial", etc.
}

// WeakCryptoPatterns returns patterns for deprecated/insecure cryptographic functions.
var WeakCryptoPatterns = []string{
	"md5", "sha1",
	"des", "3des", "triple_des",
	"rc4", "rc2",
	"blowfish",
	"ecb",
}

// LogFunctionPatterns returns patterns that indicate logging calls.
var LogFunctionPatterns = []string{
	// Go
	"log.", "slog.", "logger.",
	"fmt.Print", "fmt.Fprint", "fmt.Sprint",
	// JavaScript/TypeScript
	"console.log", "console.error", "console.warn", "console.info", "console.debug",
	// Python
	"logging.", "logger.", "print(",
	// Java
	"LOG.", "log.", "logger.", "LOGGER.",
	// Generic
	"log(", "warn(", "error(", "debug(", "info(",
}
