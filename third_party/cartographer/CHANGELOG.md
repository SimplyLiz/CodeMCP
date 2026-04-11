# Changelog

All notable changes to Cartographer will be documented in this file.

## [2.4.0] - 2026-04-10

### Added — Co-change dispersion / shotgun surgery detection

**`src/git_analysis.rs`** — `CoChangeDispersion` struct + `git_cochange_dispersion()`:
- For each file, computes: `partner_count` (distinct co-change partners), `total_cochanges`, Shannon entropy (`−Σ p_i·log₂(p_i)`), and `dispersion_score` (0–100 normalised). High entropy + many partners = shotgun surgery smell (arXiv:2504.18511)
- Reuses existing `git_cochange()` output — no extra git subprocess

**`src/api.rs`** — 4 new fields on `GraphNode`:
- `fan_in` — in-degree (number of files that import this file)
- `fan_out` — out-degree = CBO, Coupling Between Objects (number of files this imports)
- `cochange_partners` — distinct co-change partners (populated by `enrich_with_git`)
- `cochange_entropy` — Shannon entropy of co-change distribution

**CLI**: `cartographer shotgun [--commits N] [--top N] [--min-partners N]` — ranked shotgun surgery candidates with HIGH/MODERATE/LOW tiers

**MCP tool**: `shotgun_surgery` — tool #29; returns `CoChangeDispersion[]` ranked by dispersion score

**FFI**: `cartographer_shotgun_surgery(path, limit, min_partners) -> *mut c_char` — #19

---

## [2.3.0] - 2026-04-10

### Added — Context health scoring (`token_metrics`)

**`src/token_metrics.rs`** — new module with research-backed context quality analysis:

- **Signal density** — ratio of symbol-bearing tokens to total. Below 5% triggers the attention dilution warning from Morph 2024 "Context Rot" (effective attention reduced to 1/40th at 2.5% density)
- **Compression density** — zlib ratio as an information entropy proxy (Entropy Law, arXiv:2407.06645). Below 30% = high boilerplate/redundancy
- **Position health** — U-shaped attention bias score; key modules at context boundaries score higher (Liu et al., TACL 2024: >30% accuracy drop for middle-placed content)
- **Entity density** — symbols per 1K tokens, BudgetMem-style signal (arXiv:2511.04919)
- **Utilisation headroom** — buffer between used tokens and model window (penalises >85%)
- **Dedup ratio** — unique-line fraction as quick redundancy check
- Composite score (0–100, graded A–F) with BudgetMem-informed weights: signal_density 25%, compression_density 20%, position_health 20%, entity_density 15%, utilisation_headroom 10%, dedup_ratio 10%

**CLI**: `cartographer context-health [FILE] [--model claude|gpt4|llama|gpt35] [--window N] [--format text|json]`

**MCP tool**: `context_health` — tool #27; scores any context string passed directly as an argument

**FFI**: `cartographer_context_health(content, opts_json) -> *mut c_char` for CKB

**13 tests** covering all individual metrics, composite analysis, and warning generation

### Added — PKG retrieval pipeline (`query_context`, `cartographer query`)

**MCP tool #28: `query_context`** — single-call retrieval pipeline replacing the manual search → ranked_skeleton → context_health sequence:
1. Searches the codebase for files matching the query (regex)
2. Uses matching files as the PageRank personalization seed
3. Builds a token-budget-aware skeleton ranked by relevance
4. Scores the bundle with context_health
5. Returns `{ context, filesUsed, focusFiles, totalTokens, health }` — ready to inject

**CLI**: `cartographer query <QUERY> [--budget N] [--model claude|gpt4|llama|gpt35] [--format text|json]`

**BM25 search** (`src/search.rs`): `bm25_search(root, query, opts)` — TF-IDF ranked file search for natural language queries, used by `query_context` as a complement to regex matching. No external dependencies; pure Rust with standard BM25 (k1=1.5, b=0.75). Returns ranked `Vec<BM25Match>` with per-file scores and matching term snippets.

---

## [2.1.0] - 2026-04-10

### Added — C/C++ tree-sitter extraction, import extraction, tests

**C and C++ grammars** (`lang-c`, `lang-cpp` features):
- C: `function_definition`, `declaration` (prototypes), `struct_specifier`, `union_specifier`, `enum_specifier`, `type_definition`, `preproc_def`, `preproc_function_def`, `preproc_include` (→ imports)
- C++: all of C plus `class_specifier` (with body walk for inline methods), `namespace_definition` (scoped), `template_declaration` (unwrapped), `linkage_specification` (`extern "C"`)
- `.h`/`.hpp`/`.cpp`/`.cc`/`.cxx` routed to C++ grammar when `lang-cpp` is enabled; `.c` uses C grammar

**Import extraction** — tree-sitter now also replaces the regex import pass for all supported languages:
- Rust: `use_declaration` nodes → strip `use ` / `;`
- Go: `import_declaration` → `import_spec` path strings (quoted paths stripped)
- Python: `import_statement` module names, `import_from_statement` module_name field
- TypeScript / JavaScript: `import_statement` source field (quotes stripped)
- C/C++: `preproc_include` path field (retains `<>` / `""` delimiters)

**Tests** — 27 tests across all 7 languages covering function extraction, method qualification, import extraction, visibility filtering, and symbol kinds.

---

## [2.0.0] - 2026-04-10

### Added — Tree-sitter skeleton extraction (Tier 2)

**`src/extractor.rs`** — new module that replaces regex heuristics for five languages:

- **Rust** — `function_item`, `impl_item`, `trait_item`, `struct_item`, `enum_item`, `type_item`, `const_item`, `static_item`, `macro_definition`, `mod_item`
- **Go** — `function_declaration`, `method_declaration` (receiver-qualified names), `type_declaration`, `const_declaration`, `var_declaration`
- **Python** — `function_definition`, `class_definition`, `decorated_definition`, `assignment` (ALL_CAPS constants only)
- **TypeScript / TSX** — function, class, method, interface, type alias, enum, arrow function (via `export const`), export statement wrappers
- **JavaScript / JSX / MJS / CJS** — same as TypeScript minus interfaces/type aliases

### Changed — Symbol confidence upgrade

All symbols extracted from Rust, Go, Python, TS, and JS now carry `confidence = 60` (LIP Tier 2) instead of `30`. C/C++, Java, Ruby, PHP, and all other languages continue to use the Tier 1 regex path until their grammars are added.

### Wiring

`mapper.rs:extract_skeleton()` runs the regex path first (to preserve import extraction, which tree-sitter does not do), then calls `crate::extractor::ts_extract()`. When `Some(sigs)` is returned, the regex `signatures` are replaced with the higher-confidence tree-sitter result.

---

## [1.8.0] - 2026-04-09

### Added — sed + awk equivalents

**`cartographer replace <PATTERN> <REPLACEMENT>`** — regex find-and-replace across project files:
- Replacement string supports `$0` (whole match), `$1`/`$2` (capture groups)
- `--dry-run` — preview what would change (shows colored diff, no writes)
- `--backup` — write `.bak` before modifying each file
- `-i` — case-insensitive; `-w` — whole-word; `--literal` — treat as literal string
- `-C N` — context lines in diff output (default: 3)
- `--glob "*.rs"` / `--exclude "*.gen.go"` / `--path src/api` — scope filters
- `--max-per-file N` — cap replacements per file (0 = unlimited)
- `--no-ignore` — operate on vendor/generated files too
- Colored terminal diff: red `-` for removed, green `+` for added lines
- Summary: files changed, total replacements, backup notice

**`cartographer extract <PATTERN>`** — capture-group extraction across project files (awk-like):
- `-g N` / `--group N` — capture group index (repeatable; default: 0 = whole match)
- `--count` — aggregate: show frequency table sorted by count descending
- `--dedup` — deduplicate extracted values
- `--sort` — sort output alphabetically (combined with `--count` → by frequency)
- `--format text|json|csv|tsv` — output format
- `--sep SEP` — separator between multiple groups (default: tab)
- `-i` — case-insensitive; `--glob` / `--exclude` / `--path` / `--no-ignore` — scope filters
- `--limit N` — cap total results

**FFI additions** (CKB + CGo consumers):
- `cartographer_replace_content(path, pattern, replacement, opts_json)`
- `cartographer_extract_content(path, pattern, opts_json)`

**CKB bridge** — `ReplaceOptions`, `ReplaceResult`, `FileChange`, `DiffLine`, `ExtractOptions`, `ExtractResult`, `ExtractMatch`, `CountEntry` added to `internal/cartographer`

---

## [1.7.0] - 2026-04-09

### Added — full grep + find parity

**`cartographer search <PATTERN>`** — complete grep parity:
- `-e PATTERN` — additional patterns OR'd together (like `grep -e`)
- `-i` — case-insensitive
- `-v` — invert match (lines that don't match)
- `-w` — whole-word match (`\b…\b`)
- `-o` — only-matching: print just the matched portion
- `-l` — files-with-matches: print only file paths
- `--files-without-match` — print only files with no matches
- `-c` — count matches per file
- `-A N` / `-B N` / `-C N` — after/before/symmetric context lines
- `--glob "*.rs"` — include filter; `--exclude "*.gen.go"` — exclude filter
- `--path src/api` — restrict to subdirectory
- `--no-ignore` — search vendor/generated/noise files too
- `--limit N` — cap results

**`cartographer find <PATTERN>`** — complete find parity:
- `--modified-since 24h` / `7d` / `30m` / `3600s` — mtime filter
- `--newer <FILE>` — files newer than reference file's mtime
- `--min-size N` / `--max-size N` — size filter in bytes
- `--max-depth N` — depth limit (0 = root only)
- `--no-ignore` — include vendor/noise directories
- Reports language + human-readable size + ISO-8601 mtime per file

**`cartographer context --query <PATTERN>`** — bundles ranked skeleton + search results for context injection into models without tool-call support (Qwen3, Llama 3, local models)

**FFI additions** (CKB + any CGo consumer):
- `cartographer_search_content(path, pattern, opts_json)` — all grep options exposed via JSON; `opts_json` can be null for defaults
- `cartographer_find_files(path, pattern, limit, opts_json)` — all find options via JSON

**MCP tool expansion** — `search_content` and `find_files` tools now expose all new options as top-level MCP arguments

**CKB bridge** — `SearchContentOptions`, `FindOptions`, `FileCount`, `MatchedTexts`, `FilesWithMatches`, `FilesWithoutMatch`, `FileCounts` added to `internal/cartographer` package

## [1.6.0] - 2026-04-09

### Added
- **Bot-author filtering** in git history analysis — commits from bots (`[bot]`, `dependabot`, `renovate`, `github-actions`, `snyk-bot`, etc.) are excluded from churn and co-change metrics; eliminates the ~74% noise inflation documented in arXiv 2602.13170
- **Formatting-commit filtering** — commits matching patterns like `cargo fmt`, `prettier`, `rustfmt`, `eslint fix`, `trailing whitespace`, etc. are excluded; same noise gate applied to all git-history paths (`git_churn`, `git_cochange`, FFI wrappers)
- **Personalized PageRank** over the dependency graph (`ranked_skeleton()` in `api.rs`) — 30-iteration power iteration with damping 0.85; personalization vector concentrates weight on focus files; used by:
  - `cartographer context --focus src/api.rs --budget 8000` — ranked skeleton pruned to token budget, highest-rank files first
  - `cartographer_ranked_skeleton(path, focus_json, budget)` — new FFI function for CKB context injection
- **CI enforcement** — `cartographer check` scans the project and exits non-zero if any cycles or layer violations are found; suitable for CI gates (pre-commit hook, GitHub Actions step)
- **Unreferenced export detection** — `rebuild_graph` builds an import-token corpus from all files and marks public symbols whose names don't appear in any import as `unreferenced_exports`; surfaced via:
  - `cartographer symbols --unreferenced` — file-by-file listing with caveat note
  - `cartographer_unreferenced_symbols(path)` — new FFI function

## [1.5.0] - 2026-04-09

### Added
- **`cartographer_version()`** — FFI function returning the library version string; CKB uses this for compatibility checks before calling any other function
- **`cartographer_git_churn(path, limit)`** — FFI wrapper for git churn analysis; returns `{ "src/api.rs": 42, ... }` (empty object when not a git repo)
- **`cartographer_git_cochange(path, limit, min_count)`** — FFI wrapper for temporal coupling; returns sorted array of `{ fileA, fileB, count, couplingScore }` pairs
- **`cartographer_semidiff(path, commit1, commit2)`** — FFI wrapper for semantic diff; returns per-file `{ path, status, added[], removed[] }` using skeleton extraction at each commit
- `mod git_analysis` added to `lib.rs` — git subprocess helpers are now available to all FFI callers, not just the CLI binary

## [1.4.0] - 2026-04-09

### Added
- **CCE integration** — `compressor.py` now compresses context through [ContextCompressionEngine](https://github.com/SimplyLiz/ContextCompressionEngine), reducing token usage while preserving code verbatim
  - `python compressor.py --messages chat.json --token-budget 8000` compresses any message array to fit a token budget
  - Cartographer dependency context is appended as a system message before compression
  - CCE path auto-discovered via `CCE_DIST` env var, `.cartographer/cce_dist` config, or sibling directory
- **`tools/cce_bridge.mjs`** — thin stdin/stdout Node.js bridge to CCE; normalises messages (adds `id`/`index`), accepts `--cce-dist` flag
- **`launch.py` CCE setup** — steps 5–6 check Node.js 20+ and build CCE; dist path saved to `.cartographer/cce_dist` for `compressor.py` to use
  - `--cce-path <dir>` overrides the default sibling-directory assumption

## [1.3.0] - 2026-04-09

### Added
- **`cochange`** — temporal coupling analysis from git history; surfaces files that always change together without an import link (`cartographer cochange --min-count 3`)
- **`hotspots`** — churn × complexity ranking with CRITICAL / HIGH / MODERATE / LOW tiers (`cartographer hotspots --top 10`)
- **`dead`** — dead code candidates based on in-degree = 0 in the dependency graph (`cartographer dead`)
- **`diagram`** — exports dependency graph as Mermaid or Graphviz DOT with role-based colouring (`cartographer diagram --format mermaid -o graph.md`)
- **`llmstxt`** — generates `llms.txt` index (entry points first, sorted by symbol count) for LLM inference-time context (`cartographer llmstxt`)
- **`claudemd`** — generates a `CLAUDE.md` architecture guide covering entry points, core modules, hotspots, cycles, and hidden coupling (`cartographer claudemd`)
- **`semidiff`** — function-level semantic diff between two commits using skeleton extraction (`cartographer semidiff HEAD~1`)
- **`git_analysis` module** — `git_churn`, `git_cochange`, `git_show_file`, `git_diff_files` helpers (binary-only; not exposed via C FFI)
- **Role classification** — every `GraphNode` now carries `role` (entry / core / utility / leaf / dead / bridge / standard), `churn`, `hotspot_score`, and `is_dead`
- **`CoChangePair`** in `ProjectGraphResponse` — populated by `enrich_with_git()`

## [1.2.0] - 2026-04-09

### Added
- **`launch.py`** — cross-platform Python installer replacing `install.sh`; supports Linux, macOS, and Windows; updates shell RC automatically
- **`deps` command** — `cartographer deps <target> --format json` outputs dependency graph for a target module as JSON
- **`serve` command** — `cartographer serve` starts the MCP server with full JSON-RPC 2.0 stdio transport
- **MCP tools** — `get_symbol_context` (filter signatures by symbol name) and `get_blast_radius` (dependencies + dependents up to depth limit)
- **`#[serde(rename = "type")]`** fix on `McpInputSchema` and `McpProperty` so tool schemas serialise correctly

### Fixed
- `compressor.py` called a non-existent `cmp deps` subcommand; now calls `cartographer deps`
- `verify_ignore.py` hardcoded the old `cmp` binary path; now resolves the correct platform binary
- Stale "architect" branding in `install.sh`

## [1.1.0] - 2025-04-07

### Changed
- Renamed binary from `architect` to `cartographer`
- Updated package description to "Code Cartographer for Architectural Intelligence"

### Added
- LICENSE file (CKB License)

## [1.0.0] - 2025-04-04

### Added
- Initial release as `architect` (formerly `cmp`)
- Graph-based code analysis engine
- Module context generation with dependency mapping
- Git-aware file scanning
- MCP server integration
- Webhook notifications for sync events
- Analytics and agents use cases
- Webhook use case handlers
- Python integration examples
- Shell installation scripts (install.sh, install.ps1)
