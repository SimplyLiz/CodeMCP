//! Shared diagram renderer for the import graph.
//!
//! Produces Mermaid or Graphviz (DOT) from a `ProjectGraphResponse`, with two
//! node-selection modes:
//!   - No focus → top-N nodes by degree (fallback; "shape of the codebase").
//!   - With focus → BFS from anchor module over import edges up to `depth`
//!     ("shape of the neighborhood I'm editing").
//!
//! Used by both the CLI (`diagram_mode`) and the FFI
//! (`cartographer_render_architecture`) so CLI output and MCP output stay in
//! lock-step.

use std::collections::{HashMap, HashSet, VecDeque};

use crate::api::ProjectGraphResponse;
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
}

impl DiagramFormat {
    pub fn parse(s: &str) -> Result<Self, String> {
        match s.to_lowercase().as_str() {
            "mermaid" | "" => Ok(DiagramFormat::Mermaid),
            "dot" | "graphviz" => Ok(DiagramFormat::Dot),
            other => Err(format!("unknown diagram format: {other}")),
        }
    }
}

/// Rendering options. `focus` is a module_id (or suffix match on a path/module_id).
#[derive(Debug, Clone)]
pub struct RenderOptions<'a> {
    pub format: DiagramFormat,
    pub focus: Option<&'a str>,
    pub depth: usize,
    pub max_nodes: usize,
}

/// Rendered diagram plus a truncation flag so callers can tell the model to
/// tighten `focus` or lower `depth` when the cap kicked in.
#[derive(Debug, Clone)]
pub struct RenderedDiagram {
    pub diagram: String,
    pub truncated: bool,
    pub node_count: usize,
}

/// Precomputed overlays that decorate the base import graph with architectural
/// signals: cycles (from `graph.cycles`), layer violations (from
/// `graph.layer_violations`), and hotspot nodes (from `GraphNode.hotspot_score`).
///
/// We precompute once per `render()` so both Mermaid and DOT rendering paths
/// consult the same sets and stay visually consistent.
struct Overlays<'a> {
    cycle_nodes: HashSet<&'a str>,
    pivot_nodes: HashSet<&'a str>,
    cycle_edges: HashSet<(&'a str, &'a str)>,
    violations: HashMap<(&'a str, &'a str), &'a LayerViolationType>,
}

fn compute_overlays(graph: &ProjectGraphResponse) -> Overlays<'_> {
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

    Overlays { cycle_nodes, pivot_nodes, cycle_edges, violations }
}

/// Render an import-graph diagram. Pure over `graph` — no I/O.
pub fn render(graph: &ProjectGraphResponse, opts: &RenderOptions) -> Result<RenderedDiagram, String> {
    let max_nodes = opts.max_nodes.max(1);

    let (included, truncated) = match opts.focus {
        Some(anchor) => bfs_from_anchor(graph, anchor, opts.depth, max_nodes)?,
        None => top_by_degree(graph, max_nodes),
    };

    let included_set: HashSet<&str> = included.iter().map(|s| s.as_str()).collect();

    // Map module_id -> node for stable lookup during rendering.
    let node_by_id: HashMap<&str, &crate::api::GraphNode> = graph
        .nodes
        .iter()
        .map(|n| (n.module_id.as_str(), n))
        .collect();

    let overlays = compute_overlays(graph);

    let content = match opts.format {
        DiagramFormat::Dot => render_dot(&included, &included_set, &node_by_id, graph, &overlays),
        DiagramFormat::Mermaid => render_mermaid(&included, &included_set, &node_by_id, graph, &overlays),
    };

    Ok(RenderedDiagram { diagram: content, truncated, node_count: included.len() })
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
) -> String {
    let mut out = String::from("digraph cartographer {\n    rankdir=LR;\n");
    for module_id in included {
        let Some(node) = node_by_id.get(module_id.as_str()) else { continue };
        let label = node.path.rsplit('/').next().unwrap_or(&node.path);
        let fill = role_color_dot(node.role.as_deref());

        let mid = module_id.as_str();
        let is_pivot = overlays.pivot_nodes.contains(mid);
        let in_cycle = overlays.cycle_nodes.contains(mid);
        let score = node.hotspot_score.unwrap_or(0.0).clamp(0.0, 100.0);
        let hot = score >= HOTSPOT_THRESHOLD;

        // Border: pivot > cycle > hot > default. Pivot is dashed to distinguish
        // it inside a red-bordered cycle.
        let (border_color, pen_width, extra_style) = if is_pivot {
            ("#cc0000", 3.0, ",dashed")
        } else if in_cycle {
            ("#cc0000", 3.0, "")
        } else if hot {
            ("#ff6600", 3.0, "")
        } else {
            ("#333333", 1.0, "")
        };

        // Hotspot-driven sizing. score ∈ [0,100] → width ∈ [0.75, 1.80],
        // height ∈ [0.50, 0.90], fontsize ∈ [10, 16]. Nodes without a score
        // render at the default size.
        let width = 0.75 + (score / 100.0) * 1.05;
        let height = 0.50 + (score / 100.0) * 0.40;
        let fontsize = 10 + ((score / 100.0) * 6.0) as u32;

        out.push_str(&format!(
            "    \"{}\" [label=\"{}\\n{} fn\" shape=box style=\"filled{}\" fillcolor=\"{}\" color=\"{}\" penwidth={:.1} width={:.2} height={:.2} fontsize={}];\n",
            node.module_id, label, node.signature_count,
            extra_style, fill, border_color, pen_width, width, height, fontsize
        ));
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
    out.push('}');
    out
}

fn render_mermaid(
    included: &[String],
    included_set: &HashSet<&str>,
    node_by_id: &HashMap<&str, &crate::api::GraphNode>,
    graph: &ProjectGraphResponse,
    overlays: &Overlays,
) -> String {
    let mut out = String::from("graph TD\n");
    out.push_str("    classDef bridge fill:#f96,stroke:#333\n");
    out.push_str("    classDef core fill:#9cf,stroke:#333\n");
    out.push_str("    classDef dead fill:#ccc,stroke:#333\n");
    out.push_str("    classDef entry fill:#9f9,stroke:#333\n");
    out.push_str("    classDef cycle stroke:#c00,stroke-width:3px\n");
    out.push_str("    classDef pivot stroke:#c00,stroke-width:3px,stroke-dasharray:5 5\n");
    out.push_str("    classDef hot stroke:#f60,stroke-width:3px\n");

    let id_map: HashMap<&str, usize> = included
        .iter()
        .enumerate()
        .map(|(i, m)| (m.as_str(), i))
        .collect();

    // Node declarations carry the inline role class (:::core / :::bridge / etc).
    // Overlay classes (cycle/pivot/hot) are applied via separate `class` statements
    // below so a node can wear multiple classes without relying on inline chaining.
    for module_id in included {
        let Some(node) = node_by_id.get(module_id.as_str()) else { continue };
        let i = id_map[module_id.as_str()];
        let label = node.path.rsplit('/').next().unwrap_or(&node.path);
        let class_suffix = role_class_suffix(node.role.as_deref());
        out.push_str(&format!(
            "    N{}[\"{}\\n{} fn\"]{}\n",
            i, label, node.signature_count, class_suffix
        ));
    }

    // Overlay class assignments. Pivot takes precedence over cycle so a pivot
    // node gets the dashed border that distinguishes it inside a cycle.
    for module_id in included {
        let Some(node) = node_by_id.get(module_id.as_str()) else { continue };
        let i = id_map[module_id.as_str()];
        let mid = module_id.as_str();
        let mut extras: Vec<&str> = Vec::new();
        if overlays.pivot_nodes.contains(mid) {
            extras.push("pivot");
        } else if overlays.cycle_nodes.contains(mid) {
            extras.push("cycle");
        }
        if node.hotspot_score.unwrap_or(0.0) >= HOTSPOT_THRESHOLD {
            extras.push("hot");
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

    for (idx, style) in link_styles {
        out.push_str(&format!("    linkStyle {} {}\n", idx, style));
    }
    out
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
        }).unwrap();
        assert!(r.diagram.starts_with("digraph cartographer {"));
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
        }).unwrap();
        assert!(r.diagram.starts_with("graph TD\n"));
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
        }).unwrap();

        let a_line = r.diagram.lines().find(|l| l.trim_start().starts_with("\"a\" [")).unwrap();
        // Expect the cycle red colour, not the hot orange.
        assert!(a_line.contains("color=\"#cc0000\""), "expected cycle red border: {}", a_line);
        assert!(!a_line.contains("color=\"#ff6600\""), "hot border should not win over cycle: {}", a_line);
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
}
