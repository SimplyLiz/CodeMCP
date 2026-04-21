//! Content search and file discovery — grep-like text/regex search + glob find.
//!
//! Reuses the existing file scanner (`.cartographerignore`, noise filter, security
//! block) unless `no_ignore` is set, in which case raw `walkdir` is used.

use crate::scanner::{is_ignored_path, scan_files_with_noise_tracking};
use rayon::prelude::*;
use regex::{Regex, RegexBuilder};
use serde::{Deserialize, Serialize};
use std::path::{Path, PathBuf};
use std::time::{Duration, SystemTime};

// ---------------------------------------------------------------------------
// Search options
// ---------------------------------------------------------------------------

/// Options for a content search request.
#[derive(Debug, Deserialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct SearchOptions {
    /// Treat `pattern` as a literal string (escape regex metacharacters).
    #[serde(default)]
    pub literal: bool,
    /// Case-sensitive matching — default `true`.
    #[serde(default = "default_true")]
    pub case_sensitive: bool,
    /// Symmetric context lines before and after each match (like `grep -C`).
    /// Ignored when `before_context` or `after_context` is nonzero.
    #[serde(default)]
    pub context_lines: usize,
    /// Lines of context before each match (like `grep -B`).
    #[serde(default)]
    pub before_context: usize,
    /// Lines of context after each match (like `grep -A`).
    #[serde(default)]
    pub after_context: usize,
    /// Cap on returned matches (0 = unlimited). Default 100.
    #[serde(default = "default_max")]
    pub max_results: usize,
    /// Include only files matching this glob (e.g. `"*.rs"` or `"src/**/*.ts"`).
    #[serde(default)]
    pub file_glob: Option<String>,
    /// Exclude files matching this glob.
    #[serde(default)]
    pub exclude_glob: Option<String>,
    /// Additional patterns OR'd with the primary pattern (like `grep -e`).
    #[serde(default)]
    pub extra_patterns: Vec<String>,
    /// Invert match — return lines that do NOT match (like `grep -v`).
    #[serde(default)]
    pub invert_match: bool,
    /// Whole-word matching — wraps pattern in `\b…\b` (like `grep -w`).
    #[serde(default)]
    pub word_regexp: bool,
    /// Print only the matched portion of each line (like `grep -o`).
    #[serde(default)]
    pub only_matching: bool,
    /// Return only file paths that contain matches (like `grep -l`).
    #[serde(default)]
    pub files_with_matches: bool,
    /// Return only file paths that contain NO matches (like `grep --files-without-match`).
    #[serde(default)]
    pub files_without_match: bool,
    /// Return per-file match counts instead of match lines (like `grep -c`).
    #[serde(default)]
    pub count_only: bool,
    /// Bypass noise/vendor/generated-file filter — search all text files.
    #[serde(default)]
    pub no_ignore: bool,
    /// Restrict search to this repo-relative subdirectory (e.g. `"src/api"`).
    #[serde(default)]
    pub search_path: Option<String>,
}

fn default_true() -> bool { true }
fn default_max() -> usize { 100 }

impl Default for SearchOptions {
    fn default() -> Self {
        Self {
            literal: false,
            case_sensitive: true,
            context_lines: 0,
            before_context: 0,
            after_context: 0,
            max_results: 100,
            file_glob: None,
            exclude_glob: None,
            extra_patterns: vec![],
            invert_match: false,
            word_regexp: false,
            only_matching: false,
            files_with_matches: false,
            files_without_match: false,
            count_only: false,
            no_ignore: false,
            search_path: None,
        }
    }
}

// ---------------------------------------------------------------------------
// Find options
// ---------------------------------------------------------------------------

/// Options for a file-find request.
#[derive(Debug, Deserialize, Clone, Default)]
#[serde(rename_all = "camelCase")]
pub struct FindOptions {
    /// Return only files modified within this many seconds of now (e.g. 86400 = last 24 h).
    #[serde(default)]
    pub modified_since_secs: Option<u64>,
    /// Return only files with mtime newer than this repo-relative file's mtime.
    #[serde(default)]
    pub newer_than: Option<String>,
    /// Minimum file size in bytes (inclusive).
    #[serde(default)]
    pub min_size_bytes: Option<u64>,
    /// Maximum file size in bytes (inclusive).
    #[serde(default)]
    pub max_size_bytes: Option<u64>,
    /// Maximum directory depth (0 = root files only, 1 = one level deep, …).
    #[serde(default)]
    pub max_depth: Option<usize>,
    /// Bypass noise/vendor/generated-file filter — find in all files.
    #[serde(default)]
    pub no_ignore: bool,
}

// ---------------------------------------------------------------------------
// Result types — search
// ---------------------------------------------------------------------------

/// A single context line (before or after a match).
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ContextLine {
    pub line_number: usize,
    pub line: String,
}

/// One matching line with optional surrounding context.
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct ContentMatch {
    /// Repo-relative path, forward-slash separated.
    pub path: String,
    pub line_number: usize,
    /// Full line text. Empty string when `only_matching` is active.
    pub line: String,
    /// Populated only when `only_matching` is true — the matched portion(s).
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub matched_texts: Vec<String>,
    /// Byte `[start, end]` offsets of every match on this line.
    /// Useful for highlight rendering without a second regex pass.
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub match_ranges: Vec<[usize; 2]>,
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub before_context: Vec<ContextLine>,
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub after_context: Vec<ContextLine>,
}

/// Per-file match count (populated by `count_only` mode).
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FileCount {
    pub path: String,
    pub count: usize,
}

/// Aggregated result returned by [`search_content`].
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct SearchResult {
    /// Matching lines — empty in `files_with_matches`, `files_without_match`, and `count_only` modes.
    pub matches: Vec<ContentMatch>,
    /// Total matches / files / counts returned (≤ `max_results` if truncated).
    pub total_matches: usize,
    /// Files actually read and searched.
    pub files_searched: usize,
    /// `true` when capped at `max_results`.
    pub truncated: bool,
    /// Populated when `files_with_matches` is set.
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub files_with_matches: Vec<String>,
    /// Populated when `files_without_match` is set.
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub files_without_match: Vec<String>,
    /// Populated when `count_only` is set.
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub file_counts: Vec<FileCount>,
}

// ---------------------------------------------------------------------------
// Result types — find
// ---------------------------------------------------------------------------

/// One matching file returned by [`find_files`].
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FindFile {
    /// Repo-relative path, forward-slash separated.
    pub path: String,
    /// Detected language (e.g. "Rust", "Go", "Python"), if known.
    pub language: Option<String>,
    /// File size in bytes.
    pub size_bytes: u64,
    /// ISO-8601 last modification timestamp (UTC), if available.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub modified: Option<String>,
}

/// Aggregated result returned by [`find_files`].
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct FindResult {
    pub files: Vec<FindFile>,
    pub total_matches: usize,
    pub truncated: bool,
}

// ---------------------------------------------------------------------------
// BM25 ranked search
// ---------------------------------------------------------------------------

/// Options for a BM25 ranked search request.
#[derive(Debug, Clone)]
pub struct BM25Options {
    /// BM25 term saturation parameter (default 1.5).
    pub k1: f64,
    /// BM25 length normalisation parameter (default 0.75).
    pub b: f64,
    /// Maximum number of results to return (0 = unlimited, default 20).
    pub max_results: usize,
    /// Include only files matching this glob (e.g. `"*.rs"`).
    pub file_glob: Option<String>,
    /// Restrict search to this repo-relative subdirectory.
    pub search_path: Option<String>,
    /// Bypass noise/vendor filter.
    pub no_ignore: bool,
}

impl Default for BM25Options {
    fn default() -> Self {
        Self {
            k1: 1.5,
            b: 0.75,
            max_results: 20,
            file_glob: None,
            search_path: None,
            no_ignore: false,
        }
    }
}

/// A file ranked by BM25 relevance.
#[derive(Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BM25Match {
    /// Repo-relative path.
    pub path: String,
    /// BM25 score (higher = more relevant).
    pub score: f64,
    /// Query terms found in this file.
    pub matching_terms: Vec<String>,
    /// Up to 3 representative lines containing query terms.
    pub snippets: Vec<String>,
}

/// Result returned by [`bm25_search`].
#[derive(Debug, Serialize, Default)]
#[serde(rename_all = "camelCase")]
pub struct BM25Result {
    pub matches: Vec<BM25Match>,
    pub total: usize,
}

/// Rank files in `root` by BM25 relevance to `query`.
///
/// Tokenises query and documents on word boundaries (alphanumeric runs),
/// lowercased. No stemming — exact term matching only.
/// Standard BM25 with k1=1.5, b=0.75 (overridable via `opts`).
pub fn bm25_search(root: &Path, query: &str, opts: &BM25Options) -> Result<BM25Result, String> {
    let files = enumerate_files_bm25(root, opts)?;

    let query_terms: Vec<String> = tokenize(query);
    if query_terms.is_empty() {
        return Ok(BM25Result { matches: vec![], total: 0 });
    }

    // Build per-document term frequencies and collect corpus-wide doc frequencies
    struct DocInfo {
        path: String,
        tf: std::collections::HashMap<String, usize>,
        length: usize,
        content: String,
    }

    let mut docs: Vec<DocInfo> = Vec::new();
    for path in &files {
        let content = match std::fs::read_to_string(path) {
            Ok(c) => c,
            Err(_) => continue,
        };
        let tokens = tokenize(&content);
        let length = tokens.len();
        let mut tf: std::collections::HashMap<String, usize> = std::collections::HashMap::new();
        for t in &tokens {
            *tf.entry(t.clone()).or_insert(0) += 1;
        }
        let rel = path.strip_prefix(root).unwrap_or(path)
            .to_string_lossy().replace('\\', "/");
        docs.push(DocInfo { path: rel, tf, length, content });
    }

    let n = docs.len() as f64;
    if n == 0.0 {
        return Ok(BM25Result { matches: vec![], total: 0 });
    }

    // Average document length
    let avg_len: f64 = docs.iter().map(|d| d.length as f64).sum::<f64>() / n;

    // Document frequency per query term
    let df: std::collections::HashMap<String, usize> = {
        let mut map = std::collections::HashMap::new();
        for term in &query_terms {
            let count = docs.iter().filter(|d| d.tf.contains_key(term)).count();
            map.insert(term.clone(), count);
        }
        map
    };

    // Score each document
    let mut scored: Vec<(f64, usize)> = docs.iter().enumerate().filter_map(|(i, doc)| {
        let mut score = 0.0_f64;
        let mut has_match = false;
        for term in &query_terms {
            let tf_val = *doc.tf.get(term).unwrap_or(&0);
            if tf_val == 0 { continue; }
            has_match = true;
            let df_val = *df.get(term).unwrap_or(&0) as f64;
            let idf = ((n - df_val + 0.5) / (df_val + 0.5) + 1.0).ln();
            let tf_norm = (tf_val as f64 * (opts.k1 + 1.0))
                / (tf_val as f64 + opts.k1 * (1.0 - opts.b + opts.b * doc.length as f64 / avg_len));
            score += idf * tf_norm;
        }
        if has_match { Some((score, i)) } else { None }
    }).collect();

    scored.sort_by(|a, b| b.0.partial_cmp(&a.0).unwrap_or(std::cmp::Ordering::Equal));

    let limit = if opts.max_results == 0 { scored.len() } else { opts.max_results.min(scored.len()) };
    let total = scored.len();

    let matches: Vec<BM25Match> = scored.into_iter().take(limit).map(|(score, i)| {
        let doc = &docs[i];
        let matching_terms: Vec<String> = query_terms.iter()
            .filter(|t| doc.tf.contains_key(*t))
            .cloned()
            .collect();

        // Collect up to 3 snippets — lines that contain a query term
        let snippets: Vec<String> = doc.content.lines()
            .filter(|line| {
                let lower = line.to_lowercase();
                query_terms.iter().any(|t| lower.contains(t.as_str()))
            })
            .take(3)
            .map(|l| l.trim().to_string())
            .collect();

        BM25Match { path: doc.path.clone(), score, matching_terms, snippets }
    }).collect();

    Ok(BM25Result { matches, total })
}

fn tokenize(text: &str) -> Vec<String> {
    // Split on non-alphanumeric runs; lowercase; drop single-char tokens and stop words
    const STOP: &[&str] = &[
        "the","a","an","is","in","on","at","to","of","and","or","for",
        "with","this","that","it","be","as","by","from","are","was","were",
    ];
    text.split(|c: char| !c.is_alphanumeric())
        .filter(|s| s.len() > 1)
        .map(|s| s.to_lowercase())
        .filter(|s| !STOP.contains(&s.as_str()))
        .collect()
}

fn enumerate_files_bm25(root: &Path, opts: &BM25Options) -> Result<Vec<std::path::PathBuf>, String> {
    let scan_root = if let Some(sp) = &opts.search_path {
        root.join(sp)
    } else {
        root.to_path_buf()
    };

    let glob_re = opts.file_glob.as_deref().and_then(build_glob_regex);

    let files: Vec<std::path::PathBuf> = if opts.no_ignore {
        walkdir::WalkDir::new(&scan_root)
            .follow_links(false)
            .into_iter()
            .filter_map(|e| e.ok())
            .filter(|e| e.file_type().is_file())
            .filter(|e| {
                if let Some(re) = &glob_re {
                    re.is_match(&e.path().to_string_lossy())
                } else {
                    true
                }
            })
            .map(|e| e.into_path())
            .collect()
    } else {
        let scan = scan_files_with_noise_tracking(&scan_root).map_err(|e| e.to_string())?;
        scan.files.into_iter()
            .filter(|p| !is_ignored_path(p))
            .filter(|p| {
                if let Some(re) = &glob_re {
                    re.is_match(&p.to_string_lossy())
                } else {
                    true
                }
            })
            .collect()
    };

    Ok(files)
}

// ---------------------------------------------------------------------------
// Core: search_content
// ---------------------------------------------------------------------------

/// Search for `pattern` across all non-noise, non-ignored files under `root`.
///
/// Pass `opts` to control matching mode, context, file filters, and output shape.
pub fn search_content(
    root: &Path,
    pattern: &str,
    opts: &SearchOptions,
) -> Result<SearchResult, String> {
    if pattern.is_empty() && opts.extra_patterns.is_empty() {
        return Err("pattern must not be empty".into());
    }

    // Build regexes — primary + all -e extras, OR'd at match time
    let mut all_res: Vec<Regex> = Vec::new();
    if !pattern.is_empty() {
        all_res.push(build_re(pattern, opts.literal, opts.word_regexp, opts.case_sensitive)?);
    }
    for ep in &opts.extra_patterns {
        if !ep.is_empty() {
            all_res.push(build_re(ep, opts.literal, opts.word_regexp, opts.case_sensitive)?);
        }
    }
    if all_res.is_empty() {
        return Err("no non-empty patterns provided".into());
    }

    // Glob filters
    let include_filter: Option<Regex> = opts.file_glob.as_deref().and_then(build_glob_regex);
    let exclude_filter: Option<Regex> = opts.exclude_glob.as_deref().and_then(build_glob_regex);

    // Effective per-side context
    let before_ctx = if opts.before_context > 0 || opts.after_context > 0 {
        opts.before_context
    } else {
        opts.context_lines
    };
    let after_ctx = if opts.before_context > 0 || opts.after_context > 0 {
        opts.after_context
    } else {
        opts.context_lines
    };

    let cap = if opts.max_results == 0 { usize::MAX } else { opts.max_results };

    let file_list = enumerate_files(root, opts.no_ignore)?;

    // ── Parallel file processing ─────────────────────────────────────────────
    // Local result carrier — one per file that passes filters and is readable.
    enum FileResult {
        Matches(Vec<ContentMatch>),
        WithMatch(String),
        WithoutMatch(String),
        Count(FileCount),
        Searched, // read but no matches (keeps files_searched count accurate)
    }

    let per_file: Vec<FileResult> = file_list
        .par_iter()
        .filter_map(|abs_path| {
            let rel = rel_path(root, abs_path);

            // search_path prefix filter
            if let Some(ref sp) = opts.search_path {
                let sp = sp.trim_end_matches('/');
                if !rel.starts_with(&format!("{}/", sp)) && rel != sp {
                    return None;
                }
            }

            // include/exclude glob
            if let Some(ref gre) = include_filter {
                if !gre.is_match(&rel) { return None; }
            }
            if let Some(ref gre) = exclude_filter {
                if gre.is_match(&rel) { return None; }
            }

            let content = std::fs::read_to_string(abs_path).ok()?;
            let lines: Vec<&str> = content.lines().collect();

            // ── count_only ───────────────────────────────────────────────────
            if opts.count_only {
                let count = lines.iter().filter(|&&l| line_matches(&all_res, l, opts.invert_match)).count();
                return Some(FileResult::Count(FileCount { path: rel, count }));
            }

            // ── files_with_matches / files_without_match ─────────────────────
            if opts.files_with_matches || opts.files_without_match {
                let has = lines.iter().any(|&l| line_matches(&all_res, l, opts.invert_match));
                return match (opts.files_with_matches && has, opts.files_without_match && !has) {
                    (true, _) => Some(FileResult::WithMatch(rel)),
                    (_, true) => Some(FileResult::WithoutMatch(rel)),
                    _ => Some(FileResult::Searched),
                };
            }

            // ── normal match mode ────────────────────────────────────────────
            let mut file_matches: Vec<ContentMatch> = Vec::new();
            for (idx, &line) in lines.iter().enumerate() {
                if !line_matches(&all_res, line, opts.invert_match) { continue; }

                let spans: Vec<_> = all_res.iter().flat_map(|re| re.find_iter(line)).collect();
                let matched_texts: Vec<String> = if opts.only_matching {
                    spans.iter().map(|m| m.as_str().to_string()).collect()
                } else {
                    vec![]
                };
                let match_ranges: Vec<[usize; 2]> = spans.iter()
                    .map(|m| [m.start(), m.end()])
                    .collect();

                file_matches.push(ContentMatch {
                    path: rel.clone(),
                    line_number: idx + 1,
                    line: if opts.only_matching { String::new() } else { line.to_string() },
                    matched_texts,
                    match_ranges,
                    before_context: context_slice(&lines, idx, before_ctx, true),
                    after_context: context_slice(&lines, idx, after_ctx, false),
                });
            }

            if file_matches.is_empty() {
                Some(FileResult::Searched)
            } else {
                Some(FileResult::Matches(file_matches))
            }
        })
        .collect();

    // ── Phase 2: flatten results and enforce hard cap ─────────────────────────
    let mut matches: Vec<ContentMatch> = Vec::new();
    let mut files_with_m: Vec<String> = Vec::new();
    let mut files_without_m: Vec<String> = Vec::new();
    let mut file_counts: Vec<FileCount> = Vec::new();
    let mut files_searched: usize = per_file.len();
    let mut truncated = false;

    for result in per_file {
        match result {
            FileResult::Count(fc) => {
                if file_counts.len() < cap { file_counts.push(fc); }
                else { truncated = true; }
            }
            FileResult::WithMatch(path) => {
                if files_with_m.len() < cap { files_with_m.push(path); }
                else { truncated = true; }
            }
            FileResult::WithoutMatch(path) => {
                if files_without_m.len() < cap { files_without_m.push(path); }
                else { truncated = true; }
            }
            FileResult::Matches(mut file_matches) => {
                let remaining = cap.saturating_sub(matches.len());
                if file_matches.len() > remaining {
                    file_matches.truncate(remaining);
                    truncated = true;
                }
                matches.extend(file_matches);
            }
            FileResult::Searched => {}
        }
    }

    let total_matches = if opts.count_only {
        file_counts.iter().map(|fc| fc.count).sum()
    } else if opts.files_with_matches {
        files_with_m.len()
    } else if opts.files_without_match {
        files_without_m.len()
    } else {
        matches.len()
    };

    Ok(SearchResult {
        matches,
        total_matches,
        files_searched,
        truncated,
        files_with_matches: files_with_m,
        files_without_match: files_without_m,
        file_counts,
    })
}

// ---------------------------------------------------------------------------
// Core: find_files
// ---------------------------------------------------------------------------

/// Find files whose repo-relative path matches a glob pattern.
///
/// `pattern` supports `*`, `**`, and `?`. Patterns without `/` are matched
/// against the filename only. Noise and ignored files are excluded unless
/// `opts.no_ignore` is set.
pub fn find_files(
    root: &Path,
    pattern: &str,
    limit: usize,
    opts: &FindOptions,
) -> Result<FindResult, String> {
    if pattern.is_empty() {
        return Err("pattern must not be empty".into());
    }

    let glob_re = build_glob_regex(pattern)
        .ok_or_else(|| format!("invalid glob: {}", pattern))?;

    let cap = if limit == 0 { usize::MAX } else { limit };

    // Resolve newer_than mtime threshold
    let newer_than_time: Option<SystemTime> = opts.newer_than.as_ref().and_then(|np| {
        std::fs::metadata(root.join(np)).and_then(|m| m.modified()).ok()
    });

    let modified_since_threshold: Option<SystemTime> = opts
        .modified_since_secs
        .map(|s| SystemTime::now() - Duration::from_secs(s));

    let file_list = enumerate_files(root, opts.no_ignore)?;

    let mut files: Vec<FindFile> = Vec::new();
    let mut truncated = false;

    for abs_path in &file_list {
        let rel = rel_path(root, abs_path);

        // depth filter (count '/' in repo-relative path)
        if let Some(max_d) = opts.max_depth {
            if rel.matches('/').count() > max_d { continue; }
        }

        if !glob_re.is_match(&rel) { continue; }

        let meta = match std::fs::metadata(abs_path) {
            Ok(m) => m,
            Err(_) => continue,
        };
        let size = meta.len();

        if let Some(min) = opts.min_size_bytes { if size < min { continue; } }
        if let Some(max) = opts.max_size_bytes { if size > max { continue; } }

        let mtime = meta.modified().ok();

        if let Some(threshold) = modified_since_threshold {
            match mtime { Some(t) if t >= threshold => {}, _ => continue }
        }
        if let Some(newer) = newer_than_time {
            match mtime { Some(t) if t > newer => {}, _ => continue }
        }

        let modified = mtime.map(|t| {
            let secs = t.duration_since(SystemTime::UNIX_EPOCH).map(|d| d.as_secs()).unwrap_or(0);
            format_unix_ts(secs)
        });

        files.push(FindFile {
            language: detect_language(&rel),
            path: rel,
            size_bytes: size,
            modified,
        });

        if files.len() >= cap { truncated = true; break; }
    }

    let total_matches = files.len();
    Ok(FindResult { files, total_matches, truncated })
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

fn line_matches(regexes: &[Regex], line: &str, invert: bool) -> bool {
    let hit = regexes.iter().any(|re| re.is_match(line));
    if invert { !hit } else { hit }
}

fn build_re(pattern: &str, literal: bool, word: bool, case_sensitive: bool) -> Result<Regex, String> {
    let mut pat = if literal { regex::escape(pattern) } else { pattern.to_string() };
    if word { pat = format!(r"\b{}\b", pat); }
    RegexBuilder::new(&pat)
        .case_insensitive(!case_sensitive)
        .build()
        .map_err(|e| format!("invalid pattern {:?}: {}", pattern, e))
}

/// Walk files respecting the noise filter (default) or raw walkdir (no_ignore).
fn enumerate_files(root: &Path, no_ignore: bool) -> Result<Vec<PathBuf>, String> {
    if no_ignore {
        use walkdir::WalkDir;
        let mut files = Vec::new();
        for entry in WalkDir::new(root).follow_links(false).into_iter().filter_map(|e| e.ok()) {
            if entry.file_type().is_file() {
                files.push(entry.into_path());
            }
        }
        Ok(files)
    } else {
        let scan = scan_files_with_noise_tracking(root).map_err(|e| e.to_string())?;
        let files = scan.files.into_iter().filter(|p| !is_ignored_path(p)).collect();
        Ok(files)
    }
}

fn rel_path(root: &Path, abs: &Path) -> String {
    abs.strip_prefix(root)
        .unwrap_or(abs)
        .to_string_lossy()
        .replace('\\', "/")
}

fn context_slice(lines: &[&str], match_idx: usize, n: usize, before: bool) -> Vec<ContextLine> {
    if n == 0 { return vec![]; }
    if before {
        let start = match_idx.saturating_sub(n);
        lines[start..match_idx]
            .iter()
            .enumerate()
            .map(|(j, l)| ContextLine { line_number: start + j + 1, line: l.to_string() })
            .collect()
    } else {
        let end = (match_idx + 1 + n).min(lines.len());
        lines[match_idx + 1..end]
            .iter()
            .enumerate()
            .map(|(j, l)| ContextLine { line_number: match_idx + 2 + j, line: l.to_string() })
            .collect()
    }
}

/// Convert a glob pattern to an anchored regex for matching repo-relative paths.
///
/// - `*.rs`        → match filename anywhere in tree  (`(^|/)[^/]*\.rs$`)
/// - `src/**/*.ts` → anchor to path root              (`^src/.*[^/]*\.ts$`)
pub fn build_glob_regex(pattern: &str) -> Option<Regex> {
    let has_sep = pattern.contains('/');
    let mut out = if has_sep { String::from("^") } else { String::from("(^|/)") };
    let chars: Vec<char> = pattern.chars().collect();
    let mut i = 0;
    while i < chars.len() {
        match chars[i] {
            '*' if i + 1 < chars.len() && chars[i + 1] == '*' => {
                out.push_str(".*");
                i += 2;
                if i < chars.len() && chars[i] == '/' { i += 1; }
            }
            '*' => { out.push_str("[^/]*"); i += 1; }
            '?' => { out.push_str("[^/]"); i += 1; }
            c @ ('.' | '+' | '^' | '$' | '{' | '}' | '(' | ')' | '|' | '[' | ']' | '\\') => {
                out.push('\\'); out.push(c); i += 1;
            }
            c => { out.push(c); i += 1; }
        }
    }
    out.push('$');
    Regex::new(&out).ok()
}

/// Detect language from file extension.
pub fn detect_language(path: &str) -> Option<String> {
    let ext = path.rsplit('.').next()?;
    let lang = match ext {
        "rs"                          => "Rust",
        "go"                          => "Go",
        "py"                          => "Python",
        "ts" | "tsx"                  => "TypeScript",
        "js" | "jsx" | "mjs" | "cjs" => "JavaScript",
        "java"                        => "Java",
        "kt" | "kts"                  => "Kotlin",
        "swift"                       => "Swift",
        "c" | "h"                     => "C",
        "cpp" | "cc" | "cxx" | "hpp" => "C++",
        "cs"                          => "C#",
        "rb"                          => "Ruby",
        "php"                         => "PHP",
        "dart"                        => "Dart",
        "scala"                       => "Scala",
        "ex" | "exs"                  => "Elixir",
        "hs"                          => "Haskell",
        "ml" | "mli"                  => "OCaml",
        "clj" | "cljs"                => "Clojure",
        "sh" | "bash" | "zsh"         => "Shell",
        "lua"                         => "Lua",
        "r" | "R"                     => "R",
        "jl"                          => "Julia",
        "sql"                         => "SQL",
        "toml"                        => "TOML",
        "json"                        => "JSON",
        "yaml" | "yml"                => "YAML",
        "md"                          => "Markdown",
        "html" | "htm"                => "HTML",
        "css" | "scss" | "less"       => "CSS",
        "xml"                         => "XML",
        "tf"                          => "Terraform",
        "proto"                       => "Protobuf",
        "graphql" | "gql"             => "GraphQL",
        _                             => return None,
    };
    Some(lang.to_string())
}

/// Format a Unix timestamp as `YYYY-MM-DDTHH:MM:SSZ` without pulling in chrono.
fn format_unix_ts(secs: u64) -> String {
    let s = secs % 60;
    let m = (secs / 60) % 60;
    let h = (secs / 3600) % 24;
    let days = secs / 86400;
    let (y, mo, d) = days_to_ymd(days);
    format!("{:04}-{:02}-{:02}T{:02}:{:02}:{:02}Z", y, mo, d, h, m, s)
}

/// Gregorian calendar: days since Unix epoch → (year, month, day).
fn days_to_ymd(mut days: u64) -> (u64, u64, u64) {
    days += 719468;
    let era = days / 146097;
    let doe = days % 146097;
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    let y   = yoe + era * 400;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp  = (5 * doy + 2) / 153;
    let d   = doy - (153 * mp + 2) / 5 + 1;
    let mo  = if mp < 10 { mp + 3 } else { mp - 9 };
    let y   = if mo <= 2 { y + 1 } else { y };
    (y, mo, d)
}

// ---------------------------------------------------------------------------
// Replace (sed equivalent)
// ---------------------------------------------------------------------------

/// Options for `replace_content`.
#[derive(Debug, Deserialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct ReplaceOptions {
    /// Treat pattern as a literal string.
    #[serde(default)]
    pub literal: bool,
    /// Case-sensitive match (default: true).
    #[serde(default = "default_true")]
    pub case_sensitive: bool,
    /// Whole-word matching (`\b…\b`).
    #[serde(default)]
    pub word_regexp: bool,
    /// Report changes without writing to disk.
    #[serde(default)]
    pub dry_run: bool,
    /// Write a `.bak` backup before modifying each file.
    #[serde(default)]
    pub backup: bool,
    /// Context lines to include in the diff output.
    #[serde(default = "default_ctx3")]
    pub context_lines: usize,
    /// Restrict to files matching this glob.
    #[serde(default)]
    pub file_glob: Option<String>,
    /// Exclude files matching this glob.
    #[serde(default)]
    pub exclude_glob: Option<String>,
    /// Restrict to this repo-relative subdirectory.
    #[serde(default)]
    pub search_path: Option<String>,
    /// Bypass noise/vendor filter.
    #[serde(default)]
    pub no_ignore: bool,
    /// Max replacements per file (0 = unlimited).
    #[serde(default)]
    pub max_per_file: usize,
}

fn default_ctx3() -> usize { 3 }

impl Default for ReplaceOptions {
    fn default() -> Self {
        Self {
            literal: false,
            case_sensitive: true,
            word_regexp: false,
            dry_run: false,
            backup: false,
            context_lines: 3,
            file_glob: None,
            exclude_glob: None,
            search_path: None,
            no_ignore: false,
            max_per_file: 0,
        }
    }
}

/// One line in a contextual unified-style diff.
#[derive(Debug, Serialize, Clone)]
pub struct DiffLine {
    /// `"context"`, `"removed"`, `"added"`, or `"separator"`.
    pub kind: String,
    pub line_number: usize,
    pub content: String,
}

/// Changes applied (or previewed) for a single file.
#[derive(Debug, Serialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct FileChange {
    pub path: String,
    pub replacements: usize,
    pub diff: Vec<DiffLine>,
}

/// Top-level result of `replace_content`.
#[derive(Debug, Serialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct ReplaceResult {
    pub files_changed: usize,
    pub total_replacements: usize,
    pub changes: Vec<FileChange>,
    pub dry_run: bool,
}

/// Regex find-and-replace across project files.
///
/// `replacement` supports `$0` (whole match) and `$1`/`$2` (capture groups).
/// When `dry_run = true` files are not written; only the diff is returned.
pub fn replace_content(
    root: &Path,
    pattern: &str,
    replacement: &str,
    opts: &ReplaceOptions,
) -> Result<ReplaceResult, String> {
    if pattern.is_empty() {
        return Err("pattern must not be empty".into());
    }
    let re = build_re(pattern, opts.literal, opts.word_regexp, opts.case_sensitive)?;

    let effective_root = match &opts.search_path {
        Some(sp) => {
            let candidate = root.join(sp);
            let canon = candidate.canonicalize().unwrap_or(candidate);
            if !canon.starts_with(root) {
                return Err("search_path escapes project root".into());
            }
            canon
        }
        None => root.to_path_buf(),
    };
    let file_glob_re = opts.file_glob.as_deref().and_then(build_glob_regex);
    let excl_re = opts.exclude_glob.as_deref().and_then(build_glob_regex);
    let file_list = enumerate_files(&effective_root, opts.no_ignore)?;

    let mut changes: Vec<FileChange> = Vec::new();
    let mut total_replacements: usize = 0;

    for abs_path in &file_list {
        let rel = rel_path(root, abs_path);
        if let Some(ref gr) = file_glob_re { if !gr.is_match(&rel) { continue; } }
        if let Some(ref er) = excl_re { if er.is_match(&rel) { continue; } }

        let original = match std::fs::read_to_string(abs_path) {
            Ok(s) => s,
            Err(_) => continue, // binary / unreadable
        };

        let match_count = re.find_iter(&original).count();
        if match_count == 0 { continue; }

        // 0 in regex crate = replace all
        let regex_limit = opts.max_per_file; // 0 = all
        let n = if opts.max_per_file == 0 { match_count } else { match_count.min(opts.max_per_file) };

        let replaced = re.replacen(&original, regex_limit, replacement as &str).into_owned();
        let diff = build_diff(&original, &replaced, opts.context_lines);

        if !opts.dry_run {
            if opts.backup {
                let bak = format!("{}.bak", abs_path.to_string_lossy());
                let _ = std::fs::copy(abs_path, bak);
            }
            std::fs::write(abs_path, replaced.as_bytes())
                .map_err(|e| format!("write {}: {}", rel, e))?;
        }

        total_replacements += n;
        changes.push(FileChange { path: rel, replacements: n, diff });
    }

    Ok(ReplaceResult {
        files_changed: changes.len(),
        total_replacements,
        changes,
        dry_run: opts.dry_run,
    })
}

/// Build a contextual diff between two versions of a file.
///
/// For single-line replacements both versions have the same number of lines, so
/// position `i` in old and new correspond directly. For replacements whose
/// replacement string contains `\n`, the new content has more lines; the
/// `(None, Some(&nl))` arm emits the extra added lines correctly in that case.
fn build_diff(old: &str, new: &str, ctx: usize) -> Vec<DiffLine> {
    let old_lines: Vec<&str> = old.lines().collect();
    let new_lines: Vec<&str> = new.lines().collect();
    let n = old_lines.len().max(new_lines.len());

    // Collect changed line indices
    let changed: Vec<usize> = (0..n)
        .filter(|&i| old_lines.get(i) != new_lines.get(i))
        .collect();
    if changed.is_empty() { return vec![]; }

    // Merge into context hunks
    let mut hunks: Vec<(usize, usize)> = Vec::new();
    for &ci in &changed {
        let start = ci.saturating_sub(ctx);
        let end = (ci + ctx + 1).min(n);
        if let Some(last) = hunks.last_mut() {
            if start <= last.1 { last.1 = last.1.max(end); continue; }
        }
        hunks.push((start, end));
    }

    let mut result: Vec<DiffLine> = Vec::new();
    let mut last_end = 0usize;

    for (start, end) in hunks {
        if start > last_end && last_end > 0 {
            result.push(DiffLine { kind: "separator".into(), line_number: 0, content: "---".into() });
        }
        for i in start..end {
            match (old_lines.get(i), new_lines.get(i)) {
                (Some(&ol), Some(&nl)) if ol == nl => {
                    result.push(DiffLine { kind: "context".into(), line_number: i + 1, content: ol.to_string() });
                }
                (Some(&ol), Some(&nl)) => {
                    result.push(DiffLine { kind: "removed".into(), line_number: i + 1, content: ol.to_string() });
                    result.push(DiffLine { kind: "added".into(), line_number: i + 1, content: nl.to_string() });
                }
                (Some(&ol), None) => {
                    result.push(DiffLine { kind: "removed".into(), line_number: i + 1, content: ol.to_string() });
                }
                (None, Some(&nl)) => {
                    result.push(DiffLine { kind: "added".into(), line_number: i + 1, content: nl.to_string() });
                }
                _ => {}
            }
        }
        last_end = end;
    }

    result
}

// ---------------------------------------------------------------------------
// Extract (awk equivalent)
// ---------------------------------------------------------------------------

/// Options for `extract_content`.
#[derive(Debug, Deserialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct ExtractOptions {
    /// Capture group indices to extract (empty = group 0 = whole match).
    #[serde(default)]
    pub groups: Vec<usize>,
    /// Separator between groups when multiple are selected (default: tab).
    #[serde(default = "default_tab")]
    pub separator: String,
    /// Output format: `"text"`, `"json"`, `"csv"`, or `"tsv"`.
    #[serde(default = "default_text_fmt")]
    pub format: String,
    /// Aggregate: count occurrences per unique extracted value.
    #[serde(default)]
    pub count: bool,
    /// Deduplicate extracted values.
    #[serde(default)]
    pub dedup: bool,
    /// Sort output (ascending; combined with `count` → sort by frequency desc).
    #[serde(default)]
    pub sort: bool,
    /// Case-sensitive match (default: true).
    #[serde(default = "default_true")]
    pub case_sensitive: bool,
    /// Restrict to files matching this glob.
    #[serde(default)]
    pub file_glob: Option<String>,
    /// Exclude files matching this glob.
    #[serde(default)]
    pub exclude_glob: Option<String>,
    /// Restrict to this repo-relative subdirectory.
    #[serde(default)]
    pub search_path: Option<String>,
    /// Bypass noise/vendor filter.
    #[serde(default)]
    pub no_ignore: bool,
    /// Max total results (0 = unlimited).
    #[serde(default)]
    pub limit: usize,
}

fn default_tab() -> String { "\t".to_string() }
fn default_text_fmt() -> String { "text".to_string() }

impl Default for ExtractOptions {
    fn default() -> Self {
        Self {
            groups: vec![],
            separator: "\t".to_string(),
            format: "text".to_string(),
            count: false,
            dedup: false,
            sort: false,
            case_sensitive: true,
            file_glob: None,
            exclude_glob: None,
            search_path: None,
            no_ignore: false,
            limit: 0,
        }
    }
}

/// A single extracted match row.
#[derive(Debug, Serialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct ExtractMatch {
    pub path: String,
    pub line_number: usize,
    /// Extracted group values (one entry per requested group, or whole match if none specified).
    pub groups: Vec<String>,
}

/// Frequency-count entry used when `count = true`.
#[derive(Debug, Serialize, Clone)]
pub struct CountEntry {
    pub value: String,
    pub count: usize,
}

/// Top-level result of `extract_content`.
#[derive(Debug, Serialize, Clone)]
#[serde(rename_all = "camelCase")]
pub struct ExtractResult {
    /// Raw matches (populated when `count = false`).
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub matches: Vec<ExtractMatch>,
    /// Frequency table (populated when `count = true`).
    #[serde(skip_serializing_if = "Vec::is_empty", default)]
    pub counts: Vec<CountEntry>,
    pub total: usize,
    pub files_searched: usize,
    pub truncated: bool,
}

/// Extract capture-group values from every regex match across project files.
///
/// Specify groups via `opts.groups` (e.g. `[1, 2]`); empty = group 0 (whole match).
/// Use `opts.count = true` for frequency aggregation, `opts.dedup`/`sort` for post-processing.
pub fn extract_content(
    root: &Path,
    pattern: &str,
    opts: &ExtractOptions,
) -> Result<ExtractResult, String> {
    if pattern.is_empty() {
        return Err("pattern must not be empty".into());
    }
    let re = RegexBuilder::new(pattern)
        .case_insensitive(!opts.case_sensitive)
        .build()
        .map_err(|e| format!("invalid pattern {:?}: {}", pattern, e))?;

    // Validate group indices upfront so callers get a clear error instead of silent empty strings.
    let num_groups = re.captures_len(); // includes group 0
    for &g in &opts.groups {
        if g >= num_groups {
            return Err(format!(
                "group {} out of range — pattern has {} capture group{}",
                g,
                num_groups.saturating_sub(1),
                if num_groups == 2 { "" } else { "s" },
            ));
        }
    }

    let effective_root = match &opts.search_path {
        Some(sp) => {
            let candidate = root.join(sp);
            let canon = candidate.canonicalize().unwrap_or(candidate);
            if !canon.starts_with(root) {
                return Err("search_path escapes project root".into());
            }
            canon
        }
        None => root.to_path_buf(),
    };
    let file_glob_re = opts.file_glob.as_deref().and_then(build_glob_regex);
    let excl_re = opts.exclude_glob.as_deref().and_then(build_glob_regex);
    let file_list = enumerate_files(&effective_root, opts.no_ignore)?;

    let cap_limit = if opts.limit == 0 { usize::MAX } else { opts.limit };
    let mut all_matches: Vec<ExtractMatch> = Vec::new();
    let mut files_searched: usize = 0;
    let mut truncated = false;

    'outer: for abs_path in &file_list {
        let rel = rel_path(root, abs_path);
        if let Some(ref gr) = file_glob_re { if !gr.is_match(&rel) { continue; } }
        if let Some(ref er) = excl_re { if er.is_match(&rel) { continue; } }

        let content = match std::fs::read_to_string(abs_path) {
            Ok(s) => s,
            Err(_) => continue,
        };
        files_searched += 1;

        for (line_idx, line) in content.lines().enumerate() {
            for caps in re.captures_iter(line) {
                let groups: Vec<String> = if opts.groups.is_empty() {
                    vec![caps.get(0).map_or("", |m| m.as_str()).to_string()]
                } else {
                    opts.groups.iter().map(|&g| {
                        caps.get(g).map_or("", |m| m.as_str()).to_string()
                    }).collect()
                };
                all_matches.push(ExtractMatch {
                    path: rel.clone(),
                    line_number: line_idx + 1,
                    groups,
                });
                if all_matches.len() >= cap_limit {
                    truncated = true;
                    break 'outer;
                }
            }
        }
    }

    let total = all_matches.len();

    if opts.count {
        use std::collections::HashMap;
        let mut freq: HashMap<String, usize> = HashMap::new();
        for m in &all_matches {
            *freq.entry(m.groups.join(&opts.separator)).or_insert(0) += 1;
        }
        let mut counts: Vec<CountEntry> = freq.into_iter()
            .map(|(value, count)| CountEntry { value, count })
            .collect();
        counts.sort_by(|a, b| b.count.cmp(&a.count).then(a.value.cmp(&b.value)));
        return Ok(ExtractResult { matches: vec![], counts, total, files_searched, truncated });
    }

    if opts.dedup {
        let mut seen = std::collections::HashSet::new();
        all_matches.retain(|m| seen.insert(m.groups.join("\x00")));
    }

    if opts.sort {
        all_matches.sort_by(|a, b| a.groups.cmp(&b.groups));
    }

    Ok(ExtractResult { matches: all_matches, counts: vec![], total, files_searched, truncated })
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn glob_filename_only() {
        let re = build_glob_regex("*.rs").unwrap();
        assert!(re.is_match("src/api.rs"));
        assert!(re.is_match("api.rs"));
        assert!(!re.is_match("src/api.ts"));
    }

    #[test]
    fn glob_path_anchored() {
        let re = build_glob_regex("src/**/*.ts").unwrap();
        assert!(re.is_match("src/components/button.ts"));
        assert!(re.is_match("src/button.ts"));
        assert!(!re.is_match("lib/button.ts"));
    }

    #[test]
    fn glob_exact_dir() {
        let re = build_glob_regex("src/*.rs").unwrap();
        assert!(re.is_match("src/api.rs"));
        assert!(!re.is_match("src/sub/api.rs"));
    }

    #[test]
    fn word_regexp_wraps() {
        let re = build_re("fn", false, true, true).unwrap();
        assert!(re.is_match("pub fn foo()"));
        assert!(!re.is_match("foo_fn_bar")); // not word-boundary
    }

    #[test]
    fn extra_patterns_or() {
        let res = vec![
            build_re("TODO", false, false, false).unwrap(),
            build_re("FIXME", false, false, false).unwrap(),
        ];
        assert!(line_matches(&res, "// TODO: refactor", false));
        assert!(line_matches(&res, "// FIXME: broken", false));
        assert!(!line_matches(&res, "// just a comment", false));
    }

    #[test]
    fn invert_match() {
        let res = vec![build_re("test", false, false, false).unwrap()];
        assert!(!line_matches(&res, "fn test_foo()", true));
        assert!(line_matches(&res, "fn production()", true));
    }

    #[test]
    fn format_unix_ts_known() {
        // 2024-01-15T00:00:00Z = 1705276800
        let s = format_unix_ts(1705276800);
        assert_eq!(s, "2024-01-15T00:00:00Z");
    }
}
