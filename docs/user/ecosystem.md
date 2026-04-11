# The Stack — Cartographer, CKB, TruthKeeper, TurboQuant, ContextCompressionEngine, LLMRouter

Cartographer is one layer in a broader set of complementary tools. This document explains what each system does, where the boundaries are, and how a client consumes them together.

---

## Layer map

```
┌────────────────────────────────────────────────────────────────────────────┐
│  LLM / AI assistant (Claude, CKB agent, etc.)                              │
│                                                                            │
│  ContextCompressionEngine — manages this conversation window ◄──────────┐  │
└────┬──────────────────────────────────┬─────────────────────────────┘   │  │
     │ structural context               │ long-term knowledge               │
     ▼                                  ▼                                  │
┌────────────────┐          ┌──────────────────────────┐                  │
│  Cartographer  │          │       TruthKeeper         │                  │
│                │          │                           │   tool output    │
│  What the code │          │  What we know about the   │   feeds back up  │
│  looks like    │          │  code — and whether it's  │                  │
│  NOW           │          │  still true               │                  │
└────────────────┘          └──────────────┬────────────┘                  │
     │                                     │ embedding vectors              │
     │    ┌────────────────────────────┐   │                               │
     │    │          CKB               │   ▼                               │
     │    │                            │  ┌──────────────────────┐         │
     ├───►│  Compiler-accurate symbol  │  │     TurboQuant        │         │
     │    │  index, call graph, SCIP   │  │                       │         │
     │    │                            │  │  Compress embeddings  │         │
     │    └────────────────────────────┘  │  for fast retrieval   │         │
     │                 │                  └──────────────────────┘         │
     └─────────────────┴──────────────────────────────────────────────────►┘
               all outputs become LLM context
                              │
                              ▼
     ┌──────────────────────────────────────────────────────────────────────┐
     │  LLMRouter (FrugalRoute)                                             │
     │                                                                      │
     │  Every model call routes through here: cheapest capable model,       │
     │  semantic cache ($0 hits), distillation loop, budget enforcement     │
     └──────────────────────────────────────────────────────────────────────┘
               │
               ▼
     Ollama (local) → cloud (OpenAI / Anthropic / Google / Groq / Mistral / …)
```

---

## What each system does

### Cartographer

**Question it answers:** _What is the code's shape right now?_

Cartographer builds a semantic map of a codebase — not full source, but the shape: public API surfaces, imports, symbol kinds, dependency graph, git history signals. It is fast (sub-100ms on a full repo) and deliberately approximate. It does not require compilation.

**Outputs:**
- Dependency graph (nodes = files, edges = imports)
- Per-file symbol skeletons (`Signature` structs, confidence-graded)
- Git churn, co-change pairs, hotspot scores
- Architectural layer violation detection
- Dead-code and god-module detection

**Consumed by:** CKB via a C FFI (`libcartographer.a`), and directly via MCP server (26 tools over JSON-RPC stdio).

**Does NOT do:** Long-term memory, truth maintenance, embedding storage, or context window management.

---

### CKB

**Question it answers:** _What does this symbol mean, and who uses it?_

CKB is the compiler-accurate layer. It builds a SCIP index from source — actual type information, call graphs, reference chains — and exposes it as an MCP server consumed by AI assistants. Where Cartographer gives you the skeleton in 100ms, CKB gives you compiler truth in seconds.

CKB consumes Cartographer for:
- Blast-radius pre-filtering before deep graph traversal
- Git churn and co-change signals for hotspot prioritization
- Semantic diffs between commits
- Token-budget-aware context via `ranked_skeleton`

**Does NOT do:** Persistent cross-session memory or context window management.

---

### TruthKeeper

**Question it answers:** _What do we know about this codebase, and is it still true?_

TruthKeeper is an LLM memory system with dependency-aware truth maintenance. It stores facts about a project — architecture decisions, ownership, deprecated patterns, known issues — and continuously verifies them against their sources. When a source changes (a doc page, a file, a git commit), TruthKeeper cascades invalidation to all downstream facts and re-verifies them.

**States a fact can be in:** `SUPPORTED`, `OUTDATED`, `CONTESTED`, `HYPOTHESIS`

**Use cases alongside Cartographer:**
- "This module owns authentication" — fact stored in TruthKeeper, invalidated when `auth.rs` changes significantly
- "We deprecated `old_api.rs` in favour of `new_api.rs`" — tracked with provenance, surfaced when an AI tries to reference the old module
- Architecture decision records (ADRs) linked to the files they govern — when the file structure drifts, the ADR is flagged as `OUTDATED`

TruthKeeper does not parse code itself. Cartographer provides the structural signals (what changed, what's coupled) that TruthKeeper's source watchers can subscribe to.

**Does NOT do:** Structural analysis, dependency graphs, symbol extraction, or context window management.

---

### TurboQuant

**Question it answers:** _How do we store embeddings efficiently at scale?_

TurboQuant is an online vector quantization algorithm that compresses high-dimensional embedding vectors to low bit-widths with near-optimal distortion. It uses a two-stage approach: MSE-optimal quantization (rotation + scalar quantizers) followed by Quantized Johnson-Lindenstrauss for inner-product preservation.

At 3.5 bits per dimension it matches full-precision performance on long-context benchmarks.

**Relevant in this stack for:**
- TruthKeeper's semantic retrieval layer — fact embeddings stored compressed via TurboQuant
- CKB semantic search over symbol embeddings at scale

**Does NOT do:** Anything with code structure directly — it is a compression primitive.

---

### ContextCompressionEngine

**Question it answers:** _How do we keep the conversation window useful as it grows?_

ContextCompressionEngine (CCE) manages the LLM message history itself — the container that holds everything the other systems produce. As a multi-turn agent session grows, earlier turns accumulate stale prose, verbose tool output, and redundant context. CCE compresses that history deterministically (no API calls, no extra LLM) while preserving code blocks, structured data, and technical identifiers verbatim.

**How it works:**
- Multi-stage pipeline: classify → dedup → merge → summarize → size guard
- Three-tier classification: T0 (preserve — code, JSON, tables), T2 (compressible prose), T3 (removable filler)
- Deterministic sentence scoring rewards technical content (identifiers, file paths, status words)
- Size guard: if a summary would be longer than the original, the original is kept
- Fully reversible: every compression stores the original in a verbatim store; `uncompress()` restores byte-identical originals

**Measured performance:** 1.3–6.1× compression on synthetic scenarios; 1.5× on real Claude Code sessions (11.7M chars / 8,004 messages). Zero API calls, zero external dependencies.

**Relevant to Cartographer specifically:**
- When Cartographer returns a symbol graph or dependency tree as a tool response, CCE's agent pre-pass strips the verbose diagnostic noise while preserving the structured JSON payload
- Symbol names and file paths extracted by Cartographer are tracked as entities — CCE keeps them in future turns even if the original tool response is compressed away
- Cartographer's `ranked_skeleton` output (token-budget-aware) pairs naturally with CCE: Cartographer controls what goes in, CCE controls how long it stays

**Does NOT do:** Code analysis, memory maintenance, or embedding storage — it operates on messages, not source.

---

### LLMRouter (FrugalRoute)

**Question it answers:** _Which model should handle this call, and how cheaply can we do it?_

LLMRouter (published as `frugalroute`) is an OpenAI-compatible proxy that sits in front of every model call and routes it to the cheapest capable provider. It is the infrastructure layer that makes the rest of the stack economically viable at scale.

**How it works:**
- Semantic classifier: embeds each prompt against pre-defined routes (reasoning, coding, summarization, extraction, formatting) and picks the cheapest model that covers the required capabilities
- Keyword pre-classifier: sub-1ms pattern matching for obvious cases, before embedding
- Semantic cache: embedding-based deduplication of similar requests — cache hits cost $0 and return in ~1ms
- Budget enforcement: per-request and time-window budgets with atomic reservations (warn / reject / downgrade modes)
- Distillation loop: logs successful cloud calls and local model failures as training pairs; over time local models improve and more calls stay local
- Circuit breaker + health probing: detects failing providers and routes around them automatically

**Supported providers:** Ollama (local), OpenAI, Anthropic, Google, Groq, Mistral, Kimi, DeepSeek, and any OpenAI-compatible endpoint.

**Relevant to Cartographer specifically:**
- Cartographer's MCP server (27 tools) can be registered in LLMRouter's MCP registry — any agent routed through FrugalRoute automatically inherits Cartographer's tools without separate configuration
- Cartographer's `context_health` score (signal density, token count) can inform LLMRouter's model tier selection: a dense, well-structured context may not need the most capable model; a fragmented one should be escalated
- `ranked_skeleton --budget N` produces a known token count that feeds directly into LLMRouter's context-window constraint check before dispatch, preventing silent truncation
- Code analysis tasks where Cartographer's structural context was sufficient for a local model to answer correctly are ideal distillation candidates — the router's learning loop makes these cheaper over time

**Does NOT do:** Code parsing, memory, embeddings, or context compression — it is a routing and cost-optimization layer only.

---

## Boundary table

| Question | System |
|----------|--------|
| What files and symbols exist right now? | Cartographer |
| What are the exact types and call chains? | CKB |
| Which files change together? | Cartographer (`git_cochange`) |
| What's the blast radius of touching module X? | Cartographer + CKB |
| What do we know about this system's design? | TruthKeeper |
| Is our understanding of module X still accurate? | TruthKeeper |
| How do we store semantic embeddings cheaply? | TurboQuant |
| How do we stop the context window from rotting? | ContextCompressionEngine |
| How do we restore exactly what the agent saw before? | CCE verbatim store |
| Which model handles this call, and at what cost? | LLMRouter |
| How do we avoid paying cloud prices for routine tasks? | LLMRouter distillation |

---

## Using them together in a client

A fully-equipped LLM dev assistant uses all six layers:

```
1. User asks: "Is it safe to refactor AuthService?"

2. LLMRouter (routing, cost):
   → classifies as "coding / reasoning" task
   → checks semantic cache — no hit
   → estimates token budget: Cartographer context will be ~4K tokens
   → selects cheapest model that covers reasoning + code at this context size

3. Cartographer (fast, structural):
   → blast radius: 12 files import auth.rs
   → hotspot score: 87 (high churn × high complexity)
   → context_health: grade B (signal density 38%, position health good)
   → co-change: auth.rs ↔ session.rs always change together

4. CKB (accurate, deep):
   → 34 call sites for AuthService.verifyToken
   → 3 callers are in test files, 31 are in production paths

5. TruthKeeper (memory, truth):
   → "AuthService owns JWT validation, see ADR-012"  — SUPPORTED
   → "session.rs is being migrated to session_v2.rs" — HYPOTHESIS
   → "AuthService.refreshToken was deprecated in v2.1" — OUTDATED

6. TurboQuant (infrastructure):
   → TruthKeeper's embedding index compressed 4× for retrieval

7. ContextCompressionEngine (conversation layer):
   → Earlier turns compressed: probe messages, verbose tool echoes, build log noise
   → Preserved verbatim: Cartographer's dependency JSON, CKB symbol data, TruthKeeper facts
   → "AuthService", "verifyToken", "session_v2.rs" tracked as entities across all future turns
   → If user asks a follow-up, originals can be restored from verbatim store

   → LLMRouter logs this call; if local model answered correctly, becomes a distillation pair

Result: the assistant answers with structural, semantic, and institutional context —
the conversation window stays clean, and the call was routed to the cheapest viable model.
```

---

## What Cartographer is NOT trying to replace

Cartographer is intentionally scoped to fast structural analysis. It will not grow into:
- A persistent memory store (that's TruthKeeper)
- A compiler or type checker (that's CKB / SCIP)
- An embedding store (that's TurboQuant + a vector DB)
- A context window manager (that's ContextCompressionEngine)
- A model router or cost optimizer (that's LLMRouter / FrugalRoute)

These are hard boundaries. The value of each system comes from staying in its lane.
