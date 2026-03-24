# Compliance Audit — Complete Check Reference

All 126 checks across 20 frameworks. Organized by framework with check ID, article/clause, detection description, severity, CWE (where applicable), and confidence range.

---

## GDPR/DSGVO — `gdpr` (11 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `pii-field-unencrypted` | Art. 25(1) | PII fields stored without encryption | error | CWE-311 | 0.7-0.9 |
| `pii-logged` | Art. 5(1)(f) | PII written to log output | error | CWE-532 | 0.7-0.95 |
| `pii-no-retention` | Art. 5(1)(e) | PII storage with no TTL or deletion path | warning | — | 0.5-0.7 |
| `consent-missing` | Art. 7 | Data processing without consent check | warning | — | 0.5-0.7 |
| `data-export-missing` | Art. 20 | No data portability endpoint | warning | — | 0.5-0.65 |
| `deletion-missing` | Art. 17 | No right-to-erasure implementation | warning | — | 0.5-0.65 |
| `cross-border-transfer` | Art. 46 | Data sent to external endpoints without safeguards | warning | — | 0.5-0.7 |
| `special-category-unprotected` | Art. 9 | Health/biometric/racial data without extra controls | error | CWE-311 | 0.6-0.85 |
| `hardcoded-secret` | Art. 32(1)(a) | Credentials in source code | error | CWE-798 | 0.85-1.0 |
| `weak-crypto` | Art. 32(1)(a) | Use of MD5, SHA1, DES, or RC4 | error | CWE-327 | 0.9-1.0 |
| `missing-audit-log` | Art. 30 | Data processing without audit trail | warning | CWE-778 | 0.5-0.7 |

---

## CCPA/CPRA — `ccpa` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `pii-sold-without-consent` | §1798.120 | Personal information shared without opt-out | error | — | 0.5-0.7 |
| `pii-no-deletion` | §1798.105 | No deletion mechanism for consumer data | warning | — | 0.5-0.65 |
| `pii-no-disclosure` | §1798.110 | No data disclosure endpoint | warning | — | 0.5-0.65 |
| `pii-field-unencrypted` | §1798.150 | Personal information without reasonable security | error | CWE-311 | 0.7-0.9 |
| `minor-data-unprotected` | §1798.120(c) | Minor's data without additional protections | error | — | 0.5-0.7 |

---

## ISO 27701 — `iso27701` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `pii-no-purpose-limitation` | §7.2.1 | PII processing without documented purpose | warning | — | 0.5-0.65 |
| `pii-no-consent-record` | §7.2.3 | PII collection without consent record | warning | — | 0.5-0.65 |
| `pii-no-minimization` | §7.4.4 | PII collection beyond stated purpose | warning | — | 0.5-0.65 |
| `pii-no-deidentification` | §7.4.5 | PII without de-identification capability | warning | — | 0.5-0.65 |
| `pii-no-processor-agreement` | §7.5.1 | PII shared with third party without DPA | warning | — | 0.5-0.6 |

---

## EU AI Act — `eu-ai-act` (8 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `ai-no-logging` | Art. 12 | AI component without decision logging | error | CWE-778 | 0.7-0.9 |
| `ai-no-human-oversight` | Art. 14 | High-risk AI without human override mechanism | error | — | 0.6-0.8 |
| `ai-bias-risk` | Art. 10 | Training data pipeline without bias check | warning | — | 0.5-0.7 |
| `ai-no-transparency` | Art. 13 | AI output without explanation capability | warning | — | 0.5-0.7 |
| `ai-no-risk-assessment` | Art. 9 | AI system without documented risk assessment | warning | — | 0.5-0.65 |
| `ai-no-accuracy-metric` | Art. 15 | AI model without accuracy/performance metric | warning | — | 0.5-0.7 |
| `ai-no-data-governance` | Art. 10 | Training data without provenance tracking | warning | — | 0.5-0.65 |
| `ai-no-version-control` | Art. 17 | AI model artifact without version tracking | warning | — | 0.5-0.7 |

---

## ISO 27001 — `iso27001` (10 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `hardcoded-secret` | A.8.5 | Credentials in source code | error | CWE-798 | 0.85-1.0 |
| `weak-crypto` | A.8.24 | Use of deprecated cryptographic algorithms | error | CWE-327 | 0.9-1.0 |
| `missing-access-control` | A.8.3 | Resource access without authorization | warning | CWE-862 | 0.6-0.8 |
| `missing-audit-log` | A.8.15 | Security event without logging | warning | CWE-778 | 0.5-0.7 |
| `insecure-transmission` | A.8.24 | Data over unencrypted channel | error | CWE-319 | 0.7-0.9 |
| `missing-input-validation` | A.8.28 | User input without sanitization | warning | CWE-20 | 0.6-0.8 |
| `sql-injection` | A.8.28 | String concatenation in SQL query | error | CWE-89 | 0.8-0.95 |
| `path-traversal` | A.8.28 | User input in file path without sanitization | error | CWE-22 | 0.7-0.9 |
| `insecure-deserialization` | A.8.28 | Untrusted data deserialization | warning | CWE-502 | 0.6-0.8 |
| `missing-rate-limit` | A.8.6 | Public endpoint without rate limiting | warning | CWE-770 | 0.5-0.7 |

---

## NIST 800-53 — `nist-800-53` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `hardcoded-secret` | IA-5 | Credentials in source code | error | CWE-798 | 0.85-1.0 |
| `weak-crypto` | SC-13 | Deprecated cryptographic algorithms | error | CWE-327 | 0.9-1.0 |
| `missing-audit-log` | AU-2 | Security event without audit trail | warning | CWE-778 | 0.5-0.7 |
| `missing-access-control` | AC-3 | Resource access without authorization | warning | CWE-862 | 0.6-0.8 |
| `insecure-transmission` | SC-8 | Data transmitted without encryption | error | CWE-319 | 0.7-0.9 |
| `missing-session-mgmt` | AC-12 | Session without timeout or invalidation | warning | CWE-613 | 0.5-0.7 |

---

## OWASP ASVS — `owasp-asvs` (8 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `sql-injection` | V5.3.4 | SQL injection risk | error | CWE-89 | 0.8-0.95 |
| `xss-risk` | V5.3.3 | Cross-site scripting risk | error | CWE-79 | 0.7-0.9 |
| `path-traversal` | V12.3.1 | Path traversal vulnerability | error | CWE-22 | 0.7-0.9 |
| `insecure-deserialization` | V5.5.3 | Unsafe deserialization | error | CWE-502 | 0.6-0.8 |
| `missing-csrf-protection` | V4.2.2 | State-changing endpoint without CSRF token | warning | CWE-352 | 0.6-0.8 |
| `hardcoded-secret` | V2.10.4 | Credentials in source code | error | CWE-798 | 0.85-1.0 |
| `weak-crypto` | V6.2.1 | Deprecated cryptographic algorithms | error | CWE-327 | 0.9-1.0 |
| `missing-input-validation` | V5.1.3 | Unvalidated user input | warning | CWE-20 | 0.6-0.8 |

---

## SOC 2 — `soc2` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `hardcoded-secret` | CC6.1 | Credentials in source code | error | CWE-798 | 0.85-1.0 |
| `missing-access-control` | CC6.1 | Resource access without authorization check | warning | CWE-862 | 0.6-0.8 |
| `missing-audit-log` | CC7.2 | Security-relevant operations without logging | warning | CWE-778 | 0.5-0.7 |
| `weak-crypto` | CC6.1 | Deprecated cryptographic algorithms | error | CWE-327 | 0.9-1.0 |
| `missing-error-handling` | CC7.3 | Unhandled errors in critical paths | warning | CWE-754 | 0.6-0.8 |
| `insecure-dependency` | CC7.1 | Known-vulnerable dependencies | warning | CWE-1104 | 0.7-0.9 |

---

## PCI DSS 4.0 — `pci-dss` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `card-data-logged` | Req. 3.4.2 | PAN/CVV/track data in logs | error | CWE-532 | 0.7-0.95 |
| `card-data-unencrypted` | Req. 3.5.1 | Cardholder data stored without encryption | error | CWE-311 | 0.7-0.9 |
| `hardcoded-secret` | Req. 8.3.2 | Authentication credentials in source | error | CWE-798 | 0.85-1.0 |
| `weak-crypto` | Req. 6.2.4 | Deprecated cryptographic algorithms | error | CWE-327 | 0.9-1.0 |
| `missing-input-validation` | Req. 6.2.4 | User input without sanitization in payment paths | warning | CWE-20 | 0.6-0.8 |
| `insecure-transmission` | Req. 4.2.1 | Cardholder data over non-TLS channels | error | CWE-319 | 0.7-0.9 |

---

## HIPAA — `hipaa` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `phi-unencrypted` | §164.312(a)(2)(iv) | Protected health information without encryption | error | CWE-311 | 0.7-0.9 |
| `phi-logged` | §164.312(b) | PHI written to logs without audit controls | error | CWE-532 | 0.7-0.95 |
| `missing-access-control` | §164.312(a)(1) | ePHI access without authentication check | error | CWE-862 | 0.6-0.8 |
| `hardcoded-secret` | §164.312(d) | Credentials in source code | error | CWE-798 | 0.85-1.0 |
| `missing-audit-log` | §164.312(b) | Access to PHI without audit trail | warning | CWE-778 | 0.5-0.7 |

---

## DORA — `dora` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-incident-reporting` | Art. 19 | No incident classification or reporting mechanism | warning | — | 0.5-0.65 |
| `missing-resilience-test` | Art. 26 | Critical path without resilience testing | warning | — | 0.5-0.65 |
| `missing-threat-model` | Art. 8 | Service without documented threat model | warning | — | 0.5-0.6 |
| `third-party-unmonitored` | Art. 30 | Third-party ICT dependency without monitoring | warning | — | 0.5-0.65 |
| `missing-backup-strategy` | Art. 12 | Data storage without backup/recovery mechanism | warning | — | 0.5-0.65 |
| `missing-change-control` | Art. 9 | ICT change without documented approval flow | warning | — | 0.5-0.6 |

---

## NIS2 — `nis2` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-incident-response` | Art. 23 | No incident response procedure | warning | — | 0.5-0.65 |
| `missing-supply-chain-check` | Art. 21(2)(d) | Dependency without supply chain security check | warning | — | 0.5-0.65 |
| `missing-crypto-policy` | Art. 21(2)(h) | Cryptographic operations without policy reference | warning | — | 0.5-0.6 |
| `missing-access-policy` | Art. 21(2)(i) | Access management without documented policy | warning | — | 0.5-0.6 |
| `missing-vulnerability-mgmt` | Art. 21(2)(e) | No vulnerability disclosure or handling process | warning | — | 0.5-0.65 |

---

## FDA 21 CFR Part 11 — `fda-21cfr11` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-electronic-signature` | §11.100 | Record modification without authenticated signature | error | — | 0.6-0.8 |
| `missing-audit-trail` | §11.10(e) | Electronic record without tamper-evident audit trail | error | CWE-778 | 0.6-0.8 |
| `missing-access-control` | §11.10(d) | System access without authority check | error | CWE-862 | 0.6-0.8 |
| `missing-timestamp` | §11.10(e) | Record without timestamped audit entry | warning | — | 0.5-0.7 |
| `missing-validation` | §11.10(a) | System without validation evidence | warning | — | 0.5-0.6 |

---

## EU Cyber Resilience Act — `eu-cra` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-sbom` | Art. 47 | Product without software bill of materials | error | — | 0.8-0.95 |
| `missing-vulnerability-handling` | Art. 11 | No vulnerability reporting or handling process | warning | — | 0.5-0.65 |
| `insecure-default` | Annex I, 2.1 | Product shipped with insecure default configuration | error | CWE-1188 | 0.6-0.8 |
| `missing-update-mechanism` | Annex I, 2.6 | No security update delivery mechanism | warning | — | 0.5-0.65 |
| `missing-secure-boot` | Annex I, 2.3 | Product without integrity verification at boot | warning | — | 0.5-0.65 |
| `excessive-attack-surface` | Annex I, 2.1 | Unnecessary open ports, services, or interfaces | warning | CWE-1059 | 0.5-0.7 |

---

## SBOM/SLSA — `sbom-slsa` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-sbom` | EO 14028 §4 | No SBOM generated for build artifacts | error | — | 0.8-0.95 |
| `missing-provenance` | SLSA v1.0 L2 | Build without provenance attestation | warning | — | 0.6-0.8 |
| `missing-build-isolation` | SLSA v1.0 L3 | Build process without isolation/hermetic build | warning | — | 0.5-0.7 |
| `unsigned-artifact` | SLSA v1.0 L2 | Release artifact without cryptographic signature | warning | — | 0.6-0.8 |
| `unvetted-dependency` | EO 14028 §4 | Dependency without security review or pinning | warning | CWE-1104 | 0.6-0.8 |

---

## IEC 61508 — Functional Safety — `iec61508` (7 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `unchecked-error` | Table A.3 | Error return value not checked (SIL 2+) | error | CWE-252 | 0.8-0.95 |
| `dynamic-allocation` | Table B.1 | Dynamic memory allocation in safety path (SIL 3+) | error | — | 0.85-1.0 |
| `recursive-call` | Table B.1 | Recursion in safety-critical code (SIL 2+) | error | CWE-674 | 0.9-1.0 |
| `missing-assertion` | Table A.9 | Safety invariant without runtime assertion | warning | CWE-617 | 0.5-0.7 |
| `global-mutable-state` | Table B.1 | Mutable global state in safety module | warning | CWE-362 | 0.7-0.9 |
| `pointer-arithmetic` | Table B.1 | Raw pointer arithmetic in safety path | warning | CWE-468 | 0.8-0.95 |
| `missing-watchdog` | Table A.5 | Long-running safety loop without watchdog/timeout | warning | CWE-835 | 0.5-0.7 |

---

## ISO 26262 — Automotive Safety — `iso26262` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `unchecked-error` | Part 6, Table 1 | Defensive programming violation | error | CWE-252 | 0.8-0.95 |
| `dynamic-allocation` | Part 6, Table 1 | Dynamic memory in ASIL C/D code | error | — | 0.85-1.0 |
| `missing-range-check` | Part 6, Table 1 | Input without range validation in control path | warning | CWE-129 | 0.6-0.8 |
| `complex-function` | Part 6, 9.4.3 | Function exceeding cyclomatic complexity limit | warning | CWE-1121 | 0.8-0.95 |
| `missing-independence` | Part 6, 9.4.4 | Safety function without independent review evidence | warning | — | 0.5-0.65 |

---

## DO-178C — Aviation Software — `do-178c` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `unchecked-error` | §6.3.3 | Unhandled error in DAL A-C code | error | CWE-252 | 0.8-0.95 |
| `dead-code` | §6.4.4.2 | Unreachable code in certified module | error | CWE-561 | 0.7-0.9 |
| `missing-traceability` | §5.5 | Requirement-to-code traceability gap | warning | — | 0.5-0.65 |
| `missing-test-coverage` | §6.4.4.2 | Function without structural coverage evidence | warning | — | 0.5-0.7 |
| `uninitialized-variable` | §6.3.3 | Variable used before initialization | error | CWE-457 | 0.7-0.9 |

---

## MISRA C/C++ — `misra` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `unchecked-return` | Rule 17.7 | Non-void function return value discarded | error | CWE-252 | 0.8-0.95 |
| `implicit-conversion` | Rule 10.3 | Implicit narrowing type conversion | warning | CWE-681 | 0.7-0.9 |
| `recursive-function` | Rule 17.2 | Recursive function call | error | CWE-674 | 0.9-1.0 |
| `dynamic-memory` | Rule 21.3 | Use of malloc/calloc/realloc/free | error | — | 0.9-1.0 |
| `goto-usage` | Rule 15.1 | Use of goto statement | warning | — | 0.9-1.0 |
| `missing-default-case` | Rule 16.4 | Switch without default case | warning | CWE-478 | 0.8-0.95 |

---

## IEC 62443 — `iec62443` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-zone-segmentation` | SR 5.1 | Network communication without zone boundary check | warning | CWE-284 | 0.5-0.7 |
| `hardcoded-secret` | SR 1.5 | Credentials embedded in industrial control code | error | CWE-798 | 0.85-1.0 |
| `missing-integrity-check` | SR 3.4 | Software/firmware without integrity verification | warning | CWE-345 | 0.6-0.8 |
| `insecure-protocol` | SR 4.1 | Use of unencrypted industrial protocol | error | CWE-319 | 0.7-0.9 |
| `missing-access-level` | SR 2.1 | Control function without authorization level | warning | CWE-862 | 0.5-0.7 |
| `missing-event-logging` | SR 6.1 | Security event without log entry | warning | CWE-778 | 0.5-0.7 |
