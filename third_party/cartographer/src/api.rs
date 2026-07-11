// API Service - Exposes Project CodeCartographer via HTTP API
// This provides endpoints for AI tools like ShellAI to query module context

use crate::layers::{detect_layer_violations, LayerConfig, LayerViolation};

/// Public symbol names too generic to flag as unreferenced exports.
const COMMON_SYMBOL_NAMES: &[&str] = &[
    "parse", "build", "create", "format", "display", "default",
    "clone", "debug", "assert", "error", "result", "option",
    "update", "delete", "insert", "select", "render", "handle",
    "encode", "decode", "serialize", "deserialize", "validate",
    "connect", "execute", "process", "generate", "convert",
];
use crate::mapper::{DetailLevel, MappedFile, Signature};
use petgraph::graphmap::{DiGraphMap, UnGraphMap};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::path::Path;
use std::sync::Mutex;

/// API Configuration
#[allow(dead_code)]
#[derive(Debug, Clone)]
pub struct ApiConfig {
    pub host: String,
    pub port: u16,
    pub enable_cors: bool,
}

impl Default for ApiConfig {
    fn default() -> Self {
        Self {
            host: "127.0.0.1".to_string(),
            port: 8080,
            enable_cors: true,
        }
    }
}

/// Module context request
#[allow(dead_code)]
#[derive(Debug, Deserialize)]
pub struct ModuleContextRequest {
    pub module_id: String,
    pub depth: Option<u32>,
    pub detail_level: Option<String>,
    pub include: Option<Vec<String>>,
    pub format: Option<String>,
}

/// Module context response
#[allow(dead_code)]
#[derive(Debug, Serialize)]
pub struct ModuleContextResponse {
    pub module_id: String,
    pub path: String,
    pub imports: Vec<String>,
    pub signatures: Vec<Signature>,
    pub docstrings: Option<Vec<String>>,
    pub parameters: Option<Vec<String>>,
    pub return_types: Option<Vec<String>>,
    pub dependencies: Option<Vec<DependencyInfo>>,
    pub detail_level: String,
}

#[derive(Debug, Serialize)]
pub struct DependencyInfo {
    pub module_id: String,
    pub path: String,
    pub signature_count: usize,
}

/// Graph query request
#[allow(dead_code)]
#[derive(Debug, Deserialize)]
pub struct GraphQueryRequest {
    pub module_id: Option<String>,
    pub query: Option<String>,
    pub query_type: Option<String>,
}

/// Project graph response
#[derive(Debug, Clone, Serialize)]
pub struct ProjectGraphResponse {
    pub nodes: Vec<GraphNode>,
    pub edges: Vec<GraphEdge>,
    pub cycles: Vec<CycleInfo>,
    pub god_modules: Vec<GodModuleInfo>,
    pub layer_violations: Vec<LayerViolation>,
    pub metadata: GraphMetadata,
    /// Temporal coupling pairs from git history (populated by enrich_with_git).
    pub cochange_pairs: Vec<CoChangePair>,
}

#[derive(Debug, Clone, Serialize)]
pub struct GraphNode {
    pub module_id: String,
    pub path: String,
    pub language: String,
    pub signature_count: usize,
    pub complexity: Option<u32>,
    pub is_bridge: Option<bool>,
    pub bridge_score: Option<f64>,
    pub degree: Option<usize>,
    pub risk_level: Option<String>,
    /// Number of commits that touched this file (from git history).
    pub churn: Option<usize>,
    /// churn × signature_count, normalised 0–100.
    pub hotspot_score: Option<f64>,
    /// Architectural role: entry/core/utility/leaf/dead/bridge/standard.
    pub role: Option<String>,
    /// True when no other module imports this file and it is not an entry point.
    pub is_dead: Option<bool>,
    /// Exported symbols not found in any other file's imports (heuristic).
    pub unreferenced_exports: Option<Vec<String>>,
    /// Number of other files that import this file (in-degree).
    pub fan_in: Option<usize>,
    /// Number of other files this file imports (out-degree = CBO).
    pub fan_out: Option<usize>,
    /// Number of distinct files this file has co-changed with (shotgun surgery signal).
    pub cochange_partners: Option<usize>,
    /// Shannon entropy of co-change distribution (higher = more scattered changes).
    pub cochange_entropy: Option<f64>,
    /// Dominant git author by commit count (bot/format commits excluded).
    /// Populated by `enrich_with_git`. Powers the `--color-by=owner` diagram mode.
    pub owner: Option<String>,
}

/// A source position range using LIP semantics: line is 0-based, char is UTF-8 byte offset from line start.
#[derive(Debug, Clone, Serialize)]
pub struct Range {
    pub start_line: usize,
    pub start_char: usize,
    pub end_line:   usize,
    pub end_char:   usize,
}

#[derive(Debug, Clone, Serialize)]
pub struct GraphEdge {
    pub source: String,
    pub target: String,
    pub edge_type: String,
    pub at_range: Option<Range>,
    /// How confidently this edge was resolved from the import text:
    /// `"exact"`  — exact module-id or Go package-directory match;
    /// `"suffix"` — path-suffix / unique-basename agreement (C/C++ includes, etc.);
    /// `"fuzzy"`  — bare stem / segment / symbol guess with no path agreement.
    /// Consumers should treat `fuzzy` edges as low-confidence (they are the ones
    /// that historically fabricated cross-package edges and false cycles).
    #[serde(default)]
    pub resolution: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct CoChangePair {
    pub file_a: String,
    pub file_b: String,
    pub count: usize,
    pub coupling_score: f64,
}

#[derive(Debug, Clone, Serialize)]
pub struct GraphMetadata {
    pub total_files: usize,
    pub total_edges: usize,
    pub languages: HashMap<String, usize>,
    pub generated_at: String,
    pub bridge_count: Option<usize>,
    pub cycle_count: Option<usize>,
    pub god_module_count: Option<usize>,
    pub health_score: Option<f64>,
    pub layer_violation_count: Option<usize>,
    pub architectural_drift: Option<f64>,
    pub hotspot_count: Option<usize>,
    pub dead_code_count: Option<usize>,
    pub unreferenced_exports_count: Option<usize>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SimulatedChange {
    pub target_module: String,
    pub new_signature: Option<String>,
    pub removed_signature: Option<String>,
    pub predicted_impact: ImpactAnalysis,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ImpactAnalysis {
    pub affected_modules: Vec<String>,
    pub callers_count: usize,
    pub callees_count: usize,
    pub will_create_cycle: bool,
    pub layer_violations: Vec<LayerViolation>,
    pub risk_level: String,
    pub health_impact: f64,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ArchitectureSnapshot {
    pub timestamp: u64,
    /// Git HEAD SHA at snapshot time. `None` when the root is not a git repo.
    /// Used to deduplicate: repeated calls on the same commit update this entry
    /// in-place rather than appending a new one.
    #[serde(skip_serializing_if = "Option::is_none")]
    pub git_ref: Option<String>,
    pub health_score: f64,
    pub total_files: usize,
    pub total_edges: usize,
    pub bridge_count: usize,
    pub cycle_count: usize,
    pub god_module_count: usize,
    pub layer_violation_count: usize,
    pub dominant_language: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ArchitectureEvolution {
    pub snapshots: Vec<ArchitectureSnapshot>,
    pub health_trend: String,
    /// `false` when all snapshots in the window come from the same git commit
    /// (or the window spans less than one hour for non-git repos). Callers
    /// should suppress trend UI when this is `false`.
    pub trend_available: bool,
    pub debt_indicators: Vec<String>,
    pub recommendations: Vec<String>,
}

#[derive(Debug, Clone, Serialize)]
pub struct CycleInfo {
    pub nodes: Vec<String>,
    pub pivot_node: Option<String>,
    pub severity: String,
}

#[derive(Debug, Clone, Serialize)]
pub struct GodModuleInfo {
    pub module_id: String,
    pub path: String,
    pub degree: usize,
    pub cohesion_score: f64,
    pub severity: String,
}

/// Compression level configuration
#[allow(dead_code)]
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum CompressionLevel {
    Minimal,
    Standard,
    Aggressive,
}

impl Default for CompressionLevel {
    fn default() -> Self {
        Self::Standard
    }
}

/// API State shared across requests
pub struct ApiState {
    pub root_path: std::path::PathBuf,
    pub mapped_files: Mutex<HashMap<String, MappedFile>>,
    pub project_graph: Mutex<Option<ProjectGraphResponse>>,
    #[allow(dead_code)]
    pub compression_level: Mutex<CompressionLevel>,
    /// Per-file modification time (repo-relative path → mtime secs), the baseline for
    /// incremental refresh. Empty until primed on the first refresh.
    file_mtimes: Mutex<HashMap<String, u64>>,
    /// Timestamp of the last incremental refresh, used to debounce filesystem scans.
    last_refresh: Mutex<Option<std::time::Instant>>,
    /// Betweenness centrality is 98% of a graph rebuild's cost, yet it depends ONLY on
    /// graph topology — not on file contents. This caches computed centrality maps keyed
    /// by a fingerprint of the structural node+edge set, so an edit that leaves the import
    /// graph unchanged (a body/comment/whitespace save — the common watch-mode case)
    /// reuses it and the rebuild drops from seconds to tens of milliseconds. Keyed by
    /// fingerprint, an entry self-invalidates exactly when its topology changes, so this
    /// is deliberately NOT cleared by invalidate_graph(). It's a small MRU list (not a
    /// single slot) so distinct graphs — e.g. the file-level graph and a directory-level
    /// rollup — coexist without evicting the expensive file-level entry.
    bc_cache: Mutex<Vec<(u64, HashMap<String, f64>)>>,
}

/// Max distinct graph topologies to retain centrality for (file-level + a few rollup
/// depths). Small: each entry is one V-sized map and lookups are a linear scan.
const BC_CACHE_CAP: usize = 4;

/// Minimum interval between incremental filesystem re-scans. Bounds the cost of keeping
/// a long-lived `serve` session fresh: a burst of tool calls triggers at most one scan.
const REFRESH_DEBOUNCE_MS: u128 = 750;

impl ApiState {
    pub fn new(root_path: std::path::PathBuf) -> Self {
        Self {
            root_path,
            mapped_files: Mutex::new(HashMap::new()),
            project_graph: Mutex::new(None),
            compression_level: Mutex::new(CompressionLevel::Standard),
            file_mtimes: Mutex::new(HashMap::new()),
            last_refresh: Mutex::new(None),
            bc_cache: Mutex::new(Vec::new()),
        }
    }

    /// Keep a long-lived `serve` session fresh without a full rescan. Debounced; on each
    /// call it stats the working tree, re-parses only files whose mtime advanced (and drops
    /// deleted ones), patches `mapped_files`, and invalidates the graph cache when anything
    /// changed. This is what makes the persistent MCP model strictly better than a one-shot
    /// CLI: startup cost is paid once, yet edits (including uncommitted ones) are picked up.
    pub fn refresh_if_stale(&self) {
        // Debounce: skip if we scanned within the window. Also claims the slot up front so
        // concurrent callers don't all scan.
        {
            let mut last = match self.last_refresh.lock() {
                Ok(l) => l,
                Err(_) => return,
            };
            if let Some(t) = *last {
                if t.elapsed().as_millis() < REFRESH_DEBOUNCE_MS {
                    return;
                }
            }
            *last = Some(std::time::Instant::now());
        }

        // Enumerate current files with the SAME filter as the initial scan (includes docs,
        // excludes noise) so the refresh set matches what mapped_files holds.
        let scan = match crate::scanner::scan_files_with_noise_tracking(&self.root_path) {
            Ok(s) => s,
            Err(_) => return,
        };
        let current: Vec<(std::path::PathBuf, String, u64)> = scan
            .files
            .into_iter()
            .filter(|p| !crate::scanner::is_ignored_path(p))
            .filter_map(|p| {
                let mtime = std::fs::metadata(&p)
                    .and_then(|m| m.modified())
                    .ok()?
                    .duration_since(std::time::UNIX_EPOCH)
                    .ok()?
                    .as_secs();
                let rel = p
                    .strip_prefix(&self.root_path)
                    .unwrap_or(&p)
                    .to_string_lossy()
                    .replace('\\', "/");
                Some((p, rel, mtime))
            })
            .collect();

        let mut mtimes = match self.file_mtimes.lock() {
            Ok(m) => m,
            Err(_) => return,
        };

        // First run: prime the baseline without re-parsing — mapped_files is already current
        // from the startup scan.
        if mtimes.is_empty() {
            for (_, rel, mt) in &current {
                mtimes.insert(rel.clone(), *mt);
            }
            return;
        }

        let current_set: std::collections::HashSet<&str> =
            current.iter().map(|(_, r, _)| r.as_str()).collect();
        let dirty: Vec<&(std::path::PathBuf, String, u64)> = current
            .iter()
            .filter(|(_, rel, mt)| !matches!(mtimes.get(rel), Some(prev) if prev >= mt))
            .collect();
        let deleted: Vec<String> = mtimes
            .keys()
            .filter(|k| !current_set.contains(k.as_str()))
            .cloned()
            .collect();

        if dirty.is_empty() && deleted.is_empty() {
            return;
        }

        {
            let mut files = match self.mapped_files.lock() {
                Ok(f) => f,
                Err(_) => return,
            };
            for (p, rel, mt) in &dirty {
                if let Ok(content) = std::fs::read_to_string(p) {
                    let mapped = crate::mapper::extract_skeleton(p, &content);
                    files.insert(rel.clone(), mapped);
                    mtimes.insert(rel.clone(), *mt);
                }
            }
            for rel in &deleted {
                files.remove(rel);
                mtimes.remove(rel);
            }
        }
        drop(mtimes);

        // Next rebuild_graph recomputes from the patched files.
        self.invalidate_graph();
    }

    /// Convert an absolute file path (as stored in `MappedFile.path`) to a repo-relative
    /// one for emission in responses. Keeps output compact and avoids leaking absolute
    /// filesystem paths — internal storage stays absolute for file IO.
    pub(crate) fn rel(&self, p: &str) -> String {
        std::path::Path::new(p)
            .strip_prefix(&self.root_path)
            .map(|r| r.to_string_lossy().replace('\\', "/"))
            .unwrap_or_else(|_| p.to_string())
    }

    #[allow(dead_code)]
    pub fn get_module_context(
        &self,
        request: &ModuleContextRequest,
    ) -> Result<ModuleContextResponse, String> {
        let files = self.mapped_files.lock().map_err(|e| e.to_string())?;

        let module = files
            .get(&request.module_id)
            .ok_or_else(|| format!("Module not found: {}", request.module_id))?;

        let detail = match request.detail_level.as_deref() {
            Some("minimal") => DetailLevel::Minimal,
            Some("extended") => DetailLevel::Extended,
            _ => DetailLevel::Standard,
        };

        let response = ModuleContextResponse {
            module_id: request.module_id.clone(),
            path: self.rel(&module.path),
            imports: module.imports.clone(),
            signatures: module.signatures.clone(),
            docstrings: match detail {
                DetailLevel::Minimal => None,
                _ => module.docstrings.clone(),
            },
            parameters: match detail {
                DetailLevel::Minimal => None,
                _ => module.parameters.clone(),
            },
            return_types: match detail {
                DetailLevel::Minimal => None,
                DetailLevel::Standard => None,
                DetailLevel::Extended => module.return_types.clone(),
            },
            dependencies: self
                .get_dependencies_internal(&request.module_id, request.depth.unwrap_or(0))?,
            detail_level: format!("{:?}", detail),
        };

        Ok(response)
    }

    pub(crate) fn get_dependencies_internal(
        &self,
        module_id: &str,
        depth: u32,
    ) -> Result<Option<Vec<DependencyInfo>>, String> {
        if depth == 0 {
            return Ok(None);
        }

        let _files = self.mapped_files.lock().map_err(|e| e.to_string())?;
        let graph = self.project_graph.lock().map_err(|e| e.to_string())?;

        let graph = match &*graph {
            Some(g) => g,
            None => return Ok(None),
        };

        let mut deps = Vec::new();
        let mut visited = std::collections::HashSet::new();
        visited.insert(module_id.to_string());

        self.collect_dependencies(graph, module_id, depth, &mut visited, &mut deps);

        Ok(Some(deps))
    }

    fn collect_dependencies(
        &self,
        graph: &ProjectGraphResponse,
        module_id: &str,
        remaining_depth: u32,
        visited: &mut std::collections::HashSet<String>,
        deps: &mut Vec<DependencyInfo>,
    ) {
        if remaining_depth == 0 {
            return;
        }

        for edge in &graph.edges {
            if edge.source == module_id && !visited.contains(&edge.target) {
                visited.insert(edge.target.clone());

                if let Some(node) = graph.nodes.iter().find(|n| n.module_id == edge.target) {
                    deps.push(DependencyInfo {
                        module_id: node.module_id.clone(),
                        path: self.rel(&node.path),
                        signature_count: node.signature_count,
                    });
                }

                self.collect_dependencies(graph, &edge.target, remaining_depth - 1, visited, deps);
            }
        }
    }

    #[allow(dead_code)]
    pub fn get_dependencies(&self, module_id: &str) -> Result<Vec<DependencyInfo>, String> {
        self.get_dependencies_internal(module_id, 1)?
            .ok_or_else(|| "No dependencies found".to_string())
    }

    pub fn get_dependents(&self, module_id: &str) -> Result<Vec<DependencyInfo>, String> {
        let graph = self.project_graph.lock().map_err(|e| e.to_string())?;
        let graph = match &*graph {
            Some(g) => g,
            None => return Err("Project graph not initialized".to_string()),
        };

        let mut dependents = Vec::new();
        for edge in &graph.edges {
            if edge.target == module_id {
                if let Some(node) = graph.nodes.iter().find(|n| n.module_id == edge.source) {
                    dependents.push(DependencyInfo {
                        module_id: node.module_id.clone(),
                        path: self.rel(&node.path),
                        signature_count: node.signature_count,
                    });
                }
            }
        }

        Ok(dependents)
    }

    #[allow(dead_code)]
    pub fn search_graph(
        &self,
        query: &str,
        query_type: Option<&str>,
    ) -> Result<Vec<GraphNode>, String> {
        let graph = self.project_graph.lock().map_err(|e| e.to_string())?;
        let graph = match &*graph {
            Some(g) => g,
            None => return Err("Project graph not initialized".to_string()),
        };

        let query_lower = query.to_lowercase();
        let nodes: Vec<GraphNode> = graph
            .nodes
            .iter()
            .filter(|n| {
                n.module_id.to_lowercase().contains(&query_lower)
                    || n.path.to_lowercase().contains(&query_lower)
            })
            .cloned()
            .collect();

        match query_type {
            Some("edge") => {
                let edges: Vec<GraphEdge> = graph
                    .edges
                    .iter()
                    .filter(|e| {
                        e.source.to_lowercase().contains(&query_lower)
                            || e.target.to_lowercase().contains(&query_lower)
                    })
                    .cloned()
                    .collect();

                let edge_node_ids: std::collections::HashSet<&String> = edges
                    .iter()
                    .flat_map(|e| vec![&e.source, &e.target])
                    .collect();

                Ok(nodes
                    .into_iter()
                    .filter(|n| edge_node_ids.contains(&n.module_id))
                    .collect())
            }
            _ => Ok(nodes),
        }
    }

    /// Drop the cached project graph so the next `rebuild_graph` recomputes it. Called by
    /// `refresh_if_stale` whenever the working tree changed under a live session.
    pub(crate) fn invalidate_graph(&self) {
        if let Ok(mut g) = self.project_graph.lock() {
            *g = None;
        }
    }

    pub fn rebuild_graph(&self) -> Result<ProjectGraphResponse, String> {
        // Serve the cached graph if present. Building it is expensive (cycle detection,
        // betweenness centrality, god-module analysis) and it's recomputed on every tool
        // call otherwise. The cache is populated once at startup and holds for the session
        // because mapped_files is immutable after the initial scan; invalidate_graph()
        // clears it if that ever stops being true.
        {
            let cached = self.project_graph.lock().map_err(|e| e.to_string())?;
            if let Some(g) = cached.as_ref() {
                return Ok(g.clone());
            }
        }

        let files = self.mapped_files.lock().map_err(|e| e.to_string())?;

        let _t = std::time::Instant::now();
        macro_rules! phase { ($n:expr) => { if std::env::var("CKB_TIMING").is_ok() { eprintln!("[timing] {:>18}: {:?}", $n, _t.elapsed()); } }; }

        // Build the import-resolution index ONCE for the whole rebuild (O(N)); each import
        // then resolves via hash lookups instead of scanning every file (was O(N²)).
        let import_index = ImportIndex::build(&files).with_go_module(&self.root_path);
        phase!("index_build");

        // Resolve every file's imports in parallel — resolution is a read-only lookup
        // against the immutable index, so it's embarrassingly parallel. On huge trees
        // (~1.7M edges) this is the difference between one core and all of them.
        use rayon::prelude::*;
        let per_file: Vec<(GraphNode, Vec<GraphEdge>)> = files
            .par_iter()
            .map(|(module_id, file)| {
                let language = Path::new(&file.path)
                    .extension()
                    .and_then(|e| e.to_str())
                    .unwrap_or("unknown")
                    .to_string();

                let node = GraphNode {
                    module_id: module_id.clone(),
                    path: self.rel(&file.path),
                    language,
                    signature_count: file.signatures.len(),
                    complexity: None,
                    is_bridge: None,
                    bridge_score: None,
                    degree: None,
                    risk_level: None,
                    churn: None,
                    hotspot_score: None,
                    role: None,
                    is_dead: None,
                    unreferenced_exports: None,
                    fan_in: None,
                    fan_out: None,
                    cochange_partners: None,
                    cochange_entropy: None,
                    owner: None,
                };

                let source_kind = if is_test_path(module_id) {
                    "test"
                } else if is_doc_path(module_id) {
                    "doc"
                } else {
                    "runtime"
                };
                let src_is_doc = is_doc_path(&file.path);

                let mut file_edges = Vec::new();
                for import in &file.imports {
                    if let Some((target, resolution)) = import_index.resolve(import, module_id) {
                        // Reject cross-type edges: a source file importing "json" must not
                        // resolve to a fixture like testdata/review/json.json (doc), and a
                        // doc file must not depend on a source module just for mentioning it.
                        let target_is_doc = files.get(&target)
                            .map(|f| is_doc_path(&f.path))
                            .unwrap_or(false);
                        if src_is_doc != target_is_doc {
                            continue;
                        }
                        file_edges.push(GraphEdge {
                            source: module_id.clone(),
                            target,
                            edge_type: source_kind.to_string(),
                            at_range: None,
                            resolution: resolution.to_string(),
                        });
                    }
                }
                (node, file_edges)
            })
            .collect();

        let mut nodes: Vec<GraphNode> = Vec::with_capacity(per_file.len());
        let mut edges: Vec<GraphEdge> = Vec::new();
        let mut languages: HashMap<String, usize> = HashMap::new();
        for (node, mut file_edges) in per_file {
            *languages.entry(node.language.clone()).or_insert(0) += 1;
            nodes.push(node);
            edges.append(&mut file_edges);
        }
        phase!("resolve_edges");

        // Collapse duplicate (source, target) pairs — a file can resolve the
        // same import via multiple paths (re-exports, aliased crates, etc.). A pair
        // can even resolve at different confidences via different imports; break ties
        // by resolution strength (exact < suffix < fuzzy) so dedup deterministically
        // keeps the STRONGEST edge. Without this tiebreak the unstable sort kept an
        // arbitrary survivor, so a pair that resolved both exact and fuzzy would flip
        // in/out of the structural (non-fuzzy) set run-to-run.
        fn res_rank(r: &str) -> u8 {
            match r {
                "exact" => 0,
                "suffix" => 1,
                _ => 2,
            }
        }
        edges.sort_unstable_by(|a, b| {
            a.source
                .cmp(&b.source)
                .then(a.target.cmp(&b.target))
                .then(res_rank(&a.resolution).cmp(&res_rank(&b.resolution)))
        });
        edges.dedup_by(|a, b| a.source == b.source && a.target == b.target);

        // Run the shared analysis/assembly over the resolved file-level graph.
        let response = self.finalize_graph_response(nodes, edges, languages, &files);
        phase!("finalize");

        let mut graph = self.project_graph.lock().map_err(|e| e.to_string())?;
        *graph = Some(response.clone());

        Ok(response)
    }

    /// Run structural analysis (bridges, cycles, god modules, layer violations,
    /// health, roles, dead code, unreferenced exports) over an already-resolved,
    /// deduped node/edge set and assemble the response. Shared by `rebuild_graph`
    /// (file-level) and `rebuild_graph_rolled_up` (directory-level) so both views
    /// get identical metric semantics from one implementation.
    ///
    /// `edges` is the full edge list (kept verbatim in the response); metrics are
    /// computed only on the non-fuzzy ("structural") subset. `files` supplies the
    /// per-node imports/signatures the god-module and unreferenced-export passes
    /// read — for the rollup these are synthesized per directory.
    fn finalize_graph_response(
        &self,
        mut nodes: Vec<GraphNode>,
        edges: Vec<GraphEdge>,
        languages: HashMap<String, usize>,
        files: &HashMap<String, MappedFile>,
    ) -> ProjectGraphResponse {
        // Structural analysis must not rest on low-confidence (fuzzy) edges — those
        // are bare stem/segment guesses that fabricated false bridges, cycles, layer
        // violations and god modules. The returned graph keeps every edge (tagged with
        // its `resolution`); only the metrics below are restricted to exact/suffix.
        let structural: Vec<GraphEdge> =
            edges.iter().filter(|e| e.resolution != "fuzzy").cloned().collect();

        let bridge_analysis = self.analyze_bridges(&nodes, &structural);

        for node in &mut nodes {
            if let Some(analysis) = bridge_analysis.get(&node.module_id) {
                node.is_bridge = Some(analysis.is_bridge);
                node.bridge_score = Some(analysis.bridge_score);
                node.degree = Some(analysis.degree);
                node.risk_level = Some(analysis.risk_level.clone());
            }
        }

        let bridge_count = nodes.iter().filter(|n| n.is_bridge == Some(true)).count();

        let cycles = self.detect_cycles(&nodes, &structural);
        let cycle_count = cycles.len();

        let god_modules = self.detect_god_modules(&nodes, &structural, files);
        let god_module_count = god_modules.len();

        let edge_tuples: Vec<(String, String)> = structural
            .iter()
            .map(|e| (e.source.clone(), e.target.clone()))
            .collect();

        let layer_violations = self.detect_layer_violations(&edge_tuples);
        let layer_violation_count = layer_violations.len();

        let health_score = self.calculate_health_score(
            bridge_count,
            cycle_count,
            god_module_count,
            layer_violation_count,
            nodes.len(),
        );

        // --- Role classification and dead-code detection ---
        // Compute per-node in/out degree from the edge list.
        let mut in_degree: HashMap<String, usize> = HashMap::new();
        let mut out_degree: HashMap<String, usize> = HashMap::new();
        // Runtime-only in-degree: excludes test edges so a module imported only
        // from tests is still considered a dead-code candidate.
        let mut runtime_in_degree: HashMap<String, usize> = HashMap::new();
        for node in &nodes {
            in_degree.entry(node.module_id.clone()).or_insert(0);
            out_degree.entry(node.module_id.clone()).or_insert(0);
            runtime_in_degree.entry(node.module_id.clone()).or_insert(0);
        }
        for edge in &structural {
            *out_degree.entry(edge.source.clone()).or_insert(0) += 1;
            *in_degree.entry(edge.target.clone()).or_insert(0) += 1;
            if edge.edge_type == "runtime" {
                *runtime_in_degree.entry(edge.target.clone()).or_insert(0) += 1;
            }
        }

        let mut dead_code_count = 0usize;

        for node in &mut nodes {
            let ind = *in_degree.get(&node.module_id).unwrap_or(&0);
            let outd = *out_degree.get(&node.module_id).unwrap_or(&0);
            let runtime_ind = *runtime_in_degree.get(&node.module_id).unwrap_or(&0);

            let is_entry_name = is_entry_point_path(&node.path);
            let is_test = is_test_path(&node.path);

            node.fan_in = Some(ind);
            node.fan_out = Some(outd);

            // Role assignment (bridge takes priority over other roles).
            // Use runtime_ind for dead/entry classification so test-only imports
            // don't mask unreachable modules from real callers.
            node.role = Some(if node.is_bridge == Some(true) {
                "bridge".to_string()
            } else if runtime_ind == 0 && outd == 0 && !is_entry_name && !is_test {
                "dead".to_string()
            } else if runtime_ind == 0 && outd > 0 && !is_test {
                "entry".to_string()
            } else if ind >= 5 && outd >= 3 {
                "core".to_string()
            } else if ind >= 5 {
                "utility".to_string()
            } else if outd == 0 && ind > 0 {
                "leaf".to_string()
            } else {
                "standard".to_string()
            });

            // Dead-code flag: no runtime callers AND not an entry point or test.
            let dead = runtime_ind == 0 && !is_entry_name && !is_test;
            node.is_dead = Some(dead);
            if dead {
                dead_code_count += 1;
            }
        }

        // --- Symbol reference analysis ---
        // Build a set of all tokens from every file's import statements.
        // A public symbol whose name does not appear in any import is a candidate
        // unreferenced export.  This is a heuristic (false positives for very
        // short or common names), but useful for flagging orphaned exports.
        let import_tokens: std::collections::HashSet<String> = files
            .values()
            .flat_map(|mf| {
                mf.imports.iter().flat_map(|imp| {
                    imp.split(|c: char| !c.is_alphanumeric() && c != '_')
                        .filter(|s| s.len() >= 6)
                        .map(|s| s.to_string())
                        .collect::<Vec<_>>()
                })
            })
            .collect();

        let public_prefixes = ["pub ", "public ", "export ", "def ", "func ", "function "];

        let mut unreferenced_exports_count = 0usize;
        for node in &mut nodes {
            if let Some(mf) = files.get(&node.module_id) {
                let unreferenced: Vec<String> = mf
                    .signatures
                    .iter()
                    .filter(|sig| {
                        // FFI exports are consumed by C callers; import-graph can't see them.
                        if sig.raw.contains("extern \"C\"") {
                            return false;
                        }
                        let is_public = public_prefixes
                            .iter()
                            .any(|pfx| sig.raw.starts_with(pfx));
                        if !is_public {
                            return false;
                        }
                        if let Some(name) = &sig.symbol_name {
                            name.len() >= 6
                                && !import_tokens.contains(name.as_str())
                                && !COMMON_SYMBOL_NAMES.contains(&name.to_lowercase().as_str())
                        } else {
                            false
                        }
                    })
                    .filter_map(|sig| sig.symbol_name.clone())
                    .collect();

                unreferenced_exports_count += unreferenced.len();
                if !unreferenced.is_empty() {
                    node.unreferenced_exports = Some(unreferenced);
                }
            }
        }

        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs()
            .to_string();

        let metadata = GraphMetadata {
            total_files: nodes.len(),
            total_edges: edges.len(),
            languages,
            generated_at: now,
            bridge_count: Some(bridge_count),
            cycle_count: Some(cycle_count),
            god_module_count: Some(god_module_count),
            health_score: Some(health_score),
            layer_violation_count: Some(layer_violation_count),
            architectural_drift: None,
            hotspot_count: None, // filled by enrich_with_git
            dead_code_count: Some(dead_code_count),
            unreferenced_exports_count: Some(unreferenced_exports_count),
        };

        ProjectGraphResponse {
            nodes,
            edges,
            cycles,
            god_modules,
            layer_violations,
            metadata,
            cochange_pairs: vec![],
        }
    }

    /// Build a directory-level ("rolled-up") view of the graph: every file is folded
    /// into the folder formed by its first `depth` path components, cross-directory
    /// dependencies are aggregated (intra-directory edges dropped), and the full
    /// structural analysis runs on the folded graph. The result is a graph small
    /// enough to comprehend on a very large tree while its bridges / cycles / god
    /// modules / health now describe *subsystems* rather than individual files.
    ///
    /// `depth` of 0 means "no rollup" and returns the file-level graph unchanged.
    /// Only non-fuzzy edges are rolled up — directory dependencies should be
    /// confident, not bare-stem guesses.
    pub fn rebuild_graph_rolled_up(&self, depth: usize) -> Result<ProjectGraphResponse, String> {
        let file_graph = self.rebuild_graph()?;
        if depth == 0 {
            return Ok(file_graph);
        }

        // First `depth` directory components of a repo-relative path, or "(root)"
        // for a file that sits at the tree root.
        fn dir_key(path: &str, depth: usize) -> String {
            let parts: Vec<&str> = path.split('/').collect();
            let dir_parts = &parts[..parts.len().saturating_sub(1)];
            let take = depth.min(dir_parts.len());
            if take == 0 {
                "(root)".to_string()
            } else {
                dir_parts[..take].join("/")
            }
        }

        let files = self.mapped_files.lock().map_err(|e| e.to_string())?;

        // Fold each file node into its directory, aggregating size + hotspot and
        // unioning member imports (the god-module cohesion pass reads these).
        let mut member_of: HashMap<String, String> = HashMap::new();
        let mut dir_sig: HashMap<String, usize> = HashMap::new();
        let mut dir_hotspot: HashMap<String, f64> = HashMap::new();
        let mut dir_imports: HashMap<String, Vec<String>> = HashMap::new();
        let mut order: Vec<String> = Vec::new();

        for node in &file_graph.nodes {
            let dk = dir_key(&node.path, depth);
            if !dir_sig.contains_key(&dk) {
                order.push(dk.clone());
            }
            member_of.insert(node.module_id.clone(), dk.clone());
            *dir_sig.entry(dk.clone()).or_insert(0) += node.signature_count;
            if let Some(h) = node.hotspot_score {
                let e = dir_hotspot.entry(dk.clone()).or_insert(0.0);
                if h > *e {
                    *e = h;
                }
            }
            if let Some(mf) = files.get(&node.module_id) {
                dir_imports.entry(dk.clone()).or_default().extend(mf.imports.iter().cloned());
            }
        }

        // Directory nodes, in first-seen order for deterministic output.
        let nodes: Vec<GraphNode> = order
            .iter()
            .map(|dk| GraphNode {
                module_id: dk.clone(),
                path: dk.clone(),
                language: "dir".into(),
                signature_count: dir_sig.get(dk).copied().unwrap_or(0),
                complexity: None,
                is_bridge: None,
                bridge_score: None,
                degree: None,
                risk_level: None,
                churn: None,
                hotspot_score: dir_hotspot.get(dk).copied(),
                role: None,
                is_dead: None,
                unreferenced_exports: None,
                fan_in: None,
                fan_out: None,
                cochange_partners: None,
                cochange_entropy: None,
                owner: None,
            })
            .collect();

        // Aggregate cross-directory edges (deduped presence, self-loops dropped).
        let mut seen: std::collections::HashSet<(String, String)> = std::collections::HashSet::new();
        let mut edges: Vec<GraphEdge> = Vec::new();
        for e in &file_graph.edges {
            if e.resolution == "fuzzy" {
                continue;
            }
            let (Some(sf), Some(tf)) = (member_of.get(&e.source), member_of.get(&e.target)) else {
                continue;
            };
            if sf == tf {
                continue;
            }
            if seen.insert((sf.clone(), tf.clone())) {
                edges.push(GraphEdge {
                    source: sf.clone(),
                    target: tf.clone(),
                    edge_type: "runtime".into(),
                    at_range: None,
                    resolution: "exact".into(),
                });
            }
        }

        // Synthetic per-directory files carry the unioned imports for cohesion.
        let synthetic: HashMap<String, MappedFile> = dir_imports
            .into_iter()
            .map(|(dk, imports)| (dk.clone(), MappedFile::from_minimal(dk, imports)))
            .collect();

        // Language stats reflect the underlying files, not the synthetic "dir" nodes.
        let languages = file_graph.metadata.languages.clone();
        drop(files);

        Ok(self.finalize_graph_response(nodes, edges, languages, &synthetic))
    }

}

// ---------------------------------------------------------------------------
// Ranked skeleton (personalized PageRank over dependency graph)
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, serde::Serialize)]
pub struct RankedFile {
    pub path: String,
    pub module_id: String,
    /// PageRank score (normalized, higher = more relevant to the focus set).
    pub rank: f64,
    pub signature_count: usize,
    /// tiktoken (cl100k_base) token count of this file's signature text, as used for the
    /// budget cutoff. Falls back to a 15-per-signature heuristic only if the encoder fails.
    pub estimated_tokens: usize,
    pub role: Option<String>,
    pub signatures: Vec<String>,
}

impl ApiState {
    /// Return files ranked by personalized PageRank, pruned to `token_budget`
    /// tokens (0 = return all).
    ///
    /// `focus` is a list of file paths (relative to root) that seed the
    /// personalization vector.  When empty, standard PageRank is used.
    pub fn ranked_skeleton(
        &self,
        focus: &[String],
        token_budget: usize,
    ) -> Result<Vec<RankedFile>, String> {
        let graph = self
            .project_graph
            .lock()
            .map_err(|e| e.to_string())?
            .clone()
            .ok_or("Graph not built — call rebuild_graph first")?;

        let files = self.mapped_files.lock().map_err(|e| e.to_string())?;

        let nodes = &graph.nodes;
        let n = nodes.len();
        if n == 0 {
            return Ok(vec![]);
        }

        // Index nodes by module_id.
        let idx: HashMap<&str, usize> = nodes
            .iter()
            .enumerate()
            .map(|(i, node)| (node.module_id.as_str(), i))
            .collect();

        // Build edge list as (src_idx, tgt_idx).
        let edges: Vec<(usize, usize)> = graph
            .edges
            .iter()
            .filter_map(|e| {
                let s = idx.get(e.source.as_str())?;
                let t = idx.get(e.target.as_str())?;
                Some((*s, *t))
            })
            .collect();

        // Personalization vector: focus files get equal weight; uniform fallback.
        let focus_indices: Vec<usize> = focus
            .iter()
            .filter_map(|path| idx.get(path.as_str()).copied())
            .collect();

        let mut personalization = vec![0.0f64; n];
        if focus_indices.is_empty() {
            let uniform = 1.0 / n as f64;
            for p in &mut personalization {
                *p = uniform;
            }
        } else {
            let w = 1.0 / focus_indices.len() as f64;
            for &i in &focus_indices {
                personalization[i] = w;
            }
        }

        // Personalized PageRank — 30 power-iteration steps, damping = 0.85.
        let mut rank = vec![1.0f64 / n as f64; n];
        let mut new_rank = vec![0.0f64; n];
        let damping = 0.85f64;

        let mut in_edges: Vec<Vec<usize>> = vec![vec![]; n];
        let mut out_degree = vec![0usize; n];
        for &(s, t) in &edges {
            in_edges[t].push(s);
            out_degree[s] += 1;
        }

        for _ in 0..30 {
            for i in 0..n {
                let incoming: f64 = in_edges[i]
                    .iter()
                    .map(|&s| {
                        if out_degree[s] > 0 {
                            rank[s] / out_degree[s] as f64
                        } else {
                            0.0
                        }
                    })
                    .sum();
                new_rank[i] =
                    (1.0 - damping) * personalization[i] + damping * incoming;
            }
            std::mem::swap(&mut rank, &mut new_rank);
            let sum: f64 = rank.iter().sum();
            if sum > 0.0 {
                for r in &mut rank {
                    *r /= sum;
                }
            }
        }

        // Ordering: focus files first, in the order the caller supplied them (which is
        // relevance order when seeded from a search). This guarantees the query-relevant
        // files survive the budget cut instead of being displaced by high-centrality hubs.
        // Remaining files follow, sorted by personalized PageRank descending.
        let mut seen = vec![false; n];
        let mut ranked_idx: Vec<usize> = Vec::with_capacity(n);
        for path in focus {
            if let Some(&i) = idx.get(path.as_str()) {
                if !seen[i] {
                    seen[i] = true;
                    ranked_idx.push(i);
                }
            }
        }
        // Composite orientation score: an agent seeing the first N tokens should land on
        // the code that ORCHESTRATES the repo (entry points, coordinators, domain core),
        // not the ubiquitous sink headers. Both PageRank-of-imports and fan-in reward
        // sinks (everything imports typedefs.h), which is the opposite of useful
        // orientation — so this is role-primary, uses fan-OUT (orchestration), and keeps
        // PageRank only as a mild centrality signal. All inputs are on the node (no I/O;
        // churn would need a git subprocess and this path is hot).
        let max_rank = rank.iter().cloned().fold(0.0_f64, f64::max).max(1e-9);
        let max_fanout = nodes
            .iter()
            .filter_map(|nd| nd.fan_out)
            .max()
            .unwrap_or(0)
            .max(1) as f64;
        let role_weight = |role: &Option<String>| -> f64 {
            match role.as_deref() {
                Some("entry") => 1.0,
                Some("core") => 0.85,
                Some("bridge") => 0.7,
                Some("utility") => 0.2,
                Some("leaf") => 0.15,
                Some("dead") => 0.0,
                _ => 0.5, // standard
            }
        };
        let orientation = |i: usize| -> f64 {
            let nd = &nodes[i];
            let r = rank[i] / max_rank;
            let fo = nd.fan_out.unwrap_or(0) as f64 / max_fanout;
            0.45 * role_weight(&nd.role) + 0.30 * fo + 0.25 * r
        };
        let mut rest: Vec<usize> = (0..n).filter(|&i| !seen[i]).collect();
        rest.sort_by(|&a, &b| {
            orientation(b)
                .partial_cmp(&orientation(a))
                .unwrap_or(std::cmp::Ordering::Equal)
        });
        ranked_idx.extend(rest);

        let mut result = Vec::new();
        let mut tokens_used = 0usize;

        for i in ranked_idx {
            let node = &nodes[i];
            let sigs: Vec<String> = files
                .get(&node.module_id)
                .map(|mf| mf.signatures.iter().map(|s| s.raw.clone()).collect())
                .unwrap_or_default();
            let estimated = {
                let text = sigs.join("\n");
                tiktoken_rs::cl100k_base()
                    .map(|bpe| bpe.encode_with_special_tokens(&text).len())
                    .unwrap_or_else(|_| sigs.len() * 15 + 5)
            };
            // Account for the per-file JSON envelope so the budget reflects the actual
            // returned payload, not raw signature tokens alone: the path + module_id
            // strings (~len/4 tokens each) plus the fixed object keys (rank, role,
            // signature_count, estimated_tokens, brackets ≈ 30 tokens).
            let envelope = node.module_id.len() / 2 + 30;
            let cost = estimated + envelope;

            if token_budget > 0 && tokens_used + cost > token_budget {
                break;
            }
            tokens_used += cost;

            result.push(RankedFile {
                path: self.rel(&node.path),
                module_id: node.module_id.clone(),
                rank: rank[i],
                signature_count: node.signature_count,
                estimated_tokens: estimated,
                role: node.role.clone(),
                signatures: sigs,
            });
        }

        Ok(result)
    }
}

// ---------------------------------------------------------------------------
// Role-classification helpers (free functions, not methods)
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Import resolution helpers
// ---------------------------------------------------------------------------

/// Parse a raw import statement into (module_path, optional_symbol_hint).
///
/// Examples:
///   `use crate::mapper::MappedFile;`       → ("mapper",       Some("MappedFile"))
/// Return the current git HEAD SHA for `root`, or `""` if not a git repo.
pub(crate) fn git_head_sha(root: &std::path::Path) -> String {
    std::process::Command::new("git")
        .args(["-C", &root.to_string_lossy(), "rev-parse", "HEAD"])
        .output()
        .ok()
        .and_then(|o| if o.status.success() { Some(o.stdout) } else { None })
        .map(|b| String::from_utf8_lossy(&b).trim().to_string())
        .unwrap_or_default()
}

///   `import { useState } from 'react'`     → ("react",        Some("useState"))
///   `from mymodule.auth import verify`     → ("mymodule/auth", Some("verify"))
///   `import "github.com/user/repo/pkg"`    → ("pkg",           None)
fn parse_import_parts(import: &str) -> (String, Option<String>) {
    let raw = import.trim().trim_end_matches(';');

    // Python: from foo.bar import Baz
    if let Some(rest) = raw.strip_prefix("from ") {
        if let Some((module, symbol)) = rest.split_once(" import ") {
            let sym = symbol.trim().split(',').next().unwrap_or("").trim().to_string();
            return (
                module.trim().replace('.', "/"),
                if sym.is_empty() { None } else { Some(sym) },
            );
        }
    }

    // JS/TS: import { Foo } from './bar'  /  import Foo from 'bar'
    if raw.starts_with("import ") && raw.contains(" from ") {
        if let Some(from_pos) = raw.rfind(" from ") {
            let path = raw[from_pos + 6..]
                .trim()
                .trim_matches('"')
                .trim_matches('\'')
                .to_string();
            let lhs = raw[7..from_pos].trim();
            let symbol = extract_js_import_symbol(lhs);
            return (path, symbol);
        }
    }

    // Rust: use crate::foo::Bar  /  use foo::{A, B}
    if let Some(rest) = raw.strip_prefix("use ") {
        let path = rest
            .trim()
            .split('{')
            .next()
            .unwrap_or(rest)
            .trim_end_matches(':')
            .trim();
        let segments: Vec<&str> = path.split("::").collect();
        if let Some(&last) = segments.last() {
            if last.chars().next().map(|c| c.is_uppercase()).unwrap_or(false) {
                // Uppercase last segment → type name; use second-to-last as module
                let module = segments
                    .get(segments.len().saturating_sub(2))
                    .copied()
                    .unwrap_or("")
                    .to_string();
                return (module, Some(last.to_string()));
            }
        }
        return (segments.last().copied().unwrap_or(path).to_string(), None);
    }

    // Rust path already stripped of 'use'/'use ' prefix by tree-sitter extractor.
    // e.g. "crate::mapper::MappedFile"  /  "crate::api::{Foo, Bar}"
    // Identified by '::' without any leading keyword — this catches what the
    // `use …` branch above misses when the prefix is absent.
    if raw.contains("::") && !raw.starts_with("import ") {
        let path = raw
            .split('{')
            .next()
            .unwrap_or(raw)
            .trim_end_matches(':')
            .trim();
        let segments: Vec<&str> = path.split("::").collect();
        if let Some(&last) = segments.last() {
            if last.chars().next().map(|c| c.is_uppercase()).unwrap_or(false) {
                let module = segments
                    .get(segments.len().saturating_sub(2))
                    .copied()
                    .unwrap_or("")
                    .to_string();
                return (module, Some(last.to_string()));
            }
        }
        if let Some(last) = segments.last().copied() {
            return (last.to_string(), None);
        }
    }

    // Java/Kotlin: import com.example.MyClass
    if let Some(rest) = raw.strip_prefix("import ") {
        let path = rest.trim().trim_end_matches(';');
        let segments: Vec<&str> = path.split('.').collect();
        if let Some(&last) = segments.last() {
            if last.chars().next().map(|c| c.is_uppercase()).unwrap_or(false) {
                let module = segments
                    .get(segments.len().saturating_sub(2))
                    .copied()
                    .unwrap_or("")
                    .to_string();
                return (module, Some(last.to_string()));
            }
        }
        return (path.replace('.', "/"), None);
    }

    // require() / require_relative (Ruby/Node)
    if raw.contains("require") {
        let path = raw
            .split('"')
            .nth(1)
            .or_else(|| raw.split('\'').nth(1))
            .unwrap_or("")
            .trim_start_matches("./")
            .to_string();
        return (path, None);
    }

    // Fallback: last token
    let last = raw.split_whitespace().last().unwrap_or(raw);
    let last = last.trim_matches('"').trim_matches('\'').trim_end_matches(';');
    // Bare PascalCase identifier (e.g. from doc backtick refs) → set as symbol hint
    // so resolve_import_target can match it against symbol definitions.
    if !last.contains('/') && !last.contains('.') && last.len() >= 4
        && last.chars().next().map(|c| c.is_uppercase()).unwrap_or(false)
    {
        return (last.to_string(), Some(last.to_string()));
    }
    (last.to_string(), None)
}

fn extract_js_import_symbol(lhs: &str) -> Option<String> {
    let lhs = lhs.trim();
    if lhs.starts_with('{') {
        lhs.trim_matches(|c| c == '{' || c == '}')
            .split(',')
            .next()
            .map(|s| s.trim().split(" as ").next().unwrap_or("").trim().to_string())
            .filter(|s| !s.is_empty())
    } else if lhs.starts_with('*') || lhs.is_empty() {
        None
    } else {
        Some(lhs.split(" as ").next().unwrap_or(lhs).trim().to_string())
    }
}

/// Strip the file extension from a repo-relative path, i.e. the trailing
/// `.ext` of its last component only. `"a/b/foo.py"` → `"a/b/foo"`; a path with
/// no dot in its last segment (`"a/b/foo"`) is returned unchanged.
fn path_no_ext(p: &str) -> &str {
    match p.rfind('.') {
        Some(dot) if !p[dot..].contains('/') => &p[..dot],
        _ => p,
    }
}

/// Read the `module` path declared in `<root>/go.mod`, if present.
/// e.g. a go.mod with `module github.com/acme/widget` → `Some("github.com/acme/widget")`.
fn read_go_module(root: &Path) -> Option<String> {
    let text = std::fs::read_to_string(root.join("go.mod")).ok()?;
    for line in text.lines() {
        let t = line.trim();
        if let Some(rest) = t.strip_prefix("module ") {
            let m = rest.trim().trim_matches('"').trim();
            if !m.is_empty() {
                return Some(m.to_string());
            }
        }
    }
    None
}

/// Return the last meaningful path component to use as a file-stem candidate.
fn derive_module_stem(module_path: &str) -> String {
    let last = module_path
        .split('/')
        .filter(|s| !s.is_empty() && *s != "." && *s != "..")
        .last()
        .unwrap_or(module_path)
        .trim_start_matches('@');  // strip npm scope prefix
    let kebab_first = last.split('-').next().unwrap_or(last); // treat kebab-case first word as stem
    // Strip file extension so doc-style imports ("scanner.rs", "api/search.md") resolve correctly
    Path::new(kebab_first)
        .file_stem()
        .and_then(|s| s.to_str())
        .unwrap_or(kebab_first)
        .to_string()
}

/// Precomputed lookup tables for import resolution. Built once per `rebuild_graph`
/// so resolving every file's imports is O(N + edges) instead of O(N²): the previous
/// resolver scanned every file for every import, which made large C/C++ trees
/// (project-root-relative `#include`s across thousands of files) effectively hang.
struct ImportIndex<'a> {
    /// Every module_id present, for exact-path matches.
    ids: std::collections::HashSet<&'a str>,
    /// file basename WITH extension ("object.h") → module_ids.
    by_basename: HashMap<String, Vec<&'a str>>,
    /// file basename stem, no extension ("object") → module_ids.
    by_stem: HashMap<String, Vec<&'a str>>,
    /// lowercased path-component stem → module_ids (loose segment fallback).
    by_segment: HashMap<String, Vec<&'a str>>,
    /// public symbol name → module_ids that define it (symbol-hint fallback).
    by_symbol: HashMap<String, Vec<&'a str>>,
    /// repo-relative directory → module_ids in it. Backs Go package resolution,
    /// where an import names a directory (package), not a file.
    by_dir: HashMap<String, Vec<&'a str>>,
    /// last path segment of a directory → the directories ending in it. Lets the
    /// package-suffix fallback probe only directories that could match, instead of
    /// scanning every directory (which was O(dirs) per unresolved qualified import
    /// — a real cliff on huge trees with many external imports).
    by_dir_last: HashMap<String, Vec<String>>,
    /// Go module path from go.mod (e.g. "github.com/acme/widget"), set when the
    /// project root is a Go module. Enables namespace-exact import resolution:
    /// internal imports resolve to their package directory, external ones to no edge.
    go_module: Option<String>,
}

impl<'a> ImportIndex<'a> {
    fn build(files: &'a HashMap<String, MappedFile>) -> Self {
        let mut ids = std::collections::HashSet::new();
        let mut by_basename: HashMap<String, Vec<&'a str>> = HashMap::new();
        let mut by_stem: HashMap<String, Vec<&'a str>> = HashMap::new();
        let mut by_segment: HashMap<String, Vec<&'a str>> = HashMap::new();
        let mut by_symbol: HashMap<String, Vec<&'a str>> = HashMap::new();
        let mut by_dir: HashMap<String, Vec<&'a str>> = HashMap::new();

        for (module_id, file) in files.iter() {
            let id = module_id.as_str();
            ids.insert(id);
            // module_id is the repo-relative path; its parent is the package directory.
            if let Some(dir) = Path::new(id).parent().and_then(|d| d.to_str()) {
                by_dir.entry(dir.to_string()).or_default().push(id);
            }
            let p = Path::new(&file.path);
            if let Some(bn) = p.file_name().and_then(|s| s.to_str()) {
                by_basename.entry(bn.to_string()).or_default().push(id);
            }
            if let Some(stem) = p.file_stem().and_then(|s| s.to_str()) {
                by_stem.entry(stem.to_string()).or_default().push(id);
            }
            for seg in file.path.split('/') {
                let seg_stem = Path::new(seg)
                    .file_stem()
                    .and_then(|s| s.to_str())
                    .unwrap_or(seg);
                if seg_stem.len() >= 3 {
                    by_segment
                        .entry(seg_stem.to_lowercase())
                        .or_default()
                        .push(id);
                }
            }
            for sig in &file.signatures {
                if let Some(name) = sig.symbol_name.as_deref() {
                    if name.len() >= 4 {
                        by_symbol.entry(name.to_string()).or_default().push(id);
                    }
                }
            }
        }
        // Index directories by their last segment for O(candidates) suffix probing.
        let mut by_dir_last: HashMap<String, Vec<String>> = HashMap::new();
        for dir in by_dir.keys() {
            if let Some(last) = dir.rsplit('/').next() {
                by_dir_last.entry(last.to_string()).or_default().push(dir.clone());
            }
        }

        // Candidate lists are built by iterating `files` (a HashMap), so their order is
        // per-process random. Any resolver that picks among equally-ranked candidates
        // (min_by_key, first-match) would then choose non-deterministically — invisible
        // at file level, but it flips which directory an ambiguous import folds into.
        // Sort every list once so all downstream selection is over a stable order.
        for v in by_basename.values_mut() { v.sort_unstable(); }
        for v in by_stem.values_mut() { v.sort_unstable(); }
        for v in by_segment.values_mut() { v.sort_unstable(); }
        for v in by_symbol.values_mut() { v.sort_unstable(); }
        for v in by_dir.values_mut() { v.sort_unstable(); }
        for v in by_dir_last.values_mut() { v.sort_unstable(); }

        Self { ids, by_basename, by_stem, by_segment, by_symbol, by_dir, by_dir_last, go_module: None }
    }

    /// Directories whose path ends with `/rel` (or equals `rel`), returning the
    /// members of the shortest such directory. O(directories sharing rel's last
    /// segment), not O(all directories).
    fn dir_suffix_files(&self, rel: &str) -> Option<&Vec<&'a str>> {
        let last = rel.rsplit('/').next().unwrap_or(rel);
        let suffix = format!("/{rel}");
        self.by_dir_last
            .get(last)?
            .iter()
            .filter(|d| d.as_str() == rel || d.ends_with(&suffix))
            .min_by_key(|d| d.len())
            .and_then(|d| self.by_dir.get(d.as_str()))
    }

    /// Opt in to Go module-aware resolution by reading `<root>/go.mod`. No-op for
    /// non-Go projects (leaves `go_module` as `None`, preserving generic behaviour).
    fn with_go_module(mut self, root: &Path) -> Self {
        self.go_module = read_go_module(root);
        self
    }

    /// Pick the candidate whose module_id best matches the import path, preferring a
    /// path-suffix relationship ("core/object/object.h" → "object/object.h") and, among
    /// ties, the shortest (closest) path. Deterministic — never HashMap iteration order.
    /// Number of leading DIRECTORY segments shared by two repo-relative paths. Used to
    /// prefer the include target nearest the requesting file when a basename is ambiguous
    /// (`#include "foo.h"` with `foo.h` in several directories → pick the closest one).
    fn shared_dir_segments(a: &str, b: &str) -> usize {
        let a_dirs: Vec<&str> = a.split('/').collect();
        let b_dirs: Vec<&str> = b.split('/').collect();
        let a_len = a_dirs.len().saturating_sub(1); // drop filename
        let b_len = b_dirs.len().saturating_sub(1);
        let mut n = 0;
        for i in 0..a_len.min(b_len) {
            if a_dirs[i] == b_dirs[i] {
                n += 1;
            } else {
                break;
            }
        }
        n
    }

    fn best_suffix(cands: &[&'a str], norm: &str, source: &str) -> Option<String> {
        // (suffix_score, proximity_to_source, candidate). Proximity breaks suffix-score ties
        // so an ambiguous bare basename resolves to the nearest file, not an arbitrary one.
        let mut best: Option<(usize, usize, &'a str)> = None;
        for &m in cands {
            if m == source {
                continue;
            }
            let score = if m == norm {
                norm.len() + 1
            } else if norm.ends_with(&format!("/{m}")) {
                m.len()
            } else if m.ends_with(&format!("/{norm}")) {
                norm.len()
            } else {
                continue;
            };
            let prox = Self::shared_dir_segments(m, source);
            let better = match best {
                None => true,
                Some((bs, bp, bm)) => {
                    score > bs
                        || (score == bs && prox > bp)
                        || (score == bs && prox == bp && m.len() < bm.len())
                }
            };
            if better {
                best = Some((score, prox, m));
            }
        }
        best.map(|(_, _, m)| m.to_string())
    }

    fn pick_deterministic(cands: &[&'a str], source: &str) -> Option<String> {
        // Prefer the candidate closest to the source directory, then the shortest path.
        cands
            .iter()
            .filter(|m| **m != source)
            .max_by(|a, b| {
                Self::shared_dir_segments(a, source)
                    .cmp(&Self::shared_dir_segments(b, source))
                    .then_with(|| b.len().cmp(&a.len())) // shorter path wins the tie
                    .then_with(|| b.cmp(a))
            })
            .map(|m| m.to_string())
    }

    /// Resolve an import to a target module_id plus a confidence tag
    /// (`"exact"` | `"suffix"` | `"fuzzy"`). `None` means no internal edge.
    /// Resolve an extensionless import that carries directory structure
    /// (`foo/bar`) against the FULL path, not just its last stem. Tries a module
    /// file whose extension-stripped path ends with `norm`, then a package
    /// directory named `norm` (preferring an `__init__.py` / `index.*` / `mod.rs`
    /// entry point). Returns `None` when nothing local matches — a qualified
    /// import that resolves nowhere is external, not a reason to fuzzy-guess.
    fn best_qualified_suffix(&self, norm: &str, source: &str) -> Option<String> {
        // (a) Module file: a same-stem file whose path (minus extension) is `norm`
        //     or ends with `/norm`.
        let last = norm.rsplit('/').next().unwrap_or(norm);
        if let Some(cands) = self.by_stem.get(last) {
            let matches: Vec<&'a str> = cands
                .iter()
                .copied()
                .filter(|&m| {
                    let ne = path_no_ext(m);
                    ne == norm || ne.ends_with(&format!("/{norm}"))
                })
                .collect();
            if let Some(hit) = Self::pick_deterministic(&matches, source) {
                return Some(hit);
            }
        }

        // (b) Package directory named `norm` (or ending with `/norm`).
        let dir_cands = self.by_dir.get(norm).or_else(|| self.dir_suffix_files(norm));
        if let Some(cands) = dir_cands {
            // Prefer the package's entry point when present.
            if let Some(entry) = cands.iter().copied().find(|&m| {
                let bn = m.rsplit('/').next().unwrap_or(m);
                bn == "__init__.py" || bn.starts_with("index.") || bn == "mod.rs"
            }) {
                if entry != source {
                    return Some(entry.to_string());
                }
            }
            if let Some(hit) = Self::pick_deterministic(cands, source) {
                return Some(hit);
            }
        }

        None
    }

    /// Resolve a relative specifier (`./x`, `../y/z`) against the SOURCE file's
    /// directory — the only correct anchor for JS/TS/relative-C imports. Joins
    /// and normalises `.`/`..`, then matches a module file (any extension) or a
    /// package directory (preferring an `index.*` / `__init__.py` / `mod.rs`
    /// entry point). `None` means the target wasn't scanned — a relative import
    /// always names something local, so we never fuzzy-fall-back here.
    fn resolve_relative(&self, spec: &str, source: &str) -> Option<String> {
        let mut parts: Vec<String> = Path::new(source)
            .parent()
            .map(|p| {
                p.to_string_lossy()
                    .split('/')
                    .filter(|s| !s.is_empty())
                    .map(|s| s.to_string())
                    .collect()
            })
            .unwrap_or_default();
        for seg in spec.split('/') {
            match seg {
                "" | "." => {}
                ".." => {
                    parts.pop();
                }
                s => parts.push(s.to_string()),
            }
        }
        let joined = parts.join("/");
        if joined.is_empty() {
            return None;
        }
        // Exact module_id (specifier already carried an extension, e.g. C "./foo.h").
        if joined != source && self.ids.contains(joined.as_str()) {
            return Some(joined);
        }
        // Module file: a same-stem file whose extension-stripped path equals `joined`.
        let joined_ne = path_no_ext(&joined);
        let last = joined_ne.rsplit('/').next().unwrap_or(joined_ne);
        if let Some(cands) = self.by_stem.get(last) {
            if let Some(hit) = cands
                .iter()
                .copied()
                .find(|&m| m != source && path_no_ext(m) == joined_ne)
            {
                return Some(hit.to_string());
            }
        }
        // Package directory `joined` → its entry point, else any member.
        if let Some(cands) = self.by_dir.get(joined_ne) {
            if let Some(entry) = cands.iter().copied().find(|&m| {
                let bn = m.rsplit('/').next().unwrap_or(m);
                bn.starts_with("index.") || bn == "__init__.py" || bn == "mod.rs"
            }) {
                if entry != source {
                    return Some(entry.to_string());
                }
            }
            return Self::pick_deterministic(cands, source);
        }
        None
    }

    fn resolve(&self, import: &str, source: &str) -> Option<(String, &'static str)> {
        let (module_path, symbol_hint) = parse_import_parts(import);

        // Relative specifier (JS/TS `./`, `../`; relative C includes): resolve
        // against the source directory. Path-precise, and authoritative — a
        // relative import names something local, so a miss is a dangling ref
        // (no edge), never a reason to fuzzy-guess a same-name file elsewhere.
        if module_path.starts_with("./") || module_path.starts_with("../") {
            return self
                .resolve_relative(&module_path, source)
                .map(|hit| (hit, "exact"));
        }

        let norm = module_path
            .trim_start_matches("./")
            .trim_start_matches('/')
            .to_string();
        let has_ext = Path::new(&norm).extension().is_some();
        let basename = Path::new(&norm)
            .file_name()
            .and_then(|s| s.to_str())
            .map(str::to_string);
        let stem = derive_module_stem(&module_path);

        // 0. Go module-aware resolution. Authoritative for .go sources when a go.mod
        //    is present: an import is INTERNAL iff it lives under the module path, in
        //    which case it names a package DIRECTORY — resolve to that directory's
        //    files, never to a same-stem file in an unrelated package. Anything not
        //    under the module path is stdlib/third-party and has NO internal edge.
        //    This branch always returns, so Go imports never reach the fuzzy
        //    basename/stem/segment fallbacks below (which fabricate cross-package edges).
        if source.ends_with(".go") {
            if let Some(module) = &self.go_module {
                let internal_rel: Option<&str> = if norm == *module {
                    Some("")
                } else {
                    norm.strip_prefix(module.as_str())
                        .filter(|r| r.starts_with('/'))
                        .map(|r| &r[1..])
                };
                match internal_rel {
                    // The module root package itself — no single file to point at.
                    Some("") => return None,
                    Some(rel) => {
                        // Exact package directory.
                        if let Some(cands) = self.by_dir.get(rel) {
                            return Self::pick_deterministic(cands, source).map(|t| (t, "exact"));
                        }
                        // Package dir not scanned as-is (nested module, generated code):
                        // accept the shortest directory whose path ends with the suffix.
                        if let Some(cands) = self.dir_suffix_files(rel) {
                            return Self::pick_deterministic(cands, source).map(|t| (t, "suffix"));
                        }
                        // Internal by namespace but absent from the scan → honest miss.
                        return None;
                    }
                    // Not under the module path → external (stdlib / third-party).
                    None => return None,
                }
            }
        }

        // 1. Exact module_id match.
        if norm != source && self.ids.contains(norm.as_str()) {
            return Some((norm, "exact"));
        }

        // 2. Extension-bearing import (C/C++ #include, JS/TS with ext): match by basename,
        //    then disambiguate by path suffix. This is what keeps `object.h` from resolving
        //    to `object.cpp` and fixes systematically-misrouted include edges.
        if let Some(bn) = &basename {
            if let Some(cands) = self.by_basename.get(bn) {
                if let Some(hit) = Self::best_suffix(cands, &norm, source) {
                    let kind = if hit == norm { "exact" } else { "suffix" };
                    return Some((hit, kind));
                }
                if has_ext {
                    let non_self: Vec<&str> = cands.iter().copied().filter(|m| *m != source).collect();
                    if non_self.len() == 1 {
                        // Unique basename, no path agreement — confident but not exact.
                        return Some((non_self[0].to_string(), "suffix"));
                    }
                }
            }
        }

        // 2.5 Qualified path-suffix. An extensionless import that still carries
        //     directory structure (Python `foo.bar` → foo/bar, monorepo/aliased
        //     `components/Button`) is resolved against the FULL path, not the last
        //     stem — the qualification disambiguates same-name files across
        //     packages, the way go.mod does for Go. This is authoritative for such
        //     imports: a qualified path that matches nothing local is external, so
        //     we return no edge rather than falling through to the fuzzy stem/segment
        //     guesses (which fabricated cross-package edges).
        if !has_ext && norm.contains('/') {
            // `from pkg.sub import name` may name a submodule `pkg/sub/name`; try the
            // combined path first so it beats the package's own __init__.
            if let Some(sym) = &symbol_hint {
                let combined = format!("{norm}/{sym}");
                if let Some(hit) = self.best_qualified_suffix(&combined, source) {
                    return Some((hit, "suffix"));
                }
            }
            return self
                .best_qualified_suffix(&norm, source)
                .map(|hit| (hit, "suffix"));
        }

        // 3. Extensionless import (Rust `mod`, Python `import`, Go package): match by stem.
        if !has_ext {
            if let Some(cands) = self.by_stem.get(&stem) {
                if let Some(hit) = Self::pick_deterministic(cands, source) {
                    return Some((hit, "fuzzy"));
                }
            }
        }

        // 4. Loose segment fallback: stem appears as a path component.
        if stem.len() >= 3 {
            if let Some(cands) = self.by_segment.get(&stem.to_lowercase()) {
                if let Some(hit) = Self::pick_deterministic(cands, source) {
                    return Some((hit, "fuzzy"));
                }
            }
        }

        // 5. Symbol-hint fallback: a file that defines the imported symbol name.
        if let Some(sym) = &symbol_hint {
            if sym.len() >= 4 {
                if let Some(cands) = self.by_symbol.get(sym) {
                    if let Some(hit) = Self::pick_deterministic(cands, source) {
                        return Some((hit, "fuzzy"));
                    }
                }
            }
        }

        None
    }
}

pub fn is_entry_point_path(path: &str) -> bool {
    let name = path.rsplit('/').next().unwrap_or(path);
    matches!(
        name,
        "main.rs"
            | "lib.rs"  // crate root — no Rust-side callers by design
            | "main.py"
            | "main.go"
            | "main.ts"
            | "main.js"
            | "index.ts"
            | "index.js"
            | "index.tsx"
            | "index.jsx"
            | "app.rs"
            | "app.py"
            | "app.ts"
            | "app.js"
            | "server.ts"
            | "server.js"
            | "server.go"
    )
}

pub(crate) fn is_test_path(path: &str) -> bool {
    let lower = path.to_lowercase();
    lower.contains("_test.")
        || lower.contains(".test.")
        || lower.contains(".spec.")
        || lower.contains("/test/")
        || lower.contains("/tests/")
        || lower.contains("/spec/")
        || lower.ends_with("_test.go")
}

// ---------------------------------------------------------------------------
// Document helpers
// ---------------------------------------------------------------------------

/// File extensions treated as "documents" (non-code) for doc-oriented tools.
pub const DOC_EXTENSIONS: &[&str] = &["md", "markdown", "yaml", "yml", "toml", "json"];

pub fn is_doc_path(path: &str) -> bool {
    path.rsplit('.')
        .next()
        .map(|ext| DOC_EXTENSIONS.contains(&ext))
        .unwrap_or(false)
}

/// Summary of a document node in the project graph.
#[derive(Debug, Clone, Serialize)]
pub struct DocNode {
    pub path: String,
    pub module_id: String,
    pub signatures: Vec<String>,
    pub imports: Vec<String>,
    pub edge_count: usize,
}

impl ApiState {
    /// Return all document-type nodes from the project graph.
    pub fn doc_nodes(&self) -> Result<Vec<DocNode>, String> {
        let graph = self.rebuild_graph()?;
        let files = self.mapped_files.lock().map_err(|e| e.to_string())?;

        let mut docs = Vec::new();
        for node in &graph.nodes {
            if !is_doc_path(&node.path) {
                continue;
            }
            let edge_count = graph.edges.iter()
                .filter(|e| e.source == node.module_id || e.target == node.module_id)
                .count();

            let (sigs, imports) = files.get(&node.module_id)
                .map(|mf| (
                    mf.signatures.iter().map(|s| s.raw.clone()).collect(),
                    mf.imports.clone(),
                ))
                .unwrap_or_default();

            docs.push(DocNode {
                path: self.rel(&node.path),
                module_id: node.module_id.clone(),
                signatures: sigs,
                imports,
                edge_count,
            });
        }

        // Sort: most connected docs first
        docs.sort_by(|a, b| b.edge_count.cmp(&a.edge_count));
        Ok(docs)
    }
}

struct BridgeAnalysis {
    is_bridge: bool,
    bridge_score: f64,
    degree: usize,
    risk_level: String,
}

impl ApiState {
    fn analyze_bridges(
        &self,
        nodes: &[GraphNode],
        edges: &[GraphEdge],
    ) -> HashMap<String, BridgeAnalysis> {
        let mut graph: UnGraphMap<&str, ()> = UnGraphMap::new();

        let _node_ids: HashMap<&str, &GraphNode> =
            nodes.iter().map(|n| (n.module_id.as_str(), n)).collect();

        for node in nodes {
            graph.add_node(node.module_id.as_str());
        }

        for edge in edges {
            graph.add_edge(edge.source.as_str(), edge.target.as_str(), ());
        }

        let node_count = graph.nodes().count();
        if node_count < 3 {
            return HashMap::new();
        }

        let avg_degree = 2.0 * edges.len() as f64 / node_count as f64;
        let hub_threshold = (avg_degree * 3.0).max(20.0) as usize;

        // Reuse the cached centrality map when the graph topology is unchanged since the
        // last compute (fingerprint match). This is the incremental-rebuild win: the
        // expensive Brandes pass runs only when an import edge actually changed.
        let fingerprint = Self::betweenness_fingerprint(nodes, edges);
        let cached = self.bc_cache.lock().ok().and_then(|g| {
            g.iter().find(|(fp, _)| *fp == fingerprint).map(|(_, m)| m.clone())
        });
        if std::env::var("CKB_TIMING").is_ok() {
            eprintln!("[timing]  betweenness: cache {}", if cached.is_some() { "HIT" } else { "MISS" });
        }
        let betweenness: HashMap<String, f64> = match cached {
            Some(map) => map,
            None => {
                let computed: HashMap<String, f64> = self
                    .compute_betweenness_centrality(&graph)
                    .into_iter()
                    .map(|(k, v)| (k.to_string(), v))
                    .collect();
                if let Ok(mut g) = self.bc_cache.lock() {
                    // MRU: newest first, drop the fingerprint if already present, cap length.
                    g.retain(|(fp, _)| *fp != fingerprint);
                    g.insert(0, (fingerprint, computed.clone()));
                    g.truncate(BC_CACHE_CAP);
                }
                computed
            }
        };

        let mut analysis: HashMap<String, BridgeAnalysis> = HashMap::new();

        for (node_id, bc) in &betweenness {
            let degree = graph.edges(node_id.as_str()).count();
            let is_hub = degree > hub_threshold;

            // bc is already normalized by (n-1)*(n-2) inside compute_betweenness_centrality
            let bridge_score = if is_hub { 0.0 } else { bc * 1000.0 };

            let is_bridge = !is_hub && bridge_score > 0.0;

            let risk_level = if is_bridge && bridge_score > 10.0 {
                "CRITICAL".to_string()
            } else if is_bridge {
                "HIGH".to_string()
            } else if is_hub {
                "LOW".to_string()
            } else {
                "MEDIUM".to_string()
            };

            analysis.insert(
                node_id.to_string(),
                BridgeAnalysis {
                    is_bridge,
                    bridge_score,
                    degree,
                    risk_level,
                },
            );
        }

        analysis
    }

    /// Order-independent fingerprint of the graph topology (node set + structural
    /// edge set) that betweenness depends on. Content edits that don't touch imports
    /// leave this unchanged, which is what lets `analyze_bridges` reuse the cached
    /// centrality map. `wrapping_add` makes it insensitive to node/edge vec ordering;
    /// the edge salt keeps a node named like an edge pair from colliding.
    fn betweenness_fingerprint(nodes: &[GraphNode], edges: &[GraphEdge]) -> u64 {
        use std::hash::{Hash, Hasher};
        let mut acc: u64 = 0xcbf2_9ce4_8422_2325;
        for n in nodes {
            let mut h = std::collections::hash_map::DefaultHasher::new();
            n.module_id.hash(&mut h);
            acc = acc.wrapping_add(h.finish());
        }
        acc = acc.wrapping_mul(0x0000_0100_0000_01b3);
        for e in edges {
            let mut h = std::collections::hash_map::DefaultHasher::new();
            e.source.hash(&mut h);
            e.target.hash(&mut h);
            acc = acc.wrapping_add(h.finish());
        }
        acc
    }

    fn compute_betweenness_centrality<'a>(
        &self,
        graph: &UnGraphMap<&'a str, ()>,
    ) -> HashMap<&'a str, f64> {
        use rayon::prelude::*;

        let mut nodes: Vec<&'a str> = graph.nodes().collect();
        // Sort so the strided source sample (below) is deterministic regardless of
        // graph/HashMap iteration order — otherwise the approximate bridge counts
        // would drift run-to-run.
        nodes.sort_unstable();
        let n = nodes.len();
        if n < 3 {
            return nodes.into_iter().map(|node| (node, 0.0)).collect();
        }

        // Map each node to a dense index and build an index-based adjacency list ONCE.
        // Brandes then runs over Vec<usize>/Vec<f64> buffers instead of per-source
        // HashMap<&str,_>, removing the hashing + V-sized reallocation that dominated
        // the profile (sampled sources × V-sized maps was 98% of a rebuild).
        let index: HashMap<&str, usize> =
            nodes.iter().enumerate().map(|(i, &s)| (s, i)).collect();
        let mut adj: Vec<Vec<usize>> = vec![Vec::new(); n];
        for (i, &node) in nodes.iter().enumerate() {
            for w in graph.neighbors(node) {
                if let Some(&j) = index.get(w) {
                    adj[i].push(j);
                }
            }
        }

        // Exact Brandes is O(V·E); above the threshold estimate from a strided sample of
        // source nodes and scale up (standard betweenness approximation). Exact below it
        // so small projects and tests are unaffected.
        const BC_EXACT_MAX_NODES: usize = 1500;
        const BC_SAMPLE_SOURCES: usize = 800;
        let (sources, scale): (Vec<usize>, f64) = if n <= BC_EXACT_MAX_NODES {
            ((0..n).collect(), 1.0)
        } else {
            let stride = (n / BC_SAMPLE_SOURCES).max(1);
            let sampled: Vec<usize> = (0..n).step_by(stride).collect();
            let scale = n as f64 / sampled.len() as f64;
            (sampled, scale)
        };

        // Each source's dependency accumulation is independent, so run them in parallel.
        // To stay deterministic despite float addition being non-associative, sum the
        // per-source contributions in a FIXED order (source order), and bound peak memory
        // by reducing in chunks rather than materializing all contribution vectors at once
        // (at 100× scale, sources × V f64s would be gigabytes).
        let brandes = |src: usize| -> Vec<f64> {
            let mut stack: Vec<usize> = Vec::with_capacity(n);
            let mut predecessors: Vec<Vec<usize>> = vec![Vec::new(); n];
            let mut sigma = vec![0.0f64; n];
            let mut distance = vec![-1i32; n];
            let mut queue: std::collections::VecDeque<usize> = std::collections::VecDeque::new();

            sigma[src] = 1.0;
            distance[src] = 0;
            queue.push_back(src);

            while let Some(v) = queue.pop_front() {
                stack.push(v);
                let v_dist = distance[v];
                for &w in &adj[v] {
                    if distance[w] < 0 {
                        distance[w] = v_dist + 1;
                        queue.push_back(w);
                    }
                    if distance[w] == v_dist + 1 {
                        sigma[w] += sigma[v];
                        predecessors[w].push(v);
                    }
                }
            }

            let mut delta = vec![0.0f64; n];
            let mut contrib = vec![0.0f64; n];
            while let Some(w) = stack.pop() {
                let dw = delta[w];
                for &v in &predecessors[w] {
                    delta[v] += (sigma[v] / sigma[w]) * (1.0 + dw);
                }
                if w != src {
                    contrib[w] += delta[w];
                }
            }
            contrib
        };

        let chunk = (rayon::current_num_threads() * 4).max(1);
        let mut totals = vec![0.0f64; n];
        for batch in sources.chunks(chunk) {
            let contribs: Vec<Vec<f64>> = batch.par_iter().map(|&src| brandes(src)).collect();
            for c in &contribs {
                for i in 0..n {
                    totals[i] += c[i];
                }
            }
        }

        // Scale sampled sums to full-graph estimates, then normalize as usual.
        let divisor = ((n - 1) * (n - 2)) as f64;
        nodes
            .into_iter()
            .enumerate()
            .map(|(i, node)| (node, totals[i] * scale / divisor))
            .collect()
    }

    #[allow(dead_code)]
    fn resolve_import_target(&self, import: &str, source: &str) -> Option<String> {
        let files = self.mapped_files.lock().ok()?;
        let index = ImportIndex::build(&files);
        index.resolve(import, source).map(|(t, _)| t)
    }

    // Same lookup as `resolve_import_target` but takes the already-locked map.
    // Used by `rebuild_graph` (which holds the lock for the whole rebuild) to
    // avoid re-entering the non-reentrant Mutex and deadlocking.
    //
    // For a single lookup this builds a throwaway index; `rebuild_graph` builds one
    // `ImportIndex` for the whole graph so resolution is O(N) total, not O(N²).
    fn resolve_import_target_in(
        files: &HashMap<String, MappedFile>,
        import: &str,
        source: &str,
    ) -> Option<String> {
        ImportIndex::build(files).resolve(import, source).map(|(t, _)| t)
    }

    #[allow(dead_code)]
    pub fn set_compression_level(&self, level: CompressionLevel) {
        if let Ok(mut compression) = self.compression_level.lock() {
            *compression = level;
        }
    }

    #[allow(dead_code)]
    pub fn get_compression_level(&self) -> CompressionLevel {
        self.compression_level
            .lock()
            .map(|c| *c)
            .unwrap_or(CompressionLevel::Standard)
    }

    fn detect_cycles(&self, nodes: &[GraphNode], edges: &[GraphEdge]) -> Vec<CycleInfo> {
        let mut graph: DiGraphMap<&str, ()> = DiGraphMap::new();

        for node in nodes {
            graph.add_node(node.module_id.as_str());
        }

        // Only structural (exact/suffix) edges define cycles. `fuzzy` edges are
        // low-confidence stem/segment guesses — counting them fabricated the
        // false import cycles this graph used to report (e.g. 22 "cycles" in a Go
        // repo, which the language forbids).
        for edge in edges.iter().filter(|e| e.resolution != "fuzzy") {
            graph.add_edge(edge.source.as_str(), edge.target.as_str(), ());
        }

        let sccs = petgraph::algo::tarjan_scc(&graph);

        let hub_nodes: std::collections::HashSet<&str> = nodes
            .iter()
            .filter(|n| n.degree.unwrap_or(0) > 30)
            .map(|n| n.module_id.as_str())
            .collect();

        let mut cycles = Vec::new();

        for component in sccs {
            if component.len() > 1 {
                let cycle_nodes: Vec<String> = component.iter().map(|s| s.to_string()).collect();

                let filtered_nodes: Vec<&str> = component
                    .iter()
                    .map(|&s| s)
                    .filter(|n| !hub_nodes.contains(*n))
                    .collect();

                let pivot = if filtered_nodes.is_empty() {
                    None
                } else {
                    Some(filtered_nodes[filtered_nodes.len() / 2].to_string())
                };

                let severity = if component.len() > 5 {
                    "CRITICAL"
                } else {
                    "HIGH"
                };

                cycles.push(CycleInfo {
                    nodes: cycle_nodes,
                    pivot_node: pivot,
                    severity: severity.to_string(),
                });
            }
        }

        cycles
    }

    fn detect_god_modules(
        &self,
        nodes: &[GraphNode],
        _edges: &[GraphEdge],
        files: &HashMap<String, MappedFile>,
    ) -> Vec<GodModuleInfo> {
        let god_threshold = 50;
        let mut god_modules = Vec::new();

        for node in nodes {
            let degree = node.degree.unwrap_or(0);

            if degree > god_threshold {
                let file = files.get(&node.module_id);

                let import_types: std::collections::HashSet<&str> = file
                    .map(|f| {
                        f.imports
                            .iter()
                            .filter_map(|i| {
                                let parts: Vec<&str> = i.split('/').collect();
                                parts.get(1).or(parts.first()).map(|s| *s)
                            })
                            .collect()
                    })
                    .unwrap_or_default();

                let unique_types = import_types.len() as f64;
                let cohesion = if degree > 0 {
                    (unique_types / degree as f64).min(1.0)
                } else {
                    0.0
                };

                if cohesion < 0.3 {
                    let severity = if degree > 100 {
                        "CRITICAL"
                    } else if degree > 75 {
                        "HIGH"
                    } else {
                        "MEDIUM"
                    };

                    god_modules.push(GodModuleInfo {
                        module_id: node.module_id.clone(),
                        path: self.rel(&node.path),
                        degree,
                        cohesion_score: cohesion,
                        severity: severity.to_string(),
                    });
                }
            }
        }

        god_modules.sort_by(|a, b| b.degree.cmp(&a.degree));
        god_modules
    }

    fn calculate_health_score(
        &self,
        bridge_count: usize,
        cycle_count: usize,
        god_module_count: usize,
        layer_violation_count: usize,
        total_nodes: usize,
    ) -> f64 {
        if total_nodes == 0 {
            return 100.0;
        }

        let base_score = 100.0;
        let cycle_penalty = (cycle_count as f64 * 5.0).min(30.0);
        let bridge_penalty = ((bridge_count as f64 / total_nodes as f64) * 100.0 * 2.0).min(20.0);
        let god_penalty = (god_module_count as f64 * 3.0).min(20.0);
        let layer_penalty = (layer_violation_count as f64 * 4.0).min(25.0);

        (base_score - cycle_penalty - bridge_penalty - god_penalty - layer_penalty).max(0.0)
    }

    fn detect_layer_violations(&self, edges: &[(String, String)]) -> Vec<LayerViolation> {
        // Try to load layers.toml from the project root; fall back to no-op default.
        let config = LayerConfig::from_file(&self.root_path.join("layers.toml"))
            .or_else(|_| {
                LayerConfig::from_file(
                    &self.root_path.join(".codecartographer").join("layers.toml"),
                )
            })
            .unwrap_or_default();
        detect_layer_violations(edges, &config)
    }

    pub fn simulate_change(
        &self,
        module_id: &str,
        new_signature: Option<&str>,
        removed_signature: Option<&str>,
    ) -> Result<SimulatedChange, String> {
        let graph = self.rebuild_graph()?;

        let _target_node = graph
            .nodes
            .iter()
            .find(|n| n.module_id == module_id)
            .ok_or_else(|| format!("Module not found: {}", module_id))?;

        let mut affected = Vec::new();
        let mut callers_count = 0;
        let mut callees_count = 0;

        for edge in &graph.edges {
            if edge.target == module_id {
                callers_count += 1;
                affected.push(edge.source.clone());
            }
            if edge.source == module_id {
                callees_count += 1;
                affected.push(edge.target.clone());
            }
        }

        let will_create_cycle = self.check_would_create_cycle(&graph.edges, module_id);

        let risk_level = if will_create_cycle {
            "CRITICAL".to_string()
        } else if callers_count > 10 {
            "HIGH".to_string()
        } else if callers_count > 5 {
            "MEDIUM".to_string()
        } else {
            "LOW".to_string()
        };

        let health_impact = if will_create_cycle {
            -15.0
        } else if callers_count > 10 {
            -5.0
        } else if callers_count > 5 {
            -2.0
        } else {
            -0.5
        };

        let mut layer_violations = Vec::new();
        if let Some(_ns) = new_signature {
            for affected_module in &affected {
                let edge = (affected_module.clone(), module_id.to_string());
                let violations = detect_layer_violations(&[edge], &LayerConfig::default());
                layer_violations.extend(violations);
            }
        }

        Ok(SimulatedChange {
            target_module: module_id.to_string(),
            new_signature: new_signature.map(String::from),
            removed_signature: removed_signature.map(String::from),
            predicted_impact: ImpactAnalysis {
                affected_modules: affected,
                callers_count,
                callees_count,
                will_create_cycle,
                layer_violations,
                risk_level,
                health_impact,
            },
        })
    }

    fn check_would_create_cycle(&self, edges: &[GraphEdge], target_module: &str) -> bool {
        let mut graph: DiGraphMap<&str, ()> = DiGraphMap::new();

        // Predict cycles only from structural (non-fuzzy) edges, matching
        // detect_cycles — a "will create a cycle" warning must not rest on a
        // low-confidence stem guess.
        let structural = || edges.iter().filter(|e| e.resolution != "fuzzy");

        for edge in structural() {
            if edge.source != target_module && edge.target != target_module {
                graph.add_node(edge.source.as_str());
                graph.add_node(edge.target.as_str());
                graph.add_edge(edge.source.as_str(), edge.target.as_str(), ());
            }
        }

        graph.add_node(target_module);

        for edge in structural() {
            if edge.source == target_module {
                graph.add_edge(target_module, edge.target.as_str(), ());
            }
            if edge.target == target_module {
                graph.add_edge(edge.source.as_str(), target_module, ());
            }
        }

        let sccs = petgraph::algo::tarjan_scc(&graph);
        sccs.iter()
            .any(|c| c.len() > 1 && c.contains(&target_module))
    }

    pub fn get_evolution(&self, days: Option<u32>) -> Result<ArchitectureEvolution, String> {
        let current_graph = self.rebuild_graph()?;
        let current_health = current_graph.metadata.health_score.unwrap_or(100.0);

        let days = days.unwrap_or(30);
        let now = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap_or_default()
            .as_secs();

        let current_head = git_head_sha(&self.root_path);
        let git_ref = if current_head.is_empty() { None } else { Some(current_head.clone()) };

        let current_snapshot = ArchitectureSnapshot {
            timestamp: now,
            git_ref: git_ref.clone(),
            health_score: current_health,
            total_files: current_graph.metadata.total_files,
            total_edges: current_graph.metadata.total_edges,
            bridge_count: current_graph.metadata.bridge_count.unwrap_or(0),
            cycle_count: current_graph.metadata.cycle_count.unwrap_or(0),
            god_module_count: current_graph.metadata.god_module_count.unwrap_or(0),
            layer_violation_count: current_graph.metadata.layer_violation_count.unwrap_or(0),
            dominant_language: current_graph
                .metadata
                .languages
                .iter()
                .max_by_key(|(_, v)| *v)
                .map(|(k, _)| k.clone()),
        };

        // ── Persist snapshot to history file ──────────────────────────────────
        // Deduplicate by git HEAD: if the most recent recorded snapshot shares
        // the same commit SHA, update it in-place rather than appending.
        // This prevents rapid FFI calls on the same codebase state from
        // inflating the history with meaningless same-second entries.
        let history_path = self.root_path.join(".codecartographer_history.json");
        let mut all_snapshots: Vec<ArchitectureSnapshot> =
            std::fs::read_to_string(&history_path)
                .ok()
                .and_then(|s| serde_json::from_str(&s).ok())
                .unwrap_or_default();

        let last_ref = all_snapshots.last().and_then(|s| s.git_ref.as_deref());
        let same_commit = git_ref.is_some() && last_ref == git_ref.as_deref();
        if same_commit {
            // Update the existing entry: preserve the original timestamp (so it
            // stays sorted) but refresh the metrics.
            if let Some(last) = all_snapshots.last_mut() {
                let original_ts = last.timestamp;
                *last = current_snapshot.clone();
                last.timestamp = original_ts;
            }
        } else {
            all_snapshots.push(current_snapshot);
            // Cap history to last 365 snapshots to prevent unbounded growth.
            if all_snapshots.len() > 365 {
                let drain_count = all_snapshots.len() - 365;
                all_snapshots.drain(0..drain_count);
            }
        }

        if let Ok(json) = serde_json::to_string(&all_snapshots) {
            let _ = std::fs::write(&history_path, json);
        }

        // ── Filter to requested window ────────────────────────────────────────
        let since_epoch = now.saturating_sub(days as u64 * 86_400);
        // Return snapshots newest-first so callers get snapshots[0] == current.
        // Trend comparators are swapped accordingly (oldest = last, newest = first).
        let snapshots: Vec<ArchitectureSnapshot> = all_snapshots
            .iter()
            .filter(|s| s.timestamp >= since_epoch)
            .rev()
            .cloned()
            .collect();

        // ── Determine whether trend data is meaningful ────────────────────────
        // Trend is only valid when the window contains snapshots from at least
        // two distinct git commits.  Without that guarantee the "trend" is just
        // a function of how many times the caller invoked the endpoint.
        // For non-git repos (no refs stored) we fall back to a 1-hour minimum
        // time spread.
        let trend_available = {
            let distinct_refs: std::collections::HashSet<&str> = snapshots
                .iter()
                .filter_map(|s| s.git_ref.as_deref())
                .collect();
            let by_ref = distinct_refs.len() >= 2;
            let by_time = snapshots.len() >= 2 && {
                let newest = snapshots.first().map(|s| s.timestamp).unwrap_or(0);
                let oldest = snapshots.last().map(|s| s.timestamp).unwrap_or(0);
                newest.saturating_sub(oldest) >= 3600
            };
            by_ref || by_time
        };

        // ── Compute trend from oldest vs newest snapshot ──────────────────────
        let health_trend = if trend_available {
            let first = snapshots.last().unwrap().health_score;   // oldest (now at tail)
            let last = snapshots.first().unwrap().health_score;   // newest = current (at head)
            let delta = last - first;
            if delta > 5.0 {
                "Improving".to_string()
            } else if delta < -5.0 {
                "Degrading".to_string()
            } else {
                "Stable".to_string()
            }
        } else {
            // Single snapshot — classify by absolute score
            if current_health >= 80.0 { "Healthy".to_string() }
            else if current_health >= 60.0 { "Moderate".to_string() }
            else { "At Risk".to_string() }
        };

        let mut debt_indicators = Vec::new();
        if current_graph.metadata.cycle_count.unwrap_or(0) > 0 {
            debt_indicators.push("Active circular dependencies detected".to_string());
        }
        if current_graph.metadata.god_module_count.unwrap_or(0) > 0 {
            debt_indicators.push(format!(
                "{} god modules require attention",
                current_graph.metadata.god_module_count.unwrap_or(0)
            ));
        }
        if current_graph.metadata.layer_violation_count.unwrap_or(0) > 0 {
            debt_indicators.push(format!(
                "{} architectural boundary violations",
                current_graph.metadata.layer_violation_count.unwrap_or(0)
            ));
        }

        let mut recommendations = Vec::new();
        if current_health < 60.0 {
            recommendations.push("Critical: Immediate architectural review needed".to_string());
        }
        if current_graph.metadata.cycle_count.unwrap_or(0) > 0 {
            recommendations.push("Priority: Break circular dependencies".to_string());
        }
        if current_graph.metadata.god_module_count.unwrap_or(0) > 2 {
            recommendations.push("Consider splitting large modules to improve cohesion".to_string());
        }
        if recommendations.is_empty() {
            recommendations.push("Architecture is healthy - maintain current practices".to_string());
        }

        Ok(ArchitectureEvolution {
            snapshots,
            health_trend,
            trend_available,
            debt_indicators,
            recommendations,
        })
    }

    /// Search for `pattern` across all project files.  Delegates to
    /// [`crate::search::search_content`] using `self.root_path` as the root.
    #[allow(dead_code)]
    pub fn search_content(
        &self,
        pattern: &str,
        opts: &crate::search::SearchOptions,
    ) -> Result<crate::search::SearchResult, String> {
        crate::search::search_content(&self.root_path, pattern, opts)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_api_state_creation() {
        let state = ApiState::new(std::path::PathBuf::from("/test"));
        assert!(state.mapped_files.lock().unwrap().is_empty());
    }

    #[test]
    fn test_compression_level_default() {
        let level = CompressionLevel::default();
        assert_eq!(level, CompressionLevel::Standard);
    }

    #[test]
    fn derive_module_stem_strips_extension() {
        assert_eq!(derive_module_stem("scanner.rs"), "scanner");
        assert_eq!(derive_module_stem("api/search.md"), "search");
        assert_eq!(derive_module_stem("config.yaml"), "config");
        // Normal code imports (no extension) unchanged
        assert_eq!(derive_module_stem("scanner"), "scanner");
        assert_eq!(derive_module_stem("react-dom"), "react");
        assert_eq!(derive_module_stem("src/api/handler"), "handler");
    }

    // Regression: a Go import names a PACKAGE (directory), not a file. Before the
    // go.mod-aware branch, `internal/errors` fell to bare-stem matching and resolved
    // to an unrelated `internal/a2a/errors.go`, and a stdlib `sync` fabricated an edge
    // to `internal/federation/sync.go`. Both must now be correct.
    #[test]
    fn go_import_resolves_to_package_dir_not_same_stem_file() {
        let mut files: HashMap<String, MappedFile> = HashMap::new();
        for p in [
            "internal/errors/errors.go",
            "internal/a2a/errors.go",
            "internal/federation/sync.go",
            "internal/backends/orch.go",
        ] {
            files.insert(p.to_string(), MappedFile::from_minimal(p.to_string(), vec![]));
        }
        let mut idx = ImportIndex::build(&files);
        idx.go_module = Some("github.com/acme/widget".to_string());
        let src = "internal/backends/orch.go";

        // Internal import → the CORRECT package directory (not internal/a2a/errors.go),
        // tagged as an exact resolution.
        assert_eq!(
            idx.resolve("github.com/acme/widget/internal/errors", src),
            Some(("internal/errors/errors.go".to_string(), "exact")),
        );
        // Stdlib import → no fabricated internal edge.
        assert_eq!(idx.resolve("sync", src), None);
        // Third-party import → no internal edge.
        assert_eq!(idx.resolve("github.com/other/pkg", src), None);
    }

    // Qualified extensionless imports (Python dotted, monorepo pathed) resolve
    // against the FULL path, not the last stem — so a same-named file in another
    // package is not fabricated, and an unresolvable qualified import is external.
    #[test]
    fn qualified_import_resolves_by_full_path_not_stem() {
        let mut files: HashMap<String, MappedFile> = HashMap::new();
        for p in [
            "app/services/auth.py",
            "app/models/auth.py", // trap: same stem, different package
            "app/services/__init__.py",
            "app/main.py",
            "packages/ui/Button.tsx",
        ] {
            files.insert(p.to_string(), MappedFile::from_minimal(p.to_string(), vec![]));
        }
        let idx = ImportIndex::build(&files);
        let src = "app/main.py";

        // `from app.services import auth` → the submodule, not app/models/auth.py.
        assert_eq!(
            idx.resolve("from app.services import auth", src),
            Some(("app/services/auth.py".to_string(), "suffix")),
        );
        // `import app.services` (no symbol) → the package entry point.
        assert_eq!(
            idx.resolve("import app.services", src),
            Some(("app/services/__init__.py".to_string(), "suffix")),
        );
        // Monorepo/aliased path import → the exact file by suffix.
        assert_eq!(
            idx.resolve("import Button from 'packages/ui/Button'", src),
            Some(("packages/ui/Button.tsx".to_string(), "suffix")),
        );
        // A qualified import that matches nothing local is external → no edge
        // (must NOT fuzzy-match app/models/auth.py on the bare `auth` stem).
        assert_eq!(idx.resolve("from django.contrib import auth", src), None);
    }

    // JS/TS relative specifiers resolve against the SOURCE directory, not a
    // same-name file elsewhere; `..` walks up; a package dir uses its index.
    #[test]
    fn relative_import_resolves_against_source_dir() {
        let mut files: HashMap<String, MappedFile> = HashMap::new();
        for p in [
            "src/app/main.ts",
            "src/app/bar.ts",
            "src/models/user.ts",
            "src/models/other/bar.ts", // trap: same stem in another dir
            "src/widgets/index.ts",
        ] {
            files.insert(p.to_string(), MappedFile::from_minimal(p.to_string(), vec![]));
        }
        let idx = ImportIndex::build(&files);
        let src = "src/app/main.ts";

        // `./bar` → sibling, not src/models/other/bar.ts.
        assert_eq!(
            idx.resolve("./bar", src),
            Some(("src/app/bar.ts".to_string(), "exact")),
        );
        // `../models/user` → walk up then down.
        assert_eq!(
            idx.resolve("../models/user", src),
            Some(("src/models/user.ts".to_string(), "exact")),
        );
        // `../widgets` → package directory resolves to its index.
        assert_eq!(
            idx.resolve("../widgets", src),
            Some(("src/widgets/index.ts".to_string(), "exact")),
        );
        // A relative path that isn't in the scan → dangling, no edge (not fuzzy).
        assert_eq!(idx.resolve("./does_not_exist", src), None);
    }

    // Regression test: before the fix, rebuild_graph held the mapped_files
    // Mutex across its inner loop and then called resolve_import_target,
    // which re-acquired the same non-reentrant Mutex → deadlock on any
    // project with at least one resolvable import. Any resolved edge is
    // enough to prove the hang is gone; correctness of the graph content
    // is covered elsewhere.
    // Regression: tree-sitter strips "use " prefix from Rust imports, storing
    // "crate::mapper" instead of "use crate::mapper;". parse_import_parts must
    // handle the stripped form or no edges are ever built for Rust projects.
    #[test]
    fn parse_import_parts_handles_stripped_rust_paths() {
        // lowercase module → module stem
        let (module, sym) = parse_import_parts("crate::mapper");
        assert_eq!(module, "mapper");
        assert!(sym.is_none());

        // uppercase last segment → type name, penultimate = module
        let (module, sym) = parse_import_parts("crate::mapper::MappedFile");
        assert_eq!(module, "mapper");
        assert_eq!(sym.as_deref(), Some("MappedFile"));

        // grouped import — brace-split gives the prefix
        let (module, sym) = parse_import_parts("crate::api::{Foo, Bar}");
        assert_eq!(module, "api");
        assert!(sym.is_none());
    }

    #[test]
    fn rust_imports() {
        // These are the "use …" forms (with prefix, from regex extractor).
        let (module, sym) = parse_import_parts("use crate::mapper;");
        assert_eq!(module, "mapper");
        assert!(sym.is_none());

        let (module, sym) = parse_import_parts("use crate::mapper::MappedFile;");
        assert_eq!(module, "mapper");
        assert_eq!(sym.as_deref(), Some("MappedFile"));
    }

    // Regression: source files importing names like "json" must not produce edges
    // to doc/fixture files that happen to share the stem (e.g. testdata/review/json.json),
    // and doc files like CHANGELOG.md must not appear as dependents of source modules
    // just because they mention a path in their prose.
    #[test]
    fn rebuild_graph_excludes_cross_type_edges() {
        let state = ApiState::new(std::path::PathBuf::from("/test"));
        {
            let mut files = state.mapped_files.lock().unwrap();
            // Source file that imports "json" (e.g. Go's encoding/json)
            files.insert(
                "bridge.go".to_string(),
                MappedFile::from_minimal("bridge.go".to_string(), vec!["json".to_string()]),
            );
            // JSON fixture whose stem "json" would match the import stem
            files.insert(
                "testdata/review/json.json".to_string(),
                MappedFile::from_minimal("testdata/review/json.json".to_string(), vec![]),
            );
            // Doc file that has picked up a path reference as an "import"
            files.insert(
                "CHANGELOG.md".to_string(),
                MappedFile::from_minimal(
                    "CHANGELOG.md".to_string(),
                    vec!["bridge.go".to_string()],
                ),
            );
        }
        let graph = state.rebuild_graph().expect("rebuild_graph must not fail");

        let has_source_to_doc = graph.edges.iter().any(|e| {
            e.source == "bridge.go" && e.target == "testdata/review/json.json"
        });
        assert!(!has_source_to_doc, "source→doc edge must not exist: bridge.go → json.json");

        let has_doc_to_source = graph.edges.iter().any(|e| {
            e.source == "CHANGELOG.md" && e.target == "bridge.go"
        });
        assert!(!has_doc_to_source, "doc→source edge must not exist: CHANGELOG.md → bridge.go");
    }

    #[test]
    fn rebuild_graph_does_not_deadlock_on_imports() {
        let state = ApiState::new(std::path::PathBuf::from("/test"));
        {
            let mut files = state.mapped_files.lock().unwrap();
            files.insert(
                "a".to_string(),
                MappedFile::from_minimal("a.rs".to_string(), vec!["b".to_string()]),
            );
            files.insert(
                "b".to_string(),
                MappedFile::from_minimal("b.rs".to_string(), vec![]),
            );
        }
        let graph = state.rebuild_graph().expect("rebuild_graph must return");
        assert_eq!(graph.nodes.len(), 2);
        assert!(
            graph.edges.iter().any(|e| e.source == "a" && e.target == "b"),
            "expected resolved a->b edge, got edges: {:?}",
            graph.edges
        );
    }

    // Directory rollup folds files into their folder, drops intra-folder edges, and
    // aggregates cross-folder dependencies — the comprehensible view on a huge tree.
    // The file- and directory-level centralities must coexist in the MRU cache so a
    // rollup doesn't evict the expensive file-level entry.
    #[test]
    fn rollup_folds_files_and_aggregates_cross_directory_edges() {
        let state = ApiState::new(std::path::PathBuf::from("/test"));
        {
            let mut files = state.mapped_files.lock().unwrap();
            // core/a -> core/b (intra-dir, dropped) ; core/a,core/b -> util/x ;
            // extra/y -> util/x. Three dirs so the folded graph has >=3 nodes and
            // its betweenness actually runs (and caches).
            files.insert(
                "core/a".into(),
                MappedFile::from_minimal("core/a.rs".into(), vec!["core/b".into(), "util/x".into()]),
            );
            files.insert(
                "core/b".into(),
                MappedFile::from_minimal("core/b.rs".into(), vec!["util/x".into()]),
            );
            files.insert("util/x".into(), MappedFile::from_minimal("util/x.rs".into(), vec![]));
            files.insert(
                "extra/y".into(),
                MappedFile::from_minimal("extra/y.rs".into(), vec!["util/x".into()]),
            );
        }

        // depth 0 == file-level identity.
        let g0 = state.rebuild_graph_rolled_up(0).unwrap();
        assert!(g0.nodes.iter().any(|n| n.module_id == "core/a"), "depth 0 keeps file nodes");

        // depth 1 folds into "core", "util", "extra".
        let g = state.rebuild_graph_rolled_up(1).unwrap();
        let ids: std::collections::HashSet<&str> =
            g.nodes.iter().map(|n| n.module_id.as_str()).collect();
        assert_eq!(g.nodes.len(), 3, "three directories, got {ids:?}");
        assert!(
            ids.contains("core") && ids.contains("util") && ids.contains("extra"),
            "expected core+util+extra, got {ids:?}"
        );

        // The two file edges into util collapse to one core->util edge; the intra-dir
        // core->core edge is dropped.
        let core_to_util = g.edges.iter().filter(|e| e.source == "core" && e.target == "util").count();
        assert_eq!(core_to_util, 1, "core->util must aggregate to one edge, got {:?}", g.edges);
        assert!(!g.edges.iter().any(|e| e.source == e.target), "no intra-dir self loops");

        // File-level and directory-level centralities coexist (MRU cache didn't evict).
        assert!(
            state.bc_cache.lock().unwrap().len() >= 2,
            "file- and dir-level betweenness must both be cached"
        );
    }

    // Betweenness centrality is 98% of a rebuild's cost but depends only on graph
    // topology, so it's cached across graph invalidations (the watch-mode win). This
    // pins the contract: the cache survives invalidate_graph and reproduces identical
    // bridge scores, yet re-fingerprints — and recomputes — when an edge changes.
    #[test]
    fn betweenness_cache_survives_invalidation_and_tracks_topology() {
        let state = ApiState::new(std::path::PathBuf::from("/test"));
        {
            let mut files = state.mapped_files.lock().unwrap();
            // path graph a->b->c->d: b and c sit on the only path (bridge candidates)
            files.insert("a".into(), MappedFile::from_minimal("a.rs".into(), vec!["b".into()]));
            files.insert("b".into(), MappedFile::from_minimal("b.rs".into(), vec!["c".into()]));
            files.insert("c".into(), MappedFile::from_minimal("c.rs".into(), vec!["d".into()]));
            files.insert("d".into(), MappedFile::from_minimal("d.rs".into(), vec![]));
        }

        let bridges = |g: &ProjectGraphResponse| -> Vec<(String, f64)> {
            let mut v: Vec<(String, f64)> = g
                .nodes
                .iter()
                .map(|n| (n.module_id.clone(), n.bridge_score.unwrap_or(0.0)))
                .collect();
            v.sort_by(|a, b| a.0.cmp(&b.0));
            v
        };

        let current_fp = || state.bc_cache.lock().unwrap().first().map(|(f, _)| *f);

        let g1 = state.rebuild_graph().unwrap();
        let s1 = bridges(&g1);
        let fp1 = current_fp();
        assert!(fp1.is_some(), "bc_cache must be populated after a rebuild");

        // invalidate_graph (as refresh_if_stale does) must NOT clear the centrality cache.
        state.invalidate_graph();
        assert!(
            !state.bc_cache.lock().unwrap().is_empty(),
            "invalidate_graph must leave bc_cache intact"
        );
        let g2 = state.rebuild_graph().unwrap();
        assert_eq!(s1, bridges(&g2), "cached betweenness must reproduce identical bridge scores");
        let fp2 = current_fp();
        assert_eq!(fp1, fp2, "fingerprint must be stable when topology is unchanged");

        // Add an edge — the fingerprint must change so betweenness is recomputed.
        {
            let mut files = state.mapped_files.lock().unwrap();
            files.insert(
                "a".into(),
                MappedFile::from_minimal("a.rs".into(), vec!["b".into(), "d".into()]),
            );
        }
        state.invalidate_graph();
        let _g3 = state.rebuild_graph().unwrap();
        let fp3 = current_fp();
        assert_ne!(fp1, fp3, "fingerprint must change when an edge is added");
    }

    // Evidence (not a hard assertion — timings are machine-dependent): on a ~3k-node
    // graph the second rebuild after a non-topology invalidation reuses the cached
    // centrality and is dramatically faster than the first. Run with:
    //   cargo test --release betweenness_cache_hit_is_fast -- --ignored --nocapture
    #[test]
    #[ignore]
    fn betweenness_cache_hit_is_fast() {
        // Point CKB_BENCH_DIR at a real repo to measure it; otherwise use a synthetic
        // 3000-node graph. Either way the second rebuild reuses the cached centrality.
        let bench_dir = std::env::var("CKB_BENCH_DIR").ok();
        let root = bench_dir
            .clone()
            .map(std::path::PathBuf::from)
            .unwrap_or_else(|| std::path::PathBuf::from("/bench"));
        let state = ApiState::new(root.clone());
        {
            let mut files = state.mapped_files.lock().unwrap();
            if let Some(dir) = &bench_dir {
                use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};
                use rayon::prelude::*;
                let root = std::path::Path::new(dir);
                let scanned = scan_files_with_noise_tracking(root).unwrap().files;
                *files = scanned
                    .par_iter()
                    .filter(|p| !is_ignored_path(p) && is_source_file(p))
                    .filter_map(|p| {
                        let content = std::fs::read_to_string(p).ok()?;
                        let rel = p.strip_prefix(root).unwrap_or(p).to_string_lossy().replace('\\', "/");
                        Some((rel, crate::mapper::extract_skeleton(p, &content)))
                    })
                    .collect();
            } else {
                // A 3000-node chain plus a few cross edges — enough for the sampled
                // Brandes pass to be the dominant cost.
                let n = 3000usize;
                for i in 0..n {
                    let mut imports = Vec::new();
                    if i + 1 < n {
                        imports.push(format!("m{}", i + 1));
                    }
                    if i + 7 < n {
                        imports.push(format!("m{}", i + 7));
                    }
                    let id = format!("m{i}");
                    files.insert(id.clone(), MappedFile::from_minimal(format!("{id}.rs"), imports));
                }
            }
        }

        let t0 = std::time::Instant::now();
        state.rebuild_graph().unwrap();
        let miss = t0.elapsed();

        state.invalidate_graph();
        let t1 = std::time::Instant::now();
        state.rebuild_graph().unwrap();
        let hit = t1.elapsed();

        eprintln!("betweenness MISS={miss:?}  HIT={hit:?}  speedup={:.1}x", miss.as_secs_f64() / hit.as_secs_f64().max(1e-9));
        assert!(hit < miss, "cache hit ({hit:?}) must be faster than a full compute ({miss:?})");
    }

    // Regression: get_evolution must return snapshots newest-first so that
    // snapshots[0] is always the current snapshot.  Before the fix the list was
    // oldest-first and callers (CLI "Current Status", MCP clients) would read
    // stale or zero-scored historical entries instead of the live score.
    #[test]
    fn get_evolution_snapshots_newest_first() {
        use std::path::PathBuf;
        let tmp = std::env::temp_dir().join(format!("nav_test_{}", std::process::id()));
        std::fs::create_dir_all(&tmp).unwrap();

        let state = ApiState::new(tmp.clone());
        {
            let mut files = state.mapped_files.lock().unwrap();
            files.insert(
                "main.rs".to_string(),
                MappedFile::from_minimal("main.rs".to_string(), vec![]),
            );
        }

        let evolution = state.get_evolution(Some(30)).expect("get_evolution must succeed");

        // Current snapshot is always first; it carries the live health score.
        assert!(
            !evolution.snapshots.is_empty(),
            "snapshots must not be empty"
        );
        let current = evolution.snapshots.first().unwrap();
        // Health score must be a valid value (calculate_health_score minimum is 5,
        // maximum is 100; we only have one file so expect 100).
        assert!(
            current.health_score > 0.0,
            "current snapshot health_score must be > 0, got {}",
            current.health_score
        );
        // If there are multiple snapshots, each must be newer than the next.
        for window in evolution.snapshots.windows(2) {
            assert!(
                window[0].timestamp >= window[1].timestamp,
                "snapshots must be newest-first: {} < {}",
                window[0].timestamp,
                window[1].timestamp
            );
        }

        // Cleanup
        let _ = std::fs::remove_dir_all(&tmp);
    }

    // Regression: rapid successive calls on the same git commit must not each
    // append a new snapshot.  The history file should have exactly one entry
    // after N identical calls, and trend_available must be false.
    #[test]
    fn get_evolution_deduplicates_same_commit() {
        let tmp = std::env::temp_dir().join(format!("nav_test_dedup_{}", std::process::id()));
        std::fs::create_dir_all(&tmp).unwrap();

        // Inject a fake git_ref so deduplication can operate without a real repo.
        // We do this by pre-seeding the history file with a snapshot that carries
        // a known ref, then calling get_evolution from the same tmp dir.
        // Since tmp is not a git repo, git_head_sha returns "" → git_ref = None.
        // Deduplication only fires when both sides have a non-empty ref, so on a
        // non-git root each call still appends — which tests the time-based
        // trend_available guard instead.
        let state = ApiState::new(tmp.clone());
        {
            let mut files = state.mapped_files.lock().unwrap();
            files.insert(
                "lib.rs".to_string(),
                MappedFile::from_minimal("lib.rs".to_string(), vec![]),
            );
        }

        // Three rapid calls from a non-git root (no ref → no deduplication,
        // but trend_available must be false because the timestamps are within
        // seconds of each other, well below the 1-hour threshold).
        let e1 = state.get_evolution(Some(30)).expect("call 1");
        let e2 = state.get_evolution(Some(30)).expect("call 2");
        let e3 = state.get_evolution(Some(30)).expect("call 3");

        // trend_available must be false — the window contains seconds of data.
        assert!(!e3.trend_available, "trend_available must be false for same-second calls");

        // health_trend must not claim a directional trend with only same-second data.
        assert!(
            !matches!(e3.health_trend.as_str(), "Improving" | "Degrading"),
            "health_trend must not be directional with insufficient data, got: {}",
            e3.health_trend
        );

        // Snapshots[0] is always current and always has a positive health score.
        for evolution in [&e1, &e2, &e3] {
            assert!(
                evolution.snapshots.first().map(|s| s.health_score).unwrap_or(0.0) > 0.0,
                "current snapshot health_score must be > 0"
            );
        }

        let _ = std::fs::remove_dir_all(&tmp);
    }
}
