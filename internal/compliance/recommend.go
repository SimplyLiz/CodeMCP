package compliance

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// Recommendation describes a recommended framework with rationale.
type Recommendation struct {
	Framework  FrameworkID `json:"framework"`
	Name       string      `json:"name"`
	Reason     string      `json:"reason"`
	Confidence float64     `json:"confidence"` // 0.0-1.0
	Category   string      `json:"category"`   // "security", "privacy", "safety", "supply-chain"
}

// RecommendFrameworks analyzes the codebase and recommends applicable frameworks.
func RecommendFrameworks(repoRoot string) ([]Recommendation, error) {
	// Scan source files for indicators
	indicators := scanCodebaseIndicators(repoRoot)

	var recs []Recommendation

	// Universal security frameworks — always recommended
	recs = append(recs, Recommendation{
		Framework: FrameworkISO27001, Name: "ISO 27001:2022",
		Reason:     "Information security baseline — applicable to all software projects",
		Confidence: 0.95, Category: "security",
	})
	recs = append(recs, Recommendation{
		Framework: FrameworkOWASPASVS, Name: "OWASP ASVS 4.0",
		Reason:     "Application security verification — applicable to all codebases",
		Confidence: 0.90, Category: "security",
	})

	// Web application with HTTP handlers
	if indicators.hasHTTP {
		recs = append(recs, Recommendation{
			Framework: FrameworkNIST80053, Name: "NIST SP 800-53",
			Reason:     "HTTP handlers detected — security controls for networked applications",
			Confidence: 0.85, Category: "security",
		})
		recs = append(recs, Recommendation{
			Framework: FrameworkSOC2, Name: "SOC 2",
			Reason:     "Web service detected — trust service criteria for service organizations",
			Confidence: 0.75, Category: "security",
		})
	}

	// Personal data handling
	if indicators.hasPII {
		recs = append(recs, Recommendation{
			Framework: FrameworkGDPR, Name: "GDPR",
			Reason:     "Personal data fields detected (email, name, address, etc.)",
			Confidence: 0.85, Category: "privacy",
		})
		recs = append(recs, Recommendation{
			Framework: FrameworkCCPA, Name: "CCPA/CPRA",
			Reason:     "Personal data processing detected",
			Confidence: 0.80, Category: "privacy",
		})
		recs = append(recs, Recommendation{
			Framework: FrameworkISO27701, Name: "ISO 27701",
			Reason:     "PII handling detected — privacy extension to ISO 27001",
			Confidence: 0.75, Category: "privacy",
		})
	}

	// Database usage
	if indicators.hasDatabase {
		if !indicators.hasHTTP {
			recs = append(recs, Recommendation{
				Framework: FrameworkNIST80053, Name: "NIST SP 800-53",
				Reason:     "Database access detected — data protection controls needed",
				Confidence: 0.80, Category: "security",
			})
		}
	}

	// Payment processing
	if indicators.hasPayment {
		recs = append(recs, Recommendation{
			Framework: FrameworkPCIDSS, Name: "PCI DSS 4.0",
			Reason:     "Payment/financial processing patterns detected",
			Confidence: 0.90, Category: "security",
		})
	}

	// Healthcare
	if indicators.hasHealthcare {
		recs = append(recs, Recommendation{
			Framework: FrameworkHIPAA, Name: "HIPAA",
			Reason:     "Healthcare/PHI-related patterns detected",
			Confidence: 0.85, Category: "privacy",
		})
	}

	// Financial services (EU)
	if indicators.hasFinancial {
		recs = append(recs, Recommendation{
			Framework: FrameworkDORA, Name: "DORA",
			Reason:     "Financial service patterns detected — EU digital operational resilience",
			Confidence: 0.80, Category: "security",
		})
	}

	// AI/ML
	if indicators.hasAI {
		recs = append(recs, Recommendation{
			Framework: FrameworkEUAIAct, Name: "EU AI Act",
			Reason:     "AI/ML framework imports or model handling detected",
			Confidence: 0.85, Category: "security",
		})
	}

	// C/C++ safety-critical
	if indicators.isSafetyCriticalLang {
		recs = append(recs, Recommendation{
			Framework: FrameworkIEC61508, Name: "IEC 61508 / SIL",
			Reason:     "C/C++ codebase — functional safety standard applicable",
			Confidence: 0.70, Category: "safety",
		})
		recs = append(recs, Recommendation{
			Framework: FrameworkMISRA, Name: "MISRA C/C++",
			Reason:     "C/C++ codebase — safety-critical coding standard",
			Confidence: 0.75, Category: "safety",
		})
	}

	// Supply chain — check for dependency manifests
	if indicators.hasDependencies {
		recs = append(recs, Recommendation{
			Framework: FrameworkSBOM, Name: "SBOM/SLSA",
			Reason:     "Third-party dependencies detected — supply chain security",
			Confidence: 0.70, Category: "supply-chain",
		})
	}

	// Critical infrastructure
	if indicators.hasInfra {
		recs = append(recs, Recommendation{
			Framework: FrameworkNIS2, Name: "NIS2 Directive",
			Reason:     "Infrastructure/network service patterns detected",
			Confidence: 0.75, Category: "security",
		})
	}

	// Deduplicate (in case multiple signals recommend the same framework)
	seen := make(map[FrameworkID]bool)
	var deduped []Recommendation
	for _, r := range recs {
		if !seen[r.Framework] {
			seen[r.Framework] = true
			deduped = append(deduped, r)
		}
	}

	return deduped, nil
}

// codebaseIndicators holds detected characteristics of the codebase.
type codebaseIndicators struct {
	hasHTTP              bool
	hasPII               bool
	hasDatabase          bool
	hasPayment           bool
	hasHealthcare        bool
	hasFinancial         bool
	hasAI                bool
	isSafetyCriticalLang bool
	hasDependencies      bool
	hasInfra             bool
}

// scanCodebaseIndicators does a quick scan of source files for framework-relevant patterns.
func scanCodebaseIndicators(repoRoot string) codebaseIndicators {
	var ind codebaseIndicators

	// Check for dependency manifests
	depFiles := []string{"go.mod", "package.json", "Cargo.toml", "requirements.txt", "pyproject.toml", "pom.xml", "build.gradle", "Gemfile", "composer.json"}
	for _, df := range depFiles {
		if _, err := os.Stat(filepath.Join(repoRoot, df)); err == nil {
			ind.hasDependencies = true
			break
		}
	}

	// Check for C/C++ project indicators
	cppFiles := []string{"CMakeLists.txt", "compile_commands.json", "Makefile"}
	for _, cf := range cppFiles {
		if _, err := os.Stat(filepath.Join(repoRoot, cf)); err == nil {
			ind.isSafetyCriticalLang = true
			break
		}
	}

	// Quick-scan source files for import/pattern indicators (sample up to 100 files)
	scanned := 0
	_ = filepath.Walk(repoRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			if info != nil && info.IsDir() {
				base := info.Name()
				if base == ".git" || base == "node_modules" || base == "vendor" || base == ".ckb" || base == "dist" || base == "build" {
					return filepath.SkipDir
				}
			}
			return nil
		}
		if scanned >= 100 {
			return filepath.SkipAll
		}

		ext := filepath.Ext(path)
		if ext != ".go" && ext != ".ts" && ext != ".js" && ext != ".py" && ext != ".java" &&
			ext != ".rs" && ext != ".c" && ext != ".cpp" && ext != ".h" {
			return nil
		}
		// Skip test files and compliance check definitions (contain patterns that trigger false positives)
		rel, _ := filepath.Rel(repoRoot, path)
		base := filepath.Base(path)
		if strings.Contains(base, "_test.") || strings.Contains(base, ".test.") || strings.Contains(base, ".spec.") {
			return nil
		}
		if strings.Contains(rel, "compliance/") || strings.Contains(rel, "testdata/") || strings.Contains(rel, "fixtures/") {
			return nil
		}

		scanned++
		scanFileForIndicators(path, &ind)
		return nil
	})

	return ind
}

func scanFileForIndicators(path string, ind *codebaseIndicators) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	linesRead := 0
	for scanner.Scan() {
		linesRead++
		if linesRead > 200 { // Only scan first 200 lines (imports section)
			break
		}
		line := scanner.Text()

		// HTTP indicators
		if strings.Contains(line, "net/http") || strings.Contains(line, "gin-gonic") ||
			strings.Contains(line, "express") || strings.Contains(line, "fastapi") ||
			strings.Contains(line, "flask") || strings.Contains(line, "django") ||
			strings.Contains(line, "http.Handle") || strings.Contains(line, "http.ListenAndServe") ||
			strings.Contains(line, "fiber") || strings.Contains(line, "echo") {
			ind.hasHTTP = true
		}

		// PII indicators
		if strings.Contains(line, "email") || strings.Contains(line, "Email") ||
			strings.Contains(line, "firstName") || strings.Contains(line, "lastName") ||
			strings.Contains(line, "phone_number") || strings.Contains(line, "PhoneNumber") ||
			strings.Contains(line, "address") && strings.Contains(line, "struct") ||
			strings.Contains(line, "ssn") || strings.Contains(line, "SSN") ||
			strings.Contains(line, "date_of_birth") || strings.Contains(line, "DateOfBirth") {
			ind.hasPII = true
		}

		// Database indicators
		if strings.Contains(line, "database/sql") || strings.Contains(line, "gorm") ||
			strings.Contains(line, "sqlx") || strings.Contains(line, "mongodb") ||
			strings.Contains(line, "mongoose") || strings.Contains(line, "sequelize") ||
			strings.Contains(line, "prisma") || strings.Contains(line, "sqlalchemy") ||
			strings.Contains(line, "redis") || strings.Contains(line, "pg.") {
			ind.hasDatabase = true
		}

		// Payment indicators — require SDK/library imports, not just keyword mentions
		if strings.Contains(line, "\"stripe\"") || strings.Contains(line, "stripe.com") ||
			strings.Contains(line, "\"paypal\"") || strings.Contains(line, "braintree") ||
			strings.Contains(line, "credit_card") || strings.Contains(line, "card_number") ||
			strings.Contains(line, "adyen") {
			ind.hasPayment = true
		}

		// Healthcare indicators — require imports or struct fields, not prose
		if strings.Contains(line, "\"HL7\"") || strings.Contains(line, "hl7.") ||
			strings.Contains(line, "\"FHIR\"") || strings.Contains(line, "fhir.") ||
			strings.Contains(line, "medical_record") || strings.Contains(line, "MedicalRecord") ||
			strings.Contains(line, "PatientRecord") || strings.Contains(line, "PHI_") {
			ind.hasHealthcare = true
		}

		// Financial indicators
		if strings.Contains(line, "transaction") && strings.Contains(line, "amount") ||
			strings.Contains(line, "banking") || strings.Contains(line, "ledger") ||
			strings.Contains(line, "iban") || strings.Contains(line, "IBAN") {
			ind.hasFinancial = true
		}

		// AI/ML indicators
		if strings.Contains(line, "tensorflow") || strings.Contains(line, "pytorch") ||
			strings.Contains(line, "sklearn") || strings.Contains(line, "openai") ||
			strings.Contains(line, "anthropic") || strings.Contains(line, "huggingface") ||
			strings.Contains(line, "model.predict") || strings.Contains(line, "model.train") {
			ind.hasAI = true
		}

		// Infrastructure indicators
		if strings.Contains(line, "dns") || strings.Contains(line, "tls.Config") ||
			strings.Contains(line, "net.Listen") || strings.Contains(line, "grpc") ||
			strings.Contains(line, "kubernetes") || strings.Contains(line, "docker") {
			ind.hasInfra = true
		}
	}
}
