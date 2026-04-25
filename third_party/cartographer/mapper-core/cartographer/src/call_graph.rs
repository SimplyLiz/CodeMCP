//! File-local call-graph extraction for Rust and Python.
//!
//! Given one source file, produce (caller, callee) edges between functions
//! defined *in that file*. Calls into other files or the stdlib are dropped
//! and reported as `unresolved_count` — the goal is "what does this file do
//! internally", not project-wide call tracing (that's a much bigger job and
//! would need cross-file resolution).
//!
//! Output is shaped as a `ProjectGraphResponse` so the existing diagram
//! renderers (Mermaid/DOT/ASCII + focus/depth/max_nodes) work unchanged.
//!
//! Gated on `lang-rust` / `lang-python` Cargo features, matching the rest of
//! the tree-sitter surface in `extractor.rs`.

use std::path::Path;

use crate::api::{GraphEdge, GraphMetadata, GraphNode, ProjectGraphResponse};

#[cfg(any(feature = "lang-rust", feature = "lang-python"))]
use tree_sitter::{Node, Parser};

/// Aggregated call graph for a single source file.
#[derive(Debug, Clone)]
pub struct FileCallGraph {
    /// Every function/method defined in the file, in source order.
    pub functions: Vec<FunctionInfo>,
    /// Caller → callee edges, both as qualified names from `functions`.
    pub calls: Vec<(String, String)>,
    /// Number of call sites where the callee could not be matched to a
    /// function defined in this file (external, stdlib, or unresolved).
    pub unresolved_count: usize,
    /// Language tag ("rust" / "python") for the project graph we emit.
    pub language: &'static str,
}

/// One function / method definition in the file being analysed.
#[derive(Debug, Clone)]
pub struct FunctionInfo {
    /// Qualified name: `Type::method` in Rust, `Class.method` in Python, plain
    /// function name at file scope.
    pub qualified: String,
    /// Bare method name — used for simple-name resolution when a callee only
    /// names the method (e.g. `self.foo()`).
    pub simple: String,
    /// 1-based line number of the definition.
    pub line: u32,
    /// "fn" for free functions, "method" for impl/class members.
    pub kind: &'static str,
}

/// Build a call graph for the given file. Returns `Ok(None)` when the file
/// extension isn't one we extract call graphs for (currently `.rs`/`.py`).
/// Returns `Err` on unreadable files or parser init failures.
pub fn build_file_call_graph(path: &Path, source: &str) -> Result<Option<FileCallGraph>, String> {
    let ext = path
        .extension()
        .and_then(|e| e.to_str())
        .map(|s| s.to_lowercase())
        .unwrap_or_default();

    match ext.as_str() {
        #[cfg(feature = "lang-rust")]
        "rs" => Ok(Some(extract_rust(source)?)),
        #[cfg(feature = "lang-python")]
        "py" => Ok(Some(extract_python(source)?)),
        _ => Ok(None),
    }
}

/// Wrap a `FileCallGraph` in a `ProjectGraphResponse` so `diagram::render()`
/// can consume it directly. Each function becomes a node whose `module_id` is
/// the qualified name; each call becomes an edge.
pub fn to_project_graph(cg: &FileCallGraph, path: &Path) -> ProjectGraphResponse {
    let path_str = path.to_string_lossy().into_owned();
    let nodes: Vec<GraphNode> = cg
        .functions
        .iter()
        .map(|f| GraphNode {
            module_id: f.qualified.clone(),
            // Render path shows "file.rs:Type::method" so the diagram carries
            // enough info for a reader to find the function without inspecting
            // module_id separately.
            path: format!("{}:{}", path_str, f.qualified),
            language: cg.language.to_string(),
            // 1 "signature" = 1 function; used for hotspot sizing in DOT.
            signature_count: 1,
            complexity: None,
            is_bridge: None,
            bridge_score: None,
            degree: None,
            risk_level: None,
            churn: None,
            hotspot_score: None,
            role: Some(f.kind.to_string()),
            is_dead: None,
            unreferenced_exports: None,
            fan_in: None,
            fan_out: None,
            cochange_partners: None,
            cochange_entropy: None,
            owner: None,
        })
        .collect();

    let edges: Vec<GraphEdge> = cg
        .calls
        .iter()
        .map(|(src, tgt)| GraphEdge {
            source: src.clone(),
            target: tgt.clone(),
            edge_type: "call".into(),
            at_range: None,
        })
        .collect();

    let mut languages = std::collections::HashMap::new();
    languages.insert(cg.language.to_string(), nodes.len());

    let total_edges = edges.len();
    ProjectGraphResponse {
        nodes,
        edges,
        cycles: vec![],
        god_modules: vec![],
        layer_violations: vec![],
        metadata: GraphMetadata {
            total_files: 1,
            total_edges,
            languages,
            generated_at: String::new(),
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

// ---------------------------------------------------------------------------
// Rust
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-rust")]
fn extract_rust(source: &str) -> Result<FileCallGraph, String> {
    let mut parser = Parser::new();
    let lang = tree_sitter_rust::language();
    parser
        .set_language(&lang)
        .map_err(|e| format!("tree-sitter rust init failed: {e}"))?;
    let tree = parser
        .parse(source.as_bytes(), None)
        .ok_or_else(|| "tree-sitter parse returned None".to_string())?;
    let src = source.as_bytes();

    // Pass 1 — enumerate function definitions with scope stack.
    let mut functions: Vec<FunctionInfo> = Vec::new();
    let mut scope: Vec<String> = Vec::new();
    collect_rust_functions(&tree.root_node(), src, &mut scope, &mut functions);

    // Pass 2 — for each function, walk its body and resolve call sites.
    let resolver = Resolver::new(&functions);
    let mut calls: Vec<(String, String)> = Vec::new();
    let mut unresolved_count: usize = 0;
    let mut scope: Vec<String> = Vec::new();
    collect_rust_calls(
        &tree.root_node(),
        src,
        &mut scope,
        &resolver,
        &mut calls,
        &mut unresolved_count,
    );

    Ok(FileCallGraph {
        functions,
        calls,
        unresolved_count,
        language: "rust",
    })
}

#[cfg(feature = "lang-rust")]
fn collect_rust_functions(
    node: &Node,
    src: &[u8],
    scope: &mut Vec<String>,
    out: &mut Vec<FunctionInfo>,
) {
    match node.kind() {
        "impl_item" => {
            let type_name = node
                .child_by_field_name("type")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            // Drop generics for scope key so `impl Foo<T>` and `impl Foo` match.
            let base = type_name.split('<').next().unwrap_or(&type_name).trim().to_string();
            scope.push(base);
            if let Some(body) = node.child_by_field_name("body") {
                let mut cur = body.walk();
                for child in body.children(&mut cur) {
                    collect_rust_functions(&child, src, scope, out);
                }
            }
            scope.pop();
        }
        "function_item" => {
            let name = node
                .child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() {
                return;
            }
            let kind = if scope.is_empty() { "fn" } else { "method" };
            let qualified = qualify(scope, &name, "::");
            out.push(FunctionInfo {
                qualified,
                simple: name,
                line: (node.start_position().row as u32) + 1,
                kind,
            });
        }
        _ => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                collect_rust_functions(&child, src, scope, out);
            }
        }
    }
}

#[cfg(feature = "lang-rust")]
fn collect_rust_calls(
    node: &Node,
    src: &[u8],
    scope: &mut Vec<String>,
    resolver: &Resolver,
    out: &mut Vec<(String, String)>,
    unresolved: &mut usize,
) {
    match node.kind() {
        "impl_item" => {
            let type_name = node
                .child_by_field_name("type")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            let base = type_name.split('<').next().unwrap_or(&type_name).trim().to_string();
            scope.push(base);
            if let Some(body) = node.child_by_field_name("body") {
                let mut cur = body.walk();
                for child in body.children(&mut cur) {
                    collect_rust_calls(&child, src, scope, resolver, out, unresolved);
                }
            }
            scope.pop();
        }
        "function_item" => {
            let name = node
                .child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() {
                return;
            }
            let caller_qual = qualify(scope, &name, "::");
            if let Some(body) = node.child_by_field_name("body") {
                walk_rust_body(&body, src, &caller_qual, resolver, out, unresolved);
            }
        }
        _ => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                collect_rust_calls(&child, src, scope, resolver, out, unresolved);
            }
        }
    }
}

#[cfg(feature = "lang-rust")]
fn walk_rust_body(
    node: &Node,
    src: &[u8],
    caller: &str,
    resolver: &Resolver,
    out: &mut Vec<(String, String)>,
    unresolved: &mut usize,
) {
    if node.kind() == "call_expression" {
        let callee_raw = node
            .child_by_field_name("function")
            .map(|n| rust_callee_name(&n, src))
            .unwrap_or_default();
        if !callee_raw.is_empty() {
            match resolver.resolve(&callee_raw) {
                Some(target) => {
                    if target != caller {
                        out.push((caller.to_string(), target));
                    }
                }
                None => *unresolved += 1,
            }
        }
    }

    // Recurse into children regardless — nested call expressions, closures, and
    // blocks all legitimately hold more calls.
    let mut cur = node.walk();
    for child in node.children(&mut cur) {
        walk_rust_body(&child, src, caller, resolver, out, unresolved);
    }
}

/// Best-effort callee name extraction from a `call_expression`'s `function`
/// node. Returns the shortest form that a human reader would recognize:
///   foo()              → "foo"
///   mod::foo()         → "foo"
///   x.method()         → "method"
///   Type::assoc()      → "assoc"
/// Macros aren't call_expressions in tree-sitter-rust so we don't see them.
#[cfg(feature = "lang-rust")]
fn rust_callee_name(node: &Node, src: &[u8]) -> String {
    match node.kind() {
        "identifier" => node_text(node, src).to_string(),
        "field_expression" => node
            .child_by_field_name("field")
            .map(|n| node_text(&n, src).to_string())
            .unwrap_or_default(),
        "scoped_identifier" => node
            .child_by_field_name("name")
            .map(|n| node_text(&n, src).to_string())
            .unwrap_or_default(),
        "generic_function" => node
            .child_by_field_name("function")
            .map(|n| rust_callee_name(&n, src))
            .unwrap_or_default(),
        _ => String::new(),
    }
}

// ---------------------------------------------------------------------------
// Python
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-python")]
fn extract_python(source: &str) -> Result<FileCallGraph, String> {
    let mut parser = Parser::new();
    let lang = tree_sitter_python::language();
    parser
        .set_language(&lang)
        .map_err(|e| format!("tree-sitter python init failed: {e}"))?;
    let tree = parser
        .parse(source.as_bytes(), None)
        .ok_or_else(|| "tree-sitter parse returned None".to_string())?;
    let src = source.as_bytes();

    let mut functions: Vec<FunctionInfo> = Vec::new();
    let mut scope: Vec<String> = Vec::new();
    collect_python_functions(&tree.root_node(), src, &mut scope, &mut functions);

    let resolver = Resolver::new(&functions);
    let mut calls: Vec<(String, String)> = Vec::new();
    let mut unresolved_count: usize = 0;
    let mut scope: Vec<String> = Vec::new();
    collect_python_calls(
        &tree.root_node(),
        src,
        &mut scope,
        &resolver,
        &mut calls,
        &mut unresolved_count,
    );

    Ok(FileCallGraph {
        functions,
        calls,
        unresolved_count,
        language: "python",
    })
}

#[cfg(feature = "lang-python")]
fn collect_python_functions(
    node: &Node,
    src: &[u8],
    scope: &mut Vec<String>,
    out: &mut Vec<FunctionInfo>,
) {
    match node.kind() {
        "class_definition" => {
            let class_name = node
                .child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            scope.push(class_name);
            if let Some(body) = node.child_by_field_name("body") {
                let mut cur = body.walk();
                for child in body.children(&mut cur) {
                    collect_python_functions(&child, src, scope, out);
                }
            }
            scope.pop();
        }
        "function_definition" => {
            let name = node
                .child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() {
                return;
            }
            let kind = if scope.is_empty() { "fn" } else { "method" };
            let qualified = qualify(scope, &name, ".");
            out.push(FunctionInfo {
                qualified,
                simple: name,
                line: (node.start_position().row as u32) + 1,
                kind,
            });
        }
        "decorated_definition" => {
            // Decorated functions wrap the real definition in the last child.
            let mut cur = node.walk();
            let children: Vec<Node> = node.children(&mut cur).collect();
            if let Some(def) = children.last() {
                collect_python_functions(def, src, scope, out);
            }
        }
        _ => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                collect_python_functions(&child, src, scope, out);
            }
        }
    }
}

#[cfg(feature = "lang-python")]
fn collect_python_calls(
    node: &Node,
    src: &[u8],
    scope: &mut Vec<String>,
    resolver: &Resolver,
    out: &mut Vec<(String, String)>,
    unresolved: &mut usize,
) {
    match node.kind() {
        "class_definition" => {
            let class_name = node
                .child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            scope.push(class_name);
            if let Some(body) = node.child_by_field_name("body") {
                let mut cur = body.walk();
                for child in body.children(&mut cur) {
                    collect_python_calls(&child, src, scope, resolver, out, unresolved);
                }
            }
            scope.pop();
        }
        "function_definition" => {
            let name = node
                .child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() {
                return;
            }
            let caller_qual = qualify(scope, &name, ".");
            if let Some(body) = node.child_by_field_name("body") {
                walk_python_body(&body, src, &caller_qual, resolver, out, unresolved);
            }
        }
        "decorated_definition" => {
            let mut cur = node.walk();
            let children: Vec<Node> = node.children(&mut cur).collect();
            if let Some(def) = children.last() {
                collect_python_calls(def, src, scope, resolver, out, unresolved);
            }
        }
        _ => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                collect_python_calls(&child, src, scope, resolver, out, unresolved);
            }
        }
    }
}

#[cfg(feature = "lang-python")]
fn walk_python_body(
    node: &Node,
    src: &[u8],
    caller: &str,
    resolver: &Resolver,
    out: &mut Vec<(String, String)>,
    unresolved: &mut usize,
) {
    if node.kind() == "call" {
        let callee_raw = node
            .child_by_field_name("function")
            .map(|n| python_callee_name(&n, src))
            .unwrap_or_default();
        if !callee_raw.is_empty() {
            match resolver.resolve(&callee_raw) {
                Some(target) => {
                    if target != caller {
                        out.push((caller.to_string(), target));
                    }
                }
                None => *unresolved += 1,
            }
        }
    }

    let mut cur = node.walk();
    for child in node.children(&mut cur) {
        walk_python_body(&child, src, caller, resolver, out, unresolved);
    }
}

#[cfg(feature = "lang-python")]
fn python_callee_name(node: &Node, src: &[u8]) -> String {
    match node.kind() {
        "identifier" => node_text(node, src).to_string(),
        "attribute" => node
            .child_by_field_name("attribute")
            .map(|n| node_text(&n, src).to_string())
            .unwrap_or_default(),
        _ => String::new(),
    }
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

#[cfg(any(feature = "lang-rust", feature = "lang-python"))]
fn node_text<'a>(node: &Node, src: &'a [u8]) -> &'a str {
    std::str::from_utf8(&src[node.start_byte()..node.end_byte()]).unwrap_or("")
}

fn qualify(scope: &[String], name: &str, sep: &str) -> String {
    if scope.is_empty() {
        name.to_string()
    } else {
        format!("{}{}{}", scope.join(sep), sep, name)
    }
}

/// Resolves a raw callee token ("foo", "method", "thing") against a known set
/// of locally-defined functions. Rules:
///   - Exact qualified match wins (e.g. "Foo::bar" → "Foo::bar").
///   - Unique simple-name match wins when the raw token is just a bare name.
///   - Otherwise: unresolved.
///
/// We intentionally avoid any fancier disambiguation (type inference, receiver
/// tracking) — that would need the full type system. Unique-simple is enough
/// for the "here's how functions in this file relate" use case.
struct Resolver<'a> {
    by_qualified: std::collections::HashMap<&'a str, &'a str>,
    by_simple: std::collections::HashMap<&'a str, Vec<&'a str>>,
}

impl<'a> Resolver<'a> {
    fn new(functions: &'a [FunctionInfo]) -> Self {
        let mut by_qualified: std::collections::HashMap<&str, &str> =
            std::collections::HashMap::new();
        let mut by_simple: std::collections::HashMap<&str, Vec<&str>> =
            std::collections::HashMap::new();
        for f in functions {
            by_qualified.insert(f.qualified.as_str(), f.qualified.as_str());
            by_simple
                .entry(f.simple.as_str())
                .or_default()
                .push(f.qualified.as_str());
        }
        Resolver { by_qualified, by_simple }
    }

    fn resolve(&self, raw: &str) -> Option<String> {
        if let Some(q) = self.by_qualified.get(raw) {
            return Some((*q).to_string());
        }
        if let Some(list) = self.by_simple.get(raw) {
            if list.len() == 1 {
                return Some(list[0].to_string());
            }
        }
        None
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    #[cfg(feature = "lang-rust")]
    #[test]
    fn rust_resolves_free_function_calls() {
        let src = r#"
fn a() { b(); c(); }
fn b() { c(); }
fn c() {}
"#;
        let cg = build_file_call_graph(&PathBuf::from("x.rs"), src).unwrap().unwrap();
        assert_eq!(cg.functions.len(), 3);
        assert_eq!(cg.language, "rust");
        assert!(cg.calls.contains(&("a".into(), "b".into())));
        assert!(cg.calls.contains(&("a".into(), "c".into())));
        assert!(cg.calls.contains(&("b".into(), "c".into())));
        assert_eq!(cg.unresolved_count, 0);
    }

    #[cfg(feature = "lang-rust")]
    #[test]
    fn rust_method_calls_resolve_via_simple_name() {
        let src = r#"
struct S;
impl S {
    fn foo(&self) { self.bar(); }
    fn bar(&self) {}
}
"#;
        let cg = build_file_call_graph(&PathBuf::from("x.rs"), src).unwrap().unwrap();
        assert!(cg.functions.iter().any(|f| f.qualified == "S::foo"));
        assert!(cg.functions.iter().any(|f| f.qualified == "S::bar"));
        assert!(cg.calls.contains(&("S::foo".into(), "S::bar".into())),
            "missing method call edge in {:?}", cg.calls);
    }

    #[cfg(feature = "lang-rust")]
    #[test]
    fn rust_external_calls_increment_unresolved() {
        let src = r#"
fn a() { println!(); std::mem::swap(&mut 1, &mut 2); unknown(); }
"#;
        let cg = build_file_call_graph(&PathBuf::from("x.rs"), src).unwrap().unwrap();
        // println! is a macro, not a call_expression, so it doesn't count.
        // std::mem::swap and unknown are call_expressions with no local match.
        assert!(cg.unresolved_count >= 2, "expected 2+ unresolved, got {}", cg.unresolved_count);
        assert!(cg.calls.is_empty());
    }

    #[cfg(feature = "lang-rust")]
    #[test]
    fn rust_self_recursion_is_dropped() {
        let src = r#"
fn loop_forever() { loop_forever(); }
"#;
        let cg = build_file_call_graph(&PathBuf::from("x.rs"), src).unwrap().unwrap();
        // A self-edge wouldn't break rendering, but it's never interesting —
        // we drop it so the diagram doesn't loop on a single node.
        assert!(cg.calls.is_empty(), "self-recursion should not emit edges: {:?}", cg.calls);
    }

    #[cfg(feature = "lang-python")]
    #[test]
    fn python_resolves_free_function_calls() {
        let src = "\
def a():
    b()
    c()
def b():
    c()
def c():
    pass
";
        let cg = build_file_call_graph(&PathBuf::from("x.py"), src).unwrap().unwrap();
        assert_eq!(cg.language, "python");
        assert_eq!(cg.functions.len(), 3);
        assert!(cg.calls.contains(&("a".into(), "b".into())));
        assert!(cg.calls.contains(&("a".into(), "c".into())));
        assert!(cg.calls.contains(&("b".into(), "c".into())));
    }

    #[cfg(feature = "lang-python")]
    #[test]
    fn python_method_calls_via_attribute() {
        let src = "\
class S:
    def foo(self):
        self.bar()
    def bar(self):
        pass
";
        let cg = build_file_call_graph(&PathBuf::from("x.py"), src).unwrap().unwrap();
        assert!(cg.functions.iter().any(|f| f.qualified == "S.foo"));
        assert!(cg.functions.iter().any(|f| f.qualified == "S.bar"));
        assert!(
            cg.calls.contains(&("S.foo".into(), "S.bar".into())),
            "missing method edge: {:?}", cg.calls
        );
    }

    #[test]
    fn unknown_extension_returns_none() {
        let cg = build_file_call_graph(&PathBuf::from("x.xyz"), "whatever").unwrap();
        assert!(cg.is_none());
    }

    #[cfg(feature = "lang-rust")]
    #[test]
    fn to_project_graph_emits_node_per_function() {
        let src = r#"
fn a() { b(); }
fn b() {}
"#;
        let cg = build_file_call_graph(&PathBuf::from("x.rs"), src).unwrap().unwrap();
        let pg = to_project_graph(&cg, &PathBuf::from("x.rs"));
        assert_eq!(pg.nodes.len(), 2);
        assert_eq!(pg.edges.len(), 1);
        assert!(pg.nodes.iter().any(|n| n.module_id == "a"));
        assert!(pg.nodes.iter().any(|n| n.module_id == "b"));
        assert_eq!(pg.edges[0].edge_type, "call");
    }
}
