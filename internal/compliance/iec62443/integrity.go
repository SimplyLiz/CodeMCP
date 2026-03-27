package iec62443

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SimplyLiz/CodeMCP/internal/compliance"
)

// --- unvalidated-input: CR 3.5 — input validation for network/protocol input ---

type unvalidatedInputCheck struct{}

func (c *unvalidatedInputCheck) ID() string       { return "unvalidated-input" }
func (c *unvalidatedInputCheck) Name() string     { return "Unvalidated Network Input" }
func (c *unvalidatedInputCheck) Article() string  { return "CR 3.5 IEC 62443-4-2" }
func (c *unvalidatedInputCheck) Severity() string { return "error" }

// Binary protocol parsing patterns
var binaryInputPatterns = []*regexp.Regexp{
	regexp.MustCompile(`\.(Read|ReadBytes|ReadUint|ReadFull|ReadByte|ReadAt)\s*\(`),
	regexp.MustCompile(`(?i)(binary\.Read|binary\.BigEndian|binary\.LittleEndian)`),
	regexp.MustCompile(`(?i)(recv|recvfrom|recvmsg)\s*\(`),
	regexp.MustCompile(`(?i)(ParsePacket|ParseFrame|ParseMessage|DecodeMessage|UnmarshalBinary)\s*\(`),
}

var boundsCheckPattern = regexp.MustCompile(`(?i)(len\s*\(|cap\s*\(|bounds|range\s+check|size\s*[<>]=?|length\s*[<>]=?|validate|sanitize|if\s+.*\s*<\s*|if\s+.*\s*>\s*)`)

func (c *unvalidatedInputCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, "test_") {
			continue
		}

		func() {
			f, err := os.Open(filepath.Join(scope.RepoRoot, file))
			if err != nil {
				return
			}
			defer f.Close()

			scanner := bufio.NewScanner(f)
			lineNum := 0
			hasBinaryInput := false
			binaryInputLine := 0
			hasBoundsCheck := false
			braceDepth := 0

			for scanner.Scan() {
				lineNum++
				line := scanner.Text()
				trimmed := strings.TrimSpace(line)

				if strings.HasPrefix(trimmed, "//") {
					continue
				}

				prevDepth := braceDepth
				braceDepth += strings.Count(line, "{") - strings.Count(line, "}")

				// Reset at function boundaries
				if braceDepth <= 0 && prevDepth > 0 {
					if hasBinaryInput && !hasBoundsCheck {
						findings = append(findings, compliance.Finding{
							CheckID:    "unvalidated-input",
							Framework:  compliance.FrameworkIEC62443,
							Severity:   "error",
							Article:    "CR 3.5 IEC 62443-4-2",
							File:       file,
							StartLine:  binaryInputLine,
							Message:    "Network/binary input parsing without bounds checking or validation",
							Suggestion: "Add bounds checking and input validation before processing network data",
							Confidence: 0.65,
							CWE:        "CWE-20",
						})
					}
					hasBinaryInput = false
					hasBoundsCheck = false
				}

				for _, pattern := range binaryInputPatterns {
					if pattern.MatchString(line) {
						hasBinaryInput = true
						binaryInputLine = lineNum
						break
					}
				}

				if boundsCheckPattern.MatchString(line) {
					hasBoundsCheck = true
				}
			}

			// Handle last function in file
			if hasBinaryInput && !hasBoundsCheck {
				findings = append(findings, compliance.Finding{
					CheckID:    "unvalidated-input",
					Framework:  compliance.FrameworkIEC62443,
					Severity:   "error",
					Article:    "CR 3.5 IEC 62443-4-2",
					File:       file,
					StartLine:  binaryInputLine,
					Message:    "Network/binary input parsing without bounds checking or validation",
					Suggestion: "Add bounds checking and input validation before processing network data",
					Confidence: 0.65,
					CWE:        "CWE-20",
				})
			}

		}()
	}

	return findings, nil
}

// --- missing-message-auth: CR 3.1 — message authentication for network communications ---

type missingMessageAuthCheck struct{}

func (c *missingMessageAuthCheck) ID() string       { return "missing-message-auth" }
func (c *missingMessageAuthCheck) Name() string     { return "Missing Message Authentication" }
func (c *missingMessageAuthCheck) Article() string  { return "CR 3.1 IEC 62443-4-2" }
func (c *missingMessageAuthCheck) Severity() string { return "warning" }

// Network communication patterns
var networkCommPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(net\.Dial|net\.Listen|tcp|udp|socket|conn\.Write|conn\.Read)`),
	regexp.MustCompile(`(?i)(Send|SendTo|Transmit|Publish)\s*\(`),
	regexp.MustCompile(`(?i)(protocol|packet|frame|datagram|message_handler)`),
}

var messageAuthPatterns = regexp.MustCompile(`(?i)(hmac|HMAC|digital_signature|DigitalSign|mac\.|MAC\.|Verify|VerifySignature|crypto\.Sign|ed25519|ecdsa|rsa\.Sign|tls\.|TLS)`)

func (c *missingMessageAuthCheck) Run(ctx context.Context, scope *compliance.ScanScope) ([]compliance.Finding, error) {
	var findings []compliance.Finding

	for _, file := range scope.Files {
		if ctx.Err() != nil {
			return findings, ctx.Err()
		}

		if strings.Contains(file, "_test.") || strings.Contains(file, "test_") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(scope.RepoRoot, file))
		if err != nil {
			continue
		}

		contentStr := string(content)
		hasNetworkComm := false
		hasMessageAuth := false

		for _, pattern := range networkCommPatterns {
			if pattern.MatchString(contentStr) {
				hasNetworkComm = true
				break
			}
		}

		if hasNetworkComm {
			hasMessageAuth = messageAuthPatterns.MatchString(contentStr)

			if !hasMessageAuth {
				findings = append(findings, compliance.Finding{
					CheckID:    "missing-message-auth",
					Framework:  compliance.FrameworkIEC62443,
					Severity:   "warning",
					Article:    "CR 3.1 IEC 62443-4-2",
					File:       file,
					StartLine:  1,
					Message:    "Network communication code without message authentication/integrity verification",
					Suggestion: "Add HMAC, digital signatures, or TLS for message authentication on industrial communications",
					Confidence: 0.55,
				})
			}
		}
	}

	return findings, nil
}
