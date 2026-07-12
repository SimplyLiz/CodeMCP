//! C-FFI interface for CodeCartographer — consumed by CKB via CGo.
//!
//! Every function uses `extern "C"`, takes/returns `*const c_char` (C strings),
//! and never panics across the FFI boundary. Errors are returned as JSON error objects.
//!
//! Memory contract:
//!   - Input strings are borrowed (caller owns them).
//!   - Output strings are allocated by Rust and MUST be freed by the caller
//!     via `codecartographer_free_string()`.

use std::collections::HashMap;
use std::ffi::{CStr, CString};
use std::os::raw::c_char;
use std::path::{Path, PathBuf};

use rayon::prelude::*;

mod api;
mod call_graph;
mod class_graph;
mod cross_call;
mod diagram;
mod diagram_export;
mod extractor;
mod html_export;
mod git_analysis;
mod layers;
mod mapper;
mod scanner;
mod search;
mod token_metrics;

use api::ApiState;
use mapper::{extract_skeleton, MappedFile};
use scanner::{is_ignored_path, scan_files_with_noise_tracking};

// ---------------------------------------------------------------------------
// Memory management
// ---------------------------------------------------------------------------

/// Free a string returned by any `codecartographer_*` function.
///
/// # Safety
/// `ptr` must be a valid pointer returned by a CodeCartographer FFI function,
/// and must not have been freed already.
#[no_mangle]
pub unsafe extern "C" fn codecartographer_free_string(ptr: *mut c_char) {
    if ptr.is_null() {
        return;
    }
    drop(CString::from_raw(ptr));
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

fn c_str_to_path(s: *const c_char) -> Result<PathBuf, String> {
    if s.is_null() {
        return Err("null path".into());
    }
    let cstr = unsafe { CStr::from_ptr(s) };
    let rust_str = cstr.to_str().map_err(|e| e.to_string())?;
    Ok(PathBuf::from(rust_str))
}

fn result_to_json_ptr<T: serde::Serialize>(result: Result<T, String>) -> *mut c_char {
    let json = match result {
        Ok(value) => serde_json::json!({ "ok": true, "data": value }),
        Err(e) => serde_json::json!({ "ok": false, "error": e }),
    };
    let s = serde_json::to_string(&json)
        .unwrap_or_else(|_| r#"{"ok":false,"error":"serialization failed"}"#.to_string());
    match CString::new(s) {
        Ok(cs) => cs.into_raw(),
        Err(_) => {
            let fallback = CString::new(r#"{"ok":false,"error":"invalid utf8"}"#).unwrap();
            fallback.into_raw()
        }
    }
}

// ---------------------------------------------------------------------------
// Helpers: git HEAD, cache
// ---------------------------------------------------------------------------

/// Return the current git HEAD SHA for `root`, or `""` if not a git repo.
fn git_head(root: &Path) -> String {
    std::process::Command::new("git")
        .args(["-C", &root.to_string_lossy(), "rev-parse", "HEAD"])
        .output()
        .ok()
        .and_then(|o| if o.status.success() { Some(o.stdout) } else { None })
        .map(|b| String::from_utf8_lossy(&b).trim().to_string())
        .unwrap_or_default()
}

/// Persistent cache envelope stored at `<root>/.codecartographer_cache.json`.
#[derive(serde::Serialize, serde::Deserialize)]
struct MapCache {
    head: String,
    files: HashMap<String, MappedFile>,
}

fn cache_path(root: &Path) -> PathBuf {
    root.join(".codecartographer_cache.json")
}

fn load_cache(root: &Path, current_head: &str) -> Option<HashMap<String, MappedFile>> {
    if current_head.is_empty() {
        return None; // not a git repo — skip cache
    }
    let data = std::fs::read(cache_path(root)).ok()?;
    let cache: MapCache = serde_json::from_slice(&data).ok()?;
    if cache.head == current_head {
        Some(cache.files)
    } else {
        None
    }
}

fn save_cache(root: &Path, head: &str, files: &HashMap<String, MappedFile>) {
    if head.is_empty() {
        return;
    }
    let cache = MapCache { head: head.to_string(), files: files.clone() };
    if let Ok(json) = serde_json::to_vec(&cache) {
        let _ = std::fs::write(cache_path(root), json);
    }
}

// ---------------------------------------------------------------------------
// build_mapped_files: parallel scan + optional cache
// ---------------------------------------------------------------------------

/// Shared rayon pool with a large per-worker stack for the recursive tree-sitter
/// walkers. Built once (lazily) and reused, so repeated FFI calls don't respawn
/// threads. 256 MB is virtual reservation, not committed memory.
fn parse_pool() -> &'static rayon::ThreadPool {
    static POOL: std::sync::OnceLock<rayon::ThreadPool> = std::sync::OnceLock::new();
    POOL.get_or_init(|| {
        rayon::ThreadPoolBuilder::new()
            .stack_size(256 * 1024 * 1024)
            .build()
            .expect("failed to build parse thread pool")
    })
}

pub(crate) fn build_mapped_files(root: &Path) -> Result<HashMap<String, MappedFile>, String> {
    // Check persistent cache first
    let head = git_head(root);
    if let Some(cached) = load_cache(root, &head) {
        return Ok(cached);
    }

    let scan_result = scan_files_with_noise_tracking(root).map_err(|e| e.to_string())?;

    // Parallel extraction — extract_skeleton is pure, each file is independent.
    // Runs on a dedicated pool with a large stack: the tree-sitter walkers recurse
    // by AST depth, and deeply nested files (macro-generated C initializers in the
    // Linux kernel, etc.) overflow a worker's default ~2 MB stack and abort the
    // whole process — including the host when linked via FFI. The pool is built
    // once per process and shared across all FFI calls.
    let result: HashMap<String, MappedFile> = parse_pool().install(|| {
        scan_result
            .files
            .par_iter()
            .filter(|p| !is_ignored_path(p))
            .filter_map(|p| {
                let content = std::fs::read_to_string(p).ok()?;
                let mapped = extract_skeleton(p, &content);
                let rel = p
                    .strip_prefix(root)
                    .unwrap_or(p)
                    .to_string_lossy()
                    .replace('\\', "/");
                Some((rel, mapped))
            })
            .collect()
    });

    save_cache(root, &head, &result);
    Ok(result)
}

// ---------------------------------------------------------------------------
// FFI: Map Project
// ---------------------------------------------------------------------------

/// Scan a project directory and return the full project graph as JSON.
///
/// Input:  `path` — absolute path to project root (C string)
/// Output: JSON string (must be freed with `codecartographer_free_string`)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "nodes": [...],
///     "edges": [...],
///     "cycles": [...],
///     "godModules": [...],
///     "layerViolations": [...],
///     "metadata": { ... }
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_map_project(path: *const c_char) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let result = state.rebuild_graph();
    result_to_json_ptr(result)
}

// ---------------------------------------------------------------------------
// FFI: Health Score
// ---------------------------------------------------------------------------

/// Return the architectural health score for a project.
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "healthScore": 72.5,
///     "totalFiles": 150,
///     "totalEdges": 320,
///     "bridgeCount": 3,
///     "cycleCount": 1,
///     "godModuleCount": 0,
///     "layerViolationCount": 2
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_health(path: *const c_char) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let graph = match state.rebuild_graph() {
        Ok(g) => g,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let data = serde_json::json!({
        "healthScore": graph.metadata.health_score,
        "totalFiles": graph.metadata.total_files,
        "totalEdges": graph.metadata.total_edges,
        "bridgeCount": graph.metadata.bridge_count,
        "cycleCount": graph.metadata.cycle_count,
        "godModuleCount": graph.metadata.god_module_count,
        "layerViolationCount": graph.metadata.layer_violation_count,
    });

    result_to_json_ptr::<serde_json::Value>(Ok(data))
}

// ---------------------------------------------------------------------------
// FFI: Layer Violations
// ---------------------------------------------------------------------------

/// Check a project against a `layers.toml` config file.
///
/// Inputs:
///   `path`        — project root
///   `layers_path` — path to layers.toml (C string, may be null for defaults)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "violations": [
///       {
///         "sourcePath": "src/ui/button.ts",
///         "targetPath": "src/db/model.ts",
///         "sourceLayer": "ui",
///         "targetLayer": "db",
///         "violationType": "skip_call",
///         "severity": "HIGH"
///       }
///     ],
///     "violationCount": 1
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_check_layers(
    path: *const c_char,
    layers_path: *const c_char,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let config = if !layers_path.is_null() {
        let lp = match c_str_to_path(layers_path) {
            Ok(p) => p,
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
        };
        match layers::LayerConfig::from_file(&lp) {
            Ok(c) => c,
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
        }
    } else {
        layers::LayerConfig::default()
    };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let graph = match state.rebuild_graph() {
        Ok(g) => g,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let edge_tuples: Vec<(String, String)> = graph
        .edges
        .iter()
        .map(|e| (e.source.clone(), e.target.clone()))
        .collect();

    let violations = layers::detect_layer_violations(&edge_tuples, &config);

    let data = serde_json::json!({
        "violations": violations,
        "violationCount": violations.len(),
    });

    result_to_json_ptr::<serde_json::Value>(Ok(data))
}

// ---------------------------------------------------------------------------
// FFI: Simulate Change
// ---------------------------------------------------------------------------

/// Predict the architectural impact of changing a module.
///
/// Inputs:
///   `path`              — project root
///   `module_id`         — module path (relative to root)
///   `new_signature`     — optional new signature (may be null)
///   `remove_signature`  — optional signature to remove (may be null)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "targetModule": "src/auth/user.rs",
///     "predictedImpact": {
///       "affectedModules": ["src/api/handler.rs", "src/main.rs"],
///       "callersCount": 5,
///       "calleesCount": 2,
///       "willCreateCycle": false,
///       "layerViolations": [],
///       "riskLevel": "MEDIUM",
///       "healthImpact": -2.0
///     }
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_simulate_change(
    path: *const c_char,
    module_id: *const c_char,
    new_signature: *const c_char,
    remove_signature: *const c_char,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    if module_id.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null module_id".into()));
    }

    let mod_id = unsafe {
        match CStr::from_ptr(module_id).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };

    let new_sig = if !new_signature.is_null() {
        let s = unsafe {
            match CStr::from_ptr(new_signature).to_str() {
                Ok(s) => s,
                Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
            }
        };
        Some(s)
    } else {
        None
    };

    let rem_sig = if !remove_signature.is_null() {
        let s = unsafe {
            match CStr::from_ptr(remove_signature).to_str() {
                Ok(s) => s,
                Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
            }
        };
        Some(s)
    } else {
        None
    };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let result = state.simulate_change(&mod_id, new_sig, rem_sig);

    result_to_json_ptr(result)
}

// ---------------------------------------------------------------------------
// FFI: Skeleton Map (token-optimized)
// ---------------------------------------------------------------------------

/// Return a compressed skeleton map of the project for LLM context injection.
///
/// Input:
///   `path`        — project root
///   `detail`      — "minimal", "standard", or "extended" (may be null → standard)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "files": [
///       {
///         "path": "src/auth/user.rs",
///         "imports": ["std::collections::HashMap"],
///         "signatures": ["pub fn authenticate(...) -> User"]
///       }
///     ],
///     "totalFiles": 150,
///     "totalSignatures": 2300,
///     "estimatedTokens": 4500
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_skeleton_map(
    path: *const c_char,
    detail: *const c_char,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let detail_level = if !detail.is_null() {
        let d = unsafe { CStr::from_ptr(detail).to_str().unwrap_or("standard") };
        match d {
            "minimal" => mapper::DetailLevel::Minimal,
            "extended" => mapper::DetailLevel::Extended,
            _ => mapper::DetailLevel::Standard,
        }
    } else {
        mapper::DetailLevel::Standard
    };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let mut total_sigs = 0;
    let files: Vec<serde_json::Value> = mapped_files
        .values()
        .map(|f| {
            total_sigs += f.signatures.len();
            let sigs: Vec<_> = f.signatures.iter().map(|s| &s.raw).collect();
            match detail_level {
                mapper::DetailLevel::Minimal => serde_json::json!({
                    "path": f.path,
                    "signatures": sigs,
                }),
                mapper::DetailLevel::Standard => serde_json::json!({
                    "path": f.path,
                    "imports": f.imports,
                    "signatures": sigs,
                }),
                mapper::DetailLevel::Extended => serde_json::json!({
                    "path": f.path,
                    "imports": f.imports,
                    "signatures": sigs,
                    "docstrings": f.docstrings,
                    "returnTypes": f.return_types,
                }),
            }
        })
        .collect();

    let estimated_tokens = total_sigs * 15 + mapped_files.len() * 5;

    let data = serde_json::json!({
        "files": files,
        "totalFiles": mapped_files.len(),
        "totalSignatures": total_sigs,
        "estimatedTokens": estimated_tokens,
        "detailLevel": format!("{detail_level:?}"),
    });

    result_to_json_ptr::<serde_json::Value>(Ok(data))
}

// ---------------------------------------------------------------------------
// FFI: Module Context (single module with dependencies)
// ---------------------------------------------------------------------------

/// Get skeleton context for a single module with optional dependency depth.
///
/// Inputs:
///   `path`      — project root
///   `module_id` — relative file path
///   `depth`     — dependency traversal depth (0 = none)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "module": { "path": "...", "imports": [...], "signatures": [...] },
///     "dependencies": [
///       { "moduleId": "...", "path": "...", "signatureCount": 12 }
///     ]
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_module_context(
    path: *const c_char,
    module_id: *const c_char,
    depth: u32,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    if module_id.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null module_id".into()));
    }

    let mod_id = unsafe {
        match CStr::from_ptr(module_id).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    if let Err(e) = state.rebuild_graph() {
        return result_to_json_ptr::<serde_json::Value>(Err(e));
    }

    let module = state
        .mapped_files
        .lock()
        .unwrap()
        .get(&mod_id)
        .cloned()
        .ok_or_else(|| format!("Module not found: {}", mod_id));

    let module = match module {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let deps = match state.get_dependencies_internal(&mod_id, depth) {
        Ok(d) => d,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let data = serde_json::json!({
        "module": {
            "path": module.path,
            "imports": module.imports,
            "signatures": module.signatures.iter().map(|s| &s.raw).collect::<Vec<_>>(),
        },
        "dependencies": deps,
    });

    result_to_json_ptr::<serde_json::Value>(Ok(data))
}

// ---------------------------------------------------------------------------
// FFI: Version
// ---------------------------------------------------------------------------

/// Return the CodeCartographer library version string (e.g. "9.0.0").
///
/// Output: raw C string — must be freed with `codecartographer_free_string`.
#[no_mangle]
pub extern "C" fn codecartographer_version() -> *mut c_char {
    let version = env!("CARGO_PKG_VERSION");
    match CString::new(version) {
        Ok(cs) => cs.into_raw(),
        Err(_) => std::ptr::null_mut(),
    }
}

// ---------------------------------------------------------------------------
// FFI: Git Churn
// ---------------------------------------------------------------------------

/// Return per-file commit counts over the last `limit` commits.
///
/// Inputs:
///   `path`  — project root (C string)
///   `limit` — number of commits to analyse (0 → 500)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "src/api.rs": 42,
///     "src/main.rs": 18
///   }
/// }
/// ```
/// Returns an empty object when the directory is not a git repo.
#[no_mangle]
pub extern "C" fn codecartographer_git_churn(path: *const c_char, limit: u32) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let limit = if limit == 0 { 500 } else { limit as usize };
    let churn = git_analysis::git_churn(&path, limit);
    result_to_json_ptr::<std::collections::HashMap<String, usize>>(Ok(churn))
}

// ---------------------------------------------------------------------------
// FFI: Git Co-change
// ---------------------------------------------------------------------------

/// Return temporally coupled file pairs from the last `limit` commits.
///
/// Inputs:
///   `path`      — project root (C string)
///   `limit`     — number of commits to analyse (0 → 500)
///   `min_count` — minimum co-change count to include (0 → 2)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": [
///     {
///       "fileA": "src/api.rs",
///       "fileB": "src/main.rs",
///       "count": 12,
///       "couplingScore": 0.92
///     }
///   ]
/// }
/// ```
/// Returns an empty array when the directory is not a git repo.
#[no_mangle]
pub extern "C" fn codecartographer_git_cochange(
    path: *const c_char,
    limit: u32,
    min_count: u32,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let limit = if limit == 0 { 500 } else { limit as usize };
    let min_count = if min_count == 0 { 2 } else { min_count as usize };

    let pairs: Vec<serde_json::Value> = git_analysis::git_cochange(&path, limit)
        .into_iter()
        .filter(|p| p.count >= min_count)
        .map(|p| {
            serde_json::json!({
                "fileA": p.file_a,
                "fileB": p.file_b,
                "count": p.count,
                "couplingScore": p.coupling_score,
            })
        })
        .collect();

    result_to_json_ptr::<Vec<serde_json::Value>>(Ok(pairs))
}

// ---------------------------------------------------------------------------
// FFI: Semantic Diff
// ---------------------------------------------------------------------------

/// Return a function-level diff between two commits.
///
/// Inputs:
///   `path`    — project root (C string)
///   `commit1` — base commit (C string)
///   `commit2` — target commit (C string; use "HEAD" for latest)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": [
///     {
///       "path": "src/api.rs",
///       "status": "modified",
///       "added": ["pub fn new_handler(...)"],
///       "removed": ["fn old_helper(...)"]
///     },
///     {
///       "path": "src/old.rs",
///       "status": "deleted",
///       "added": [],
///       "removed": ["pub fn foo()", "pub fn bar()"]
///     }
///   ]
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_semidiff(
    path: *const c_char,
    commit1: *const c_char,
    commit2: *const c_char,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let c1 = if commit1.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null commit1".into()));
    } else {
        unsafe {
            match CStr::from_ptr(commit1).to_str() {
                Ok(s) => s.to_string(),
                Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
            }
        }
    };

    let c2 = if commit2.is_null() {
        "HEAD".to_string()
    } else {
        unsafe {
            match CStr::from_ptr(commit2).to_str() {
                Ok(s) => s.to_string(),
                Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
            }
        }
    };

    let changed = git_analysis::git_diff_files(&path, &c1, &c2);

    let diff: Vec<serde_json::Value> = changed
        .iter()
        .map(|(file_path, status)| {
            let status_str = match status {
                'A' => "added",
                'D' => "deleted",
                _ => "modified",
            };
            let fake_path = std::path::Path::new(file_path);

            let before_sigs: Vec<String> = if *status != 'A' {
                git_analysis::git_show_file(&path, &c1, file_path)
                    .map(|content| {
                        let mf = mapper::extract_skeleton(fake_path, &content);
                        mf.signatures.into_iter().map(|s| s.raw).collect()
                    })
                    .unwrap_or_default()
            } else {
                vec![]
            };

            let after_sigs: Vec<String> = if *status != 'D' {
                git_analysis::git_show_file(&path, &c2, file_path)
                    .map(|content| {
                        let mf = mapper::extract_skeleton(fake_path, &content);
                        mf.signatures.into_iter().map(|s| s.raw).collect()
                    })
                    .unwrap_or_default()
            } else {
                vec![]
            };

            let before_set: std::collections::HashSet<&str> =
                before_sigs.iter().map(|s| s.as_str()).collect();
            let after_set: std::collections::HashSet<&str> =
                after_sigs.iter().map(|s| s.as_str()).collect();

            let added: Vec<&str> = after_sigs
                .iter()
                .filter(|s| !before_set.contains(s.as_str()))
                .map(|s| s.as_str())
                .collect();
            let removed: Vec<&str> = before_sigs
                .iter()
                .filter(|s| !after_set.contains(s.as_str()))
                .map(|s| s.as_str())
                .collect();

            serde_json::json!({
                "path": file_path,
                "status": status_str,
                "added": added,
                "removed": removed,
            })
        })
        .collect();

    result_to_json_ptr::<Vec<serde_json::Value>>(Ok(diff))
}

// ---------------------------------------------------------------------------
// FFI: Hidden Coupling
// ---------------------------------------------------------------------------

/// Return file pairs that co-change frequently but have NO import edge between
/// them — i.e. implicit/hidden coupling that is invisible in the static graph.
///
/// Inputs:
///   `path`      — project root
///   `limit`     — commits to analyse (0 → 500)
///   `min_count` — minimum co-change count to include (0 → 2)
///
/// Response shape: same as `codecartographer_git_cochange` (array of CoChangePair).
/// Returns an empty array when the directory is not a git repo.
#[no_mangle]
pub extern "C" fn codecartographer_hidden_coupling(
    path: *const c_char,
    limit: u32,
    min_count: u32,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let limit = if limit == 0 { 500 } else { limit as usize };
    let min_count = if min_count == 0 { 2 } else { min_count as usize };

    // Build the static import-edge set from the dependency graph.
    let scan_result = match scan_files_with_noise_tracking(&path) {
        Ok(r) => r,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
    };
    let mapped: std::collections::HashMap<String, MappedFile> = scan_result
        .files
        .iter()
        .filter(|p| !is_ignored_path(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(&path)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(path.clone());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped;
    }

    // Normalise: store both (a,b) and (b,a) so lookup is direction-agnostic.
    let import_edges: std::collections::HashSet<(String, String)> =
        match state.rebuild_graph() {
            Ok(graph) => graph
                .edges
                .iter()
                .flat_map(|e| {
                    [
                        (e.source.clone(), e.target.clone()),
                        (e.target.clone(), e.source.clone()),
                    ]
                })
                .collect(),
            Err(_) => std::collections::HashSet::new(),
        };

    // Keep only pairs with no import edge — those are the hidden coupling.
    let pairs: Vec<serde_json::Value> = git_analysis::git_cochange(&path, limit)
        .into_iter()
        .filter(|p| p.count >= min_count)
        .filter(|p| !import_edges.contains(&(p.file_a.clone(), p.file_b.clone())))
        .map(|p| {
            serde_json::json!({
                "fileA": p.file_a,
                "fileB": p.file_b,
                "count": p.count,
                "couplingScore": p.coupling_score,
            })
        })
        .collect();

    result_to_json_ptr::<Vec<serde_json::Value>>(Ok(pairs))
}

// ---------------------------------------------------------------------------
// FFI: Ranked Skeleton (personalized PageRank, token-budget-aware)
// ---------------------------------------------------------------------------

/// Return a token-budget-aware ranked skeleton using personalized PageRank.
///
/// Inputs:
///   `path`       — project root (C string)
///   `focus_json` — JSON array of focus file paths for personalization (C string, may be null/empty)
///   `budget`     — max tokens to include (0 = unlimited)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": [
///     {
///       "path": "src/api.rs",
///       "moduleId": "src/api.rs",
///       "rank": 0.0842,
///       "signatureCount": 45,
///       "estimatedTokens": 680,
///       "role": "core",
///       "signatures": ["pub fn rebuild_graph(...) -> ...", "..."]
///     }
///   ]
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_ranked_skeleton(
    path: *const c_char,
    focus_json: *const c_char,
    budget: u32,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let focus: Vec<String> = if !focus_json.is_null() {
        let s = unsafe {
            match CStr::from_ptr(focus_json).to_str() {
                Ok(s) => s,
                Err(_) => "",
            }
        };
        serde_json::from_str(s).unwrap_or_default()
    } else {
        vec![]
    };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    if let Err(e) = state.rebuild_graph() {
        return result_to_json_ptr::<serde_json::Value>(Err(e));
    }

    let ranked = match state.ranked_skeleton(&focus, budget as usize) {
        Ok(r) => r,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let data: Vec<serde_json::Value> = ranked
        .into_iter()
        .map(|f| serde_json::json!({
            "path": f.path,
            "moduleId": f.module_id,
            "rank": f.rank,
            "signatureCount": f.signature_count,
            "estimatedTokens": f.estimated_tokens,
            "role": f.role,
            "signatures": f.signatures,
        }))
        .collect();

    result_to_json_ptr::<Vec<serde_json::Value>>(Ok(data))
}

// ---------------------------------------------------------------------------
// FFI: Unreferenced Symbols
// ---------------------------------------------------------------------------

/// Return public symbols that appear unreferenced across the project (heuristic).
///
/// Input:  `path` — project root (C string)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "totalCount": 12,
///     "files": [
///       {
///         "path": "src/utils.rs",
///         "symbols": ["pub fn unused_helper(...)", "pub const OLD_VALUE: ..."]
///       }
///     ]
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_unreferenced_symbols(path: *const c_char) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let graph = match state.rebuild_graph() {
        Ok(g) => g,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let mut total_count = 0usize;
    let files: Vec<serde_json::Value> = graph
        .nodes
        .iter()
        .filter_map(|n| {
            let exports = n.unreferenced_exports.as_ref()?;
            if exports.is_empty() {
                return None;
            }
            total_count += exports.len();
            Some(serde_json::json!({
                "path": n.path,
                "symbols": exports,
            }))
        })
        .collect();

    let data = serde_json::json!({
        "totalCount": total_count,
        "files": files,
    });

    result_to_json_ptr::<serde_json::Value>(Ok(data))
}

// ---------------------------------------------------------------------------
// FFI: Content Search (grep-like)
// ---------------------------------------------------------------------------

/// Search for text or regex patterns across all project files.
///
/// Inputs:
///   `path`      — project root (C string)
///   `pattern`   — search pattern (C string; regex unless `literal` is set in opts)
///   `opts_json` — JSON-encoded search options (may be null → defaults)
///
/// Options JSON shape:
/// ```json
/// {
///   "literal":       false,
///   "caseSensitive": true,
///   "contextLines":  0,
///   "maxResults":    100,
///   "fileGlob":      "*.rs"
/// }
/// ```
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "matches": [
///       {
///         "path": "src/api.rs",
///         "lineNumber": 42,
///         "line": "pub fn rebuild_graph(&self) -> Result<...",
///         "beforeContext": [{"lineNumber": 40, "line": "// comment"}, ...],
///         "afterContext":  [{"lineNumber": 43, "line": "    let g = Graph::new();"}, ...]
///       }
///     ],
///     "totalMatches": 1,
///     "filesSearched": 18,
///     "truncated": false
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_search_content(
    path: *const c_char,
    pattern: *const c_char,
    opts_json: *const c_char,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    if pattern.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null pattern".into()));
    }
    let pat = unsafe {
        match std::ffi::CStr::from_ptr(pattern).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };

    let opts: search::SearchOptions = if !opts_json.is_null() {
        let raw = unsafe {
            match std::ffi::CStr::from_ptr(opts_json).to_str() {
                Ok(s) => s,
                Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
            }
        };
        serde_json::from_str(raw).unwrap_or_default()
    } else {
        search::SearchOptions::default()
    };

    let result = search::search_content(&path, &pat, &opts);
    result_to_json_ptr(result)
}

/// Find files matching a glob pattern across the project.
///
/// Parameters:
/// - `path`      – absolute path to repo root (UTF-8 C string)
/// - `pattern`   – glob pattern, e.g. `"*.rs"` or `"src/subdir/*.go"` (C string)
/// - `limit`     – max files to return; 0 = unlimited
/// - `opts_json` – optional JSON `FindOptions` or null for defaults:
///   `{ modifiedSinceSecs, newerThan, minSizeBytes, maxSizeBytes, maxDepth, noIgnore }`
///
/// Returns a JSON envelope:
/// ```json
/// { "ok": true, "data": { "files": [...], "totalMatches": N, "truncated": false } }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_find_files(
    path: *const c_char,
    pattern: *const c_char,
    limit: u32,
    opts_json: *const c_char,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    if pattern.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null pattern".into()));
    }
    let pat = unsafe {
        match std::ffi::CStr::from_ptr(pattern).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };

    let opts: search::FindOptions = if !opts_json.is_null() {
        let raw = unsafe {
            match std::ffi::CStr::from_ptr(opts_json).to_str() {
                Ok(s) => s,
                Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
            }
        };
        serde_json::from_str(raw).unwrap_or_default()
    } else {
        search::FindOptions::default()
    };

    let result = search::find_files(&path, &pat, limit as usize, &opts);
    result_to_json_ptr(result)
}

// ---------------------------------------------------------------------------
// FFI: Blast Radius
// ---------------------------------------------------------------------------

/// Get files/modules directly impacted by changing a target module.
///
/// Inputs:
///   `path`        — project root (C string)
///   `target`      — module ID or path fragment (C string)
///   `max_related` — cap on returned entries (0 → 10)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "target": "src/api.rs",
///     "moduleId": "src/api.rs",
///     "related": [
///       { "moduleId": "src/main.rs", "path": "src/main.rs", "relationship": "dependent" },
///       { "moduleId": "src/lib.rs",  "path": "src/lib.rs",  "relationship": "dependency" }
///     ]
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_blast_radius(
    path: *const c_char,
    target: *const c_char,
    max_related: u32,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    if target.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null target".into()));
    }
    let target = unsafe {
        match CStr::from_ptr(target).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };
    let max = if max_related == 0 { 10 } else { max_related as usize };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    { let mut f = state.mapped_files.lock().unwrap(); *f = mapped_files; }

    if let Err(e) = state.rebuild_graph() {
        return result_to_json_ptr::<serde_json::Value>(Err(e));
    }

    // Resolve module_id (exact match or path substring)
    let module_id = {
        let graph = state.project_graph.lock().unwrap();
        graph.as_ref().and_then(|g| {
            g.nodes.iter().find(|n| {
                n.module_id == target
                    || n.path == target
                    || n.path.starts_with(&format!("{}/", target))
                    || n.module_id.starts_with(&format!("{}/", target))
            }).map(|n| n.module_id.clone())
        })
    };

    let module_id = match module_id {
        Some(id) => id,
        None => return result_to_json_ptr::<serde_json::Value>(
            Err(format!("target not found: {}", target))
        ),
    };

    let deps = state.get_dependencies_internal(&module_id, 1)
        .unwrap_or_default()
        .unwrap_or_default();
    let dependents = state.get_dependents(&module_id).unwrap_or_default();

    let mut related: Vec<serde_json::Value> = Vec::new();
    for d in &deps {
        if related.len() >= max { break; }
        related.push(serde_json::json!({
            "moduleId": d.module_id, "path": d.path, "relationship": "dependency"
        }));
    }
    for d in &dependents {
        if related.len() >= max { break; }
        related.push(serde_json::json!({
            "moduleId": d.module_id, "path": d.path, "relationship": "dependent"
        }));
    }

    result_to_json_ptr::<serde_json::Value>(Ok(serde_json::json!({
        "target": target,
        "moduleId": module_id,
        "related": related,
    })))
}

// ---------------------------------------------------------------------------
// FFI: Architecture Evolution
// ---------------------------------------------------------------------------

/// Return architecture health trend over time for a project.
///
/// Inputs:
///   `path` — project root (C string)
///   `days` — look-back window in days (0 → default 30)
///
/// Snapshot deduplication: if the current git HEAD matches the most recently
/// recorded snapshot, that entry is updated in-place rather than a new one
/// being appended.  Callers may invoke this function on every startup without
/// inflating the history.
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "snapshots": [
///       { "timestamp": 1777507200, "gitRef": "abc123", "healthScore": 72.5, ... }
///     ],
///     "healthTrend": "Stable",
///     "trendAvailable": false,
///     "debtIndicators": ["2 dependency cycles detected"],
///     "recommendations": ["Resolve dependency cycles to improve health score"]
///   }
/// }
/// ```
///
/// `snapshots` is ordered newest-first; `snapshots[0]` is always the current
/// reading and carries the same health score as `codecartographer_health`.
///
/// `trendAvailable` is `false` when the window contains fewer than two
/// snapshots from distinct git commits (or, for non-git roots, when the
/// window spans less than one hour).  Callers should suppress directional
/// trend UI when this field is `false`.
#[no_mangle]
pub extern "C" fn codecartographer_evolution(
    path: *const c_char,
    days: u32,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    { let mut f = state.mapped_files.lock().unwrap(); *f = mapped_files; }

    let days_opt = if days == 0 { None } else { Some(days) };
    let result = state.get_evolution(days_opt);
    result_to_json_ptr(result)
}

// ---------------------------------------------------------------------------
// FFI: Poll Changes
// ---------------------------------------------------------------------------

/// Return project files modified since a given epoch-millisecond timestamp.
///
/// Inputs:
///   `path`     — project root (C string)
///   `since_ms` — epoch milliseconds; 0 → last 60 seconds
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "changedFiles": ["src/api.rs", "src/main.rs"],
///     "checkedAtMs": 1712345678901
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_poll_changes(
    path: *const c_char,
    since_ms: u64,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let threshold_ms = if since_ms == 0 {
        // default: last 60 seconds
        std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_millis()
            .saturating_sub(60_000) as u64
    } else {
        since_ms
    };

    let threshold = std::time::UNIX_EPOCH
        + std::time::Duration::from_millis(threshold_ms);

    let now_ms = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_millis() as u64;

    let scan = match scan_files_with_noise_tracking(&path) {
        Ok(s) => s,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
    };

    let changed: Vec<String> = scan.files
        .iter()
        .filter(|p| !is_ignored_path(p))
        .filter_map(|p| {
            let mtime = std::fs::metadata(p).ok()?.modified().ok()?;
            if mtime > threshold {
                let rel = p.strip_prefix(&path).unwrap_or(p)
                    .to_string_lossy().replace('\\', "/");
                Some(rel)
            } else {
                None
            }
        })
        .collect();

    result_to_json_ptr::<serde_json::Value>(Ok(serde_json::json!({
        "changedFiles": changed,
        "checkedAtMs": now_ms,
    })))
}

/// Regex find-and-replace across project files (sed-like).
///
/// Inputs:
///   `path`        — project root (C string)
///   `pattern`     — regex pattern (C string)
///   `replacement` — replacement string; supports `$0` / `$1` capture refs (C string)
///   `opts_json`   — JSON-encoded `ReplaceOptions` (may be null → defaults)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "filesChanged": 3,
///     "totalReplacements": 12,
///     "dryRun": false,
///     "changes": [
///       {
///         "path": "src/api.rs",
///         "replacements": 4,
///         "diff": [
///           { "kind": "context",  "lineNumber": 9,  "content": "fn old()" },
///           { "kind": "removed",  "lineNumber": 10, "content": "    let x = 1;" },
///           { "kind": "added",    "lineNumber": 10, "content": "    let x = 2;" }
///         ]
///       }
///     ]
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_replace_content(
    path: *const c_char,
    pattern: *const c_char,
    replacement: *const c_char,
    opts_json: *const c_char,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    if pattern.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null pattern".into()));
    }
    let pat = unsafe {
        match std::ffi::CStr::from_ptr(pattern).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };
    if replacement.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null replacement".into()));
    }
    let repl = unsafe {
        match std::ffi::CStr::from_ptr(replacement).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };
    let opts: search::ReplaceOptions = if !opts_json.is_null() {
        let raw = unsafe {
            match std::ffi::CStr::from_ptr(opts_json).to_str() {
                Ok(s) => s,
                Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
            }
        };
        serde_json::from_str(raw).unwrap_or_default()
    } else {
        search::ReplaceOptions::default()
    };

    let result = search::replace_content(&path, &pat, &repl, &opts);
    result_to_json_ptr(result)
}

/// Extract capture-group values from regex matches across project files (awk-like).
///
/// Inputs:
///   `path`      — project root (C string)
///   `pattern`   — regex pattern with optional capture groups (C string)
///   `opts_json` — JSON-encoded `ExtractOptions` (may be null → defaults)
///
/// Options JSON shape:
/// ```json
/// {
///   "groups":        [1, 2],
///   "separator":     "\t",
///   "format":        "text",
///   "count":         false,
///   "dedup":         false,
///   "sort":          false,
///   "caseSensitive": true,
///   "fileGlob":      "*.rs",
///   "excludeGlob":   null,
///   "searchPath":    null,
///   "noIgnore":      false,
///   "limit":         0
/// }
/// ```
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "matches": [
///       { "path": "src/api.rs", "lineNumber": 42, "groups": ["pub fn foo", "foo"] }
///     ],
///     "counts": [],
///     "total": 1,
///     "filesSearched": 18,
///     "truncated": false
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_extract_content(
    path: *const c_char,
    pattern: *const c_char,
    opts_json: *const c_char,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    if pattern.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null pattern".into()));
    }
    let pat = unsafe {
        match std::ffi::CStr::from_ptr(pattern).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };
    let opts: search::ExtractOptions = if !opts_json.is_null() {
        let raw = unsafe {
            match std::ffi::CStr::from_ptr(opts_json).to_str() {
                Ok(s) => s,
                Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
            }
        };
        serde_json::from_str(raw).unwrap_or_default()
    } else {
        search::ExtractOptions::default()
    };

    let result = search::extract_content(&path, &pat, &opts);
    result_to_json_ptr(result)
}

// ---------------------------------------------------------------------------
// FFI: Context Health
// ---------------------------------------------------------------------------

/// Analyse the quality of an LLM context bundle and return a health report.
///
/// `content`   — the context text to analyse (C string)
/// `opts_json` — optional JSON object with scoring options:
///               `{ "model": "claude"|"gpt4"|"llama"|"gpt35",
///                  "windowSize": 0,           // 0 = use model default
///                  "signatureCount": 0,        // number of symbols in content
///                  "signatureTokens": 0,       // tokens used by signatures
///                  "keyPositions": [0.0, 1.0]  // relative positions of key modules
///               }`
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "tokenCount": 4200,
///     "charCount": 17500,
///     "windowSize": 200000,
///     "utilizationPct": 2.1,
///     "score": 78.4,
///     "grade": "B",
///     "metrics": { "signalDensity": 0.42, ... },
///     "warnings": [...],
///     "recommendations": [...]
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_context_health(
    content: *const c_char,
    opts_json: *const c_char,
) -> *mut c_char {
    if content.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null content".into()));
    }
    let text = unsafe {
        match std::ffi::CStr::from_ptr(content).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };

    #[derive(serde::Deserialize, Default)]
    #[serde(rename_all = "camelCase")]
    struct HealthOptsJson {
        model: Option<String>,
        window_size: Option<usize>,
        signature_count: Option<usize>,
        signature_tokens: Option<usize>,
        key_positions: Option<Vec<f64>>,
    }

    let json_opts: HealthOptsJson = if !opts_json.is_null() {
        let raw = unsafe {
            match std::ffi::CStr::from_ptr(opts_json).to_str() {
                Ok(s) => s,
                Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
            }
        };
        serde_json::from_str(raw).unwrap_or_default()
    } else {
        HealthOptsJson::default()
    };

    let model = json_opts
        .model
        .as_deref()
        .and_then(|s| s.parse::<token_metrics::ModelFamily>().ok())
        .unwrap_or_default();

    let opts = token_metrics::HealthOpts {
        model,
        window_size:      json_opts.window_size.unwrap_or(0),
        key_positions:    json_opts.key_positions.unwrap_or_default(),
        signature_count:  json_opts.signature_count.unwrap_or(0),
        signature_tokens: json_opts.signature_tokens.unwrap_or(0),
    };

    let report = token_metrics::analyze(&text, &opts);
    result_to_json_ptr(Ok::<_, String>(report))
}

// ---------------------------------------------------------------------------
// FFI: BM25 Search
// ---------------------------------------------------------------------------

/// Rank project files by BM25 relevance to a natural-language query.
///
/// `path`      — project root (C string)
/// `query`     — natural language query or symbol name (C string)
/// `opts_json` — optional JSON object:
///               `{ "k1": 1.5, "b": 0.75, "maxResults": 20,
///                  "fileGlob": "*.rs", "searchPath": "src/", "noIgnore": false }`
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "matches": [
///       {
///         "path": "src/api.rs",
///         "score": 4.21,
///         "matchingTerms": ["rebuild", "graph"],
///         "snippets": ["pub fn rebuild_graph(&self) -> Result<..."]
///       }
///     ],
///     "total": 3
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_bm25_search(
    path: *const c_char,
    query: *const c_char,
    opts_json: *const c_char,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    if query.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null query".into()));
    }
    let q = unsafe {
        match std::ffi::CStr::from_ptr(query).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };

    #[derive(serde::Deserialize, Default)]
    #[serde(rename_all = "camelCase")]
    struct Bm25OptsJson {
        k1: Option<f64>,
        b: Option<f64>,
        max_results: Option<usize>,
        file_glob: Option<String>,
        search_path: Option<String>,
        no_ignore: Option<bool>,
    }

    let json_opts: Bm25OptsJson = if !opts_json.is_null() {
        let raw = unsafe {
            match std::ffi::CStr::from_ptr(opts_json).to_str() {
                Ok(s) => s,
                Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
            }
        };
        serde_json::from_str(raw).unwrap_or_default()
    } else {
        Bm25OptsJson::default()
    };

    let mut opts = search::BM25Options::default();
    if let Some(k1) = json_opts.k1 { opts.k1 = k1; }
    if let Some(b) = json_opts.b { opts.b = b; }
    if let Some(mr) = json_opts.max_results { opts.max_results = mr; }
    if let Some(g) = json_opts.file_glob { opts.file_glob = Some(g); }
    if let Some(sp) = json_opts.search_path { opts.search_path = Some(sp); }
    if let Some(ni) = json_opts.no_ignore { opts.no_ignore = ni; }

    let result = search::bm25_search(&path, &q, &opts);
    result_to_json_ptr(Ok::<_, String>(result))
}

// ---------------------------------------------------------------------------
// FFI: Query Context (PKG retrieval pipeline)
// ---------------------------------------------------------------------------

/// Full retrieval pipeline: search → PageRank → health → ready-to-inject bundle.
///
/// `path`      — project root (C string)
/// `query`     — natural language query or symbol name (C string)
/// `opts_json` — optional JSON:
///               `{ "budget": 8000, "model": "claude", "maxSearchResults": 20 }`
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "context": "## Ranked Context for: ...\n\n// src/api.rs ...",
///     "filesUsed": ["src/api.rs", "src/mapper.rs"],
///     "focusFiles": ["src/api.rs"],
///     "totalTokens": 3420,
///     "health": { "score": 82.1, "grade": "B", ... }
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_query_context(
    path: *const c_char,
    query: *const c_char,
    opts_json: *const c_char,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    if query.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null query".into()));
    }
    let q = unsafe {
        match std::ffi::CStr::from_ptr(query).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };

    #[derive(serde::Deserialize, Default)]
    #[serde(rename_all = "camelCase")]
    struct QueryOptsJson {
        budget: Option<usize>,
        model: Option<String>,
        max_search_results: Option<usize>,
    }

    let json_opts: QueryOptsJson = if !opts_json.is_null() {
        let raw = unsafe {
            match std::ffi::CStr::from_ptr(opts_json).to_str() {
                Ok(s) => s,
                Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
            }
        };
        serde_json::from_str(raw).unwrap_or_default()
    } else {
        QueryOptsJson::default()
    };

    let budget = json_opts.budget.unwrap_or(8000);
    let max_search = json_opts.max_search_results.unwrap_or(20);
    let model_str = json_opts.model.unwrap_or_else(|| "claude".to_string());

    // Step 1: BM25 + regex search for focus seeds
    let bm25_opts = search::BM25Options { max_results: max_search, ..Default::default() };
    let bm25_result = search::bm25_search(&path, &q, &bm25_opts).unwrap_or_default();

    let search_opts = search::SearchOptions { case_sensitive: false, max_results: max_search, ..Default::default() };
    let regex_hits: Vec<String> = search::search_content(&path, &q, &search_opts)
        .map(|sr| {
            let mut seen = std::collections::HashSet::new();
            sr.matches.into_iter()
                .filter_map(|m| if seen.insert(m.path.clone()) { Some(m.path) } else { None })
                .collect()
        })
        .unwrap_or_default();

    // Merge: BM25 first (ranked), then regex hits not already present
    let mut focus_files: Vec<String> = bm25_result.matches.iter().map(|m| m.path.clone()).collect();
    for p in regex_hits {
        if !focus_files.contains(&p) {
            focus_files.push(p);
        }
    }
    focus_files.truncate(max_search);

    // Code-question retrieval: bias focus seeds toward actual source files so
    // raw-content BM25 doesn't latch onto data/doc files (e.g. Godot's
    // doc/classes/*.xml) that mention the query terms but carry no code. Fall
    // back to the unfiltered list if nothing source-like matched.
    let code_seeds: Vec<String> = focus_files
        .iter()
        .filter(|p| scanner::is_source_file(std::path::Path::new(p.as_str())))
        .cloned()
        .collect();
    if !code_seeds.is_empty() {
        focus_files = code_seeds;
    }

    // Step 2: ranked skeleton personalised to focus files
    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    { let mut f = state.mapped_files.lock().unwrap(); *f = mapped_files; }
    if let Err(e) = state.rebuild_graph() {
        return result_to_json_ptr::<serde_json::Value>(Err(e));
    }

    let ranked = match state.ranked_skeleton(&focus_files, budget) {
        Ok(r) => r,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    // Step 3: build context text
    let mut context_text = format!("## Ranked Context for: {}\n\n", q);
    let total_tokens: usize = ranked.iter().map(|f| f.estimated_tokens).sum();
    let sig_count: usize = ranked.iter().map(|f| f.signatures.len()).sum();
    let files_used: Vec<String> = ranked.iter().map(|f| f.path.clone()).collect();

    for f in &ranked {
        context_text.push_str(&format!("// {} (rank: {:.4}, {} tokens)\n", f.path, f.rank, f.estimated_tokens));
        for sig in &f.signatures {
            context_text.push_str(&format!("  {}\n", sig));
        }
        context_text.push('\n');
    }

    // Step 4: health score
    let model = model_str.parse::<token_metrics::ModelFamily>().unwrap_or_default();
    let health_opts = token_metrics::HealthOpts {
        model,
        window_size: 0,
        key_positions: token_metrics::key_positions_from_order(&files_used, &focus_files),
        signature_count: sig_count,
        signature_tokens: (total_tokens as f64 * 0.85) as usize,
    };
    let health = token_metrics::analyze(&context_text, &health_opts);

    let data = serde_json::json!({
        "context": context_text,
        "filesUsed": files_used,
        "focusFiles": focus_files,
        "totalTokens": total_tokens,
        "health": health,
    });

    result_to_json_ptr::<serde_json::Value>(Ok(data))
}

// ---------------------------------------------------------------------------
// FFI: Shotgun Surgery (co-change dispersion)
// ---------------------------------------------------------------------------

/// Return files ranked by co-change dispersion — the shotgun surgery smell.
///
/// `path`         — project root (C string)
/// `limit`        — commits to analyse (0 → 500)
/// `min_partners` — minimum distinct co-change partners (0 → 3)
///
/// Response shape:
/// ```json
/// { "ok": true, "data": [{ "file": "src/api.rs", "partnerCount": 12,
///   "totalCochanges": 47, "entropy": 3.58, "dispersionScore": 87.0 }] }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_shotgun_surgery(
    path: *const c_char,
    limit: u32,
    min_partners: u32,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let limit = if limit == 0 { 500 } else { limit as usize };
    let min_partners = if min_partners == 0 { 3 } else { min_partners as usize };

    let mut entries = git_analysis::git_cochange_dispersion(&path, limit);
    entries.retain(|e| e.partner_count >= min_partners);

    result_to_json_ptr(Ok::<_, String>(entries))
}

// ---------------------------------------------------------------------------
// FFI: Doc Index
// ---------------------------------------------------------------------------

/// Return all document-type nodes from the project graph.
///
/// Input:  `path` — project root (C string)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": [
///     {
///       "path": "docs/architecture.md",
///       "module_id": "docs/architecture.md",
///       "signatures": ["# Architecture", "## Overview"],
///       "imports": ["src/api.rs"],
///       "edge_count": 3
///     }
///   ]
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_doc_index(path: *const c_char) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let result = state.doc_nodes();
    result_to_json_ptr(result)
}

// ---------------------------------------------------------------------------
// FFI: Doc Context
// ---------------------------------------------------------------------------

/// Return a single document's structure plus skeletons of referenced code files.
///
/// Inputs:
///   `path`     — project root (C string)
///   `doc_path` — relative path to the document (C string)
///   `budget`   — max tokens for referenced code (0 → 4000)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "doc": { "path": "...", "moduleId": "...", "signatures": [...], "imports": [...] },
///     "referencedFiles": [{ "path": "...", "rank": 0.05, "signatures": [...] }],
///     "totalTokens": 2100
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_doc_context(
    path: *const c_char,
    doc_path: *const c_char,
    budget: u32,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    if doc_path.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null doc_path".into()));
    }
    let doc_path_str = unsafe {
        match CStr::from_ptr(doc_path).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };
    let budget = if budget == 0 { 4000 } else { budget as usize };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    if let Err(e) = state.rebuild_graph() {
        return result_to_json_ptr::<serde_json::Value>(Err(e));
    }

    // Find the doc in mapped_files (exact match or substring)
    let (module_id, doc_sigs, doc_imports, doc_path_owned) = {
        let files = state.mapped_files.lock().unwrap();
        match files.iter()
            .find(|(_, f)| f.path == doc_path_str || f.path.contains(&doc_path_str))
        {
            Some((mid, mf)) => (
                mid.clone(),
                mf.signatures.iter().map(|s| s.raw.clone()).collect::<Vec<String>>(),
                mf.imports.clone(),
                mf.path.clone(),
            ),
            None => return result_to_json_ptr::<serde_json::Value>(
                Err(format!("Document not found: {}", doc_path_str)),
            ),
        }
    };

    // Use doc's imports as focus for ranked skeleton
    let ranked = if doc_imports.is_empty() {
        vec![]
    } else {
        state.ranked_skeleton(&doc_imports, budget).unwrap_or_default()
    };

    let total_tokens: usize = ranked.iter().map(|f| f.estimated_tokens).sum();

    let referenced: Vec<serde_json::Value> = ranked.iter().map(|f| {
        serde_json::json!({
            "path": f.path,
            "rank": f.rank,
            "signatureCount": f.signature_count,
            "estimatedTokens": f.estimated_tokens,
            "signatures": f.signatures,
        })
    }).collect();

    let data = serde_json::json!({
        "doc": {
            "path": doc_path_owned,
            "moduleId": module_id,
            "signatures": doc_sigs,
            "imports": doc_imports,
        },
        "referencedFiles": referenced,
        "totalTokens": total_tokens,
    });

    result_to_json_ptr::<serde_json::Value>(Ok(data))
}

// ---------------------------------------------------------------------------
// FFI: Query Docs (doc-biased context retrieval)
// ---------------------------------------------------------------------------

/// Doc-biased context retrieval: search docs first, follow cross-refs into code.
///
/// Inputs:
///   `path`      — project root (C string)
///   `query`     — natural language query (C string)
///   `opts_json` — optional JSON: `{ "budget": 8000, "model": "claude" }`
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "context": "## Doc Context for: ...\n\n...",
///     "docFiles": [...],
///     "codeFiles": [...],
///     "focusDocs": ["docs/setup.md"],
///     "totalTokens": 5200,
///     "health": { "score": 81.0, "grade": "B", ... }
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_query_docs(
    path: *const c_char,
    query: *const c_char,
    opts_json: *const c_char,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    if query.is_null() {
        return result_to_json_ptr::<serde_json::Value>(Err("null query".into()));
    }
    let q = unsafe {
        match CStr::from_ptr(query).to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };

    #[derive(serde::Deserialize, Default)]
    #[serde(rename_all = "camelCase")]
    struct QueryDocsOpts {
        budget: Option<usize>,
        model: Option<String>,
    }

    let json_opts: QueryDocsOpts = if !opts_json.is_null() {
        let raw = unsafe {
            match CStr::from_ptr(opts_json).to_str() {
                Ok(s) => s,
                Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
            }
        };
        serde_json::from_str(raw).unwrap_or_default()
    } else {
        QueryDocsOpts::default()
    };

    let budget = json_opts.budget.unwrap_or(8000);
    let model_str = json_opts.model.unwrap_or_else(|| "claude".to_string());

    // Step 1: BM25 search across all files
    let bm25_opts = search::BM25Options { max_results: 30, ..Default::default() };
    let bm25_result = search::bm25_search(&path, &q, &bm25_opts).unwrap_or_default();

    // Step 2: Separate into doc files and code files
    let mut doc_files: Vec<String> = Vec::new();
    let mut code_files: Vec<String> = Vec::new();
    let mut seen = std::collections::HashSet::new();

    for m in &bm25_result.matches {
        if !seen.insert(m.path.clone()) { continue; }
        if api::is_doc_path(&m.path) {
            doc_files.push(m.path.clone());
        } else {
            code_files.push(m.path.clone());
        }
    }

    // Step 3: Build graph + follow doc cross-refs into code
    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    { let mut f = state.mapped_files.lock().unwrap(); *f = mapped_files; }
    if let Err(e) = state.rebuild_graph() {
        return result_to_json_ptr::<serde_json::Value>(Err(e));
    }

    {
        let files = state.mapped_files.lock().unwrap();
        for doc_path in &doc_files {
            if let Some(mf) = files.get(doc_path.as_str()) {
                for imp in &mf.imports {
                    if !seen.contains(imp) && !api::is_doc_path(imp) {
                        seen.insert(imp.clone());
                        code_files.push(imp.clone());
                    }
                }
            }
        }
    }

    // Step 4: Ranked skeleton — docs as primary focus, code as secondary
    let mut all_focus = doc_files.clone();
    all_focus.extend(code_files.iter().cloned());
    all_focus.truncate(30);

    let ranked = state.ranked_skeleton(&all_focus, budget).unwrap_or_default();

    // Step 5: Build context text — docs first, then code
    let mut doc_entries = Vec::new();
    let mut code_entries = Vec::new();
    let mut context_text = format!("## Doc Context for: {}\n\n", q);
    let mut total_tokens = 0usize;

    for f in &ranked {
        let entry = serde_json::json!({
            "path": f.path,
            "rank": f.rank,
            "signatureCount": f.signature_count,
            "estimatedTokens": f.estimated_tokens,
            "signatures": f.signatures,
        });
        total_tokens += f.estimated_tokens;

        if api::is_doc_path(&f.path) {
            context_text.push_str(&format!(
                "// [DOC] {} (rank: {:.4}, {} tokens)\n", f.path, f.rank, f.estimated_tokens
            ));
            doc_entries.push(entry);
        } else {
            context_text.push_str(&format!(
                "// {} (rank: {:.4}, {} tokens)\n", f.path, f.rank, f.estimated_tokens
            ));
            code_entries.push(entry);
        }
        for sig in &f.signatures {
            context_text.push_str(&format!("  {}\n", sig));
        }
        context_text.push('\n');
    }

    // Step 6: Health score
    let sig_count: usize = ranked.iter().map(|f| f.signatures.len()).sum();
    let model = model_str.parse::<token_metrics::ModelFamily>().unwrap_or_default();
    let health_opts = token_metrics::HealthOpts {
        model,
        window_size: 0,
        key_positions: token_metrics::key_positions_from_order(
            &ranked.iter().map(|f| f.path.clone()).collect::<Vec<_>>(),
            &doc_files,
        ),
        signature_count: sig_count,
        signature_tokens: (total_tokens as f64 * 0.85) as usize,
    };
    let health = token_metrics::analyze(&context_text, &health_opts);

    let data = serde_json::json!({
        "context": context_text,
        "docFiles": doc_entries,
        "codeFiles": code_entries,
        "focusDocs": doc_files,
        "totalTokens": total_tokens,
        "health": health,
    });

    result_to_json_ptr::<serde_json::Value>(Ok(data))
}

// ---------------------------------------------------------------------------
// FFI: Render Architecture Diagram (Mermaid / DOT)
// ---------------------------------------------------------------------------

/// Render the project's import graph as a Mermaid or Graphviz (DOT) diagram.
///
/// Inputs:
///   `path`      — project root (C string)
///   `format`    — "mermaid" or "dot" (C string; may be null → "mermaid")
///   `focus`     — optional module_id or path to anchor BFS on (C string, may
///                 be null → top-N by degree)
///   `depth`     — BFS depth when `focus` is set (0 → 2; ignored without focus)
///   `max_nodes` — cap on nodes in the output (0 → 40)
///
/// Response shape:
/// ```json
/// {
///   "ok": true,
///   "data": {
///     "diagram":   "graph TD\n    N0[...] --> N1[...]\n...",
///     "truncated": false,
///     "format":    "mermaid",
///     "nodeCount": 23
///   }
/// }
/// ```
#[no_mangle]
pub extern "C" fn codecartographer_render_architecture(
    path: *const c_char,
    format: *const c_char,
    focus: *const c_char,
    depth: u32,
    max_nodes: u32,
) -> *mut c_char {
    let path = match c_str_to_path(path) {
        Ok(p) => p,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let format_str = if format.is_null() {
        "mermaid".to_string()
    } else {
        match unsafe { CStr::from_ptr(format) }.to_str() {
            Ok(s) => s.to_string(),
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };
    let fmt = match diagram::DiagramFormat::parse(&format_str) {
        Ok(f) => f,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let focus_str = if focus.is_null() {
        None
    } else {
        match unsafe { CStr::from_ptr(focus) }.to_str() {
            Ok(s) if !s.is_empty() => Some(s.to_string()),
            Ok(_) => None,
            Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e.to_string())),
        }
    };

    let depth = if depth == 0 { 2 } else { depth as usize };
    let max_nodes = if max_nodes == 0 { 40 } else { max_nodes as usize };

    let mapped_files = match build_mapped_files(&path) {
        Ok(m) => m,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };
    let state = ApiState::new(path.clone());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let graph = match state.rebuild_graph() {
        Ok(g) => g,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let opts = diagram::RenderOptions {
        format: fmt,
        focus: focus_str.as_deref(),
        depth,
        max_nodes,
        // Co-change + blast-radius overlays aren't plumbed through this FFI
        // yet — keep the signature stable and expose new overlays via CLI
        // first. A v2 FFI entry can add them without breaking callers.
        show_cochange: None,
        blast_radius: None,
        docs_only: false,
        group_by_folder_depth: None,
        color_by_owner: false,
    };
    let rendered = match diagram::render(&graph, &opts) {
        Ok(r) => r,
        Err(e) => return result_to_json_ptr::<serde_json::Value>(Err(e)),
    };

    let format_name = match fmt {
        diagram::DiagramFormat::Mermaid => "mermaid",
        diagram::DiagramFormat::Dot => "dot",
        diagram::DiagramFormat::Ascii => "ascii",
        diagram::DiagramFormat::Sequence => "sequence",
        diagram::DiagramFormat::Class => "class",
        diagram::DiagramFormat::Quadrant => "quadrant",
        diagram::DiagramFormat::Er => "er",
    };

    let data = serde_json::json!({
        "diagram": rendered.diagram,
        "truncated": rendered.truncated,
        "format": format_name,
        "nodeCount": rendered.node_count,
    });

    result_to_json_ptr::<serde_json::Value>(Ok(data))
}
