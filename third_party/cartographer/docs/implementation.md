# Cartographer — Implementation Reference

Current version: `1.6.0` (Rust, `mapper-core/cartographer/`)

---

## Signature extraction (`mapper.rs`)

The entry point is `extract_skeleton(path, content) -> MappedFile`. It dispatches by file extension to a per-language extractor. Each extractor runs one pass over the file's lines and produces:

- `imports: Vec<String>` — raw import/use/require statements
- `signatures: Vec<Signature>` — extracted symbol definitions

### Per-line extraction loop

Each extractor follows the same structure:

```
for (line_idx, line) in content.lines().enumerate():
  1. blank line          → clear doc_buf
  2. doc comment line    → push to doc_buf (/// / # / /** etc.)
  3. other comment       → clear doc_buf
  4. import statement    → push to imports, clear doc_buf
  5. scope opener        → emit Signature + update ScopeTracker
  6. symbol definition   → emit Signature with doc_buf, clear doc_buf
  7. anything else       → clear doc_buf
```

### `ScopeTracker`

Brace-depth tracker for `{}`-delimited languages (Rust, JS/TS, Java, PHP, C/C++):

```rust
struct ScopeTracker {
    stack: Vec<(String, usize)>,  // (scope_name, depth_when_opened)
    depth: usize,
}
```

- `.update(line, Some("Foo"))` — push scope "Foo" if the line has a net `{` opening
- `.update(line, None)` — just count braces, no new scope
- `.qualify("bar")` → `"Foo.bar"` if inside Foo scope, else `"bar"`

Python uses indentation-based class tracking instead. Go extracts the receiver type directly from the method signature (`func (r ReceiverType) Name()`). Ruby uses `end`-keyword depth counting.

### Symbol URI generation

```rust
fn lip_uri(path: &str, qualified_name: &str) -> String {
    let norm = path.trim_start_matches("./").trim_start_matches('/');
    format!("lip://local/{}#{}", norm, qualified_name)
}
```

`ckb_id` on every `Signature` is this URI. Stable across internal refactors, human-readable, LIP-compatible.

### Doc comment extraction

Preceding comment lines are buffered into `doc_buf: Vec<String>`. When a signature line is matched, `take_doc(&mut doc_buf)` drains the buffer into `sig.doc_comment`. A blank line clears the buffer, so only adjacent comments are attached.

Comment markers stripped: `///`, `//!`, `//`, `#`, `/**`, `* `.

---

## Graph construction (`api.rs`)

`ApiState::rebuild_graph()` runs over all `MappedFile`s and builds `ProjectGraphResponse`.

### Import resolution

`resolve_import_target(import, source)` maps a raw import string to a `module_id` using three strategies in cascade:

1. **Exact stem** — file stem matches the module name derived from the import
2. **Path segment** — file path contains the module stem as a component (min 3 chars)
3. **Symbol match** — a file's `signatures` contains `symbol_name` equal to the imported symbol (min 4 chars to reduce false positives)

Helpers:
- `parse_import_parts(import)` → `(module_path, Option<symbol_hint>)` — handles Rust `use`, Python `from … import`, JS/TS `import … from`, Java `import`, `require()`
- `derive_module_stem(path)` → last path component, strips npm scope prefix and kebab suffix
- `extract_js_import_symbol(lhs)` → extracts named/default import from the LHS of `import … from`

### Graph algorithms

| Analysis | Algorithm | Location |
|----------|-----------|----------|
| Cycle detection | Tarjan SCC (`petgraph`) | `detect_cycles` |
| Bridge detection | Brandes betweenness centrality (BFS) | `analyze_bridges`, `compute_betweenness_centrality` |
| God module detection | degree > 50 AND cohesion < 0.3 | `detect_god_modules` |
| Layer violations | Edge (source_layer → target_layer) against allowed_flows | `detect_layer_violations`, `layers.rs` |
| Role classification | In/out-degree + path heuristics | inline in `rebuild_graph` |
| Unreferenced exports | Symbol name not in any file's import tokens | inline in `rebuild_graph` |
| PageRank | Personalized PageRank, 30 iterations, damping=0.85 | `ranked_skeleton` |

### Health score formula

```
base = 100.0
- cycle_penalty        = min(cycle_count × 5, 30)
- bridge_penalty       = min((bridge_count / total_nodes) × 200, 20)
- god_module_penalty   = min(god_count × 3, 20)
- layer_penalty        = min(violation_count × 4, 25)
health = max(base - penalties, 0.0)
```

---

## Git analysis (`git_analysis.rs`)

All git operations shell out to `git` — no libgit2.

### Bot and formatting-commit filtering

Before any metric is computed, commits are filtered:

- **Bot filter**: author name contains `[bot]`, `dependabot`, `renovate`, `github-actions`, or similar patterns
- **Formatting filter**: commits where every changed file was touched by a formatter (prettier, rustfmt, eslint, gofmt) and the diff has no functional additions

### Co-change coupling score

Adam Tornhill's formula: `count / min(churn_a, churn_b)` where `count` is the number of commits that changed both files and `churn_a`/`churn_b` are the individual file churn counts.

### Hidden coupling

`cartographer_hidden_coupling` returns co-change pairs that have **no** static import edge between them. These files change together but are not explicitly linked in code — a useful architectural smell.

---

## Scanner (`scanner.rs`)

### Noise filtering pipeline

```
WalkDir
  → skip ignored dirs (node_modules, .git, target, dist, …)
  → skip security-blocked files (.env, *.pem, credentials.json, …)
  → skip .cartographerignore patterns
  → skip noise files (lock files, *.log, *.map, minified *.min.js, large SVG)
  → collect clean files
```

Noise files are tracked separately (not silently dropped) so the CLI can report how many tokens were saved by excluding them.

### `.cartographerignore`

Parsed as gitignore-style glob patterns. Patterns without `/` match filename only. `!pattern` negates. Compiled to `Regex` at load time.

---

## C FFI (`lib.rs`)

All FFI functions follow this contract:

```rust
#[no_mangle]
pub extern "C" fn cartographer_foo(path: *const c_char) -> *mut c_char {
    // 1. Convert C string inputs to Rust paths/strings
    // 2. Run the operation
    // 3. Serialize result to JSON
    // 4. Return heap-allocated C string (caller frees with cartographer_free_string)
}
```

All outputs are `{"ok": true, "data": ...}` on success or `{"ok": false, "error": "..."}` on failure. The `result_to_json_ptr` helper handles this pattern.

`cartographer_free_string(ptr)` reconstructs the `CString` and drops it, freeing the memory.

---

## MCP server (`mcp.rs`)

JSON-RPC 2.0 over stdio. Each tool call is dispatched through `McpServer::handle_tool_call` which builds an `ApiState`, calls the appropriate method, and returns the result as a JSON-RPC response.

Tools are declared as `McpTool` structs with JSON Schema input definitions so Claude and other MCP clients can call them with typed arguments.

---

## Adding a new language

To add skeleton extraction for a new language:

1. Add the file extension(s) to the `match` in `extract_skeleton` (mapper.rs)
2. Write `extract_<lang>(path: String, content: &str) -> MappedFile`
3. Use `ScopeTracker` for brace-delimited scopes, or implement indentation/keyword tracking for others
4. Map each pattern to the correct `SymbolKind`
5. Populate `qualified_name` using `scope.qualify(name)` for methods, bare `name` for top-level symbols
6. Add import statement pattern to `parse_import_parts` (api.rs) so dependency edges resolve correctly

---

## Adding a new FFI function

1. Implement the logic as a method on `ApiState` (or a free function in the relevant module)
2. Add the FFI wrapper in `lib.rs` following the existing pattern
3. Update the C header consumed by CKB
4. Document the response shape in a `/// Response shape: ...` doc comment on the function
