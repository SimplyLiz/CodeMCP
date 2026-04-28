//! Shared diagram renderer for the import graph.
//!
//! Produces Mermaid or Graphviz (DOT) from a `ProjectGraphResponse`, with two
//! node-selection modes:
//!   - No focus → top-N nodes by degree (fallback; "shape of the codebase").
//!   - With focus → BFS from anchor module over import edges up to `depth`
//!     ("shape of the neighborhood I'm editing").
//!
//! Used by both the CLI (`diagram_mode`) and the FFI
//! (`navigator_render_architecture`) so CLI output and MCP output stay in
//! lock-step.

use std::collections::{HashMap, HashSet, VecDeque};

use crate::api::{is_doc_path, ProjectGraphResponse};
use crate::layers::LayerViolationType;

/// Nodes with `hotspot_score` at or above this threshold get the `hot` overlay
/// (thick orange stroke in Mermaid, thicker orange border + larger size in DOT).
/// Picked to match the "top decile" of hotspots on real codebases.
const HOTSPOT_THRESHOLD: f64 = 70.0;

/// Output format requested by the caller.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DiagramFormat {
    Mermaid,
    Dot,
    /// Terminal-friendly indented tree with box-drawing characters.
    /// Always rooted at a single node: `focus` if set, else the blast-radius
    /// epicenter, else the most-connected node in the graph.
    Ascii,
    /// Mermaid `sequenceDiagram` — only valid when rendering a `FileCallGraph`
    /// via `render_sequence()`. Not supported by the import-graph `render()`.
    Sequence,
    /// Mermaid `classDiagram` — only valid when rendering a `ClassGraph`
    /// via `render_class()`. Not supported by the import-graph `render()`.
    Class,
    /// Mermaid `quadrantChart` — churn × complexity scatter over the project graph.
    /// Requires git enrichment; nodes missing either metric are omitted.
    Quadrant,
    /// Mermaid `erDiagram` — entity-relationship view over struct/class fields.
    /// Only valid with `render_er()` on a `ClassGraph`; not for `render()`.
    Er,
}

impl DiagramFormat {
    pub fn parse(s: &str) -> Result<Self, String> {
        match s.to_lowercase().as_str() {
            "mermaid" | "" => Ok(DiagramFormat::Mermaid),
            "dot" | "graphviz" => Ok(DiagramFormat::Dot),
            "ascii" | "tree" | "text" => Ok(DiagramFormat::Ascii),
            "sequence" | "seq" => Ok(DiagramFormat::Sequence),
            "class" | "uml" => Ok(DiagramFormat::Class),
            "quadrant" => Ok(DiagramFormat::Quadrant),
            "er" | "entity" | "erd" => Ok(DiagramFormat::Er),
            other => Err(format!("unknown diagram format: {other}")),
        }
    }
}

/// Rendering options. `focus` is a module_id (or suffix match on a path/module_id).
///
/// Selection precedence: `blast_radius` > `focus` > top-by-degree.
#[derive(Debug, Clone)]
pub struct RenderOptions<'a> {
    pub format: DiagramFormat,
    pub focus: Option<&'a str>,
    pub depth: usize,
    pub max_nodes: usize,
    /// When `Some(threshold)`, overlay dotted purple edges for every co-change
    /// pair whose `coupling_score >= threshold` and whose both endpoints are in
    /// the included node set. `None` disables the overlay (default).
    pub show_cochange: Option<f64>,
    /// When `Some(target)`, override selection: included = {target} ∪ direct
    /// dependencies ∪ direct dependents. The target module renders as an
    /// epicenter (bold red fill). `None` uses the focus/top-by-degree path.
    pub blast_radius: Option<&'a str>,
    /// When `true`, filter the selection to the doc subgraph: all document
    /// nodes (markdown/YAML/TOML/JSON) plus every code file they directly
    /// reference. Docs render with a distinct shape regardless of this flag.
    pub docs_only: bool,
    /// When `Some(n)`, collapse the graph to folder granularity at path depth
    /// `n` before rendering. All files whose path shares the same first `n`
    /// directory components become a single folder node; edges are aggregated
    /// (self-loops dropped, counts summed). Combines with focus/blast-radius —
    /// selection happens after collapsing, so anchors must match folder ids.
    pub group_by_folder_depth: Option<usize>,
    /// When `true`, replace role-based node fills with owner-derived colors
    /// (dominant git author mapped to a stable palette). Nodes without an
    /// `owner` value fall through to the default (white/grey). Overlay borders
    /// (cycle/pivot/hot/epicenter) still take precedence.
    pub color_by_owner: bool,
}

impl<'a> RenderOptions<'a> {
    /// Convenience constructor that fills every new overlay option with `None`.
    /// Intended for call sites that only care about the base top-by-degree /
    /// focused rendering and don't want to list every overlay field.
    #[allow(dead_code)]
    pub fn basic(format: DiagramFormat, max_nodes: usize) -> Self {
        Self {
            format,
            focus: None,
            depth: 2,
            max_nodes,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }
    }
}

/// Hash an author name into the shared palette. Palette picked for reasonable
/// contrast on white and for staying distinguishable when several owners
/// appear in the same diagram. Stable across runs — the same owner always
/// lands on the same color.
fn owner_color(owner: &str) -> &'static str {
    // 10-color palette. Order matters — first entries are the most visually
    // distinct from each other; later entries fall back to neighbors.
    const PALETTE: &[&str] = &[
        "#a6cee3", "#b2df8a", "#fb9a99", "#fdbf6f",
        "#cab2d6", "#ffff99", "#1f78b4", "#33a02c",
        "#e31a1c", "#ff7f00",
    ];
    // FNV-1a 32-bit hash; good-enough distribution for a handful of owners.
    let mut h: u32 = 0x811c9dc5;
    for b in owner.bytes() {
        h ^= b as u32;
        h = h.wrapping_mul(0x01000193);
    }
    PALETTE[(h as usize) % PALETTE.len()]
}

/// Rendered diagram plus a truncation flag so callers can tell the model to
/// tighten `focus` or lower `depth` when the cap kicked in.
#[derive(Debug, Clone)]
pub struct RenderedDiagram {
    pub diagram: String,
    pub truncated: bool,
    #[allow(dead_code)]
    pub node_count: usize,
    /// Module ids that made it into the render. Exposed so downstream
    /// exporters (e.g. the interactive HTML builder) can reuse the selection
    /// without re-running focus/blast-radius logic.
    #[allow(dead_code)]
    pub included: Vec<String>,
}

/// Precomputed overlays that decorate the base import graph with architectural
/// signals: cycles (from `graph.cycles`), layer violations (from
/// `graph.layer_violations`), co-change pairs (from `graph.cochange_pairs`),
/// hotspot nodes (from `GraphNode.hotspot_score`), and an epicenter marker for
/// blast-radius renderings.
///
/// We precompute once per `render()` so both Mermaid and DOT rendering paths
/// consult the same sets and stay visually consistent.
struct Overlays<'a> {
    cycle_nodes: HashSet<&'a str>,
    pivot_nodes: HashSet<&'a str>,
    cycle_edges: HashSet<(&'a str, &'a str)>,
    violations: HashMap<(&'a str, &'a str), &'a LayerViolationType>,
    /// Co-change pairs above threshold, keyed by (file_a, file_b). We don't
    /// key symmetrically here — the renderer iterates this map and filters by
    /// `included_set`, treating each pair as a single undirected coupling edge.
    cochange: HashMap<(&'a str, &'a str), f64>,
    /// The target of a blast-radius selection, if any. Rendered as an
    /// epicenter (bold red fill) so the "you are here" is unambiguous.
    epicenter: Option<&'a str>,
}

fn compute_overlays<'a>(
    graph: &'a ProjectGraphResponse,
    show_cochange: Option<f64>,
    epicenter: Option<&'a str>,
) -> Overlays<'a> {
    let mut cycle_nodes: HashSet<&str> = HashSet::new();
    let mut pivot_nodes: HashSet<&str> = HashSet::new();
    let mut cycle_edges: HashSet<(&str, &str)> = HashSet::new();

    for cycle in &graph.cycles {
        let members: HashSet<&str> = cycle.nodes.iter().map(|s| s.as_str()).collect();
        for n in &cycle.nodes {
            cycle_nodes.insert(n.as_str());
        }
        if let Some(pivot) = &cycle.pivot_node {
            pivot_nodes.insert(pivot.as_str());
        }
        // An edge participates in this cycle iff both endpoints are cycle members.
        for edge in &graph.edges {
            if members.contains(edge.source.as_str()) && members.contains(edge.target.as_str()) {
                cycle_edges.insert((edge.source.as_str(), edge.target.as_str()));
            }
        }
    }

    let mut violations: HashMap<(&str, &str), &LayerViolationType> = HashMap::new();
    for v in &graph.layer_violations {
        // LayerViolation.source_path/target_path are actually module_ids
        // (they come from edge_tuples in api.rs, which clone edge.source/target).
        violations.insert(
            (v.source_path.as_str(), v.target_path.as_str()),
            &v.violation_type,
        );
    }

    let mut cochange: HashMap<(&str, &str), f64> = HashMap::new();
    if let Some(threshold) = show_cochange {
        for p in &graph.cochange_pairs {
            if p.coupling_score >= threshold {
                cochange.insert((p.file_a.as_str(), p.file_b.as_str()), p.coupling_score);
            }
        }
    }

    Overlays { cycle_nodes, pivot_nodes, cycle_edges, violations, cochange, epicenter }
}

/// Doc-map selection. Included = all doc nodes ∪ every code file they connect
/// to (either as source or target of an edge). Docs are identified via
/// `api::is_doc_path`. Ordered by edge count descending so the most-connected
/// docs survive `max_nodes` truncation first.
fn docs_only_selection(
    graph: &ProjectGraphResponse,
    max_nodes: usize,
) -> (Vec<String>, bool) {
    let doc_ids: HashSet<&str> = graph
        .nodes
        .iter()
        .filter(|n| is_doc_path(&n.path))
        .map(|n| n.module_id.as_str())
        .collect();

    let mut neighbors: HashSet<&str> = HashSet::new();
    for edge in &graph.edges {
        if doc_ids.contains(edge.source.as_str()) {
            neighbors.insert(edge.target.as_str());
        }
        if doc_ids.contains(edge.target.as_str()) {
            neighbors.insert(edge.source.as_str());
        }
    }

    // Rank each candidate by its edge count in the full graph so truncation
    // keeps the most-connected nodes. Doc nodes are listed before code
    // neighbors so a heavy truncation still shows the docs themselves.
    let mut degree: HashMap<&str, usize> = HashMap::new();
    for edge in &graph.edges {
        *degree.entry(edge.source.as_str()).or_insert(0) += 1;
        *degree.entry(edge.target.as_str()).or_insert(0) += 1;
    }

    let mut docs: Vec<&str> = doc_ids.iter().copied().collect();
    docs.sort_by(|a, b| {
        degree.get(b).copied().unwrap_or(0)
            .cmp(&degree.get(a).copied().unwrap_or(0))
            .then_with(|| a.cmp(b))
    });

    let mut code: Vec<&str> = neighbors.difference(&doc_ids).copied().collect();
    code.sort_by(|a, b| {
        degree.get(b).copied().unwrap_or(0)
            .cmp(&degree.get(a).copied().unwrap_or(0))
            .then_with(|| a.cmp(b))
    });

    let mut ordered: Vec<String> = docs.iter().map(|s| s.to_string()).collect();
    ordered.extend(code.iter().map(|s| s.to_string()));

    let truncated = ordered.len() > max_nodes;
    ordered.truncate(max_nodes);
    (ordered, truncated)
}

/// Blast-radius selection. Included = {target} ∪ direct deps ∪ direct dependents,
/// capped at `max_nodes`. Computed purely from the graph — no `ApiState` needed.
///
/// The target is resolved with the same rules as `bfs_from_anchor`: exact
/// module_id, exact path, then path/module_id suffix match.
fn blast_radius_selection(
    graph: &ProjectGraphResponse,
    target: &str,
    max_nodes: usize,
) -> Result<(Vec<String>, bool), String> {
    let resolved = graph
        .nodes
        .iter()
        .find(|n| n.module_id == target)
        .or_else(|| graph.nodes.iter().find(|n| n.path == target))
        .or_else(|| {
            graph
                .nodes
                .iter()
                .find(|n| n.module_id.ends_with(target) || n.path.ends_with(target))
        })
        .ok_or_else(|| format!("blast-radius target not found in graph: {target}"))?;

    let epicenter_id = resolved.module_id.clone();
    let mut included: Vec<String> = Vec::new();
    let mut seen: HashSet<String> = HashSet::new();

    // Epicenter first so it stays in the output even under truncation.
    included.push(epicenter_id.clone());
    seen.insert(epicenter_id.clone());

    // Direct dependencies: where epicenter is the source.
    for edge in &graph.edges {
        if edge.source == epicenter_id && seen.insert(edge.target.clone()) {
            included.push(edge.target.clone());
        }
    }
    // Direct dependents: where epicenter is the target.
    for edge in &graph.edges {
        if edge.target == epicenter_id && seen.insert(edge.source.clone()) {
            included.push(edge.source.clone());
        }
    }

    let truncated = included.len() > max_nodes;
    included.truncate(max_nodes);
    Ok((included, truncated))
}

/// Collapse a project graph to folder granularity. All files whose path shares
/// the same first `depth` directory components are merged into a single folder
/// node; edges are aggregated, with intra-folder self-loops dropped. Signature
/// counts sum; hotspot score is the max across member files. Language is set
/// to `"folder"` so renderers can give folder nodes a distinct shape.
///
/// `depth` of 0 collapses everything to a single root — not useful, so we
/// treat 0 as "don't collapse". `depth` beyond any file's directory depth just
/// keeps that file as its own node (folder = its full parent path).
fn collapse_by_folder(graph: &ProjectGraphResponse, depth: usize) -> ProjectGraphResponse {
    use crate::api::{GraphMetadata, GraphNode, GraphEdge};

    fn folder_key(path: &str, depth: usize) -> String {
        let parts: Vec<&str> = path.split('/').collect();
        // File sits at parts[parts.len()-1]; directories are parts[0..len-1].
        let dir_parts = &parts[..parts.len().saturating_sub(1)];
        let take = depth.min(dir_parts.len());
        if take == 0 {
            // File sits at the root — group under "(root)" so it's one folder.
            "(root)".to_string()
        } else {
            dir_parts[..take].join("/")
        }
    }

    // Map each module_id to its folder id.
    let mut member_folder: HashMap<String, String> = HashMap::new();
    // Aggregate per-folder state.
    let mut folder_files: HashMap<String, Vec<&crate::api::GraphNode>> = HashMap::new();

    for node in &graph.nodes {
        let fid = folder_key(&node.path, depth);
        member_folder.insert(node.module_id.clone(), fid.clone());
        folder_files.entry(fid).or_default().push(node);
    }

    // Build folder nodes. We stash the file count in `signature_count`'s sibling
    // field `fan_in` so the renderer can show "N files" — but simpler: encode it
    // directly in the label via a dedicated render branch keyed off language.
    let mut nodes: Vec<GraphNode> = Vec::with_capacity(folder_files.len());
    for (fid, files) in &folder_files {
        let signature_count: usize = files.iter().map(|n| n.signature_count).sum();
        let hotspot_score = files
            .iter()
            .filter_map(|n| n.hotspot_score)
            .fold(None::<f64>, |acc, v| Some(acc.map_or(v, |a| a.max(v))));
        // fan_in repurposed to carry member file count for the renderer label.
        let member_count: usize = files.len();

        nodes.push(GraphNode {
            module_id: fid.clone(),
            path: fid.clone(),
            language: "folder".into(),
            signature_count,
            complexity: None,
            is_bridge: None,
            bridge_score: None,
            degree: None,
            risk_level: None,
            churn: None,
            hotspot_score,
            role: None,
            is_dead: None,
            unreferenced_exports: None,
            fan_in: Some(member_count),
            fan_out: None,
            cochange_partners: None,
            cochange_entropy: None,
            owner: None,
        });
    }

    // Aggregate edges. (src_folder, tgt_folder) → count. Drop self-loops.
    let mut edge_counts: HashMap<(String, String), u32> = HashMap::new();
    for e in &graph.edges {
        let Some(sf) = member_folder.get(&e.source) else { continue };
        let Some(tf) = member_folder.get(&e.target) else { continue };
        if sf == tf {
            continue;
        }
        *edge_counts.entry((sf.clone(), tf.clone())).or_insert(0) += 1;
    }

    let edges: Vec<GraphEdge> = edge_counts
        .into_iter()
        .map(|((src, tgt), _)| GraphEdge {
            source: src,
            target: tgt,
            edge_type: "import".into(),
            at_range: None,
        })
        .collect();

    // Cycles/violations/cochange don't survive collapse — they describe the
    // file-level graph and would be ambiguous at folder granularity. Callers
    // who want those overlays should render the file-level view.
    ProjectGraphResponse {
        nodes,
        edges,
        cycles: vec![],
        god_modules: vec![],
        layer_violations: vec![],
        metadata: GraphMetadata {
            total_files: graph.metadata.total_files,
            total_edges: graph.metadata.total_edges,
            languages: HashMap::new(),
            generated_at: graph.metadata.generated_at.clone(),
            bridge_count: None,
            cycle_count: None,
            god_module_count: None,
            health_score: None,
            layer_violation_count: None,
            architectural_drift: None,
            hotspot_count: None,
            dead_code_count: None,
            unreferenced_exports_count: None,
        },
        cochange_pairs: vec![],
    }
}

/// Render an import-graph diagram. Pure over `graph` — no I/O.
pub fn render(graph: &ProjectGraphResponse, opts: &RenderOptions) -> Result<RenderedDiagram, String> {
    let max_nodes = opts.max_nodes.max(1);

    // Folder collapse happens before anything else so focus/blast_radius and
    // overlays all see the collapsed view. Overlays derived from the file-level
    // graph (cycles, violations, cochange) are intentionally dropped.
    let collapsed: Option<ProjectGraphResponse> = opts
        .group_by_folder_depth
        .filter(|&d| d > 0)
        .map(|d| collapse_by_folder(graph, d));
    let graph: &ProjectGraphResponse = collapsed.as_ref().unwrap_or(graph);

    // Selection precedence: blast_radius > focus > docs_only > top-by-degree.
    // Blast radius resolves the target and overrides everything else so
    // callers don't have to null neighboring options.
    let (included, truncated, epicenter) = match (opts.blast_radius, opts.focus, opts.docs_only) {
        (Some(target), _, _) => {
            let (inc, trunc) = blast_radius_selection(graph, target, max_nodes)?;
            // `inc[0]` is the epicenter module_id (pushed first in selection).
            let epi = inc.first().cloned();
            (inc, trunc, epi)
        }
        (None, Some(anchor), _) => {
            let (inc, trunc) = bfs_from_anchor(graph, anchor, opts.depth, max_nodes)?;
            (inc, trunc, None)
        }
        (None, None, true) => {
            let (inc, trunc) = docs_only_selection(graph, max_nodes);
            (inc, trunc, None)
        }
        (None, None, false) => {
            let (inc, trunc) = top_by_degree(graph, max_nodes);
            (inc, trunc, None)
        }
    };

    let included_set: HashSet<&str> = included.iter().map(|s| s.as_str()).collect();

    // Map module_id -> node for stable lookup during rendering.
    let node_by_id: HashMap<&str, &crate::api::GraphNode> = graph
        .nodes
        .iter()
        .map(|n| (n.module_id.as_str(), n))
        .collect();

    // We need a stable &str borrow of the epicenter id for the Overlays lifetime.
    // Reuse the node_by_id key to get a borrow that lives as long as `graph`.
    let epicenter_ref: Option<&str> = epicenter
        .as_deref()
        .and_then(|id| node_by_id.get_key_value(id).map(|(k, _)| *k));

    let overlays = compute_overlays(graph, opts.show_cochange, epicenter_ref);

    let content = match opts.format {
        DiagramFormat::Dot => render_dot(&included, &included_set, &node_by_id, graph, &overlays, opts.color_by_owner),
        DiagramFormat::Mermaid => render_mermaid(&included, &included_set, &node_by_id, graph, &overlays, opts.color_by_owner),
        DiagramFormat::Ascii => render_ascii(
            &included, &included_set, &node_by_id, graph, &overlays,
            opts.focus, opts.blast_radius, opts.depth,
        ),
        DiagramFormat::Quadrant => render_quadrant(&included, &node_by_id),
        DiagramFormat::Sequence | DiagramFormat::Class | DiagramFormat::Er => {
            return Err("use render_sequence() / render_class() / render_er() for sequence, class, and ER diagrams — render() only handles import graphs".to_string());
        }
    };

    let node_count = included.len();
    Ok(RenderedDiagram { diagram: content, truncated, node_count, included })
}

/// Build the node selection without rendering. Exposed so the HTML exporter
/// (and other future non-text renderers) can reuse the same selection rules
/// as Mermaid/DOT — focus, blast-radius, docs-only, folder-collapse.
///
/// Returns `(graph_to_render, included_module_ids, truncated)`. When folder
/// collapsing is active, `graph_to_render` is an owned collapsed
/// `ProjectGraphResponse`; otherwise it's `None` and the caller uses the
/// original graph.
#[allow(dead_code)]
pub fn select_for_render(
    graph: &ProjectGraphResponse,
    opts: &RenderOptions,
) -> Result<(Option<ProjectGraphResponse>, Vec<String>, bool), String> {
    let max_nodes = opts.max_nodes.max(1);
    let collapsed: Option<ProjectGraphResponse> = opts
        .group_by_folder_depth
        .filter(|&d| d > 0)
        .map(|d| collapse_by_folder(graph, d));
    let g: &ProjectGraphResponse = collapsed.as_ref().unwrap_or(graph);

    let (included, truncated, _epi) = match (opts.blast_radius, opts.focus, opts.docs_only) {
        (Some(target), _, _) => {
            let (inc, trunc) = blast_radius_selection(g, target, max_nodes)?;
            let epi = inc.first().cloned();
            (inc, trunc, epi)
        }
        (None, Some(anchor), _) => {
            let (inc, trunc) = bfs_from_anchor(g, anchor, opts.depth, max_nodes)?;
            (inc, trunc, None)
        }
        (None, None, true) => {
            let (inc, trunc) = docs_only_selection(g, max_nodes);
            (inc, trunc, None)
        }
        (None, None, false) => {
            let (inc, trunc) = top_by_degree(g, max_nodes);
            (inc, trunc, None)
        }
    };
    Ok((collapsed, included, truncated))
}

fn top_by_degree(graph: &ProjectGraphResponse, max_nodes: usize) -> (Vec<String>, bool) {
    let mut degree: HashMap<&str, usize> = HashMap::new();
    for edge in &graph.edges {
        *degree.entry(edge.source.as_str()).or_insert(0) += 1;
        *degree.entry(edge.target.as_str()).or_insert(0) += 1;
    }

    let mut ranked: Vec<&crate::api::GraphNode> = graph
        .nodes
        .iter()
        .filter(|n| degree.get(n.module_id.as_str()).copied().unwrap_or(0) > 0)
        .collect();
    ranked.sort_by(|a, b| {
        let da = degree.get(a.module_id.as_str()).copied().unwrap_or(0);
        let db = degree.get(b.module_id.as_str()).copied().unwrap_or(0);
        db.cmp(&da)
            .then_with(|| a.module_id.cmp(&b.module_id))
    });

    let truncated = ranked.len() > max_nodes;
    ranked.truncate(max_nodes);

    (ranked.into_iter().map(|n| n.module_id.clone()).collect(), truncated)
}

/// Undirected BFS from `anchor` over import edges. We treat imports as
/// bidirectional here because "the area I'm editing" includes both what I
/// import *and* what imports me — callers usually want the full neighborhood.
fn bfs_from_anchor(
    graph: &ProjectGraphResponse,
    anchor: &str,
    depth: usize,
    max_nodes: usize,
) -> Result<(Vec<String>, bool), String> {
    // Resolve anchor: accept exact module_id match, then path suffix match.
    let resolved = graph
        .nodes
        .iter()
        .find(|n| n.module_id == anchor)
        .or_else(|| graph.nodes.iter().find(|n| n.path == anchor))
        .or_else(|| graph.nodes.iter().find(|n| n.module_id.ends_with(anchor) || n.path.ends_with(anchor)))
        .ok_or_else(|| format!("focus not found in graph: {anchor}"))?;

    let start = resolved.module_id.clone();

    // Build an adjacency map (undirected) once.
    let mut adj: HashMap<&str, Vec<&str>> = HashMap::new();
    for edge in &graph.edges {
        adj.entry(edge.source.as_str()).or_default().push(edge.target.as_str());
        adj.entry(edge.target.as_str()).or_default().push(edge.source.as_str());
    }

    let mut visited: HashSet<String> = HashSet::new();
    let mut order: Vec<String> = Vec::new();
    let mut queue: VecDeque<(String, usize)> = VecDeque::new();

    visited.insert(start.clone());
    order.push(start.clone());
    queue.push_back((start, 0));

    let mut truncated = false;

    while let Some((module, d)) = queue.pop_front() {
        if d >= depth {
            continue;
        }
        if let Some(neighbors) = adj.get(module.as_str()) {
            for &n in neighbors {
                if visited.insert(n.to_string()) {
                    if order.len() >= max_nodes {
                        truncated = true;
                        // Drain the queue so we stop adding further frontier nodes.
                        queue.clear();
                        break;
                    }
                    order.push(n.to_string());
                    queue.push_back((n.to_string(), d + 1));
                }
            }
        }
    }

    Ok((order, truncated))
}

fn render_dot(
    included: &[String],
    included_set: &HashSet<&str>,
    node_by_id: &HashMap<&str, &crate::api::GraphNode>,
    graph: &ProjectGraphResponse,
    overlays: &Overlays,
    color_by_owner: bool,
) -> String {
    let mut out = String::from("digraph navigator {\n    rankdir=LR;\n");
    for module_id in included {
        let Some(node) = node_by_id.get(module_id.as_str()) else { continue };
        let label = node.path.rsplit('/').next().unwrap_or(&node.path);
        let fill = if color_by_owner {
            node.owner.as_deref().map(owner_color).unwrap_or("#fff")
        } else {
            role_color_dot(node.role.as_deref())
        };

        let mid = module_id.as_str();
        let is_epicenter = overlays.epicenter == Some(mid);
        let is_pivot = overlays.pivot_nodes.contains(mid);
        let in_cycle = overlays.cycle_nodes.contains(mid);
        let score = node.hotspot_score.unwrap_or(0.0).clamp(0.0, 100.0);
        let hot = score >= HOTSPOT_THRESHOLD;

        // Epicenter overrides everything — this is the "you are here" marker
        // for blast-radius renderings. Otherwise: pivot > cycle > hot > default.
        // Pivot is dashed to distinguish it inside a red-bordered cycle.
        let (fill_override, border_color, pen_width, extra_style) = if is_epicenter {
            (Some("#ff3333"), "#660000", 4.0, "")
        } else if is_pivot {
            (None, "#cc0000", 3.0, ",dashed")
        } else if in_cycle {
            (None, "#cc0000", 3.0, "")
        } else if hot {
            (None, "#ff6600", 3.0, "")
        } else {
            (None, "#333333", 1.0, "")
        };
        let actual_fill = fill_override.unwrap_or(fill);

        // Hotspot-driven sizing. score ∈ [0,100] → width ∈ [0.75, 1.80],
        // height ∈ [0.50, 0.90], fontsize ∈ [10, 16]. Nodes without a score
        // render at the default size.
        let width = 0.75 + (score / 100.0) * 1.05;
        let height = 0.50 + (score / 100.0) * 0.40;
        let fontsize = 10 + ((score / 100.0) * 6.0) as u32;

        // Doc nodes render as `shape=note` with a light yellow fill so readers
        // can distinguish documentation from code at a glance. Folder-collapsed
        // nodes use `shape=folder` with a light blue fill and a "(N files)"
        // count inline in the label. Epicenter fill still wins when set.
        let is_doc = is_doc_path(&node.path);
        let is_folder = node.language == "folder";
        let shape = if is_folder {
            "folder"
        } else if is_doc {
            "note"
        } else {
            "box"
        };
        let final_fill = if fill_override.is_some() {
            actual_fill
        } else if is_folder {
            "#d6e9ff"
        } else if is_doc {
            "#fff4c0"
        } else {
            actual_fill
        };
        let unit_label = if is_doc { "sec" } else { "fn" };

        if is_folder {
            let files = node.fan_in.unwrap_or(0);
            let folder_label = if node.module_id == "(root)" { "(root)" } else { label };
            out.push_str(&format!(
                "    \"{}\" [label=\"{}/\\n{} files, {} fn\" shape={} style=\"filled{}\" fillcolor=\"{}\" color=\"{}\" penwidth={:.1} width={:.2} height={:.2} fontsize={}];\n",
                node.module_id, folder_label, files, node.signature_count,
                shape, extra_style, final_fill, border_color, pen_width, width, height, fontsize
            ));
        } else {
            out.push_str(&format!(
                "    \"{}\" [label=\"{}\\n{} {}\" shape={} style=\"filled{}\" fillcolor=\"{}\" color=\"{}\" penwidth={:.1} width={:.2} height={:.2} fontsize={}];\n",
                node.module_id, label, node.signature_count, unit_label,
                shape, extra_style, final_fill, border_color, pen_width, width, height, fontsize
            ));
        }
    }
    for edge in &graph.edges {
        if !(included_set.contains(edge.source.as_str())
            && included_set.contains(edge.target.as_str()))
        {
            continue;
        }
        let key = (edge.source.as_str(), edge.target.as_str());
        let viol = overlays.violations.get(&key).copied();
        let in_cycle = overlays.cycle_edges.contains(&key);

        let (color, style, pen) = match viol {
            Some(LayerViolationType::BackCall)
            | Some(LayerViolationType::CircularCrossLayer) => ("#cc0000", "dashed", 2.5),
            Some(LayerViolationType::SkipCall) => ("#ff9900", "dotted", 2.0),
            Some(LayerViolationType::DirectForeignImport) => ("#cccc00", "dotted", 1.5),
            None if in_cycle => ("#cc0000", "solid", 2.5),
            None => ("#666666", "solid", 1.0),
        };

        out.push_str(&format!(
            "    \"{}\" -> \"{}\" [color=\"{}\" style={} penwidth={:.1}];\n",
            edge.source, edge.target, color, style, pen
        ));
    }

    // Co-change overlay edges. `constraint=false` keeps DOT's layout engine
    // from treating these as part of the import DAG — they'd otherwise pull
    // unrelated nodes together and blow up the layout. Rendered bidirectionally
    // as `arrowhead=none` to signal these are coupling, not dependency.
    for ((a, b), score) in &overlays.cochange {
        if !(included_set.contains(a) && included_set.contains(b)) {
            continue;
        }
        out.push_str(&format!(
            "    \"{}\" -> \"{}\" [color=\"#8844cc\" style=dotted penwidth={:.1} arrowhead=none constraint=false label=\"{:.2}\" fontsize=9 fontcolor=\"#8844cc\"];\n",
            a, b, 1.0 + score * 2.0, score
        ));
    }

    out.push('}');
    out
}

fn render_mermaid(
    included: &[String],
    included_set: &HashSet<&str>,
    node_by_id: &HashMap<&str, &crate::api::GraphNode>,
    graph: &ProjectGraphResponse,
    overlays: &Overlays,
    color_by_owner: bool,
) -> String {
    let mut out = String::from("flowchart TD\n");
    out.push_str("    classDef bridge fill:#f96,stroke:#333\n");
    out.push_str("    classDef core fill:#9cf,stroke:#333\n");
    out.push_str("    classDef dead fill:#ccc,stroke:#333\n");
    out.push_str("    classDef entry fill:#9f9,stroke:#333\n");
    out.push_str("    classDef cycle stroke:#c00,stroke-width:3px\n");
    out.push_str("    classDef pivot stroke:#c00,stroke-width:3px,stroke-dasharray:5 5\n");
    out.push_str("    classDef hot stroke:#f60,stroke-width:3px\n");
    out.push_str("    classDef epicenter fill:#f33,stroke:#600,stroke-width:4px,color:#fff\n");
    out.push_str("    classDef doc fill:#fff4c0,stroke:#aa8,stroke-dasharray:3 2\n");
    out.push_str("    classDef folder fill:#d6e9ff,stroke:#468,stroke-width:2px\n");

    let id_map: HashMap<&str, usize> = included
        .iter()
        .enumerate()
        .map(|(i, m)| (m.as_str(), i))
        .collect();

    // Node declarations carry the inline role class (:::core / :::bridge / etc).
    // Overlay classes (cycle/pivot/hot) are applied via separate `class` statements
    // below so a node can wear multiple classes without relying on inline chaining.
    // Doc nodes use Mermaid stadium shape `([...])` + "sec" label; folder nodes
    // use subroutine shape `[[...]]` + "N files, M fn" label.
    for module_id in included {
        let Some(node) = node_by_id.get(module_id.as_str()) else { continue };
        let i = id_map[module_id.as_str()];
        let label = node.path.rsplit('/').next().unwrap_or(&node.path);
        let is_doc = is_doc_path(&node.path);
        let is_folder = node.language == "folder";
        // In owner-color mode we drop the role class suffix so the per-node
        // `style` directive we emit below wins without fighting the classDef.
        let class_suffix = if color_by_owner { "" } else { role_class_suffix(node.role.as_deref()) };
        if is_folder {
            let files = node.fan_in.unwrap_or(0);
            let folder_label = if node.module_id == "(root)" { "(root)" } else { label };
            out.push_str(&format!(
                "    N{}[[\"{}/\\n{} files, {} fn\"]]{}\n",
                i, folder_label, files, node.signature_count, class_suffix
            ));
        } else {
            let unit_label = if is_doc { "sec" } else { "fn" };
            let (open, close) = if is_doc { ("([\"", "\"])") } else { ("[\"", "\"]") };
            out.push_str(&format!(
                "    N{}{}{}\\n{} {}{}{}\n",
                i, open, label, node.signature_count, unit_label, close, class_suffix
            ));
        }
    }

    // Owner coloring emits per-node style directives. Overlay borders
    // (cycle/pivot/hot/epicenter) are applied via stroke-only classes, so
    // they don't collide with the fill we set here.
    if color_by_owner {
        for module_id in included {
            let Some(node) = node_by_id.get(module_id.as_str()) else { continue };
            let i = id_map[module_id.as_str()];
            if let Some(owner) = node.owner.as_deref() {
                out.push_str(&format!(
                    "    style N{} fill:{},stroke:#333\n",
                    i, owner_color(owner)
                ));
            }
        }
    }

    // Overlay class assignments. Epicenter wins outright so blast-radius
    // renderings have an unambiguous "you are here" marker. Otherwise pivot
    // takes precedence over cycle so pivots are visually distinguishable
    // inside a cycle.
    for module_id in included {
        let Some(node) = node_by_id.get(module_id.as_str()) else { continue };
        let i = id_map[module_id.as_str()];
        let mid = module_id.as_str();
        let mut extras: Vec<&str> = Vec::new();
        if overlays.epicenter == Some(mid) {
            extras.push("epicenter");
        } else if overlays.pivot_nodes.contains(mid) {
            extras.push("pivot");
        } else if overlays.cycle_nodes.contains(mid) {
            extras.push("cycle");
        }
        if node.hotspot_score.unwrap_or(0.0) >= HOTSPOT_THRESHOLD
            && overlays.epicenter != Some(mid)
        {
            extras.push("hot");
        }
        // Doc nodes get the `doc` overlay class on top of whatever else they
        // wear. Epicenter still wins visually because `class` statements apply
        // in order and later declarations override earlier ones in Mermaid.
        if is_doc_path(&node.path) && overlays.epicenter != Some(mid) {
            extras.push("doc");
        }
        // Folder nodes get the `folder` overlay class (blue fill + thick stroke).
        if node.language == "folder" && overlays.epicenter != Some(mid) {
            extras.push("folder");
        }
        if !extras.is_empty() {
            out.push_str(&format!("    class N{} {};\n", i, extras.join(",")));
        }
    }

    // Edges. We emit them in source order and remember each edge's index so we
    // can append `linkStyle` directives for cycle/violation edges at the end.
    let mut edge_index: usize = 0;
    let mut link_styles: Vec<(usize, &'static str)> = Vec::new();
    for edge in &graph.edges {
        if !(included_set.contains(edge.source.as_str())
            && included_set.contains(edge.target.as_str()))
        {
            continue;
        }
        let (Some(&si), Some(&ti)) = (
            id_map.get(edge.source.as_str()),
            id_map.get(edge.target.as_str()),
        ) else {
            continue;
        };
        let key = (edge.source.as_str(), edge.target.as_str());
        let viol = overlays.violations.get(&key).copied();
        let in_cycle = overlays.cycle_edges.contains(&key);

        // Arrow: `==>` for plain cycles, `-.->` for any violation (dotted
        // Mermaid arrow covers both back-calls and skip-calls visually;
        // linkStyle below distinguishes them by colour/dash).
        let arrow = match (viol, in_cycle) {
            (Some(_), _) => "-.->",
            (None, true) => "==>",
            (None, false) => "-->",
        };
        out.push_str(&format!("    N{} {} N{}\n", si, arrow, ti));

        let style: Option<&'static str> = match viol {
            Some(LayerViolationType::BackCall)
            | Some(LayerViolationType::CircularCrossLayer) => {
                Some("stroke:#c00,stroke-width:2.5px,stroke-dasharray:6 3")
            }
            Some(LayerViolationType::SkipCall) => {
                Some("stroke:#f90,stroke-width:2px,stroke-dasharray:3 3")
            }
            Some(LayerViolationType::DirectForeignImport) => {
                Some("stroke:#cc0,stroke-width:1.5px,stroke-dasharray:2 2")
            }
            None if in_cycle => Some("stroke:#c00,stroke-width:2.5px"),
            None => None,
        };
        if let Some(s) = style {
            link_styles.push((edge_index, s));
        }
        edge_index += 1;
    }

    // Co-change overlay edges. Mermaid lacks a directionless arrow; we use
    // `---` (plain line) so readers don't mistake these for imports. Each gets
    // a linkStyle directive that dashes them purple.
    for ((a, b), score) in &overlays.cochange {
        if !(included_set.contains(a) && included_set.contains(b)) {
            continue;
        }
        let (Some(&ai), Some(&bi)) = (id_map.get(a), id_map.get(b)) else {
            continue;
        };
        // Mermaid uses `---|label|` for edge labels on undirected-style lines.
        out.push_str(&format!("    N{} ---|{:.2}| N{}\n", ai, score, bi));
        link_styles.push((
            edge_index,
            "stroke:#84c,stroke-width:2px,stroke-dasharray:2 4",
        ));
        edge_index += 1;
    }

    for (idx, style) in link_styles {
        out.push_str(&format!("    linkStyle {} {}\n", idx, style));
    }
    out
}

/// Render a terminal-friendly indented tree. Always single-rooted — the idea
/// is "what does this one module reach, and where does it fit" which falls
/// apart if we emit a forest. Cycles are broken with a `↑ seen` marker so the
/// output stays bounded and readable.
///
/// Root selection: explicit `focus` → blast_radius epicenter → first node in
/// `included` (which is top-by-degree #1 under the default selection).
#[allow(clippy::too_many_arguments)]
fn render_ascii(
    included: &[String],
    included_set: &HashSet<&str>,
    node_by_id: &HashMap<&str, &crate::api::GraphNode>,
    graph: &ProjectGraphResponse,
    overlays: &Overlays,
    focus: Option<&str>,
    blast_radius: Option<&str>,
    depth: usize,
) -> String {
    // Directed adjacency over included edges only. We walk imports in their
    // natural direction (source → target) so the tree reads "X depends on Y".
    let mut adj: HashMap<&str, Vec<&str>> = HashMap::new();
    for edge in &graph.edges {
        let s = edge.source.as_str();
        let t = edge.target.as_str();
        if included_set.contains(s) && included_set.contains(t) {
            adj.entry(s).or_default().push(t);
        }
    }
    for targets in adj.values_mut() {
        targets.sort();
        targets.dedup();
    }

    // Pick the root. Fall back through explicit > epicenter > best by out-degree.
    //
    // "First included" as the fallback is what top_by_degree gives us (#1 by
    // total degree), but for a tree that's the wrong signal: a node with high
    // in-degree and zero out-degree would render as a lone root with an
    // empty subtree. We want the node that *reaches* the most, so we rank
    // included nodes by out-degree within included_set before falling back.
    let root: &str = match (focus, blast_radius, overlays.epicenter, included.first()) {
        (Some(anchor), _, _, _) => {
            // Re-resolve the same way bfs_from_anchor did so we land on the
            // actual module_id (the anchor may have been a path suffix).
            included
                .iter()
                .find(|m| m.as_str() == anchor)
                .map(|s| s.as_str())
                .or_else(|| {
                    included
                        .iter()
                        .find(|m| {
                            node_by_id
                                .get(m.as_str())
                                .map(|n| n.path.ends_with(anchor) || n.module_id.ends_with(anchor))
                                .unwrap_or(false)
                        })
                        .map(|s| s.as_str())
                })
                .or_else(|| included.first().map(|s| s.as_str()))
                .unwrap_or("")
        }
        (None, Some(_), Some(epi), _) => epi,
        (None, None, _, Some(_)) => {
            let best = included
                .iter()
                .map(|m| m.as_str())
                .max_by_key(|m| adj.get(m).map(|v| v.len()).unwrap_or(0));
            best.unwrap_or("")
        }
        _ => "",
    };

    if root.is_empty() {
        return String::from("(empty graph)\n");
    }

    // DFS with visited tracking. `depth` from RenderOptions is the traversal
    // cap; 0 means "just the root". We default the practical cap to 32 if the
    // caller passed 0, so top-by-degree invocations still produce useful output.
    let effective_depth = if depth == 0 && focus.is_none() { 32 } else { depth };

    let mut out = String::new();
    // Header line: the root itself, un-prefixed.
    out.push_str(&ascii_label(root, node_by_id, overlays));
    out.push('\n');

    let mut visited: HashSet<&str> = HashSet::new();
    visited.insert(root);

    let children: Vec<&str> = adj.get(root).cloned().unwrap_or_default();
    for (i, child) in children.iter().enumerate() {
        let is_last = i + 1 == children.len();
        ascii_walk(
            child,
            &adj,
            node_by_id,
            overlays,
            &mut visited,
            &mut out,
            "",
            is_last,
            1,
            effective_depth,
        );
    }

    // Orphans: other included nodes not reachable from the root. Report as a
    // flat tail so the user sees them without losing the tree structure.
    let mut orphans: Vec<&str> = included
        .iter()
        .map(|s| s.as_str())
        .filter(|m| !visited.contains(m))
        .collect();
    if !orphans.is_empty() {
        orphans.sort();
        out.push_str("\n(disconnected)\n");
        for (i, m) in orphans.iter().enumerate() {
            let is_last = i + 1 == orphans.len();
            let branch = if is_last { "└── " } else { "├── " };
            out.push_str(branch);
            out.push_str(&ascii_label(m, node_by_id, overlays));
            out.push('\n');
        }
    }

    out
}

#[allow(clippy::too_many_arguments)]
fn ascii_walk<'a>(
    node: &'a str,
    adj: &HashMap<&'a str, Vec<&'a str>>,
    node_by_id: &HashMap<&'a str, &crate::api::GraphNode>,
    overlays: &Overlays,
    visited: &mut HashSet<&'a str>,
    out: &mut String,
    prefix: &str,
    is_last: bool,
    current_depth: usize,
    max_depth: usize,
) {
    let branch = if is_last { "└── " } else { "├── " };
    out.push_str(prefix);
    out.push_str(branch);

    if visited.contains(node) {
        // Cycle or re-entry — emit a terminator so the output stays bounded.
        out.push_str(&ascii_label(node, node_by_id, overlays));
        out.push_str("  ↑ seen\n");
        return;
    }
    visited.insert(node);

    out.push_str(&ascii_label(node, node_by_id, overlays));
    out.push('\n');

    if current_depth >= max_depth {
        return;
    }

    let children: Vec<&str> = adj.get(node).cloned().unwrap_or_default();
    if children.is_empty() {
        return;
    }

    let child_prefix = format!("{}{}", prefix, if is_last { "    " } else { "│   " });
    for (i, child) in children.iter().enumerate() {
        let last = i + 1 == children.len();
        ascii_walk(
            child,
            adj,
            node_by_id,
            overlays,
            visited,
            out,
            &child_prefix,
            last,
            current_depth + 1,
            max_depth,
        );
    }
}

fn ascii_label(
    module_id: &str,
    node_by_id: &HashMap<&str, &crate::api::GraphNode>,
    overlays: &Overlays,
) -> String {
    let Some(node) = node_by_id.get(module_id) else {
        return module_id.to_string();
    };
    let name = node.path.rsplit('/').next().unwrap_or(&node.path);
    let unit = if is_doc_path(&node.path) { "sec" } else { "fn" };

    // Overlay markers — mirror what Mermaid/DOT apply, but flattened into
    // ASCII-safe suffixes.
    let mut tags: Vec<&str> = Vec::new();
    if overlays.epicenter == Some(module_id) {
        tags.push("★ epicenter");
    }
    if overlays.cycle_nodes.contains(module_id) {
        tags.push("◉ cycle");
    }
    if overlays.pivot_nodes.contains(module_id) {
        tags.push("✦ pivot");
    }
    let is_hot = node.hotspot_score.unwrap_or(0.0) >= HOTSPOT_THRESHOLD;
    if is_hot {
        tags.push("♨ hot");
    }
    if let Some(role) = node.role.as_deref() {
        // Role is a short word (core/bridge/dead/entry); inline it plainly.
        tags.push(role);
    }

    let tag_suffix = if tags.is_empty() {
        String::new()
    } else {
        format!(" [{}]", tags.join(", "))
    };

    format!("{}  ({} {}){}", name, node.signature_count, unit, tag_suffix)
}

/// Render a `ProjectGraphResponse` selection as a Mermaid `quadrantChart`.
///
/// Each node in `included` that has `churn` data becomes a point. The Y axis uses
/// `complexity` when populated, falling back to `signature_count` until tree-sitter
/// cyclomatic complexity extraction is implemented. Coordinates are min-max
/// normalised to `[0.01, 0.99]`; nodes without churn (no git history) are omitted.
fn render_quadrant(
    included: &[String],
    node_by_id: &HashMap<&str, &crate::api::GraphNode>,
) -> String {
    // Collect nodes that have churn data. Complexity falls back to
    // signature_count when tree-sitter cyclomatic complexity isn't populated —
    // that field is planned but not yet extracted.
    let qualifying: Vec<(&crate::api::GraphNode, usize, u32)> = included
        .iter()
        .filter_map(|id| {
            let node = *node_by_id.get(id.as_str())?;
            let churn = node.churn?;
            let complexity = node.complexity
                .unwrap_or(node.signature_count as u32);
            Some((node, churn, complexity))
        })
        .collect();

    let omitted = included.len().saturating_sub(qualifying.len());

    let mut out = String::from("quadrantChart\n");
    out.push_str("    title Churn vs Complexity\n");
    out.push_str("    x-axis Low Churn --> High Churn\n");
    out.push_str("    y-axis Low Complexity --> High Complexity\n");
    out.push_str("    quadrant-1 Danger zone\n");
    out.push_str("    quadrant-2 Risky debt\n");
    out.push_str("    quadrant-3 Stable\n");
    out.push_str("    quadrant-4 Hotspots\n");

    if qualifying.is_empty() {
        if omitted > 0 {
            out.push_str(&format!("%% {} nodes omitted (no churn data — run in a git repo)\n", omitted));
        }
        return out;
    }

    let min_churn = qualifying.iter().map(|(_, c, _)| *c).min().unwrap_or(0) as f64;
    let max_churn = qualifying.iter().map(|(_, c, _)| *c).max().unwrap_or(1) as f64;
    let min_cplx = qualifying.iter().map(|(_, _, k)| *k).min().unwrap_or(0) as f64;
    let max_cplx = qualifying.iter().map(|(_, _, k)| *k).max().unwrap_or(1) as f64;

    let norm = |v: f64, lo: f64, hi: f64| -> f64 {
        if (hi - lo).abs() < f64::EPSILON {
            0.5
        } else {
            0.01 + (v - lo) / (hi - lo) * 0.98
        }
    };

    for (node, churn, complexity) in &qualifying {
        let x = norm(*churn as f64, min_churn, max_churn);
        let y = norm(*complexity as f64, min_cplx, max_cplx);
        // Use the last path component as the label; strip chars that break the
        // quadrantChart parser (colon is the key-value separator).
        let label = node.module_id
            .rsplit('/')
            .next()
            .unwrap_or(&node.module_id)
            .replace(':', "_");
        out.push_str(&format!("    {}: [{:.2}, {:.2}]\n", label, x, y));
    }

    if omitted > 0 {
        out.push_str(&format!("%% {} nodes omitted (no churn data — run in a git repo)\n", omitted));
    }

    out
}

// render_sequence / render_class / render_er are only called from the binary (main.rs);
// the lib-only analysis treats them as unused. Same situation as diagram_export.rs.
#[allow(dead_code)]
/// Render a `FileCallGraph` as a Mermaid `sequenceDiagram`.
///
/// Participants are the functions defined in the file (in source order).
/// Messages are the call edges in AST order — an approximation of execution
/// order for top-level flows. Self-calls are kept (Mermaid renders them as a
/// loop arrow). Unresolved external call count is surfaced as a note so the
/// reader knows what's dropped.
pub fn render_sequence(cg: &crate::call_graph::FileCallGraph) -> RenderedDiagram {
    let mut out = String::from("sequenceDiagram\n");

    // Stable participant ID: replace "::" and "." separators with "_" so Mermaid
    // doesn't parse them as namespace syntax.
    let participant_id = |q: &str| q.replace("::", "_").replace('.', "_");

    for f in &cg.functions {
        out.push_str(&format!(
            "    participant {} as {}\n",
            participant_id(&f.qualified),
            f.simple,
        ));
    }

    for (caller, callee) in &cg.calls {
        out.push_str(&format!(
            "    {}->>{}: call\n",
            participant_id(caller),
            participant_id(callee),
        ));
    }

    if cg.unresolved_count > 0 {
        // Attach the note to the first participant, or skip if the graph is empty.
        if let Some(first) = cg.functions.first() {
            out.push_str(&format!(
                "    Note right of {}: {} unresolved external calls dropped\n",
                participant_id(&first.qualified),
                cg.unresolved_count,
            ));
        }
    }

    let node_count = cg.functions.len();
    RenderedDiagram {
        diagram: out,
        truncated: false,
        node_count,
        included: cg.functions.iter().map(|f| f.qualified.clone()).collect(),
    }
}

#[allow(dead_code)]
/// Render a `CrossCallTrace` as a Mermaid `sequenceDiagram`.
///
/// One participant per unique module. Messages show the callee function name.
/// Heuristic-matched edges are annotated with `(~)` so reviewers can spot
/// lower-confidence steps.
pub fn render_cross_sequence(trace: &crate::cross_call::CrossCallTrace) -> RenderedDiagram {
    use crate::cross_call::ResolutionKind;
    use std::collections::LinkedList;

    // Collect participants in first-seen order.
    let mut seen: std::collections::HashSet<String> = std::collections::HashSet::new();
    let mut participants: LinkedList<String> = LinkedList::new();
    let add = |mid: &str, seen: &mut std::collections::HashSet<String>, p: &mut LinkedList<String>| {
        if seen.insert(mid.to_string()) { p.push_back(mid.to_string()); }
    };
    add(&trace.entry_module, &mut seen, &mut participants);
    for step in &trace.steps {
        add(&step.from.0, &mut seen, &mut participants);
        add(&step.to.0, &mut seen, &mut participants);
    }

    let mut out = String::from("sequenceDiagram\n");

    // Participant aliases: use the file stem as the display label.
    for mid in &participants {
        let label = std::path::Path::new(mid)
            .file_stem()
            .and_then(|s| s.to_str())
            .unwrap_or(mid);
        let id = label.replace(['-', '.'], "_");
        out.push_str(&format!("    participant {} as {}\n", id, label));
    }

    for step in &trace.steps {
        let from_label = std::path::Path::new(&step.from.0)
            .file_stem().and_then(|s| s.to_str()).unwrap_or(&step.from.0);
        let to_label = std::path::Path::new(&step.to.0)
            .file_stem().and_then(|s| s.to_str()).unwrap_or(&step.to.0);
        let from_id = from_label.replace(['-', '.'], "_");
        let to_id = to_label.replace(['-', '.'], "_");
        let callee_simple = step.to.1.split("::").last().unwrap_or(&step.to.1);
        let suffix = if step.confidence == ResolutionKind::Heuristic { " (~)" } else { "" };
        out.push_str(&format!("    {}->>{}:{}{}\n", from_id, to_id, callee_simple, suffix));
    }

    if !trace.unmatched.is_empty() {
        if let Some(first) = participants.front() {
            let first_label = std::path::Path::new(first)
                .file_stem().and_then(|s| s.to_str()).unwrap_or(first);
            let first_id = first_label.replace(['-', '.'], "_");
            out.push_str(&format!(
                "    Note right of {}: {} calls unresolved (stdlib / external)\n",
                first_id,
                trace.unmatched.len()
            ));
        }
    }

    let module_count = seen.len();
    RenderedDiagram {
        diagram: out,
        truncated: false,
        node_count: module_count,
        included: seen.into_iter().collect(),
    }
}

#[allow(dead_code)]
/// Render a `ClassGraph` as a Mermaid `classDiagram`.
///
/// Each `ClassNode` becomes a Mermaid class block. Fields and methods carry
/// Mermaid visibility prefixes (`+` public, `-` private, `#` protected).
/// Trait/interface nodes get the `<<interface>>` annotation; enums get
/// `<<enumeration>>`. Relationships (`Inherits`, `Implements`) are emitted
/// as Mermaid arrows after all class blocks.
pub fn render_class(graph: &crate::class_graph::ClassGraph) -> RenderedDiagram {
    use crate::class_graph::{ClassKind, ClassRelationship, Vis};

    let mut out = String::from("classDiagram\n");

    let vis_char = |v: &Vis| match v {
        Vis::Public => '+',
        Vis::Private => '-',
        Vis::Protected => '#',
    };

    // Sanitise a name for Mermaid: replace "::" / "." separators and strip
    // generic angle-brackets which confuse the Mermaid parser.
    let mmd_id = |s: &str| {
        s.split('<').next().unwrap_or(s)
            .replace("::", "_")
            .replace('.', "_")
    };

    for cls in &graph.classes {
        out.push_str(&format!("    class {} {{\n", mmd_id(&cls.name)));

        match cls.kind {
            ClassKind::Interface | ClassKind::Trait => {
                out.push_str("        <<interface>>\n");
            }
            ClassKind::Enum => {
                out.push_str("        <<enumeration>>\n");
            }
            _ => {}
        }

        for field in &cls.fields {
            out.push_str(&format!(
                "        {}{} {}\n",
                vis_char(&field.visibility),
                field.type_annotation,
                field.name,
            ));
        }

        for method in &cls.methods {
            let ctor_marker = if method.is_constructor { " <<create>>" } else { "" };
            let ret = if method.return_type.is_empty() {
                String::new()
            } else {
                format!(" {}", method.return_type)
            };
            out.push_str(&format!(
                "        {}{}({}){}{}\n",
                vis_char(&method.visibility),
                method.name,
                method.params,
                ret,
                ctor_marker,
            ));
        }

        out.push_str("    }\n");
    }

    for rel in &graph.relationships {
        match rel {
            ClassRelationship::Inherits { child, parent } => {
                out.push_str(&format!("    {} --|> {}\n", mmd_id(child), mmd_id(parent)));
            }
            ClassRelationship::Implements { class, interface } => {
                out.push_str(&format!("    {} ..|> {}\n", mmd_id(class), mmd_id(interface)));
            }
        }
    }

    let node_count = graph.classes.len();
    RenderedDiagram {
        diagram: out,
        truncated: false,
        node_count,
        included: graph.classes.iter().map(|c| c.name.clone()).collect(),
    }
}

#[allow(dead_code)]
/// Render a `ClassGraph` as a Mermaid `erDiagram`.
///
/// Each `ClassNode` becomes an entity. Fields whose stripped type matches another
/// known class name produce a relationship edge. Cardinality is inferred from the
/// wrapper: `Vec<T>` → one-to-many, `Option<T>` → zero-or-one, bare `T` →
/// exactly-one. `Box<T>`, `Arc<T>`, `Rc<T>`, and `&T` are treated as exactly-one.
pub fn render_er(graph: &crate::class_graph::ClassGraph) -> RenderedDiagram {
    let class_names: std::collections::HashSet<&str> =
        graph.classes.iter().map(|c| c.name.as_str()).collect();

    // Strip common single-type wrappers and return (inner_type, mermaid_cardinality).
    fn strip_wrapper(ty: &str) -> (&str, &'static str) {
        let ty = ty.trim();
        for prefix in &["Vec<", "HashSet<", "BTreeSet<", "VecDeque<"] {
            if let Some(inner) = ty.strip_prefix(prefix).and_then(|s| s.strip_suffix('>')) {
                return (inner.trim(), "||--o{");
            }
        }
        if let Some(inner) = ty.strip_prefix("Option<").and_then(|s| s.strip_suffix('>')) {
            return (inner.trim(), "||--o|");
        }
        for prefix in &["Box<", "Arc<", "Rc<", "Cell<", "RefCell<", "Mutex<"] {
            if let Some(inner) = ty.strip_prefix(prefix).and_then(|s| s.strip_suffix('>')) {
                return (inner.trim(), "||--||");
            }
        }
        if let Some(inner) = ty.strip_prefix('&') {
            return (inner.trim().trim_start_matches("mut "), "||--||");
        }
        (ty, "||--||")
    }

    // Sanitise a name for the erDiagram parser: no spaces, brackets, or colons.
    let er_id = |s: &str| -> String {
        s.split('<').next().unwrap_or(s)
            .replace("::", "_")
            .replace(['.', '-', ' '], "_")
    };

    // Sanitise a type annotation for display inside an entity block.
    // Keep only the outermost type name (strip generics).
    let er_type = |s: &str| -> String {
        er_id(s.split('<').next().unwrap_or(s))
    };

    let mut out = String::from("erDiagram\n");

    // Entity blocks.
    for cls in &graph.classes {
        out.push_str(&format!("    {} {{\n", er_id(&cls.name)));
        for field in &cls.fields {
            let display_type = er_type(&field.type_annotation);
            out.push_str(&format!("        {} {}\n", display_type, field.name));
        }
        out.push_str("    }\n");
    }

    // Relationship edges — deduplicated via a HashSet.
    let mut seen: std::collections::HashSet<String> = std::collections::HashSet::new();
    let mut rel_count = 0usize;
    for cls in &graph.classes {
        for field in &cls.fields {
            let (bare, cardinality) = strip_wrapper(&field.type_annotation);
            // One more strip for nested wrappers (e.g. Option<Vec<T>>).
            let (bare, cardinality) = if cardinality == "||--o|" {
                let (b2, c2) = strip_wrapper(bare);
                if c2 == "||--o{" { (b2, c2) } else { (bare, cardinality) }
            } else {
                (bare, cardinality)
            };
            let bare_id = er_id(bare);
            if class_names.contains(bare) && bare_id != er_id(&cls.name) {
                let key = format!("{} {} {}", er_id(&cls.name), cardinality, bare_id);
                if seen.insert(key) {
                    out.push_str(&format!(
                        "    {} {} {} : \"has\"\n",
                        er_id(&cls.name), cardinality, bare_id
                    ));
                    rel_count += 1;
                }
            }
        }
    }

    RenderedDiagram {
        diagram: out,
        truncated: false,
        node_count: rel_count,
        included: graph.classes.iter().map(|c| c.name.clone()).collect(),
    }
}

fn role_color_dot(role: Option<&str>) -> &'static str {
    match role {
        Some("core") => "#9cf",
        Some("bridge") => "#f96",
        Some("dead") => "#ccc",
        Some("entry") => "#9f9",
        _ => "#fff",
    }
}

fn role_class_suffix(role: Option<&str>) -> &'static str {
    match role {
        Some("core") => ":::core",
        Some("bridge") => ":::bridge",
        Some("dead") => ":::dead",
        Some("entry") => ":::entry",
        _ => "",
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::api::{CycleInfo, GraphEdge, GraphMetadata, GraphNode, ProjectGraphResponse};
    use crate::layers::{LayerViolation, LayerViolationType};
    use std::collections::HashMap;

    fn node(id: &str, role: Option<&str>) -> GraphNode {
        GraphNode {
            module_id: id.into(),
            path: format!("src/{}.rs", id),
            language: "rust".into(),
            signature_count: 3,
            complexity: None,
            is_bridge: None,
            bridge_score: None,
            degree: None,
            risk_level: None,
            churn: None,
            hotspot_score: None,
            role: role.map(String::from),
            is_dead: None,
            unreferenced_exports: None,
            fan_in: None,
            fan_out: None,
            cochange_partners: None,
            cochange_entropy: None,
            owner: None,
        }
    }

    fn edge(src: &str, tgt: &str) -> GraphEdge {
        GraphEdge {
            source: src.into(),
            target: tgt.into(),
            edge_type: "import".into(),
            at_range: None,
        }
    }

    fn fixture() -> ProjectGraphResponse {
        ProjectGraphResponse {
            nodes: vec![
                node("a", Some("core")),
                node("b", None),
                node("c", Some("bridge")),
                node("d", None),
                node("isolated", None),
            ],
            edges: vec![edge("a", "b"), edge("b", "c"), edge("c", "d")],
            cycles: vec![],
            god_modules: vec![],
            layer_violations: vec![],
            metadata: GraphMetadata {
                total_files: 5,
                total_edges: 3,
                languages: HashMap::new(),
                generated_at: "".into(),
                bridge_count: None,
                cycle_count: None,
                god_module_count: None,
                health_score: None,
                layer_violation_count: None,
                architectural_drift: None,
                hotspot_count: None,
                dead_code_count: None,
                unreferenced_exports_count: None,
            },
            cochange_pairs: vec![],
        }
    }

    #[test]
    fn top_n_skips_isolated_nodes() {
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert!(!r.diagram.contains("isolated"));
        assert_eq!(r.node_count, 4);
        assert!(!r.truncated);
    }

    #[test]
    fn top_n_truncates_and_reports() {
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 2,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert!(r.truncated);
        assert_eq!(r.node_count, 2);
    }

    #[test]
    fn focus_bfs_expands_neighborhood() {
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: Some("a"),
            depth: 1,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        // depth=1 from a → reaches b but not c
        assert_eq!(r.node_count, 2);
    }

    #[test]
    fn focus_bfs_depth_two_reaches_further() {
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: Some("a"),
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert_eq!(r.node_count, 3); // a, b, c
    }

    #[test]
    fn focus_accepts_path_suffix() {
        let g = fixture();
        // path is "src/a.rs" — match by suffix
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: Some("a.rs"),
            depth: 1,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert_eq!(r.node_count, 2);
    }

    #[test]
    fn focus_not_found_returns_error() {
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: Some("does_not_exist"),
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        });
        assert!(r.is_err());
    }

    #[test]
    fn dot_output_has_role_colors() {
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Dot,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert!(r.diagram.starts_with("digraph navigator {"));
        assert!(r.diagram.contains("#9cf")); // core color present for node a
    }

    #[test]
    fn format_parse_accepts_aliases_and_rejects_unknown() {
        assert_eq!(DiagramFormat::parse("mermaid").unwrap(), DiagramFormat::Mermaid);
        assert_eq!(DiagramFormat::parse("MERMAID").unwrap(), DiagramFormat::Mermaid);
        assert_eq!(DiagramFormat::parse("").unwrap(), DiagramFormat::Mermaid);
        assert_eq!(DiagramFormat::parse("dot").unwrap(), DiagramFormat::Dot);
        assert_eq!(DiagramFormat::parse("graphviz").unwrap(), DiagramFormat::Dot);
        assert!(DiagramFormat::parse("svg").is_err());
    }

    #[test]
    fn focus_bfs_is_undirected() {
        // "The area I'm editing" includes both what I import and what imports
        // me. Verify BFS from a leaf picks up its importers, not just its
        // imports. Here `d` is imported by `c` (edge c→d) but imports nothing.
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: Some("d"),
            depth: 1,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert_eq!(r.node_count, 2); // d + its importer c
        assert!(r.diagram.contains("c.rs"));
    }

    #[test]
    fn focus_respects_node_cap() {
        // depth=2 from a would reach {a,b,c}; cap at 2 should truncate.
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: Some("a"),
            depth: 2,
            max_nodes: 2,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert_eq!(r.node_count, 2);
        assert!(r.truncated);
    }

    #[test]
    fn focus_bfs_handles_cycles_without_looping() {
        // Add a cycle a→b→c→a and BFS should still terminate and not
        // duplicate nodes in the output.
        let mut g = fixture();
        g.edges.push(edge("c", "a"));
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: Some("a"),
            depth: 5,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        // a, b, c, d reachable undirected; no duplicates.
        assert_eq!(r.node_count, 4);
    }

    #[test]
    fn mermaid_output_declares_classes_and_direction() {
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert!(r.diagram.starts_with("flowchart TD\n"));
        assert!(r.diagram.contains("classDef core"));
        assert!(r.diagram.contains("classDef bridge"));
        // Role-tagged nodes carry their class suffix.
        assert!(r.diagram.contains(":::core"));
        assert!(r.diagram.contains(":::bridge"));
        // Overlay classes are always declared so later `class` statements resolve.
        assert!(r.diagram.contains("classDef cycle"));
        assert!(r.diagram.contains("classDef pivot"));
        assert!(r.diagram.contains("classDef hot"));
    }

    fn cycle(nodes: &[&str], pivot: Option<&str>) -> CycleInfo {
        CycleInfo {
            nodes: nodes.iter().map(|s| s.to_string()).collect(),
            pivot_node: pivot.map(String::from),
            severity: "high".into(),
        }
    }

    fn violation(src: &str, tgt: &str, vt: LayerViolationType) -> LayerViolation {
        LayerViolation {
            source_path: src.into(),
            target_path: tgt.into(),
            source_layer: "x".into(),
            target_layer: "y".into(),
            violation_type: vt,
            severity: "CRITICAL".into(),
        }
    }

    #[test]
    fn mermaid_marks_cycle_nodes_edges_and_pivot() {
        let mut g = fixture();
        g.edges.push(edge("c", "a")); // closes a → b → c → a
        g.cycles.push(cycle(&["a", "b", "c"], Some("b")));

        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();

        // Cycle edges use thick arrow and pick up a linkStyle.
        assert!(r.diagram.contains("==>"), "expected cycle edges to use ==>:\n{}", r.diagram);
        assert!(r.diagram.contains("linkStyle"), "expected linkStyle for cycle edges");

        // Pivot takes precedence over cycle — node b gets the pivot class.
        assert!(
            r.diagram.lines().any(|l| l.trim_start().starts_with("class N") && l.contains("pivot")),
            "expected a class statement assigning pivot:\n{}", r.diagram
        );
        // Non-pivot cycle members still get the cycle class.
        assert!(
            r.diagram.lines().any(|l| l.trim_start().starts_with("class N") && l.contains("cycle")),
            "expected a class statement assigning cycle:\n{}", r.diagram
        );
    }

    #[test]
    fn dot_marks_cycle_edges_red() {
        let mut g = fixture();
        g.edges.push(edge("c", "a"));
        g.cycles.push(cycle(&["a", "b", "c"], None));

        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Dot,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();

        // At least one cycle edge must carry the red colour and solid style.
        let cycle_edge_line = r
            .diagram
            .lines()
            .find(|l| l.contains("\"a\" -> \"b\"") || l.contains("\"b\" -> \"c\"") || l.contains("\"c\" -> \"a\""))
            .expect("cycle edge should be rendered");
        assert!(cycle_edge_line.contains("#cc0000"), "cycle edge missing red colour: {}", cycle_edge_line);

        // Non-cycle edges stay grey.
        assert!(r.diagram.contains("#666666") || !g.edges.iter().any(|e| {
            let members = ["a", "b", "c"];
            !members.contains(&e.source.as_str()) || !members.contains(&e.target.as_str())
        }));
    }

    #[test]
    fn mermaid_marks_layer_violations() {
        let mut g = fixture();
        g.layer_violations.push(violation("a", "b", LayerViolationType::BackCall));
        g.layer_violations.push(violation("b", "c", LayerViolationType::SkipCall));

        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();

        // Both violations use the dotted-violation arrow.
        assert!(r.diagram.contains("-.->"), "expected dotted arrow for violations:\n{}", r.diagram);
        // linkStyle distinguishes the two by colour.
        assert!(r.diagram.contains("stroke:#c00"), "expected red stroke for BackCall");
        assert!(r.diagram.contains("stroke:#f90"), "expected orange stroke for SkipCall");
    }

    #[test]
    fn dot_marks_layer_violations_with_style_and_colour() {
        let mut g = fixture();
        g.layer_violations.push(violation("a", "b", LayerViolationType::BackCall));
        g.layer_violations.push(violation("b", "c", LayerViolationType::SkipCall));

        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Dot,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();

        let back = r.diagram.lines().find(|l| l.contains("\"a\" -> \"b\"")).unwrap();
        assert!(back.contains("#cc0000"), "BackCall edge missing red: {}", back);
        assert!(back.contains("style=dashed"), "BackCall edge missing dashed: {}", back);

        let skip = r.diagram.lines().find(|l| l.contains("\"b\" -> \"c\"")).unwrap();
        assert!(skip.contains("#ff9900"), "SkipCall edge missing orange: {}", skip);
        assert!(skip.contains("style=dotted"), "SkipCall edge missing dotted: {}", skip);
    }

    #[test]
    fn dot_sizes_hot_nodes_and_applies_orange_border() {
        let mut g = fixture();
        if let Some(n) = g.nodes.iter_mut().find(|n| n.module_id == "a") {
            n.hotspot_score = Some(90.0);
        }

        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Dot,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();

        let hot_line = r.diagram.lines().find(|l| l.trim_start().starts_with("\"a\" [")).unwrap();
        // width at score=90 ≈ 0.75 + 0.9 * 1.05 = 1.695 → formatted as 1.70
        assert!(hot_line.contains("width=1.70"), "hot node width wrong: {}", hot_line);
        assert!(hot_line.contains("#ff6600"), "hot node missing orange border: {}", hot_line);

        // A cold node stays at default width.
        let cold_line = r.diagram.lines().find(|l| l.trim_start().starts_with("\"b\" [")).unwrap();
        assert!(cold_line.contains("width=0.75"), "cold node width wrong: {}", cold_line);
    }

    #[test]
    fn mermaid_marks_hot_nodes_with_class() {
        let mut g = fixture();
        if let Some(n) = g.nodes.iter_mut().find(|n| n.module_id == "a") {
            n.hotspot_score = Some(90.0);
        }
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();

        // `a` should get a class statement including `hot`.
        assert!(
            r.diagram.lines().any(|l| l.trim_start().starts_with("class N") && l.contains("hot")),
            "expected class statement assigning hot:\n{}", r.diagram
        );
    }

    #[test]
    fn cycle_border_takes_precedence_over_hot_border_in_dot() {
        // A node that's both hot and in a cycle wears the cycle red border,
        // not the hot orange border — architectural signal wins.
        let mut g = fixture();
        g.edges.push(edge("c", "a"));
        g.cycles.push(cycle(&["a", "b", "c"], None));
        if let Some(n) = g.nodes.iter_mut().find(|n| n.module_id == "a") {
            n.hotspot_score = Some(95.0);
        }

        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Dot,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();

        let a_line = r.diagram.lines().find(|l| l.trim_start().starts_with("\"a\" [")).unwrap();
        // Expect the cycle red colour, not the hot orange.
        assert!(a_line.contains("color=\"#cc0000\""), "expected cycle red border: {}", a_line);
        assert!(!a_line.contains("color=\"#ff6600\""), "hot border should not win over cycle: {}", a_line);
    }

    fn cochange_pair(a: &str, b: &str, score: f64) -> crate::api::CoChangePair {
        crate::api::CoChangePair {
            file_a: a.into(),
            file_b: b.into(),
            count: 3,
            coupling_score: score,
        }
    }

    #[test]
    fn cochange_overlay_off_by_default() {
        let mut g = fixture();
        g.cochange_pairs.push(cochange_pair("a", "c", 0.9));
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        // No undirected-style edge, no purple link styling.
        assert!(!r.diagram.contains("---|"));
        assert!(!r.diagram.contains("stroke:#84c"));
    }

    #[test]
    fn cochange_overlay_renders_above_threshold_mermaid() {
        let mut g = fixture();
        g.cochange_pairs.push(cochange_pair("a", "c", 0.9));
        g.cochange_pairs.push(cochange_pair("a", "d", 0.2)); // below threshold
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: Some(0.5),
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        // 0.9 pair shows up with the score label; 0.2 pair filtered out.
        assert!(r.diagram.contains("---|0.90|"), "missing cochange line:\n{}", r.diagram);
        assert!(!r.diagram.contains("---|0.20|"));
        // linkStyle appends the purple dash style.
        assert!(r.diagram.contains("stroke:#84c"));
    }

    #[test]
    fn cochange_overlay_renders_above_threshold_dot() {
        let mut g = fixture();
        g.cochange_pairs.push(cochange_pair("a", "c", 0.9));
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Dot,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: Some(0.5),
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        // Purple edge with arrowhead=none and constraint=false so it doesn't
        // warp the DAG layout.
        let line = r.diagram.lines().find(|l| l.contains("\"a\" -> \"c\"") && l.contains("#8844cc")).unwrap();
        assert!(line.contains("arrowhead=none"), "cochange edge must be undirected: {}", line);
        assert!(line.contains("constraint=false"), "cochange edge must not constrain layout: {}", line);
    }

    #[test]
    fn cochange_overlay_skips_pairs_with_excluded_endpoint() {
        // `isolated` is dropped by the selection stage; any cochange pair
        // involving it must not appear as an edge referencing a missing node.
        let mut g = fixture();
        g.cochange_pairs.push(cochange_pair("a", "isolated", 0.9));
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: Some(0.5),
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert!(!r.diagram.contains("---|"), "cochange to excluded node must not render");
    }

    #[test]
    fn blast_radius_selects_epicenter_deps_and_dependents() {
        // fixture: a -> b -> c -> d, plus isolated. Blast radius of `b`:
        // {b} ∪ {c} (dependency) ∪ {a} (dependent) = {a, b, c}.
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: Some("b"),
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert_eq!(r.node_count, 3);
        assert!(r.diagram.contains("a.rs"));
        assert!(r.diagram.contains("b.rs"));
        assert!(r.diagram.contains("c.rs"));
        assert!(!r.diagram.contains("d.rs"));
        // Epicenter class applied to b.
        assert!(
            r.diagram.lines().any(|l| l.trim_start().starts_with("class N") && l.contains("epicenter")),
            "expected epicenter class assignment:\n{}", r.diagram
        );
    }

    #[test]
    fn blast_radius_marks_epicenter_in_dot() {
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Dot,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: Some("b"),
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        // Epicenter node `b` gets the bold red fill; other nodes don't.
        let b_line = r.diagram.lines().find(|l| l.trim_start().starts_with("\"b\" [")).unwrap();
        assert!(b_line.contains("fillcolor=\"#ff3333\""), "epicenter missing red fill: {}", b_line);
        let a_line = r.diagram.lines().find(|l| l.trim_start().starts_with("\"a\" [")).unwrap();
        assert!(!a_line.contains("fillcolor=\"#ff3333\""), "non-epicenter got epicenter fill: {}", a_line);
    }

    #[test]
    fn blast_radius_overrides_focus() {
        // When both are set, blast_radius wins. Fixture: a -> b -> c -> d.
        // With blast_radius=d: {d} ∪ {} ∪ {c} = {d, c}. Focus=a would give
        // {a, b} at depth=1 — verify we get the blast set, not the BFS set.
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: Some("a"),
            depth: 1,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: Some("d"),
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert_eq!(r.node_count, 2);
        assert!(r.diagram.contains("d.rs"));
        assert!(r.diagram.contains("c.rs"));
        assert!(!r.diagram.contains("a.rs"));
        assert!(!r.diagram.contains("b.rs"));
    }

    #[test]
    fn blast_radius_accepts_path_suffix() {
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: Some("b.rs"),
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert_eq!(r.node_count, 3);
    }

    #[test]
    fn blast_radius_unknown_target_errors() {
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: Some("does_not_exist"),
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        });
        assert!(r.is_err());
        let err = r.unwrap_err();
        assert!(err.contains("blast-radius target not found"), "wrong error: {}", err);
    }

    #[test]
    fn basic_constructor_matches_manual_defaults() {
        let opts = RenderOptions::basic(DiagramFormat::Mermaid, 42);
        assert_eq!(opts.format, DiagramFormat::Mermaid);
        assert!(opts.focus.is_none());
        assert_eq!(opts.depth, 2);
        assert_eq!(opts.max_nodes, 42);
        assert!(opts.show_cochange.is_none());
        assert!(opts.blast_radius.is_none());
        assert!(!opts.docs_only);
        assert!(opts.group_by_folder_depth.is_none());
        assert!(!opts.color_by_owner);
    }

    #[test]
    fn overlays_respect_max_nodes_truncation() {
        // Cycle spans a,b,c but max_nodes=2 cuts the graph — the renderer must
        // not reference excluded nodes in linkStyle / class statements.
        let mut g = fixture();
        g.edges.push(edge("c", "a"));
        g.cycles.push(cycle(&["a", "b", "c"], Some("c")));

        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 2,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert!(r.truncated);
        assert_eq!(r.node_count, 2);

        // No linkStyle index should exceed the count of emitted edges.
        let edge_count = r.diagram.lines().filter(|l| {
            l.contains(" --> ") || l.contains(" ==> ") || l.contains(" -.-> ")
        }).count();
        for line in r.diagram.lines().filter(|l| l.trim_start().starts_with("linkStyle")) {
            let idx: usize = line.split_whitespace().nth(1).unwrap().parse().unwrap();
            assert!(idx < edge_count, "linkStyle {} refers to an edge that wasn't emitted", idx);
        }
    }

    // ---------- doc-map (Phase 3.2) ----------------------------------------

    fn doc_node(id: &str, ext: &str) -> GraphNode {
        let mut n = node(id, None);
        n.path = format!("docs/{}.{}", id, ext);
        n.language = "markdown".into();
        n
    }

    fn fixture_with_docs() -> ProjectGraphResponse {
        // a.rs <- README.md (doc → code), config.yaml isolated from code edges,
        // plus b/c/d/isolated from the base fixture. README references a.rs.
        let mut g = fixture();
        g.nodes.push(doc_node("README", "md"));
        g.nodes.push(doc_node("config", "yaml"));
        g.edges.push(edge("README", "a")); // README references code file a
        g
    }

    #[test]
    fn mermaid_doc_node_uses_stadium_shape_and_sec_label() {
        let g = fixture_with_docs();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 20,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        // Stadium shape `([...])` for docs, `sec` unit label, and `doc` classDef.
        assert!(r.diagram.contains("classDef doc"), "missing doc classDef:\n{}", r.diagram);
        assert!(
            r.diagram.lines().any(|l| l.contains("([\"README.md")),
            "doc node missing stadium shape:\n{}", r.diagram
        );
        assert!(r.diagram.contains("sec\"])"), "doc node missing sec unit:\n{}", r.diagram);
        // Non-doc node still uses square bracket + fn.
        assert!(
            r.diagram.lines().any(|l| l.contains("[\"a.rs") && l.contains("fn\"]")),
            "code node shape/unit regressed:\n{}", r.diagram
        );
        // Doc class is applied via a class statement.
        assert!(
            r.diagram.lines().any(|l| l.trim_start().starts_with("class N") && l.contains("doc")),
            "doc class not assigned:\n{}", r.diagram
        );
    }

    #[test]
    fn dot_doc_node_uses_note_shape_and_yellow_fill() {
        let g = fixture_with_docs();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Dot,
            focus: None,
            depth: 2,
            max_nodes: 20,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        let doc_line = r.diagram.lines().find(|l| l.trim_start().starts_with("\"README\" [")).unwrap();
        assert!(doc_line.contains("shape=note"), "doc not shape=note: {}", doc_line);
        assert!(doc_line.contains("#fff4c0"), "doc not yellow fill: {}", doc_line);
        assert!(doc_line.contains("sec\""), "doc missing sec unit: {}", doc_line);

        let code_line = r.diagram.lines().find(|l| l.trim_start().starts_with("\"a\" [")).unwrap();
        assert!(code_line.contains("shape=box"), "code shape regressed: {}", code_line);
        assert!(code_line.contains("fn\""), "code unit regressed: {}", code_line);
    }

    #[test]
    fn docs_only_selects_docs_and_their_neighbors() {
        // README references a.rs. docs_only should yield {README, config, a}:
        // both docs plus the one code neighbor. b/c/d/isolated are excluded.
        let g = fixture_with_docs();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 20,
            show_cochange: None,
            blast_radius: None,
            docs_only: true,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert!(r.diagram.contains("README.md"), "missing README:\n{}", r.diagram);
        assert!(r.diagram.contains("config.yaml"), "missing config:\n{}", r.diagram);
        assert!(r.diagram.contains("a.rs"), "missing referenced code file a.rs:\n{}", r.diagram);
        assert!(!r.diagram.contains("b.rs"), "b.rs should not render in docs_only:\n{}", r.diagram);
        assert!(!r.diagram.contains("c.rs"), "c.rs should not render in docs_only:\n{}", r.diagram);
        assert!(!r.diagram.contains("d.rs"), "d.rs should not render in docs_only:\n{}", r.diagram);
        assert_eq!(r.node_count, 3);
    }

    #[test]
    fn docs_only_blast_radius_wins_over_docs_only() {
        // Selection precedence: blast_radius > focus > docs_only > top. When
        // both blast_radius and docs_only are set, blast_radius selection
        // applies — docs_only is ignored.
        let g = fixture_with_docs();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 20,
            show_cochange: None,
            blast_radius: Some("b"),
            docs_only: true,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        // Blast radius of b in the base edges: {a, b, c}.
        assert!(r.diagram.contains("a.rs"));
        assert!(r.diagram.contains("b.rs"));
        assert!(r.diagram.contains("c.rs"));
        assert!(!r.diagram.contains("README.md"), "docs_only should be overridden:\n{}", r.diagram);
    }

    // ---------- folder-collapsed view (Phase 3.3) --------------------------

    fn node_at(id: &str, path: &str) -> GraphNode {
        let mut n = node(id, None);
        n.path = path.into();
        n
    }

    fn fixture_with_folders() -> ProjectGraphResponse {
        // Layout:
        //   src/api/users.rs  (api_users) → src/db/sql.rs  (db_sql)
        //   src/api/posts.rs  (api_posts) → src/db/sql.rs
        //   src/api/users.rs  → src/api/posts.rs (intra-folder, must be dropped)
        //   tests/foo.rs      (tests_foo) → src/api/users.rs
        // Depth 1 groups: {src, tests}; edges src↔src dropped, src↔tests kept,
        // tests→src kept.
        let mut g = ProjectGraphResponse {
            nodes: vec![
                node_at("api_users", "src/api/users.rs"),
                node_at("api_posts", "src/api/posts.rs"),
                node_at("db_sql", "src/db/sql.rs"),
                node_at("tests_foo", "tests/foo.rs"),
            ],
            edges: vec![
                edge("api_users", "db_sql"),
                edge("api_posts", "db_sql"),
                edge("api_users", "api_posts"),
                edge("tests_foo", "api_users"),
            ],
            cycles: vec![],
            god_modules: vec![],
            layer_violations: vec![],
            metadata: GraphMetadata {
                total_files: 4,
                total_edges: 4,
                languages: HashMap::new(),
                generated_at: "".into(),
                bridge_count: None,
                cycle_count: None,
                god_module_count: None,
                health_score: None,
                layer_violation_count: None,
                architectural_drift: None,
                hotspot_count: None,
                dead_code_count: None,
                unreferenced_exports_count: None,
            },
            cochange_pairs: vec![],
        };
        // Give api_users a hotspot score so we can assert max-folding.
        if let Some(n) = g.nodes.iter_mut().find(|n| n.module_id == "api_users") {
            n.hotspot_score = Some(85.0);
        }
        g
    }

    #[test]
    fn folder_collapse_depth_one_groups_top_level_dirs() {
        let g = fixture_with_folders();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 20,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: Some(1),
            color_by_owner: false,
        }).unwrap();
        // Two folder nodes: `src` and `tests`. The individual files must not
        // appear by filename (only the folder labels do).
        assert_eq!(r.node_count, 2);
        assert!(r.diagram.contains("src/"), "folder label missing:\n{}", r.diagram);
        assert!(r.diagram.contains("tests/"), "folder label missing:\n{}", r.diagram);
        assert!(!r.diagram.contains("users.rs"));
        assert!(!r.diagram.contains("posts.rs"));
        assert!(!r.diagram.contains("sql.rs"));
        // Subroutine shape + folder class applied.
        assert!(r.diagram.contains("classDef folder"));
        assert!(r.diagram.contains("[["), "missing subroutine shape:\n{}", r.diagram);
        assert!(
            r.diagram.lines().any(|l| l.trim_start().starts_with("class N") && l.contains("folder")),
            "folder class not assigned:\n{}", r.diagram
        );
    }

    #[test]
    fn folder_collapse_drops_intra_folder_edges_and_aggregates() {
        let g = fixture_with_folders();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Dot,
            focus: None,
            depth: 2,
            max_nodes: 20,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: Some(1),
            color_by_owner: false,
        }).unwrap();
        // Expected edges after collapse: tests → src (1). The src→src edges
        // from the file graph must be dropped. The src→src count is non-zero
        // in the file graph, but at folder granularity it's a self-loop.
        let edge_count = r.diagram.lines().filter(|l| l.contains(" -> ")).count();
        assert_eq!(edge_count, 1, "expected exactly 1 folder edge:\n{}", r.diagram);
        assert!(r.diagram.contains("\"tests\" -> \"src\""), "expected tests→src:\n{}", r.diagram);
        // Folder shape + fill.
        let src_line = r.diagram.lines().find(|l| l.trim_start().starts_with("\"src\" [")).unwrap();
        assert!(src_line.contains("shape=folder"), "folder shape missing: {}", src_line);
        assert!(src_line.contains("#d6e9ff"), "folder fill missing: {}", src_line);
        // `src` contains 3 files with 3+3+3 = 9 fn.
        assert!(src_line.contains("3 files"), "file count missing: {}", src_line);
        assert!(src_line.contains("9 fn"), "fn sum missing: {}", src_line);
    }

    #[test]
    fn folder_collapse_depth_two_separates_api_from_db() {
        let g = fixture_with_folders();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 20,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: Some(2),
            color_by_owner: false,
        }).unwrap();
        // Groups: src/api, src/db, tests. api→db edges collapse into one.
        // Mermaid labels use the folder *tail*, not the full path, to keep the
        // labels readable — the full folder id remains in the node id.
        assert_eq!(r.node_count, 3);
        assert!(r.diagram.contains("api/"), "api/ label missing:\n{}", r.diagram);
        assert!(r.diagram.contains("db/"), "db/ label missing:\n{}", r.diagram);
        assert!(r.diagram.contains("tests/"), "tests/ label missing:\n{}", r.diagram);
        // api (6 fn from api_users+api_posts) and db (3 fn from db_sql) are separate.
        assert!(r.diagram.contains("2 files, 6 fn"), "api aggregation wrong:\n{}", r.diagram);
        assert!(r.diagram.contains("1 files, 3 fn"), "db aggregation wrong:\n{}", r.diagram);
    }

    // ---------- ownership coloring (Phase 1.6) -----------------------------

    #[test]
    fn owner_color_is_stable_and_within_palette() {
        // Two calls for the same owner must yield the same color.
        let c1 = owner_color("alice");
        let c2 = owner_color("alice");
        assert_eq!(c1, c2);
        // All colors must start with `#` and be 7 chars (hex triple).
        assert_eq!(c1.len(), 7);
        assert!(c1.starts_with('#'));
        // Different owners hash to (likely) different palette entries — at
        // minimum: both are valid palette entries even if they collide.
        let c_bob = owner_color("bob");
        assert_eq!(c_bob.len(), 7);
        assert!(c_bob.starts_with('#'));
    }

    #[test]
    fn mermaid_color_by_owner_emits_style_and_drops_role_class() {
        let mut g = fixture();
        if let Some(n) = g.nodes.iter_mut().find(|n| n.module_id == "a") {
            n.owner = Some("alice".into());
        }
        if let Some(n) = g.nodes.iter_mut().find(|n| n.module_id == "b") {
            n.owner = Some("bob".into());
        }
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: true,
        }).unwrap();
        // Role class suffixes must be dropped so the per-node `style` wins.
        assert!(!r.diagram.contains(":::core"), "role class leaked:\n{}", r.diagram);
        assert!(!r.diagram.contains(":::bridge"), "role class leaked:\n{}", r.diagram);
        // Owner colors are applied via explicit `style` lines.
        let alice = owner_color("alice");
        let bob = owner_color("bob");
        assert!(
            r.diagram.contains(&format!("fill:{}", alice)),
            "alice color missing:\n{}", r.diagram
        );
        assert!(
            r.diagram.contains(&format!("fill:{}", bob)),
            "bob color missing:\n{}", r.diagram
        );
    }

    #[test]
    fn dot_color_by_owner_paints_fillcolor() {
        let mut g = fixture();
        if let Some(n) = g.nodes.iter_mut().find(|n| n.module_id == "a") {
            n.owner = Some("alice".into());
        }
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Dot,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: true,
        }).unwrap();
        let alice = owner_color("alice");
        let a_line = r.diagram.lines().find(|l| l.trim_start().starts_with("\"a\" [")).unwrap();
        assert!(
            a_line.contains(&format!("fillcolor=\"{}\"", alice)),
            "alice fill missing: {}", a_line
        );
        // Nodes without an owner fall back to the default white.
        let b_line = r.diagram.lines().find(|l| l.trim_start().starts_with("\"b\" [")).unwrap();
        assert!(b_line.contains("fillcolor=\"#fff\""), "default fill missing: {}", b_line);
    }

    #[test]
    fn folder_collapse_depth_zero_is_noop() {
        let g = fixture_with_folders();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Mermaid,
            focus: None,
            depth: 2,
            max_nodes: 20,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: Some(0),
            color_by_owner: false,
        }).unwrap();
        // Depth=0 falls back to the uncollapsed graph — every file node renders.
        assert_eq!(r.node_count, 4);
        assert!(r.diagram.contains("users.rs"));
        assert!(r.diagram.contains("sql.rs"));
    }

    #[test]
    fn ascii_format_parses() {
        assert_eq!(DiagramFormat::parse("ascii").unwrap(), DiagramFormat::Ascii);
        assert_eq!(DiagramFormat::parse("tree").unwrap(), DiagramFormat::Ascii);
        assert_eq!(DiagramFormat::parse("TEXT").unwrap(), DiagramFormat::Ascii);
    }

    #[test]
    fn ascii_renders_top_by_degree_as_tree() {
        // Default selection: top-by-degree picks a highly connected root.
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Ascii,
            focus: None,
            depth: 2,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        // Tree characters must show up somewhere below the root line.
        assert!(r.diagram.contains("├── ") || r.diagram.contains("└── "),
            "expected tree glyphs in:\n{}", r.diagram);
        // Every included node should appear at least once.
        assert!(r.diagram.contains("a.rs"));
        assert!(r.diagram.contains("b.rs"));
    }

    #[test]
    fn ascii_rooted_on_focus() {
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Ascii,
            focus: Some("a"),
            depth: 3,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        // First line is the root, un-prefixed.
        let first = r.diagram.lines().next().unwrap();
        assert!(first.starts_with("a.rs"), "root line wrong: {first:?}");
        assert!(!first.starts_with("├") && !first.starts_with("└"));
    }

    #[test]
    fn ascii_breaks_cycles_with_seen_marker() {
        // Build a->b->a cycle so the walker must stop re-entering `a`.
        let mut g = fixture();
        g.edges.push(edge("b", "a"));
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Ascii,
            focus: Some("a"),
            depth: 5,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert!(r.diagram.contains("↑ seen"),
            "expected cycle marker in ascii output:\n{}", r.diagram);
    }

    #[test]
    fn ascii_respects_depth_cap() {
        // a->b->c->d chain; depth=1 from a should reach b but not c.
        let g = fixture();
        let r = render(&g, &RenderOptions {
            format: DiagramFormat::Ascii,
            focus: Some("a"),
            depth: 1,
            max_nodes: 10,
            show_cochange: None,
            blast_radius: None,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        }).unwrap();
        assert!(r.diagram.contains("a.rs"));
        assert!(r.diagram.contains("b.rs"));
        // depth=1 selection via bfs_from_anchor only *includes* a and b, so
        // c.rs must not appear in the ascii tree either.
        assert!(!r.diagram.contains("c.rs"), "depth cap not respected:\n{}", r.diagram);
    }

    #[test]
    fn render_sequence_produces_valid_mermaid() {
        use crate::call_graph::{FileCallGraph, FunctionInfo};
        let cg = FileCallGraph {
            functions: vec![
                FunctionInfo { qualified: "Foo::main".into(), simple: "main".into(), line: 1, kind: "fn" },
                FunctionInfo { qualified: "Foo::helper".into(), simple: "helper".into(), line: 5, kind: "fn" },
            ],
            calls: vec![("Foo::main".into(), "Foo::helper".into())],
            unresolved_count: 2,
            unresolved_calls: vec![],
            language: "rust",
        };
        let r = render_sequence(&cg);
        assert!(r.diagram.starts_with("sequenceDiagram\n"));
        assert!(r.diagram.contains("participant Foo_main as main"));
        assert!(r.diagram.contains("participant Foo_helper as helper"));
        assert!(r.diagram.contains("Foo_main->>Foo_helper: call"));
        assert!(r.diagram.contains("unresolved"));
        assert_eq!(r.node_count, 2);
    }

    #[test]
    fn render_class_produces_valid_mermaid() {
        use crate::class_graph::{ClassGraph, ClassKind, ClassNode, ClassRelationship, FieldDef, MethodDef, Vis};
        let g = ClassGraph {
            classes: vec![
                ClassNode {
                    name: "Point".into(),
                    kind: ClassKind::Struct,
                    fields: vec![
                        FieldDef { name: "x".into(), type_annotation: "f64".into(), visibility: Vis::Public },
                    ],
                    methods: vec![
                        MethodDef { name: "new".into(), params: "x: f64".into(), return_type: "Self".into(),
                            visibility: Vis::Public, is_static: true, is_constructor: false },
                    ],
                },
                ClassNode {
                    name: "Shape".into(),
                    kind: ClassKind::Trait,
                    fields: vec![],
                    methods: vec![
                        MethodDef { name: "area".into(), params: String::new(), return_type: "f64".into(),
                            visibility: Vis::Public, is_static: false, is_constructor: false },
                    ],
                },
            ],
            relationships: vec![
                ClassRelationship::Implements { class: "Point".into(), interface: "Shape".into() },
            ],
            language: "rust",
        };
        let r = render_class(&g);
        assert!(r.diagram.starts_with("classDiagram\n"));
        assert!(r.diagram.contains("class Point {"));
        assert!(r.diagram.contains("+f64 x"));
        assert!(r.diagram.contains("class Shape {"));
        assert!(r.diagram.contains("<<interface>>"));
        assert!(r.diagram.contains("Point ..|> Shape"));
        assert_eq!(r.node_count, 2);
    }
}
