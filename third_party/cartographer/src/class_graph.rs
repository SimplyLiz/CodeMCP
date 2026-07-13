//! File-local class/struct diagram extraction for Rust, Python, TypeScript, and Go.
#![allow(dead_code)]
//!
//! Given one source file, produce a `ClassGraph` containing:
//!   - Classes/structs/interfaces/enums with their fields and methods
//!   - Inheritance and implementation relationships
//!
//! Output is rendered by `diagram::render_class()` as a Mermaid `classDiagram`.
//!
//! Gated on the same `lang-*` Cargo features as `extractor.rs` and `call_graph.rs`.

use std::path::Path;

#[cfg(any(
    feature = "lang-rust",
    feature = "lang-python",
    feature = "lang-typescript",
    feature = "lang-go",
    feature = "lang-java",
    feature = "lang-csharp",
    feature = "lang-ruby",
    feature = "lang-kotlin",
    feature = "lang-swift",
    feature = "lang-php",
))]
use tree_sitter::{Node, Parser};

// ---------------------------------------------------------------------------
// Public data model
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ClassKind {
    Struct,
    Class,
    Interface,
    Trait,
    Enum,
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Vis {
    Public,
    Private,
    Protected,
}

#[derive(Debug, Clone)]
pub struct FieldDef {
    pub name: String,
    pub type_annotation: String,
    pub visibility: Vis,
}

#[derive(Debug, Clone)]
pub struct MethodDef {
    pub name: String,
    /// Comma-separated param list with types, e.g. `"x: f64, y: f64"`.
    /// `self`/`&self`/`&mut self` are stripped before storing.
    pub params: String,
    pub return_type: String,
    pub visibility: Vis,
    pub is_static: bool,
    pub is_constructor: bool,
}

#[derive(Debug, Clone)]
pub enum ClassRelationship {
    /// `child` extends / inherits from `parent`.
    Inherits { child: String, parent: String },
    /// `class` implements `interface`.
    Implements { class: String, interface: String },
}

#[derive(Debug, Clone)]
pub struct ClassNode {
    pub name: String,
    pub kind: ClassKind,
    pub fields: Vec<FieldDef>,
    pub methods: Vec<MethodDef>,
}

#[derive(Debug, Clone)]
pub struct ClassGraph {
    pub classes: Vec<ClassNode>,
    pub relationships: Vec<ClassRelationship>,
    pub language: &'static str,
}

// ---------------------------------------------------------------------------
// Public entry point
// ---------------------------------------------------------------------------

/// Extract a class graph from `source`. Returns `Ok(None)` for unsupported file
/// types. Returns `Err` on parser init failures.
pub fn build_class_graph(path: &Path, source: &str) -> Result<Option<ClassGraph>, String> {
    let ext = path
        .extension()
        .and_then(|e| e.to_str())
        .map(|s| s.to_lowercase())
        .unwrap_or_default();

    match ext.as_str() {
        #[cfg(feature = "lang-rust")]
        "rs" => extract_rust(source).map(Some),

        #[cfg(feature = "lang-python")]
        "py" => extract_python(source).map(Some),

        #[cfg(feature = "lang-typescript")]
        "ts" | "tsx" => extract_typescript(source).map(Some),

        #[cfg(feature = "lang-go")]
        "go" => extract_go(source).map(Some),

        #[cfg(feature = "lang-java")]
        "java" => oo_cls::extract(source, tree_sitter_java::language(), "java").map(Some),
        #[cfg(feature = "lang-csharp")]
        "cs" => oo_cls::extract(source, tree_sitter_c_sharp::language(), "csharp").map(Some),
        #[cfg(feature = "lang-kotlin")]
        "kt" | "kts" => oo_cls::extract(source, tree_sitter_kotlin::language(), "kotlin").map(Some),
        #[cfg(feature = "lang-swift")]
        "swift" => oo_cls::extract(source, tree_sitter_swift::language(), "swift").map(Some),
        #[cfg(feature = "lang-php")]
        "php" => oo_cls::extract(source, tree_sitter_php::language_php(), "php").map(Some),
        #[cfg(feature = "lang-ruby")]
        "rb" => oo_cls::extract(source, tree_sitter_ruby::language(), "ruby").map(Some),

        _ => Ok(None),
    }
}

// ---------------------------------------------------------------------------
// Shared helpers
// ---------------------------------------------------------------------------

#[cfg(any(
    feature = "lang-rust",
    feature = "lang-python",
    feature = "lang-typescript",
    feature = "lang-go",
    feature = "lang-java",
    feature = "lang-csharp",
    feature = "lang-ruby",
    feature = "lang-kotlin",
    feature = "lang-swift",
    feature = "lang-php",
))]
fn node_text<'a>(node: &Node, src: &'a [u8]) -> &'a str {
    std::str::from_utf8(&src[node.start_byte()..node.end_byte()]).unwrap_or("")
}

/// Strip leading/trailing whitespace and collapse internal runs of whitespace
/// to a single space — used to normalise raw type annotation text.
#[cfg(any(
    feature = "lang-rust",
    feature = "lang-python",
    feature = "lang-typescript",
    feature = "lang-go",
    feature = "lang-java",
    feature = "lang-csharp",
    feature = "lang-ruby",
    feature = "lang-kotlin",
    feature = "lang-swift",
    feature = "lang-php",
))]
fn normalise(s: &str) -> String {
    s.split_whitespace().collect::<Vec<_>>().join(" ")
}

// ---------------------------------------------------------------------------
// Rust
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-rust")]
fn extract_rust(source: &str) -> Result<ClassGraph, String> {
    let mut parser = Parser::new();
    let lang = tree_sitter_rust::language();
    parser
        .set_language(&lang)
        .map_err(|e| format!("tree-sitter rust init failed: {e}"))?;
    let tree = parser
        .parse(source.as_bytes(), None)
        .ok_or_else(|| "tree-sitter parse returned None".to_string())?;
    let src = source.as_bytes();

    let mut classes: Vec<ClassNode> = Vec::new();
    let mut relationships: Vec<ClassRelationship> = Vec::new();

    walk_rust(&tree.root_node(), src, &mut classes, &mut relationships);
    Ok(ClassGraph { classes, relationships, language: "rust" })
}

#[cfg(feature = "lang-rust")]
fn walk_rust(
    node: &Node,
    src: &[u8],
    classes: &mut Vec<ClassNode>,
    rels: &mut Vec<ClassRelationship>,
) {
    match node.kind() {
        "struct_item" => {
            if let Some(cls) = rust_struct(node, src) {
                classes.push(cls);
            }
        }
        "enum_item" => {
            if let Some(cls) = rust_enum(node, src) {
                classes.push(cls);
            }
        }
        "trait_item" => {
            if let Some(cls) = rust_trait(node, src) {
                classes.push(cls);
            }
        }
        "impl_item" => {
            rust_impl(node, src, classes, rels);
        }
        _ => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                walk_rust(&child, src, classes, rels);
            }
        }
    }
}

#[cfg(feature = "lang-rust")]
fn rust_struct(node: &Node, src: &[u8]) -> Option<ClassNode> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    // Strip generics for the class name — `Foo<T>` → `Foo`.
    let name = name.split('<').next().unwrap_or(&name).trim().to_string();
    if name.is_empty() {
        return None;
    }

    let mut fields: Vec<FieldDef> = Vec::new();
    if let Some(body) = node.child_by_field_name("body") {
        let mut cur = body.walk();
        for child in body.children(&mut cur) {
            if child.kind() == "field_declaration" {
                if let Some(f) = rust_field_decl(&child, src) {
                    fields.push(f);
                }
            }
        }
    }
    Some(ClassNode { name, kind: ClassKind::Struct, fields, methods: Vec::new() })
}

#[cfg(feature = "lang-rust")]
fn rust_field_decl(node: &Node, src: &[u8]) -> Option<FieldDef> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    let type_annotation = node.child_by_field_name("type")
        .map(|n| normalise(node_text(&n, src)))
        .unwrap_or_default();
    let visibility = if rust_is_pub(node, src) { Vis::Public } else { Vis::Private };
    Some(FieldDef { name, type_annotation, visibility })
}

#[cfg(feature = "lang-rust")]
fn rust_enum(node: &Node, src: &[u8]) -> Option<ClassNode> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    let name = name.split('<').next().unwrap_or(&name).trim().to_string();
    if name.is_empty() {
        return None;
    }
    // Enum variants become "fields" in the diagram for compactness.
    let mut fields: Vec<FieldDef> = Vec::new();
    if let Some(body) = node.child_by_field_name("body") {
        let mut cur = body.walk();
        for child in body.children(&mut cur) {
            if child.kind() == "enum_variant" {
                if let Some(n) = child.child_by_field_name("name") {
                    fields.push(FieldDef {
                        name: node_text(&n, src).to_string(),
                        type_annotation: String::new(),
                        visibility: Vis::Public,
                    });
                }
            }
        }
    }
    Some(ClassNode { name, kind: ClassKind::Enum, fields, methods: Vec::new() })
}

#[cfg(feature = "lang-rust")]
fn rust_trait(node: &Node, src: &[u8]) -> Option<ClassNode> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    let name = name.split('<').next().unwrap_or(&name).trim().to_string();
    if name.is_empty() {
        return None;
    }
    let mut methods: Vec<MethodDef> = Vec::new();
    if let Some(body) = node.child_by_field_name("body") {
        let mut cur = body.walk();
        for child in body.children(&mut cur) {
            if child.kind() == "function_item" || child.kind() == "function_signature_item" {
                if let Some(m) = rust_fn_def(&child, src, true) {
                    methods.push(m);
                }
            }
        }
    }
    Some(ClassNode { name, kind: ClassKind::Trait, fields: Vec::new(), methods })
}

/// Extract a method definition or signature from a `function_item` /
/// `function_signature_item` node.
#[cfg(feature = "lang-rust")]
fn rust_fn_def(node: &Node, src: &[u8], default_public: bool) -> Option<MethodDef> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    if name.is_empty() {
        return None;
    }

    let visibility = if rust_is_pub(node, src) || default_public {
        Vis::Public
    } else {
        Vis::Private
    };

    let params = rust_params(node, src);
    let is_static = !params.contains("self");

    // Strip self receivers from the display params.
    let clean_params = rust_strip_self_params(node, src);

    let return_type = node.child_by_field_name("return_type")
        .map(|n| normalise(node_text(&n, src)))
        .unwrap_or_default();

    Some(MethodDef {
        name,
        params: clean_params,
        return_type,
        visibility,
        is_static,
        is_constructor: false,
    })
}

/// Returns `true` if a tree-sitter node has a `visibility_modifier` child
/// (tree-sitter-rust does not expose `visibility` as a named field).
#[cfg(feature = "lang-rust")]
fn rust_is_pub(node: &Node, src: &[u8]) -> bool {
    let mut cur = node.walk();
    for child in node.children(&mut cur) {
        if child.kind() == "visibility_modifier" {
            let text = node_text(&child, src);
            return text == "pub" || text.starts_with("pub(");
        }
    }
    false
}

/// Returns the raw text of the `parameters` node (including self) for inspection.
#[cfg(feature = "lang-rust")]
fn rust_params(node: &Node, src: &[u8]) -> String {
    node.child_by_field_name("parameters")
        .map(|n| node_text(&n, src).to_string())
        .unwrap_or_default()
}

/// Returns a cleaned param string with `self` / `&self` / `&mut self` stripped.
#[cfg(feature = "lang-rust")]
fn rust_strip_self_params(node: &Node, src: &[u8]) -> String {
    let params_node = match node.child_by_field_name("parameters") {
        Some(n) => n,
        None => return String::new(),
    };
    let mut parts: Vec<String> = Vec::new();
    let mut cur = params_node.walk();
    for child in params_node.children(&mut cur) {
        let k = child.kind();
        if k == "self_parameter" || k == "self" {
            continue;
        }
        if k == "parameter" {
            parts.push(normalise(node_text(&child, src)));
        }
    }
    parts.join(", ")
}

/// Process an `impl_item`: attach methods to the target type's `ClassNode`,
/// and emit `Implements` if this is a trait impl.
#[cfg(feature = "lang-rust")]
fn rust_impl(
    node: &Node,
    src: &[u8],
    classes: &mut Vec<ClassNode>,
    rels: &mut Vec<ClassRelationship>,
) {
    let type_name = match node.child_by_field_name("type") {
        Some(n) => {
            let raw = node_text(&n, src);
            raw.split('<').next().unwrap_or(raw).trim().to_string()
        }
        None => return,
    };

    // Detect `impl TraitName for TypeName`.
    let trait_name: Option<String> = node.child_by_field_name("trait")
        .map(|n| {
            let raw = node_text(&n, src);
            raw.split('<').next().unwrap_or(raw).trim().to_string()
        });

    if let Some(ref tname) = trait_name {
        rels.push(ClassRelationship::Implements {
            class: type_name.clone(),
            interface: tname.clone(),
        });
    }

    // Collect methods and attach to the existing ClassNode (or create a stub).
    let body = match node.child_by_field_name("body") {
        Some(b) => b,
        None => return,
    };

    let mut methods: Vec<MethodDef> = Vec::new();
    let mut cur = body.walk();
    for child in body.children(&mut cur) {
        if child.kind() == "function_item" {
            if let Some(m) = rust_fn_def(&child, src, false) {
                methods.push(m);
            }
        }
    }

    if methods.is_empty() {
        return;
    }

    // Attach to an existing ClassNode or create a stub struct node.
    if let Some(existing) = classes.iter_mut().find(|c| c.name == type_name) {
        existing.methods.extend(methods);
    } else {
        classes.push(ClassNode {
            name: type_name,
            kind: ClassKind::Struct,
            fields: Vec::new(),
            methods,
        });
    }
}

// ---------------------------------------------------------------------------
// Python
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-python")]
fn extract_python(source: &str) -> Result<ClassGraph, String> {
    let mut parser = Parser::new();
    let lang = tree_sitter_python::language();
    parser
        .set_language(&lang)
        .map_err(|e| format!("tree-sitter python init failed: {e}"))?;
    let tree = parser
        .parse(source.as_bytes(), None)
        .ok_or_else(|| "tree-sitter parse returned None".to_string())?;
    let src = source.as_bytes();

    let mut classes: Vec<ClassNode> = Vec::new();
    let mut relationships: Vec<ClassRelationship> = Vec::new();

    let root = tree.root_node();
    let mut cur = root.walk();
    for child in root.children(&mut cur) {
        if child.kind() == "class_definition" {
            if let Some(cls) = python_class(&child, src, &mut relationships) {
                classes.push(cls);
            }
        }
    }

    Ok(ClassGraph { classes, relationships, language: "python" })
}

#[cfg(feature = "lang-python")]
fn python_class(
    node: &Node,
    src: &[u8],
    rels: &mut Vec<ClassRelationship>,
) -> Option<ClassNode> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    if name.is_empty() {
        return None;
    }

    // Base classes from the `superclasses` field (argument_list).
    if let Some(bases) = node.child_by_field_name("superclasses") {
        let mut cur = bases.walk();
        for child in bases.children(&mut cur) {
            let k = child.kind();
            if k == "identifier" || k == "attribute" {
                let base = node_text(&child, src)
                    .split('<').next().unwrap_or("")
                    .trim().to_string();
                if !base.is_empty() {
                    rels.push(ClassRelationship::Inherits {
                        child: name.clone(),
                        parent: base,
                    });
                }
            }
        }
    }

    let mut fields: Vec<FieldDef> = Vec::new();
    let mut methods: Vec<MethodDef> = Vec::new();

    let body = match node.child_by_field_name("body") {
        Some(b) => b,
        None => return Some(ClassNode { name, kind: ClassKind::Class, fields, methods }),
    };

    // Walk class body for method definitions.
    let mut cur = body.walk();
    for child in body.children(&mut cur) {
        match child.kind() {
            "function_definition" | "decorated_definition" => {
                let fn_node = if child.kind() == "decorated_definition" {
                    // decorated_definition wraps a function_definition.
                    let mut c = child.walk();
                    let found = child.children(&mut c).find(|ch| ch.kind() == "function_definition");
                    found
                } else {
                    Some(child)
                };
                if let Some(fn_node) = fn_node {
                    if let Some(m) = python_method(&fn_node, src, &name, &mut fields) {
                        methods.push(m);
                    }
                }
            }
            _ => {}
        }
    }

    Some(ClassNode { name, kind: ClassKind::Class, fields, methods })
}

#[cfg(feature = "lang-python")]
fn python_method(
    node: &Node,
    src: &[u8],
    class_name: &str,
    fields: &mut Vec<FieldDef>,
) -> Option<MethodDef> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    if name.is_empty() {
        return None;
    }

    let is_constructor = name == "__init__";
    // Python methods are public unless name starts with `_`.
    let visibility = if name.starts_with("__") && !is_constructor {
        Vis::Private
    } else if name.starts_with('_') {
        Vis::Protected
    } else {
        Vis::Public
    };

    // Collect params, strip `self`.
    let params = python_clean_params(node, src);

    // Return type annotation.
    let return_type = node.child_by_field_name("return_type")
        .map(|n| normalise(node_text(&n, src)))
        .unwrap_or_default();

    // For __init__, harvest `self.x = ...` assignments as instance fields.
    if is_constructor {
        if let Some(body) = node.child_by_field_name("body") {
            python_collect_instance_fields(&body, src, class_name, fields);
        }
    }

    Some(MethodDef {
        name,
        params,
        return_type,
        visibility,
        is_static: false,
        is_constructor,
    })
}

#[cfg(feature = "lang-python")]
fn python_clean_params(node: &Node, src: &[u8]) -> String {
    let params_node = match node.child_by_field_name("parameters") {
        Some(n) => n,
        None => return String::new(),
    };
    let mut parts: Vec<String> = Vec::new();
    let mut cur = params_node.walk();
    for child in params_node.children(&mut cur) {
        let k = child.kind();
        if k == "identifier" && node_text(&child, src) == "self" {
            continue;
        }
        if matches!(k, "identifier" | "typed_parameter" | "typed_default_parameter"
            | "default_parameter" | "list_splat_pattern" | "dictionary_splat_pattern") {
            parts.push(normalise(node_text(&child, src)));
        }
    }
    parts.join(", ")
}

/// Walk a function body looking for `self.attr = value` or `self.attr: Type = value`.
#[cfg(feature = "lang-python")]
fn python_collect_instance_fields(
    body: &Node,
    src: &[u8],
    _class_name: &str,
    fields: &mut Vec<FieldDef>,
) {
    let mut cur = body.walk();
    for stmt in body.children(&mut cur) {
        let kind = stmt.kind();
        if kind == "expression_statement" {
            if let Some(inner) = stmt.child(0) {
                if inner.kind() == "assignment" || inner.kind() == "augmented_assignment" {
                    python_try_self_field(&inner, src, fields);
                }
            }
        } else if kind == "assignment" {
            python_try_self_field(&stmt, src, fields);
        }
    }
}

#[cfg(feature = "lang-python")]
fn python_try_self_field(node: &Node, src: &[u8], fields: &mut Vec<FieldDef>) {
    let lhs = match node.child_by_field_name("left") {
        Some(n) => n,
        None => return,
    };
    // `self.attr` is an `attribute` node whose object is `self`.
    if lhs.kind() != "attribute" {
        return;
    }
    let obj = match lhs.child_by_field_name("object") {
        Some(n) => node_text(&n, src),
        None => return,
    };
    if obj != "self" {
        return;
    }
    let attr = match lhs.child_by_field_name("attribute") {
        Some(n) => node_text(&n, src).to_string(),
        None => return,
    };
    if attr.is_empty() || fields.iter().any(|f| f.name == attr) {
        return;
    }
    // Type annotation from `self.attr: Type = ...`
    let type_annotation = node.child_by_field_name("type")
        .map(|n| normalise(node_text(&n, src)))
        .unwrap_or_default();
    fields.push(FieldDef {
        name: attr,
        type_annotation,
        visibility: Vis::Public,
    });
}

// ---------------------------------------------------------------------------
// TypeScript
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-typescript")]
fn extract_typescript(source: &str) -> Result<ClassGraph, String> {
    let mut parser = Parser::new();
    let lang = tree_sitter_typescript::language_typescript();
    parser
        .set_language(&lang)
        .map_err(|e| format!("tree-sitter typescript init failed: {e}"))?;
    let tree = parser
        .parse(source.as_bytes(), None)
        .ok_or_else(|| "tree-sitter parse returned None".to_string())?;
    let src = source.as_bytes();

    let mut classes: Vec<ClassNode> = Vec::new();
    let mut relationships: Vec<ClassRelationship> = Vec::new();

    walk_ts(&tree.root_node(), src, &mut classes, &mut relationships);
    Ok(ClassGraph { classes, relationships, language: "typescript" })
}

#[cfg(feature = "lang-typescript")]
fn walk_ts(
    node: &Node,
    src: &[u8],
    classes: &mut Vec<ClassNode>,
    rels: &mut Vec<ClassRelationship>,
) {
    match node.kind() {
        "class_declaration" | "abstract_class_declaration" => {
            if let Some(cls) = ts_class(node, src, rels) {
                classes.push(cls);
            }
        }
        "interface_declaration" => {
            if let Some(cls) = ts_interface(node, src, rels) {
                classes.push(cls);
            }
        }
        _ => {
            let mut cur = node.walk();
            for child in node.children(&mut cur) {
                walk_ts(&child, src, classes, rels);
            }
        }
    }
}

#[cfg(feature = "lang-typescript")]
fn ts_class(
    node: &Node,
    src: &[u8],
    rels: &mut Vec<ClassRelationship>,
) -> Option<ClassNode> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    let name = name.split('<').next().unwrap_or(&name).trim().to_string();
    if name.is_empty() {
        return None;
    }

    // `extends` clause.
    if let Some(extends) = node.child_by_field_name("extends") {
        let parent = node_text(&extends, src);
        let parent = parent.trim_start_matches("extends").trim();
        let parent = parent.split('<').next().unwrap_or(parent).trim();
        if !parent.is_empty() {
            rels.push(ClassRelationship::Inherits {
                child: name.clone(),
                parent: parent.to_string(),
            });
        }
    }

    // `implements` clause — may list multiple interfaces separated by commas.
    let mut cur = node.walk();
    for child in node.children(&mut cur) {
        if child.kind() == "implements_clause" {
            let mut ic = child.walk();
            for iface in child.children(&mut ic) {
                if iface.kind() == "type_identifier" || iface.kind() == "generic_type" {
                    let raw = node_text(&iface, src);
                    let iface_name = raw.split('<').next().unwrap_or(raw).trim();
                    if !iface_name.is_empty() {
                        rels.push(ClassRelationship::Implements {
                            class: name.clone(),
                            interface: iface_name.to_string(),
                        });
                    }
                }
            }
        }
    }

    let body = match node.child_by_field_name("body") {
        Some(b) => b,
        None => return Some(ClassNode { name, kind: ClassKind::Class, fields: Vec::new(), methods: Vec::new() }),
    };

    let (fields, methods) = ts_class_body(&body, src);
    Some(ClassNode { name, kind: ClassKind::Class, fields, methods })
}

#[cfg(feature = "lang-typescript")]
fn ts_interface(
    node: &Node,
    src: &[u8],
    rels: &mut Vec<ClassRelationship>,
) -> Option<ClassNode> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    let name = name.split('<').next().unwrap_or(&name).trim().to_string();
    if name.is_empty() {
        return None;
    }

    // Interface extends.
    let mut cur = node.walk();
    for child in node.children(&mut cur) {
        if child.kind() == "extends_type_clause" {
            let mut ec = child.walk();
            for iface in child.children(&mut ec) {
                if iface.kind() == "type_identifier" || iface.kind() == "generic_type" {
                    let raw = node_text(&iface, src);
                    let iface_name = raw.split('<').next().unwrap_or(raw).trim();
                    if !iface_name.is_empty() {
                        rels.push(ClassRelationship::Inherits {
                            child: name.clone(),
                            parent: iface_name.to_string(),
                        });
                    }
                }
            }
        }
    }

    let body = match node.child_by_field_name("body") {
        Some(b) => b,
        None => return Some(ClassNode { name, kind: ClassKind::Interface, fields: Vec::new(), methods: Vec::new() }),
    };

    let mut fields: Vec<FieldDef> = Vec::new();
    let mut methods: Vec<MethodDef> = Vec::new();
    let mut cur = body.walk();
    for child in body.children(&mut cur) {
        match child.kind() {
            "property_signature" => {
                if let Some(f) = ts_property_sig(&child, src) {
                    fields.push(f);
                }
            }
            "method_signature" => {
                if let Some(m) = ts_method_sig(&child, src) {
                    methods.push(m);
                }
            }
            _ => {}
        }
    }

    Some(ClassNode { name, kind: ClassKind::Interface, fields, methods })
}

#[cfg(feature = "lang-typescript")]
fn ts_class_body(body: &Node, src: &[u8]) -> (Vec<FieldDef>, Vec<MethodDef>) {
    let mut fields: Vec<FieldDef> = Vec::new();
    let mut methods: Vec<MethodDef> = Vec::new();

    let mut cur = body.walk();
    for child in body.children(&mut cur) {
        match child.kind() {
            "public_field_definition" | "field_definition" => {
                if let Some(f) = ts_field_def(&child, src) {
                    fields.push(f);
                }
            }
            "method_definition" => {
                if let Some(m) = ts_method_def(&child, src) {
                    methods.push(m);
                }
            }
            _ => {}
        }
    }
    (fields, methods)
}

#[cfg(feature = "lang-typescript")]
fn ts_visibility(node: &Node, src: &[u8]) -> Vis {
    let mut cur = node.walk();
    for child in node.children(&mut cur) {
        if child.kind() == "accessibility_modifier" {
            return match node_text(&child, src) {
                "public" => Vis::Public,
                "private" => Vis::Private,
                "protected" => Vis::Protected,
                _ => Vis::Public,
            };
        }
    }
    Vis::Public
}

#[cfg(feature = "lang-typescript")]
fn ts_field_def(node: &Node, src: &[u8]) -> Option<FieldDef> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    let type_annotation = node.child_by_field_name("type")
        .map(|n| {
            // type node text includes the leading `:`, strip it.
            normalise(node_text(&n, src)).trim_start_matches(':').trim().to_string()
        })
        .unwrap_or_default();
    let visibility = ts_visibility(node, src);
    Some(FieldDef { name, type_annotation, visibility })
}

#[cfg(feature = "lang-typescript")]
fn ts_method_def(node: &Node, src: &[u8]) -> Option<MethodDef> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    if name.is_empty() {
        return None;
    }
    let is_constructor = name == "constructor";
    let visibility = ts_visibility(node, src);
    let params = ts_clean_params(node, src);
    let return_type = node.child_by_field_name("return_type")
        .map(|n| normalise(node_text(&n, src)).trim_start_matches(':').trim().to_string())
        .unwrap_or_default();
    Some(MethodDef {
        name,
        params,
        return_type,
        visibility,
        is_static: false,
        is_constructor,
    })
}

#[cfg(feature = "lang-typescript")]
fn ts_property_sig(node: &Node, src: &[u8]) -> Option<FieldDef> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    let type_annotation = node.child_by_field_name("type")
        .map(|n| normalise(node_text(&n, src)).trim_start_matches(':').trim().to_string())
        .unwrap_or_default();
    Some(FieldDef { name, type_annotation, visibility: Vis::Public })
}

#[cfg(feature = "lang-typescript")]
fn ts_method_sig(node: &Node, src: &[u8]) -> Option<MethodDef> {
    let name = node.child_by_field_name("name")
        .map(|n| node_text(&n, src).to_string())?;
    let params = ts_clean_params(node, src);
    let return_type = node.child_by_field_name("return_type")
        .map(|n| normalise(node_text(&n, src)).trim_start_matches(':').trim().to_string())
        .unwrap_or_default();
    Some(MethodDef {
        name,
        params,
        return_type,
        visibility: Vis::Public,
        is_static: false,
        is_constructor: false,
    })
}

#[cfg(feature = "lang-typescript")]
fn ts_clean_params(node: &Node, src: &[u8]) -> String {
    let params_node = match node.child_by_field_name("parameters") {
        Some(n) => n,
        None => return String::new(),
    };
    // Return the inner text of the parameters node, stripping the parentheses.
    let raw = node_text(&params_node, src);
    let inner = raw.trim_start_matches('(').trim_end_matches(')');
    normalise(inner)
}

// ---------------------------------------------------------------------------
// Go
// ---------------------------------------------------------------------------

#[cfg(feature = "lang-go")]
fn extract_go(source: &str) -> Result<ClassGraph, String> {
    let mut parser = Parser::new();
    let lang = tree_sitter_go::language();
    parser
        .set_language(&lang)
        .map_err(|e| format!("tree-sitter go init failed: {e}"))?;
    let tree = parser
        .parse(source.as_bytes(), None)
        .ok_or_else(|| "tree-sitter parse returned None".to_string())?;
    let src = source.as_bytes();

    let mut classes: Vec<ClassNode> = Vec::new();
    let mut relationships: Vec<ClassRelationship> = Vec::new();

    let root = tree.root_node();
    let mut cur = root.walk();
    for child in root.children(&mut cur) {
        match child.kind() {
            "type_declaration" => {
                go_type_decl(&child, src, &mut classes, &mut relationships);
            }
            "method_declaration" => {
                go_method_decl(&child, src, &mut classes);
            }
            _ => {}
        }
    }

    Ok(ClassGraph { classes, relationships, language: "go" })
}

#[cfg(feature = "lang-go")]
fn go_type_decl(
    node: &Node,
    src: &[u8],
    classes: &mut Vec<ClassNode>,
    rels: &mut Vec<ClassRelationship>,
) {
    let mut cur = node.walk();
    for child in node.children(&mut cur) {
        if child.kind() == "type_spec" {
            go_type_spec(&child, src, classes, rels);
        }
    }
}

#[cfg(feature = "lang-go")]
fn go_type_spec(
    node: &Node,
    src: &[u8],
    classes: &mut Vec<ClassNode>,
    rels: &mut Vec<ClassRelationship>,
) {
    let name = match node.child_by_field_name("name") {
        Some(n) => node_text(&n, src).to_string(),
        None => return,
    };
    let type_node = match node.child_by_field_name("type") {
        Some(n) => n,
        None => return,
    };

    match type_node.kind() {
        "struct_type" => {
            let (fields, embed_rels) = go_struct_fields(&type_node, src, &name);
            rels.extend(embed_rels);
            classes.push(ClassNode { name, kind: ClassKind::Struct, fields, methods: Vec::new() });
        }
        "interface_type" => {
            let (methods, iface_rels) = go_interface_body(&type_node, src, &name);
            rels.extend(iface_rels);
            classes.push(ClassNode { name, kind: ClassKind::Interface, fields: Vec::new(), methods });
        }
        _ => {}
    }
}

#[cfg(feature = "lang-go")]
fn go_struct_fields(
    node: &Node,
    src: &[u8],
    struct_name: &str,
) -> (Vec<FieldDef>, Vec<ClassRelationship>) {
    let mut fields: Vec<FieldDef> = Vec::new();
    let mut rels: Vec<ClassRelationship> = Vec::new();

    let mut cur = node.walk();
    for child in node.children(&mut cur) {
        if child.kind() == "field_declaration_list" {
            let mut fc = child.walk();
            for decl in child.children(&mut fc) {
                if decl.kind() == "field_declaration" {
                    // Check for embedded field (anonymous struct embedding).
                    let field_name_node = decl.child_by_field_name("name");
                    let field_type_node = decl.child_by_field_name("type");

                    if field_name_node.is_none() {
                        // Embedded type — no name means anonymous embedding.
                        if let Some(t) = field_type_node {
                            let embedded = node_text(&t, src).trim_start_matches('*').trim().to_string();
                            rels.push(ClassRelationship::Inherits {
                                child: struct_name.to_string(),
                                parent: embedded,
                            });
                        }
                    } else if let Some(name_node) = field_name_node {
                        let name = node_text(&name_node, src).to_string();
                        let type_annotation = field_type_node
                            .map(|n| normalise(node_text(&n, src)))
                            .unwrap_or_default();
                        // Go fields are public if name starts with uppercase.
                        let visibility = if name.chars().next().map(|c| c.is_uppercase()).unwrap_or(false) {
                            Vis::Public
                        } else {
                            Vis::Private
                        };
                        fields.push(FieldDef { name, type_annotation, visibility });
                    }
                }
            }
        }
    }
    (fields, rels)
}

#[cfg(feature = "lang-go")]
fn go_interface_body(
    node: &Node,
    src: &[u8],
    iface_name: &str,
) -> (Vec<MethodDef>, Vec<ClassRelationship>) {
    let mut methods: Vec<MethodDef> = Vec::new();
    let mut rels: Vec<ClassRelationship> = Vec::new();

    let mut cur = node.walk();
    for child in node.children(&mut cur) {
        if child.kind() == "method_elem" {
            // `method_elem` has a `name` (method) or a `type_name` (embedding).
            if let Some(name_node) = child.child_by_field_name("name") {
                let name = node_text(&name_node, src).to_string();
                let params = go_params(&child, src);
                let return_type = go_result(&child, src);
                methods.push(MethodDef {
                    name,
                    params,
                    return_type,
                    visibility: Vis::Public,
                    is_static: false,
                    is_constructor: false,
                });
            } else {
                // Interface embedding.
                let embedded = node_text(&child, src).trim().to_string();
                if !embedded.is_empty() {
                    rels.push(ClassRelationship::Implements {
                        class: iface_name.to_string(),
                        interface: embedded,
                    });
                }
            }
        }
    }
    (methods, rels)
}

#[cfg(feature = "lang-go")]
fn go_method_decl(node: &Node, src: &[u8], classes: &mut Vec<ClassNode>) {
    // Receiver type tells us which struct this method belongs to.
    let receiver = match node.child_by_field_name("receiver") {
        Some(r) => r,
        None => return,
    };

    // The receiver parameter list: `(r *MyStruct)` or `(r MyStruct)`.
    let receiver_type = {
        let raw = node_text(&receiver, src);
        // Strip parens, then find the type part (last token before whitespace groups).
        let inner = raw.trim_start_matches('(').trim_end_matches(')').trim();
        // Format is `name Type` or just `Type`.
        let type_part = inner.split_whitespace().last().unwrap_or("").trim_start_matches('*');
        type_part.to_string()
    };

    if receiver_type.is_empty() {
        return;
    }

    let fn_name = match node.child_by_field_name("name") {
        Some(n) => node_text(&n, src).to_string(),
        None => return,
    };
    if fn_name.is_empty() {
        return;
    }

    let params = go_params(node, src);
    let return_type = go_result(node, src);
    let visibility = if fn_name.chars().next().map(|c| c.is_uppercase()).unwrap_or(false) {
        Vis::Public
    } else {
        Vis::Private
    };

    let method = MethodDef {
        name: fn_name,
        params,
        return_type,
        visibility,
        is_static: false,
        is_constructor: false,
    };

    if let Some(existing) = classes.iter_mut().find(|c| c.name == receiver_type) {
        existing.methods.push(method);
    } else {
        classes.push(ClassNode {
            name: receiver_type,
            kind: ClassKind::Struct,
            fields: Vec::new(),
            methods: vec![method],
        });
    }
}

#[cfg(feature = "lang-go")]
fn go_params(node: &Node, src: &[u8]) -> String {
    node.child_by_field_name("parameters")
        .map(|n| {
            let raw = node_text(&n, src);
            let inner = raw.trim_start_matches('(').trim_end_matches(')');
            normalise(inner)
        })
        .unwrap_or_default()
}

#[cfg(feature = "lang-go")]
fn go_result(node: &Node, src: &[u8]) -> String {
    node.child_by_field_name("result")
        .map(|n| normalise(node_text(&n, src)))
        .unwrap_or_default()
}

// ---------------------------------------------------------------------------
// Generic OO class graph: Java, C#, Kotlin, Swift, PHP, Ruby
// ---------------------------------------------------------------------------

#[cfg(any(
    feature = "lang-java", feature = "lang-csharp", feature = "lang-ruby",
    feature = "lang-kotlin", feature = "lang-swift", feature = "lang-php",
))]
mod oo_cls {
    use super::{
        node_text, ClassGraph, ClassKind, ClassNode, ClassRelationship, FieldDef, MethodDef, Vis,
    };
    use tree_sitter::{Language, Node, Parser};

    fn name_of(node: &Node, src: &[u8]) -> Option<String> {
        if let Some(n) = node.child_by_field_name("name") {
            let t = node_text(&n, src);
            if !t.is_empty() {
                return Some(t.to_string());
            }
        }
        let mut cur = node.walk();
        for ch in node.children(&mut cur) {
            if matches!(
                ch.kind(),
                "identifier" | "type_identifier" | "simple_identifier" | "name" | "constant"
            ) {
                let t = node_text(&ch, src);
                if !t.is_empty() {
                    return Some(t.to_string());
                }
            }
        }
        None
    }

    fn kind_of(k: &str) -> Option<ClassKind> {
        match k {
            "class_declaration" | "object_declaration" | "class" => Some(ClassKind::Class),
            "interface_declaration" | "protocol_declaration" => Some(ClassKind::Interface),
            "trait_declaration" => Some(ClassKind::Trait),
            "struct_declaration" => Some(ClassKind::Struct),
            "enum_declaration" => Some(ClassKind::Enum),
            _ => None,
        }
    }

    fn head(node: &Node, src: &[u8]) -> String {
        let t = node_text(node, src);
        t.split(|c| c == '{' || c == '(').next().unwrap_or("").to_string()
    }

    fn vis_of(node: &Node, src: &[u8]) -> Vis {
        let h = head(node, src);
        if h.contains("private") {
            Vis::Private
        } else if h.contains("protected") {
            Vis::Protected
        } else {
            Vis::Public
        }
    }

    fn params_text(node: &Node, src: &[u8]) -> String {
        let mut cur = node.walk();
        for ch in node.children(&mut cur) {
            if matches!(
                ch.kind(),
                "formal_parameters" | "parameter_list" | "function_value_parameters"
                    | "parameters" | "method_parameters"
            ) {
                let t = node_text(&ch, src);
                let inner = t.trim().trim_start_matches('(').trim_end_matches(')').trim();
                return inner.split_whitespace().collect::<Vec<_>>().join(" ");
            }
        }
        String::new()
    }

    fn method_def(node: &Node, src: &[u8]) -> Option<MethodDef> {
        let name = name_of(node, src)?;
        if name.is_empty() {
            return None;
        }
        Some(MethodDef {
            name,
            params: params_text(node, src),
            return_type: String::new(),
            visibility: vis_of(node, src),
            is_static: head(node, src).contains("static"),
            is_constructor: node.kind() == "constructor_declaration",
        })
    }

    fn field_defs(node: &Node, src: &[u8]) -> Vec<FieldDef> {
        let vis = vis_of(node, src);
        let mut out = Vec::new();
        if let Some(n) = node.child_by_field_name("name") {
            out.push(FieldDef {
                name: node_text(&n, src).to_string(),
                type_annotation: String::new(),
                visibility: vis,
            });
            return out;
        }
        let mut cur = node.walk();
        for ch in node.children(&mut cur) {
            if ch.kind() == "variable_declarator" {
                if let Some(nm) = ch.child_by_field_name("name") {
                    out.push(FieldDef {
                        name: node_text(&nm, src).to_string(),
                        type_annotation: String::new(),
                        visibility: vis.clone(),
                    });
                }
            }
        }
        if out.is_empty() {
            if let Some(nm) = name_of(node, src) {
                out.push(FieldDef { name: nm, type_annotation: String::new(), visibility: vis });
            }
        }
        out
    }

    fn body_of<'a>(node: &Node<'a>) -> Option<Node<'a>> {
        let mut cur = node.walk();
        let children: Vec<Node<'a>> = node.children(&mut cur).collect();
        children.into_iter().find(|c| {
            matches!(
                c.kind(),
                "class_body" | "declaration_list" | "enum_body" | "interface_body"
                    | "enum_class_body" | "protocol_body" | "body_statement"
            )
        })
    }

    fn collect_members(node: &Node, src: &[u8], fields: &mut Vec<FieldDef>, methods: &mut Vec<MethodDef>) {
        if let Some(body) = body_of(node) {
            let mut cur = body.walk();
            for ch in body.children(&mut cur) {
                match ch.kind() {
                    "method_declaration" | "constructor_declaration" | "function_declaration"
                    | "function_definition" | "method" | "singleton_method" => {
                        if let Some(m) = method_def(&ch, src) {
                            methods.push(m);
                        }
                    }
                    "field_declaration" | "property_declaration" | "property_definition" => {
                        fields.extend(field_defs(&ch, src));
                    }
                    _ => {}
                }
            }
        }
    }

    fn type_names(node: &Node, src: &[u8], out: &mut Vec<String>) {
        if matches!(
            node.kind(),
            "type_identifier" | "identifier" | "name" | "user_type" | "constant"
                | "qualified_name" | "scoped_type_identifier" | "simple_identifier"
        ) {
            let t = node_text(node, src).trim();
            if t.chars().next().map(|c| c.is_alphabetic()).unwrap_or(false) {
                out.push(t.rsplit(|c| c == '.' || c == '\\' || c == ':').next().unwrap_or(t).to_string());
            }
            return;
        }
        let mut cur = node.walk();
        for ch in node.children(&mut cur) {
            type_names(&ch, src, out);
        }
    }

    fn collect_rels(node: &Node, src: &[u8], name: &str, rels: &mut Vec<ClassRelationship>) {
        let mut cur = node.walk();
        for ch in node.children(&mut cur) {
            let k = ch.kind();
            let mut names = Vec::new();
            if k == "superclass" || k == "base_clause" {
                type_names(&ch, src, &mut names);
                for p in names {
                    rels.push(ClassRelationship::Inherits { child: name.to_string(), parent: p });
                }
            } else if matches!(
                k,
                "super_interfaces" | "class_interface_clause" | "type_inheritance_clause"
                    | "delegation_specifier"
            ) {
                type_names(&ch, src, &mut names);
                for i in names {
                    rels.push(ClassRelationship::Implements { class: name.to_string(), interface: i });
                }
            }
        }
    }

    fn walk(node: &Node, src: &[u8], classes: &mut Vec<ClassNode>, rels: &mut Vec<ClassRelationship>) {
        if let Some(kind) = kind_of(node.kind()) {
            if let Some(name) = name_of(node, src) {
                if !name.is_empty() {
                    let mut fields = Vec::new();
                    let mut methods = Vec::new();
                    collect_members(node, src, &mut fields, &mut methods);
                    collect_rels(node, src, &name, rels);
                    classes.push(ClassNode { name, kind, fields, methods });
                }
            }
        }
        let mut cur = node.walk();
        for ch in node.children(&mut cur) {
            walk(&ch, src, classes, rels);
        }
    }

    pub(super) fn extract(
        source: &str,
        lang: Language,
        language: &'static str,
    ) -> Result<ClassGraph, String> {
        let mut parser = Parser::new();
        parser
            .set_language(&lang)
            .map_err(|e| format!("tree-sitter {language} init failed: {e}"))?;
        let tree = parser
            .parse(source.as_bytes(), None)
            .ok_or_else(|| "tree-sitter parse returned None".to_string())?;
        let src = source.as_bytes();
        let mut classes = Vec::new();
        let mut relationships = Vec::new();
        walk(&tree.root_node(), src, &mut classes, &mut relationships);
        Ok(ClassGraph { classes, relationships, language })
    }
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

#[cfg(test)]
mod tests {
    use super::*;
    use std::path::PathBuf;

    fn path(name: &str) -> PathBuf {
        PathBuf::from(name)
    }

    #[cfg(feature = "lang-rust")]
    #[test]
    fn rust_struct_fields_and_impl_methods() {
        let src = r#"
pub struct Point {
    pub x: f64,
    pub y: f64,
    label: String,
}

impl Point {
    pub fn new(x: f64, y: f64) -> Self { todo!() }
    pub fn distance(&self, other: &Point) -> f64 { todo!() }
    fn internal(&self) {}
}
"#;
        let g = build_class_graph(&path("src/lib.rs"), src)
            .unwrap()
            .unwrap();
        assert_eq!(g.classes.len(), 1);
        let cls = &g.classes[0];
        assert_eq!(cls.name, "Point");
        assert_eq!(cls.kind, ClassKind::Struct);
        assert_eq!(cls.fields.len(), 3);
        assert_eq!(cls.fields[0].name, "x");
        assert_eq!(cls.fields[0].visibility, Vis::Public);
        assert_eq!(cls.fields[2].visibility, Vis::Private);
        assert_eq!(cls.methods.len(), 3);
        let new_m = cls.methods.iter().find(|m| m.name == "new").unwrap();
        assert_eq!(new_m.visibility, Vis::Public);
        assert!(new_m.is_static);
        let dist = cls.methods.iter().find(|m| m.name == "distance").unwrap();
        assert!(!dist.is_static);
        let internal = cls.methods.iter().find(|m| m.name == "internal").unwrap();
        assert_eq!(internal.visibility, Vis::Private);
    }

    #[cfg(feature = "lang-rust")]
    #[test]
    fn rust_trait_impl_emits_implements_relationship() {
        let src = r#"
trait Animal {
    fn sound(&self) -> &str;
}
struct Dog {}
impl Animal for Dog {
    fn sound(&self) -> &str { "woof" }
}
"#;
        let g = build_class_graph(&path("src/lib.rs"), src)
            .unwrap()
            .unwrap();
        assert!(g.relationships.iter().any(|r| matches!(r,
            ClassRelationship::Implements { class, interface }
            if class == "Dog" && interface == "Animal"
        )));
    }

    #[cfg(feature = "lang-rust")]
    #[test]
    fn rust_enum_variants_as_fields() {
        let src = r#"
pub enum Color { Red, Green, Blue }
"#;
        let g = build_class_graph(&path("src/lib.rs"), src).unwrap().unwrap();
        let cls = g.classes.iter().find(|c| c.name == "Color").unwrap();
        assert_eq!(cls.kind, ClassKind::Enum);
        assert_eq!(cls.fields.len(), 3);
    }

    #[cfg(feature = "lang-python")]
    #[test]
    fn python_class_with_base_and_init_fields() {
        let src = r#"
class Animal:
    def __init__(self, name: str):
        self.name = name

class Dog(Animal):
    def __init__(self, name: str, breed: str):
        self.breed = breed
    def bark(self) -> str:
        return "woof"
"#;
        let g = build_class_graph(&path("animals.py"), src).unwrap().unwrap();
        let dog = g.classes.iter().find(|c| c.name == "Dog").unwrap();
        assert_eq!(dog.kind, ClassKind::Class);
        assert!(dog.fields.iter().any(|f| f.name == "breed"));
        assert!(g.relationships.iter().any(|r| matches!(r,
            ClassRelationship::Inherits { child, parent }
            if child == "Dog" && parent == "Animal"
        )));
    }
}
