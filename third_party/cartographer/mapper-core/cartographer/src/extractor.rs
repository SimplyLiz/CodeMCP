//! Tree-sitter based skeleton extraction — Tier 2 (confidence = 60).
//!
//! Replaces the regex heuristics in `mapper.rs` for supported languages.
//! Also extracts imports for supported languages, replacing the regex import pass.
//! Falls back gracefully: `None` from `ts_extract` means caller uses the regex path.
//!
//! Each language is an optional Cargo feature:
//!   lang-rust, lang-go, lang-python, lang-typescript, lang-javascript, lang-c, lang-cpp
//!
//! Build without any grammar:   cargo build --no-default-features
//! Single language:             cargo build --no-default-features --features lang-rust

use crate::mapper::{Signature, SymbolKind};
use std::path::Path;

#[cfg(any(
    feature = "lang-rust",   feature = "lang-go",   feature = "lang-python",
    feature = "lang-typescript", feature = "lang-javascript",
    feature = "lang-c",      feature = "lang-cpp",
))]
use tree_sitter::{Language, Node, Parser};

/// Confidence score for tree-sitter extracted symbols (LIP Tier 2).
#[cfg(any(
    feature = "lang-rust",   feature = "lang-go",   feature = "lang-python",
    feature = "lang-typescript", feature = "lang-javascript",
    feature = "lang-c",      feature = "lang-cpp",
))]
const CONFIDENCE_TS: u8 = 60;

// ---------------------------------------------------------------------------
// Public output type
// ---------------------------------------------------------------------------

/// Output of a successful tree-sitter extraction pass.
pub struct TsOutput {
    /// Symbols extracted at Tier 2 confidence.
    pub signatures: Vec<Signature>,
    /// Import paths extracted by tree-sitter. Empty if the language extractor
    /// does not implement import extraction yet — caller keeps the regex imports.
    pub imports: Vec<String>,
}

impl TsOutput {
    #[cfg(any(
        feature = "lang-rust",   feature = "lang-go",   feature = "lang-python",
        feature = "lang-typescript", feature = "lang-javascript",
        feature = "lang-c",      feature = "lang-cpp",
    ))]
    fn new(signatures: Vec<Signature>, imports: Vec<String>) -> Self {
        Self { signatures, imports }
    }
}

// ---------------------------------------------------------------------------
// Public entry point
// ---------------------------------------------------------------------------

/// Attempt tree-sitter extraction for `path` / `source`.
///
/// Returns `Some(TsOutput)` for supported languages, `None` otherwise.
/// When `Some`, `signatures` replace the regex signatures.
/// When `imports` is non-empty, it also replaces the regex imports.
pub fn ts_extract(path: &Path, source: &str) -> Option<TsOutput> {
    let ext = path.extension()?.to_str()?.to_lowercase();
    match ext.as_str() {
        #[cfg(feature = "lang-rust")]
        "rs" => Some(extract_rust(source, path)),

        #[cfg(feature = "lang-go")]
        "go" => Some(extract_go(source, path)),

        #[cfg(feature = "lang-python")]
        "py" => Some(extract_python(source, path)),

        #[cfg(feature = "lang-typescript")]
        "ts" => Some(extract_typescript(source, path, false)),

        #[cfg(feature = "lang-typescript")]
        "tsx" => Some(extract_typescript(source, path, true)),

        #[cfg(feature = "lang-javascript")]
        "js" | "jsx" | "mjs" | "cjs" => Some(extract_javascript(source, path)),

        #[cfg(feature = "lang-c")]
        "c" => Some(extract_c(source, path)),

        // .h: prefer C++ grammar when available, fall back to C
        #[cfg(feature = "lang-cpp")]
        "h" | "hpp" | "cpp" | "cc" | "cxx" => Some(extract_cpp(source, path)),
        #[cfg(all(feature = "lang-c", not(feature = "lang-cpp")))]
        "h" => Some(extract_c(source, path)),

        _ => None,
    }
}

// ---------------------------------------------------------------------------
// Shared helpers — compiled when any grammar is active
// ---------------------------------------------------------------------------

#[cfg(any(
    feature = "lang-rust",   feature = "lang-go",   feature = "lang-python",
    feature = "lang-typescript", feature = "lang-javascript",
    feature = "lang-c",      feature = "lang-cpp",
))]
fn node_text<'a>(node: &Node, src: &'a [u8]) -> &'a str {
    node.utf8_text(src).unwrap_or("")
}

/// Signature text up to (not including) the opening brace / body node.
#[cfg(any(
    feature = "lang-rust",   feature = "lang-go",   feature = "lang-python",
    feature = "lang-typescript", feature = "lang-javascript",
    feature = "lang-c",      feature = "lang-cpp",
))]
fn sig_up_to_block(node: &Node, src: &[u8]) -> String {
    let body_start = {
        let mut cur = node.walk();
        let children: Vec<_> = node.children(&mut cur).collect();
        children.iter()
            .find(|c| matches!(c.kind(),
                "block" | "statement_block" | "compound_statement" |
                "class_body" | "declaration_list" | "field_declaration_list" |
                "enum_body" | "interface_body" | "object_type"
            ))
            .map(|c| c.start_byte())
            .unwrap_or(node.end_byte())
    };
    let raw = std::str::from_utf8(&src[node.start_byte()..body_start]).unwrap_or("");
    let collapsed: String = raw.split_whitespace().collect::<Vec<_>>().join(" ");
    collapsed.trim_end_matches(|c: char| c == '{' || c.is_whitespace()).to_string()
}

/// Python variant: trim up to the colon ending the function/class header.
#[cfg(feature = "lang-python")]
fn sig_up_to_colon(node: &Node, src: &[u8]) -> String {
    let body_start = {
        let mut cur = node.walk();
        let children: Vec<_> = node.children(&mut cur).collect();
        children.iter()
            .find(|c| c.kind() == "block")
            .map(|c| c.start_byte())
            .unwrap_or(node.end_byte())
    };
    let raw = std::str::from_utf8(&src[node.start_byte()..body_start]).unwrap_or("");
    let collapsed: String = raw.split_whitespace().collect::<Vec<_>>().join(" ");
    collapsed.trim_end_matches(|c: char| c == ':' || c.is_whitespace()).to_string()
}

/// First non-body line of a node.
#[cfg(any(
    feature = "lang-rust",   feature = "lang-go",   feature = "lang-python",
    feature = "lang-typescript", feature = "lang-javascript",
    feature = "lang-c",      feature = "lang-cpp",
))]
fn first_line(node: &Node, src: &[u8]) -> String {
    let text = node_text(node, src);
    text.lines().next().unwrap_or("").split_whitespace().collect::<Vec<_>>().join(" ")
}

/// Walk backwards from `node` to collect preceding doc-comment siblings.
#[cfg(any(
    feature = "lang-rust",   feature = "lang-go",   feature = "lang-python",
    feature = "lang-typescript", feature = "lang-javascript",
    feature = "lang-c",      feature = "lang-cpp",
))]
fn preceding_doc_comment(node: &Node, src: &[u8]) -> Option<String> {
    let mut prev = node.prev_sibling()?;
    let mut lines: Vec<String> = Vec::new();
    loop {
        match prev.kind() {
            "line_comment" | "block_comment" | "comment" => {
                lines.push(node_text(&prev, src).to_string());
                match prev.prev_sibling() {
                    Some(p) => prev = p,
                    None => break,
                }
            }
            _ => break,
        }
    }
    if lines.is_empty() {
        None
    } else {
        lines.reverse();
        Some(lines.join("\n"))
    }
}

/// Build a LIP URI: `lip://local/<path>#<qualified>`.
#[cfg(any(
    feature = "lang-rust",   feature = "lang-go",   feature = "lang-python",
    feature = "lang-typescript", feature = "lang-javascript",
    feature = "lang-c",      feature = "lang-cpp",
))]
fn lip_uri(path: &Path, qualified: &str) -> String {
    let p = path.to_string_lossy().replace('\\', "/");
    let p = p.trim_start_matches("./").trim_start_matches('/');
    format!("lip://local/{}#{}", p, qualified)
}

#[cfg(any(
    feature = "lang-rust",   feature = "lang-go",   feature = "lang-python",
    feature = "lang-typescript", feature = "lang-javascript",
    feature = "lang-c",      feature = "lang-cpp",
))]
fn make_sig(
    raw: String, kind: SymbolKind, node: &Node, path: &Path,
    name: &str, qualified: &str, doc: Option<String>,
) -> Signature {
    let sp = node.start_position();
    let ep = node.end_position();
    Signature {
        raw,
        ckb_id: Some(lip_uri(path, qualified)),
        symbol_name: Some(name.to_string()),
        qualified_name: Some(qualified.to_string()),
        kind,
        line_start: sp.row,
        col_start: sp.column,
        line_end: ep.row,
        col_end: ep.column,
        confidence: CONFIDENCE_TS,
        doc_comment: doc,
    }
}

#[cfg(any(
    feature = "lang-rust",   feature = "lang-go",   feature = "lang-python",
    feature = "lang-typescript", feature = "lang-javascript",
    feature = "lang-c",      feature = "lang-cpp",
))]
fn scope_qualify(scope: &[String], name: &str) -> String {
    match scope.last() {
        Some(s) if !s.is_empty() => format!("{}.{}", s, name),
        _ => name.to_string(),
    }
}

// ---------------------------------------------------------------------------
// Rust
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-rust")]
fn extract_rust(source: &str, path: &Path) -> TsOutput {
    let mut parser = Parser::new();
    let lang: Language = tree_sitter_rust::language();
    if parser.set_language(&lang).is_err() { return TsOutput::new(vec![], vec![]); }
    let tree = match parser.parse(source.as_bytes(), None) {
        Some(t) => t,
        None => return TsOutput::new(vec![], vec![]),
    };
    let src = source.as_bytes();
    let root = tree.root_node();
    let mut sigs = Vec::new();
    let mut imports = Vec::new();
    let mut scope: Vec<String> = Vec::new();

    let mut cur = root.walk();
    for child in root.children(&mut cur) {
        if child.kind() == "use_declaration" {
            // Strip "use " prefix and trailing ";"
            let text = node_text(&child, src);
            let imp = text.trim_start_matches("use ").trim_end_matches(';').trim();
            if !imp.is_empty() {
                imports.push(imp.to_string());
            }
        }
    }

    walk_rust(&root, src, path, &mut sigs, &mut scope);
    TsOutput::new(sigs, imports)
}

#[cfg(feature = "lang-rust")]
fn walk_rust(node: &Node, src: &[u8], path: &Path, sigs: &mut Vec<Signature>, scope: &mut Vec<String>) {
    match node.kind() {
        "impl_item" => {
            let type_name = node.child_by_field_name("type")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            let base = type_name.split('<').next().unwrap_or(&type_name).trim().to_string();
            scope.push(base);
            if let Some(body) = node.child_by_field_name("body") {
                let mut cur = body.walk();
                for child in body.children(&mut cur) {
                    walk_rust(&child, src, path, sigs, scope);
                }
            }
            scope.pop();
        }
        "trait_item" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            let raw = sig_up_to_block(node, src);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, SymbolKind::Interface, node, path, &name, &name, doc));
            scope.push(name);
            if let Some(body) = node.child_by_field_name("body") {
                let mut cur = body.walk();
                for child in body.children(&mut cur) {
                    walk_rust(&child, src, path, sigs, scope);
                }
            }
            scope.pop();
        }
        "function_item" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            let vis = {
                let mut cur = node.walk();
                let children: Vec<_> = node.children(&mut cur).collect();
                children.iter()
                    .find(|c| c.kind() == "visibility_modifier")
                    .map(|n| node_text(n, src).to_string())
            };
            let is_pub = vis.as_deref().map(|v| v.contains("pub")).unwrap_or(false);
            if scope.is_empty() && !is_pub { return; }
            let qualified = scope_qualify(scope, &name);
            let kind = if scope.is_empty() { SymbolKind::Function } else { SymbolKind::Method };
            let raw = sig_up_to_block(node, src);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, kind, node, path, &name, &qualified, doc));
        }
        "struct_item" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            let raw = first_line(node, src);
            let doc = preceding_doc_comment(node, src);
            let qualified = scope_qualify(scope, &name);
            sigs.push(make_sig(raw, SymbolKind::Struct, node, path, &name, &qualified, doc));
        }
        "enum_item" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            let raw = first_line(node, src);
            let doc = preceding_doc_comment(node, src);
            let qualified = scope_qualify(scope, &name);
            sigs.push(make_sig(raw, SymbolKind::Enum, node, path, &name, &qualified, doc));
        }
        "type_item" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            let raw = node_text(node, src).split_whitespace().collect::<Vec<_>>().join(" ");
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, SymbolKind::TypeAlias, node, path, &name, &name, doc));
        }
        "const_item" | "static_item" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            let vis = {
                let mut cur = node.walk();
                let children: Vec<_> = node.children(&mut cur).collect();
                children.iter()
                    .find(|c| c.kind() == "visibility_modifier")
                    .map(|n| node_text(n, src).to_string())
            };
            if scope.is_empty() && !vis.as_deref().map(|v| v.contains("pub")).unwrap_or(false) {
                return;
            }
            let raw = node_text(node, src).split_whitespace().collect::<Vec<_>>().join(" ");
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, SymbolKind::Variable, node, path, &name, &name, doc));
        }
        "macro_definition" => {
            let name = {
                let mut cur = node.walk();
                let children: Vec<_> = node.children(&mut cur).collect();
                children.iter()
                    .find(|c| c.kind() == "identifier")
                    .map(|n| node_text(n, src).to_string())
                    .unwrap_or_default()
            };
            if name.is_empty() { return; }
            let raw = format!("macro_rules! {}", name);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, SymbolKind::Macro, node, path, &name, &name, doc));
        }
        "mod_item" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            let doc = preceding_doc_comment(node, src);
            let raw = format!("mod {}", name);
            sigs.push(make_sig(raw, SymbolKind::Namespace, node, path, &name, &name, doc));
            if let Some(body) = node.child_by_field_name("body") {
                scope.push(name);
                let mut cur = body.walk();
                for child in body.children(&mut cur) {
                    walk_rust(&child, src, path, sigs, scope);
                }
                scope.pop();
            }
        }
        _ => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                walk_rust(&child, src, path, sigs, scope);
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Go
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-go")]
fn extract_go(source: &str, path: &Path) -> TsOutput {
    let mut parser = Parser::new();
    let lang: Language = tree_sitter_go::language();
    if parser.set_language(&lang).is_err() { return TsOutput::new(vec![], vec![]); }
    let tree = match parser.parse(source.as_bytes(), None) {
        Some(t) => t,
        None => return TsOutput::new(vec![], vec![]),
    };
    let src = source.as_bytes();
    let root = tree.root_node();
    let mut sigs = Vec::new();
    let mut imports = Vec::new();

    // Extract import paths from import_declaration nodes
    let mut cur = root.walk();
    for child in root.children(&mut cur) {
        if child.kind() == "import_declaration" {
            let mut c2 = child.walk();
            for spec in child.children(&mut c2) {
                if spec.kind() == "import_spec" || spec.kind() == "import_spec_list" {
                    collect_go_import_specs(&spec, src, &mut imports);
                }
            }
        }
    }

    walk_go(&root, src, path, &mut sigs);
    TsOutput::new(sigs, imports)
}

#[cfg(feature = "lang-go")]
fn collect_go_import_specs(node: &Node, src: &[u8], imports: &mut Vec<String>) {
    match node.kind() {
        "import_spec" => {
            if let Some(path_node) = node.child_by_field_name("path") {
                let raw = node_text(&path_node, src);
                let clean = raw.trim_matches('"');
                if !clean.is_empty() {
                    imports.push(clean.to_string());
                }
            }
        }
        "import_spec_list" => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                collect_go_import_specs(&child, src, imports);
            }
        }
        _ => {}
    }
}

#[cfg(feature = "lang-go")]
fn walk_go(node: &Node, src: &[u8], path: &Path, sigs: &mut Vec<Signature>) {
    match node.kind() {
        "function_declaration" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            let raw = sig_up_to_block(node, src);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, SymbolKind::Function, node, path, &name, &name, doc));
        }
        "method_declaration" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            let receiver_type = node.child_by_field_name("receiver")
                .and_then(|r| {
                    let mut cur = r.walk();
                    let children: Vec<_> = r.children(&mut cur).collect();
                    children.iter()
                        .find(|c| matches!(c.kind(), "parameter_declaration" | "variadic_parameter_declaration"))
                        .and_then(|p| p.child_by_field_name("type"))
                        .map(|t| {
                            node_text(&t, src)
                                .trim_start_matches('*')
                                .split('<').next().unwrap_or("")
                                .trim().to_string()
                        })
                })
                .unwrap_or_default();
            let qualified = if receiver_type.is_empty() { name.clone() } else { format!("{}.{}", receiver_type, name) };
            let raw = sig_up_to_block(node, src);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, SymbolKind::Method, node, path, &name, &qualified, doc));
        }
        "type_declaration" => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                if child.kind() == "type_spec" {
                    let name = child.child_by_field_name("name")
                        .map(|n| node_text(&n, src).to_string())
                        .unwrap_or_default();
                    if name.is_empty() { continue; }
                    let kind = match child.child_by_field_name("type").as_ref().map(|n| n.kind()) {
                        Some("struct_type")    => SymbolKind::Struct,
                        Some("interface_type") => SymbolKind::Interface,
                        _                      => SymbolKind::TypeAlias,
                    };
                    let raw = first_line(&child, src);
                    let doc = preceding_doc_comment(&child, src);
                    sigs.push(make_sig(raw, kind, &child, path, &name, &name, doc));
                }
            }
        }
        "const_declaration" | "var_declaration" => {
            let mut cur = node.walk();
            let top_children: Vec<_> = node.children(&mut cur).collect();
            for child in top_children {
                if matches!(child.kind(), "const_spec" | "var_spec") {
                    let name = child.child_by_field_name("name")
                        .or_else(|| {
                            let mut c = child.walk();
                            let cc: Vec<_> = child.children(&mut c).collect();
                            cc.into_iter().find(|n| n.kind() == "identifier")
                        })
                        .map(|n| node_text(&n, src).to_string())
                        .unwrap_or_default();
                    if name.is_empty() { continue; }
                    let raw = node_text(&child, src).split_whitespace().collect::<Vec<_>>().join(" ");
                    let doc = preceding_doc_comment(&child, src);
                    sigs.push(make_sig(raw, SymbolKind::Variable, &child, path, &name, &name, doc));
                }
            }
        }
        _ => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                walk_go(&child, src, path, sigs);
            }
        }
    }
}

// ---------------------------------------------------------------------------
// Python
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-python")]
fn extract_python(source: &str, path: &Path) -> TsOutput {
    let mut parser = Parser::new();
    let lang: Language = tree_sitter_python::language();
    if parser.set_language(&lang).is_err() { return TsOutput::new(vec![], vec![]); }
    let tree = match parser.parse(source.as_bytes(), None) {
        Some(t) => t,
        None => return TsOutput::new(vec![], vec![]),
    };
    let src = source.as_bytes();
    let root = tree.root_node();
    let mut sigs = Vec::new();
    let mut imports = Vec::new();
    let mut scope: Vec<String> = Vec::new();

    // Extract imports at root level
    let mut cur = root.walk();
    for child in root.children(&mut cur) {
        match child.kind() {
            "import_statement" => {
                // import os, import os.path
                let mut c2 = child.walk();
                for n in child.children(&mut c2) {
                    if matches!(n.kind(), "dotted_name" | "aliased_import") {
                        let name = n.child_by_field_name("name")
                            .map(|x| node_text(&x, src))
                            .unwrap_or_else(|| node_text(&n, src));
                        imports.push(name.to_string());
                    }
                }
            }
            "import_from_statement" => {
                // from os import path  /  from . import foo
                if let Some(module) = child.child_by_field_name("module_name") {
                    imports.push(node_text(&module, src).to_string());
                }
            }
            _ => {}
        }
    }

    walk_python(&root, src, path, &mut sigs, &mut scope);
    TsOutput::new(sigs, imports)
}

#[cfg(feature = "lang-python")]
fn walk_python(node: &Node, src: &[u8], path: &Path, sigs: &mut Vec<Signature>, scope: &mut Vec<String>) {
    match node.kind() {
        "function_definition" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            if name.starts_with('_') && !name.starts_with("__") && scope.is_empty() { return; }
            let qualified = scope_qualify(scope, &name);
            let kind = if scope.is_empty() { SymbolKind::Function } else { SymbolKind::Method };
            let raw = sig_up_to_colon(node, src);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, kind, node, path, &name, &qualified, doc));
        }
        "class_definition" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            let raw = sig_up_to_colon(node, src);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, SymbolKind::Class, node, path, &name, &name, doc));
            scope.push(name);
            if let Some(body) = node.child_by_field_name("body") {
                let mut cur = body.walk();
                for child in body.children(&mut cur) {
                    walk_python(&child, src, path, sigs, scope);
                }
            }
            scope.pop();
        }
        "decorated_definition" => {
            let mut cur = node.walk();
            let children: Vec<Node> = node.children(&mut cur).collect();
            if let Some(def) = children.last() {
                walk_python(def, src, path, sigs, scope);
            }
        }
        "assignment" => {
            if scope.is_empty() {
                let name = node.child_by_field_name("left")
                    .map(|n| node_text(&n, src).to_string())
                    .unwrap_or_default();
                if !name.is_empty() && name.chars().all(|c| c.is_uppercase() || c == '_' || c.is_numeric()) {
                    let raw = first_line(node, src);
                    sigs.push(make_sig(raw, SymbolKind::Variable, node, path, &name, &name, None));
                }
            }
        }
        _ => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                walk_python(&child, src, path, sigs, scope);
            }
        }
    }
}

// ---------------------------------------------------------------------------
// TypeScript / TSX
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-typescript")]
fn extract_typescript(source: &str, path: &Path, is_tsx: bool) -> TsOutput {
    let mut parser = Parser::new();
    let lang: Language = if is_tsx {
        tree_sitter_typescript::language_tsx()
    } else {
        tree_sitter_typescript::language_typescript()
    };
    if parser.set_language(&lang).is_err() { return TsOutput::new(vec![], vec![]); }
    let tree = match parser.parse(source.as_bytes(), None) {
        Some(t) => t,
        None => return TsOutput::new(vec![], vec![]),
    };
    let src = source.as_bytes();
    let root = tree.root_node();
    let mut sigs = Vec::new();
    let imports = collect_js_ts_imports(&root, src);
    let mut scope: Vec<String> = Vec::new();
    walk_ts(&root, src, path, &mut sigs, &mut scope);
    TsOutput::new(sigs, imports)
}

// ---------------------------------------------------------------------------
// JavaScript (JSX / MJS / CJS)
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-javascript")]
fn extract_javascript(source: &str, path: &Path) -> TsOutput {
    let mut parser = Parser::new();
    let lang: Language = tree_sitter_javascript::language();
    if parser.set_language(&lang).is_err() { return TsOutput::new(vec![], vec![]); }
    let tree = match parser.parse(source.as_bytes(), None) {
        Some(t) => t,
        None => return TsOutput::new(vec![], vec![]),
    };
    let src = source.as_bytes();
    let root = tree.root_node();
    let mut sigs = Vec::new();
    let imports = collect_js_ts_imports(&root, src);
    let mut scope: Vec<String> = Vec::new();
    walk_ts(&root, src, path, &mut sigs, &mut scope);
    TsOutput::new(sigs, imports)
}

/// Collect import source strings from JS/TS `import_statement` nodes.
#[cfg(any(feature = "lang-typescript", feature = "lang-javascript"))]
fn collect_js_ts_imports(root: &Node, src: &[u8]) -> Vec<String> {
    let mut imports = Vec::new();
    let mut cur = root.walk();
    for child in root.children(&mut cur) {
        if child.kind() == "import_statement" {
            if let Some(source_node) = child.child_by_field_name("source") {
                let raw = node_text(&source_node, src);
                let clean = raw.trim_matches('"').trim_matches('\'');
                if !clean.is_empty() {
                    imports.push(clean.to_string());
                }
            }
        }
    }
    imports
}

/// Shared walker for TypeScript and JavaScript (both grammars produce compatible node kinds).
#[cfg(any(feature = "lang-typescript", feature = "lang-javascript"))]
fn walk_ts(node: &Node, src: &[u8], path: &Path, sigs: &mut Vec<Signature>, scope: &mut Vec<String>) {
    match node.kind() {
        "function_declaration" | "function" | "generator_function_declaration" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            let qualified = scope_qualify(scope, &name);
            let kind = if scope.is_empty() { SymbolKind::Function } else { SymbolKind::Method };
            let raw = sig_up_to_block(node, src);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, kind, node, path, &name, &qualified, doc));
        }
        "class_declaration" | "abstract_class_declaration" | "class" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            let raw = sig_up_to_block(node, src);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, SymbolKind::Class, node, path, &name, &name, doc));
            scope.push(name);
            if let Some(body) = node.child_by_field_name("body") {
                let mut cur = body.walk();
                for child in body.children(&mut cur) {
                    walk_ts(&child, src, path, sigs, scope);
                }
            }
            scope.pop();
        }
        "method_definition" | "method_signature" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() || name == "constructor" { return; }
            if name.starts_with('#') || name.starts_with('[') { return; }
            let qualified = scope_qualify(scope, &name);
            let raw = sig_up_to_block(node, src);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, SymbolKind::Method, node, path, &name, &qualified, doc));
        }
        "interface_declaration" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            let raw = sig_up_to_block(node, src);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, SymbolKind::Interface, node, path, &name, &name, doc));
        }
        "type_alias_declaration" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            let raw = first_line(node, src);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, SymbolKind::TypeAlias, node, path, &name, &name, doc));
        }
        "enum_declaration" => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if name.is_empty() { return; }
            let raw = first_line(node, src);
            let doc = preceding_doc_comment(node, src);
            sigs.push(make_sig(raw, SymbolKind::Enum, node, path, &name, &name, doc));
        }
        "export_statement" | "export_clause" => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                if !matches!(child.kind(), "export" | "default" | "from" | "string" | ";" | "*" | "as") {
                    walk_ts(&child, src, path, sigs, scope);
                }
            }
        }
        "lexical_declaration" | "variable_declaration" => {
            let mut cur = node.walk();
            for decl in node.children(&mut cur) {
                if decl.kind() != "variable_declarator" { continue; }
                let name = decl.child_by_field_name("name")
                    .map(|n| node_text(&n, src).to_string())
                    .unwrap_or_default();
                if name.is_empty() { continue; }
                let value = decl.child_by_field_name("value");
                let is_fn = value.as_ref().map(|v| {
                    matches!(v.kind(), "arrow_function" | "function" | "function_expression" | "generator_function")
                }).unwrap_or(false);
                if !is_fn { continue; }
                let val = value.unwrap();
                let raw = format!("const {} = {}", name, sig_up_to_block(&val, src));
                let qualified = scope_qualify(scope, &name);
                let doc = preceding_doc_comment(node, src);
                sigs.push(make_sig(raw, SymbolKind::Function, &decl, path, &name, &qualified, doc));
            }
        }
        _ => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                walk_ts(&child, src, path, sigs, scope);
            }
        }
    }
}

// ---------------------------------------------------------------------------
// C
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-c")]
fn extract_c(source: &str, path: &Path) -> TsOutput {
    let mut parser = Parser::new();
    let lang: Language = tree_sitter_c::language();
    if parser.set_language(&lang).is_err() { return TsOutput::new(vec![], vec![]); }
    let tree = match parser.parse(source.as_bytes(), None) {
        Some(t) => t,
        None => return TsOutput::new(vec![], vec![]),
    };
    let src = source.as_bytes();
    let root = tree.root_node();
    let mut sigs = Vec::new();
    let mut imports = Vec::new();
    let mut scope: Vec<String> = Vec::new();
    walk_c_cpp(&root, src, path, &mut sigs, &mut imports, &mut scope, false);
    TsOutput::new(sigs, imports)
}

// ---------------------------------------------------------------------------
// C++
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-cpp")]
fn extract_cpp(source: &str, path: &Path) -> TsOutput {
    let mut parser = Parser::new();
    let lang: Language = tree_sitter_cpp::language();
    if parser.set_language(&lang).is_err() { return TsOutput::new(vec![], vec![]); }
    let tree = match parser.parse(source.as_bytes(), None) {
        Some(t) => t,
        None => return TsOutput::new(vec![], vec![]),
    };
    let src = source.as_bytes();
    let root = tree.root_node();
    let mut sigs = Vec::new();
    let mut imports = Vec::new();
    let mut scope: Vec<String> = Vec::new();
    walk_c_cpp(&root, src, path, &mut sigs, &mut imports, &mut scope, true);
    TsOutput::new(sigs, imports)
}

/// Shared walker for C and C++ (C++ grammar is a superset).
/// `is_cpp` gates C++-specific node kinds (class, namespace, template, etc.).
#[cfg(any(feature = "lang-c", feature = "lang-cpp"))]
fn walk_c_cpp(
    node: &Node, src: &[u8], path: &Path,
    sigs: &mut Vec<Signature>, imports: &mut Vec<String>,
    scope: &mut Vec<String>, is_cpp: bool,
) {
    match node.kind() {
        // #include → import
        "preproc_include" => {
            let path_text = node.child_by_field_name("path")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if !path_text.is_empty() {
                imports.push(path_text);
            }
            return;
        }

        // Function definition: int foo(int x) { ... }
        "function_definition" => {
            if let Some(decl) = node.child_by_field_name("declarator") {
                let (name, qualified) = c_declarator_names(&decl, src, scope);
                if !name.is_empty() {
                    let raw = sig_up_to_block(node, src);
                    let doc = preceding_doc_comment(node, src);
                    let kind = if scope.is_empty() { SymbolKind::Function } else { SymbolKind::Method };
                    sigs.push(make_sig(raw, kind, node, path, &name, &qualified, doc));
                }
            }
            // Don't recurse into the body — we don't want nested functions
            return;
        }

        // Function declaration (prototype): int foo(int x);
        "declaration" => {
            if let Some(decl) = node.child_by_field_name("declarator") {
                if is_function_declarator(&decl) {
                    let (name, qualified) = c_declarator_names(&decl, src, scope);
                    if !name.is_empty() {
                        let raw = node_text(node, src).split_whitespace().collect::<Vec<_>>().join(" ");
                        let doc = preceding_doc_comment(node, src);
                        sigs.push(make_sig(raw, SymbolKind::Function, node, path, &name, &qualified, doc));
                    }
                }
            }
            return;
        }

        // struct Foo { ... } / union Foo { ... }
        "struct_specifier" | "union_specifier" => {
            if let Some(name_node) = node.child_by_field_name("name") {
                let name = node_text(&name_node, src).to_string();
                if !name.is_empty() {
                    let raw = first_line(node, src);
                    let doc = preceding_doc_comment(node, src);
                    let qualified = scope_qualify(scope, &name);
                    sigs.push(make_sig(raw, SymbolKind::Struct, node, path, &name, &qualified, doc));
                }
            }
            // Still walk the body for nested types
        }

        // enum Foo { ... }
        "enum_specifier" => {
            if let Some(name_node) = node.child_by_field_name("name") {
                let name = node_text(&name_node, src).to_string();
                if !name.is_empty() {
                    let raw = first_line(node, src);
                    let doc = preceding_doc_comment(node, src);
                    let qualified = scope_qualify(scope, &name);
                    sigs.push(make_sig(raw, SymbolKind::Enum, node, path, &name, &qualified, doc));
                }
            }
            return;
        }

        // typedef ... Foo;
        "type_definition" => {
            // The alias name is in the `declarator` field (a type_identifier).
            // Fall back to scanning children if the field isn't set (some grammar versions vary).
            let name = node.child_by_field_name("declarator")
                .map(|n| node_text(&n, src).to_string())
                .filter(|s| !s.is_empty())
                .or_else(|| {
                    let mut cur = node.walk();
                    let children: Vec<_> = node.children(&mut cur).collect();
                    children.iter().rev()
                        .find(|c| c.kind() == "type_identifier")
                        .map(|n| node_text(n, src).to_string())
                })
                .unwrap_or_default();
            if !name.is_empty() {
                let raw = node_text(node, src).split_whitespace().collect::<Vec<_>>().join(" ");
                let doc = preceding_doc_comment(node, src);
                sigs.push(make_sig(raw, SymbolKind::TypeAlias, node, path, &name, &name, doc));
            }
            return;
        }

        // #define FOO value / #define FOO(x) expr
        "preproc_def" => {
            if let Some(name_node) = node.child_by_field_name("name") {
                let name = node_text(&name_node, src).to_string();
                if !name.is_empty() {
                    let raw = first_line(node, src);
                    sigs.push(make_sig(raw, SymbolKind::Variable, node, path, &name, &name, None));
                }
            }
            return;
        }
        "preproc_function_def" => {
            if let Some(name_node) = node.child_by_field_name("name") {
                let name = node_text(&name_node, src).to_string();
                if !name.is_empty() {
                    let raw = first_line(node, src);
                    sigs.push(make_sig(raw, SymbolKind::Macro, node, path, &name, &name, None));
                }
            }
            return;
        }

        // C++ only -------------------------------------------------------
        // class Foo { ... }
        "class_specifier" if is_cpp => {
            if let Some(name_node) = node.child_by_field_name("name") {
                let name = node_text(&name_node, src).to_string();
                if !name.is_empty() {
                    let raw = first_line(node, src);
                    let doc = preceding_doc_comment(node, src);
                    let qualified = scope_qualify(scope, &name);
                    sigs.push(make_sig(raw, SymbolKind::Class, node, path, &name, &qualified, doc));
                    // Walk class body for inline method definitions
                    if let Some(body) = node.child_by_field_name("body") {
                        scope.push(name);
                        let mut cur = body.walk();
                        for child in body.children(&mut cur) {
                            walk_c_cpp(&child, src, path, sigs, imports, scope, is_cpp);
                        }
                        scope.pop();
                    }
                    return;
                }
            }
        }

        // namespace foo { ... }
        "namespace_definition" if is_cpp => {
            let name = node.child_by_field_name("name")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            if !name.is_empty() {
                let doc = preceding_doc_comment(node, src);
                sigs.push(make_sig(
                    format!("namespace {}", name), SymbolKind::Namespace,
                    node, path, &name, &name, doc,
                ));
                scope.push(name);
            }
            if let Some(body) = node.child_by_field_name("body") {
                let mut cur = body.walk();
                for child in body.children(&mut cur) {
                    walk_c_cpp(&child, src, path, sigs, imports, scope, is_cpp);
                }
            }
            if !node.child_by_field_name("name").map(|n| node_text(&n, src)).unwrap_or("").is_empty() {
                scope.pop();
            }
            return;
        }

        // template<...> class/function
        "template_declaration" if is_cpp => {
            // Recurse into the inner declaration
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                if !matches!(child.kind(), "template_parameters" | "template") {
                    walk_c_cpp(&child, src, path, sigs, imports, scope, is_cpp);
                }
            }
            return;
        }

        // extern "C" { ... }
        "linkage_specification" if is_cpp => {
            if let Some(body) = node.child_by_field_name("body") {
                let mut cur = body.walk();
                for child in body.children(&mut cur) {
                    walk_c_cpp(&child, src, path, sigs, imports, scope, is_cpp);
                }
            }
            return;
        }

        _ => {}
    }

    // Default: recurse
    let mut cur = node.walk();
    for child in node.children(&mut cur) {
        walk_c_cpp(&child, src, path, sigs, imports, scope, is_cpp);
    }
}

/// Recursively find the declared name inside a C/C++ declarator chain.
/// Returns `(simple_name, qualified_name)`.
#[cfg(any(feature = "lang-c", feature = "lang-cpp"))]
fn c_declarator_names(node: &Node, src: &[u8], scope: &[String]) -> (String, String) {
    let name = find_c_name(node, src).unwrap_or_default();
    // For C++: if the declarator is (or contains) a qualified_identifier,
    // use its scope as the qualified prefix instead of the current scope stack.
    let qualified = if let Some(q) = find_qualified_c_name(node, src) {
        q
    } else {
        scope_qualify(scope, &name)
    };
    (name, qualified)
}

/// Walk a declarator node recursively to find the innermost identifier.
#[cfg(any(feature = "lang-c", feature = "lang-cpp"))]
fn find_c_name(node: &Node, src: &[u8]) -> Option<String> {
    match node.kind() {
        "identifier" | "field_identifier" | "type_identifier" => {
            Some(node_text(node, src).to_string())
        }
        "destructor_name" | "operator_name" => {
            Some(node_text(node, src).to_string())
        }
        "qualified_identifier" => {
            // Foo::bar → name is the last component
            node.child_by_field_name("name")
                .as_ref()
                .and_then(|n| find_c_name(n, src))
        }
        "function_declarator" | "pointer_declarator" | "reference_declarator" |
        "array_declarator" | "abstract_function_declarator" => {
            node.child_by_field_name("declarator")
                .as_ref()
                .and_then(|n| find_c_name(n, src))
        }
        _ => None,
    }
}

/// If the declarator contains a `qualified_identifier`, return `"Scope.name"`.
#[cfg(any(feature = "lang-c", feature = "lang-cpp"))]
fn find_qualified_c_name(node: &Node, src: &[u8]) -> Option<String> {
    match node.kind() {
        "qualified_identifier" => {
            let scope_part = node.child_by_field_name("scope")
                .map(|n| node_text(&n, src).to_string())
                .unwrap_or_default();
            let name_part = node.child_by_field_name("name")
                .as_ref()
                .and_then(|n| find_c_name(n, src))
                .unwrap_or_default();
            if scope_part.is_empty() || name_part.is_empty() {
                None
            } else {
                Some(format!("{}.{}", scope_part, name_part))
            }
        }
        "function_declarator" | "pointer_declarator" | "reference_declarator" => {
            node.child_by_field_name("declarator")
                .as_ref()
                .and_then(|n| find_qualified_c_name(n, src))
        }
        _ => None,
    }
}

/// True if this declarator node (or any nested declarator) is a function_declarator.
#[cfg(any(feature = "lang-c", feature = "lang-cpp"))]
fn is_function_declarator(node: &Node) -> bool {
    match node.kind() {
        "function_declarator" => true,
        "pointer_declarator" | "reference_declarator" | "array_declarator" => {
            node.child_by_field_name("declarator")
                .as_ref()
                .map(|n| is_function_declarator(n))
                .unwrap_or(false)
        }
        _ => false,
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::Path;

    // --- Rust ---------------------------------------------------------------

    #[cfg(feature = "lang-rust")]
    #[test]
    fn rust_pub_function() {
        let src = r#"pub fn greet(name: &str) -> String { format!("Hi {}", name) }"#;
        let out = ts_extract(Path::new("lib.rs"), src).unwrap();
        assert_eq!(out.signatures.len(), 1);
        let sig = &out.signatures[0];
        assert_eq!(sig.symbol_name.as_deref(), Some("greet"));
        assert_eq!(sig.kind, SymbolKind::Function);
        assert_eq!(sig.confidence, 60);
    }

    #[cfg(feature = "lang-rust")]
    #[test]
    fn rust_private_function_skipped() {
        let src = "fn internal() {}";
        let out = ts_extract(Path::new("lib.rs"), src).unwrap();
        assert!(out.signatures.is_empty(), "private top-level fn should be skipped");
    }

    #[cfg(feature = "lang-rust")]
    #[test]
    fn rust_struct_and_impl_method() {
        let src = r#"
pub struct Point { pub x: f64, pub y: f64 }
impl Point {
    pub fn distance(&self, other: &Point) -> f64 { 0.0 }
}
"#;
        let out = ts_extract(Path::new("geo.rs"), src).unwrap();
        let names: Vec<_> = out.signatures.iter()
            .filter_map(|s| s.symbol_name.as_deref()).collect();
        assert!(names.contains(&"Point"), "missing struct");
        assert!(names.contains(&"distance"), "missing method");
        let dist = out.signatures.iter().find(|s| s.symbol_name.as_deref() == Some("distance")).unwrap();
        assert_eq!(dist.kind, SymbolKind::Method);
        assert_eq!(dist.qualified_name.as_deref(), Some("Point.distance"));
    }

    #[cfg(feature = "lang-rust")]
    #[test]
    fn rust_imports() {
        let src = "use std::collections::HashMap;\nuse crate::mapper::Signature;\npub fn foo() {}";
        let out = ts_extract(Path::new("main.rs"), src).unwrap();
        assert!(out.imports.iter().any(|i| i.contains("HashMap")), "missing HashMap import");
        assert!(out.imports.iter().any(|i| i.contains("Signature")), "missing Signature import");
    }

    // --- Go -----------------------------------------------------------------

    #[cfg(feature = "lang-go")]
    #[test]
    fn go_function_and_method() {
        let src = r#"
package main
import "fmt"
func Hello(name string) string { return fmt.Sprintf("Hi %s", name) }
func (s *Server) Start(port int) error { return nil }
"#;
        let out = ts_extract(Path::new("main.go"), src).unwrap();
        let names: Vec<_> = out.signatures.iter()
            .filter_map(|s| s.symbol_name.as_deref()).collect();
        assert!(names.contains(&"Hello"));
        assert!(names.contains(&"Start"));
        let start = out.signatures.iter().find(|s| s.symbol_name.as_deref() == Some("Start")).unwrap();
        assert_eq!(start.qualified_name.as_deref(), Some("Server.Start"));
        assert!(out.imports.contains(&"fmt".to_string()));
    }

    #[cfg(feature = "lang-go")]
    #[test]
    fn go_struct_and_interface() {
        let src = "package p\ntype Server struct { port int }\ntype Handler interface { Handle() }";
        let out = ts_extract(Path::new("server.go"), src).unwrap();
        let server = out.signatures.iter().find(|s| s.symbol_name.as_deref() == Some("Server")).unwrap();
        assert_eq!(server.kind, SymbolKind::Struct);
        let handler = out.signatures.iter().find(|s| s.symbol_name.as_deref() == Some("Handler")).unwrap();
        assert_eq!(handler.kind, SymbolKind::Interface);
    }

    // --- Python -------------------------------------------------------------

    #[cfg(feature = "lang-python")]
    #[test]
    fn python_function_and_class() {
        let src = r#"
import os
from pathlib import Path

def greet(name: str) -> str:
    return f"Hi {name}"

class MyClass:
    def method(self) -> None:
        pass
"#;
        let out = ts_extract(Path::new("main.py"), src).unwrap();
        let names: Vec<_> = out.signatures.iter()
            .filter_map(|s| s.symbol_name.as_deref()).collect();
        assert!(names.contains(&"greet"));
        assert!(names.contains(&"MyClass"));
        assert!(names.contains(&"method"));
        let method = out.signatures.iter().find(|s| s.symbol_name.as_deref() == Some("method")).unwrap();
        assert_eq!(method.kind, SymbolKind::Method);
        assert_eq!(method.qualified_name.as_deref(), Some("MyClass.method"));
        assert!(out.imports.contains(&"os".to_string()));
        assert!(out.imports.iter().any(|i| i.contains("pathlib")));
    }

    #[cfg(feature = "lang-python")]
    #[test]
    fn python_private_top_level_skipped() {
        let src = "def _helper(): pass\ndef public(): pass";
        let out = ts_extract(Path::new("util.py"), src).unwrap();
        assert!(!out.signatures.iter().any(|s| s.symbol_name.as_deref() == Some("_helper")));
        assert!(out.signatures.iter().any(|s| s.symbol_name.as_deref() == Some("public")));
    }

    // --- TypeScript ---------------------------------------------------------

    #[cfg(feature = "lang-typescript")]
    #[test]
    fn typescript_class_and_interface() {
        let src = r#"
import { EventEmitter } from 'events';

export interface Handler {
    handle(req: Request): Response;
}
export class Server extends EventEmitter {
    listen(port: number): void {}
}
"#;
        let out = ts_extract(Path::new("server.ts"), src).unwrap();
        let names: Vec<_> = out.signatures.iter()
            .filter_map(|s| s.symbol_name.as_deref()).collect();
        assert!(names.contains(&"Handler"));
        assert!(names.contains(&"Server"));
        assert!(names.contains(&"listen"));
        assert!(out.imports.contains(&"events".to_string()));
    }

    // --- JavaScript ---------------------------------------------------------

    #[cfg(feature = "lang-javascript")]
    #[test]
    fn javascript_function_and_arrow() {
        let src = r#"
import path from 'path';
function add(a, b) { return a + b; }
const multiply = (a, b) => a * b;
"#;
        let out = ts_extract(Path::new("math.js"), src).unwrap();
        let names: Vec<_> = out.signatures.iter()
            .filter_map(|s| s.symbol_name.as_deref()).collect();
        assert!(names.contains(&"add"));
        assert!(names.contains(&"multiply"));
        assert!(out.imports.contains(&"path".to_string()));
    }

    // --- C ------------------------------------------------------------------

    #[cfg(feature = "lang-c")]
    #[test]
    fn c_function_and_struct() {
        let src = r#"
#include <stdio.h>
struct Point { int x; int y; };
int add(int a, int b) { return a + b; }
"#;
        let out = ts_extract(Path::new("math.c"), src).unwrap();
        let names: Vec<_> = out.signatures.iter()
            .filter_map(|s| s.symbol_name.as_deref()).collect();
        assert!(names.contains(&"Point"), "missing struct Point");
        assert!(names.contains(&"add"), "missing function add");
        assert!(out.imports.iter().any(|i| i.contains("stdio")));
    }

    #[cfg(feature = "lang-c")]
    #[test]
    fn c_macro_and_typedef() {
        let src = "#define MAX_SIZE 1024\ntypedef unsigned int uint32_t;\n";
        let out = ts_extract(Path::new("types.h"), src).unwrap();
        assert!(out.signatures.iter().any(|s| s.symbol_name.as_deref() == Some("MAX_SIZE")));
        assert!(out.signatures.iter().any(|s| s.symbol_name.as_deref() == Some("uint32_t")));
    }

    // --- C++ ----------------------------------------------------------------

    #[cfg(feature = "lang-cpp")]
    #[test]
    fn cpp_class_and_namespace() {
        let src = r#"
#include <string>
namespace myapp {
    class Server {
    public:
        void start(int port) {}
    };
}
"#;
        let out = ts_extract(Path::new("server.cpp"), src).unwrap();
        let names: Vec<_> = out.signatures.iter()
            .filter_map(|s| s.symbol_name.as_deref()).collect();
        assert!(names.contains(&"myapp"), "missing namespace");
        assert!(names.contains(&"Server"), "missing class");
        assert!(names.contains(&"start"), "missing method");
        assert!(out.imports.iter().any(|i| i.contains("string")));
    }
}
