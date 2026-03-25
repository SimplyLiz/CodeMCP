# Compliance Audit — Complete Check Reference

All 131 checks across 20 frameworks. Generated from source code.

---

## GDPR (Regulation (EU) 2016/679) — `gdpr` (11 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `pii-detection` | Art. 4(1) GDPR | PII Field Detection | info | — | varies |
| `pii-in-logs` | Art. 25, 32 GDPR | PII in Log Statements | error | CWE-532 | varies |
| `pii-in-errors` | Art. 25 GDPR | PII in Error Messages | error | — | varies |
| `weak-pii-crypto` | Art. 32 GDPR | Weak Cryptography on PII | error | CWE-327 | 0.85 |
| `plaintext-pii` | Art. 32 GDPR | Plaintext PII Storage | warning | — | 0.60 |
| `no-retention-policy` | Art. 5(1)(e) GDPR | Missing Data Retention Policy | warning | — | 0.65 |
| `no-deletion-endpoint` | Art. 17 GDPR | Missing Right to Erasure | warning | — | 0.60 |
| `missing-consent` | Art. 6, 7 GDPR | Missing Consent Verification | warning | — | 0.55 |
| `excessive-collection` | Art. 25 GDPR | Excessive Data Collection | warning | — | 0.70 |
| `unencrypted-transport` | Art. 32 GDPR | Unencrypted PII Transport | error | CWE-319 | 0.75 |
| `missing-access-logging` | Art. 30 GDPR | Missing Data Access Logging | warning | — | 0.60 |

---

## CCPA/CPRA (California Privacy Rights Act) — `ccpa` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-do-not-sell` | §1798.120 CCPA | Missing Do Not Sell/Share Opt-Out | warning | — | 0.70 |
| `third-party-sharing` | §1798.100 CCPA | Third-Party Data Sharing Detection | info | — | 0.75 |
| `sensitive-pi-exposure` | §1798.121 CCPA | Sensitive Personal Information Exposure | warning | — | 0.65 |
| `missing-data-access` | §1798.110 CCPA | Missing Data Access/Export Capability | warning | — | 0.60 |
| `missing-deletion` | §1798.105 CCPA | Missing Data Deletion Capability | warning | — | 0.60 |

---

## ISO 27701 (Privacy Extension) — `iso27701` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `no-consent-mechanism` | A.7.2.2 ISO 27701 | Missing Consent Mechanism | warning | — | 0.55 |
| `no-deletion-endpoint` | A.7.3.6 ISO 27701 | Missing Data Erasure Endpoint | warning | — | 0.60 |
| `no-access-endpoint` | A.7.3.6 ISO 27701 | Missing Data Access Endpoint | warning | — | 0.55 |
| `no-data-portability` | A.7.3.6 ISO 27701 | Missing Data Portability | info | — | 0.50 |
| `no-purpose-logging` | A.7.2.1 ISO 27701 | Missing Purpose Logging | warning | — | 0.55 |

---

## EU AI Act (Regulation (EU) 2024/1689) — `eu-ai-act` (8 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-model-logging` | Art. 12 EU AI Act | Missing Model I/O Logging | error | — | 0.70 |
| `no-audit-trail` | Art. 12, 19 EU AI Act | Missing AI Audit Trail | error | — | 0.60 |
| `missing-confidence-score` | Art. 13 EU AI Act | Missing Confidence Scores | warning | — | 0.60 |
| `no-human-override` | Art. 14 EU AI Act | Missing Human Override | error | — | 0.60 |
| `no-kill-switch` | Art. 14 EU AI Act | Missing Kill Switch | error | — | 0.60 |
| `missing-bias-testing` | Art. 10 EU AI Act | Missing Bias Testing | warning | — | 0.55 |
| `no-data-provenance` | Art. 10 EU AI Act | Missing Data Provenance | warning | — | 0.55 |
| `missing-version-tracking` | Art. 12 EU AI Act | Missing Model Version Tracking | warning | — | 0.55 |

---

## ISO 27001:2022 (Annex A) — `iso27001` (10 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `hardcoded-secret` | A.8.4 ISO 27001:2022 | Hardcoded Secrets | error | CWE-798 | 0.80 |
| `pii-in-logs` | A.8.12 ISO 27001:2022 | PII Data Leakage in Logs | error | — | varies |
| `hardcoded-config` | A.8.9 ISO 27001:2022 | Hardcoded Configuration | warning | — | 0.65 |
| `weak-crypto` | A.8.24 ISO 27001:2022 | Weak Cryptographic Algorithms | error | CWE-327 | 0.90 |
| `insecure-random` | A.8.24 ISO 27001:2022 | Insecure Random Number Generator | error | CWE-338 | 0.60-0.90 |
| `sql-injection` | A.8.28 ISO 27001:2022 | SQL Injection Risk | error | CWE-89 | 0.75 |
| `path-traversal` | A.8.28 ISO 27001:2022 | Path Traversal Risk | error | CWE-22 | 0.60-0.70 |
| `unsafe-deserialization` | A.8.7 ISO 27001:2022 | Unsafe Deserialization | error | CWE-502 | 0.75 |
| `missing-tls` | A.8.20 ISO 27001:2022 | Missing TLS Encryption | error | CWE-319 | 0.80 |
| `cors-wildcard` | A.8.27 ISO 27001:2022 | CORS Wildcard Origin | warning | — | 0.85 |

---

## NIST SP 800-53 Rev 5 — `nist-800-53` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-access-enforcement` | AC-3 NIST 800-53 | Missing Access Enforcement | error | — | 0.60 |
| `default-credentials` | IA-5(1) NIST 800-53 | Default Credentials | error | CWE-798 | 0.85 |
| `insufficient-audit-content` | AU-3 NIST 800-53 | Insufficient Audit Record Content | warning | — | 0.65 |
| `missing-audit-events` | AU-2 NIST 800-53 | Missing Auditable Events | warning | — | 0.70 |
| `non-fips-crypto` | SC-13 NIST 800-53 | Non-FIPS Cryptographic Algorithm | error | CWE-327 | 0.90 |
| `missing-input-validation` | SI-10 NIST 800-53 | Missing Input Validation | warning | — | 0.60 |

---

## OWASP ASVS 4.0 (Application Security Verification Standard) — `owasp-asvs` (13 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `weak-password-hash` | V2.4.1 ASVS | Weak Password Hashing Algorithm | error | CWE-916 | 0.85 |
| `hardcoded-credentials` | V2.10.4 ASVS | Hardcoded Credentials | error | CWE-798 | 0.80 |
| `insecure-cookie` | V3.4.2/V3.4.3 ASVS | Insecure Cookie Configuration | warning | CWE-614 | 0.60-0.80 |
| `eval-injection` | V5.2.4 ASVS | Dynamic Code Execution (Eval Injection) | error | CWE-95 | 0.75 |
| `sql-injection` | V5.3.4 ASVS | SQL Injection Risk | error | CWE-89 | 0.75 |
| `xss-prevention` | V5.3.3 ASVS | Cross-Site Scripting (XSS) Risk | error | CWE-79 | 0.80 |
| `command-injection` | V5.3.8 ASVS | OS Command Injection Risk | error | CWE-78 | 0.80 |
| `xxe` | V5.5.2 ASVS | XML External Entity (XXE) Risk | warning | CWE-611 | 0.60 |
| `weak-algorithm` | V6.2.5 ASVS | Deprecated Cryptographic Algorithm | error | CWE-327 | 0.90 |
| `insecure-random` | V6.3.1 ASVS | Insecure Random Number Generator | error | CWE-338 | 0.60-0.90 |
| `missing-tls` | V9.1.1 ASVS | Missing TLS for Sensitive Data | error | CWE-319 | 0.80 |
| `tls-bypass` | V9.2.1 ASVS | TLS Certificate Validation Bypass | error | CWE-295 | 0.90 |
| `asvs-cors-wildcard` | V14.5.3 ASVS | CORS Wildcard Origin | warning | CWE-346 | 0.85 |

---

## SOC 2 (Trust Service Criteria) — `soc2` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-auth-middleware` | CC6.1 SOC 2 | Missing Authentication Middleware | error | — | 0.60 |
| `insecure-tls-config` | CC6.7 SOC 2 | Insecure TLS Configuration | error | CWE-295 | 0.90 |
| `swallowed-errors` | CC7.2 SOC 2 | Swallowed Errors | warning | — | 0.70-0.80 |
| `missing-security-logging` | CC7.2 SOC 2 | Missing Security Event Logging | warning | — | 0.65 |
| `todo-in-production` | CC8.1 SOC 2 | TODO/FIXME in Production Code | info | — | 0.95 |
| `debug-mode-enabled` | CC8.1 SOC 2 | Debug Mode Enabled | warning | — | 0.75 |

---

## PCI DSS 4.0 (Payment Card Industry) — `pci-dss` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `pan-in-source` | Req 3.4 PCI DSS 4.0 | PAN in Source Code | error | CWE-312 | 0.70-0.90 |
| `pan-in-logs` | Req 3.3.1 PCI DSS 4.0 | Card Data in Logs | error | CWE-532 | 0.85 |
| `sql-injection` | Req 6.2.4 PCI DSS 4.0 | SQL Injection Risk | error | CWE-89 | 0.75 |
| `xss-prevention` | Req 6.2.4 PCI DSS 4.0 | Cross-Site Scripting (XSS) Risk | error | CWE-79 | 0.80 |
| `weak-password-policy` | Req 8.3.6 PCI DSS 4.0 | Weak Password Policy | warning | — | 0.70 |
| `hardcoded-credentials` | Req 8.6.2 PCI DSS 4.0 | Hardcoded Credentials | error | CWE-798 | 0.80 |

---

## HIPAA (Health Insurance Portability and Accountability Act) — `hipaa` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `phi-detection` | §164.514(b) HIPAA | PHI Field Detection | info | — | varies |
| `phi-in-logs` | §164.312(b) HIPAA | PHI in Log Statements | error | CWE-532 | varies |
| `missing-audit-trail` | §164.312(b) HIPAA | Missing HIPAA Audit Trail | warning | — | 0.65 |
| `phi-unencrypted` | §164.312(a)(2)(iv) HIPAA | Unencrypted PHI Storage | error | CWE-311 | 0.70 |
| `minimum-necessary` | §164.502(b) HIPAA | Minimum Necessary Violation | warning | — | 0.75 |

---

## DORA (Digital Operational Resilience Act) — `dora` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-circuit-breaker` | Art. 9 DORA | Missing Circuit Breaker Pattern | warning | — | 0.65 |
| `missing-timeout` | Art. 9 DORA | Missing Timeout on HTTP Client | warning | — | 0.75 |
| `missing-retry-logic` | Art. 9 DORA | Missing Retry/Backoff Logic | info | — | 0.55 |
| `missing-health-endpoint` | Art. 10 DORA | Missing Health Check Endpoint | warning | — | 0.70 |
| `missing-correlation-id` | Art. 10 DORA | Missing Correlation/Trace ID Propagation | info | — | 0.55 |
| `missing-rollback` | Art. 15 DORA | Missing Migration Rollback | warning | — | 0.55-0.70 |

---

## NIS2 Directive (EU 2022/2555) — `nis2` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `unverified-dependencies` | Art. 21(2)(d) NIS2 | Unverified Dependencies | warning | — | 0.80-0.90 |
| `missing-integrity-check` | Art. 21(2)(d) NIS2 | Missing Integrity Verification | warning | — | 0.70 |
| `missing-security-scanning` | Art. 21(2)(e) NIS2 | Missing Security Scanning in CI/CD | warning | — | 0.60-0.75 |
| `deprecated-crypto` | Art. 21(2)(j) NIS2 | Deprecated Cryptographic Algorithm | error | CWE-327 | 0.90 |
| `hardcoded-secrets` | Art. 21(2)(g) NIS2 | Hardcoded Secrets/Credentials | error | CWE-798 | 0.80 |

---

## FDA 21 CFR Part 11 (Electronic Records) — `fda-21cfr11` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-audit-trail` | §11.10(e) 21 CFR Part 11 | Missing Audit Trail | error | — | 0.70 |
| `mutable-audit-records` | §11.10(e) 21 CFR Part 11 | Mutable Audit Records | warning | — | 0.85 |
| `missing-authority-check` | §11.10(d) 21 CFR Part 11 | Missing Authority Check | warning | — | 0.55 |
| `missing-esignature` | §11.50 21 CFR Part 11 | Missing Electronic Signature Support | info | — | 0.50 |
| `missing-input-validation` | §11.10(a) 21 CFR Part 11 | Missing Input Validation | warning | — | 0.60 |

---

## EU Cyber Resilience Act (Regulation 2024/2847) — `eu-cra` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `insecure-defaults` | Art. 13 EU CRA | Insecure Default Configuration | error | — | 0.80 |
| `unnecessary-attack-surface` | Annex I, Part I(1) EU CRA | Unnecessary Attack Surface | warning | — | 0.55 |
| `missing-dep-scanning` | Annex I, Part I(2) EU CRA | Missing Dependency Scanning | warning | — | 0.75 |
| `known-vulnerable-patterns` | Annex I, Part I(1) EU CRA | Known Vulnerable Code Patterns | error | CWE-89, CWE-79, CWE-78, CWE-22, CWE-502 | 0.75 |
| `missing-sbom` | Art. 13(6) EU CRA | Missing SBOM Generation | warning | — | 0.80 |
| `missing-update-mechanism` | Annex I, Part I(3) EU CRA | Missing Update Mechanism | info | — | 0.55 |

---

## SBOM & Supply Chain Security (EO 14028, SLSA) — `sbom-slsa` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `missing-sbom-generation` | EO 14028 §4(e) | Missing SBOM Generation | warning | — | 0.75 |
| `missing-lock-file` | SLSA Level 1 | Missing Dependency Lock File | warning | — | 0.90 |
| `unpinned-dependencies` | SLSA Level 2 | Unpinned Dependency Versions | warning | — | 0.80-0.85 |
| `missing-provenance` | SLSA Level 2 | Missing Build Provenance | info | — | 0.60 |
| `unsigned-commits` | SLSA Level 2 | Unsigned Commits Policy | info | — | 0.55 |

---

## IEC 61508 / SIL (Safety Integrity) — `iec61508` (7 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `goto-usage` | Table B.1 IEC 61508-3 | Goto Statement Usage | warning | — | 0.95 |
| `recursion` | Table B.9 IEC 61508-3 | Recursive Function Calls | warning | — | 0.80 |
| `deep-nesting` | Table B.1 IEC 61508-3 | Deep Nesting | warning | — | 0.85 |
| `large-function` | Table B.9 IEC 61508-3 | Large Function | warning | — | 0.90 |
| `global-state` | Table B.9 IEC 61508-3 | Global Mutable State | warning | — | 0.65 |
| `unchecked-error` | Table A.3 IEC 61508-3 | Unchecked Error Returns | error | — | 0.85 |
| `complexity-exceeded` | Table B.9 IEC 61508-3 | Complexity Limit Exceeded | error | — | 0.95 |

---

## ISO 26262 (Automotive Functional Safety) — `iso26262` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `complexity-exceeded` | Part 6, Table 3 ISO 26262 | Complexity Limit Exceeded | error | — | 0.95 |
| `recursion` | Part 6, Table 3 ISO 26262 | Recursive Function Calls | warning | — | 0.80 |
| `dynamic-memory` | Part 6, Table 3 ISO 26262 | Dynamic Memory Allocation | warning | — | 0.90 |
| `missing-null-check` | Part 6, 8.4.4 ISO 26262 | Missing Null Check Before Dereference | warning | — | 0.60 |
| `unchecked-return` | Part 6, 8.4.4 ISO 26262 | Unchecked Return Value | error | — | 0.85 |

---

## DO-178C (Software Considerations in Airborne Systems) — `do-178c` (5 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `dead-code` | §6.4.4.2 DO-178C | Dead Code Detection | error | — | 0.70 |
| `complexity-exceeded` | §6.3.4 DO-178C | Complexity Limit Exceeded | error | — | 0.95 |
| `goto-usage` | §6.3.4 DO-178C | Goto Statement Usage | error | — | 0.95 |
| `recursion` | §6.3.4 DO-178C | Recursive Function Calls | error | — | 0.80 |
| `missing-requirement-tag` | §6.3.1 DO-178C | Missing Requirement Traceability Tag | warning | — | 0.55 |

---

## MISRA C:2023 / C++:2023 — `misra` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `goto-usage` | Rule 15.1 MISRA C | Goto Statement Usage | error | — | 0.95 |
| `unreachable-code` | Rule 2.1 MISRA C | Unreachable Code | warning | — | 0.75 |
| `missing-switch-default` | Rule 16.4 MISRA C | Missing Switch Default Case | warning | — | 0.80 |
| `dynamic-allocation` | Rule 21.3 MISRA C | Dynamic Memory Allocation | warning | — | 0.90 |
| `unsafe-string-functions` | Rule 21.14 MISRA C | Unsafe String Functions | error | CWE-676 | 0.95 |
| `implicit-conversion` | Rule 10.1 MISRA C | Implicit Type Conversion | warning | — | 0.65 |

---

## IEC 62443 (Industrial Automation Security) — `iec62443` (6 checks)

| Check ID | Article | What It Detects | Severity | CWE | Confidence |
|----------|---------|-----------------|----------|-----|------------|
| `default-credentials` | CR 1.1 IEC 62443-4-2 | Default/Hardcoded Credentials | error | CWE-798 | 0.85 |
| `missing-auth` | CR 1.2 IEC 62443-4-2 | Missing Authentication on Control Functions | error | — | 0.70 |
| `unvalidated-input` | CR 3.5 IEC 62443-4-2 | Unvalidated Network Input | error | CWE-20 | 0.65 |
| `missing-message-auth` | CR 3.1 IEC 62443-4-2 | Missing Message Authentication | warning | — | 0.55 |
| `unsafe-functions` | SD-4 IEC 62443-4-1 | Unsafe/Banned Functions | error | CWE-676 | 0.95 |
| `missing-error-handling` | SD-4 IEC 62443-4-1 | Missing Error Handling | warning | — | 0.70-0.85 |
