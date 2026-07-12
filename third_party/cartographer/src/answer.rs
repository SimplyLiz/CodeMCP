//! `answer` — question-driven evidence chain for AI context injection.
//!
//! Given a natural-language question, assembles the minimum set of semantic
//! units that together answer it, ordered so they read like an explanation:
//! entry points first, then types, then internals. Each item is numbered and
//! annotated with its connection to adjacent items (`[calls #2]`, `[type used by #1]`).
//!
//! Pipeline:
//!   1. BM25 search across all files → ranked candidate files
//!   2. For each candidate file, score its public symbols against query terms
//!   3. Select the top-scoring symbols across all files, capped by item budget
//!   4. Order: structs/types before functions, entry points before internals
//!   5. Annotate inter-item connections via import and call-graph edges
//!   6. Decide body vs sig-only per item: show body for the single "core logic" item
//!   7. Render as numbered evidence chain

use std::collections::HashMap;
use std::path::Path;

use crate::formatter::estimate_tokens;
use regex::Regex;
use crate::mapper::{MappedFile, Signature, SymbolKind};
use crate::search::{bm25_search, bm25_search_symbols, BM25Options};

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

#[derive(Debug, Clone)]
pub struct AnswerOptions {
    /// Maximum tokens in the output. Default 8000.
    pub budget: usize,
    /// Maximum evidence items. Default 6.
    pub max_items: usize,
    /// Show body for the top-scoring item. Default true.
    pub show_top_body: bool,
}

impl Default for AnswerOptions {
    fn default() -> Self {
        Self {
            budget: 8000,
            max_items: 6,
            show_top_body: true,
        }
    }
}

#[derive(Debug, Clone)]
pub struct EvidenceItem {
    /// 1-based index in the final chain.
    pub index: usize,
    pub name: String,
    pub file: String,
    pub line: u32,
    pub kind: SymbolKind,
    pub sig: String,
    /// Function body — only populated for the "core logic" item.
    pub body: Option<String>,
    /// How this item relates to adjacent items in the chain.
    pub connection: Option<String>,
    /// Role label: "entry point", "core logic", "type", "caller", etc.
    pub role_note: Option<String>,
}

#[derive(Debug)]
pub struct AnswerResult {
    pub query: String,
    pub items: Vec<EvidenceItem>,
    pub tokens_used: usize,
    pub budget_hit: bool,
    /// Files searched during BM25 phase.
    pub files_searched: usize,
}

// ---------------------------------------------------------------------------
// build_answer
// ---------------------------------------------------------------------------

pub fn build_answer(
    root_path: &Path,
    mapped: &[MappedFile],
    query: &str,
    opts: &AnswerOptions,
) -> AnswerResult {
    // --- 1. BM25 search ---
    // Rank over the SYMBOL corpus (names + signatures + doc-comments) first so the answer
    // is anchored on code intent; fall back to raw-content BM25 only if it finds nothing.
    let bm25_opts = BM25Options {
        max_results: 30,
        ..Default::default()
    };
    let sym = bm25_search_symbols(
        mapped.iter().map(|mf| (mf.path.as_str(), mf)),
        query,
        &bm25_opts,
    );
    let bm25 = if !sym.matches.is_empty() {
        sym
    } else {
        match bm25_search(root_path, query, &bm25_opts) {
            Ok(r) => r,
            Err(_) => {
                return AnswerResult {
                    query: query.to_string(),
                    items: vec![],
                    tokens_used: 0,
                    budget_hit: false,
                    files_searched: 0,
                };
            }
        }
    };

    // Unique file count from BM25 results.
    let files_searched: usize = {
        let mut seen = std::collections::HashSet::new();
        for m in &bm25.matches { seen.insert(m.path.clone()); }
        seen.len()
    };

    // Extract query terms upfront — needed for both file selection and symbol scoring.
    // Extract query terms and PascalCase symbol names upfront (used in file selection too).
    let query_terms = tokenize_query(query);
    let pascal_terms: Vec<String> = query
        .split(|c: char| !c.is_alphanumeric())
        .filter(|t| {
            t.len() >= 4
                && t.chars().next().map(|c| c.is_uppercase()).unwrap_or(false)
                && t.chars().any(|c| c.is_lowercase())
        })
        .map(|t| t.to_string())
        .collect();

    // Collect unique candidate files ranked by BM25 score.
    // Normalize by match count so large files don't dominate small ones.
    let mut file_matches: HashMap<String, usize> = HashMap::new();
    let mut file_total_chars: HashMap<String, usize> = HashMap::new();
    for m in &bm25.matches {
        *file_matches.entry(m.path.clone()).or_default() += 1;
    }
    for (path, _) in &file_matches {
        let chars = mapped
            .iter()
            .find(|mf| &mf.path == path)
            .map(|mf| mf.signatures.iter().map(|s| s.raw.len()).sum::<usize>())
            .unwrap_or(1000);
        file_total_chars.insert(path.clone(), chars.max(1));
    }

    // Score every candidate file in a single map so name-matched code files reliably
    // outrank doc coincidences whether or not BM25 already surfaced them.
    let mut scores: HashMap<String, f64> = HashMap::new();
    for (path, &hits) in &file_matches {
        // Normalise hits by estimated file density so large files don't win by volume.
        let mut density = hits as f64 / (file_total_chars[path] as f64).sqrt();
        // Code bias: "how does X work" is answered by code, but docs win BM25 on
        // conceptual terms (a design doc mentions "churn" far more than git_churn's
        // body does). Down-weight docs so the implementation surfaces first.
        if crate::api::is_doc_path(path) {
            density *= 0.3;
        }
        scores.insert(path.clone(), density);
    }

    // Name-match boost across ALL files. BM25 IDF-penalises terms common to many files
    // (e.g. "reach" in a repo named "reach") and misses vocabulary matches (a term
    // `churn` vs symbol `git_churn`). Any file matching by filename stem, PascalCase
    // symbol, or symbol-name substring is floored to a strong score — above doc noise.
    for mf in mapped {
        let stem = mf.path
            .rsplit('/')
            .next()
            .unwrap_or("")
            .trim_end_matches(".rs")
            .trim_end_matches(".py")
            .trim_end_matches(".go")
            .trim_end_matches(".ts")
            .trim_end_matches(".js")
            .to_lowercase();
        let matches_filename = query_terms.iter().any(|t| {
            t.len() >= 4 && (stem.contains(t.as_str()) || t.contains(stem.as_str()))
        });
        let matches_pascal = pascal_terms.iter().any(|pt| {
            mf.signatures.iter().any(|s| {
                s.symbol_name.as_deref() == Some(pt.as_str())
                    || s.qualified_name.as_deref() == Some(pt.as_str())
            })
        });
        let matches_symbol = mf.signatures.iter().any(|s| {
            let nm = s.symbol_name.as_deref().unwrap_or("").to_lowercase();
            !nm.is_empty()
                && query_terms
                    .iter()
                    .any(|t| t.len() >= 3 && nm.contains(t.as_str()))
        });
        if matches_filename || matches_pascal || matches_symbol {
            let floor = if crate::api::is_doc_path(&mf.path) { 0.5 } else { 2.0 };
            let e = scores.entry(mf.path.clone()).or_insert(0.0);
            *e = e.max(floor);
        }
    }

    let mut ranked_files: Vec<(String, f64)> = scores.into_iter().collect();
    ranked_files.sort_by(|a, b| b.1.partial_cmp(&a.1).unwrap_or(std::cmp::Ordering::Equal));
    ranked_files.truncate(12);

    let mut scored: Vec<ScoredSymbol> = vec![];

    for (file, file_score) in &ranked_files {
        let mf = match mapped.iter().find(|m| m.path == *file) {
            Some(m) => m,
            None => continue,
        };

        // Collect public symbols from the skeleton.
        let mut file_sigs: Vec<std::borrow::Cow<Signature>> = mf
            .signatures
            .iter()
            .map(std::borrow::Cow::Borrowed)
            .collect();

        // Supplement with private function signatures for BM25-matched files.
        // The skeleton only contains public symbols; private helper functions
        // (e.g. find_callers, trim_to_budget) are often the real implementation.
        let private_sigs = extract_private_fn_sigs(root_path, file);
        file_sigs.extend(private_sigs.into_iter().map(std::borrow::Cow::Owned));

        for sig in &file_sigs {
            // Skip test functions — they're never the answer to "how does X work".
            if is_test_symbol(sig, mf) {
                continue;
            }
            // Skip bare module declarations — not explanatory.
            if matches!(sig.kind, SymbolKind::Namespace) {
                continue;
            }
            if sig.raw.trim_start().starts_with("mod ") {
                continue;
            }

            let sym_score = score_symbol(sig, &query_terms, &pascal_terms, *file_score);
            if sym_score > 0.0 {
                scored.push(ScoredSymbol {
                    file: file.clone(),
                    sig: sig.as_ref().clone(),
                    score: sym_score,
                });
            }
        }
    }

    // Sort by score descending.
    scored.sort_by(|a, b| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal));
    scored.truncate(opts.max_items * 2); // keep extras for ordering pass

    // --- 3. Order for readability ---
    let file_ages = collect_file_creation_ages(root_path, &scored);
    let ordered = order_for_reading(scored, opts.max_items, &file_ages);

    // --- 4. Build evidence items with connections ---
    let mut items: Vec<EvidenceItem> = build_evidence_items(root_path, mapped, &ordered, opts);

    // --- 5. Apply token budget ---
    let rendered = render_answer(&AnswerResult {
        query: query.to_string(),
        items: items.clone(),
        tokens_used: 0,
        budget_hit: false,
        files_searched,
    });
    let tokens = estimate_tokens(&rendered);

    let budget_hit = tokens > opts.budget;
    if budget_hit {
        // Trim bodies first, then items from the tail.
        for item in &mut items {
            item.body = None;
        }
        while items.len() > 1 {
            let t = estimate_tokens(&render_items(&items, query));
            if t <= opts.budget {
                break;
            }
            items.pop();
        }
    }

    let final_tokens = estimate_tokens(&render_items(&items, query));

    AnswerResult {
        query: query.to_string(),
        items,
        tokens_used: final_tokens,
        budget_hit,
        files_searched,
    }
}

// ---------------------------------------------------------------------------
// Symbol scoring
// ---------------------------------------------------------------------------

struct ScoredSymbol {
    file: String,
    sig: Signature,
    score: f64,
}

const STOP_WORDS: &[&str] = &[
    "how", "does", "what", "where", "when", "who", "why", "which",
    "the", "and", "but", "for", "are", "was", "is", "it", "its",
    "this", "that", "with", "from", "have", "has", "had", "not",
    "can", "will", "should", "would", "could", "may", "get", "do",
    "did", "use", "used", "find", "work", "works", "make", "made",
    "show", "shows", "give", "gives", "call", "calls", "run", "runs",
    "set", "sets", "add", "adds", "all", "any", "each", "into",
];

fn tokenize_query(query: &str) -> Vec<String> {
    let mut terms: Vec<String> = Vec::new();
    for t in query.split(|c: char| !c.is_alphanumeric() && c != '_') {
        let lower = t.to_lowercase();
        if lower.len() >= 3 && !STOP_WORDS.contains(&lower.as_str()) {
            terms.push(lower.clone());
        }
        // Also emit identifier subwords so a camelCase/snake_case term in the
        // question (e.g. `getUserById`, `git_churn`) matches individual symbols.
        for part in lower.split('_') {
            for sub in crate::search::split_identifier(part) {
                if sub.len() >= 3 && sub != lower && !STOP_WORDS.contains(&sub.as_str()) {
                    terms.push(sub);
                }
            }
        }
    }
    terms.sort();
    terms.dedup();
    terms
}

fn common_prefix_len(a: &str, b: &str) -> usize {
    a.bytes().zip(b.bytes()).take_while(|(x, y)| x == y).count()
}

/// Cheap inflectional/derivational match: a shared prefix of ≥4 chars with a
/// short divergent inflectional tail (≤2) on each side. Matches approve~approval,
/// execute~executes, confirm~confirmed, proposed~proposal — without matching
/// action~actionable ("able") or compounds like gate~gateway ("way"), which are
/// different words, not inflections.
fn is_inflection(a: &str, b: &str) -> bool {
    if a == b {
        return true;
    }
    let p = common_prefix_len(a, b);
    p >= 4 && a.len().saturating_sub(p) <= 2 && b.len().saturating_sub(p) <= 2
}

fn score_symbol(
    sig: &Signature,
    query_terms: &[String],
    pascal_terms: &[String],
    file_score: f64,
) -> f64 {
    let sym_name = sig.symbol_name.as_deref().unwrap_or("");
    let name = sym_name.to_lowercase();
    let raw = sig.raw.to_lowercase();
    let doc = sig.doc_comment.as_deref().unwrap_or("").to_lowercase();

    let mut sym_score = 0.0f64;

    // Exact PascalCase symbol name match — very strong signal.
    for pt in pascal_terms {
        if sym_name == pt.as_str()
            || sig.qualified_name.as_deref() == Some(pt.as_str())
        {
            sym_score += 15.0;
        }
    }

    let mut name_score = 0.0f64;
    let mut name_term_hits: usize = 0;
    let mut sig_score = 0.0f64;
    let mut doc_score = 0.0f64;

    // Tokenise the symbol name into word tokens (snake_case + camelCase) so a
    // query term scores on a WHOLE name word, not an incidental substring:
    // "action" must not fully match `converse_actionable`, or a coincidental
    // hit outranks the real `Gate::submit` / `approve` / `execute_proposal`.
    let name_tokens: Vec<String> = name
        .split('_')
        .flat_map(|p| crate::search::split_identifier(p))
        .filter(|t| t.len() >= 2)
        .collect();

    for term in query_terms {
        // Name: best match across the symbol's word tokens. Exact word is the
        // strongest signal; an inflection (approve~approval, execute~executes)
        // is a bit weaker; a mere incidental substring is weak and does NOT
        // count as a term hit (so it can't satisfy the private-symbol gate).
        let mut best = 0.0f64;
        for tok in &name_tokens {
            let s = if tok == term {
                3.0
            } else if is_inflection(term, tok) {
                2.0
            } else {
                0.0
            };
            if s > best {
                best = s;
            }
        }
        if best > 0.0 {
            name_score += best;
            name_term_hits += 1;
        } else if name.contains(term.as_str()) {
            name_score += 1.0;
        }
        if raw.contains(term.as_str()) { sig_score += 1.5; }
        if doc.contains(term.as_str()) { doc_score += 0.5; }
    }

    sym_score += name_score + sig_score + doc_score;

    // Gate: require meaningful signal beyond a doc-comment coincidence.
    // A doc-only match (name=0, sig=0, doc>0) is too weak — the symbol is
    // unrelated and the doc comment just happened to use a common word.
    if sym_score == 0.0 || (name_score == 0.0 && sig_score == 0.0) {
        return 0.0;
    }

    // Boost public symbols.
    if sig.raw.trim_start().starts_with("pub") {
        sym_score *= 1.4;
    }

    // Boost functions and types over misc symbols.
    sym_score *= match sig.kind {
        SymbolKind::Function | SymbolKind::Method | SymbolKind::Constructor => 1.2,
        SymbolKind::Struct | SymbolKind::Class | SymbolKind::Interface => 1.1,
        _ => 1.0,
    };

    // Private functions (confidence < 25) must match at least two distinct
    // query terms in their name. A single-term hit (e.g. "graph" in
    // health_graph_at_ref for a "call graph" query) is too weak — it surfaces
    // private helpers from high-BM25 files (main.rs, api.rs) above more
    // relevant public symbols.
    if sig.confidence < 25 && name_term_hits < 2 {
        return 0.0;
    }

    // File-level BM25 relevance as a mild multiplier (not the primary driver).
    sym_score * (1.0 + file_score * 0.05)
}

/// True if this signature is a test function that should not appear in answer chains.
fn is_test_symbol(sig: &Signature, mf: &MappedFile) -> bool {
    let name = sig.symbol_name.as_deref().unwrap_or("");
    // Inline test functions listed in the file's test set.
    if mf.inline_test_fns.iter().any(|t| t == name) {
        return true;
    }
    // Function name patterns common in test modules.
    if name.ends_with("_works") || name.ends_with("_test") || name.ends_with("_spec")
        || name.starts_with("test_") || name.starts_with("check_")
    {
        return true;
    }
    // Test files by path.
    crate::reach::is_test_path_pub(&mf.path)
}

// ---------------------------------------------------------------------------
// File creation age (for companion ordering)
// ---------------------------------------------------------------------------

/// Return the Unix timestamp of the first git commit that added `file`
/// (relative to `root_path`). Returns `None` if git is unavailable or the
/// file has no history.
fn git_file_creation_timestamp(root_path: &Path, file: &str) -> Option<u64> {
    let out = std::process::Command::new("git")
        .args([
            "-C",
            &root_path.to_string_lossy(),
            "log",
            "--follow",
            "--diff-filter=A",
            "--reverse",
            "--format=%ct",
            "--",
            file,
        ])
        .output()
        .ok()?;
    let stdout = std::str::from_utf8(&out.stdout).ok()?;
    stdout.lines().next()?.trim().parse::<u64>().ok()
}

/// Batch-collect creation timestamps for all unique files in `scored`.
/// Files with no git history are absent from the returned map, and the
/// caller treats absent entries as u64::MAX (newest).
fn collect_file_creation_ages(root_path: &Path, scored: &[ScoredSymbol]) -> HashMap<String, u64> {
    let mut unique: Vec<&str> = scored.iter().map(|s| s.file.as_str()).collect();
    unique.sort_unstable();
    unique.dedup();
    unique
        .into_iter()
        .filter_map(|file| {
            git_file_creation_timestamp(root_path, file).map(|ts| (file.to_string(), ts))
        })
        .collect()
}

// ---------------------------------------------------------------------------
// Ordering for readability
// ---------------------------------------------------------------------------

/// Order symbols so types come before functions that use them, and entry
/// points come before internal helpers. The goal is to read like a guided tour.
///
/// `file_ages` maps relative file paths to their git first-commit Unix timestamp.
/// When two functions from different files score within 10% of the top scorer,
/// the one from the older file is ranked first — it's the original implementation.
fn order_for_reading(
    mut scored: Vec<ScoredSymbol>,
    max: usize,
    file_ages: &HashMap<String, u64>,
) -> Vec<ScoredSymbol> {
    // Partition into: types first, then functions/methods, then rest.
    // Within each partition, keep score order.
    let mut types: Vec<ScoredSymbol> = vec![];
    let mut fns: Vec<ScoredSymbol> = vec![];
    let mut rest: Vec<ScoredSymbol> = vec![];

    for s in scored.drain(..) {
        match s.sig.kind {
            SymbolKind::Struct
            | SymbolKind::Class
            | SymbolKind::Interface
            | SymbolKind::Enum
            | SymbolKind::TypeAlias => types.push(s),
            SymbolKind::Function | SymbolKind::Method | SymbolKind::Constructor => fns.push(s),
            _ => rest.push(s),
        }
    }

    // Companion ordering: functions that score within 10% of the top scorer
    // are re-sorted by file creation date so the original implementation
    // appears before companion files added later (e.g. class_graph.rs vs
    // call_graph.rs scoring nearly identically for a "call graph" query).
    // The tail (outside 10%) keeps pure score order.
    if !file_ages.is_empty() {
        let max_score = fns.iter().map(|s| s.score).fold(f64::NEG_INFINITY, f64::max);
        if max_score > 0.0 {
            let threshold = max_score * 0.9;
            // fns is already score-sorted; find where the within-10% group ends.
            let tail_start = fns.iter().position(|s| s.score < threshold);
            let tail = tail_start.map(|i| fns.split_off(i)).unwrap_or_default();
            // Re-sort the within-10% group by age (smaller timestamp = older = first),
            // breaking ties by score descending.
            fns.sort_by(|a, b| {
                let a_age = file_ages.get(&a.file).copied().unwrap_or(u64::MAX);
                let b_age = file_ages.get(&b.file).copied().unwrap_or(u64::MAX);
                a_age.cmp(&b_age)
                    .then_with(|| b.score.partial_cmp(&a.score).unwrap_or(std::cmp::Ordering::Equal))
            });
            fns.extend(tail);
        }
    }

    // Interleave: a type next to the function that introduces it reads better
    // than all types first. Strategy: highest-scored function goes first, then
    // its parameter types, then next function, etc. For simplicity, just
    // sort types by score and interleave every-other.
    let mut result: Vec<ScoredSymbol> = vec![];
    let mut fi = fns.into_iter().peekable();
    let mut ti = types.into_iter().peekable();

    // Lead with the top function (most likely the "core" item).
    if let Some(f) = fi.next() {
        result.push(f);
    }
    // Then alternate type → function → type → function…
    loop {
        let had_type = if let Some(t) = ti.next() {
            result.push(t);
            true
        } else {
            false
        };
        let had_fn = if let Some(f) = fi.next() {
            result.push(f);
            true
        } else {
            false
        };
        if !had_type && !had_fn {
            break;
        }
        if result.len() >= max * 2 {
            break;
        }
    }
    result.extend(rest);
    result.truncate(max);
    result
}

// ---------------------------------------------------------------------------
// Evidence item construction
// ---------------------------------------------------------------------------

fn build_evidence_items(
    root_path: &Path,
    mapped: &[MappedFile],
    ordered: &[ScoredSymbol],
    opts: &AnswerOptions,
) -> Vec<EvidenceItem> {
    let mut items: Vec<EvidenceItem> = ordered
        .iter()
        .enumerate()
        .map(|(i, s)| {
            let role_note = role_note_for(&s.sig, i == 0);
            let body = if i == 0 && opts.show_top_body {
                read_function_body(root_path, &s.file, s.sig.line_start, 30)
            } else {
                None
            };
            EvidenceItem {
                index: i + 1,
                name: s
                    .sig
                    .symbol_name
                    .clone()
                    .or_else(|| s.sig.qualified_name.clone())
                    .unwrap_or_default(),
                file: s.file.clone(),
                line: s.sig.line_start as u32 + 1,
                kind: s.sig.kind,
                sig: s.sig.raw.clone(),
                body,
                connection: None, // filled in next pass
                role_note,
            }
        })
        .collect();

    // Annotate inter-item connections.
    annotate_connections(&mut items, mapped);

    items
}

fn role_note_for(sig: &Signature, is_top: bool) -> Option<String> {
    let raw = sig.raw.trim();
    if is_top {
        return Some("core logic".to_string());
    }
    match sig.kind {
        SymbolKind::Struct | SymbolKind::Class => Some("type".to_string()),
        SymbolKind::Enum => Some("enum".to_string()),
        SymbolKind::Interface => Some("interface/trait".to_string()),
        SymbolKind::Function | SymbolKind::Method => {
            if raw.starts_with("pub") {
                Some("entry point".to_string())
            } else {
                Some("internal".to_string())
            }
        }
        _ => None,
    }
}

/// Extract private (non-pub) top-level function signatures from a source file.
/// These are absent from the skeleton (which only stores public symbols) but are
/// often the real implementation behind a public entry point.
/// Returns lightweight Signature objects with name, raw sig, and line_start.
fn extract_private_fn_sigs(root_path: &Path, file_path: &str) -> Vec<Signature> {
    let abs = root_path.join(file_path);
    let source = match std::fs::read_to_string(&abs) {
        Ok(s) => s,
        Err(_) => return vec![],
    };

    // Regex to match private function declarations.
    // Matches lines that start a fn but NOT pub/pub(crate)/pub(super)/extern.
    let fn_start = Regex::new(
        r"^(?:async\s+)?fn\s+(\w+)\s*(?:<[^>]*>)?\s*\("
    ).unwrap();
    // Exclude lines that are pub or part of a trait/impl definition.
    let pub_prefix = Regex::new(r"^pub|^extern|^impl\b|^trait\b").unwrap();

    let mut sigs: Vec<Signature> = vec![];
    let lines: Vec<&str> = source.lines().collect();

    for (i, line) in lines.iter().enumerate() {
        let trimmed = line.trim();
        if pub_prefix.is_match(trimmed) {
            continue;
        }
        if let Some(caps) = fn_start.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            if name.is_empty() { continue; }

            // Collect the signature up to the opening brace (may span multiple lines).
            let mut sig_parts: Vec<&str> = vec![];
            for sig_line in &lines[i..] {
                sig_parts.push(sig_line.trim());
                if sig_line.contains('{') || sig_parts.len() >= 5 { break; }
            }
            let raw = sig_parts.join(" ")
                .split('{').next()
                .unwrap_or(trimmed)
                .trim()
                .to_string();

            sigs.push(Signature {
                raw,
                ckb_id: None,
                symbol_name: Some(name.clone()),
                qualified_name: Some(name),
                kind: SymbolKind::Function,
                line_start: i,
                col_start: 0,
                line_end: i,
                col_end: 0,
                confidence: 20, // lower confidence than public symbols
                doc_comment: None,
                body: None,
                tested: false,
            });
        }
    }

    sigs
}

/// Annotate each item with how it connects to others in the chain.
/// Uses import edges and name-matching as a simple proxy for call edges.
fn annotate_connections(items: &mut Vec<EvidenceItem>, mapped: &[MappedFile]) {
    // Build a map from name → index for quick lookup.
    let name_to_idx: HashMap<String, usize> = items
        .iter()
        .enumerate()
        .map(|(i, item)| (item.name.clone(), i))
        .collect();

    for i in 0..items.len() {
        let sig_raw = items[i].sig.clone();
        let file = items[i].file.clone();

        // Check if any other item's name appears in this item's signature
        // (parameter type, return type) or if this file imports another item's file.
        let mut refs: Vec<String> = vec![];

        for (j, other) in items.iter().enumerate() {
            if i == j {
                continue;
            }
            // Type reference: other item's name appears in this item's sig.
            if sig_raw.contains(&other.name) && !other.name.is_empty() {
                match other.kind {
                    SymbolKind::Struct | SymbolKind::Class | SymbolKind::Enum
                    | SymbolKind::Interface | SymbolKind::TypeAlias => {
                        refs.push(format!("[uses type #{}]", j + 1));
                    }
                    SymbolKind::Function | SymbolKind::Method => {
                        refs.push(format!("[calls #{}]", j + 1));
                    }
                    _ => {}
                }
            }

            // Import reference: this file imports the other item's file.
            if let Some(mf) = mapped.iter().find(|m| m.path == file) {
                let other_stem = other
                    .file
                    .rsplit('/')
                    .next()
                    .unwrap_or("")
                    .trim_end_matches(".rs")
                    .trim_end_matches(".py")
                    .trim_end_matches(".go")
                    .trim_end_matches(".ts");
                if other_stem.len() >= 3
                    && mf.imports.iter().any(|imp| imp.contains(other_stem))
                    && other.file != file
                {
                    if !refs.iter().any(|r| r.contains(&(j + 1).to_string())) {
                        refs.push(format!("[imports from #{}]", j + 1));
                    }
                }
            }
        }

        if !refs.is_empty() {
            items[i].connection = Some(refs.join(", "));
        }
    }
}

// ---------------------------------------------------------------------------
// Body extraction
// ---------------------------------------------------------------------------

fn read_function_body(
    root_path: &Path,
    file: &str,
    line_start: usize,
    max_lines: usize,
) -> Option<String> {
    let abs = root_path.join(file);
    let source = std::fs::read_to_string(&abs).ok()?;
    let lines: Vec<&str> = source.lines().collect();
    if line_start >= lines.len() {
        return None;
    }

    // Find the opening brace.
    let mut brace_start = line_start;
    for (i, line) in lines[line_start..].iter().enumerate() {
        if line.contains('{') {
            brace_start = line_start + i;
            break;
        }
        if i > 8 {
            break; // give up if no brace within 8 lines
        }
    }

    // Collect body lines up to the matching closing brace or max_lines.
    let mut depth = 0i32;
    let mut body_lines: Vec<&str> = vec![];
    let mut in_body = false;

    for line in &lines[brace_start..] {
        for ch in line.chars() {
            if ch == '{' {
                depth += 1;
                in_body = true;
            } else if ch == '}' {
                depth -= 1;
            }
        }
        if in_body {
            body_lines.push(line);
        }
        if in_body && depth == 0 {
            break;
        }
        if body_lines.len() >= max_lines {
            body_lines.push("    // … (truncated)");
            break;
        }
    }

    if body_lines.is_empty() {
        None
    } else {
        Some(body_lines.join("\n"))
    }
}

// ---------------------------------------------------------------------------
// Renderer
// ---------------------------------------------------------------------------

pub fn render_answer(result: &AnswerResult) -> String {
    render_items(&result.items, &result.query)
}

fn render_items(items: &[EvidenceItem], query: &str) -> String {
    let mut out = String::with_capacity(4096);

    out.push_str(&format!("Evidence for: \"{}\"\n\n", query));

    for item in items {
        let kind_label = kind_str(item.kind);
        let short_file = short_path(&item.file);
        let role = item
            .role_note
            .as_deref()
            .map(|r| format!("  [{}]", r))
            .unwrap_or_default();
        let conn = item
            .connection
            .as_deref()
            .map(|c| format!("  {}", c))
            .unwrap_or_default();

        out.push_str(&format!(
            "{}  {}  {}  {}:{}{}{}\n",
            item.index, item.name, kind_label, short_file, item.line, role, conn
        ));
        out.push_str(&format!("   {}\n", item.sig.trim()));

        if let Some(ref body) = item.body {
            out.push_str(body);
            out.push('\n');
        }

        out.push('\n');
    }

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
    let last = match path.rfind('/') {
        None => return path,
        Some(i) => i,
    };
    match path[..last].rfind('/') {
        None | Some(0) => path,
        Some(second) => &path[second + 1..],
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
    fn score_symbol_matches_name_terms() {
        let sig = make_sig("verify_token", SymbolKind::Function, "pub fn verify_token(t: &str)", 0);
        let score = score_symbol(&sig, &["verify".to_string(), "token".to_string()], &[], 1.0);
        assert!(score > 5.0, "expected high score for name match, got {score}");
    }

    #[test]
    fn inflection_matches_inflections_not_lookalikes() {
        assert!(is_inflection("approval", "approve"));
        assert!(is_inflection("executes", "execute"));
        assert!(is_inflection("confirmed", "confirm"));
        assert!(is_inflection("proposed", "proposal"));
        // Must NOT collapse a word into a longer derived word, or unrelated
        // same-prefix words — that's what mis-ranked the NL answer.
        assert!(!is_inflection("action", "actionable"));
        assert!(!is_inflection("gate", "gateway"));
        assert!(!is_inflection("exec", "executor")); // tail too long
    }

    #[test]
    fn action_gate_query_ranks_real_gate_over_prompt_helper() {
        // The exact miss from the Madison test-drive: for an "action approval
        // gate" question, a helper whose name merely CONTAINS "action"
        // (`converse_actionable`, an LLM-prompt builder) outranked the real
        // gate methods. Token-aware name matching must demote it below them.
        let terms: Vec<String> =
            ["action", "approval", "gate", "proposed", "confirmed", "executes"]
                .iter()
                .map(|s| s.to_string())
                .collect();
        let helper = make_sig(
            "converse_actionable",
            SymbolKind::Function,
            "pub fn converse_actionable(vitals: &Vitals) -> Result<String, String>",
            0,
        );
        let execute = make_sig(
            "execute_proposal",
            SymbolKind::Function,
            "pub fn execute_proposal(&mut self, action: &Proposal) -> String",
            0,
        );
        let gate = make_sig("Gate", SymbolKind::Struct, "pub struct Gate {", 0);

        let s_helper = score_symbol(&helper, &terms, &[], 1.0);
        let s_execute = score_symbol(&execute, &terms, &[], 1.0);
        let s_gate = score_symbol(&gate, &terms, &[], 1.0);
        assert!(
            s_execute > s_helper,
            "execute_proposal ({s_execute}) should beat converse_actionable ({s_helper})"
        );
        assert!(
            s_gate > s_helper,
            "Gate ({s_gate}) should beat converse_actionable ({s_helper})"
        );
    }

    #[test]
    fn score_symbol_zero_for_no_match() {
        let sig = make_sig("unrelated_fn", SymbolKind::Function, "fn unrelated_fn()", 0);
        let score = score_symbol(&sig, &["auth".to_string(), "login".to_string()], &[], 1.0);
        assert_eq!(score, 0.0);
    }

    #[test]
    fn order_puts_top_function_first() {
        let scored = vec![
            ScoredSymbol {
                file: "src/auth.rs".into(),
                sig: make_sig("Claims", SymbolKind::Struct, "pub struct Claims {}", 0),
                score: 8.0,
            },
            ScoredSymbol {
                file: "src/auth.rs".into(),
                sig: make_sig("verify", SymbolKind::Function, "pub fn verify()", 10),
                score: 10.0,
            },
        ];
        let ordered = order_for_reading(scored, 4, &HashMap::new());
        assert_eq!(ordered[0].sig.symbol_name.as_deref(), Some("verify"),
            "top fn should come first");
        assert_eq!(ordered[1].sig.symbol_name.as_deref(), Some("Claims"),
            "type should follow");
    }

    #[test]
    fn order_prefers_older_file_within_10pct_score() {
        // Two companion functions that score within 10% of each other.
        // The one from the older file (smaller timestamp) should rank first.
        let scored = vec![
            ScoredSymbol {
                file: "src/class_graph.rs".into(),
                sig: make_sig("build_class_graph", SymbolKind::Function, "pub fn build_class_graph()", 0),
                score: 10.0,
            },
            ScoredSymbol {
                file: "src/call_graph.rs".into(),
                sig: make_sig("build_file_call_graph", SymbolKind::Function, "pub fn build_file_call_graph()", 0),
                score: 9.6, // within 10% of 10.0
            },
        ];
        let mut ages = HashMap::new();
        ages.insert("src/class_graph.rs".to_string(), 1_700_000_000u64); // newer
        ages.insert("src/call_graph.rs".to_string(),  1_600_000_000u64); // older
        let ordered = order_for_reading(scored, 4, &ages);
        assert_eq!(
            ordered[0].sig.symbol_name.as_deref(),
            Some("build_file_call_graph"),
            "older file should come first when scores within 10%"
        );
    }

    #[test]
    fn order_respects_score_outside_10pct() {
        // When the score gap exceeds 10%, the higher-scoring item wins
        // regardless of which file is older. Items must be pre-sorted by score
        // descending (matching build_answer's contract).
        let scored = vec![
            ScoredSymbol {
                file: "src/new.rs".into(),
                sig: make_sig("high_scorer", SymbolKind::Function, "pub fn high_scorer()", 0),
                score: 10.0,
            },
            ScoredSymbol {
                file: "src/old.rs".into(),
                sig: make_sig("low_scorer", SymbolKind::Function, "pub fn low_scorer()", 0),
                score: 7.0, // 30% below 10.0 — outside the 10% band
            },
        ];
        let mut ages = HashMap::new();
        ages.insert("src/old.rs".to_string(), 1_600_000_000u64); // older
        ages.insert("src/new.rs".to_string(), 1_700_000_000u64); // newer
        let ordered = order_for_reading(scored, 4, &ages);
        assert_eq!(
            ordered[0].sig.symbol_name.as_deref(),
            Some("high_scorer"),
            "higher score should win outside 10% band"
        );
    }

    #[test]
    fn render_answer_produces_numbered_items() {
        let result = AnswerResult {
            query: "how does auth work?".into(),
            items: vec![
                EvidenceItem {
                    index: 1,
                    name: "verify_token".into(),
                    file: "src/auth.rs".into(),
                    line: 42,
                    kind: SymbolKind::Function,
                    sig: "pub fn verify_token(token: &str) -> Result<Claims>".into(),
                    body: None,
                    connection: Some("[uses type #2]".into()),
                    role_note: Some("core logic".into()),
                },
                EvidenceItem {
                    index: 2,
                    name: "Claims".into(),
                    file: "src/types.rs".into(),
                    line: 14,
                    kind: SymbolKind::Struct,
                    sig: "pub struct Claims { sub: UserId, exp: u64 }".into(),
                    body: None,
                    connection: None,
                    role_note: Some("type".into()),
                },
            ],
            tokens_used: 0,
            budget_hit: false,
            files_searched: 10,
        };
        let rendered = render_answer(&result);
        assert!(rendered.contains("Evidence for:"));
        assert!(rendered.contains("1  verify_token"));
        assert!(rendered.contains("2  Claims"));
        assert!(rendered.contains("[uses type #2]"));
        assert!(rendered.contains("[core logic]"));
    }
}
