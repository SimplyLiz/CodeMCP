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

    let content = match opts.format {
        DiagramFormat::Dot => render_dot(&included, &included_set, &node_by_id, graph),
        DiagramFormat::Mermaid => render_mermaid(&included, &included_set, &node_by_id, graph),
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
) -> String {
    let mut out = String::from("digraph cartographer {\n    rankdir=LR;\n");
    for module_id in included {
        let Some(node) = node_by_id.get(module_id.as_str()) else { continue };
        let label = node.path.rsplit('/').next().unwrap_or(&node.path);
        let color = role_color_dot(node.role.as_deref());
        out.push_str(&format!(
            "    \"{}\" [label=\"{}\\n{} fn\" shape=box style=filled fillcolor=\"{}\"];\n",
            node.module_id, label, node.signature_count, color
        ));
    }
    for edge in &graph.edges {
        if included_set.contains(edge.source.as_str()) && included_set.contains(edge.target.as_str()) {
            out.push_str(&format!("    \"{}\" -> \"{}\";\n", edge.source, edge.target));
        }
    }
    out.push('}');
    out
}

fn render_mermaid(
    included: &[String],
    included_set: &HashSet<&str>,
    node_by_id: &HashMap<&str, &crate::api::GraphNode>,
    graph: &ProjectGraphResponse,
) -> String {
    let mut out = String::from("graph TD\n");
    out.push_str("    classDef bridge fill:#f96,stroke:#333\n");
    out.push_str("    classDef core fill:#9cf,stroke:#333\n");
    out.push_str("    classDef dead fill:#ccc,stroke:#333\n");
    out.push_str("    classDef entry fill:#9f9,stroke:#333\n");

    let id_map: HashMap<&str, usize> = included
        .iter()
        .enumerate()
        .map(|(i, m)| (m.as_str(), i))
        .collect();

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

    for edge in &graph.edges {
        if included_set.contains(edge.source.as_str()) && included_set.contains(edge.target.as_str()) {
            if let (Some(&si), Some(&ti)) = (
                id_map.get(edge.source.as_str()),
                id_map.get(edge.target.as_str()),
            ) {
                out.push_str(&format!("    N{} --> N{}\n", si, ti));
            }
        }
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
    use crate::api::{GraphEdge, GraphMetadata, GraphNode, ProjectGraphResponse};
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
    }
}
