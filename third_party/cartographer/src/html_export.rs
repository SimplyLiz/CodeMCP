//! Self-contained interactive HTML diagram.
// All items here are used by main.rs (binary); dead_code fires for lib-only analysis.
#![allow(dead_code)]
//!
//! One-file output: no network dependency, no external assets. The graph is
//! serialized into a `<script>` block and a tiny JS app renders a sidebar
//! (filterable node list) plus a detail panel that shows the currently
//! selected node's imports/importers, role, owner, and hotspot score.
//!
//! This is deliberately not a visual force-directed graph. For large
//! codebases, the explorer pattern (searchable list + neighbor lookup) is
//! more useful than a SVG hairball. For visual diagrams, callers write `.svg`
//! or `.png` via `diagram_export` and pipe through `mmdc` or `dot`.

use crate::api::{GraphNode, ProjectGraphResponse};
use crate::layers::LayerViolationType;
use std::collections::{HashMap, HashSet};

/// Build a self-contained HTML string for the given graph. `included` is the
/// set of module_ids to show (respecting the same focus/blast-radius selection
/// the Mermaid/DOT renderers use); pass the full node set for the unfiltered
/// explorer.
pub fn render_html(graph: &ProjectGraphResponse, included: &[String]) -> String {
    let included_set: HashSet<&str> = included.iter().map(|s| s.as_str()).collect();

    let node_by_id: HashMap<&str, &GraphNode> = graph
        .nodes
        .iter()
        .map(|n| (n.module_id.as_str(), n))
        .collect();

    // Precompute violation edges for quick lookup when serializing.
    let mut violation_edges: HashMap<(&str, &str), &LayerViolationType> = HashMap::new();
    for v in &graph.layer_violations {
        violation_edges.insert(
            (v.source_path.as_str(), v.target_path.as_str()),
            &v.violation_type,
        );
    }

    let mut nodes_json = String::from("[");
    let mut first = true;
    for id in included {
        let Some(node) = node_by_id.get(id.as_str()) else { continue };
        if !first {
            nodes_json.push(',');
        }
        first = false;
        let label = node.path.rsplit('/').next().unwrap_or(&node.path);
        nodes_json.push_str(&format!(
            "{{\"id\":{},\"label\":{},\"path\":{},\"role\":{},\"owner\":{},\"hotspot\":{},\"churn\":{},\"fan_in\":{},\"fan_out\":{},\"signatures\":{}}}",
            json_str(&node.module_id),
            json_str(label),
            json_str(&node.path),
            json_opt_str(node.role.as_deref()),
            json_opt_str(node.owner.as_deref()),
            node.hotspot_score.map(|v| format!("{:.0}", v)).unwrap_or_else(|| "null".into()),
            node.churn.map(|v| v.to_string()).unwrap_or_else(|| "null".into()),
            node.fan_in.map(|v| v.to_string()).unwrap_or_else(|| "null".into()),
            node.fan_out.map(|v| v.to_string()).unwrap_or_else(|| "null".into()),
            node.signature_count
        ));
    }
    nodes_json.push(']');

    let mut edges_json = String::from("[");
    let mut first = true;
    for edge in &graph.edges {
        if !(included_set.contains(edge.source.as_str())
            && included_set.contains(edge.target.as_str()))
        {
            continue;
        }
        if !first {
            edges_json.push(',');
        }
        first = false;
        let violation = violation_edges
            .get(&(edge.source.as_str(), edge.target.as_str()))
            .map(|vt| violation_tag(vt))
            .unwrap_or("null");
        edges_json.push_str(&format!(
            "{{\"s\":{},\"t\":{},\"v\":{}}}",
            json_str(&edge.source),
            json_str(&edge.target),
            violation
        ));
    }
    edges_json.push(']');

    // The JS app is intentionally small — vanilla DOM, no framework. Keeping
    // it under ~150 lines means the HTML output stays under 20KB for a
    // thousand-node graph, which prints and attaches cleanly to issues.
    format!(
        r#"<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<title>CodeCartographer — Interactive Diagram</title>
<style>
  :root {{
    --fg: #222; --bg: #fafafa; --panel: #fff; --border: #ddd; --muted: #666;
    --accent: #1f78b4; --hot: #ff6600; --cycle: #cc0000;
  }}
  * {{ box-sizing: border-box; }}
  body {{ margin:0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
         background: var(--bg); color: var(--fg); height: 100vh; display: flex; }}
  #sidebar {{ width: 320px; border-right: 1px solid var(--border); background: var(--panel);
              display:flex; flex-direction:column; }}
  #sidebar header {{ padding: 12px; border-bottom: 1px solid var(--border); }}
  #filter {{ width:100%; padding:6px; border:1px solid var(--border); border-radius:4px;
             font-size: 13px; }}
  #node-list {{ flex:1; overflow-y:auto; }}
  .node-item {{ padding: 6px 12px; border-bottom: 1px solid #f0f0f0; cursor:pointer;
                display:flex; justify-content:space-between; align-items:center; font-size:13px; }}
  .node-item:hover {{ background: #eef; }}
  .node-item.active {{ background: #dde; font-weight:600; }}
  .node-item .role {{ font-size: 10px; color: var(--muted); padding: 1px 5px;
                       border-radius: 3px; background:#eee; }}
  .node-item .role.core {{ background:#9cf; color:#024; }}
  .node-item .role.bridge {{ background:#f96; color:#420; }}
  .node-item .role.entry {{ background:#9f9; color:#042; }}
  .node-item .role.dead {{ background:#ccc; color:#444; }}
  #main {{ flex:1; padding: 20px; overflow-y:auto; }}
  h1 {{ margin: 0 0 4px 0; font-size: 18px; }}
  .path {{ color: var(--muted); font-size: 12px; margin-bottom: 16px; font-family: monospace; }}
  .metrics {{ display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr));
              gap: 8px; margin-bottom: 16px; }}
  .metric {{ background:var(--panel); border:1px solid var(--border); border-radius:6px; padding: 8px; }}
  .metric .v {{ font-size: 20px; font-weight:600; }}
  .metric .k {{ font-size: 11px; color: var(--muted); text-transform: uppercase; letter-spacing: 0.05em; }}
  .metric.hot .v {{ color: var(--hot); }}
  h2 {{ font-size: 14px; color: var(--muted); text-transform: uppercase; letter-spacing: 0.05em;
        margin-top: 20px; margin-bottom: 8px; }}
  .neighbor-list {{ list-style: none; padding: 0; }}
  .neighbor-list li {{ padding: 4px 0; border-bottom: 1px solid #f0f0f0; }}
  .neighbor-list a {{ color: var(--accent); cursor:pointer; text-decoration:none; font-family: monospace; font-size: 13px; }}
  .neighbor-list a:hover {{ text-decoration: underline; }}
  .badge {{ display: inline-block; padding: 1px 6px; border-radius: 3px; font-size: 10px;
            margin-left: 6px; }}
  .badge.BackCall, .badge.CircularCrossLayer {{ background: #fdd; color: var(--cycle); }}
  .badge.SkipCall {{ background: #fed; color: #a60; }}
  .badge.DirectForeignImport {{ background: #ffd; color: #660; }}
  .empty {{ color: var(--muted); font-style: italic; }}
</style>
</head>
<body>
<div id="sidebar">
  <header>
    <div style="font-weight:600; margin-bottom:8px;">CodeCartographer</div>
    <input id="filter" placeholder="Filter nodes…" />
  </header>
  <div id="node-list"></div>
</div>
<div id="main"></div>
<script>
const NODES = {nodes_json};
const EDGES = {edges_json};

// Build adjacency maps once. `imports` = what this file depends on (edge src=this);
// `importers` = who depends on this (edge tgt=this). Violations surface as badges.
const byId = new Map(NODES.map(n => [n.id, n]));
const imports = new Map();     // id -> [{{target, violation}}]
const importers = new Map();   // id -> [{{source, violation}}]
for (const n of NODES) {{ imports.set(n.id, []); importers.set(n.id, []); }}
for (const e of EDGES) {{
  if (imports.has(e.s)) imports.get(e.s).push({{other: e.t, violation: e.v}});
  if (importers.has(e.t)) importers.get(e.t).push({{other: e.s, violation: e.v}});
}}

const listEl = document.getElementById('node-list');
const mainEl = document.getElementById('main');
const filterEl = document.getElementById('filter');

function renderList(query) {{
  const q = (query || '').toLowerCase();
  listEl.innerHTML = '';
  for (const n of NODES) {{
    if (q && !n.label.toLowerCase().includes(q) && !n.path.toLowerCase().includes(q)) continue;
    const item = document.createElement('div');
    item.className = 'node-item' + (n.id === currentId ? ' active' : '');
    item.dataset.id = n.id;
    const left = document.createElement('span');
    left.textContent = n.label;
    const right = document.createElement('span');
    if (n.role) {{
      right.className = 'role ' + n.role;
      right.textContent = n.role;
    }}
    item.appendChild(left);
    item.appendChild(right);
    item.onclick = () => select(n.id);
    listEl.appendChild(item);
  }}
}}

function metric(k, v, extra) {{
  const cls = 'metric' + (extra ? ' ' + extra : '');
  return `<div class="${{cls}}"><div class="v">${{v}}</div><div class="k">${{k}}</div></div>`;
}}

function neighborList(title, entries) {{
  if (!entries.length) {{
    return `<h2>${{title}}</h2><div class="empty">none</div>`;
  }}
  const items = entries.map(entry => {{
    const target = byId.get(entry.other);
    if (!target) return '';
    const badge = entry.violation
      ? `<span class="badge ${{entry.violation}}">${{entry.violation}}</span>`
      : '';
    return `<li><a data-id="${{entry.other}}">${{target.label}}</a>${{badge}}</li>`;
  }}).join('');
  return `<h2>${{title}}</h2><ul class="neighbor-list">${{items}}</ul>`;
}}

let currentId = null;
function select(id) {{
  const n = byId.get(id);
  if (!n) return;
  currentId = id;
  renderList(filterEl.value);

  const metrics = [];
  metrics.push(metric('signatures', n.signatures));
  if (n.fan_in != null) metrics.push(metric('fan-in', n.fan_in));
  if (n.fan_out != null) metrics.push(metric('fan-out', n.fan_out));
  if (n.churn != null) metrics.push(metric('churn', n.churn));
  if (n.hotspot != null) metrics.push(metric('hotspot', n.hotspot, n.hotspot >= 70 ? 'hot' : ''));
  if (n.owner) metrics.push(metric('owner', n.owner));

  mainEl.innerHTML =
    `<h1>${{n.label}}</h1>
     <div class="path">${{n.path}}</div>
     <div class="metrics">${{metrics.join('')}}</div>
     ${{neighborList('Imports', imports.get(n.id) || [])}}
     ${{neighborList('Importers', importers.get(n.id) || [])}}`;

  // Rebind neighbor anchors.
  for (const a of mainEl.querySelectorAll('a[data-id]')) {{
    a.onclick = () => select(a.dataset.id);
  }}
}}

filterEl.oninput = () => renderList(filterEl.value);
renderList('');
if (NODES.length) select(NODES[0].id);
else mainEl.innerHTML = '<div class="empty">No nodes in graph.</div>';
</script>
</body>
</html>
"#,
        nodes_json = nodes_json,
        edges_json = edges_json,
    )
}

fn json_str(s: &str) -> String {
    let mut out = String::with_capacity(s.len() + 2);
    out.push('"');
    for c in s.chars() {
        match c {
            '"' => out.push_str("\\\""),
            '\\' => out.push_str("\\\\"),
            '\n' => out.push_str("\\n"),
            '\r' => out.push_str("\\r"),
            '\t' => out.push_str("\\t"),
            c if (c as u32) < 0x20 => out.push_str(&format!("\\u{:04x}", c as u32)),
            c => out.push(c),
        }
    }
    out.push('"');
    out
}

fn json_opt_str(s: Option<&str>) -> String {
    match s {
        Some(v) => json_str(v),
        None => "null".into(),
    }
}

fn violation_tag(vt: &LayerViolationType) -> &'static str {
    match vt {
        LayerViolationType::BackCall => "\"BackCall\"",
        LayerViolationType::SkipCall => "\"SkipCall\"",
        LayerViolationType::CircularCrossLayer => "\"CircularCrossLayer\"",
        LayerViolationType::DirectForeignImport => "\"DirectForeignImport\"",
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::api::{GraphEdge, GraphMetadata};

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
            churn: Some(5),
            hotspot_score: Some(42.0),
            role: role.map(String::from),
            is_dead: None,
            unreferenced_exports: None,
            fan_in: Some(2),
            fan_out: Some(1),
            cochange_partners: None,
            cochange_entropy: None,
            owner: Some("alice".into()),
        }
    }

    fn fixture() -> ProjectGraphResponse {
        ProjectGraphResponse {
            nodes: vec![
                node("a", Some("core")),
                node("b", None),
                node("c", Some("bridge")),
            ],
            edges: vec![
                GraphEdge {
                    source: "a".into(),
                    target: "b".into(),
                    edge_type: "import".into(),
                    at_range: None,
                },
                GraphEdge {
                    source: "b".into(),
                    target: "c".into(),
                    edge_type: "import".into(),
                    at_range: None,
                },
            ],
            cycles: vec![],
            god_modules: vec![],
            layer_violations: vec![],
            metadata: GraphMetadata {
                total_files: 3,
                total_edges: 2,
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
    fn html_contains_structure_and_embedded_graph() {
        let g = fixture();
        let included: Vec<String> = g.nodes.iter().map(|n| n.module_id.clone()).collect();
        let html = render_html(&g, &included);
        assert!(html.starts_with("<!doctype html>"));
        assert!(html.contains("<script>"));
        assert!(html.contains("const NODES ="));
        assert!(html.contains("const EDGES ="));
        // All three nodes + both edges serialized.
        assert!(html.contains("\"id\":\"a\""));
        assert!(html.contains("\"id\":\"b\""));
        assert!(html.contains("\"id\":\"c\""));
        assert!(html.contains("\"s\":\"a\",\"t\":\"b\""));
        assert!(html.contains("\"s\":\"b\",\"t\":\"c\""));
    }

    #[test]
    fn html_includes_owner_and_hotspot_metadata() {
        let g = fixture();
        let included: Vec<String> = g.nodes.iter().map(|n| n.module_id.clone()).collect();
        let html = render_html(&g, &included);
        assert!(html.contains("\"owner\":\"alice\""));
        assert!(html.contains("\"hotspot\":42"));
        assert!(html.contains("\"churn\":5"));
    }

    #[test]
    fn html_filters_edges_to_included_set() {
        let mut g = fixture();
        // Add a node/edge outside the included selection.
        g.nodes.push(node("d", None));
        g.edges.push(GraphEdge {
            source: "c".into(),
            target: "d".into(),
            edge_type: "import".into(),
            at_range: None,
        });
        let included = vec!["a".into(), "b".into(), "c".into()];
        let html = render_html(&g, &included);
        // d-related edge must be filtered out — only a→b and b→c remain.
        assert!(!html.contains("\"s\":\"c\",\"t\":\"d\""));
        assert!(!html.contains("\"id\":\"d\""));
    }

    #[test]
    fn json_string_escapes_quotes_and_control_chars() {
        assert_eq!(json_str(r#"foo"bar"#), r#""foo\"bar""#);
        assert_eq!(json_str("line1\nline2"), r#""line1\nline2""#);
        assert_eq!(json_str("tab\there"), r#""tab\there""#);
    }
}
