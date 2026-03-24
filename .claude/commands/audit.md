Run a CKB-augmented compliance audit optimized for minimal token usage.

## Input
$ARGUMENTS - Optional: framework(s) to audit (default: auto-detect from repo context). Examples: "gdpr", "gdpr,pci-dss,hipaa", "all"

## Philosophy

CKB already ran 126 deterministic checks across 20 regulatory frameworks, mapped every finding
to a specific regulation article, and assigned confidence scores. The LLM's job is ONLY what
CKB can't do: assess whether findings are real compliance risks or false positives given the
repo's actual purpose, and prioritize remediation by business impact.

### CKB's blind spots (what the LLM must catch)

CKB maps code patterns to regulation articles using AST + regex + tree-sitter. It is
structurally correct but contextually blind:

- **Business context**: CKB flags PII patterns in a healthcare app and a game engine equally
- **Architecture awareness**: a finding in dead/test code vs production code has different weight
- **Compensating controls**: CKB can't see infrastructure-level encryption, WAFs, or IAM policies
- **Regulatory applicability**: CKB flags HIPAA in a repo that doesn't handle PHI
- **Risk prioritization**: 50 findings need ordering by actual business/legal exposure
- **Cross-reference noise**: the same hardcoded credential maps to 6 frameworks — that's 1 fix, not 6

## Phase 1: Structural scan (~2k tokens into context)

```bash
ckb audit compliance --framework=$ARGUMENTS --format=json --min-confidence=0.7 2>/dev/null
```

If no framework specified, pick based on repo context:
- Has health/patient/medical code → `hipaa,gdpr`
- Has payment/billing/card code → `pci-dss,soc2`
- EU company or processes EU data → `gdpr,dora,nis2`
- AI/ML code → `eu-ai-act`
- Safety-critical/embedded → `iec61508,iso26262,misra`
- General SaaS → `iso27001,soc2,owasp-asvs`
- If unsure → `iso27001,owasp-asvs` (broadest applicability)

From the output, note:
- **Per-framework scores** — which frameworks are clean vs problematic
- **Verdict** — pass/warn/fail
- **Finding count by severity** — errors are your priority
- **Cross-framework findings** — deduplicate (1 code issue = 1 fix regardless of how many frameworks flag it)

**Early exit**: If verdict=pass and all framework scores ≥ 90, write a one-line summary and stop.

## Phase 2: Triage findings (targeted reads only)

Do NOT read every flagged file. Group findings by root cause first:

1. **Deduplicate cross-framework findings** — a hardcoded secret flagged by GDPR, PCI DSS, HIPAA, and ISO 27001 is one fix
2. **Check applicability** — does this repo actually fall under the flagged framework? (e.g., HIPAA findings in a non-healthcare repo)
3. **Read only error-severity files** — warnings and info can wait
4. **For each error finding**, read just the flagged lines (not the whole file) and assess:
   - Is this a real compliance risk or a pattern false positive?
   - Are there compensating controls elsewhere? (check imports, config, middleware)
   - What's the remediation effort: one-liner fix vs architectural change?

## Phase 3: Write the audit summary (be terse)

```markdown
## [COMPLIANT|NEEDS REMEDIATION|NON-COMPLIANT] — CKB score: [N]/100

[One sentence: what frameworks were audited and overall posture]

### Critical findings (must remediate)
1. **[framework]** `file:line` Art. [X] — [issue + remediation in one sentence]
2. ...

### Not applicable (false positives from context)
[List findings CKB flagged but that don't apply to this repo, with one-line reason]

### Cross-framework deduplication
[N findings deduplicated to M root causes]

### Framework scores
[framework]: [score] — [pass/warn/fail]
```

If fully compliant: just the header + framework scores. Nothing else.

## Anti-patterns (token waste)

- Reading every flagged file → waste (group by root cause, read only errors)
- Treating cross-framework duplicates as separate issues → waste (1 code fix = 1 issue)
- Explaining what each regulation requires → waste (CKB already mapped articles)
- Re-checking frameworks CKB scored at 100 → waste
- Auditing frameworks that don't apply to this repo → waste
- Reading low-confidence findings (< 0.7) → waste (likely false positives)
- Suggesting infrastructure controls for code-level findings → out of scope
