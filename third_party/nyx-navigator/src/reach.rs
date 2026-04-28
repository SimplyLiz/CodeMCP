//! Semantic graph traversal for AI context injection.
//!
//! `reach` starts from a named symbol and walks the call graph + import graph
//! outward, returning a compact context tree at distance-proportional detail:
//!   depth 0  — root symbol: signature + body (up to 30 lines if available)
//!   depth 1  — callers: signature + one-line call context snippet
//!              callees: signature only
//!   depth 2  — neighbors of callees: name + file + signature only
//!
//! The output format is a compact indented-tree notation designed to minimize
//! token count while preserving semantic type information — roughly 40% of
//! equivalent JSON for the same information.
//!
//! # Caller discovery
//! Uses regex word-search across all mapped files (Option A: heuristic).
//! For Rust and Python, the file-local call graph cross-checks callees.
//! Cross-file callers are found by text search; false-positive rate is ~10%
//! from substring matches (e.g. a variable named after the function).
//! A future Option B pass using the inverted call graph would eliminate these.

use std::path::Path;

use crate::call_graph::build_file_call_graph;
use crate::formatter::estimate_tokens;
use crate::mapper::{MappedFile, Signature, SymbolKind};
use crate::search::{search_content, SearchOptions};

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

#[derive(Debug, Clone)]
pub struct ReachOptions {
    /// Hop count. Default 2.
    pub depth: usize,
    /// Hard token cap; trims leaf nodes first. Default 6000.
    pub budget: usize,
    /// Disambiguate when the symbol appears in multiple files.
    pub file_filter: Option<String>,
    /// Expand test call sites instead of collapsing them. Default false.
    pub include_tests: bool,
    /// Maximum caller entries to show (before collapsing). Default 12.
    pub max_callers: usize,
    /// Include the function body of the root symbol (up to 40 lines). Default false.
    pub show_body: bool,
}

impl Default for ReachOptions {
    fn default() -> Self {
        Self {
            depth: 2,
            budget: 6000,
            file_filter: None,
            include_tests: false,
            max_callers: 12,
            show_body: false,
        }
    }
}

#[derive(Debug)]
pub enum ReachError {
    /// Symbol not found in any mapped file.
    NotFound(String),
    /// Symbol found in multiple files; user must specify --file.
    Ambiguous(Vec<AmbiguousCandidate>),
}

#[derive(Debug)]
pub struct AmbiguousCandidate {
    pub file: String,
    pub line: u32,
    pub sig: String,
}

impl std::fmt::Display for ReachError {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            ReachError::NotFound(s) => write!(f, "symbol not found: {s}"),
            ReachError::Ambiguous(candidates) => {
                writeln!(f, "ambiguous — found in {} files:", candidates.len())?;
                for c in candidates {
                    writeln!(f, "  {}:{} — {}", c.file, c.line, c.sig)?;
                }
                write!(f, "use --file to disambiguate")
            }
        }
    }
}

/// The resolved location of the root symbol.
#[derive(Debug, Clone)]
pub struct RootSymbol {
    pub file: String,
    pub line: u32,
    pub kind: SymbolKind,
    pub sig: String,
    pub name: String,
    pub visibility: Visibility,
}

#[derive(Debug, Clone, PartialEq)]
pub enum Visibility {
    Public,
    Crate,
    Private,
}

/// One call site found by the caller search.
#[derive(Debug, Clone)]
pub struct CallerInfo {
    pub file: String,
    pub line: usize,
    pub snippet: String,
    pub tag: Option<CallerTag>,
    pub is_test: bool,
}

#[derive(Debug, Clone, PartialEq)]
pub enum CallerTag {
    Handler,
    Middleware,
    Entry,
}

impl CallerTag {
    pub fn label(&self) -> &'static str {
        match self {
            CallerTag::Handler => "handler",
            CallerTag::Middleware => "middleware",
            CallerTag::Entry => "entry",
        }
    }
}

/// One callee — a function this symbol calls.
#[derive(Debug, Clone)]
pub struct CalleeInfo {
    pub name: String,
    pub file: String,
    pub line: u32,
    pub sig: String,
}

/// One depth-2 neighbor (signature-only).
#[derive(Debug, Clone)]
pub struct NeighborInfo {
    pub name: String,
    pub file: String,
    pub line: u32,
    pub sig: String,
}

/// The assembled context tree returned by `build_reach`.
#[derive(Debug)]
pub struct ReachResult {
    pub root: RootSymbol,
    pub callers: Vec<CallerInfo>,
    pub test_callers_collapsed: usize,
    pub callees: Vec<CalleeInfo>,
    pub depth2: Vec<NeighborInfo>,
    /// Warning emitted when call graph is unavailable for the root's language.
    pub language_note: Option<String>,
    /// Function body of the root symbol (populated when show_body = true).
    pub root_body: Option<String>,
    pub tokens_used: usize,
    pub budget_hit: bool,
}

// ---------------------------------------------------------------------------
// build_reach
// ---------------------------------------------------------------------------

/// Build a reach context tree starting from `symbol_query` in `mapped` files.
///
/// `root_path` is the project root used for content search.
pub fn build_reach(
    root_path: &Path,
    mapped: &[MappedFile],
    symbol_query: &str,
    opts: &ReachOptions,
) -> Result<ReachResult, ReachError> {
    // --- 1. Resolve the root symbol ---
    let root = resolve_symbol(mapped, symbol_query, opts.file_filter.as_deref())?;

    // --- 2. Find callers via text search ---
    let (callers, test_callers_collapsed) = find_callers(root_path, mapped, &root, opts);

    // --- 3. Find callees via call graph (Rust/Python) or unresolved stubs ---
    let (callees, language_note) = find_callees(root_path, mapped, &root, opts);

    // --- 4. Depth-2 neighbors ---
    // For functions: types referenced in callee signatures.
    // For structs/enums/types: types referenced in the struct's own fields (body).
    let depth2 = if opts.depth >= 2 {
        match root.kind {
            SymbolKind::Struct | SymbolKind::Class | SymbolKind::Interface | SymbolKind::Enum => {
                find_depth2_from_type(mapped, &root)
            }
            _ => find_depth2(mapped, &callees),
        }
    } else {
        vec![]
    };

    // --- 5. Optionally read the root function body ---
    let root_body = if opts.show_body {
        read_root_body(root_path, &root, 40)
    } else {
        None
    };

    // --- 6. Assemble + apply token budget ---
    let mut result = ReachResult {
        root,
        callers,
        test_callers_collapsed,
        callees,
        depth2,
        language_note,
        root_body,
        tokens_used: 0,
        budget_hit: false,
    };

    let rendered = render_reach(&result);
    result.tokens_used = estimate_tokens(&rendered);
    if result.tokens_used > opts.budget {
        trim_to_budget(&mut result, opts.budget);
        result.tokens_used = estimate_tokens(&render_reach(&result));
        result.budget_hit = true;
    }

    Ok(result)
}

// ---------------------------------------------------------------------------
// Symbol resolution
// ---------------------------------------------------------------------------

fn resolve_symbol(
    mapped: &[MappedFile],
    query: &str,
    file_filter: Option<&str>,
) -> Result<RootSymbol, ReachError> {
    // Normalize query: strip common prefixes like "fn ", "pub fn ", "struct " etc.
    let name = query
        .trim()
        .trim_start_matches("pub ")
        .trim_start_matches("async ")
        .trim_start_matches("fn ")
        .trim_start_matches("struct ")
        .trim_start_matches("enum ")
        .trim_start_matches("trait ")
        .trim()
        .to_string();

    let mut candidates: Vec<AmbiguousCandidate> = vec![];
    let mut best: Option<RootSymbol> = None;

    'outer: for mf in mapped {
        // Apply file filter if present.
        if let Some(ff) = file_filter {
            if !mf.path.contains(ff) {
                continue;
            }
        }

        for sig in &mf.signatures {
            let matches = sig_matches_query(sig, &name);
            if !matches {
                continue;
            }

            let root = RootSymbol {
                file: mf.path.clone(),
                line: sig.line_start as u32 + 1,
                kind: sig.kind,
                sig: sig.raw.clone(),
                name: sig
                    .symbol_name
                    .clone()
                    .or_else(|| sig.qualified_name.clone())
                    .unwrap_or_else(|| name.clone()),
                visibility: detect_visibility(&sig.raw),
            };

            if best.is_none() {
                best = Some(root);
            } else {
                // More than one — collect for ambiguity error.
                if candidates.is_empty() {
                    if let Some(ref b) = best {
                        candidates.push(AmbiguousCandidate {
                            file: b.file.clone(),
                            line: b.line,
                            sig: b.sig.clone(),
                        });
                    }
                }
                // Use mf.path directly — the root we built already holds it.
                candidates.push(AmbiguousCandidate {
                    file: root.file.clone(),
                    line: root.line,
                    sig: root.sig.clone(),
                });
                // Keep scanning to collect all candidates for the error message.
                continue 'outer;
            }
        }
    }

    if !candidates.is_empty() {
        // Add the best we found too.
        if let Some(b) = best {
            if !candidates.iter().any(|c| c.file == b.file && c.line == b.line) {
                candidates.insert(0, AmbiguousCandidate {
                    file: b.file,
                    line: b.line,
                    sig: b.sig,
                });
            }
        }
        return Err(ReachError::Ambiguous(candidates));
    }

    best.ok_or_else(|| ReachError::NotFound(name))
}

fn sig_matches_query(sig: &Signature, query: &str) -> bool {
    // Qualified name exact match: "Foo::bar" or "Foo.bar"
    if let Some(ref qn) = sig.qualified_name {
        if qn == query || qn.ends_with(&format!("::{query}")) || qn.ends_with(&format!(".{query}")) {
            return true;
        }
    }
    // Simple name match
    if let Some(ref sn) = sig.symbol_name {
        if sn == query {
            return true;
        }
    }
    false
}

fn detect_visibility(raw: &str) -> Visibility {
    let t = raw.trim_start();
    if t.starts_with("pub(crate)") || t.starts_with("pub(super)") {
        Visibility::Crate
    } else if t.starts_with("pub") {
        Visibility::Public
    } else {
        Visibility::Private
    }
}

// ---------------------------------------------------------------------------
// Caller discovery
// ---------------------------------------------------------------------------

fn find_callers(
    root_path: &Path,
    mapped: &[MappedFile],
    root: &RootSymbol,
    opts: &ReachOptions,
) -> (Vec<CallerInfo>, usize) {
    let search_opts = SearchOptions {
        word_regexp: true,
        case_sensitive: true,
        max_results: opts.max_callers * 3, // over-fetch; we'll filter definitions
        context_lines: 0,
        ..Default::default()
    };

    let results = match search_content(root_path, &root.name, &search_opts) {
        Ok(r) => r,
        Err(_) => return (vec![], 0),
    };

    let mut callers: Vec<CallerInfo> = vec![];
    let mut test_count = 0usize;

    for m in &results.matches {
        // Skip the definition site itself.
        if m.path == root.file && m.line_number == root.line as usize {
            continue;
        }

        // Skip non-source files (JSON memory files, Markdown, etc. that may
        // contain source code as embedded strings).
        if !is_source_ext(&m.path) {
            continue;
        }

        // Skip lines that look like definitions, imports, or comments.
        let trimmed = m.line.trim();
        if is_definition_line(trimmed) || is_import_line(trimmed) || is_comment_line(trimmed) {
            continue;
        }

        let is_test = is_test_path(&m.path)
            || snippet_looks_like_test(trimmed)
            || same_file_is_test_fn(mapped, &m.path, m.line_number);

        if is_test {
            test_count += 1;
            if !opts.include_tests {
                continue;
            }
        }

        let tag = classify_caller_path(&m.path);

        callers.push(CallerInfo {
            file: m.path.clone(),
            line: m.line_number,
            snippet: trimmed.to_string(),
            tag,
            is_test,
        });

        if callers.len() >= opts.max_callers {
            break;
        }
    }

    // If we're not including tests, the count is what we collapsed.
    let collapsed = if opts.include_tests { 0 } else { test_count };
    (callers, collapsed)
}

fn is_source_ext(path: &str) -> bool {
    matches!(
        path.rsplit('.').next().unwrap_or(""),
        "rs" | "go" | "py" | "ts" | "tsx" | "js" | "jsx" | "mjs"
        | "c" | "cpp" | "cc" | "cxx" | "h" | "hpp"
        | "cs" | "java" | "kt" | "rb" | "swift" | "zig" | "ex" | "exs"
        | "lua" | "php" | "dart" | "scala" | "clj" | "hs" | "ml" | "fs"
    )
}

fn is_definition_line(line: &str) -> bool {
    let t = line.trim_start_matches("pub ")
        .trim_start_matches("pub(crate) ")
        .trim_start_matches("pub(super) ")
        .trim_start_matches("async ")
        .trim_start_matches("#[")
        .trim_start_matches("///");
    t.starts_with("fn ")
        || t.starts_with("def ")
        || t.starts_with("struct ")
        || t.starts_with("class ")
        || t.starts_with("func ")
        || t.starts_with("type ")
        || t.starts_with("enum ")
        || t.starts_with("trait ")
        || t.starts_with("interface ")
}

fn is_import_line(line: &str) -> bool {
    let t = line.trim();
    t.starts_with("use ")
        || t.starts_with("import ")
        || t.starts_with("from ")
        || t.starts_with("require(")
        || t.starts_with("mod ")
}

fn is_comment_line(line: &str) -> bool {
    let t = line.trim();
    t.starts_with("//") || t.starts_with('#') || t.starts_with("/*") || t.starts_with('*')
}

pub fn is_test_path_pub(path: &str) -> bool {
    is_test_path(path)
}

fn is_test_path(path: &str) -> bool {
    let lower = path.to_lowercase();
    // "/tests/foo" or "tests/foo" at the top level
    lower.contains("/tests/") || lower.starts_with("tests/")
    // "src/auth_test.rs" or "auth_test"
    || lower.contains("_test.") || lower.ends_with("_test")
    // "src/spec/foo.ts" or "foo_spec.ts"
    || lower.contains("/spec") || lower.contains("_spec.")
    // "src/test/foo" — single "test" directory component with surrounding slashes
    || lower.contains("/test/")
}

/// Heuristic: does this call-site snippet look like it's inside a test?
/// Catches `mod tests` blocks in source files that don't have `_test` in the path.
fn snippet_looks_like_test(snippet: &str) -> bool {
    // Test snippets almost always contain assert macros or expect calls.
    let s = snippet;
    s.contains("assert!(") || s.contains("assert_eq!(") || s.contains("assert_ne!(")
        || s.contains("assert!(") || s.contains(".unwrap().unwrap()")
        || (s.contains(".unwrap()") && s.trim_start().starts_with("let cg ="))
        || s.contains("should_panic") || s.contains("proptest!")
}

/// Check if a call site in `file` at `line` falls inside one of the file's
/// known test functions. We use `inline_test_fns` names from the mapped skeleton
/// to find test function signatures and estimate their line ranges.
fn same_file_is_test_fn(mapped: &[MappedFile], file: &str, line: usize) -> bool {
    let mf = match mapped.iter().find(|m| m.path == file) {
        Some(m) => m,
        None => return false,
    };
    if mf.inline_test_fns.is_empty() {
        return false;
    }
    // Find signatures of test functions and check if our line falls after
    // the last test fn's line_start. Test functions are at the end of the file
    // and clustered together, so the earliest test fn start is a reasonable boundary.
    let test_start = mf
        .signatures
        .iter()
        .filter(|s| {
            s.symbol_name
                .as_deref()
                .map(|n| mf.inline_test_fns.iter().any(|t| t == n))
                .unwrap_or(false)
        })
        .map(|s| s.line_start)
        .min();

    match test_start {
        Some(start) => line > start,
        None => false,
    }
}

fn classify_caller_path(path: &str) -> Option<CallerTag> {
    let lower = path.to_lowercase();
    if lower.contains("middleware") {
        Some(CallerTag::Middleware)
    } else if lower.contains("handler") || lower.contains("route") || lower.contains("router") || lower.contains("endpoint") {
        Some(CallerTag::Handler)
    } else if lower.ends_with("main.rs") || lower.ends_with("main.go") || lower.ends_with("main.py")
        || lower.ends_with("mod.rs") || lower.ends_with("index.ts") || lower.ends_with("index.js")
    {
        Some(CallerTag::Entry)
    } else {
        None
    }
}

// ---------------------------------------------------------------------------
// Callee discovery
// ---------------------------------------------------------------------------

fn find_callees(
    root_path: &Path,
    mapped: &[MappedFile],
    root: &RootSymbol,
    _opts: &ReachOptions,
) -> (Vec<CalleeInfo>, Option<String>) {
    // Try call graph for Rust / Python.
    let abs_file = root_path.join(&root.file);
    let source = match std::fs::read_to_string(&abs_file) {
        Ok(s) => s,
        Err(_) => return (vec![], None),
    };

    let cg = match build_file_call_graph(&abs_file, &source) {
        Ok(Some(cg)) => cg,
        Ok(None) => {
            // Language not supported by call graph — return heuristic note.
            return (vec![], Some(format!(
                "call graph unavailable for this language — callee list is heuristic"
            )));
        }
        Err(e) => return (vec![], Some(format!("call graph error: {e}"))),
    };

    // Find the root function in the call graph.
    let root_qualified = cg
        .functions
        .iter()
        .find(|f| {
            f.simple == root.name
                || f.qualified == root.name
                || f.qualified.ends_with(&format!("::{}", root.name))
                || f.qualified.ends_with(&format!(".{}", root.name))
        })
        .map(|f| f.qualified.clone());

    let root_qual = match root_qualified {
        Some(q) => q,
        None => {
            // Function in a language we support but wasn't found in the call graph
            // (e.g. it's a trait impl or a #[no_mangle] fn). Fall through gracefully.
            return (vec![], None);
        }
    };

    // Collect resolved callee names (in-file calls).
    let in_file_callees: Vec<String> = cg
        .calls
        .iter()
        .filter(|(caller, _)| *caller == root_qual)
        .map(|(_, callee)| callee.clone())
        .collect();

    // Also collect unresolved (cross-file) callees.
    let cross_file_callees: Vec<String> = cg
        .unresolved_calls
        .iter()
        .filter(|(caller, _)| *caller == root_qual)
        .map(|(_, raw)| raw.clone())
        .collect();

    let mut callees: Vec<CalleeInfo> = vec![];
    let mut seen_callees: std::collections::HashSet<String> = std::collections::HashSet::new();

    // Resolve in-file callees against the call graph's function list.
    for callee_name in &in_file_callees {
        if let Some(fi) = cg.functions.iter().find(|f| &f.qualified == callee_name) {
            if !seen_callees.insert(fi.simple.clone()) {
                continue; // duplicate call — same function called more than once
            }
            // Prefer the mapper skeleton signature; fall back to reading the source line.
            let sig_str = lookup_sig(mapped, &root.file, fi.simple.as_str())
                .map(|s| s.raw.clone())
                .or_else(|| read_sig_line(&source, fi.line as usize))
                .unwrap_or_else(|| format!("fn {}", fi.simple));
            callees.push(CalleeInfo {
                name: fi.simple.clone(),
                file: root.file.clone(),
                line: fi.line,
                sig: sig_str,
            });
        }
    }

    // Resolve cross-file callees — only when we have corroborating import evidence.
    // Without this guard, common method names (extension, as_str, len, new …)
    // match unrelated functions all over the codebase.
    let root_imports = mapped
        .iter()
        .find(|m| m.path == root.file)
        .map(|m| m.imports.as_slice())
        .unwrap_or(&[]);

    for raw_name in &cross_file_callees {
        // Skip names so generic they'll produce false positives in every project.
        if is_too_generic(raw_name) {
            continue;
        }
        if let Some((mf, sig)) = lookup_sig_any(mapped, raw_name) {
            // Only include when the root file plausibly imports the callee's module.
            // We check if any import path mentions the callee file's stem or directory.
            if !imports_file(root_imports, &mf.path) {
                continue;
            }
            if !seen_callees.insert(raw_name.clone()) {
                continue;
            }
            callees.push(CalleeInfo {
                name: raw_name.clone(),
                file: mf.path.clone(),
                line: sig.line_start as u32 + 1,
                sig: sig.raw.clone(),
            });
        }
    }

    (callees, None)
}

// ---------------------------------------------------------------------------
// Depth-2 neighbors
// ---------------------------------------------------------------------------

/// Extract PascalCase type names referenced in a signature string.
/// These are the types that an AI actually needs to understand the callee's contract.
fn sig_type_names(sig: &str) -> Vec<String> {
    // Match PascalCase identifiers: start uppercase, contain at least one more char.
    // Excludes all-caps acronyms shorter than 3 chars (Err, Ok, IO noise).
    let re = regex::Regex::new(r"\b([A-Z][a-zA-Z0-9]{2,})\b").unwrap();
    let mut names: Vec<String> = re
        .captures_iter(sig)
        .map(|c| c[1].to_string())
        // Skip ubiquitous Rust/stdlib types that aren't project-specific.
        .filter(|n| !matches!(
            n.as_str(),
            "Option" | "Result" | "Vec" | "Box" | "Arc" | "Rc" | "Mutex"
            | "HashMap" | "HashSet" | "BTreeMap" | "BTreeSet" | "String"
            | "PathBuf" | "OsString" | "None" | "Some" | "Ok" | "Err"
            | "True" | "False" | "Self" | "Debug" | "Clone" | "Default"
            | "Send" | "Sync" | "Sized" | "Copy" | "Display" | "Error"
        ))
        .collect();
    names.dedup();
    names
}

fn find_depth2(mapped: &[MappedFile], callees: &[CalleeInfo]) -> Vec<NeighborInfo> {
    let mut neighbors: Vec<NeighborInfo> = vec![];
    let mut seen: std::collections::HashSet<String> = std::collections::HashSet::new();

    for callee in callees {
        // Extract the type names referenced in this callee's signature.
        let type_names = sig_type_names(&callee.sig);

        for type_name in &type_names {
            if seen.contains(type_name) {
                continue;
            }
            // Find a definition for this type anywhere in the mapped files.
            if let Some((mf, sig)) = lookup_sig_any(mapped, type_name) {
                seen.insert(type_name.clone());
                neighbors.push(NeighborInfo {
                    name: type_name.clone(),
                    file: mf.path.clone(),
                    line: sig.line_start as u32 + 1,
                    sig: sig.raw.clone(),
                });
                if neighbors.len() >= 10 {
                    return neighbors;
                }
            }
        }
    }

    neighbors
}

/// Depth-2 for a struct/enum root: find the types of its fields from the body.
/// This surfaces the types an AI needs to understand the data shape — more
/// useful than callee types when the root is a type, not a function.
fn find_depth2_from_type(mapped: &[MappedFile], root: &RootSymbol) -> Vec<NeighborInfo> {
    // The struct body is stored in Signature.body as a compact field list,
    // e.g. "pub callers: Vec<CallerInfo>; pub depth2: Vec<NeighborInfo>; …"
    // We also have the raw signature line and file for additional context.
    let sig = mapped
        .iter()
        .find(|m| m.path == root.file)
        .and_then(|m| {
            m.signatures.iter().find(|s| {
                s.symbol_name.as_deref() == Some(&root.name)
                    || s.qualified_name.as_deref() == Some(&root.name)
            })
        });

    let body_text = sig
        .and_then(|s| s.body.as_deref())
        .unwrap_or("");

    // Combine body + raw sig to extract type names.
    let combined = format!("{} {}", root.sig, body_text);
    let type_names = sig_type_names(&combined);

    let mut neighbors: Vec<NeighborInfo> = vec![];
    let mut seen: std::collections::HashSet<String> = std::collections::HashSet::new();
    seen.insert(root.name.clone()); // don't self-reference

    for type_name in type_names {
        if seen.contains(&type_name) { continue; }
        if let Some((mf, field_sig)) = lookup_sig_any(mapped, &type_name) {
            seen.insert(type_name.clone());
            neighbors.push(NeighborInfo {
                name: type_name,
                file: mf.path.clone(),
                line: field_sig.line_start as u32 + 1,
                sig: field_sig.raw.clone(),
            });
            if neighbors.len() >= 8 { break; }
        }
    }

    neighbors
}

// ---------------------------------------------------------------------------
// Signature lookup helpers
// ---------------------------------------------------------------------------

/// Returns true if the import list of `root_file` plausibly references `callee_file`.
/// Checks: module stem match, parent directory match, or any import path suffix match.
fn imports_file(imports: &[String], callee_file: &str) -> bool {
    // Derive a module stem from the callee file path:
    //   "src/crypto.rs" → "crypto"
    //   "src/auth/jwt.rs" → "jwt" and "auth"
    let parts: Vec<&str> = callee_file
        .trim_end_matches(".rs")
        .trim_end_matches(".py")
        .trim_end_matches(".go")
        .split('/')
        .collect();

    for imp in imports {
        let imp_lower = imp.to_lowercase();
        for part in &parts {
            if part.len() >= 3 && imp_lower.contains(part) {
                return true;
            }
        }
    }
    false
}

/// Returns true if `name` is so common it will produce false positives across
/// almost any codebase when used as a simple-name callee lookup.
fn is_too_generic(name: &str) -> bool {
    matches!(
        name,
        "new" | "clone" | "default" | "from" | "into" | "as_str" | "as_ref"
        | "to_string" | "to_owned" | "unwrap" | "expect" | "ok" | "err"
        | "len" | "is_empty" | "push" | "pop" | "get" | "set" | "insert"
        | "remove" | "contains" | "iter" | "map" | "filter" | "collect"
        | "extension" | "parent" | "exists" | "join" | "display" | "write"
        | "read" | "open" | "close" | "format" | "parse" | "split" | "trim"
        | "lock" | "unlock" | "send" | "recv" | "next" | "yield" | "drop"
    )
}

/// Read the body of the root symbol from source, starting after its signature.
fn read_root_body(root_path: &Path, root: &RootSymbol, max_lines: usize) -> Option<String> {
    let abs = root_path.join(&root.file);
    let source = std::fs::read_to_string(&abs).ok()?;
    let lines: Vec<&str> = source.lines().collect();
    let start = root.line as usize; // root.line is 1-based; skip the sig line itself

    // Find the opening brace.
    let mut brace_line = start.saturating_sub(1);
    for (i, line) in lines[brace_line..].iter().enumerate() {
        if line.contains('{') {
            brace_line = brace_line + i;
            break;
        }
        if i > 8 { return None; }
    }

    let mut depth = 0i32;
    let mut body: Vec<&str> = vec![];
    let mut in_body = false;

    for line in &lines[brace_line..] {
        for ch in line.chars() {
            if ch == '{' { depth += 1; in_body = true; }
            else if ch == '}' { depth -= 1; }
        }
        if in_body { body.push(line); }
        if in_body && depth == 0 { break; }
        if body.len() >= max_lines {
            body.push("    // … (truncated)");
            break;
        }
    }

    if body.is_empty() { None } else { Some(body.join("\n")) }
}

/// Read the function signature starting at `line` (1-based) from source text.
/// Collects lines until we see `{` (body start) or a blank line, normalizing
/// whitespace so multi-line signatures are compacted to a single line.
fn read_sig_line(source: &str, line: usize) -> Option<String> {
    if line == 0 {
        return None;
    }
    let lines: Vec<&str> = source.lines().collect();
    let start = line.saturating_sub(1); // convert to 0-based
    if start >= lines.len() {
        return None;
    }

    let mut parts: Vec<&str> = vec![];
    for l in &lines[start..] {
        let trimmed = l.trim();
        if trimmed.is_empty() && !parts.is_empty() {
            break;
        }
        parts.push(trimmed);
        // Stop when we reach the opening brace of the body.
        if trimmed.ends_with('{') || trimmed.contains("{ }") {
            break;
        }
        // Stop at a standalone arrow or where ) ends the sig cleanly.
        if parts.len() >= 8 {
            break; // safety: don't collect the whole file
        }
    }

    if parts.is_empty() {
        return None;
    }

    // Join and strip the trailing { if present.
    let joined = parts.join(" ");
    let sig = joined.trim_end_matches('{').trim_end_matches("{ }").trim();

    // Strip everything after the return type's opening brace to avoid body leakage.
    Some(sig.to_string())
}

fn lookup_sig<'a>(mapped: &'a [MappedFile], file: &str, name: &str) -> Option<&'a Signature> {
    let mf = mapped.iter().find(|m| m.path == file)?;
    mf.signatures.iter().find(|s| {
        s.symbol_name.as_deref() == Some(name) || s.qualified_name.as_deref() == Some(name)
    })
}

fn lookup_sig_any<'a>(mapped: &'a [MappedFile], name: &str) -> Option<(&'a MappedFile, &'a Signature)> {
    for mf in mapped {
        for sig in &mf.signatures {
            if sig.symbol_name.as_deref() == Some(name)
                || sig.qualified_name.as_deref() == Some(name)
                || sig
                    .qualified_name
                    .as_deref()
                    .map(|q| q.ends_with(&format!("::{name}")) || q.ends_with(&format!(".{name}")))
                    .unwrap_or(false)
            {
                return Some((mf, sig));
            }
        }
    }
    None
}

// ---------------------------------------------------------------------------
// Budget trimming
// ---------------------------------------------------------------------------

fn trim_to_budget(result: &mut ReachResult, budget: usize) {
    // Trim depth-2 first (lowest priority).
    while !result.depth2.is_empty() {
        result.depth2.pop();
        let t = estimate_tokens(&render_reach(result));
        if t <= budget {
            return;
        }
    }

    // Then trim callees from the end.
    while result.callees.len() > 1 {
        result.callees.pop();
        let t = estimate_tokens(&render_reach(result));
        if t <= budget {
            return;
        }
    }

    // Then trim callers from the end.
    while result.callers.len() > 1 {
        result.callers.pop();
        let t = estimate_tokens(&render_reach(result));
        if t <= budget {
            return;
        }
    }
}

// ---------------------------------------------------------------------------
// Renderer — compact indented-tree notation
// ---------------------------------------------------------------------------

/// Render a `ReachResult` as the compact AI-native tree notation.
///
/// Target: ~40% of equivalent JSON token cost for the same information.
pub fn render_reach(result: &ReachResult) -> String {
    let mut out = String::with_capacity(2048);

    let kind_label = kind_str(result.root.kind);
    let vis_label = match result.root.visibility {
        Visibility::Public => "  pub",
        Visibility::Crate => "  pub(crate)",
        Visibility::Private => "",
    };

    let short_file = short_path(&result.root.file);

    // Header line
    out.push_str(&format!(
        "── {}  {}  {}:{}{}\n",
        result.root.name, kind_label, short_file, result.root.line, vis_label
    ));

    // Signature
    out.push_str(&format!("   sig  {}\n", result.root.sig.trim()));

    // Body (optional)
    if let Some(ref body) = result.root_body {
        out.push_str("   body\n");
        for line in body.lines() {
            out.push_str(&format!("   {}\n", line));
        }
    }

    // Callers
    let prod_count = result.callers.iter().filter(|c| !c.is_test).count();
    let test_total = result.test_callers_collapsed
        + result.callers.iter().filter(|c| c.is_test).count();

    if prod_count > 0 || test_total > 0 {
        let test_part = if test_total > 0 {
            format!(" · {} test", test_total)
        } else {
            String::new()
        };
        out.push_str(&format!("   callers  {} prod{}\n", prod_count, test_part));

        // Compute column widths for alignment.
        let max_file_len = result
            .callers
            .iter()
            .filter(|c| !c.is_test || result.callers.iter().any(|x| x.is_test))
            .map(|c| format!("{}:{}", short_path(&c.file), c.line).len())
            .max()
            .unwrap_or(0);

        for caller in &result.callers {
            let loc = format!("{}:{}", short_path(&caller.file), caller.line);
            let padding = " ".repeat(max_file_len.saturating_sub(loc.len()));
            let tag_str = caller
                .tag
                .as_ref()
                .map(|t| format!("[{}]  ", t.label()))
                .unwrap_or_else(|| "  ".to_string());
            let snip = truncate_snippet(&caller.snippet, 72);
            out.push_str(&format!(
                "     {}{}  {}{}\n",
                loc, padding, tag_str, snip
            ));
        }

        if result.test_callers_collapsed > 0 {
            out.push_str(&format!(
                "     [{} test caller{} — use --include-tests to expand]\n",
                result.test_callers_collapsed,
                if result.test_callers_collapsed == 1 { "" } else { "s" }
            ));
        }
    } else {
        out.push_str("   callers  none found\n");
    }

    // Callees
    if !result.callees.is_empty() {
        out.push_str("   callees\n");
        let max_name = result.callees.iter().map(|c| c.name.len()).max().unwrap_or(0);
        let max_loc = result
            .callees
            .iter()
            .map(|c| format!("{}:{}", short_path(&c.file), c.line).len())
            .max()
            .unwrap_or(0);

        for callee in &result.callees {
            let name_pad = " ".repeat(max_name.saturating_sub(callee.name.len()));
            let loc = format!("{}:{}", short_path(&callee.file), callee.line);
            let loc_pad = " ".repeat(max_loc.saturating_sub(loc.len()));
            let sig = truncate_snippet(callee.sig.trim(), 80);
            out.push_str(&format!(
                "     {}{}  {}{}  {}\n",
                callee.name, name_pad, loc, loc_pad, sig
            ));
        }
    }

    // Depth-2 neighbors
    if !result.depth2.is_empty() {
        out.push_str("   depth-2  [sig only]\n");
        for n in &result.depth2 {
            let loc = format!("{}:{}", short_path(&n.file), n.line);
            let sig = truncate_snippet(n.sig.trim(), 80);
            out.push_str(&format!("     {}  {}  {}\n", n.name, loc, sig));
        }
    }

    // Language note (heuristic warning)
    if let Some(ref note) = result.language_note {
        out.push_str(&format!("   note  {}\n", note));
    }

    // Budget / token metadata
    if result.budget_hit {
        out.push_str(&format!(
            "   [budget hit — trimmed to {} tokens]\n",
            result.tokens_used
        ));
    }

    out
}

/// Render a unified context tree from two or more reach results.
///
/// Callers are merged (deduped by file:line). Callees are merged (deduped by
/// name; those appearing in multiple roots are annotated `[shared]`). Depth-2
/// types that appear in more than one result are promoted to a "shared types"
/// section so they don't get buried.
pub fn render_multi_reach(results: &[ReachResult]) -> String {
    assert!(!results.is_empty(), "render_multi_reach: empty results slice");

    let mut out = String::with_capacity(4096);

    // Header
    let names: Vec<&str> = results.iter().map(|r| r.root.name.as_str()).collect();
    out.push_str(&format!("── reach: {}\n\n", names.join(" + ")));

    // One header line + signature per root symbol.
    for r in results {
        let kind_label = kind_str(r.root.kind);
        let vis_label = match r.root.visibility {
            Visibility::Public => "  pub",
            Visibility::Crate => "  pub(crate)",
            Visibility::Private => "",
        };
        out.push_str(&format!(
            "── {}  {}  {}:{}{}\n",
            r.root.name, kind_label, short_path(&r.root.file), r.root.line, vis_label
        ));
        out.push_str(&format!("   sig  {}\n", r.root.sig.trim()));
        if let Some(ref body) = r.root_body {
            out.push_str("   body\n");
            for line in body.lines() {
                out.push_str(&format!("   {}\n", line));
            }
        }
    }
    out.push('\n');

    // Merged callers — union deduped by (file, line).
    let mut seen_locs: std::collections::HashSet<(String, usize)> = Default::default();
    let mut all_callers: Vec<&CallerInfo> = Vec::new();
    for r in results {
        for c in &r.callers {
            if seen_locs.insert((c.file.clone(), c.line)) {
                all_callers.push(c);
            }
        }
    }
    let prod_count = all_callers.iter().filter(|c| !c.is_test).count();
    let test_total = all_callers.iter().filter(|c| c.is_test).count()
        + results.iter().map(|r| r.test_callers_collapsed).sum::<usize>();

    if prod_count > 0 || test_total > 0 {
        let test_part = if test_total > 0 { format!(" · {} test", test_total) } else { String::new() };
        out.push_str(&format!("   callers  {} prod{}\n", prod_count, test_part));
        let max_loc_len = all_callers
            .iter()
            .map(|c| format!("{}:{}", short_path(&c.file), c.line).len())
            .max()
            .unwrap_or(0);
        for caller in &all_callers {
            let loc = format!("{}:{}", short_path(&caller.file), caller.line);
            let pad = " ".repeat(max_loc_len.saturating_sub(loc.len()));
            let tag_str = caller
                .tag
                .as_ref()
                .map(|t| format!("[{}]  ", t.label()))
                .unwrap_or_else(|| "  ".to_string());
            let snip = truncate_snippet(&caller.snippet, 72);
            out.push_str(&format!("     {}{}  {}{}\n", loc, pad, tag_str, snip));
        }
    } else {
        out.push_str("   callers  none found\n");
    }

    // Merged callees — union deduped by name; shared ones annotated.
    let callee_hit_count: std::collections::HashMap<&str, usize> = {
        let mut m: std::collections::HashMap<&str, usize> = Default::default();
        for r in results {
            for c in &r.callees {
                *m.entry(c.name.as_str()).or_insert(0) += 1;
            }
        }
        m
    };
    let mut seen_callee_names: std::collections::HashSet<&str> = Default::default();
    let mut all_callees: Vec<&CalleeInfo> = Vec::new();
    for r in results {
        for c in &r.callees {
            if seen_callee_names.insert(c.name.as_str()) {
                all_callees.push(c);
            }
        }
    }
    if !all_callees.is_empty() {
        out.push_str("   callees\n");
        let max_name = all_callees.iter().map(|c| c.name.len()).max().unwrap_or(0);
        let max_loc = all_callees
            .iter()
            .map(|c| format!("{}:{}", short_path(&c.file), c.line).len())
            .max()
            .unwrap_or(0);
        for callee in &all_callees {
            let shared = *callee_hit_count.get(callee.name.as_str()).unwrap_or(&0) >= 2;
            let name_pad = " ".repeat(max_name.saturating_sub(callee.name.len()));
            let loc = format!("{}:{}", short_path(&callee.file), callee.line);
            let loc_pad = " ".repeat(max_loc.saturating_sub(loc.len()));
            let sig = truncate_snippet(callee.sig.trim(), 72);
            let shared_tag = if shared { "  [shared]" } else { "" };
            out.push_str(&format!(
                "     {}{}  {}{}  {}{}\n",
                callee.name, name_pad, loc, loc_pad, sig, shared_tag
            ));
        }
    }

    // Depth-2: collect counts across all results.
    let depth2_hit_count: std::collections::HashMap<&str, usize> = {
        let mut m: std::collections::HashMap<&str, usize> = Default::default();
        for r in results {
            for n in &r.depth2 {
                *m.entry(n.name.as_str()).or_insert(0) += 1;
            }
        }
        m
    };
    let mut seen_depth2: std::collections::HashSet<&str> = Default::default();
    let mut promoted: Vec<&NeighborInfo> = Vec::new();
    let mut ordinary: Vec<&NeighborInfo> = Vec::new();
    for r in results {
        for n in &r.depth2 {
            if seen_depth2.insert(n.name.as_str()) {
                if *depth2_hit_count.get(n.name.as_str()).unwrap_or(&0) >= 2 {
                    promoted.push(n);
                } else {
                    ordinary.push(n);
                }
            }
        }
    }

    if !promoted.is_empty() {
        out.push_str("   shared types  [promoted from depth-2]\n");
        for n in &promoted {
            let loc = format!("{}:{}", short_path(&n.file), n.line);
            let sig = truncate_snippet(n.sig.trim(), 80);
            out.push_str(&format!("     {}  {}  {}\n", n.name, loc, sig));
        }
    }
    if !ordinary.is_empty() {
        out.push_str("   depth-2  [sig only]\n");
        for n in &ordinary {
            let loc = format!("{}:{}", short_path(&n.file), n.line);
            let sig = truncate_snippet(n.sig.trim(), 80);
            out.push_str(&format!("     {}  {}  {}\n", n.name, loc, sig));
        }
    }

    // Language notes
    for r in results {
        if let Some(ref note) = r.language_note {
            out.push_str(&format!("   note  {} ({})\n", note, r.root.name));
        }
    }

    let total: usize = results.iter().map(|r| r.tokens_used).sum();
    out.push_str(&format!("\n[{} tokens combined]\n", total));

    out
}

fn kind_str(kind: SymbolKind) -> &'static str {
    match kind {
        SymbolKind::Function | SymbolKind::Method | SymbolKind::Constructor => "fn",
        SymbolKind::Struct => "struct",
        SymbolKind::Class => "class",
        SymbolKind::Interface => "interface",
        SymbolKind::Enum => "enum",
        SymbolKind::TypeAlias => "type",
        SymbolKind::Macro => "macro",
        _ => "sym",
    }
}

fn short_path(path: &str) -> &str {
    // Return the last two path components to keep lines readable.
    // "src/api/routes.rs" → "api/routes.rs"
    // "src/routes.rs"     → "src/routes.rs"  (already 2 components)
    let last_slash = match path.rfind('/') {
        None => return path,
        Some(i) => i,
    };
    match path[..last_slash].rfind('/') {
        None | Some(0) => path,
        Some(second_last) => &path[second_last + 1..],
    }
}

fn truncate_snippet(s: &str, max_chars: usize) -> &str {
    if s.len() <= max_chars {
        s
    } else {
        // Find a char boundary near max_chars
        let mut end = max_chars;
        while !s.is_char_boundary(end) {
            end -= 1;
        }
        &s[..end]
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use crate::mapper::{MappedFile, Signature, SymbolKind};

    fn make_sig(name: &str, kind: SymbolKind, raw: &str, line: usize) -> Signature {
        Signature {
            raw: raw.to_string(),
            ckb_id: None,
            symbol_name: Some(name.to_string()),
            qualified_name: Some(name.to_string()),
            kind,
            line_start: line,
            col_start: 0,
            line_end: line,
            col_end: raw.len(),
            confidence: 30,
            doc_comment: None,
            body: None,
            tested: false,
        }
    }

    fn make_file(path: &str, sigs: Vec<Signature>) -> MappedFile {
        MappedFile {
            path: path.to_string(),
            imports: vec![],
            signatures: sigs,
            docstrings: None,
            parameters: None,
            return_types: None,
            churn_label: None,
            inline_test_fns: vec![],
        }
    }

    #[test]
    fn resolve_exact_name_match() {
        let mapped = vec![make_file(
            "src/auth.rs",
            vec![make_sig(
                "verify_token",
                SymbolKind::Function,
                "pub fn verify_token(token: &str) -> Result<Claims>",
                111,
            )],
        )];
        let root = resolve_symbol(&mapped, "verify_token", None).unwrap();
        assert_eq!(root.name, "verify_token");
        assert_eq!(root.file, "src/auth.rs");
        assert_eq!(root.line, 112);
        assert_eq!(root.visibility, Visibility::Public);
    }

    #[test]
    fn resolve_ambiguous_returns_error() {
        let mapped = vec![
            make_file(
                "src/auth.rs",
                vec![make_sig("verify", SymbolKind::Function, "pub fn verify()", 1)],
            ),
            make_file(
                "src/other.rs",
                vec![make_sig("verify", SymbolKind::Function, "pub fn verify()", 1)],
            ),
        ];
        let err = resolve_symbol(&mapped, "verify", None).unwrap_err();
        assert!(matches!(err, ReachError::Ambiguous(_)));
    }

    #[test]
    fn resolve_not_found() {
        let mapped = vec![make_file(
            "src/auth.rs",
            vec![make_sig("verify", SymbolKind::Function, "fn verify()", 1)],
        )];
        let err = resolve_symbol(&mapped, "nonexistent", None).unwrap_err();
        assert!(matches!(err, ReachError::NotFound(_)));
    }

    #[test]
    fn resolve_file_filter_narrows_ambiguous() {
        let mapped = vec![
            make_file(
                "src/auth.rs",
                vec![make_sig("check", SymbolKind::Function, "fn check()", 5)],
            ),
            make_file(
                "src/rate.rs",
                vec![make_sig("check", SymbolKind::Function, "fn check()", 10)],
            ),
        ];
        let root = resolve_symbol(&mapped, "check", Some("auth")).unwrap();
        assert_eq!(root.file, "src/auth.rs");
    }

    #[test]
    fn is_test_path_detects_test_files() {
        assert!(is_test_path("src/auth_test.rs"));
        assert!(is_test_path("tests/auth.rs"));
        assert!(is_test_path("src/tests/auth.rs"));
        assert!(!is_test_path("src/auth.rs"));
        assert!(!is_test_path("src/attestation.rs")); // "test" as substring of word — should be false
    }

    #[test]
    fn classify_caller_path_tags_middleware() {
        assert_eq!(classify_caller_path("src/middleware/auth.rs"), Some(CallerTag::Middleware));
        assert_eq!(classify_caller_path("src/api/routes.rs"), Some(CallerTag::Handler));
        assert_eq!(classify_caller_path("src/main.rs"), Some(CallerTag::Entry));
        assert_eq!(classify_caller_path("src/utils.rs"), None);
    }

    #[test]
    fn renderer_produces_nonempty_output() {
        let result = ReachResult {
            root: RootSymbol {
                file: "src/auth.rs".into(),
                line: 42,
                kind: SymbolKind::Function,
                sig: "pub fn verify_token(token: &str) -> Result<Claims>".into(),
                name: "verify_token".into(),
                visibility: Visibility::Public,
            },
            callers: vec![CallerInfo {
                file: "src/api/routes.rs".into(),
                line: 15,
                snippet: "let claims = verify_token(&tok)?;".into(),
                tag: Some(CallerTag::Handler),
                is_test: false,
            }],
            test_callers_collapsed: 2,
            callees: vec![CalleeInfo {
                name: "decode_jwt".into(),
                file: "src/crypto.rs".into(),
                line: 8,
                sig: "fn decode_jwt(token: &str) -> Result<Payload>".into(),
            }],
            depth2: vec![NeighborInfo {
                name: "Claims".into(),
                file: "src/types.rs".into(),
                line: 14,
                sig: "struct Claims { sub: UserId, exp: u64 }".into(),
            }],
            language_note: None,
            root_body: None,
            tokens_used: 0,
            budget_hit: false,
        };

        let rendered = render_reach(&result);
        assert!(rendered.contains("verify_token"));
        assert!(rendered.contains("fn"));
        assert!(rendered.contains("auth.rs:42"));
        assert!(rendered.contains("[handler]"));
        assert!(rendered.contains("decode_jwt"));
        assert!(rendered.contains("depth-2"));
        assert!(rendered.contains("Claims"));
        assert!(rendered.contains("2 test caller")); // collapsed test callers
    }

    #[test]
    fn short_path_returns_two_components() {
        assert_eq!(short_path("src/api/routes.rs"), "api/routes.rs");
        assert_eq!(short_path("routes.rs"), "routes.rs");
        assert_eq!(short_path("src/routes.rs"), "src/routes.rs");
    }
}
