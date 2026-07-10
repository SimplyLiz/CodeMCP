//! Cross-file call tracing — spike implementation.
//!
//! Resolves unresolved callee names from a single-file `FileCallGraph` against
//! the project-wide symbol index, producing an ordered sequence of
//! (module, function) steps that represent how a call flows across files.
//!
//! Resolution strategy (two-level, as described in charts.md):
//!   1. Qualified match: callee name matches a symbol in a module that the
//!      entry file directly imports (from the import graph).
//!   2. Heuristic match: callee name matches a symbol in any module reachable
//!      within `depth` import hops. First unique match wins; ambiguous names
//!      are skipped with a note.
//!
//! This is intentionally imprecise — the spike goal is to check whether
//! "useful, not garbage" results emerge before committing to a full
//! implementation.

use std::collections::{HashMap, HashSet, VecDeque};
use std::path::Path;

use crate::call_graph::{build_file_call_graph, FileCallGraph};
use crate::mapper::MappedFile;

/// One step in a cross-file call trace.
#[derive(Debug, Clone)]
pub struct TraceStep {
    /// Caller: (module_id, qualified function name within that module).
    pub from: (String, String),
    /// Callee: (module_id, best-guess function name within that module).
    pub to: (String, String),
    /// How the resolution was made.
    pub confidence: ResolutionKind,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ResolutionKind {
    /// Callee module is a direct import of the caller's module.
    DirectImport,
    /// Callee module is reachable within the configured hop limit.
    Heuristic,
}

/// Result of a cross-file trace from an entry point.
pub struct CrossCallTrace {
    pub entry_module: String,
    pub entry_fn: String,
    pub steps: Vec<TraceStep>,
    /// Names that appeared as unresolved calls but couldn't be matched.
    pub unmatched: Vec<(String, String)>,
}

/// Run a cross-file call trace.
///
/// `entry_file` — absolute path to the file containing the entry function.
/// `entry_fn`   — simple or qualified function name (e.g. `"diagram_mode"`).
/// `mapped_files` — the project's full symbol + import index.
/// `depth`      — how many cross-file hops to follow (1 = direct callees only).
pub fn trace_from_entry(
    entry_file: &Path,
    entry_fn: &str,
    mapped_files: &HashMap<String, MappedFile>,
    root: &Path,
    depth: usize,
) -> Result<CrossCallTrace, String> {
    let source = std::fs::read_to_string(entry_file)
        .map_err(|e| format!("cannot read {}: {e}", entry_file.display()))?;

    let cg = build_file_call_graph(entry_file, &source)?
        .ok_or_else(|| format!(
            "cross-file tracing not supported for this file type: {}",
            entry_file.display()
        ))?;

    let entry_module = entry_file
        .strip_prefix(root)
        .unwrap_or(entry_file)
        .to_string_lossy()
        .replace('\\', "/");

    // Build symbol index: simple_name → [(module_id, qualified_name)]
    let mut sym_index: HashMap<String, Vec<(String, String)>> = HashMap::new();
    for (module_id, mf) in mapped_files {
        for sig in &mf.signatures {
            if let Some(name) = &sig.symbol_name {
                sym_index
                    .entry(name.clone())
                    .or_default()
                    .push((module_id.clone(), name.clone()));
            }
            if let Some(qname) = &sig.qualified_name {
                // Also index by unqualified tail (e.g. "build_file_call_graph"
                // from "call_graph::build_file_call_graph").
                let tail = qname.split("::").last().unwrap_or(qname);
                sym_index
                    .entry(tail.to_string())
                    .or_default()
                    .push((module_id.clone(), qname.clone()));
            }
        }
    }

    // Build import adjacency: module_id → [directly imported module_ids].
    // We derive this from MappedFile.imports. Rust imports are stored as the
    // full `use` path (e.g. "crate::mapper::extract_skeleton" or
    // "crate::scanner::{foo, bar}"). We extract the module name — the first
    // path component after "crate::" — and match it against file stems.
    let mut import_adj: HashMap<String, Vec<String>> = HashMap::new();
    let module_ids: Vec<&str> = mapped_files.keys().map(|s| s.as_str()).collect();
    for (module_id, mf) in mapped_files {
        let mut direct: Vec<String> = Vec::new();
        for imp in &mf.imports {
            if let Some(mod_name) = crate_module_name(imp) {
                for &mid in &module_ids {
                    let stem = Path::new(mid)
                        .file_stem()
                        .and_then(|s| s.to_str())
                        .unwrap_or("");
                    if stem == mod_name && mid != module_id.as_str() {
                        direct.push(mid.to_string());
                    }
                }
            }
        }
        direct.sort();
        direct.dedup();
        import_adj.insert(module_id.clone(), direct);
    }

    // Resolve the entry function name against the file-local call graph.
    let entry_qualified = cg
        .functions
        .iter()
        .find(|f| f.simple == entry_fn || f.qualified == entry_fn)
        .map(|f| f.qualified.clone())
        .ok_or_else(|| format!("function `{entry_fn}` not found in {}", entry_file.display()))?;

    let mut steps: Vec<TraceStep> = Vec::new();
    let mut unmatched: Vec<(String, String)> = Vec::new();
    let mut visited: HashSet<(String, String)> = HashSet::new();

    // BFS queue: (module_id, qualified_fn, remaining_depth, file_call_graph)
    let mut queue: VecDeque<(String, String, usize, FileCallGraph)> = VecDeque::new();
    queue.push_back((entry_module.clone(), entry_qualified.clone(), depth, cg));
    visited.insert((entry_module.clone(), entry_qualified.clone()));

    while let Some((cur_module, cur_fn, remaining, cur_cg)) = queue.pop_front() {
        let direct_imports = import_adj.get(&cur_module).cloned().unwrap_or_default();

        // Collect unresolved calls made by cur_fn.
        let unresolved: Vec<(String, String)> = cur_cg
            .unresolved_calls
            .iter()
            .filter(|(caller, _)| caller == &cur_fn || caller.ends_with(&format!("::{cur_fn}")))
            .cloned()
            .collect();

        for (_, callee_raw) in &unresolved {
            // Skip noise: very short names, common Rust idioms.
            if callee_raw.len() <= 2 || matches!(callee_raw.as_str(), "new" | "clone" | "into" | "from" | "unwrap" | "map" | "ok" | "err" | "len" | "push" | "pop" | "iter" | "collect" | "lock" | "map_err" | "ok_or_else" | "and_then" | "or_else" | "filter" | "format" | "println" | "eprintln" | "vec" | "default" | "is_empty" | "contains" | "get" | "insert" | "remove" | "take") {
                continue;
            }

            let candidates = sym_index.get(callee_raw.as_str()).cloned().unwrap_or_default();
            // Deduplicate by module.
            let mut by_module: HashMap<String, String> = HashMap::new();
            for (mid, qname) in &candidates {
                by_module.entry(mid.clone()).or_insert_with(|| qname.clone());
            }
            // Prefer direct imports.
            let direct_match = direct_imports.iter().find_map(|imp| {
                by_module.get(imp).map(|qname| (imp.clone(), qname.clone()))
            });

            let resolved = if let Some(m) = direct_match {
                Some((m.0, m.1, ResolutionKind::DirectImport))
            } else if by_module.len() == 1 {
                let (mid, qname) = by_module.into_iter().next().unwrap();
                Some((mid, qname, ResolutionKind::Heuristic))
            } else {
                None
            };

            match resolved {
                Some((to_module, to_fn, kind)) if to_module != cur_module => {
                    steps.push(TraceStep {
                        from: (cur_module.clone(), cur_fn.clone()),
                        to: (to_module.clone(), to_fn.clone()),
                        confidence: kind,
                    });

                    // Recurse if depth allows and we haven't visited this step.
                    if remaining > 0 && !visited.contains(&(to_module.clone(), to_fn.clone())) {
                        visited.insert((to_module.clone(), to_fn.clone()));
                        if let Some(mf) = mapped_files.get(&to_module) {
                            if let Ok(content) = std::fs::read_to_string(root.join(&to_module)) {
                                let to_abs = root.join(&to_module);
                                if let Ok(Some(next_cg)) = build_file_call_graph(&to_abs, &content) {
                                    queue.push_back((to_module, to_fn, remaining - 1, next_cg));
                                }
                            } else {
                                // File not readable — skip recursion but keep the step.
                                let _ = mf;
                            }
                        }
                    }
                }
                None if !candidates.is_empty() => {
                    // Multiple candidates, ambiguous — skip silently.
                }
                None => {
                    unmatched.push((cur_fn.clone(), callee_raw.clone()));
                }
                _ => {}
            }
        }
    }

    Ok(CrossCallTrace {
        entry_module,
        entry_fn: entry_qualified,
        steps,
        unmatched,
    })
}

/// Extract the module name from a Rust `use` import string.
///
/// Rust imports are stored after stripping `use ` and `;`, so they look like:
///   "crate::call_graph"                          → "call_graph"
///   "crate::mapper::extract_skeleton"            → "mapper"
///   "crate::scanner::{is_ignored, is_source}"    → "scanner"
///   "super::utils"                               → "utils"
///
/// Non-crate imports (std::, external crates) return `None` — they can't
/// resolve to a project module_id.
fn crate_module_name(imp: &str) -> Option<&str> {
    let rest = imp
        .trim()
        .strip_prefix("crate::")
        .or_else(|| imp.trim().strip_prefix("super::"))
        .or_else(|| {
            // Plain single-component name with no `::` could be a local mod.
            if !imp.contains("::") && !imp.contains(' ') {
                Some(imp.trim())
            } else {
                None
            }
        })?;
    // Take the first identifier — stop at `::`, `{`, `<`, or whitespace.
    let end = rest
        .find(|c: char| c == ':' || c == '{' || c == '<' || c.is_whitespace())
        .unwrap_or(rest.len());
    let name = rest[..end].trim();
    if name.is_empty() { None } else { Some(name) }
}
