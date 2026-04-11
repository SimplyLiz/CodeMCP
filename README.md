# Code Cartographer for Architectural Intelligence

> The "GPS with Traffic Data" for your codebase - warns you about roadblocks before you even start driving.

## What is Cartographer?

Cartographer is a **structural intelligence engine** that maps your codebase's architecture, monitors its health, and predicts the ripple effects of changes before you make them.

It answers questions like:
- "What files are architectural bottlenecks?"
- "What happens if I change this function?"
- "Is my codebase getting healthier or more tangled?"
- "Who can I legally import from?"

## Quick Start

```bash
# Build
cd mapper-core/cargo && cargo build --release

# Generate architectural map
cartographer map

# Check health score
cartographer health

# Predict impact of a change
cartographer simulate --module src/auth/user.rs --new-signature "fn login(u: User)"

# See architectural trends
cartographer evolution --days 30
```

## Core Features

### 🗺️ The Map (Dependency Graph)
Generates `project_graph.json` - a complete dependency map at file/module level:
- Nodes: Files with their public API signatures
- Edges: Import/require/use relationships
- Metadata: Language, complexity estimates, bridge detection

### 🏛️ Bridge Detection
Identifies "Global Bridges" - files that connect disparate subsystems. Using **Bridgeness Centrality** (betweenness filtered to exclude noisy utility hubs), Cartographer finds the true architectural bottlenecks.

### 🛡️ Layer Enforcement
Prevents architectural drift with `layers.toml`:
```toml
[layers]
ui = ["components", "pages"]
services = ["api", "auth"]
db = ["models"]

[allowed_flows]
ui -> services
services -> db
```
Detects: BackCalls (db→ui), SkipCalls (ui→db without business layer)

### 📊 Health Scoring
Calculates architectural health from 0-100:
```
health = 100 - (cycles × 5) - (bridges × 2) - (god_modules × 3) - (violations × 4)
```

### 🔮 Predictive Simulation
Before you write code, Cartographer simulates the ripple effect:
- Will this create a cycle?
- Which modules will be affected?
- What layer violations will this cause?
- What's the health impact?

### 📈 Historical Evolution
Track architecture over time - see debt indicators, health trends, and get recommendations.

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    Cartographer                          │
├─────────────────────────────────────────────────────────┤
│  mapper.rs     │ Skeleton extraction (10+ languages) │
│  api.rs         │ Graph generation, health scoring    │
│  layers.rs      │ Layer config, violation detection    │
│  webhooks.rs    │ Change notifications                 │
│  mcp.rs         │ MCP server for AI tool integration  │
└─────────────────────────────────────────────────────────┘
                        │
                        ▼ (webhook / API)
                   ┌─────────┐
                   │   CKB   │  ← Deep semantic analysis
                   └─────────┘
```

## Cartographer vs CKB

| Aspect | Cartographer | CKB |
|--------|--------------|-----|
| View | Macro (file/module) | Micro (symbol/AST) |
| Speed | Fast (regex) | Deep (AST) |
| Purpose | Map, warn, predict | Analyze, refactor |
| Output | `project_graph.json` | Call graphs, refs |

**The handoff:** Cartographer identifies "where to look," CKB does the deep analysis there.

## CLI Commands

### Architecture & analysis

| Command | Description |
|---------|-------------|
| `cartographer map` | Skeleton map — imports + signatures only |
| `cartographer health` | Architectural health score (cycles, bridges, god modules) |
| `cartographer simulate --module <FILE>` | Predict impact before making a change |
| `cartographer check` | CI gate — exits non-zero on cycles or layer violations |
| `cartographer dead` | Dead code candidates (in-degree = 0) |
| `cartographer symbols --unreferenced` | Public exports not referenced anywhere |
| `cartographer diagram --format mermaid` | Dependency graph as Mermaid or Graphviz DOT |

### Git history signals

| Command | Description |
|---------|-------------|
| `cartographer hotspots` | High churn × high complexity files |
| `cartographer cochange --min-count 3` | Temporal coupling — files that always change together |
| `cartographer semidiff HEAD~1` | Function-level semantic diff between two commits |

### Search & file discovery

| Command | Description |
|---------|-------------|
| `cartographer search <PATTERN>` | Grep-like content search; `-i -v -w -o -l -c -A -B -C`, `--glob`, `--exclude`, `--path`, `--no-ignore` |
| `cartographer find <PATTERN>` | File find by glob; `--modified-since 24h`, `--newer`, `--min-size`, `--max-size`, `--max-depth` |

### Context injection (AI / local models)

| Command | Description |
|---------|-------------|
| `cartographer context --focus <FILE> --budget 8000` | Ranked skeleton pruned to token budget (personalized PageRank) |
| `cartographer context --query <PATTERN>` | Skeleton + search results bundled for models without tool calls |
| `cartographer llmstxt` | Generate `llms.txt` project index |
| `cartographer claudemd` | Generate `CLAUDE.md` architecture guide |

### Sync & MCP

| Command | Description |
|---------|-------------|
| `cartographer serve` | Start MCP server (JSON-RPC 2.0 stdio) |
| `cartographer watch` | Live file watching with optional cloud push |
| `cartographer evolution --days 30` | Architectural trends over time |

## Token Efficiency

Cartographer achieves **90%+ token reduction** vs full source code:
- Full source: ~5,000 tokens/module
- Cartographer skeleton: ~200 tokens/module
- AI-Lang compression strips `pub`, `private`, `async`, etc.

## Integrations

- **MCP Server** - AI tools can query via Model Context Protocol
- **Webhooks** - Notify CKB when graph changes
- **CKB** - Uses Cartographer as a filter for deep analysis

## Version History

- **v1.7.0** - Full grep + find parity: `-v`, `-w`, `-o`, `-l`, `-c`, `-e`, `-A/-B/-C`, `--exclude`, `--no-ignore`, `--path`; find with `--modified-since`, `--newer`, `--min/max-size`, `--max-depth`; ISO-8601 mtime in results; FFI + MCP updated
- **v1.6.0** - Bot/formatting-commit filtering in git history; personalized PageRank context (`cartographer context`); CI gate (`cartographer check`); unreferenced export detection
- **v1.5.0** - FFI wrappers for git churn, co-change, semidiff; `cartographer_version()` for compatibility checks
- **v1.4.0** - CCE integration, context compression
- **v1.3.0** - `cochange`, `hotspots`, `dead`, `diagram`, `llmstxt`, `claudemd`, `semidiff`; role classification; hotspot scoring
- **v1.2.0** - Hidden coupling detection; `cartographer_hidden_coupling` FFI; CKB query engine integration
- **v1.1.0** - Predictive simulation, historical evolution
- **v1.0.0** - CKB integration, symbol mapping
- **v0.5.0** - Layer enforcement, border patrol
- **v0.4.0** - Health monitoring, cycle/god detection
- **v0.3.0** - Bridge detection, AI-Lang compression
- **v0.2.0** - API, MCP server

## Author

SimplyLiz