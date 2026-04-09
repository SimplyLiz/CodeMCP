package compliance

import (
	"fmt"
	"strings"
	"testing"
)

// =============================================================================
// Compliance Scanner Benchmarks
// =============================================================================
// These cover the innermost hot paths of the audit pipeline:
//
//   normalizeIdentifier   — called per identifier per line across all files
//   extractIdentifiers    — called per line (regex + dedup)
//   extractContainer      — called per line (7 compiled regexes)
//   matchPII              — called per identifier against ~80 patterns
//   isNonPIIIdentifier    — called on every matchPII hit
//
// Baselines (Apple M4 Pro, arm64, -count=6):
//   normalizeIdentifier:        ~138 ns/op,  138 B/op,  4 allocs/op
//   normalizeIdentifier_Long:   ~650 ns/op, 1352 B/op,  9 allocs/op  ← allocs on long idents
//   extractIdentifiers:         ~555 ns/op,  219 B/op,  6 allocs/op
//   extractContainer:           ~502 ns/op,   24 B/op,  0 allocs/op  (allocation is submatches slice)
//   isNonPIIIdentifier:         ~197 ns/op,    0 B/op,  0 allocs/op
//   matchPII (hit):             ~706 ns/op,    0 B/op,  0 allocs/op
//   matchPII (miss/full scan):  ~1.13 µs/op,   0 B/op,  0 allocs/op
//   ScannerPipeline/100lines:   ~283 µs/op, 37 KB/op, 1190 allocs/op ← ~1190 allocs for 100 lines
//   ScannerPipeline/500lines:   ~1.42 ms/op
//   ScannerPipeline/1000lines:  ~2.85 ms/op
//   NewPIIScanner:              ~2.4 µs/op,  13 KB/op,  6 allocs/op
//   NewPIIScannerWithExtras:    ~4.2 µs/op,  21 KB/op, 32 allocs/op
//
// Notable: ScannerPipeline allocates ~12 allocs/line. Most come from
// extractIdentifiers (map + slice per line). Worth profiling if audit
// latency becomes a bottleneck on large repos.
//
// Use benchstat for before/after comparison:
//   go test -bench=. -benchmem -count=6 -run=^$ ./internal/compliance > before.txt
//   # make changes
//   go test -bench=. -benchmem -count=6 -run=^$ ./internal/compliance > after.txt
//   benchstat before.txt after.txt
//
// To update the stored baseline:
//   go test -bench=. -benchmem -count=6 -run=^$ ./internal/compliance > testdata/benchmarks/compliance_baseline.txt
// =============================================================================

// BenchmarkNormalizeIdentifier measures identifier normalization (camelCase → snake_case).
// This is the hottest inner loop function — called on every identifier in every line.
func BenchmarkNormalizeIdentifier(b *testing.B) {
	cases := []string{
		"firstName",        // camelCase
		"UserEmailAddress", // PascalCase
		"http_client",      // already snake_case
		"HTMLParser",       // acronym boundary
		"getUser",          // short camelCase
		"SCREAMING_SNAKE",  // all-caps
		"userID",           // mixed acronym
		"date_of_birth",    // long snake_case
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		normalizeIdentifier(cases[i%len(cases)])
	}
}

// BenchmarkNormalizeIdentifier_Long measures the cost on long identifiers.
func BenchmarkNormalizeIdentifier_Long(b *testing.B) {
	long := "VeryLongCamelCaseIdentifierWithManyBoundariesForTestingPurposes"
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		normalizeIdentifier(long)
	}
}

// BenchmarkExtractIdentifiers measures per-line identifier extraction (regex + dedup).
func BenchmarkExtractIdentifiers(b *testing.B) {
	lines := []string{
		"\tfirstName string `json:\"first_name\"`",                   // Go struct field
		"\tprivate String userEmailAddress;",                         // Java field
		"\temail_address: Optional[str] = None",                      // Python
		"export interface UserProfile { dateOfBirth: string }",       // TypeScript
		"func (u *User) GetFullName() string { return u.FullName }",  // Go method
		"// This is a comment with identifiers: email phone address", // comment (should skip)
		"",                          // empty line
		"const MAX_RETRY_COUNT = 3", // constant
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		extractIdentifiers(lines[i%len(lines)])
	}
}

// BenchmarkExtractContainer measures per-line container detection (7 compiled regexes).
func BenchmarkExtractContainer(b *testing.B) {
	lines := []string{
		`type UserProfile struct {`,                   // Go struct
		`class UserService extends BaseService {`,     // Java/Python/TS class
		`interface PaymentProvider {`,                 // TypeScript interface
		`export type UserRecord = {`,                  // TypeScript type alias
		`data class UserData(val name: String) {`,     // Kotlin data class
		`pub struct ConnectionPool {`,                 // Rust struct
		`func processPayment(amount float64) error {`, // non-container (no match)
		`	return nil`, // non-container (no match)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		extractContainer(lines[i%len(lines)])
	}
}

// BenchmarkIsNonPIIIdentifier measures the filter function for false-positive suppression.
func BenchmarkIsNonPIIIdentifier(b *testing.B) {
	identifiers := []string{
		"fingerprint",      // filtered: non-biometric
		"display_name",     // filtered: UI label
		"file_name",        // filtered: code entity
		"function_name",    // filtered: code entity
		"email",            // NOT filtered: real PII
		"first_name",       // NOT filtered: real PII
		"host_name",        // filtered: infra
		"user_fingerprint", // NOT filtered: biometric context
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		isNonPIIIdentifier(identifiers[i%len(identifiers)])
	}
}

// BenchmarkMatchPII measures PII pattern matching against the full pattern set (~80 patterns).
// This is called per-identifier per-line so allocation pressure matters.
func BenchmarkMatchPII(b *testing.B) {
	scanner := NewPIIScanner(nil)

	identifiers := []string{
		"email",          // exact match (hit)
		"first_name",     // exact match (hit)
		"user_email",     // suffix match (hit)
		"customer_phone", // suffix match (hit)
		"fingerprint",    // non-PII filter (filtered before lookup)
		"engine_name",    // non-PII filter (filtered before lookup)
		"query_result",   // no match (miss)
		"backend_ladder", // no match (miss)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scanner.matchPII(identifiers[i%len(identifiers)])
	}
}

// BenchmarkMatchPII_Miss measures the worst-case path: no match, full suffix scan.
func BenchmarkMatchPII_Miss(b *testing.B) {
	scanner := NewPIIScanner(nil)
	// A normalized identifier that won't match any pattern and doesn't get filtered early.
	// Forces a full suffix scan of all patterns.
	ident := "orchestrator_backend_result"

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		scanner.matchPII(ident)
	}
}

// syntheticLines builds a realistic mixed-content source file of n lines.
// The mix (~60% logic/control flow, ~20% field declarations, ~20% comments/blank)
// is tuned to match real Go/TS/Python distribution in a large service repo.
func syntheticLines(n int) []string {
	templates := []string{
		// Field declarations — trigger PII matches
		"\tfirstName string `json:\"first_name\"`",
		"\temailAddress string `json:\"email\"`",
		"\tphoneNumber string `json:\"phone\"`",
		"\tdateOfBirth time.Time `json:\"date_of_birth\"`",
		"\tuserID string `json:\"user_id\"`",
		"\tcreatedAt time.Time",
		"\tupdatedAt time.Time",
		"\tscore int",
		// Container declarations
		"type %s struct {",
		"func (u *%s) Validate() error {",
		"func (u *%s) GetDisplayName() string {",
		// Logic — dense identifiers but no PII hits
		"\tif u.score > 0 {",
		"\t\treturn fmt.Errorf(\"validation failed: %w\", err)",
		"\t}",
		"\treturn nil",
		"\tresult := make([]string, 0, len(items))",
		"\tfor i, item := range items {",
		"\t\tresult = append(result, item.String())",
		"\t}",
		"\tctx, cancel := context.WithTimeout(ctx, 30*time.Second)",
		"\tdefer cancel()",
		"\tif err := db.QueryContext(ctx, query, args...); err != nil {",
		"\t\treturn nil, fmt.Errorf(\"query failed: %w\", err)",
		"\t}",
		// Comments — skipped by extractIdentifiers
		"// GetDisplayName returns the user's display name for UI purposes",
		"// TODO: add rate limiting",
		"",
		// Constants / vars — few identifiers
		"const maxRetryCount = 3",
		"var defaultTimeout = 30 * time.Second",
		"\tlog.Printf(\"processing request for user %s\", u.userID)",
	}

	lines := make([]string, n)
	for i := range lines {
		tmpl := templates[i%len(templates)]
		if strings.Contains(tmpl, "%s") {
			lines[i] = fmt.Sprintf(tmpl, "Entity"+fmt.Sprint(i%20))
		} else {
			lines[i] = tmpl
		}
	}
	return lines
}

// BenchmarkScannerPipeline simulates the per-line processing loop inside scanFile.
// This is the composite hot path: extractContainer + extractIdentifiers + matchPII
// called for every line in every source file during a full audit.
//
// Sizes reflect real-world CKB usage:
//   - 500 lines:   typical service file
//   - 5k lines:    large generated file or fat service
//   - 50k lines:   worst-case single file (e.g. generated protobuf, large test fixture)
//
// For the full-repo picture see BenchmarkAuditFileSet.
func BenchmarkScannerPipeline(b *testing.B) {
	pii := NewPIIScanner(nil)

	sizes := []struct {
		name  string
		lines int
	}{
		{"500lines", 500},
		{"5klines", 5_000},
		{"50klines", 50_000},
	}

	for _, sz := range sizes {
		lines := syntheticLines(sz.lines)

		// Measure total bytes so b.SetBytes gives us a lines-throughput proxy.
		var totalBytes int64
		for _, l := range lines {
			totalBytes += int64(len(l))
		}

		b.Run(sz.name, func(b *testing.B) {
			b.SetBytes(totalBytes)
			b.ReportAllocs()
			b.ResetTimer()

			for iter := 0; iter < b.N; iter++ {
				currentContainer := ""
				for _, line := range lines {
					if container := extractContainer(line); container != "" {
						currentContainer = container
					}
					if strings.HasPrefix(strings.TrimSpace(line), "}") {
						currentContainer = ""
					}
					identifiers := extractIdentifiers(line)
					for _, ident := range identifiers {
						normalized := normalizeIdentifier(ident)
						pii.matchPII(normalized)
						_ = currentContainer
					}
				}
			}
		})
	}
}

// BenchmarkAuditFileSet simulates the full audit scanning N files of M lines each.
// This is the scenario where CKB struggles on huge repos — the alloc cascade
// from extractIdentifiers accumulates across every file in the set.
//
// File counts reflect real-world repo sizes:
//   - 100 files × 300 lines  ≈ small service (~30k lines)
//   - 1k files × 300 lines   ≈ mid-size repo (~300k lines)
//   - 5k files × 300 lines   ≈ large monorepo (~1.5M lines)
//
// Each sub-benchmark runs a single b.N iteration over the full file set,
// so ns/op = total wall time for one complete scan of that repo size.
func BenchmarkAuditFileSet(b *testing.B) {
	pii := NewPIIScanner(nil)
	fileLines := syntheticLines(300) // one representative file

	sets := []struct {
		name  string
		files int
	}{
		{"100files", 100},
		{"1kfiles", 1_000},
		{"5kfiles", 5_000},
	}

	for _, set := range sets {
		var totalBytes int64
		for _, l := range fileLines {
			totalBytes += int64(len(l))
		}
		totalBytes *= int64(set.files)

		b.Run(set.name, func(b *testing.B) {
			b.SetBytes(totalBytes)
			b.ReportAllocs()
			b.ResetTimer()

			for iter := 0; iter < b.N; iter++ {
				for f := 0; f < set.files; f++ {
					currentContainer := ""
					for _, line := range fileLines {
						if container := extractContainer(line); container != "" {
							currentContainer = container
						}
						if strings.HasPrefix(strings.TrimSpace(line), "}") {
							currentContainer = ""
						}
						identifiers := extractIdentifiers(line)
						for _, ident := range identifiers {
							normalized := normalizeIdentifier(ident)
							pii.matchPII(normalized)
							_ = currentContainer
						}
					}
				}
			}
		})
	}
}

// BenchmarkMatchPII_PatternScale measures how matchPII degrades as the pattern
// set grows. CKB users can add custom patterns via config; this shows the cost.
func BenchmarkMatchPII_PatternScale(b *testing.B) {
	// A miss forces a full suffix scan — worst case for pattern count scaling.
	ident := "orchestrator_backend_result"

	sizes := []struct {
		name    string
		nExtras int
	}{
		{"default_~80patterns", 0},
		{"100patterns", 20},
		{"200patterns", 120},
		{"500patterns", 420},
	}

	for _, sz := range sizes {
		extras := make([]string, sz.nExtras)
		for i := range extras {
			extras[i] = fmt.Sprintf("custom_field_%d", i)
		}
		scanner := NewPIIScanner(extras)

		b.Run(sz.name, func(b *testing.B) {
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				scanner.matchPII(ident)
			}
		})
	}
}

// BenchmarkNewPIIScanner measures scanner construction cost (pattern compilation).
// Relevant for callers that create per-request scanners.
func BenchmarkNewPIIScanner(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		NewPIIScanner(nil)
	}
}

// BenchmarkNewPIIScannerWithExtras measures construction cost with additional patterns.
func BenchmarkNewPIIScannerWithExtras(b *testing.B) {
	extra := []string{"patient_id", "member_number", "policy_holder", "claim_ref", "beneficiary"}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		NewPIIScanner(extra)
	}
}
