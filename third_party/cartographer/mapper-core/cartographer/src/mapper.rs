//! Mapper module — Extracts skeleton signatures from source files.
//!
//! Symbol metadata follows the LIP (Linked Incremental Protocol) taxonomy:
//!   - `SymbolKind` : matches LIP §4.1 enum (+ `Struct` extension for Rust/C/Go)
//!   - `ckb_id`     : LIP symbol URI  `lip://local/<path>#<qualified_name>`
//!   - `confidence` : 30 = Tier 1 regex heuristic
//!   - `line_start` : 0-indexed, matches LIP `Range.start_line`

use regex::Regex;
use serde::{Deserialize, Serialize};
use std::path::Path;

// ---------------------------------------------------------------------------
// SymbolKind — LIP §4.1 taxonomy
// ---------------------------------------------------------------------------

/// Symbol classification following LIP SymbolKind (§4.1).
/// `Struct` is a Cartographer extension; maps to `Class` in future LIP wire format.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize, Default)]
pub enum SymbolKind {
    #[default]
    Unknown,
    Namespace,
    Class,
    Struct,
    Interface,
    Method,
    Field,
    Variable,
    Function,
    TypeParameter,
    Parameter,
    Macro,
    Enum,
    EnumMember,
    Constructor,
    TypeAlias,
}

// ---------------------------------------------------------------------------
// Signature
// ---------------------------------------------------------------------------

fn default_confidence() -> u8 {
    30
}

/// A symbol extracted from a source file with LIP-compatible metadata.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Signature {
    /// Raw text of the signature (no body).
    pub raw: String,
    /// LIP symbol URI: `lip://local/<path>#<qualified_name>`.
    pub ckb_id: Option<String>,
    /// Unqualified symbol name (e.g. `"bar"`).
    pub symbol_name: Option<String>,
    /// Scope-qualified name (e.g. `"Foo.bar"`).
    #[serde(default)]
    pub qualified_name: Option<String>,
    /// Symbol kind from LIP taxonomy.
    #[serde(default)]
    pub kind: SymbolKind,
    /// 0-indexed line number of this signature.
    #[serde(default)]
    pub line_start: usize,
    /// Confidence score (1–100). 30 = Tier 1 regex heuristic.
    #[serde(default = "default_confidence")]
    pub confidence: u8,
    /// Doc comment extracted from lines immediately preceding this signature.
    #[serde(default)]
    pub doc_comment: Option<String>,
}

impl Signature {
    fn new(
        raw: String,
        kind: SymbolKind,
        line_start: usize,
        path: &str,
        qualified_name: String,
        doc_comment: Option<String>,
    ) -> Self {
        let symbol_name = unqualified(&qualified_name);
        let ckb_id = lip_uri(path, &qualified_name);
        Self {
            raw,
            ckb_id: Some(ckb_id),
            symbol_name,
            qualified_name: Some(qualified_name),
            kind,
            line_start,
            confidence: 30,
            doc_comment,
        }
    }
}

fn unqualified(name: &str) -> Option<String> {
    let s = name.split('.').last().unwrap_or(name);
    if s.is_empty() {
        None
    } else {
        Some(s.to_string())
    }
}

fn lip_uri(path: &str, qualified: &str) -> String {
    let norm = path.trim_start_matches("./").trim_start_matches('/');
    format!("lip://local/{}#{}", norm, qualified)
}

// ---------------------------------------------------------------------------
// Scope tracker — brace-depth based (for {}-delimited languages)
// ---------------------------------------------------------------------------

struct ScopeTracker {
    stack: Vec<(String, usize)>, // (scope_name, depth_when_opened)
    depth: usize,
}

impl ScopeTracker {
    fn new() -> Self {
        Self {
            stack: Vec::new(),
            depth: 0,
        }
    }

    fn current(&self) -> Option<&str> {
        self.stack.last().map(|(n, _)| n.as_str())
    }

    fn qualify(&self, name: &str) -> String {
        match self.current() {
            Some(s) if !s.is_empty() => format!("{}.{}", s, name),
            _ => name.to_string(),
        }
    }

    /// Process a line, optionally pushing a new scope name.
    fn update(&mut self, line: &str, new_scope: Option<String>) {
        let opens = line.chars().filter(|&c| c == '{').count();
        let closes = line.chars().filter(|&c| c == '}').count();

        if let Some(name) = new_scope {
            if opens > closes {
                self.stack.push((name, self.depth));
            }
        }

        self.depth = self.depth.saturating_add(opens).saturating_sub(closes);

        while matches!(self.stack.last(), Some((_, ed)) if *ed >= self.depth) {
            self.stack.pop();
        }
    }
}

// ---------------------------------------------------------------------------
// Doc comment helpers
// ---------------------------------------------------------------------------

fn take_doc(buf: &mut Vec<String>) -> Option<String> {
    if buf.is_empty() {
        return None;
    }
    let text = buf.join(" ");
    buf.clear();
    let t = text.trim().to_string();
    if t.is_empty() {
        None
    } else {
        Some(t)
    }
}

fn strip_doc_marker(line: &str) -> String {
    let t = line.trim();
    for prefix in &["///", "//!", "//", "#", "/**", "*/", "* "] {
        if let Some(rest) = t.strip_prefix(prefix) {
            return rest.trim().to_string();
        }
    }
    t.trim_start_matches('*').trim().to_string()
}

// ---------------------------------------------------------------------------
// Detail level
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum DetailLevel {
    Minimal,
    Standard,
    Extended,
}

// ---------------------------------------------------------------------------
// MappedFile
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MappedFile {
    pub path: String,
    pub imports: Vec<String>,
    pub signatures: Vec<Signature>,
    pub docstrings: Option<Vec<String>>,
    pub parameters: Option<Vec<String>>,
    pub return_types: Option<Vec<String>>,
}

impl MappedFile {
    pub fn new(path: String, imports: Vec<String>, signatures: Vec<Signature>) -> Self {
        Self {
            path,
            imports,
            signatures,
            docstrings: None,
            parameters: None,
            return_types: None,
        }
    }

    pub fn from_minimal(path: String, imports: Vec<String>) -> Self {
        Self {
            path,
            imports,
            signatures: Vec::new(),
            docstrings: None,
            parameters: None,
            return_types: None,
        }
    }

    pub fn with_signatures(mut self, signatures: Vec<Signature>) -> Self {
        self.signatures = signatures;
        self
    }

    pub fn with_docstrings(mut self, docstrings: Vec<String>) -> Self {
        self.docstrings = Some(docstrings);
        self
    }

    pub fn with_parameters(mut self, parameters: Vec<String>) -> Self {
        self.parameters = Some(parameters);
        self
    }

    pub fn with_return_types(mut self, return_types: Vec<String>) -> Self {
        self.return_types = Some(return_types);
        self
    }

    pub fn format(&self) -> String {
        let mut out = String::new();
        if !self.imports.is_empty() {
            for imp in &self.imports {
                out.push_str(imp);
                out.push('\n');
            }
            out.push('\n');
        }
        for sig in &self.signatures {
            out.push_str(&sig.raw);
            out.push_str(" // ...\n");
        }
        out
    }

    pub fn to_ai_lang(&self, detail_level: DetailLevel) -> String {
        let mut out = String::new();
        out.push_str(&format!("({})\n", self.path));

        if !self.imports.is_empty() {
            let imports: Vec<String> = self
                .imports
                .iter()
                .map(|i| {
                    let parts: Vec<&str> = i.split_whitespace().collect();
                    parts
                        .last()
                        .map(|s| s.trim_matches(';'))
                        .unwrap_or(i)
                        .to_string()
                })
                .collect();
            out.push_str(&format!(" (imports: [{}])\n", imports.join(", ")));
        }

        match detail_level {
            DetailLevel::Minimal => {
                if !self.signatures.is_empty() {
                    let sigs: Vec<String> = self
                        .signatures
                        .iter()
                        .map(|s| {
                            let trimmed = s.raw.trim();
                            let without_body =
                                trimmed.split('{').next().unwrap_or(trimmed).trim();
                            without_body
                                .replace("pub ", "")
                                .replace("private ", "")
                                .replace("async ", "")
                                .replace("function ", "fn ")
                                .replace("def ", "fn ")
                                .replace("interface ", "if ")
                        })
                        .collect();
                    out.push_str(&format!(" (sigs: {})\n", sigs.join(", ")));
                }
            }
            DetailLevel::Standard => {
                if !self.signatures.is_empty() {
                    out.push_str(" (exports:\n");
                    for sig in &self.signatures {
                        let simplified = sig
                            .raw
                            .replace("pub ", "")
                            .replace("private ", "")
                            .replace("protected ", "");
                        out.push_str(&format!(
                            "  {} [{}]\n",
                            simplified,
                            sig.ckb_id.as_deref().unwrap_or("?")
                        ));
                    }
                    out.push_str(" )\n");
                }
            }
            DetailLevel::Extended => {
                if !self.signatures.is_empty() {
                    out.push_str(" (exports:\n");
                    for sig in &self.signatures {
                        if let Some(doc) = &sig.doc_comment {
                            out.push_str(&format!("  // {}\n", doc));
                        }
                        out.push_str(&format!(
                            "  {} [{:?}@L{}|{}]\n",
                            sig.raw,
                            sig.kind,
                            sig.line_start,
                            sig.ckb_id.as_deref().unwrap_or("?")
                        ));
                    }
                    out.push_str(" )\n");
                }
                if let Some(ref docs) = self.docstrings {
                    if !docs.is_empty() {
                        out.push_str(&format!(" (doc: {})\n", docs[0]));
                    }
                }
            }
        }

        out
    }
}

// ---------------------------------------------------------------------------
// DirectorySummary
// ---------------------------------------------------------------------------

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DirectorySummary {
    pub path: String,
    pub file_count: usize,
    pub signature_count: usize,
    pub description: Option<String>,
    pub modules: Vec<String>,
}

pub fn summarize_directory(files: &[&MappedFile], root_path: &str) -> DirectorySummary {
    let mut file_count = 0;
    let mut signature_count = 0;
    let mut modules = Vec::new();

    for file in files {
        file_count += 1;
        signature_count += file.signatures.len();
        modules.push(file.path.clone());
    }

    let description = find_directory_description(files, root_path);

    DirectorySummary {
        path: root_path.to_string(),
        file_count,
        signature_count,
        description,
        modules,
    }
}

fn find_directory_description(files: &[&MappedFile], _root_path: &str) -> Option<String> {
    for file in files {
        let path_lower = file.path.to_lowercase();
        if path_lower.contains("readme")
            || path_lower.contains("mod.rs")
            || path_lower.contains("index.js")
            || path_lower.contains("index.ts")
        {
            if let Some(ref sigs) = file.docstrings {
                if !sigs.is_empty() {
                    return Some(sigs[0].clone());
                }
            }
        }
    }
    None
}

// ---------------------------------------------------------------------------
// Dispatcher
// ---------------------------------------------------------------------------

pub fn extract_skeleton(path: &Path, content: &str) -> MappedFile {
    let ext = path
        .extension()
        .and_then(|e| e.to_str())
        .unwrap_or("")
        .to_lowercase();

    let rel_path = path.to_string_lossy().replace('\\', "/");

    // Run regex extraction first to get imports (tree-sitter doesn't extract imports).
    let mut mapped = match ext.as_str() {
        "js" | "jsx" | "ts" | "tsx" | "mjs" | "cjs" => extract_js_ts(rel_path, content),
        "rs" => extract_rust(rel_path, content),
        "py" => extract_python(rel_path, content),
        "go" => extract_go(rel_path, content),
        "java" | "kt" | "scala" => extract_java_like(rel_path, content),
        "c" | "cpp" | "cc" | "cxx" | "h" | "hpp" => extract_c_cpp(rel_path, content),
        "rb" => extract_ruby(rel_path, content),
        "php" => extract_php(rel_path, content),
        "md" | "txt" | "json" | "yaml" | "yml" | "toml" | "xml" | "html" | "css" | "scss"
        | "less" | "svg" | "lock" => {
            return MappedFile {
                path: path.to_string_lossy().replace('\\', "/"),
                imports: Vec::new(),
                signatures: Vec::new(),
                docstrings: None,
                parameters: None,
                return_types: None,
            }
        }
        _ => extract_generic(path.to_string_lossy().replace('\\', "/"), content),
    };

    // Upgrade to tree-sitter (Tier 2, confidence=60) for supported languages.
    // Tree-sitter replaces signatures; also replaces imports when non-empty.
    if let Some(ts_out) = crate::extractor::ts_extract(path, content) {
        mapped.signatures = ts_out.signatures;
        if !ts_out.imports.is_empty() {
            mapped.imports = ts_out.imports;
        }
    }

    mapped
}

// ---------------------------------------------------------------------------
// Rust
// ---------------------------------------------------------------------------

fn extract_rust(path: String, content: &str) -> MappedFile {
    let import_re = Regex::new(r"^(?:use\s+.+;|mod\s+\w+;|extern\s+crate\s+\w+;)").unwrap();

    // Scope opener: impl blocks — extract the implementing type name.
    let impl_re =
        Regex::new(r"^(?:pub(?:\([^)]+\))?\s+)?impl(?:<[^>]+>)?\s+(?:\w+\s+for\s+)?(\w+)")
            .unwrap();

    // Per-kind patterns: (regex, SymbolKind, also_opens_scope)
    // Checked in priority order; first match wins.
    struct RustPat {
        re: Regex,
        kind: SymbolKind,
        scope: bool,
    }
    let pats: Vec<RustPat> = vec![
        RustPat {
            re: Regex::new(r"^(?:pub(?:\([^)]+\))?\s+)?trait\s+(\w+)").unwrap(),
            kind: SymbolKind::Interface,
            scope: true,
        },
        RustPat {
            re: Regex::new(r"^(?:pub(?:\([^)]+\))?\s+)?struct\s+(\w+)").unwrap(),
            kind: SymbolKind::Struct,
            scope: false,
        },
        RustPat {
            re: Regex::new(r"^(?:pub(?:\([^)]+\))?\s+)?enum\s+(\w+)").unwrap(),
            kind: SymbolKind::Enum,
            scope: false,
        },
        RustPat {
            re: Regex::new(r"^(?:pub(?:\([^)]+\))?\s+)?type\s+(\w+)\s*=").unwrap(),
            kind: SymbolKind::TypeAlias,
            scope: false,
        },
        RustPat {
            re: Regex::new(r"^(?:pub(?:\([^)]+\))?\s+)?(?:async\s+)?fn\s+(\w+)").unwrap(),
            kind: SymbolKind::Function, // upgraded to Method below if in scope
            scope: false,
        },
        RustPat {
            re: Regex::new(r"^(?:pub(?:\([^)]+\))?\s+)?const\s+(\w+)\s*:").unwrap(),
            kind: SymbolKind::Variable,
            scope: false,
        },
        RustPat {
            re: Regex::new(r"^(?:pub(?:\([^)]+\))?\s+)?static\s+(\w+)\s*:").unwrap(),
            kind: SymbolKind::Variable,
            scope: false,
        },
        RustPat {
            re: Regex::new(r"^macro_rules!\s+(\w+)").unwrap(),
            kind: SymbolKind::Macro,
            scope: false,
        },
    ];

    let mut imports = Vec::new();
    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();
    let mut scope = ScopeTracker::new();
    let mut file_doc: Option<String> = None;
    let mut pre_code = true; // still in the file header comment zone

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();

        if trimmed.is_empty() {
            doc_buf.clear();
            scope.update(line, None);
            continue;
        }

        // Module-level doc comments (//!)
        if trimmed.starts_with("//!") {
            if pre_code && file_doc.is_none() {
                file_doc = Some(strip_doc_marker(trimmed));
            }
            doc_buf.clear();
            scope.update(line, None);
            continue;
        }

        // Item-level doc comments (///)
        if trimmed.starts_with("///") {
            doc_buf.push(strip_doc_marker(trimmed));
            scope.update(line, None);
            continue;
        }

        // Other comments — don't add to doc_buf
        if trimmed.starts_with("//") || trimmed.starts_with("/*") {
            doc_buf.clear();
            scope.update(line, None);
            continue;
        }

        pre_code = false;

        // Imports
        if import_re.is_match(trimmed) {
            imports.push(trimmed.to_string());
            doc_buf.clear();
            scope.update(line, None);
            continue;
        }

        // impl blocks — scope opener, emit as Class
        if let Some(caps) = impl_re.captures(trimmed) {
            let type_name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                raw,
                SymbolKind::Class,
                line_idx,
                &path,
                type_name.clone(),
                doc,
            ));
            scope.update(line, Some(type_name));
            continue;
        }

        // Per-kind patterns
        let mut matched = false;
        for pat in &pats {
            if let Some(caps) = pat.re.captures(trimmed) {
                let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
                let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
                let qualified = scope.qualify(&name);
                let mut kind = pat.kind;
                // fn inside an impl scope → Method
                if kind == SymbolKind::Function && scope.current().is_some() {
                    kind = SymbolKind::Method;
                }
                let doc = take_doc(&mut doc_buf);
                signatures.push(Signature::new(raw, kind, line_idx, &path, qualified, doc));
                if pat.scope {
                    scope.update(line, Some(name));
                } else {
                    scope.update(line, None);
                }
                matched = true;
                break;
            }
        }

        if !matched {
            doc_buf.clear();
            scope.update(line, None);
        }
    }

    MappedFile {
        path,
        imports,
        signatures,
        docstrings: file_doc.map(|d| vec![d]),
        parameters: None,
        return_types: None,
    }
}

// ---------------------------------------------------------------------------
// JavaScript / TypeScript
// ---------------------------------------------------------------------------

fn extract_js_ts(path: String, content: &str) -> MappedFile {
    let import_re = Regex::new(
        r"^(?:import\s+.+|export\s+\{[^}]+\}\s+from\s+.+|export\s+\*\s+from\s+.+|const\s+\w+\s*=\s*require\(.+\))",
    )
    .unwrap();

    let class_re = Regex::new(r"^(?:export\s+(?:default\s+)?)?class\s+(\w+)").unwrap();
    let interface_re = Regex::new(r"^(?:export\s+(?:default\s+)?)?interface\s+(\w+)").unwrap();
    let type_re = Regex::new(r"^(?:export\s+(?:default\s+)?)?type\s+(\w+)\s*=").unwrap();

    struct JsPat {
        re: Regex,
        kind: SymbolKind,
    }
    let fn_pats: Vec<JsPat> = vec![
        JsPat {
            re: Regex::new(
                r"^(?:export\s+(?:default\s+)?)?(?:async\s+)?function\s+(\w+)",
            )
            .unwrap(),
            kind: SymbolKind::Function,
        },
        JsPat {
            re: Regex::new(
                r"^(?:export\s+(?:default\s+)?)?const\s+(\w+)\s*(?::\s*[^=]+)?\s*=\s*(?:async\s+)?\(",
            )
            .unwrap(),
            kind: SymbolKind::Function,
        },
        JsPat {
            re: Regex::new(r"^(?:export\s+(?:default\s+)?)?const\s+(\w+)\s*:").unwrap(),
            kind: SymbolKind::Variable,
        },
    ];

    let mut imports = Vec::new();
    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();
    let mut scope = ScopeTracker::new();
    let mut in_block_comment = false;
    let mut file_doc_buf: Vec<String> = Vec::new();
    let mut pre_code = true; // still in the file-header comment zone

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();

        if trimmed.is_empty() {
            if !in_block_comment {
                doc_buf.clear();
            }
            scope.update(line, None);
            continue;
        }

        if in_block_comment {
            if trimmed.contains("*/") {
                in_block_comment = false;
            } else {
                let stripped = strip_doc_marker(trimmed);
                doc_buf.push(stripped.clone());
                if pre_code {
                    file_doc_buf.push(stripped);
                }
            }
            scope.update(line, None);
            continue;
        }

        if trimmed.starts_with("/**") {
            in_block_comment = !trimmed.contains("*/");
            let stripped = strip_doc_marker(trimmed);
            doc_buf.push(stripped.clone());
            if pre_code {
                file_doc_buf.push(stripped);
            }
            scope.update(line, None);
            continue;
        }

        if trimmed.starts_with("//") {
            let stripped = strip_doc_marker(trimmed);
            doc_buf.push(stripped.clone());
            if pre_code {
                file_doc_buf.push(stripped);
            }
            scope.update(line, None);
            continue;
        }

        pre_code = false;

        if import_re.is_match(trimmed) {
            imports.push(trimmed.to_string());
            doc_buf.clear();
            scope.update(line, None);
            continue;
        }

        // class
        if let Some(caps) = class_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                raw,
                SymbolKind::Class,
                line_idx,
                &path,
                name.clone(),
                doc,
            ));
            scope.update(line, Some(name));
            continue;
        }

        // interface
        if let Some(caps) = interface_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                raw,
                SymbolKind::Interface,
                line_idx,
                &path,
                name.clone(),
                doc,
            ));
            scope.update(line, Some(name));
            continue;
        }

        // type alias
        if let Some(caps) = type_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                trimmed.to_string(),
                SymbolKind::TypeAlias,
                line_idx,
                &path,
                scope.qualify(&name),
                doc,
            ));
            scope.update(line, None);
            continue;
        }

        // functions / arrow functions / variables
        let mut matched = false;
        for pat in &fn_pats {
            if let Some(caps) = pat.re.captures(trimmed) {
                let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
                let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
                let qualified = scope.qualify(&name);
                let kind = if pat.kind == SymbolKind::Function && scope.current().is_some() {
                    SymbolKind::Method
                } else {
                    pat.kind
                };
                let doc = take_doc(&mut doc_buf);
                signatures.push(Signature::new(raw, kind, line_idx, &path, qualified, doc));
                scope.update(line, None);
                matched = true;
                break;
            }
        }

        if !matched {
            doc_buf.clear();
            scope.update(line, None);
        }
    }

    let file_docstring = if file_doc_buf.is_empty() {
        None
    } else {
        Some(vec![file_doc_buf.join(" ")])
    };

    MappedFile {
        path,
        imports,
        signatures,
        docstrings: file_docstring,
        parameters: None,
        return_types: None,
    }
}

// ---------------------------------------------------------------------------
// Python
// ---------------------------------------------------------------------------

fn extract_python(path: String, content: &str) -> MappedFile {
    let import_re = Regex::new(r"^(?:import\s+.+|from\s+.+\s+import\s+.+)").unwrap();
    let class_re = Regex::new(r"^class\s+(\w+)").unwrap();
    let def_re = Regex::new(r"^(?:async\s+)?def\s+(\w+)\s*\(([^)]*)").unwrap();
    let decorator_re = Regex::new(r"^@\w+(?:\([^)]*\))?").unwrap();

    let mut imports = Vec::new();
    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();
    // (class_name, indent_of_class_keyword)
    let mut current_class: Option<(String, usize)> = None;

    // Collect module-level docstring (triple-quoted string before any imports/defs/classes).
    let mut module_docstring: Option<String> = None;
    {
        let mut lines_iter = content.lines().peekable();
        // skip shebang and encoding lines
        while let Some(l) = lines_iter.peek() {
            let t = l.trim();
            if t.starts_with("#!") || t.starts_with("# -*-") || t.starts_with("# coding") || t.is_empty() {
                lines_iter.next();
            } else {
                break;
            }
        }
        if let Some(first) = lines_iter.peek() {
            let t = first.trim();
            let quote = if t.starts_with("\"\"\"") {
                Some("\"\"\"")
            } else if t.starts_with("'''") {
                Some("'''")
            } else {
                None
            };
            if let Some(q) = quote {
                let mut buf = Vec::new();
                let first_line = lines_iter.next().unwrap().trim().to_string();
                let inner = first_line.trim_start_matches(q);
                // Single-line docstring: ends on the same line
                if let Some(end) = inner.find(q) {
                    module_docstring = Some(inner[..end].trim().to_string());
                } else {
                    buf.push(inner.trim().to_string());
                    for l in lines_iter.by_ref() {
                        let t = l.trim();
                        if let Some(end) = t.find(q) {
                            buf.push(t[..end].trim().to_string());
                            break;
                        } else {
                            buf.push(t.to_string());
                        }
                    }
                    module_docstring = Some(buf.into_iter().filter(|s| !s.is_empty()).collect::<Vec<_>>().join(" "));
                }
            }
        }
    }

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();
        let indent = line.len() - trimmed.len();

        if trimmed.is_empty() {
            doc_buf.clear();
            continue;
        }

        // Doc comment
        if trimmed.starts_with('#') {
            doc_buf.push(strip_doc_marker(trimmed));
            continue;
        }

        // Exit class scope when we return to class indent level or below
        if let Some((_, class_indent)) = &current_class {
            if indent <= *class_indent && !trimmed.starts_with("class ") {
                current_class = None;
            }
        }

        // Import
        if import_re.is_match(trimmed) {
            imports.push(trimmed.to_string());
            doc_buf.clear();
            continue;
        }

        // Decorator — keep in doc_buf as context
        if decorator_re.is_match(trimmed) {
            doc_buf.push(trimmed.to_string());
            continue;
        }

        // Class
        if let Some(caps) = class_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.trim_end_matches(':').to_string();
            let doc = take_doc(&mut doc_buf);
            current_class = Some((name.clone(), indent));
            signatures.push(Signature::new(
                raw,
                SymbolKind::Class,
                line_idx,
                &path,
                name,
                doc,
            ));
            continue;
        }

        // def
        if let Some(caps) = def_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let params = caps.get(2).map(|m| m.as_str()).unwrap_or("");
            let raw = trimmed.trim_end_matches(':').to_string();
            let is_method = params.split(',').next().map(|p| {
                let p = p.trim();
                p == "self" || p == "cls" || p.starts_with("self:") || p.starts_with("cls:")
            });
            let (kind, qualified) = match (&current_class, is_method) {
                (Some((cls, _)), Some(true)) => {
                    (SymbolKind::Method, format!("{}.{}", cls, name))
                }
                _ => (SymbolKind::Function, name.clone()),
            };
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(raw, kind, line_idx, &path, qualified, doc));
            continue;
        }

        doc_buf.clear();
    }

    MappedFile {
        path,
        imports,
        signatures,
        docstrings: module_docstring.map(|d| vec![d]),
        parameters: None,
        return_types: None,
    }
}

// ---------------------------------------------------------------------------
// Go
// ---------------------------------------------------------------------------

fn extract_go(path: String, content: &str) -> MappedFile {
    let import_re = Regex::new(r#"^import\s+(?:\(|"[^"]+")"#).unwrap();
    // method: func (recv Type) Name(...)
    let method_re =
        Regex::new(r"^func\s+\(\s*\w+\s+\*?(\w+)[^)]*\)\s+(\w+)\s*\(").unwrap();
    // free function: func Name(...)
    let fn_re = Regex::new(r"^func\s+(\w+)\s*\(").unwrap();

    struct GoPat {
        re: Regex,
        kind: SymbolKind,
    }
    let type_pats: Vec<GoPat> = vec![
        GoPat {
            re: Regex::new(r"^type\s+(\w+)\s+struct").unwrap(),
            kind: SymbolKind::Struct,
        },
        GoPat {
            re: Regex::new(r"^type\s+(\w+)\s+interface").unwrap(),
            kind: SymbolKind::Interface,
        },
        GoPat {
            re: Regex::new(r"^type\s+(\w+)\s+=?\s*\w+").unwrap(),
            kind: SymbolKind::TypeAlias,
        },
        GoPat {
            re: Regex::new(r"^var\s+(\w+)\s+").unwrap(),
            kind: SymbolKind::Variable,
        },
        GoPat {
            re: Regex::new(r"^const\s+(\w+)\s+").unwrap(),
            kind: SymbolKind::Variable,
        },
    ];

    let mut imports = Vec::new();
    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();

        if trimmed.is_empty() {
            doc_buf.clear();
            continue;
        }

        if trimmed.starts_with("//") {
            doc_buf.push(strip_doc_marker(trimmed));
            continue;
        }

        if import_re.is_match(trimmed) {
            imports.push(trimmed.to_string());
            doc_buf.clear();
            continue;
        }

        // method with receiver
        if let Some(caps) = method_re.captures(trimmed) {
            let receiver = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let name = caps.get(2).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let qualified = format!("{}.{}", receiver, name);
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                raw,
                SymbolKind::Method,
                line_idx,
                &path,
                qualified,
                doc,
            ));
            continue;
        }

        // free function
        if let Some(caps) = fn_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                raw,
                SymbolKind::Function,
                line_idx,
                &path,
                name,
                doc,
            ));
            continue;
        }

        // type declarations, var, const
        let mut matched = false;
        for pat in &type_pats {
            if let Some(caps) = pat.re.captures(trimmed) {
                let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
                let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
                let doc = take_doc(&mut doc_buf);
                signatures.push(Signature::new(raw, pat.kind, line_idx, &path, name, doc));
                matched = true;
                break;
            }
        }

        if !matched {
            doc_buf.clear();
        }
    }

    MappedFile {
        path,
        imports,
        signatures,
        docstrings: None,
        parameters: None,
        return_types: None,
    }
}

// ---------------------------------------------------------------------------
// Java / Kotlin / Scala
// ---------------------------------------------------------------------------

fn extract_java_like(path: String, content: &str) -> MappedFile {
    let import_re = Regex::new(r"^(?:import\s+.+;|package\s+.+;)").unwrap();
    let class_re =
        Regex::new(r"^(?:(?:public|private|protected|abstract|final|sealed)\s+)*(?:class|record)\s+(\w+)").unwrap();
    let interface_re =
        Regex::new(r"^(?:(?:public|private|protected)\s+)*interface\s+(\w+)").unwrap();
    // Kotlin
    let kt_fn_re = Regex::new(r"^(?:(?:public|private|protected|override|suspend)\s+)*fun\s+(\w+)").unwrap();
    // Java method: return_type name(
    let method_re = Regex::new(
        r"^(?:(?:public|private|protected|static|final|abstract|synchronized|native|default)\s+)*\w+(?:<[^>]+>)?\s+(\w+)\s*\(",
    )
    .unwrap();
    let annotation_re = Regex::new(r"^@\w+(?:\([^)]*\))?").unwrap();

    let mut imports = Vec::new();
    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();
    let mut scope = ScopeTracker::new();
    let mut in_block_comment = false;

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();

        if trimmed.is_empty() {
            if !in_block_comment {
                doc_buf.clear();
            }
            scope.update(line, None);
            continue;
        }

        if in_block_comment {
            if trimmed.contains("*/") {
                in_block_comment = false;
            } else {
                doc_buf.push(strip_doc_marker(trimmed));
            }
            scope.update(line, None);
            continue;
        }

        if trimmed.starts_with("/**") {
            in_block_comment = !trimmed.contains("*/");
            doc_buf.push(strip_doc_marker(trimmed));
            scope.update(line, None);
            continue;
        }

        if trimmed.starts_with("//") {
            doc_buf.push(strip_doc_marker(trimmed));
            scope.update(line, None);
            continue;
        }

        if import_re.is_match(trimmed) {
            imports.push(trimmed.to_string());
            doc_buf.clear();
            scope.update(line, None);
            continue;
        }

        if annotation_re.is_match(trimmed) {
            doc_buf.push(trimmed.to_string());
            scope.update(line, None);
            continue;
        }

        if let Some(caps) = class_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                raw,
                SymbolKind::Class,
                line_idx,
                &path,
                name.clone(),
                doc,
            ));
            scope.update(line, Some(name));
            continue;
        }

        if let Some(caps) = interface_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                raw,
                SymbolKind::Interface,
                line_idx,
                &path,
                name.clone(),
                doc,
            ));
            scope.update(line, Some(name));
            continue;
        }

        // Kotlin fun
        if let Some(caps) = kt_fn_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let qualified = scope.qualify(&name);
            let kind = if scope.current().is_some() {
                SymbolKind::Method
            } else {
                SymbolKind::Function
            };
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(raw, kind, line_idx, &path, qualified, doc));
            scope.update(line, None);
            continue;
        }

        // Java method
        if let Some(caps) = method_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            // Filter out control-flow keywords that can match
            if !matches!(
                name.as_str(),
                "if" | "for" | "while" | "switch" | "catch" | "return" | "new"
            ) {
                let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
                let qualified = scope.qualify(&name);
                let kind = if scope.current().is_some() {
                    SymbolKind::Method
                } else {
                    SymbolKind::Function
                };
                let doc = take_doc(&mut doc_buf);
                signatures.push(Signature::new(raw, kind, line_idx, &path, qualified, doc));
                scope.update(line, None);
                continue;
            }
        }

        doc_buf.clear();
        scope.update(line, None);
    }

    MappedFile {
        path,
        imports,
        signatures,
        docstrings: None,
        parameters: None,
        return_types: None,
    }
}

// ---------------------------------------------------------------------------
// C / C++
// ---------------------------------------------------------------------------

fn extract_c_cpp(path: String, content: &str) -> MappedFile {
    let import_re = Regex::new(r#"^#include\s+[<"][^>"]+[>"]"#).unwrap();
    let class_re = Regex::new(r"^(?:class|struct)\s+(\w+)[^;]*$").unwrap();
    let enum_re = Regex::new(r"^enum\s+(?:class\s+)?(\w+)").unwrap();
    let ns_re = Regex::new(r"^namespace\s+(\w+)").unwrap();
    let typedef_re = Regex::new(r"^typedef\s+.+\s+(\w+)\s*;").unwrap();
    let using_re = Regex::new(r"^using\s+(\w+)\s*=").unwrap();
    let define_re = Regex::new(r"^#define\s+(\w+)").unwrap();
    let fn_re = Regex::new(
        r"^(?:(?:static|inline|virtual|explicit|constexpr|override|const)\s+)*(?:\w+(?:::\w+)*(?:<[^>]+>)?[\s*&]+)+(\w+)\s*\(",
    )
    .unwrap();
    let template_re = Regex::new(r"^template\s*<[^>]+>").unwrap();

    let mut imports = Vec::new();
    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();
    let mut scope = ScopeTracker::new();
    let mut in_block_comment = false;

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();

        if trimmed.is_empty() {
            if !in_block_comment {
                doc_buf.clear();
            }
            scope.update(line, None);
            continue;
        }

        if in_block_comment {
            if trimmed.contains("*/") {
                in_block_comment = false;
            } else {
                doc_buf.push(strip_doc_marker(trimmed));
            }
            scope.update(line, None);
            continue;
        }

        if trimmed.starts_with("/**") || trimmed.starts_with("/*") {
            in_block_comment = !trimmed.contains("*/");
            doc_buf.push(strip_doc_marker(trimmed));
            scope.update(line, None);
            continue;
        }

        if trimmed.starts_with("//") {
            doc_buf.push(strip_doc_marker(trimmed));
            scope.update(line, None);
            continue;
        }

        if import_re.is_match(trimmed) {
            imports.push(trimmed.to_string());
            doc_buf.clear();
            scope.update(line, None);
            continue;
        }

        if template_re.is_match(trimmed) {
            // Keep doc_buf, next line is usually the function/class
            scope.update(line, None);
            continue;
        }

        if define_re.is_match(trimmed) {
            if let Some(caps) = define_re.captures(trimmed) {
                let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
                let doc = take_doc(&mut doc_buf);
                signatures.push(Signature::new(
                    trimmed.to_string(),
                    SymbolKind::Macro,
                    line_idx,
                    &path,
                    name,
                    doc,
                ));
            }
            scope.update(line, None);
            continue;
        }

        if let Some(caps) = ns_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                raw,
                SymbolKind::Namespace,
                line_idx,
                &path,
                name.clone(),
                doc,
            ));
            scope.update(line, Some(name));
            continue;
        }

        if let Some(caps) = class_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let kind = if trimmed.starts_with("struct") {
                SymbolKind::Struct
            } else {
                SymbolKind::Class
            };
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(raw, kind, line_idx, &path, scope.qualify(&name), doc));
            scope.update(line, Some(name));
            continue;
        }

        if let Some(caps) = enum_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                raw,
                SymbolKind::Enum,
                line_idx,
                &path,
                scope.qualify(&name),
                doc,
            ));
            scope.update(line, None);
            continue;
        }

        if let Some(caps) = typedef_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                trimmed.to_string(),
                SymbolKind::TypeAlias,
                line_idx,
                &path,
                name,
                doc,
            ));
            scope.update(line, None);
            continue;
        }

        if let Some(caps) = using_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                trimmed.to_string(),
                SymbolKind::TypeAlias,
                line_idx,
                &path,
                name,
                doc,
            ));
            scope.update(line, None);
            continue;
        }

        // Function / method — ends with `;` is a declaration, `{` is definition
        if trimmed.ends_with('{') || trimmed.ends_with(';') {
            if let Some(caps) = fn_re.captures(trimmed) {
                let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
                if !name.is_empty()
                    && !matches!(
                        name.as_str(),
                        "if" | "for" | "while" | "switch" | "return" | "else"
                    )
                {
                    let raw = trimmed.trim_end_matches('{').trim().to_string();
                    let qualified = scope.qualify(&name);
                    let kind = if scope.current().is_some() {
                        SymbolKind::Method
                    } else {
                        SymbolKind::Function
                    };
                    let doc = take_doc(&mut doc_buf);
                    signatures.push(Signature::new(raw, kind, line_idx, &path, qualified, doc));
                    scope.update(line, None);
                    continue;
                }
            }
        }

        doc_buf.clear();
        scope.update(line, None);
    }

    MappedFile {
        path,
        imports,
        signatures,
        docstrings: None,
        parameters: None,
        return_types: None,
    }
}

// ---------------------------------------------------------------------------
// Ruby
// ---------------------------------------------------------------------------

fn extract_ruby(path: String, content: &str) -> MappedFile {
    let import_re =
        Regex::new(r"^(?:require\s+.+|require_relative\s+.+|include\s+\w+|extend\s+\w+)").unwrap();
    let class_re = Regex::new(r"^(?:class|module)\s+(\w+)").unwrap();
    let def_re = Regex::new(r"^def\s+(?:self\.)?(\w+)").unwrap();
    let attr_re = Regex::new(r"^attr_(?:reader|writer|accessor)\s+(.+)").unwrap();

    let mut imports = Vec::new();
    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();
    // Track class scope via end-keyword counting
    let mut current_class: Option<String> = None;
    let mut scope_depth: usize = 0; // def/class/module/do increments, end decrements

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();

        if trimmed.is_empty() {
            doc_buf.clear();
            continue;
        }

        if trimmed.starts_with('#') {
            doc_buf.push(strip_doc_marker(trimmed));
            continue;
        }

        if import_re.is_match(trimmed) {
            imports.push(trimmed.to_string());
            doc_buf.clear();
            continue;
        }

        // Track end keywords for scope depth
        if trimmed == "end" {
            if scope_depth > 0 {
                scope_depth -= 1;
            }
            if scope_depth == 0 {
                current_class = None;
            }
            doc_buf.clear();
            continue;
        }

        if let Some(caps) = class_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let kind = if trimmed.starts_with("module") {
                SymbolKind::Namespace
            } else {
                SymbolKind::Class
            };
            let doc = take_doc(&mut doc_buf);
            current_class = Some(name.clone());
            scope_depth += 1;
            signatures.push(Signature::new(trimmed.to_string(), kind, line_idx, &path, name, doc));
            continue;
        }

        if let Some(caps) = def_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let (kind, qualified) = match &current_class {
                Some(cls) => (SymbolKind::Method, format!("{}.{}", cls, name)),
                None => (SymbolKind::Function, name),
            };
            let doc = take_doc(&mut doc_buf);
            scope_depth += 1;
            signatures.push(Signature::new(trimmed.to_string(), kind, line_idx, &path, qualified, doc));
            continue;
        }

        if let Some(caps) = attr_re.captures(trimmed) {
            let names = caps.get(1).map(|m| m.as_str()).unwrap_or("");
            for raw_name in names.split(',') {
                let name = raw_name.trim().trim_start_matches(':').to_string();
                if name.is_empty() {
                    continue;
                }
                let qualified = match &current_class {
                    Some(cls) => format!("{}.{}", cls, name),
                    None => name.clone(),
                };
                let doc = take_doc(&mut doc_buf);
                signatures.push(Signature::new(
                    format!("attr {}", name),
                    SymbolKind::Field,
                    line_idx,
                    &path,
                    qualified,
                    doc,
                ));
            }
            continue;
        }

        // Count scope-opening keywords (do/if with blocks, begin, etc.)
        if trimmed.ends_with(" do") || trimmed.ends_with(" do |")
            || trimmed == "begin"
        {
            scope_depth += 1;
        }

        doc_buf.clear();
    }

    MappedFile {
        path,
        imports,
        signatures,
        docstrings: None,
        parameters: None,
        return_types: None,
    }
}

// ---------------------------------------------------------------------------
// PHP
// ---------------------------------------------------------------------------

fn extract_php(path: String, content: &str) -> MappedFile {
    let import_re = Regex::new(
        r"^(?:use\s+.+;|namespace\s+.+;|require(?:_once)?\s+.+;|include(?:_once)?\s+.+;)",
    )
    .unwrap();
    let class_re = Regex::new(r"^(?:abstract\s+)?class\s+(\w+)").unwrap();
    let interface_re = Regex::new(r"^interface\s+(\w+)").unwrap();
    let trait_re = Regex::new(r"^trait\s+(\w+)").unwrap();
    let fn_re = Regex::new(
        r"^(?:(?:public|private|protected|static|abstract|final)\s+)*function\s+(\w+)",
    )
    .unwrap();

    let mut imports = Vec::new();
    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();
    let mut scope = ScopeTracker::new();
    let mut in_block_comment = false;

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();

        if trimmed.is_empty() {
            if !in_block_comment {
                doc_buf.clear();
            }
            scope.update(line, None);
            continue;
        }

        if in_block_comment {
            if trimmed.contains("*/") {
                in_block_comment = false;
            } else {
                doc_buf.push(strip_doc_marker(trimmed));
            }
            scope.update(line, None);
            continue;
        }

        if trimmed.starts_with("/**") || trimmed.starts_with("/*") {
            in_block_comment = !trimmed.contains("*/");
            doc_buf.push(strip_doc_marker(trimmed));
            scope.update(line, None);
            continue;
        }

        if trimmed.starts_with("//") || trimmed.starts_with('#') {
            doc_buf.push(strip_doc_marker(trimmed));
            scope.update(line, None);
            continue;
        }

        if import_re.is_match(trimmed) {
            imports.push(trimmed.to_string());
            doc_buf.clear();
            scope.update(line, None);
            continue;
        }

        if let Some(caps) = class_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                raw,
                SymbolKind::Class,
                line_idx,
                &path,
                name.clone(),
                doc,
            ));
            scope.update(line, Some(name));
            continue;
        }

        if let Some(caps) = interface_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                raw,
                SymbolKind::Interface,
                line_idx,
                &path,
                name.clone(),
                doc,
            ));
            scope.update(line, Some(name));
            continue;
        }

        if let Some(caps) = trait_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                raw,
                SymbolKind::Interface,
                line_idx,
                &path,
                name.clone(),
                doc,
            ));
            scope.update(line, Some(name));
            continue;
        }

        if let Some(caps) = fn_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let qualified = scope.qualify(&name);
            let kind = if scope.current().is_some() {
                SymbolKind::Method
            } else {
                SymbolKind::Function
            };
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(raw, kind, line_idx, &path, qualified, doc));
            scope.update(line, None);
            continue;
        }

        doc_buf.clear();
        scope.update(line, None);
    }

    MappedFile {
        path,
        imports,
        signatures,
        docstrings: None,
        parameters: None,
        return_types: None,
    }
}

// ---------------------------------------------------------------------------
// Generic fallback
// ---------------------------------------------------------------------------

fn extract_generic(path: String, content: &str) -> MappedFile {
    let import_re = Regex::new(r"^(?:import|require|include|use)\s+.+").unwrap();
    let sig_re = Regex::new(
        r"^(?:function|def|fn|func|class|struct|interface|type|enum|trait|module)\s+(\w+)",
    )
    .unwrap();

    let mut imports = Vec::new();
    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();

        if trimmed.is_empty() {
            doc_buf.clear();
            continue;
        }

        if trimmed.starts_with("//") || trimmed.starts_with('#') {
            doc_buf.push(strip_doc_marker(trimmed));
            continue;
        }

        if import_re.is_match(trimmed) {
            imports.push(trimmed.to_string());
            doc_buf.clear();
            continue;
        }

        if let Some(caps) = sig_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(
                trimmed.to_string(),
                SymbolKind::Unknown,
                line_idx,
                &path,
                name,
                doc,
            ));
            continue;
        }

        doc_buf.clear();
    }

    MappedFile {
        path,
        imports,
        signatures,
        docstrings: None,
        parameters: None,
        return_types: None,
    }
}
