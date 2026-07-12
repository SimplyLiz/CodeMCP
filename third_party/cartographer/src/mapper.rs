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
/// `Struct` is a CodeCartographer extension; maps to `Class` in future LIP wire format.
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
    ConfigKey,
    Endpoint,
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
    /// Column byte offset (UTF-8) of this signature on its start line. 0-indexed.
    #[serde(default)]
    pub col_start: usize,
    /// 0-indexed end line (inclusive).
    #[serde(default)]
    pub line_end: usize,
    /// Column byte offset (UTF-8) of the end of this signature. 0-indexed, exclusive.
    #[serde(default)]
    pub col_end: usize,
    /// Confidence score (1–100). 30 = Tier 1 regex heuristic.
    #[serde(default = "default_confidence")]
    pub confidence: u8,
    /// Doc comment extracted from lines immediately preceding this signature.
    #[serde(default)]
    pub doc_comment: Option<String>,
    /// Compact field/variant list for Struct and Enum kinds, populated post-extraction.
    #[serde(default)]
    pub body: Option<String>,
    /// True when a test function whose name derives from this symbol's name exists.
    #[serde(default)]
    pub tested: bool,
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
            col_start: 0,
            line_end: 0,
            col_end: 0,
            confidence: 30,
            doc_comment,
            body: None,
            tested: false,
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
    /// "hot" (top-quartile churn) or "stable" (bottom-quartile), None otherwise.
    #[serde(default)]
    pub churn_label: Option<String>,
    /// Function names that carry `#[test]` (or are in `#[cfg(test)]`) in this file,
    /// collected before tree-sitter override so annotate_tested() can use them.
    #[serde(default)]
    pub inline_test_fns: Vec<String>,
}

impl MappedFile {
    #[allow(dead_code)]
    pub fn new(path: String, imports: Vec<String>, signatures: Vec<Signature>) -> Self {
        Self {
            path,
            imports,
            signatures,
            docstrings: None,
            parameters: None,
            return_types: None,
            churn_label: None,
            inline_test_fns: Vec::new(),
        }
    }

    #[allow(dead_code)]
    pub fn from_minimal(path: String, imports: Vec<String>) -> Self {
        Self {
            path,
            imports,
            signatures: Vec::new(),
            docstrings: None,
            parameters: None,
            return_types: None,
            churn_label: None,
            inline_test_fns: Vec::new(),
        }
    }

    #[allow(dead_code)]
    pub fn with_signatures(mut self, signatures: Vec<Signature>) -> Self {
        self.signatures = signatures;
        self
    }

    #[allow(dead_code)]
    pub fn with_docstrings(mut self, docstrings: Vec<String>) -> Self {
        self.docstrings = Some(docstrings);
        self
    }

    #[allow(dead_code)]
    pub fn with_parameters(mut self, parameters: Vec<String>) -> Self {
        self.parameters = Some(parameters);
        self
    }

    #[allow(dead_code)]
    pub fn with_return_types(mut self, return_types: Vec<String>) -> Self {
        self.return_types = Some(return_types);
        self
    }

    #[allow(dead_code)]
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
            if let Some(body) = &sig.body {
                // Strip a bare trailing `{` that tree-sitter may include in `raw`.
                let decl = sig.raw.trim_end_matches('{').trim_end();
                out.push_str(&format!("{} {{ {} }}\n", decl, body));
            } else if sig.tested {
                out.push_str(&format!("{} // tested\n", sig.raw));
            } else {
                out.push_str(&format!("{} // ...\n", sig.raw));
            }
        }
        out
    }

    #[allow(dead_code)]
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
// Type body extractor — collects fields/variants for Struct and Enum sigs.
// ---------------------------------------------------------------------------

/// Scans forward from `start_line` and returns a compact, semicolon-separated
/// list of field/variant lines found at brace-depth 1 (the body of the type).
/// Capped at 40 lines of lookahead to keep output token-tight.
fn extract_body_at_line(lines: &[&str], start_line: usize) -> Option<String> {
    let mut depth: usize = 0;
    let mut opened = false;
    let mut fields: Vec<String> = Vec::new();

    for line in lines.iter().skip(start_line).take(40) {
        let opens = line.chars().filter(|&c| c == '{').count();
        let closes = line.chars().filter(|&c| c == '}').count();

        depth = depth.saturating_add(opens).saturating_sub(closes);

        if !opened && opens > 0 {
            opened = true;
            // This is the declaration line (e.g. `pub struct Foo {`); skip it.
            continue;
        }

        if opened && depth == 1 {
            let t = line.trim();
            // Skip empty lines, comments, attributes, and bare punctuation.
            if !t.is_empty()
                && !t.starts_with("//")
                && !t.starts_with("#[")
                && t != "{"
                && t != ","
            {
                // Strip inline comments before storing; trim whitespace first so
                // trailing spaces don't block comma removal (e.g. `field: T, // comment`).
                let clean = if let Some(pos) = t.find("//") {
                    t[..pos].trim().trim_end_matches(',').trim()
                } else {
                    t.trim_end_matches(',')
                };
                if !clean.is_empty() {
                    fields.push(clean.to_string());
                }
            }
        }

        if opened && depth == 0 {
            break;
        }
    }

    if fields.is_empty() {
        None
    } else {
        Some(fields.join("; "))
    }
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
        "cs" => extract_csharp(rel_path, content),
        "swift" => extract_swift(rel_path, content),
        "lua" => extract_lua(rel_path, content),
        "sh" | "bash" | "zsh" | "fish" => extract_shell(rel_path, content),
        "sql" => extract_sql(rel_path, content),
        "md" | "markdown" => extract_markdown(rel_path, content),
        "yaml" | "yml" => extract_yaml(rel_path, content),
        "toml" => extract_toml(rel_path, content),
        "json" => extract_json(rel_path, content),
        "txt" | "xml" | "html" | "css" | "scss" | "less" | "svg" | "lock" => {
            return MappedFile {
                path: path.to_string_lossy().replace('\\', "/"),
                imports: Vec::new(),
                signatures: Vec::new(),
                docstrings: None,
                parameters: None,
                return_types: None,
                churn_label: None,
                inline_test_fns: Vec::new(),
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

    // Feature 1: enrich struct/enum signatures with compact field/variant lists.
    let all_lines: Vec<&str> = content.lines().collect();
    for sig in &mut mapped.signatures {
        if matches!(sig.kind, SymbolKind::Struct | SymbolKind::Enum) {
            sig.body = extract_body_at_line(&all_lines, sig.line_start);
        }
    }

    // Feature 2: collect inline #[test] function names (survives tree-sitter override).
    // We scan the raw content rather than the (possibly overwritten) signatures.
    {
        let fn_name_re = Regex::new(r"^(?:pub\s+)?(?:async\s+)?fn\s+(\w+)").unwrap();
        let mut next_is_test = false;
        let mut in_test_cfg = false;
        for line in content.lines() {
            let t = line.trim();
            if t == "#[cfg(test)]" || t.starts_with("#[cfg(test)") {
                in_test_cfg = true;
                continue;
            }
            if t == "#[test]" || t.starts_with("#[test,") || t.starts_with("#[test]") {
                next_is_test = true;
                continue;
            }
            if t.starts_with("#[") {
                // Other attribute — don't reset next_is_test so stacked attrs work.
                continue;
            }
            if next_is_test || in_test_cfg {
                if let Some(caps) = fn_name_re.captures(t) {
                    mapped.inline_test_fns.push(caps.get(1).unwrap().as_str().to_string());
                }
                next_is_test = false;
            }
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
    let mut attr_has_test = false; // preceding line was #[test]
    let mut in_test_mod = false;   // inside #[cfg(test)] mod
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

        // Attributes — track #[test] and #[cfg(test)]
        if trimmed.starts_with("#[") {
            if trimmed.contains("cfg(test)") {
                in_test_mod = true;
            }
            if trimmed == "#[test]" || trimmed.starts_with("#[test]") {
                attr_has_test = true;
            }
            scope.update(line, None);
            continue;
        }

        pre_code = false;

        // Imports
        if import_re.is_match(trimmed) {
            imports.push(trimmed.to_string());
            doc_buf.clear();
            attr_has_test = false;
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
            attr_has_test = false;
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
                let is_test_fn = attr_has_test || in_test_mod;
                let mut sig = Signature::new(raw, kind, line_idx, &path, qualified, doc);
                // Re-use `tested` field to tag test functions so annotate_tested() can
                // harvest their names without needing to re-read source files.
                sig.tested = is_test_fn;
                signatures.push(sig);
                attr_has_test = false;
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
            attr_has_test = false;
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
        churn_label: None,
        inline_test_fns: Vec::new(),
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
        churn_label: None,
        inline_test_fns: Vec::new(),
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
        churn_label: None,
        inline_test_fns: Vec::new(),
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
        churn_label: None,
        inline_test_fns: Vec::new(),
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
        churn_label: None,
        inline_test_fns: Vec::new(),
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
        churn_label: None,
        inline_test_fns: Vec::new(),
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
        churn_label: None,
        inline_test_fns: Vec::new(),
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
        churn_label: None,
        inline_test_fns: Vec::new(),
    }
}

// ---------------------------------------------------------------------------
// C#
// ---------------------------------------------------------------------------

fn extract_csharp(path: String, content: &str) -> MappedFile {
    let import_re = Regex::new(r"^using\s+([\w.]+)").unwrap();
    let type_re = Regex::new(
        r"^(?:(?:public|private|protected|internal|static|abstract|sealed|virtual|override|readonly|partial)\s+)*(?:class|interface|enum|struct|record)\s+(\w+)",
    )
    .unwrap();
    let fn_re = Regex::new(
        r"^(?:(?:public|private|protected|internal|static|abstract|sealed|virtual|override|readonly|async)\s+)+[\w<>\[\]?]+\s+(\w+)\s*\(",
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
            if !in_block_comment { doc_buf.clear(); }
            scope.update(line, None);
            continue;
        }

        if in_block_comment {
            if trimmed.contains("*/") { in_block_comment = false; }
            else { doc_buf.push(strip_doc_marker(trimmed)); }
            scope.update(line, None);
            continue;
        }

        if trimmed.starts_with("/**") || trimmed.starts_with("/*") {
            in_block_comment = !trimmed.contains("*/");
            doc_buf.push(strip_doc_marker(trimmed));
            scope.update(line, None);
            continue;
        }

        if trimmed.starts_with("///") || trimmed.starts_with("//") {
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

        if let Some(caps) = type_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let kind = if trimmed.contains("interface") {
                SymbolKind::Interface
            } else if trimmed.contains("enum") {
                SymbolKind::Enum
            } else if trimmed.contains("struct") {
                SymbolKind::Struct
            } else {
                SymbolKind::Class
            };
            let doc = take_doc(&mut doc_buf);
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            signatures.push(Signature::new(raw, kind, line_idx, &path, name.clone(), doc));
            scope.update(line, Some(name));
            continue;
        }

        if let Some(caps) = fn_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            if !matches!(name.as_str(), "if" | "for" | "while" | "switch" | "foreach" | "catch") {
                let qualified = scope.qualify(&name);
                let kind = if scope.current().is_some() { SymbolKind::Method } else { SymbolKind::Function };
                let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
                let doc = take_doc(&mut doc_buf);
                signatures.push(Signature::new(raw, kind, line_idx, &path, qualified, doc));
            }
            scope.update(line, None);
            continue;
        }

        doc_buf.clear();
        scope.update(line, None);
    }

    MappedFile { path, imports, signatures, docstrings: None, parameters: None, return_types: None, churn_label: None, inline_test_fns: Vec::new() }
}

// ---------------------------------------------------------------------------
// Swift
// ---------------------------------------------------------------------------

fn extract_swift(path: String, content: &str) -> MappedFile {
    let import_re = Regex::new(r"^import\s+(\w+)").unwrap();
    let type_re = Regex::new(
        r"^(?:(?:public|private|internal|fileprivate|open|final)\s+)*(?:class|struct|enum|protocol|actor)\s+(\w+)",
    )
    .unwrap();
    let fn_re = Regex::new(
        r"^(?:(?:public|private|internal|fileprivate|open|final|override|static|class|mutating|lazy)\s+)*func\s+(\w+)",
    )
    .unwrap();
    let prop_re = Regex::new(
        r"^(?:(?:public|private|internal|fileprivate|open|final|lazy|static)\s+)*(?:var|let)\s+(\w+)\s*:",
    )
    .unwrap();
    let ext_re = Regex::new(r"^extension\s+(\w+)").unwrap();
    let alias_re = Regex::new(r"^typealias\s+(\w+)").unwrap();

    let mut imports = Vec::new();
    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();
    let mut scope = ScopeTracker::new();
    let mut in_block_comment = false;

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();

        if trimmed.is_empty() {
            if !in_block_comment { doc_buf.clear(); }
            scope.update(line, None);
            continue;
        }

        if in_block_comment {
            if trimmed.contains("*/") { in_block_comment = false; }
            else { doc_buf.push(strip_doc_marker(trimmed)); }
            scope.update(line, None);
            continue;
        }

        if trimmed.starts_with("/**") || trimmed.starts_with("/*") {
            in_block_comment = !trimmed.contains("*/");
            doc_buf.push(strip_doc_marker(trimmed));
            scope.update(line, None);
            continue;
        }

        if trimmed.starts_with("///") || trimmed.starts_with("//") {
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

        if let Some(caps) = type_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let kind = if trimmed.contains("protocol") { SymbolKind::Interface }
                else if trimmed.contains("enum") { SymbolKind::Enum }
                else if trimmed.contains("struct") { SymbolKind::Struct }
                else { SymbolKind::Class };
            let doc = take_doc(&mut doc_buf);
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            signatures.push(Signature::new(raw, kind, line_idx, &path, name.clone(), doc));
            scope.update(line, Some(name));
            continue;
        }

        if let Some(caps) = ext_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let doc = take_doc(&mut doc_buf);
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            signatures.push(Signature::new(raw, SymbolKind::Namespace, line_idx, &path, name.clone(), doc));
            scope.update(line, Some(name));
            continue;
        }

        if let Some(caps) = alias_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(trimmed.to_string(), SymbolKind::TypeAlias, line_idx, &path, name, doc));
            scope.update(line, None);
            continue;
        }

        if let Some(caps) = fn_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let qualified = scope.qualify(&name);
            let kind = if scope.current().is_some() { SymbolKind::Method } else { SymbolKind::Function };
            let raw = trimmed.split('{').next().unwrap_or(trimmed).trim().to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(raw, kind, line_idx, &path, qualified, doc));
            scope.update(line, None);
            continue;
        }

        if let Some(caps) = prop_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            if scope.current().is_some() {
                let qualified = scope.qualify(&name);
                let doc = take_doc(&mut doc_buf);
                let raw = trimmed.split('=').next().unwrap_or(trimmed).trim().to_string();
                signatures.push(Signature::new(raw, SymbolKind::Field, line_idx, &path, qualified, doc));
            } else {
                doc_buf.clear();
            }
            scope.update(line, None);
            continue;
        }

        doc_buf.clear();
        scope.update(line, None);
    }

    MappedFile { path, imports, signatures, docstrings: None, parameters: None, return_types: None, churn_label: None, inline_test_fns: Vec::new() }
}

// ---------------------------------------------------------------------------
// Lua
// ---------------------------------------------------------------------------

fn extract_lua(path: String, content: &str) -> MappedFile {
    let require_re = Regex::new(r#"^(?:local\s+\w+\s*=\s*)?require\s*\(?['"]([^'"]+)['"]\)?"#).unwrap();
    let fn_decl_re = Regex::new(r"^(?:local\s+)?function\s+([\w.:]+)\s*\(").unwrap();
    let fn_assign_re = Regex::new(r"^(?:local\s+)?([\w.:]+)\s*=\s*function\s*\(").unwrap();

    let mut imports = Vec::new();
    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();

        if trimmed.is_empty() {
            doc_buf.clear();
            continue;
        }

        if trimmed.starts_with("--") {
            doc_buf.push(strip_doc_marker(trimmed));
            continue;
        }

        if let Some(caps) = require_re.captures(trimmed) {
            let module = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            imports.push(format!("require '{}'", module));
            doc_buf.clear();
            continue;
        }

        if let Some(caps) = fn_decl_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let doc = take_doc(&mut doc_buf);
            let raw = trimmed.split(')').next().map(|s| format!("{})", s)).unwrap_or_else(|| trimmed.to_string());
            signatures.push(Signature::new(raw, SymbolKind::Function, line_idx, &path, name, doc));
            continue;
        }

        if let Some(caps) = fn_assign_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            if !name.is_empty() && !name.starts_with('_') {
                let doc = take_doc(&mut doc_buf);
                let raw = format!("function {}(...)", name);
                signatures.push(Signature::new(raw, SymbolKind::Function, line_idx, &path, name, doc));
            } else {
                doc_buf.clear();
            }
            continue;
        }

        doc_buf.clear();
    }

    MappedFile { path, imports, signatures, docstrings: None, parameters: None, return_types: None, churn_label: None, inline_test_fns: Vec::new() }
}

// ---------------------------------------------------------------------------
// Shell (sh / bash / zsh / fish)
// ---------------------------------------------------------------------------

fn extract_shell(path: String, content: &str) -> MappedFile {
    let fn_paren_re = Regex::new(r"^(\w[\w-]*)\s*\(\)\s*(?:\{|$)").unwrap();
    let fn_keyword_re = Regex::new(r"^function\s+(\w[\w-]*)").unwrap();

    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();

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

        if let Some(caps) = fn_keyword_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(format!("function {}()", name), SymbolKind::Function, line_idx, &path, name, doc));
            continue;
        }

        if let Some(caps) = fn_paren_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(format!("{}()", name), SymbolKind::Function, line_idx, &path, name, doc));
            continue;
        }

        doc_buf.clear();
    }

    MappedFile { path, imports: Vec::new(), signatures, docstrings: None, parameters: None, return_types: None, churn_label: None, inline_test_fns: Vec::new() }
}

// ---------------------------------------------------------------------------
// SQL
// ---------------------------------------------------------------------------

fn extract_sql(path: String, content: &str) -> MappedFile {
    let ddl_re = Regex::new(
        r"(?i)^CREATE\s+(?:OR\s+REPLACE\s+)?(?:TABLE|VIEW|FUNCTION|PROCEDURE|INDEX|TRIGGER)\s+(?:\w+\.)?(\w+)",
    )
    .unwrap();
    let alter_re = Regex::new(r"(?i)^ALTER\s+TABLE\s+(?:\w+\.)?(\w+)").unwrap();

    let mut signatures = Vec::new();
    let mut doc_buf: Vec<String> = Vec::new();

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();

        if trimmed.is_empty() {
            doc_buf.clear();
            continue;
        }

        if trimmed.starts_with("--") {
            doc_buf.push(strip_doc_marker(trimmed));
            continue;
        }

        if let Some(caps) = ddl_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let upper = trimmed.to_uppercase();
            let kind = if upper.contains("TABLE") { SymbolKind::Struct }
                else if upper.contains("VIEW") { SymbolKind::Class }
                else if upper.contains("FUNCTION") || upper.contains("PROCEDURE") { SymbolKind::Function }
                else { SymbolKind::Unknown };
            let doc = take_doc(&mut doc_buf);
            let raw = trimmed.split('(').next().unwrap_or(trimmed).trim_end_matches(';').trim().to_string();
            signatures.push(Signature::new(raw, kind, line_idx, &path, name, doc));
            continue;
        }

        if let Some(caps) = alter_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let doc = take_doc(&mut doc_buf);
            signatures.push(Signature::new(trimmed.trim_end_matches(';').to_string(), SymbolKind::Struct, line_idx, &path, name, doc));
            continue;
        }

        doc_buf.clear();
    }

    MappedFile { path, imports: Vec::new(), signatures, docstrings: None, parameters: None, return_types: None, churn_label: None, inline_test_fns: Vec::new() }
}

// ---------------------------------------------------------------------------
// Markdown
// ---------------------------------------------------------------------------

/// Code-file extensions used to detect file-path cross-references in docs.
const CODE_EXTENSIONS: &[&str] = &[
    "rs", "go", "py", "js", "jsx", "ts", "tsx", "mjs", "cjs",
    "java", "kt", "scala", "c", "cpp", "cc", "cxx", "h", "hpp",
    "rb", "php", "cs", "swift", "lua", "sh", "sql",
    "yaml", "yml", "toml", "json", "md",
];

fn looks_like_file_path(s: &str) -> bool {
    // Contains a slash or ends with a known extension
    if s.contains('/') { return true; }
    if let Some(dot) = s.rfind('.') {
        let ext = &s[dot + 1..];
        return CODE_EXTENSIONS.contains(&ext);
    }
    false
}

fn extract_markdown(path: String, content: &str) -> MappedFile {
    let heading_re = Regex::new(r"^(#{1,6})\s+(.+)").unwrap();
    let link_re = Regex::new(r"\[.*?\]\(([^)]+)\)").unwrap();
    let backtick_sym_re = Regex::new(r"`([A-Z]\w{3,})`").unwrap();
    // Captures backtick file refs like `scanner.rs`, `search.md`, `config.yaml`
    let backtick_file_re = Regex::new(
        r"`([\w_-]+\.(?:rs|go|py|ts|tsx|js|jsx|mjs|cjs|java|kt|rb|php|cs|swift|lua|sh|sql|c|h|cpp|cc|cxx|hpp|md|yaml|yml|toml|json))`"
    ).unwrap();
    let bare_path_re = Regex::new(r"(?:^|\s)((?:\./|src/|lib/|pkg/|cmd/|internal/)[\w/.@-]+)").unwrap();
    let frontmatter_key_re = Regex::new(r"^([\w_-]+)\s*:").unwrap();

    let mut signatures = Vec::new();
    let mut imports = Vec::new();
    let mut seen_imports = std::collections::HashSet::new();

    let lines: Vec<&str> = content.lines().collect();
    let mut start_line = 0;

    // --- YAML front-matter (fenced by --- at line 0) ---
    if lines.first().map(|l| l.trim()) == Some("---") {
        for (i, line) in lines.iter().enumerate().skip(1) {
            let trimmed = line.trim();
            if trimmed == "---" {
                start_line = i + 1;
                break;
            }
            if let Some(caps) = frontmatter_key_re.captures(trimmed) {
                let name = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
                if !name.is_empty() {
                    signatures.push(Signature::new(
                        trimmed.to_string(), SymbolKind::ConfigKey, i, &path,
                        format!("frontmatter.{}", name), None,
                    ));
                }
            }
        }
    }

    // --- Main pass: headings + cross-references ---
    for (line_idx, line) in lines.iter().enumerate().skip(start_line) {
        let trimmed = line.trim();

        // Headings
        if let Some(caps) = heading_re.captures(trimmed) {
            let level = caps.get(1).map(|m| m.as_str().len()).unwrap_or(1);
            let title = caps.get(2).map(|m| m.as_str().trim()).unwrap_or("").to_string();
            if title.is_empty() { continue; }
            let kind = if level == 1 { SymbolKind::Namespace } else { SymbolKind::Field };
            let slug = title.to_lowercase().replace(|c: char| !c.is_alphanumeric(), "-");
            let raw = format!("{} {}", "#".repeat(level), title);
            signatures.push(Signature::new(raw, kind, line_idx, &path, slug, None));
        }

        // Markdown link cross-refs: [text](target)
        for caps in link_re.captures_iter(trimmed) {
            let target = caps.get(1).map(|m| m.as_str()).unwrap_or("");
            // Skip URLs, anchors, and images
            if target.starts_with("http") || target.starts_with('#') || target.is_empty() {
                continue;
            }
            let target = target.split('#').next().unwrap_or(target); // strip anchor
            if looks_like_file_path(target) && seen_imports.insert(target.to_string()) {
                imports.push(target.trim_start_matches("./").to_string());
            }
        }

        // Backtick PascalCase symbol refs: `ApiState`, `MappedFile`
        for caps in backtick_sym_re.captures_iter(trimmed) {
            let sym = caps.get(1).map(|m| m.as_str()).unwrap_or("");
            if seen_imports.insert(sym.to_string()) {
                imports.push(sym.to_string());
            }
        }

        // Backtick file refs: `scanner.rs`, `search.md`, `config.yaml`
        for caps in backtick_file_re.captures_iter(trimmed) {
            let file_ref = caps.get(1).map(|m| m.as_str()).unwrap_or("");
            if seen_imports.insert(file_ref.to_string()) {
                imports.push(file_ref.to_string());
            }
        }

        // Bare file paths: src/foo/bar.rs, ./lib/util.ts
        for caps in bare_path_re.captures_iter(trimmed) {
            let p = caps.get(1).map(|m| m.as_str()).unwrap_or("");
            let clean = p.trim_start_matches("./");
            if seen_imports.insert(clean.to_string()) {
                imports.push(clean.to_string());
            }
        }
    }

    MappedFile { path, imports, signatures, docstrings: None, parameters: None, return_types: None, churn_label: None, inline_test_fns: Vec::new() }
}

// ---------------------------------------------------------------------------
// YAML
// ---------------------------------------------------------------------------

fn extract_yaml(path: String, content: &str) -> MappedFile {
    let key_re = Regex::new(r"^(\s*)([\w_-]+)\s*:(.*)").unwrap();
    let max_depth: usize = 3;

    let mut signatures = Vec::new();
    let mut top_level_keys = Vec::new();

    // Indent-stack: (indent_level, key_name)
    let mut stack: Vec<(usize, String)> = Vec::new();

    for (line_idx, line) in content.lines().enumerate() {
        // Skip comments, empty lines, list items
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') || trimmed.starts_with('-') {
            continue;
        }

        if let Some(caps) = key_re.captures(line) {
            let indent = caps.get(1).map(|m| m.as_str().len()).unwrap_or(0);
            let key = caps.get(2).map(|m| m.as_str()).unwrap_or("").to_string();
            let value = caps.get(3).map(|m| m.as_str().trim()).unwrap_or("");

            if key.is_empty() { continue; }

            // Pop stack entries at same or deeper indent
            while let Some(&(level, _)) = stack.last() {
                if level >= indent { stack.pop(); } else { break; }
            }

            // Track top-level keys for OpenAPI detection
            if indent == 0 {
                top_level_keys.push(key.clone());
            }

            // Build dot-path from stack
            let depth = stack.len();
            if depth < max_depth {
                let dot_path = if stack.is_empty() {
                    key.clone()
                } else {
                    let prefix: Vec<&str> = stack.iter().map(|(_, k)| k.as_str()).collect();
                    format!("{}.{}", prefix.join("."), key)
                };

                let kind = if indent == 0 { SymbolKind::Field } else { SymbolKind::ConfigKey };
                let raw = if value.is_empty() {
                    format!("{}:", dot_path)
                } else {
                    format!("{}: {}", dot_path, value)
                };
                signatures.push(Signature::new(raw, kind, line_idx, &path, dot_path, None));
            }

            stack.push((indent, key));
        }
    }

    // --- OpenAPI detection ---
    let is_openapi = top_level_keys.iter().any(|k| k == "openapi" || k == "swagger");
    if is_openapi {
        extract_yaml_openapi_paths(content, &path, &mut signatures);
    }

    MappedFile { path, imports: Vec::new(), signatures, docstrings: None, parameters: None, return_types: None, churn_label: None, inline_test_fns: Vec::new() }
}

/// Extract OpenAPI endpoint paths from YAML content.
/// Looks for lines under `paths:` that start with `/`.
fn extract_yaml_openapi_paths(content: &str, path: &str, signatures: &mut Vec<Signature>) {
    let path_entry_re = Regex::new(r"^  (/\S+)\s*:").unwrap();
    let method_re = Regex::new(r"^    (get|post|put|patch|delete|head|options)\s*:").unwrap();

    let mut in_paths = false;
    let mut current_path = String::new();

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();

        // Detect `paths:` section (top-level, no indent)
        if !line.starts_with(' ') && trimmed.starts_with("paths:") {
            in_paths = true;
            continue;
        }
        // Exit paths section when next top-level key appears
        if in_paths && !line.starts_with(' ') && !trimmed.is_empty() {
            in_paths = false;
        }

        if !in_paths { continue; }

        // Path entry: /api/users:
        if let Some(caps) = path_entry_re.captures(line) {
            current_path = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            continue;
        }

        // HTTP method under a path
        if !current_path.is_empty() {
            if let Some(caps) = method_re.captures(line) {
                let method = caps.get(1).map(|m| m.as_str().to_uppercase()).unwrap_or_default();
                let raw = format!("{} {}", method, current_path);
                let qname = format!("paths.{}.{}", current_path, method.to_lowercase());
                signatures.push(Signature::new(
                    raw, SymbolKind::Endpoint, line_idx, path, qname, None,
                ));
            }
        }
    }
}

// ---------------------------------------------------------------------------
// TOML
// ---------------------------------------------------------------------------

fn extract_toml(path: String, content: &str) -> MappedFile {
    // Try structured parsing first; fall back to regex on failure.
    if let Ok(table) = content.parse::<toml::Value>() {
        let mut signatures = Vec::new();
        toml_walk(&table, "", &path, &mut signatures, 0, 3);

        // Map line numbers: for each signature, find the matching line in source.
        // This is a best-effort pass — qualified names are used as search keys.
        let lines: Vec<&str> = content.lines().collect();
        for sig in &mut signatures {
            if let Some(qname) = &sig.qualified_name {
                // Use the last segment as the key to search for
                let search_key = qname.rsplit('.').next().unwrap_or(qname);
                for (i, line) in lines.iter().enumerate() {
                    let trimmed = line.trim();
                    if trimmed.starts_with(search_key) || trimmed.starts_with(&format!("[{}]", qname)) {
                        sig.line_start = i;
                        break;
                    }
                }
            }
        }

        return MappedFile { path, imports: Vec::new(), signatures, docstrings: None, parameters: None, return_types: None, churn_label: None, inline_test_fns: Vec::new() };
    }

    // Fallback: regex-only for malformed TOML
    let section_re = Regex::new(r"^\[([^\]]+)\]").unwrap();
    let key_re = Regex::new(r"^([\w_-]+)\s*=").unwrap();
    let mut signatures = Vec::new();
    let mut current_section = String::new();

    for (line_idx, line) in content.lines().enumerate() {
        let trimmed = line.trim();
        if trimmed.is_empty() || trimmed.starts_with('#') { continue; }
        if let Some(caps) = section_re.captures(trimmed) {
            let name = caps.get(1).map(|m| m.as_str().trim()).unwrap_or("").to_string();
            if name.is_empty() { continue; }
            current_section = name.clone();
            signatures.push(Signature::new(trimmed.to_string(), SymbolKind::Namespace, line_idx, &path, name, None));
        } else if let Some(caps) = key_re.captures(trimmed) {
            let key = caps.get(1).map(|m| m.as_str()).unwrap_or("").to_string();
            let qname = if current_section.is_empty() {
                key.clone()
            } else {
                format!("{}.{}", current_section, key)
            };
            signatures.push(Signature::new(trimmed.to_string(), SymbolKind::ConfigKey, line_idx, &path, qname, None));
        }
    }

    MappedFile { path, imports: Vec::new(), signatures, docstrings: None, parameters: None, return_types: None, churn_label: None, inline_test_fns: Vec::new() }
}

/// Recursively walk a parsed TOML value tree, emitting signatures.
fn toml_walk(
    value: &toml::Value,
    prefix: &str,
    path: &str,
    signatures: &mut Vec<Signature>,
    depth: usize,
    max_depth: usize,
) {
    if depth > max_depth { return; }

    if let Some(table) = value.as_table() {
        for (key, val) in table {
            let qname = if prefix.is_empty() {
                key.clone()
            } else {
                format!("{}.{}", prefix, key)
            };

            match val {
                toml::Value::Table(_) => {
                    // Section/table → Namespace
                    let raw = if depth == 0 {
                        format!("[{}]", qname)
                    } else {
                        format!("{}:", qname)
                    };
                    signatures.push(Signature::new(
                        raw, SymbolKind::Namespace, 0, path, qname.clone(), None,
                    ));
                    toml_walk(val, &qname, path, signatures, depth + 1, max_depth);
                }
                toml::Value::Array(arr) if arr.first().map(|v| v.is_table()).unwrap_or(false) => {
                    // Array of tables (e.g. [[bin]])
                    let raw = format!("[[{}]]", qname);
                    signatures.push(Signature::new(
                        raw, SymbolKind::Namespace, 0, path, qname.clone(), None,
                    ));
                    // Walk first entry only for structure discovery
                    if let Some(first) = arr.first() {
                        toml_walk(first, &qname, path, signatures, depth + 1, max_depth);
                    }
                }
                _ => {
                    // Leaf value → ConfigKey
                    let val_str = match val {
                        toml::Value::String(s) => format!("\"{}\"", s),
                        other => other.to_string(),
                    };
                    let raw = format!("{} = {}", qname, val_str);
                    signatures.push(Signature::new(
                        raw, SymbolKind::ConfigKey, 0, path, qname, None,
                    ));
                }
            }
        }
    }
}

// ---------------------------------------------------------------------------
// JSON
// ---------------------------------------------------------------------------

/// Max JSON file size to attempt parsing (512 KB). Larger files (data fixtures,
/// generated output) are skipped to avoid slow extraction.
const JSON_MAX_SIZE: usize = 512 * 1024;

fn extract_json(path: String, content: &str) -> MappedFile {
    let empty = MappedFile {
        path: path.clone(), imports: Vec::new(), signatures: Vec::new(),
        docstrings: None, parameters: None, return_types: None, churn_label: None,
        inline_test_fns: Vec::new(),
    };

    if content.len() > JSON_MAX_SIZE {
        return empty;
    }

    let parsed: serde_json::Value = match serde_json::from_str(content) {
        Ok(v) => v,
        Err(_) => return empty,
    };

    let obj = match parsed.as_object() {
        Some(o) => o,
        None => return empty,
    };

    let mut signatures = Vec::new();
    let mut imports = Vec::new();

    // Detect variant
    let has_schema = obj.contains_key("$schema");
    let is_openapi = obj.contains_key("openapi") || obj.contains_key("swagger");
    let is_package_json = obj.contains_key("name") && obj.contains_key("version")
        && (obj.contains_key("dependencies") || obj.contains_key("devDependencies"));

    if is_openapi {
        extract_json_openapi(obj, &path, &mut signatures);
    } else if has_schema {
        extract_json_schema(obj, &path, &mut signatures, &mut imports);
    } else if is_package_json {
        extract_json_package(obj, &path, &mut signatures, &mut imports);
    } else {
        // Generic: top-level keys as Field, nested at depth <= 2 as ConfigKey
        json_walk(obj, "", &path, &mut signatures, 0, 2);
    }

    MappedFile { path, imports, signatures, docstrings: None, parameters: None, return_types: None, churn_label: None, inline_test_fns: Vec::new() }
}

fn json_walk(
    obj: &serde_json::Map<String, serde_json::Value>,
    prefix: &str,
    path: &str,
    signatures: &mut Vec<Signature>,
    depth: usize,
    max_depth: usize,
) {
    for (key, val) in obj {
        let qname = if prefix.is_empty() { key.clone() } else { format!("{}.{}", prefix, key) };
        let kind = if depth == 0 { SymbolKind::Field } else { SymbolKind::ConfigKey };

        match val {
            serde_json::Value::Object(inner) if depth < max_depth => {
                signatures.push(Signature::new(
                    format!("{}:", qname), kind, 0, path, qname.clone(), None,
                ));
                json_walk(inner, &qname, path, signatures, depth + 1, max_depth);
            }
            _ => {
                let val_str = match val {
                    serde_json::Value::String(s) => format!("\"{}\"", truncate_str(s, 60)),
                    serde_json::Value::Array(_) => "[...]".to_string(),
                    serde_json::Value::Object(_) => "{...}".to_string(),
                    other => other.to_string(),
                };
                signatures.push(Signature::new(
                    format!("{}: {}", qname, val_str), kind, 0, path, qname, None,
                ));
            }
        }
    }
}

fn truncate_str(s: &str, max: usize) -> String {
    if s.len() <= max {
        s.to_string()
    } else {
        // `max` is a byte budget. Slicing a &str at an arbitrary byte offset
        // panics if it lands inside a multi-byte UTF-8 char (e.g. an en-dash
        // '–' or 'ü' in indexed JSON strings), so back off to the nearest
        // char boundary at or below `max`.
        let mut end = max;
        while end > 0 && !s.is_char_boundary(end) {
            end -= 1;
        }
        format!("{}...", &s[..end])
    }
}

/// Extract OpenAPI endpoints from a parsed JSON object.
fn extract_json_openapi(
    obj: &serde_json::Map<String, serde_json::Value>,
    path: &str,
    signatures: &mut Vec<Signature>,
) {
    // info.title
    if let Some(info) = obj.get("info").and_then(|v| v.as_object()) {
        if let Some(title) = info.get("title").and_then(|v| v.as_str()) {
            signatures.push(Signature::new(
                format!("info.title: \"{}\"", title), SymbolKind::Field, 0, path,
                "info.title".to_string(), None,
            ));
        }
    }

    // paths
    if let Some(paths) = obj.get("paths").and_then(|v| v.as_object()) {
        for (endpoint, methods) in paths {
            if let Some(methods_obj) = methods.as_object() {
                for method in methods_obj.keys() {
                    let m = method.to_uppercase();
                    if matches!(m.as_str(), "GET" | "POST" | "PUT" | "PATCH" | "DELETE" | "HEAD" | "OPTIONS") {
                        let raw = format!("{} {}", m, endpoint);
                        let qname = format!("paths.{}.{}", endpoint, method);
                        signatures.push(Signature::new(
                            raw, SymbolKind::Endpoint, 0, path, qname, None,
                        ));
                    }
                }
            }
        }
    }

    // components.schemas
    if let Some(components) = obj.get("components").and_then(|v| v.as_object()) {
        if let Some(schemas) = components.get("schemas").and_then(|v| v.as_object()) {
            for schema_name in schemas.keys() {
                let qname = format!("components.schemas.{}", schema_name);
                signatures.push(Signature::new(
                    format!("schema {}", schema_name), SymbolKind::Namespace, 0, path,
                    qname, None,
                ));
            }
        }
    }
}

/// Extract JSON Schema properties and $ref imports.
fn extract_json_schema(
    obj: &serde_json::Map<String, serde_json::Value>,
    path: &str,
    signatures: &mut Vec<Signature>,
    imports: &mut Vec<String>,
) {
    // Title
    if let Some(title) = obj.get("title").and_then(|v| v.as_str()) {
        signatures.push(Signature::new(
            format!("schema: {}", title), SymbolKind::Namespace, 0, path,
            title.to_string(), None,
        ));
    }

    // Properties
    if let Some(props) = obj.get("properties").and_then(|v| v.as_object()) {
        for (key, val) in props {
            let type_str = val.get("type").and_then(|v| v.as_str()).unwrap_or("any");
            let raw = format!("{}: {}", key, type_str);
            signatures.push(Signature::new(
                raw, SymbolKind::ConfigKey, 0, path,
                format!("properties.{}", key), None,
            ));
        }
    }

    // $ref values → imports
    collect_json_refs(obj, imports);
}

fn collect_json_refs(obj: &serde_json::Map<String, serde_json::Value>, imports: &mut Vec<String>) {
    // Depth-guard: deeply nested JSON (adversarial or generated) could otherwise
    // recurse the stack to overflow, same class as the tree-sitter walkers.
    let _depth = match crate::extractor::DepthGuard::enter() { Some(g) => g, None => return };
    for (key, val) in obj {
        if key == "$ref" {
            if let Some(r) = val.as_str() {
                // Only add file-path refs, not internal #/definitions/... refs
                if !r.starts_with('#') && !r.is_empty() {
                    imports.push(r.trim_start_matches("./").to_string());
                }
            }
        }
        match val {
            serde_json::Value::Object(inner) => collect_json_refs(inner, imports),
            serde_json::Value::Array(arr) => {
                for item in arr {
                    if let Some(inner) = item.as_object() {
                        collect_json_refs(inner, imports);
                    }
                }
            }
            _ => {}
        }
    }
}

/// Extract package.json: name, scripts, dependencies as imports.
fn extract_json_package(
    obj: &serde_json::Map<String, serde_json::Value>,
    path: &str,
    signatures: &mut Vec<Signature>,
    imports: &mut Vec<String>,
) {
    // name + version
    if let Some(name) = obj.get("name").and_then(|v| v.as_str()) {
        let version = obj.get("version").and_then(|v| v.as_str()).unwrap_or("?");
        signatures.push(Signature::new(
            format!("{} @ {}", name, version), SymbolKind::Namespace, 0, path,
            "package".to_string(), None,
        ));
    }

    // scripts
    if let Some(scripts) = obj.get("scripts").and_then(|v| v.as_object()) {
        for (key, val) in scripts {
            let cmd = val.as_str().unwrap_or("...");
            signatures.push(Signature::new(
                format!("script {}: {}", key, truncate_str(cmd, 60)),
                SymbolKind::ConfigKey, 0, path,
                format!("scripts.{}", key), None,
            ));
        }
    }

    // main / module entry points → imports
    for field in &["main", "module", "types"] {
        if let Some(entry) = obj.get(*field).and_then(|v| v.as_str()) {
            imports.push(entry.trim_start_matches("./").to_string());
        }
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
        churn_label: None,
        inline_test_fns: Vec::new(),
    }
}

// ---------------------------------------------------------------------------
// Tests — document extractors
// ---------------------------------------------------------------------------

#[cfg(test)]
mod doc_tests {
    use super::*;
    use std::path::Path;

    // ── Markdown ──────────────────────────────────────────────────────────

    #[test]
    fn markdown_headings_preserved() {
        let content = "# Title\n\nSome text.\n\n## Section One\n\n### Subsection";
        let mf = extract_skeleton(Path::new("README.md"), content);
        let heading_sigs: Vec<_> = mf.signatures.iter()
            .filter(|s| s.kind == SymbolKind::Namespace || s.kind == SymbolKind::Field)
            .collect();
        assert_eq!(heading_sigs.len(), 3);
        assert_eq!(heading_sigs[0].kind, SymbolKind::Namespace); // H1
        assert_eq!(heading_sigs[1].kind, SymbolKind::Field);     // H2
        assert_eq!(heading_sigs[2].kind, SymbolKind::Field);     // H3
    }

    #[test]
    fn markdown_link_crossrefs() {
        let content = "# Guide\n\nSee [the handler](src/api/handler.rs) for details.\n\nAlso check [config](./config.toml).";
        let mf = extract_skeleton(Path::new("docs/guide.md"), content);
        assert!(mf.imports.iter().any(|i| i == "src/api/handler.rs"), "should import handler.rs");
        assert!(mf.imports.iter().any(|i| i == "config.toml"), "should import config.toml");
    }

    #[test]
    fn markdown_backtick_symbol_refs() {
        let content = "# API\n\nThe `ApiState` struct manages the graph. See `MappedFile` too.\n\nIgnore `foo` (too short).";
        let mf = extract_skeleton(Path::new("docs/api.md"), content);
        assert!(mf.imports.iter().any(|i| i == "ApiState"), "should import ApiState");
        assert!(mf.imports.iter().any(|i| i == "MappedFile"), "should import MappedFile");
        assert!(!mf.imports.iter().any(|i| i == "foo"), "should NOT import short names");
    }

    #[test]
    fn markdown_frontmatter() {
        let content = "---\ntitle: My Doc\ntags: rust, api\n---\n# Content\n\nBody text.";
        let mf = extract_skeleton(Path::new("docs/post.md"), content);
        let fm_sigs: Vec<_> = mf.signatures.iter().filter(|s| s.kind == SymbolKind::ConfigKey).collect();
        assert_eq!(fm_sigs.len(), 2, "should extract 2 front-matter keys");
        assert!(fm_sigs.iter().any(|s| s.qualified_name.as_deref() == Some("frontmatter.title")));
    }

    #[test]
    fn markdown_skips_urls() {
        let content = "# Links\n\n[Google](https://google.com)\n[Anchor](#section)";
        let mf = extract_skeleton(Path::new("README.md"), content);
        assert!(mf.imports.is_empty(), "should not import URLs or anchors");
    }

    #[test]
    fn markdown_bare_paths() {
        let content = "# Guide\n\nEdit src/mapper.rs to change extraction.";
        let mf = extract_skeleton(Path::new("CONTRIBUTING.md"), content);
        assert!(mf.imports.iter().any(|i| i == "src/mapper.rs"), "should detect bare file paths");
    }

    #[test]
    fn markdown_backtick_file_refs() {
        let content = "| `scanner.rs` | File discovery |\n| `mapper.rs` | Extraction |\n| `api.rs` | Graph |\n\nSee `config.yaml` too.";
        let mf = extract_skeleton(Path::new("docs/architecture.md"), content);
        assert!(mf.imports.iter().any(|i| i == "scanner.rs"), "should import scanner.rs");
        assert!(mf.imports.iter().any(|i| i == "mapper.rs"), "should import mapper.rs");
        assert!(mf.imports.iter().any(|i| i == "api.rs"), "should import api.rs");
        assert!(mf.imports.iter().any(|i| i == "config.yaml"), "should import config.yaml");
    }

    // ── YAML ─────────────────────────────────────────────────────────────

    #[test]
    fn yaml_nested_keys() {
        let content = "server:\n  host: localhost\n  port: 8080\ndatabase:\n  name: mydb";
        let mf = extract_skeleton(Path::new("config.yaml"), content);
        let qnames: Vec<_> = mf.signatures.iter().filter_map(|s| s.qualified_name.as_deref()).collect();
        assert!(qnames.contains(&"server"), "should have top-level key");
        assert!(qnames.contains(&"server.host"), "should have nested key");
        assert!(qnames.contains(&"server.port"), "should have nested key");
        assert!(qnames.contains(&"database.name"), "should have nested key");
    }

    #[test]
    fn yaml_depth_cap() {
        let content = "a:\n  b:\n    c:\n      d:\n        e: deep";
        let mf = extract_skeleton(Path::new("deep.yml"), content);
        // Depth 3 cap means a, a.b, a.b.c are extracted; a.b.c.d and deeper are not
        let qnames: Vec<_> = mf.signatures.iter().filter_map(|s| s.qualified_name.as_deref()).collect();
        assert!(qnames.contains(&"a.b.c"), "depth 3 should be included");
        assert!(!qnames.iter().any(|q| q.contains("d")), "depth 4+ should be excluded");
    }

    #[test]
    fn yaml_openapi_endpoints() {
        let content = "\
openapi: 3.0.0
info:
  title: Test API
paths:
  /users:
    get:
    post:
  /users/{id}:
    get:
    delete:
components:
  schemas:";
        let mf = extract_skeleton(Path::new("openapi.yaml"), content);
        let endpoints: Vec<_> = mf.signatures.iter()
            .filter(|s| s.kind == SymbolKind::Endpoint)
            .collect();
        assert!(endpoints.len() >= 4, "should extract at least 4 endpoints, got {}", endpoints.len());
        assert!(endpoints.iter().any(|s| s.raw == "GET /users"));
        assert!(endpoints.iter().any(|s| s.raw == "DELETE /users/{id}"));
    }

    // ── TOML ─────────────────────────────────────────────────────────────

    #[test]
    fn toml_sections_and_keys() {
        let content = "[package]\nname = \"codecartographer\"\nversion = \"3.0.0\"\n\n[dependencies]\nserde = \"1.0\"";
        let mf = extract_skeleton(Path::new("Cargo.toml"), content);
        let qnames: Vec<_> = mf.signatures.iter().filter_map(|s| s.qualified_name.as_deref()).collect();
        assert!(qnames.contains(&"package"), "should have package section");
        assert!(qnames.contains(&"package.name"), "should have package.name key");
        assert!(qnames.contains(&"dependencies"), "should have dependencies section");
        assert!(qnames.contains(&"dependencies.serde"), "should have dependencies.serde key");
    }

    #[test]
    fn toml_fallback_on_bad_input() {
        // Malformed TOML — should still extract what it can via regex fallback
        let content = "[section]\nkey = value\n[bad\nmore = stuff";
        let mf = extract_skeleton(Path::new("bad.toml"), content);
        // Regex fallback should get at least the section and key
        assert!(!mf.signatures.is_empty(), "fallback should extract something");
    }

    // ── JSON ─────────────────────────────────────────────────────────────

    #[test]
    fn json_generic_keys() {
        let content = r#"{"name": "test", "version": 1, "config": {"debug": true}}"#;
        let mf = extract_skeleton(Path::new("settings.json"), content);
        assert!(!mf.signatures.is_empty(), "should extract JSON keys");
        let qnames: Vec<_> = mf.signatures.iter().filter_map(|s| s.qualified_name.as_deref()).collect();
        assert!(qnames.contains(&"name"));
        assert!(qnames.contains(&"config.debug"));
    }

    #[test]
    fn json_openapi() {
        let content = r#"{"openapi": "3.0.0", "info": {"title": "My API"}, "paths": {"/health": {"get": {}}}}"#;
        let mf = extract_skeleton(Path::new("api.json"), content);
        let endpoints: Vec<_> = mf.signatures.iter().filter(|s| s.kind == SymbolKind::Endpoint).collect();
        assert_eq!(endpoints.len(), 1);
        assert_eq!(endpoints[0].raw, "GET /health");
    }

    #[test]
    fn json_schema_properties() {
        let content = r#"{"$schema": "http://json-schema.org/draft-07/schema#", "title": "User", "properties": {"name": {"type": "string"}, "age": {"type": "integer"}}}"#;
        let mf = extract_skeleton(Path::new("user.schema.json"), content);
        let props: Vec<_> = mf.signatures.iter().filter(|s| s.kind == SymbolKind::ConfigKey).collect();
        assert_eq!(props.len(), 2, "should extract 2 properties");
    }

    #[test]
    fn json_package_json() {
        let content = r#"{"name": "my-app", "version": "1.0.0", "main": "dist/index.js", "dependencies": {"express": "^4.18.0"}}"#;
        let mf = extract_skeleton(Path::new("package.json"), content);
        assert!(mf.imports.iter().any(|i| i == "dist/index.js"), "should import main entry point");
    }

    #[test]
    fn json_size_guard() {
        // Content > 512KB should return empty
        let content = "x".repeat(600_000);
        let mf = extract_skeleton(Path::new("huge.json"), &content);
        assert!(mf.signatures.is_empty(), "should skip oversized JSON");
    }
}
