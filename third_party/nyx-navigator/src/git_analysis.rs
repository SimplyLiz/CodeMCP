//! Git history analysis — co-change coupling, churn, and semantic diff helpers.
//! All functions fail gracefully (empty results) when git is unavailable or the
//! directory is not a repository.
//!
//! Bot commits and formatting-only commits are filtered by default because they
//! inflate churn and coupling metrics without representing real work.
//! (Research: ~74% of "hotspot" commits in practice come from bots or formatters.)

use std::collections::HashMap;
use std::path::Path;
use std::process::Command;

// ---------------------------------------------------------------------------
// Public types
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct CoChangePair {
    pub file_a: String,
    pub file_b: String,
    /// Number of commits where both files changed together.
    pub count: usize,
    /// count / min(churn_a, churn_b) — 1.0 means they always change together.
    pub coupling_score: f64,
}

// ---------------------------------------------------------------------------
// Noise-filter helpers
// ---------------------------------------------------------------------------

/// Returns true for known bot/automation author name patterns.
fn is_bot_author(author: &str) -> bool {
    let lower = author.to_lowercase();
    lower.contains("[bot]")
        || lower.contains("dependabot")
        || lower.contains("renovate")
        || lower.contains("github-actions")
        || lower.contains("snyk-bot")
        || lower.contains("greenkeeper")
        || lower.contains("semantic-release")
        || lower.contains("auto-merge")
        || lower.contains("release-bot")
        || lower.contains("ci-bot")
}

/// Returns true for commit subjects that look like formatting/lint-only passes.
fn is_formatting_subject(subject: &str) -> bool {
    let lower = subject.to_lowercase();
    // Common formatting commit patterns
    let patterns = [
        "apply prettier",
        "run prettier",
        "prettier format",
        "format code",
        "fix formatting",
        "auto format",
        "lint fix",
        "eslint fix",
        "fix lint",
        "apply lint",
        "rustfmt",
        "cargo fmt",
        "gofmt",
        "black format",
        "isort",
        "trailing whitespace",
        "fix whitespace",
        "whitespace fix",
        "normalize line endings",
        "editorconfig",
    ];
    patterns.iter().any(|p| lower.contains(p))
}

// ---------------------------------------------------------------------------
// Parse the log format we use for both churn and cochange:
//   --format=%x1f%an%x1f%s
//
// Each commit emits one line: \x1f<author>\x1f<subject>
// followed by the (--name-only) file list, followed by a blank line.
// A line is a commit header if it starts with \x1f.
// ---------------------------------------------------------------------------

struct CommitHeader {
    skip: bool, // bot author or formatting subject
}

fn parse_header(line: &str) -> Option<CommitHeader> {
    if !line.starts_with('\x1f') {
        return None;
    }
    let parts: Vec<&str> = line.splitn(3, '\x1f').collect();
    // parts[0] = "" (before first \x1f), parts[1] = author, parts[2] = subject
    let author = parts.get(1).copied().unwrap_or("").trim();
    let subject = parts.get(2).copied().unwrap_or("").trim();
    let skip = is_bot_author(author) || is_formatting_subject(subject);
    Some(CommitHeader { skip })
}

// ---------------------------------------------------------------------------
// git_churn
// ---------------------------------------------------------------------------

/// Return the number of commits that touched each file over the last `limit`
/// commits, relative paths from the repo root.
///
/// Bot and formatting-only commits are excluded.
/// Returns an empty map if git is unavailable or the directory is not a repo.
pub fn git_churn(root: &Path, limit: usize) -> HashMap<String, usize> {
    let output = Command::new("git")
        .args([
            "-C",
            &root.to_string_lossy(),
            "log",
            &format!("-n {}", limit),
            "--name-only",
            "--format=%x1f%an%x1f%s", // \x1f<author>\x1f<subject>
        ])
        .output();

    let output = match output {
        Ok(o) if o.status.success() => o,
        _ => return HashMap::new(),
    };

    let text = String::from_utf8_lossy(&output.stdout);
    let mut churn: HashMap<String, usize> = HashMap::new();
    let mut skip_current = false;

    for line in text.lines() {
        let line = line.trim();
        if let Some(header) = parse_header(line) {
            skip_current = header.skip;
            continue;
        }
        if line.is_empty() || skip_current {
            continue;
        }
        *churn.entry(line.to_string()).or_insert(0) += 1;
    }

    churn
}

// ---------------------------------------------------------------------------
// git_cochange
// ---------------------------------------------------------------------------

/// Analyse the last `limit` commits and return file pairs that changed together,
/// sorted descending by coupling_score.
///
/// Bot and formatting-only commits are excluded.
///
/// Uses Adam Tornhill's coupling formula:
///   coupling = co_changes / min(churn_a, churn_b)
pub fn git_cochange(root: &Path, limit: usize) -> Vec<CoChangePair> {
    let output = Command::new("git")
        .args([
            "-C",
            &root.to_string_lossy(),
            "log",
            &format!("-n {}", limit),
            "--name-only",
            "--format=%x1f%an%x1f%s",
        ])
        .output();

    let output = match output {
        Ok(o) if o.status.success() => o,
        _ => return vec![],
    };

    let text = String::from_utf8_lossy(&output.stdout);

    // Build per-commit file sets (filtered).
    let mut commits: Vec<Vec<String>> = Vec::new();
    let mut current: Vec<String> = Vec::new();
    let mut skip_current = false;

    for line in text.lines() {
        let line = line.trim();
        if let Some(header) = parse_header(line) {
            // Flush previous commit
            if !current.is_empty() {
                commits.push(std::mem::take(&mut current));
            }
            skip_current = header.skip;
            continue;
        }
        if line.is_empty() {
            continue;
        }
        if !skip_current {
            current.push(line.to_string());
        }
    }
    if !current.is_empty() {
        commits.push(current);
    }

    // Build churn map.
    let mut churn: HashMap<String, usize> = HashMap::new();
    for files in &commits {
        for f in files {
            *churn.entry(f.clone()).or_insert(0) += 1;
        }
    }

    // Count co-changes for every pair.
    let mut pair_counts: HashMap<(String, String), usize> = HashMap::new();
    for files in &commits {
        if files.len() < 2 {
            continue;
        }
        for i in 0..files.len() {
            for j in (i + 1)..files.len() {
                let (a, b) = if files[i] <= files[j] {
                    (files[i].clone(), files[j].clone())
                } else {
                    (files[j].clone(), files[i].clone())
                };
                *pair_counts.entry((a, b)).or_insert(0) += 1;
            }
        }
    }

    // Convert to CoChangePair with coupling score.
    let mut pairs: Vec<CoChangePair> = pair_counts
        .into_iter()
        .map(|((a, b), count)| {
            let ca = *churn.get(&a).unwrap_or(&1);
            let cb = *churn.get(&b).unwrap_or(&1);
            let min_churn = ca.min(cb) as f64;
            let coupling_score = if min_churn > 0.0 {
                (count as f64 / min_churn).min(1.0)
            } else {
                0.0
            };
            CoChangePair {
                file_a: a,
                file_b: b,
                count,
                coupling_score,
            }
        })
        .collect();

    pairs.sort_by(|a, b| {
        b.coupling_score
            .partial_cmp(&a.coupling_score)
            .unwrap_or(std::cmp::Ordering::Equal)
    });
    pairs
}

// ---------------------------------------------------------------------------
// Co-change dispersion / shotgun-surgery detection
// ---------------------------------------------------------------------------

/// Per-file co-change dispersion — how widely a file's changes scatter across
/// the codebase. High dispersion is the shotgun-surgery code smell:
/// one change triggers edits across many unrelated modules.
/// (arXiv:2504.18511 — Co-Change Graph Entropy for defect prediction)
#[derive(Debug, Clone, serde::Serialize, serde::Deserialize)]
pub struct CoChangeDispersion {
    pub file: String,
    /// Number of distinct files this file has co-changed with.
    pub partner_count: usize,
    /// Sum of co-change counts across all partners.
    pub total_cochanges: usize,
    /// Shannon entropy: −Σ p_i·log₂(p_i). Higher = more evenly spread across partners.
    pub entropy: f64,
    /// partner_count normalised to 0–100 across all files in the project.
    pub dispersion_score: f64,
}

/// Compute co-change dispersion for every file that appears in the co-change graph.
///
/// Reuses the existing co-change pairs — no additional git call.
pub fn git_cochange_dispersion(root: &Path, limit: usize) -> Vec<CoChangeDispersion> {
    let pairs = git_cochange(root, limit);
    if pairs.is_empty() {
        return vec![];
    }

    // Build per-file partner maps: file → { partner → count }
    let mut partner_counts: HashMap<String, HashMap<String, usize>> = HashMap::new();
    for p in &pairs {
        *partner_counts
            .entry(p.file_a.clone())
            .or_default()
            .entry(p.file_b.clone())
            .or_insert(0) += p.count;
        *partner_counts
            .entry(p.file_b.clone())
            .or_default()
            .entry(p.file_a.clone())
            .or_insert(0) += p.count;
    }

    let max_partners = partner_counts.values().map(|m| m.len()).max().unwrap_or(1).max(1) as f64;

    let mut result: Vec<CoChangeDispersion> = partner_counts
        .into_iter()
        .map(|(file, partners)| {
            let partner_count = partners.len();
            let total_cochanges: usize = partners.values().sum();
            let total = total_cochanges as f64;

            // Shannon entropy
            let entropy = if total > 0.0 {
                partners
                    .values()
                    .filter(|&&c| c > 0)
                    .map(|&c| {
                        let p = c as f64 / total;
                        -p * p.log2()
                    })
                    .sum::<f64>()
            } else {
                0.0
            };

            let dispersion_score = (partner_count as f64 / max_partners * 100.0).round();

            CoChangeDispersion {
                file,
                partner_count,
                total_cochanges,
                entropy,
                dispersion_score,
            }
        })
        .collect();

    result.sort_by(|a, b| {
        b.dispersion_score
            .partial_cmp(&a.dispersion_score)
            .unwrap_or(std::cmp::Ordering::Equal)
    });
    result
}

// ---------------------------------------------------------------------------
// git_show_file
// ---------------------------------------------------------------------------

/// Return the contents of `path` at `commit`, or None if unavailable.
pub fn git_show_file(root: &Path, commit: &str, path: &str) -> Option<String> {
    let spec = format!("{}:{}", commit, path);
    let output = Command::new("git")
        .args(["-C", &root.to_string_lossy(), "show", &spec])
        .output()
        .ok()?;

    if output.status.success() {
        String::from_utf8(output.stdout).ok()
    } else {
        None
    }
}

// ---------------------------------------------------------------------------
// git_diff_files
// ---------------------------------------------------------------------------

/// Return files that changed between `c1` and `c2`, with their status:
/// `'A'` = added, `'M'` = modified, `'D'` = deleted.
pub fn git_diff_files(root: &Path, c1: &str, c2: &str) -> Vec<(String, char)> {
    let output = Command::new("git")
        .args([
            "-C",
            &root.to_string_lossy(),
            "diff",
            "--name-status",
            c1,
            c2,
        ])
        .output();

    let output = match output {
        Ok(o) if o.status.success() => o,
        _ => return vec![],
    };

    let text = String::from_utf8_lossy(&output.stdout);
    let mut result = Vec::new();

    for line in text.lines() {
        let line = line.trim();
        if line.is_empty() {
            continue;
        }
        let parts: Vec<&str> = line.splitn(2, '\t').collect();
        if parts.len() != 2 {
            continue;
        }
        let status = parts[0].chars().next().unwrap_or('M');
        let file = parts[1].to_string();
        result.push((file, status));
    }

    result
}

// ---------------------------------------------------------------------------
// git_ownership
// ---------------------------------------------------------------------------

/// Dominant author per file over the last `limit` commits. "Dominant" =
/// highest raw commit count; ties broken alphabetically. Bot authors and
/// formatting-only commits are excluded (same filters as churn/cochange).
///
/// Returns an empty map if git is unavailable or the directory is not a repo.
/// Keys are repo-relative paths matching `git log --name-only` output.
#[allow(dead_code)]
pub fn git_ownership(root: &Path, limit: usize) -> HashMap<String, String> {
    let output = Command::new("git")
        .args([
            "-C",
            &root.to_string_lossy(),
            "log",
            &format!("-n {}", limit),
            "--name-only",
            "--format=%x1f%an%x1f%s",
        ])
        .output();

    let output = match output {
        Ok(o) if o.status.success() => o,
        _ => return HashMap::new(),
    };

    let text = String::from_utf8_lossy(&output.stdout);

    // Per-file per-author commit counts. We need the raw author name here
    // (not just the skip flag) so we parse the header locally instead of
    // reusing `parse_header`.
    let mut counts: HashMap<String, HashMap<String, usize>> = HashMap::new();
    let mut current_author: Option<String> = None;
    let mut skip_current = false;

    for line in text.lines() {
        let line = line.trim();
        if line.starts_with('\x1f') {
            let parts: Vec<&str> = line.splitn(3, '\x1f').collect();
            let author = parts.get(1).copied().unwrap_or("").trim().to_string();
            let subject = parts.get(2).copied().unwrap_or("").trim();
            skip_current = is_bot_author(&author) || is_formatting_subject(subject);
            current_author = if skip_current { None } else { Some(author) };
            continue;
        }
        if line.is_empty() || skip_current {
            continue;
        }
        if let Some(ref author) = current_author {
            *counts
                .entry(line.to_string())
                .or_default()
                .entry(author.clone())
                .or_insert(0) += 1;
        }
    }

    let mut owners: HashMap<String, String> = HashMap::with_capacity(counts.len());
    for (file, authors) in counts {
        // Pick the author with the highest count; ties → alphabetical so the
        // result is deterministic across runs.
        let dominant = authors
            .into_iter()
            .max_by(|a, b| a.1.cmp(&b.1).then_with(|| b.0.cmp(&a.0)));
        if let Some((name, _)) = dominant {
            owners.insert(file, name);
        }
    }
    owners
}

/// Return the number of distinct human authors per file (bus factor proxy).
/// A file touched by only one author has bus_factor = 1 — highest risk.
#[allow(dead_code)]
pub fn git_bus_factor(root: &Path, limit: usize) -> HashMap<String, usize> {
    let output = Command::new("git")
        .args([
            "-C",
            &root.to_string_lossy(),
            "log",
            &format!("-n {}", limit),
            "--name-only",
            "--format=%x1f%an%x1f%s",
        ])
        .output();

    let output = match output {
        Ok(o) if o.status.success() => o,
        _ => return HashMap::new(),
    };

    let text = String::from_utf8_lossy(&output.stdout);

    let mut file_authors: HashMap<String, std::collections::HashSet<String>> = HashMap::new();
    let mut current_author: Option<String> = None;
    let mut skip_current = false;

    for line in text.lines() {
        let line = line.trim();
        if line.starts_with('\x1f') {
            let parts: Vec<&str> = line.splitn(3, '\x1f').collect();
            let author = parts.get(1).copied().unwrap_or("").trim().to_string();
            let subject = parts.get(2).copied().unwrap_or("").trim();
            skip_current = is_bot_author(&author) || is_formatting_subject(subject);
            current_author = if skip_current { None } else { Some(author) };
            continue;
        }
        if line.is_empty() || skip_current {
            continue;
        }
        if let Some(ref author) = current_author {
            file_authors
                .entry(line.to_string())
                .or_default()
                .insert(author.clone());
        }
    }

    file_authors
        .into_iter()
        .map(|(file, authors)| (file, authors.len()))
        .collect()
}
