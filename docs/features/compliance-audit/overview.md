# Compliance Audit — Overview

CKB's compliance audit (`ckb audit compliance`) performs static analysis of your codebase against 20 regulatory frameworks, mapping code-level findings directly to regulation articles. Unlike tools that audit one framework at a time, CKB's cross-framework mapping means a single scan surfaces all regulatory exposure—a hardcoded credential finding simultaneously references GDPR Art. 32, PCI DSS Req. 8.3, HIPAA §164.312(d), SOC 2 CC6.1, ISO 27001 A.8.5, and NIST 800-53 IA-5.

## Key Stats

- **20 frameworks** across 8 categories (privacy, AI governance, security, industry, EU product, supply chain, safety, coding standards)
- **131 checks** total, each mapped to specific regulation articles
- **Cross-framework mapping** — one finding, all applicable regulations
- **Confidence scoring** — 0.0-1.0 per finding to reduce false positives
- **4 output formats** — human, JSON, markdown, SARIF (GitHub Code Scanning compatible)

## Why It Matters

No competing tool maps code findings directly to regulation articles across multiple frameworks simultaneously. Existing solutions require:

1. Running separate scans per framework
2. Manually correlating findings across reports
3. Hiring consultants to map code issues to regulatory text

CKB eliminates this overhead. One command, all frameworks, all mappings.

## Target Markets

| Market | Key Frameworks | Pain Point |
|--------|---------------|------------|
| **Healthcare** | HIPAA, FDA 21 CFR Part 11 | PHI protection, electronic records compliance |
| **Payments** | PCI DSS 4.0, SOC 2 | Cardholder data security, trust criteria |
| **B2B SaaS** | SOC 2, ISO 27001, GDPR | Multi-framework audit fatigue |
| **EU Companies** | GDPR, DORA, NIS2, EU CRA, EU AI Act | Overlapping EU regulations |
| **Automotive** | ISO 26262, MISRA C/C++ | ASIL functional safety |
| **Aviation** | DO-178C, IEC 61508 | DAL certification evidence |
| **Pharma** | FDA 21 CFR Part 11, HIPAA | Electronic records, audit trails |
| **Industrial** | IEC 62443, IEC 61508 | Industrial control system security |

## Usage

```bash
# Quick single-framework audit
ckb audit compliance --framework=gdpr

# Multi-framework for EU SaaS company
ckb audit compliance --framework=gdpr,dora,nis2,eu-ai-act

# Full audit for regulated industry
ckb audit compliance --framework=all --min-confidence=0.7 --format=sarif

# CI gate
ckb audit compliance --framework=gdpr,pci-dss,hipaa --ci --fail-on=error
```

See [checks.md](checks.md) for the complete reference of all 131 checks.
