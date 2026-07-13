// MCP Server - Exposes Project CodeCartographer via Model Context Protocol
// This allows AI tools and agents to interact with CodeCartographer using MCP

use crate::api::{ApiState, ModuleContextRequest};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

// ---------------------------------------------------------------------------
// watch_graph event types
// ---------------------------------------------------------------------------

#[derive(Serialize)]
#[serde(rename_all = "snake_case")]
pub enum GraphEventKind {
    FileReindexed,
    #[allow(dead_code)]
    GraphUpdated,
}

#[derive(Serialize)]
pub struct GraphEvent {
    pub kind:         GraphEventKind,
    pub path:         String,
    pub timestamp_ms: u64,
}

macro_rules! mcprop {
    ($type:literal, $desc:literal) => {
        McpProperty {
            type_: $type.to_string(),
            description: $desc.to_string(),
            enum_: None,
        }
    };
    ($type:literal, $desc:literal, [$($val:literal),+]) => {
        McpProperty {
            type_: $type.to_string(),
            description: $desc.to_string(),
            enum_: Some(vec![$($val.to_string()),+]),
        }
    };
}

macro_rules! mcinput {
    ($($key:literal => $type:literal => $desc:literal),* $(,)?) => {{
        let mut props = HashMap::new();
        $(
            props.insert($key.to_string(), mcprop!($type, $desc));
        )*
        McpInputSchema {
            type_: "object".to_string(),
            properties: props,
            required: vec![$($key.to_string()),*],
        }
    }};
}

macro_rules! read_only {
    () => {
        Some(ToolAnnotations {
            read_only_hint: Some(true),
            destructive_hint: None,
            idempotent_hint: None,
        })
    };
}

macro_rules! destructive {
    () => {
        Some(ToolAnnotations {
            read_only_hint: Some(false),
            destructive_hint: Some(true),
            idempotent_hint: None,
        })
    };
}

/// MCP Tool definition
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct McpTool {
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    pub description: String,
    #[serde(rename = "inputSchema")]
    pub input_schema: McpInputSchema,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub annotations: Option<ToolAnnotations>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct McpInputSchema {
    #[serde(rename = "type")]
    pub type_: String,
    pub properties: HashMap<String, McpProperty>,
    pub required: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct McpProperty {
    #[serde(rename = "type")]
    pub type_: String,
    pub description: String,
    #[serde(rename = "enum", skip_serializing_if = "Option::is_none")]
    pub enum_: Option<Vec<String>>,
}

/// Hints about a tool's behaviour, consumed by MCP clients and LLM planners.
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ToolAnnotations {
    /// true = tool never modifies state (safe to call freely)
    #[serde(rename = "readOnlyHint", skip_serializing_if = "Option::is_none")]
    pub read_only_hint: Option<bool>,
    /// true = tool may permanently destroy or overwrite data
    #[serde(rename = "destructiveHint", skip_serializing_if = "Option::is_none")]
    pub destructive_hint: Option<bool>,
    /// true = repeated calls with the same arguments produce the same result
    #[serde(rename = "idempotentHint", skip_serializing_if = "Option::is_none")]
    pub idempotent_hint: Option<bool>,
}

/// MCP Resource definition
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct McpResource {
    pub uri: String,
    pub name: String,
    pub description: String,
    pub mime_type: Option<String>,
}

/// MCP Prompt definition
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct McpPrompt {
    pub name: String,
    pub description: String,
    pub arguments: Vec<McpArgument>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct McpArgument {
    pub name: String,
    pub description: String,
    pub required: bool,
}

/// MCP Server capabilities
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct McpCapabilities {
    pub tools: bool,
    pub resources: bool,
    pub prompts: bool,
}

/// MCP Server info
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct McpServerInfo {
    pub name: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub title: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub description: Option<String>,
    pub version: String,
    pub capabilities: McpCapabilities,
}

impl Default for McpServerInfo {
    fn default() -> Self {
        Self {
            name: "Project CodeCartographer MCP Server".to_string(),
            title: Some("CodeCartographer".to_string()),
            description: Some(
                "Code intelligence and architectural analysis server. Provides dependency \
                 graph analysis, skeleton views, git intelligence, architectural health \
                 scoring, and context retrieval for AI-assisted development."
                    .to_string(),
            ),
            version: "1.0.0".to_string(),
            capabilities: McpCapabilities {
                tools: true,
                resources: true,
                prompts: true,
            },
        }
    }
}

/// MCP Tool Call request
#[derive(Debug, Deserialize)]
pub struct McpToolCall {
    pub name: String,
    pub arguments: serde_json::Value,
}

/// The canonical identifier argument for a tool, so `target` can serve as a universal
/// alias for it. Returns `None` for tools that take no single file/module/symbol target.
fn primary_key(tool: &str) -> Option<&'static str> {
    Some(match tool {
        "get_module_context" | "get_dependencies" | "get_dependents" | "get_symbol_context"
        | "get_semantic_impact" | "simulate_change" | "analyze_module" | "plan_refactoring" => {
            "module_id"
        }
        "focused_skeleton" => "focus",
        "reach_symbol" => "symbol",
        "search_in_symbol" | "extract_content" | "list_key_handlers" | "map_state_machine" => "file",
        "doc_context" => "doc_path",
        "get_blast_radius" => "target",
        "switch_project" => "path",
        _ => return None,
    })
}

/// MCP Tool Call response
#[derive(Debug, Serialize)]
pub struct McpToolResult {
    pub content: Vec<McpContent>,
    #[serde(rename = "isError", skip_serializing_if = "Option::is_none")]
    pub is_error: Option<bool>,
}

#[derive(Debug, Serialize)]
#[serde(tag = "type", rename_all = "lowercase")]
pub enum McpContent {
    Text { text: String },
    #[allow(dead_code)]
    Image { data: String, mime_type: String },
    #[allow(dead_code)]
    Resource { resource: McpResource },
}

impl McpContent {
    pub fn text(content: String) -> Self {
        McpContent::Text { text: content }
    }
}

/// MCP Server implementation
/// The "core" preset: the highest-value discovery tools for large-codebase work.
/// Keeps the surface small so an LLM picks the right tool. `serve --preset=core`.
const CORE_PRESET: &[&str] = &[
    "reach_symbol",
    "get_dependents",
    "get_blast_radius",
    "skeleton_map",
    "ranked_skeleton",
    "focused_skeleton",
    "search_content",
    "query_context",
    "git_cochange",
    "semidiff",
    "get_health",
    "simulate_change",
    "switch_project",
];

/// JSON-RPC id used for the server-initiated `roots/list` request. Responses
/// carrying this id are routed to the roots auto-follow path, not treated as
/// client requests.
const ROOTS_REQUEST_ID: &str = "__cartographer_roots_list__";

pub struct McpServer {
    /// The active project. Held behind a lock so `switch_project` and the roots
    /// auto-follow can re-root the server without rebuilding it.
    state_cell: std::sync::RwLock<std::sync::Arc<ApiState>>,
    /// Set when the client advertised the `roots` capability at initialize;
    /// gates whether serve() asks it for the workspace root.
    client_supports_roots: std::sync::atomic::AtomicBool,
    tools: Vec<McpTool>,
    resources: Vec<McpResource>,
    prompts: Vec<McpPrompt>,
    /// When set, only these tool names are exposed and callable (preset filtering).
    enabled: Option<std::collections::HashSet<String>>,
}

impl McpServer {
    #[allow(dead_code)] // convenience constructor; used by tests and as public API
    pub fn new(api_state: std::sync::Arc<ApiState>) -> Self {
        Self::with_preset(api_state, None)
    }

    /// Build a server exposing only the named preset's tools, or all tools when `preset`
    /// is `None`. Unknown preset names fall back to all tools (logged to stderr).
    pub fn with_preset(api_state: std::sync::Arc<ApiState>, preset: Option<&str>) -> Self {
        let all = Self::create_tools();
        let resources = Self::create_resources();
        let prompts = Self::create_prompts();

        let (tools, enabled) = match preset.map(|p| p.trim().to_lowercase()) {
            None => (all, None),
            Some(p) if p.is_empty() || p == "all" || p == "full" => (all, None),
            Some(p) if p == "core" => {
                let set: std::collections::HashSet<String> =
                    CORE_PRESET.iter().map(|s| s.to_string()).collect();
                let filtered: Vec<McpTool> =
                    all.into_iter().filter(|t| set.contains(&t.name)).collect();
                (filtered, Some(set))
            }
            Some(other) => {
                eprintln!(
                    "cartographer: unknown preset '{other}' — exposing all tools (valid: core)"
                );
                (all, None)
            }
        };

        Self {
            state_cell: std::sync::RwLock::new(api_state),
            client_supports_roots: std::sync::atomic::AtomicBool::new(false),
            tools,
            resources,
            prompts,
            enabled,
        }
    }

    /// A cheap Arc snapshot of the active project. Taken once per request so a
    /// concurrent `switch_project` can't swap the root under a running handler.
    fn state(&self) -> std::sync::Arc<ApiState> {
        self.state_cell.read().unwrap().clone()
    }

    /// Re-root the server at `new_root`, rebuilding the graph eagerly so any
    /// error surfaces now and the first tool call is warm. Powers both the
    /// `switch_project` tool and the roots auto-follow in `serve`.
    pub fn switch_project(&self, new_root: std::path::PathBuf) -> Result<String, String> {
        let canon = new_root
            .canonicalize()
            .map_err(|e| format!("cannot resolve {}: {}", new_root.display(), e))?;
        if !canon.is_dir() {
            return Err(format!("not a directory: {}", canon.display()));
        }
        // Scan + parse the new root the same way the initial serve path does,
        // then build the graph from it. ApiState::new alone does not touch disk.
        let mapped = crate::build_mapped_files_cached(&canon).map_err(|e| e.to_string())?;
        let state = std::sync::Arc::new(ApiState::new(canon.clone()));
        {
            let mut files = state.mapped_files.lock().map_err(|e| e.to_string())?;
            *files = mapped;
        }
        let graph = state.rebuild_graph()?;
        let (files, edges) = (graph.nodes.len(), graph.edges.len());
        state.refresh_if_stale(); // prime the mtime baseline for future edits
        *self.state_cell.write().unwrap() = state;
        eprintln!("cartographer: project root -> {}", canon.display());
        Ok(format!(
            "Switched project root to {} ({} files, {} edges).",
            canon.display(),
            files,
            edges
        ))
    }

    /// Handle a client->server *response* (a reply to a request we sent). Only
    /// the `roots/list` response is meaningful: re-root at the first workspace.
    fn handle_client_response(&self, v: &serde_json::Value) {
        if v.get("id").and_then(|i| i.as_str()) != Some(ROOTS_REQUEST_ID) {
            return;
        }
        let root = v
            .pointer("/result/roots")
            .and_then(|r| r.as_array())
            .and_then(|arr| arr.first())
            .and_then(|r0| r0.get("uri"))
            .and_then(|u| u.as_str())
            .and_then(file_uri_to_path);
        if let Some(path) = root {
            match self.switch_project(path) {
                Ok(msg) => eprintln!("cartographer: roots auto-follow — {}", msg),
                Err(e) => eprintln!("cartographer: roots auto-follow failed — {}", e),
            }
        }
    }

    fn create_tools() -> Vec<McpTool> {
        let mut tools = vec![
            McpTool {
                name: "get_module_context".to_string(),
                title: Some("Get Module Context".to_string()),
                description: "Returns the public API surface of a module: exports, signatures, and \
                              imports. Set depth>0 to traverse transitive dependencies. Use \
                              detail_level=extended for doc comments and inferred types. Prefer \
                              focused_skeleton for neighbourhood context, or get_symbol_context \
                              when you need a single symbol."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert(
                            "module_id".to_string(),
                            McpProperty {
                                type_: "string".to_string(),
                                description: "File path or module name (e.g. \"src/api.rs\")"
                                    .to_string(),
                                enum_: None,
                            },
                        );
                        props.insert(
                            "depth".to_string(),
                            McpProperty {
                                type_: "number".to_string(),
                                description: "Transitive dependency depth (0 = this module only, default 0)"
                                    .to_string(),
                                enum_: None,
                            },
                        );
                        props.insert(
                            "detail_level".to_string(),
                            mcprop!("string", "Detail level", ["minimal", "standard", "extended"]),
                        );
                        props
                    },
                    required: vec!["module_id".to_string()],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "get_symbol_context".to_string(),
                title: Some("Get Symbol Context".to_string()),
                description: "Returns the full definition, inferred type, and call-site context for a \
                              single named symbol inside a module. More targeted than get_module_context \
                              when you already know which function, struct, or constant you need."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("module_id".to_string(), mcprop!("string", "Module containing the symbol (file path or module name)"));
                        props.insert("symbol_name".to_string(), mcprop!("string", "Name of the symbol to retrieve"));
                        props.insert("detail_level".to_string(), mcprop!("string", "Detail level", ["minimal", "standard", "extended"]));
                        props
                    },
                    required: vec!["module_id".to_string(), "symbol_name".to_string()],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "get_project_graph".to_string(),
                title: Some("Get Project Dependency Graph".to_string()),
                description: "Returns the complete project import/dependency graph as structured JSON. \
                              Nodes are modules; edges are import relationships with direction and weight. \
                              Use skeleton_map or ranked_skeleton for token-efficient structural overviews \
                              and get_module_context for per-module detail."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "get_dependencies".to_string(),
                title: Some("Get Module Dependencies".to_string()),
                description: "Returns the dependency tree rooted at the given module. depth=1 gives \
                              direct imports only; higher values trace the transitive closure. Each \
                              node includes a public signature summary."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert(
                            "module_id".to_string(),
                            mcprop!("string", "Module to get dependencies for (file path or module name)"),
                        );
                        props.insert(
                            "depth".to_string(),
                            mcprop!("number", "Dependency depth to traverse (default 1)"),
                        );
                        props
                    },
                    required: vec!["module_id".to_string()],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "get_dependents".to_string(),
                title: Some("Get Module Dependents".to_string()),
                description: "Returns all modules that directly import the given module (reverse \
                              dependency lookup). Run this before changing or removing a public API \
                              to understand the full breakage scope."
                    .to_string(),
                input_schema: mcinput!(
                    "module_id" => "string" => "Module to get dependents for (file path or module name)"
                ),
                annotations: read_only!(),
            },
            McpTool {
                name: "search_project".to_string(),
                title: Some("Search Project Graph".to_string()),
                description: "Searches the project graph by module name or import edge pattern. \
                              Use query_type=node to match file paths and module names; \
                              query_type=edge to match import relationships. For full-text search \
                              inside files, use search_content instead."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("query".to_string(), mcprop!("string", "Search pattern to match against module names or import edges"));
                        props.insert("query_type".to_string(), mcprop!("string", "Search domain: node (module names/paths) or edge (import relationships)", ["node", "edge"]));
                        props
                    },
                    required: vec!["query".to_string()],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "get_blast_radius".to_string(),
                title: Some("Get Blast Radius".to_string()),
                description: "Returns all files and symbols transitively reachable from the target — \
                              the full impact set for a proposed change. Each related entry includes \
                              lip_uris (LIP symbol URIs: lip://local/<path>#<symbol>) so CKB can \
                              resolve any affected symbol without a follow-up lookup."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("target".to_string(), mcprop!("string", "File path or symbol name to analyse"));
                        props.insert("max_related".to_string(), mcprop!("number", "Maximum related items to return (default 10)"));
                        props
                    },
                    required: vec!["target".to_string()],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "get_evolution".to_string(),
                title: Some("Get Architectural Evolution".to_string()),
                description: "Returns the architectural health trend over a configurable time window: \
                              health score trajectory, per-indicator deltas (cycles added/removed, \
                              god-module growth, new layer violations), and actionable recommendations. \
                              Increase days to widen the look-back period."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert(
                            "days".to_string(),
                            mcprop!("number", "Look-back window in days (default 30)"),
                        );
                        props
                    },
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "watch_status".to_string(),
                title: Some("Watch Status".to_string()),
                description: "Polls the background watch daemon for recent file changes. Returns \
                              { lastChangedMs, changedFiles } when the daemon is active, or \
                              { watching: false } when it is not running. Prefer poll_changes for \
                              timestamp-based queries that work without the watch daemon."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "set_compression_level".to_string(),
                title: Some("Set Compression Level".to_string()),
                description: "Configures how aggressively responses are compressed for the remainder \
                              of this session. minimal preserves full detail; standard collapses \
                              boilerplate; aggressive maximises token savings at the cost of some \
                              fidelity. Affects all subsequent tool responses."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("level".to_string(), mcprop!("string", "Compression level", ["minimal", "standard", "aggressive"]));
                        props
                    },
                    required: vec!["level".to_string()],
                },
                annotations: Some(ToolAnnotations {
                    read_only_hint: Some(false),
                    destructive_hint: None,
                    idempotent_hint: Some(true),
                }),
            },
            McpTool {
                name: "find_files".to_string(),
                title: Some("Find Files".to_string()),
                description: "Finds files whose path matches a glob pattern and returns path, language, \
                              and byte size for each match. Patterns without a path separator match the \
                              filename anywhere in the tree. Use instead of shell find or ls. For \
                              content search, use search_content."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert(
                            "pattern".to_string(),
                            mcprop!("string", "Glob pattern, e.g. \"*.rs\" or \"src/**/*.ts\". Patterns without \"/\" match filename anywhere in tree."),
                        );
                        props.insert(
                            "limit".to_string(),
                            mcprop!("number", "Max files to return — 0 = unlimited (default 200)"),
                        );
                        props
                    },
                    required: vec!["pattern".to_string()],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "search_content".to_string(),
                title: Some("Search File Content".to_string()),
                description: "Searches file content for a regex or literal pattern (ripgrep-style) \
                              and returns matching lines with optional surrounding context. Restrict \
                              scope with fileGlob. Use instead of shell grep calls. For searches \
                              scoped to a function body, use search_in_symbol."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert(
                            "pattern".to_string(),
                            mcprop!("string", "Search pattern (regex by default, or literal string if literal=true)"),
                        );
                        props.insert(
                            "literal".to_string(),
                            mcprop!("boolean", "Treat pattern as a literal string (default false)"),
                        );
                        props.insert(
                            "caseSensitive".to_string(),
                            mcprop!("boolean", "Case-sensitive matching (default true)"),
                        );
                        props.insert(
                            "contextLines".to_string(),
                            mcprop!("number", "Lines of context before and after each match (default 0)"),
                        );
                        props.insert(
                            "maxResults".to_string(),
                            mcprop!("number", "Max matches to return — 0 = unlimited (default 100)"),
                        );
                        props.insert(
                            "fileGlob".to_string(),
                            mcprop!("string", "Optional glob to restrict files, e.g. \"*.rs\" or \"src/**/*.ts\""),
                        );
                        props
                    },
                    required: vec!["pattern".to_string()],
                },
                annotations: read_only!(),
            },

            // -----------------------------------------------------------------
            // Architectural analysis
            // -----------------------------------------------------------------
            McpTool {
                name: "get_health".to_string(),
                title: Some("Get Architectural Health".to_string()),
                description: "Returns the overall architectural health score (0–100) and summary \
                              counts: dependency cycles, bridge modules, god modules, and layer \
                              violations. Use get_cycles or check_layers to drill into specific \
                              issue categories."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "get_cycles".to_string(),
                title: Some("Get Dependency Cycles".to_string()),
                description: "Returns all circular dependency cycles in the project graph, each with \
                              a severity rating and a suggested pivot node to break the cycle. Call \
                              get_health first for a quick count; use this tool when you need \
                              actionable cycle-breaking details."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "check_layers".to_string(),
                title: Some("Check Layer Constraints".to_string()),
                description: "Validates the project against its layers.toml architectural layer \
                              config. Returns each violation with source module, target module, \
                              the layers involved, and severity. Reports no violations if \
                              layers.toml is absent."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "unreferenced_symbols".to_string(),
                title: Some("Find Unreferenced Symbols".to_string()),
                description: "Lists public symbols that appear unreferenced anywhere in the project — \
                              dead-code candidates. Heuristic: does not account for dynamic dispatch, \
                              reflection, or external consumers outside the mapped scope."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "simulate_change".to_string(),
                title: Some("Simulate Module Change".to_string()),
                description: "Predicts the architectural impact of modifying a module without writing \
                              any code: affected module set, cycle risk introduced, new layer \
                              violations, and the projected health score delta. Optionally specify a \
                              signature being added or removed for more precise impact modelling."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("module_id".to_string(), mcprop!("string", "Relative path of the module to change"));
                        props.insert("new_signature".to_string(), mcprop!("string", "Optional new public signature being added"));
                        props.insert("remove_signature".to_string(), mcprop!("string", "Optional signature being removed"));
                        props
                    },
                    required: vec!["module_id".to_string()],
                },
                annotations: read_only!(),
            },

            // -----------------------------------------------------------------
            // Context / skeleton
            // -----------------------------------------------------------------
            McpTool {
                name: "skeleton_map".to_string(),
                title: Some("Get Skeleton Map".to_string()),
                description: "Returns a compressed structural overview of every project file: imports \
                              and public signatures only, no bodies. Ideal for a full project overview \
                              within a tight token budget. Use ranked_skeleton for relevance-ordered \
                              output, or focused_skeleton when working in a specific area."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("detail".to_string(), mcprop!("string", "Detail level", ["minimal", "standard", "extended"]));
                        props
                    },
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "ranked_skeleton".to_string(),
                title: Some("Get Ranked Skeleton".to_string()),
                description: "Returns a token-budget-aware skeleton of project files ordered by \
                              PageRank importance. Optionally personalise to a set of focus files so \
                              the most relevant modules surface first. The preferred starting point \
                              for broad architectural questions."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("focus".to_string(), mcprop!("string", "JSON array of focus file paths for personalisation, e.g. [\"src/api.rs\"]"));
                        props.insert("budget".to_string(), mcprop!("number", "Max tokens to include (0 = unlimited)"));
                        props
                    },
                    required: vec![],
                },
                annotations: read_only!(),
            },

            McpTool {
                name: "focused_skeleton".to_string(),
                title: Some("Get Focused Skeleton".to_string()),
                description: "Returns the enriched skeleton for a focus file and all files within N \
                              import-hops of it — both importers and importees. Use this instead of \
                              skeleton_map when working in a specific area and needing neighbourhood \
                              context without the full project. depth=1 is usually sufficient."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("focus".to_string(), mcprop!("string", "File path or module ID to centre on, e.g. \"src/api.rs\""));
                        props.insert("depth".to_string(), mcprop!("number", "Import-hop radius (default 1). 0 = focus file only, 2 = two hops out."));
                        props.insert("detail".to_string(), mcprop!("string", "Detail level", ["minimal", "standard", "extended"]));
                        props
                    },
                    required: vec!["focus".to_string()],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "diff_skeleton".to_string(),
                title: Some("Get Diff Skeleton".to_string()),
                description: "Returns the enriched skeleton for files changed between two commits plus \
                              their immediate importers — the minimal structural context needed to \
                              understand a diff's blast radius. Defaults to HEAD~1..HEAD. Pass from/to \
                              to target specific commits or refs."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("from".to_string(), mcprop!("string", "Base commit or ref (default HEAD~1)"));
                        props.insert("to".to_string(), mcprop!("string", "Target commit or ref (default HEAD)"));
                        props.insert("include_importers".to_string(), mcprop!("boolean", "Also include files that import the changed files (default true)"));
                        props
                    },
                    required: vec![],
                },
                annotations: read_only!(),
            },

            McpTool {
                name: "search_skeleton".to_string(),
                title: Some("Search Skeleton".to_string()),
                description: "Return skeleton sections for files whose path or symbol names \
                              contain the given pattern (case-insensitive substring). Use this \
                              when you know a keyword but not the exact module — cheaper than \
                              skeleton_map, more discoverable than focused_skeleton."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("pattern".to_string(), mcprop!("string", "Substring matched against file paths and symbol names (case-insensitive)"));
                        props.insert("detail".to_string(), mcprop!("string", "Detail level: minimal, standard, or extended (default standard)"));
                        props.insert("budget".to_string(), mcprop!("number", "Max tokens to return (0 = unlimited)"));
                        props
                    },
                    required: vec!["pattern".to_string()],
                },
                annotations: read_only!(),
            },

            // -----------------------------------------------------------------
            // Git intelligence
            // -----------------------------------------------------------------
            McpTool {
                name: "git_churn".to_string(),
                title: Some("Get Git Churn".to_string()),
                description: "Returns per-file commit frequency over recent git history, sorted by \
                              churn count. High-churn files are hotspot candidates likely to need \
                              refactoring or extra test coverage. Pair with shotgun_surgery to \
                              identify churn driven by scattered cross-module changes."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("limit".to_string(), mcprop!("number", "Number of commits to analyse (0 → 500)"));
                        props
                    },
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "git_cochange".to_string(),
                title: Some("Get Git Co-Change".to_string()),
                description: "Returns pairs of files that frequently change in the same commit, ranked \
                              by coupling score. A high score indicates implicit structural coupling \
                              even without an import edge. Use hidden_coupling to filter to pairs \
                              with no static import edge."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("limit".to_string(), mcprop!("number", "Commits to analyse (0 → 500)"));
                        props.insert("min_count".to_string(), mcprop!("number", "Minimum co-change count to include (0 → 2)"));
                        props
                    },
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "hidden_coupling".to_string(),
                title: Some("Find Hidden Coupling".to_string()),
                description: "Returns file pairs that co-change frequently in git history but share \
                              no import edge — implicit coupling invisible in the static graph. These \
                              pairs are candidates for extracting shared logic or making the \
                              dependency explicit."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("limit".to_string(), mcprop!("number", "Commits to analyse (0 → 500)"));
                        props.insert("min_count".to_string(), mcprop!("number", "Minimum co-change count (0 → 2)"));
                        props
                    },
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "semidiff".to_string(),
                title: Some("Semantic Diff".to_string()),
                description: "Returns a function-level semantic diff between two commits: which \
                              public signatures were added, removed, or changed. More informative \
                              than a raw git diff for understanding API-level impact. commit1 \
                              defaults to HEAD~1; commit2 defaults to HEAD."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("commit1".to_string(), mcprop!("string", "Base commit SHA or ref (default HEAD~1)"));
                        props.insert("commit2".to_string(), mcprop!("string", "Target commit SHA or ref (default HEAD)"));
                        props
                    },
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "poll_changes".to_string(),
                title: Some("Poll File Changes".to_string()),
                description: "Returns project files modified since a given epoch-millisecond \
                              timestamp. Pass since_ms=0 to retrieve files changed in the last \
                              60 seconds. Useful for building incremental update loops independent \
                              of the watch daemon."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("since_ms".to_string(), mcprop!("number", "Epoch milliseconds; 0 = last 60 seconds"));
                        props
                    },
                    required: vec![],
                },
                annotations: read_only!(),
            },

            // -----------------------------------------------------------------
            // Surgical editing
            // -----------------------------------------------------------------
            McpTool {
                name: "replace_content".to_string(),
                title: Some("Replace File Content".to_string()),
                description: "Performs find-and-replace across project files using a regex or literal \
                              pattern. Supports $0 (whole match) and $1/$2 capture-group references in \
                              the replacement string. Always use dryRun=true first to preview changes \
                              as a unified diff before writing."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("pattern".to_string(), mcprop!("string", "Regex pattern to search for"));
                        props.insert("replacement".to_string(), mcprop!("string", "Replacement string; supports $0 (whole match) and $1/$2 (capture groups)"));
                        props.insert("dryRun".to_string(), mcprop!("boolean", "Preview changes as a unified diff without writing to disk (default false)"));
                        props.insert("literal".to_string(), mcprop!("boolean", "Treat pattern as a literal string (default false)"));
                        props.insert("caseSensitive".to_string(), mcprop!("boolean", "Case-sensitive matching (default true)"));
                        props.insert("fileGlob".to_string(), mcprop!("string", "Restrict to files matching this glob, e.g. \"*.rs\""));
                        props.insert("excludeGlob".to_string(), mcprop!("string", "Exclude files matching this glob"));
                        props.insert("searchPath".to_string(), mcprop!("string", "Restrict to this repo-relative subdirectory"));
                        props.insert("maxPerFile".to_string(), mcprop!("number", "Max replacements per file (0 = unlimited)"));
                        props.insert("contextLines".to_string(), mcprop!("number", "Context lines in diff output (default 3)"));
                        props
                    },
                    required: vec!["pattern".to_string(), "replacement".to_string()],
                },
                annotations: destructive!(),
            },
            McpTool {
                name: "extract_content".to_string(),
                title: Some("Extract Pattern Matches".to_string()),
                description: "Extracts regex capture-group values from matching lines across project \
                              files. Use count=true for a frequency table, groups=[1,2] to isolate \
                              specific capture groups, and dedup=true to unique the output. Useful \
                              for auditing patterns like all pub fn names, TODO owners, or import paths."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("pattern".to_string(), mcprop!("string", "Regex pattern with optional capture groups, e.g. \"pub fn (\\w+)\""));
                        props.insert("groups".to_string(), mcprop!("string", "JSON array of capture group indices to extract, e.g. [1]. Empty = whole match."));
                        props.insert("count".to_string(), mcprop!("boolean", "Return frequency table instead of raw matches (default false)"));
                        props.insert("dedup".to_string(), mcprop!("boolean", "Deduplicate extracted values (default false)"));
                        props.insert("sort".to_string(), mcprop!("boolean", "Sort output (default false)"));
                        props.insert("caseSensitive".to_string(), mcprop!("boolean", "Case-sensitive matching (default true)"));
                        props.insert("fileGlob".to_string(), mcprop!("string", "Restrict to files matching this glob"));
                        props.insert("searchPath".to_string(), mcprop!("string", "Restrict to this repo-relative subdirectory"));
                        props.insert("limit".to_string(), mcprop!("number", "Max total results (0 = unlimited, default 1000)"));
                        props
                    },
                    required: vec!["pattern".to_string()],
                },
                annotations: read_only!(),
            },
            // PKG retrieval — full query → rank → score pipeline
            // -----------------------------------------------------------------
            McpTool {
                name: "query_context".to_string(),
                title: Some("Query Context".to_string()),
                description: "Full retrieval pipeline for code-question context injection. Given a \
                              natural-language query or symbol name: (1) searches the codebase for \
                              matching files, (2) uses PageRank personalised to those files to build \
                              a token-budget-aware skeleton, (3) scores the bundle with context_health. \
                              Returns the ready-to-inject context string plus health metadata. Use this \
                              instead of calling search_content + ranked_skeleton + context_health \
                              separately."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("query".to_string(), mcprop!("string", "Natural language question or symbol/pattern to search for"));
                        props.insert("budget".to_string(), mcprop!("number", "Max tokens for the skeleton portion (default: 8000)"));
                        props.insert("model".to_string(), mcprop!("string", "Target model family for health scoring", ["claude", "gpt4", "llama", "gpt35"]));
                        props.insert("maxSearchResults".to_string(), mcprop!("number", "Max search hits used as focus seeds (default: 20)"));
                        props
                    },
                    required: vec!["query".to_string()],
                },
                annotations: read_only!(),
            },
            // Shotgun surgery / co-change dispersion
            // -----------------------------------------------------------------
            McpTool {
                name: "shotgun_surgery".to_string(),
                title: Some("Detect Shotgun Surgery".to_string()),
                description: "Identifies files whose changes scatter widely across unrelated modules — \
                              a classic shotgun surgery smell. Computes co-change dispersion \
                              (arXiv:2504.18511): partner count and Shannon entropy over each file's \
                              co-change distribution. High entropy + many partners means a single \
                              change forces edits in many unrelated places."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("maxResults".to_string(), mcprop!("number", "Max entries to return (default 20)"));
                        props.insert("minPartners".to_string(), mcprop!("number", "Minimum distinct co-change partners to include (default 3)"));
                        props.insert("commits".to_string(), mcprop!("number", "Number of commits to analyse (default 500)"));
                        props
                    },
                    required: vec![],
                },
                annotations: read_only!(),
            },
            // Context quality
            // -----------------------------------------------------------------
            McpTool {
                name: "context_health".to_string(),
                title: Some("Score Context Health".to_string()),
                description: "Scores an LLM context bundle on a 0–100 scale (A–F grade) with a \
                              per-metric breakdown: signal density, compression density, position \
                              health, entity density, utilisation headroom, and dedup ratio. Warnings \
                              and recommendations are included when thresholds are breached. Pair with \
                              ranked_skeleton to iteratively build high-scoring context bundles."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("content".to_string(), mcprop!("string", "The context text to score (e.g. a ranked_skeleton output)"));
                        props.insert("model".to_string(), mcprop!("string", "Target model family", ["claude", "gpt4", "llama", "gpt35"]));
                        props.insert("windowSize".to_string(), mcprop!("number", "Override context window size in tokens (0 = use model default)"));
                        props.insert("signatureCount".to_string(), mcprop!("number", "Number of symbol signatures in the content (improves entity density scoring)"));
                        props.insert("signatureTokens".to_string(), mcprop!("number", "Tokens occupied by signature text (improves signal density scoring)"));
                        props.insert("keyPositions".to_string(), mcprop!("string", "JSON array of 0.0–1.0 relative positions of key modules in the output"));
                        props
                    },
                    required: vec!["content".to_string()],
                },
                annotations: read_only!(),
            },
            // -----------------------------------------------------------------
            // Symbol-scoped search
            // -----------------------------------------------------------------
            McpTool {
                name: "search_in_symbol".to_string(),
                title: Some("Search Within Symbol".to_string()),
                description: "Searches for a pattern scoped to the body of a named function or \
                              method, filtering out matches elsewhere in the file. Returns only \
                              lines within that symbol's approximate body range. Useful for queries \
                              like \"find X only inside handleKeyMsg()\" without wading through \
                              whole-file grep results."
                    .to_string(),
                input_schema: {
                    let mut props = HashMap::new();
                    props.insert("file".to_string(),    mcprop!("string", "Relative path or filename fragment (e.g. chatview.go)"));
                    props.insert("symbol".to_string(),  mcprop!("string", "Function or method name to scope the search to"));
                    props.insert("pattern".to_string(), mcprop!("string", "Regex or literal search pattern"));
                    props.insert("context_lines".to_string(), mcprop!("number", "Lines of context around each match (default 2)"));
                    McpInputSchema {
                        type_: "object".to_string(),
                        properties: props,
                        required: vec!["file".to_string(), "symbol".to_string(), "pattern".to_string()],
                    }
                },
                annotations: read_only!(),
            },
            // -----------------------------------------------------------------
            // TUI key-binding map
            // -----------------------------------------------------------------
            McpTool {
                name: "list_key_handlers".to_string(),
                title: Some("List Key Handlers".to_string()),
                description: "Extracts a structured key-binding map from a TUI source file. Groups \
                              all `case \"key\":` and `== \"key\"` patterns by key string with \
                              surrounding context. Works for Go/Bubble Tea, Rust/crossterm, and any \
                              framework using quoted key strings in match arms or if-chains."
                    .to_string(),
                input_schema: {
                    let mut props = HashMap::new();
                    props.insert("file".to_string(), mcprop!("string", "Relative path or filename fragment"));
                    props.insert("context_lines".to_string(), mcprop!("number", "Lines of context around each binding (default 4)"));
                    McpInputSchema {
                        type_: "object".to_string(),
                        properties: props,
                        required: vec!["file".to_string()],
                    }
                },
                annotations: read_only!(),
            },
            // -----------------------------------------------------------------
            // State-machine mapper
            // -----------------------------------------------------------------
            McpTool {
                name: "map_state_machine".to_string(),
                title: Some("Map State Machine".to_string()),
                description: "Correlates state guards with nearby key handlers to produce a \
                              state × handlers matrix. Given a state variable name and enum prefix, \
                              returns which keys are active in each state with guard line numbers. \
                              Ideal for large TUI files with switch-on-state dispatch such as \
                              Bubble Tea chatview."
                    .to_string(),
                input_schema: {
                    let mut props = HashMap::new();
                    props.insert("file".to_string(),         mcprop!("string", "Relative path or filename fragment"));
                    props.insert("state_var".to_string(),    mcprop!("string", "State variable expression to look for in guards (default: m.state)"));
                    props.insert("state_prefix".to_string(), mcprop!("string", "Enum variant prefix used to identify state constants (default: State)"));
                    props.insert("context_lines".to_string(), mcprop!("number", "Context lines around each guard (default 3)"));
                    McpInputSchema {
                        type_: "object".to_string(),
                        properties: props,
                        required: vec!["file".to_string()],
                    }
                },
                annotations: read_only!(),
            },
            // -----------------------------------------------------------------
            // Incremental graph push events
            // -----------------------------------------------------------------
            McpTool {
                name: "watch_graph".to_string(),
                title: Some("Watch Graph".to_string()),
                description: "Watches a directory recursively for source file changes and emits \
                              incremental graph events as newline-delimited JSON to stdout. Each \
                              event includes kind (file_reindexed or graph_updated), the file path, \
                              and a millisecond timestamp. Runs until timeout_secs elapses \
                              (default 30, max 300)."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("root".to_string(), mcprop!("string", "Root directory path to watch recursively"));
                        props.insert("timeout_secs".to_string(), mcprop!("number", "How long to watch in seconds (default 30, max 300)"));
                        props
                    },
                    required: vec!["root".to_string()],
                },
                annotations: read_only!(),
            },
            // -----------------------------------------------------------------
            // Document-oriented tools
            // -----------------------------------------------------------------
            McpTool {
                name: "doc_index".to_string(),
                title: Some("Get Document Index".to_string()),
                description: "Returns all document-type files (Markdown, YAML, TOML, JSON) in the \
                              project with their extracted headings, config keys, cross-reference \
                              edges, and edge counts. Use as a table of contents before drilling \
                              into a specific document with doc_context."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "doc_context".to_string(),
                title: Some("Get Document Context".to_string()),
                description: "Returns a single document's extracted structure plus the skeleton of \
                              all code files it cross-references. Follows import edges from the doc \
                              into code, ranked by relevance. The document comes first, supporting \
                              code follows — ideal for understanding what a doc describes before \
                              editing."
                    .to_string(),
                input_schema: {
                    let mut props = HashMap::new();
                    props.insert("doc_path".to_string(), mcprop!("string",
                        "Path to the document file (relative to project root, or a path fragment)"));
                    props.insert("budget".to_string(), mcprop!("number",
                        "Max tokens for referenced code context (default 4000)"));
                    McpInputSchema {
                        type_: "object".to_string(),
                        properties: props,
                        required: vec!["doc_path".to_string()],
                    }
                },
                annotations: read_only!(),
            },
            // -----------------------------------------------------------------
            // Semantic graph traversal [EXPERIMENTAL]
            // -----------------------------------------------------------------
            McpTool {
                name: "reach_symbol".to_string(),
                title: Some("Reach — Semantic Graph Traversal".to_string()),
                description: "START HERE for symbol discovery. Give it a bare symbol name (no file \
                              needed) and it walks the call graph + import graph outward, returning a \
                              compact context tree in AI-native format: the definition, all callers \
                              with call-site context, and callees. Detail is distance-proportional: \
                              depth-0 is the root symbol with full signature; depth-1 callers include \
                              a one-line call context; depth-2 neighbors show signature only. If the \
                              name is defined in several files it returns the candidate list so you \
                              can re-run with `file` set. Test callers are collapsed and counted. \
                              Roughly 40% of the token cost of equivalent JSON for the same semantic \
                              information. Call-graph support: Rust and Python; other languages fall \
                              back to heuristic text search."
                    .to_string(),
                input_schema: {
                    let mut props = HashMap::new();
                    props.insert("symbol".to_string(), mcprop!("string",
                        "Symbol name to start from (e.g. \"verify_token\" or \"Auth::verify_token\")"));
                    props.insert("file".to_string(), mcprop!("string",
                        "Disambiguate to a specific file when the symbol appears in multiple files (path fragment)"));
                    props.insert("depth".to_string(), mcprop!("number",
                        "Graph traversal depth (default 2; max 3)"));
                    props.insert("budget".to_string(), mcprop!("number",
                        "Token budget — hard cap; leaf nodes trimmed first (default 6000)"));
                    props.insert("includeTests".to_string(), mcprop!("boolean",
                        "Expand test call sites instead of collapsing them (default false)"));
                    props.insert("showBody".to_string(), mcprop!("boolean",
                        "Include the function body of the root symbol, up to 40 lines (default false)"));
                    McpInputSchema {
                        type_: "object".to_string(),
                        properties: props,
                        required: vec!["symbol".to_string()],
                    }
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "answer_question".to_string(),
                title: Some("Answer — Question-Driven Evidence Chain".to_string()),
                description: "EXPERIMENTAL. Takes a natural-language question and assembles the \
                              minimum set of semantic units that together answer it, in reading order. \
                              Unlike query_context (which returns a skeleton by PageRank), answer \
                              returns a numbered evidence chain: types before the functions that use \
                              them, entry points before internals, connections annotated between items. \
                              The top-scored item shows its function body by default."
                    .to_string(),
                input_schema: {
                    let mut props = HashMap::new();
                    props.insert("question".to_string(), mcprop!("string",
                        "Natural language question (e.g. \"how does rate limiting work?\")"));
                    props.insert("maxItems".to_string(), mcprop!("number",
                        "Maximum evidence items (default 6)"));
                    props.insert("budget".to_string(), mcprop!("number",
                        "Token budget (default 8000)"));
                    props.insert("showBody".to_string(), mcprop!("boolean",
                        "Show function body for top item (default true)"));
                    McpInputSchema {
                        type_: "object".to_string(),
                        properties: props,
                        required: vec!["question".to_string()],
                    }
                },
                annotations: read_only!(),
            },
            McpTool {
                name: "query_docs".to_string(),
                title: Some("Query Documentation".to_string()),
                description: "Doc-biased context retrieval: searches project documents first, then \
                              follows cross-reference edges into the code they describe. Returns a \
                              bundle with docs and supporting code separated. Prefer this over \
                              query_context when the user's question is most likely answered by \
                              documentation."
                    .to_string(),
                input_schema: {
                    let mut props = HashMap::new();
                    props.insert("query".to_string(), mcprop!("string",
                        "Natural language query or keyword to search for"));
                    props.insert("budget".to_string(), mcprop!("number",
                        "Max total tokens (default 8000)"));
                    props.insert("model".to_string(), mcprop!("string",
                        "Target model for health scoring", ["claude", "gpt4", "llama", "gpt35"]));
                    McpInputSchema {
                        type_: "object".to_string(),
                        properties: props,
                        required: vec!["query".to_string()],
                    }
                },
                annotations: read_only!(),
            },
        ];
        // Advertise `target` as the universal identifier alias on every tool that has a
        // primary file/module/symbol arg, and relax the old arg's `required` flag so a
        // caller can use either name. The handler maps `target` → the primary at dispatch.
        for t in &mut tools {
            if let Some(pk) = primary_key(&t.name) {
                t.input_schema.properties.entry("target".to_string()).or_insert_with(|| {
                    mcprop!(
                        "string",
                        "Canonical target: a file path, module id, or symbol name. The tool's \
                         original argument name still works as an alias."
                    )
                });
                // Keep `target` required only where it was already the canonical arg.
                if pk != "target" {
                    t.input_schema.required.retain(|k| k != pk);
                }
            }
        }

        // Re-root the running server at another repo, without restarting. Lets a
        // single session evaluate multiple codebases (the roots auto-follow does
        // this automatically for clients that support it).
        tools.push(McpTool {
            name: "switch_project".to_string(),
            title: Some("Switch Project Root".to_string()),
            description: "Re-point this server at a different repository by absolute path and \
                          rebuild its graph, for the rest of the session (or until switched \
                          again). Use when you want to analyse a repo other than the one the \
                          server was launched in. Returns the new root and its file/edge counts."
                .to_string(),
            input_schema: mcinput!(
                "path" => "string" => "Absolute path to the repository root to analyse"
            ),
            annotations: Some(ToolAnnotations {
                read_only_hint: Some(false),
                destructive_hint: Some(false),
                idempotent_hint: Some(true),
            }),
        });

        tools
    }

    fn create_resources() -> Vec<McpResource> {
        // Intentionally empty. The old `project-graph` and `module-index` resources each
        // serialized the WHOLE graph / module map as a single JSON blob — the exact
        // "dump everything" anti-pattern this tool exists to avoid. Worse, editors surface
        // MCP resources in the `@`-mention menu, so selecting one tried to inline the entire
        // repo into the prompt and failed with "too big for the index". Everything they
        // offered is available on-demand and budget-aware via tools instead:
        // `get_project_graph`, `skeleton_map`, `ranked_skeleton --budget N`.
        vec![]
    }

    fn create_prompts() -> Vec<McpPrompt> {
        vec![
            McpPrompt {
                name: "analyze_module".to_string(),
                description: "Pre-filled prompt for deep analysis of a module: its purpose, \
                              dependencies, and improvement opportunities."
                    .to_string(),
                arguments: vec![McpArgument {
                    name: "module_id".to_string(),
                    description: "File path or module name to analyse".to_string(),
                    required: true,
                }],
            },
            McpPrompt {
                name: "plan_refactoring".to_string(),
                description: "Pre-filled prompt for generating a step-by-step refactoring plan \
                              targeting a specific module and stated goal."
                    .to_string(),
                arguments: vec![
                    McpArgument {
                        name: "module_id".to_string(),
                        description: "File path or module name to refactor".to_string(),
                        required: true,
                    },
                    McpArgument {
                        name: "goal".to_string(),
                        description: "Refactoring goal (e.g. \"reduce coupling\", \"extract service\")".to_string(),
                        required: true,
                    },
                ],
            },
        ]
    }

    pub fn get_server_info(&self) -> McpServerInfo {
        McpServerInfo::default()
    }

    pub fn list_tools(&self) -> Vec<McpTool> {
        self.tools.clone()
    }

    pub fn list_resources(&self) -> Vec<McpResource> {
        self.resources.clone()
    }

    pub fn list_prompts(&self) -> Vec<McpPrompt> {
        self.prompts.clone()
    }

    pub fn call_tool(&self, mut call: McpToolCall) -> Result<McpToolResult, String> {
        // Snapshot the active project once so a mid-call switch_project can't swap
        // the root under a running handler.
        let api_state = self.state();
        // Keep the session fresh: pick up edits/deletes since the last call (debounced,
        // incremental — only changed files are re-parsed). No-op within the debounce window.
        api_state.refresh_if_stale();

        // Enforce preset filtering: a tool hidden from tools/list is not callable either.
        if let Some(enabled) = &self.enabled {
            if !enabled.contains(&call.name) {
                return Err(format!(
                    "Tool '{}' is not available in this preset. Restart `serve` without --preset for the full toolset.",
                    call.name
                ));
            }
        }
        // Universal `target` alias: if the tool's primary arg is absent, populate it from
        // `target` (or another accepted alias). Secondary args (e.g. reach_symbol's `file`
        // filter, get_symbol_context's `symbol_name`) are never overwritten.
        if let Some(pk) = primary_key(&call.name) {
            if let Some(obj) = call.arguments.as_object_mut() {
                let has_primary = obj.get(pk).and_then(|v| v.as_str()).is_some();
                if !has_primary {
                    for alias in ["target", "module_id", "focus", "file", "doc_path", "symbol"] {
                        if alias == pk {
                            continue;
                        }
                        if let Some(v) = obj.get(alias).filter(|v| v.is_string()).cloned() {
                            obj.insert(pk.to_string(), v);
                            break;
                        }
                    }
                }
            }
        }
        match call.name.as_str() {
            "get_module_context" => {
                let args = call.arguments;
                let request = ModuleContextRequest {
                    module_id: args
                        .get("module_id")
                        .and_then(|v| v.as_str())
                        .ok_or("Missing module_id")?
                        .to_string(),
                    depth: args.get("depth").and_then(|v| v.as_u64()).map(|v| v as u32),
                    detail_level: args
                        .get("detail_level")
                        .and_then(|v| v.as_str())
                        .map(String::from),
                    include: None,
                    format: None,
                };

                let response = api_state.get_module_context(&request)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&response).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "get_project_graph" => {
                let graph = api_state.rebuild_graph()?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&graph).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "get_dependencies" => {
                let args = call.arguments;
                let module_id = args
                    .get("module_id")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing module_id")?;
                let depth = args.get("depth").and_then(|v| v.as_u64()).unwrap_or(1) as u32;

                let deps = api_state
                    .get_dependencies_internal(module_id, depth)?
                    .ok_or("No dependencies found")?;

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&deps).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "get_dependents" => {
                let args = call.arguments;
                let module_id = args
                    .get("module_id")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing module_id")?;

                let dependents = api_state.get_dependents(module_id)?;

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&dependents).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "search_project" => {
                let args = call.arguments;
                let query = args
                    .get("query")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing query")?;
                let query_type = args.get("query_type").and_then(|v| v.as_str());

                let results = api_state.search_graph(query, query_type)?;

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&results).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "set_compression_level" => {
                let args = call.arguments;
                let level = args
                    .get("level")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing level")?;

                let level = match level {
                    "minimal" => crate::api::CompressionLevel::Minimal,
                    "aggressive" => crate::api::CompressionLevel::Aggressive,
                    _ => crate::api::CompressionLevel::Standard,
                };

                api_state.set_compression_level(level);

                Ok(McpToolResult {
                    content: vec![McpContent::text(format!(
                        "Compression level set to: {:?}",
                        level
                    ))],
                    is_error: None,
                })
            }

            "get_cycle_fix_plan" => {
                let args = call.arguments;
                let _cycle_index = args
                    .get("cycle_index")
                    .and_then(|v| v.as_u64())
                    .ok_or("Missing cycle_index")? as usize;

                let graph = api_state.rebuild_graph()?;

                let fix_plan = if graph.cycles.is_empty() {
                    "No cycles detected - graph is healthy!".to_string()
                } else {
                    let mut plan = String::from("## Cycle Fix Plans\n\n");
                    for (i, cycle) in graph.cycles.iter().enumerate() {
                        plan.push_str(&format!(
                            "### Cycle {} (severity: {})\n",
                            i + 1,
                            cycle.severity
                        ));
                        plan.push_str(&format!("  Nodes: {}\n", cycle.nodes.join(" -> ")));
                        if let Some(ref pivot) = cycle.pivot_node {
                            plan.push_str(&format!(
                                "  💡 Pivot node (remove this import to break cycle): {}\n",
                                pivot
                            ));
                        }
                        plan.push('\n');
                    }
                    plan
                };

                Ok(McpToolResult {
                    content: vec![McpContent::text(fix_plan)],
                    is_error: None,
                })
            }

            "explain_health_drop" => {
                let args = call.arguments;
                let _old_score = args
                    .get("old_score")
                    .and_then(|v| v.as_f64())
                    .unwrap_or(100.0);
                let _new_score = args
                    .get("new_score")
                    .and_then(|v| v.as_f64())
                    .unwrap_or(100.0);

                let graph = api_state.rebuild_graph()?;

                let health = graph.metadata.health_score.unwrap_or(100.0);
                let drop = 100.0 - health;

                let explanation = format!(
                    "## Architectural Health Analysis\n\n\
                     Current Health Score: {:.1}/100\n\
                     Score Drop: {:.1}\n\n\
                     ### Contributing Factors:\n\
                     - Bridges: {:?}\n\
                     - Cycles: {:?}\n\
                     - God Modules: {:?}\n\
                     - Layer Violations: {:?}\n\n\
                     ### Recommendations:\n\
                     {}",
                    health,
                    drop,
                    graph.metadata.bridge_count.unwrap_or(0),
                    graph.metadata.cycle_count.unwrap_or(0),
                    graph.metadata.god_module_count.unwrap_or(0),
                    graph.metadata.layer_violation_count.unwrap_or(0),
                    if drop > 20.0 {
                        "⚠️ Critical - Address immediately"
                    } else if drop > 10.0 {
                        "⚡ High - Review in this sprint"
                    } else {
                        "✅ Acceptable - Monitor trends"
                    }
                );

                Ok(McpToolResult {
                    content: vec![McpContent::text(explanation)],
                    is_error: None,
                })
            }

            "get_semantic_impact" => {
                let args = call.arguments;
                let module_id = args
                    .get("module_id")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing module_id")?;

                let graph = api_state.rebuild_graph()?;

                let node = graph.nodes.iter().find(|n| n.module_id == module_id);

                let impact = if let Some(n) = node {
                    let dependents: Vec<&str> = graph
                        .edges
                        .iter()
                        .filter(|e| e.target == module_id)
                        .map(|e| e.source.as_str())
                        .collect();

                    let dependencies: Vec<&str> = graph
                        .edges
                        .iter()
                        .filter(|e| e.source == module_id)
                        .map(|e| e.target.as_str())
                        .collect();

                    format!(
                        "## Semantic Impact Analysis for {}\n\n\
                         Path: {}\n\
                         Type: {}\n\
                         Risk Level: {}\n\
                         Is Bridge: {}\n\n\
                         ### Direct Dependencies ({})\n\
                         {}\n\n\
                         ### Direct Dependents ({})\n\
                         {}\n\n\
                         ### Bridge Score: {:?}\n\
                         ### Degree: {:?}",
                        module_id,
                        n.path,
                        n.language,
                        n.risk_level.as_deref().unwrap_or("UNKNOWN"),
                        n.is_bridge
                            .map(|b| if b { "Yes - HIGH IMPACT" } else { "No" })
                            .unwrap_or("No"),
                        dependencies.len(),
                        if dependencies.is_empty() {
                            "  (none)".to_string()
                        } else {
                            dependencies
                                .iter()
                                .map(|s| format!("  - {}", s))
                                .collect::<Vec<_>>()
                                .join("\n")
                        },
                        dependents.len(),
                        if dependents.is_empty() {
                            "  (none)".to_string()
                        } else {
                            dependents
                                .iter()
                                .map(|s| format!("  - {}", s))
                                .collect::<Vec<_>>()
                                .join("\n")
                        },
                        n.bridge_score,
                        n.degree
                    )
                } else {
                    format!("Module not found: {}", module_id)
                };

                Ok(McpToolResult {
                    content: vec![McpContent::text(impact)],
                    is_error: None,
                })
            }

            "get_symbol_context" => {
                let args = call.arguments;
                let module_id = args
                    .get("module_id")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing module_id")?
                    .to_string();
                let symbol_name = args
                    .get("symbol_name")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing symbol_name")?
                    .to_string();
                let detail_level = args
                    .get("detail_level")
                    .and_then(|v| v.as_str())
                    .map(String::from);

                let request = ModuleContextRequest {
                    module_id: module_id.clone(),
                    depth: None,
                    detail_level,
                    include: None,
                    format: None,
                };

                let mut response = api_state.get_module_context(&request)?;
                response.signatures.retain(|sig| {
                    sig.symbol_name.as_deref() == Some(symbol_name.as_str())
                });

                if response.signatures.is_empty() {
                    return Err(format!(
                        "Symbol '{}' not found in module '{}'",
                        symbol_name, module_id
                    ));
                }

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&response).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "get_blast_radius" => {
                let args = call.arguments;
                let target = args
                    .get("target")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing target")?;
                let max_related = args
                    .get("max_related")
                    .and_then(|v| v.as_u64())
                    .unwrap_or(10) as usize;

                // Rebuild graph to ensure edges are populated
                let graph = api_state.rebuild_graph()?;

                let node = graph
                    .nodes
                    .iter()
                    .find(|n| n.module_id == target || n.path.contains(target))
                    .ok_or_else(|| format!("Target not found: {}", target))?;
                let module_id = node.module_id.clone();

                let deps = api_state
                    .get_dependencies_internal(&module_id, 1)?
                    .unwrap_or_default();

                let dependents = api_state.get_dependents(&module_id)?;

                // Pre-fetch mapped_files once for LIP URI extraction.
                let files_snapshot = api_state.mapped_files.lock()
                    .map(|g| g.clone())
                    .unwrap_or_default();

                let lip_uris_for = |path: &str| -> Vec<String> {
                    files_snapshot.get(path)
                        .map(|mf| {
                            mf.signatures.iter()
                                .filter_map(|s| s.ckb_id.clone())
                                .collect()
                        })
                        .unwrap_or_default()
                };

                let mut related: Vec<serde_json::Value> = Vec::new();
                for dep in &deps {
                    if related.len() >= max_related {
                        break;
                    }
                    related.push(serde_json::json!({
                        "module_id": dep.module_id,
                        "path": dep.path,
                        "relationship": "dependency",
                        "lip_uris": lip_uris_for(&dep.path),
                    }));
                }
                for dep in &dependents {
                    if related.len() >= max_related {
                        break;
                    }
                    related.push(serde_json::json!({
                        "module_id": dep.module_id,
                        "path": dep.path,
                        "relationship": "dependent",
                        "lip_uris": lip_uris_for(&dep.path),
                    }));
                }

                let result = serde_json::json!({
                    "target": target,
                    "module_id": module_id,
                    "lip_uris": lip_uris_for(&node.path),
                    "related": related,
                });

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&result).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "find_files" => {
                let args = &call.arguments;
                let pattern = args
                    .get("pattern")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing pattern")?
                    .to_string();
                let limit = args
                    .get("limit")
                    .and_then(|v| v.as_u64())
                    .unwrap_or(200) as usize;

                let result =
                    crate::search::find_files(&api_state.root_path, &pattern, limit, &crate::search::FindOptions::default())
                        .map_err(|e| e)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&result).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "search_content" => {
                let args = &call.arguments;

                let pattern = args
                    .get("pattern")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing pattern")?
                    .to_string();

                // Build SearchOptions from the individual MCP arguments so callers
                // don't need to nest a JSON object — each option is a top-level field.
                let opts = crate::search::SearchOptions {
                    literal: args.get("literal").and_then(|v| v.as_bool()).unwrap_or(false),
                    case_sensitive: args.get("caseSensitive").and_then(|v| v.as_bool()).unwrap_or(true),
                    context_lines: args.get("contextLines").and_then(|v| v.as_u64()).unwrap_or(0) as usize,
                    before_context: args.get("beforeContext").and_then(|v| v.as_u64()).unwrap_or(0) as usize,
                    after_context: args.get("afterContext").and_then(|v| v.as_u64()).unwrap_or(0) as usize,
                    max_results: args.get("maxResults").and_then(|v| v.as_u64()).unwrap_or(100) as usize,
                    file_glob: args.get("fileGlob").and_then(|v| v.as_str()).map(String::from),
                    exclude_glob: args.get("excludeGlob").and_then(|v| v.as_str()).map(String::from),
                    invert_match: args.get("invertMatch").and_then(|v| v.as_bool()).unwrap_or(false),
                    word_regexp: args.get("wordRegexp").and_then(|v| v.as_bool()).unwrap_or(false),
                    only_matching: args.get("onlyMatching").and_then(|v| v.as_bool()).unwrap_or(false),
                    files_with_matches: args.get("filesWithMatches").and_then(|v| v.as_bool()).unwrap_or(false),
                    files_without_match: args.get("filesWithoutMatch").and_then(|v| v.as_bool()).unwrap_or(false),
                    count_only: args.get("countOnly").and_then(|v| v.as_bool()).unwrap_or(false),
                    no_ignore: args.get("noIgnore").and_then(|v| v.as_bool()).unwrap_or(false),
                    search_path: args.get("searchPath").and_then(|v| v.as_str()).map(String::from),
                    extra_patterns: args.get("extraPatterns")
                        .and_then(|v| v.as_array())
                        .map(|arr| arr.iter().filter_map(|x| x.as_str().map(String::from)).collect())
                        .unwrap_or_default(),
                };

                let result = api_state.search_content(&pattern, &opts)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&result).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "get_evolution" => {
                let days = call.arguments
                    .get("days")
                    .and_then(|v| v.as_u64())
                    .map(|d| d as u32);
                let result = api_state.get_evolution(days)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&result).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "watch_status" => {
                let state_path = api_state.root_path.join(".codecartographer_watch_state.json");
                let content = match std::fs::read_to_string(&state_path) {
                    Ok(s) => s,
                    Err(_) => r#"{"watching":false}"#.to_string(),
                };
                Ok(McpToolResult {
                    content: vec![McpContent::text(content)],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // Architectural analysis tools
            // -----------------------------------------------------------------

            "get_health" => {
                let graph = api_state.rebuild_graph()?;
                let m = &graph.metadata;
                let result = serde_json::json!({
                    "healthScore":         m.health_score,
                    "totalFiles":          m.total_files,
                    "totalEdges":          m.total_edges,
                    "bridgeCount":         m.bridge_count,
                    "cycleCount":          m.cycle_count,
                    "godModuleCount":      m.god_module_count,
                    "layerViolationCount": m.layer_violation_count,
                });
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            "get_cycles" => {
                let graph = api_state.rebuild_graph()?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&graph.cycles).unwrap_or_default())],
                    is_error: None,
                })
            }

            "check_layers" => {
                let graph = api_state.rebuild_graph()?;
                let result = serde_json::json!({
                    "violations":     graph.layer_violations,
                    "violationCount": graph.layer_violations.len(),
                });
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            "unreferenced_symbols" => {
                let graph = api_state.rebuild_graph()?;
                let files: Vec<serde_json::Value> = graph.nodes.iter()
                    .filter_map(|n| {
                        let exports = n.unreferenced_exports.as_ref()?;
                        if exports.is_empty() { return None; }
                        Some(serde_json::json!({ "path": n.path, "symbols": exports }))
                    })
                    .collect();
                let total: usize = files.iter()
                    .map(|f| f["symbols"].as_array().map(|a| a.len()).unwrap_or(0))
                    .sum();
                let result = serde_json::json!({ "totalCount": total, "files": files });
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            "simulate_change" => {
                let args = &call.arguments;
                let module_id = args.get("module_id").and_then(|v| v.as_str()).ok_or("Missing module_id")?.to_string();
                let new_sig = args.get("new_signature").and_then(|v| v.as_str()).map(str::to_string);
                let rem_sig = args.get("remove_signature").and_then(|v| v.as_str()).map(str::to_string);
                // Ensure graph is built before simulate_change
                let _ = api_state.rebuild_graph()?;
                let result = api_state.simulate_change(&module_id, new_sig.as_deref(), rem_sig.as_deref())?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // Context / skeleton tools
            // -----------------------------------------------------------------

            "skeleton_map" => {
                let args = &call.arguments;
                let detail = args.get("detail").and_then(|v| v.as_str()).unwrap_or("standard");
                // Rebuild graph ensures mapped_files is populated
                let _ = api_state.rebuild_graph()?;
                let files = api_state.mapped_files.lock().map_err(|e| e.to_string())?;
                // Compute churn labels for this render (not stored — files lock is immutable).
                let churn_map = crate::git_analysis::git_churn(&api_state.root_path, 300);
                let churn_labels: std::collections::HashMap<String, &'static str> = {
                    let mut counts: Vec<usize> = files.values()
                        .map(|f| *churn_map.get(&f.path).unwrap_or(&0))
                        .collect();
                    counts.sort_unstable();
                    let n = counts.len().max(1);
                    let hot_t = counts[n * 3 / 4];
                    let stable_t = counts[n / 4];
                    let max_c = *counts.last().unwrap_or(&0);
                    let mut labels = std::collections::HashMap::new();
                    if max_c > 0 && hot_t != stable_t {
                        for f in files.values() {
                            let c = *churn_map.get(&f.path).unwrap_or(&0);
                            if c > hot_t {
                                labels.insert(f.path.clone(), "hot");
                            } else if stable_t > 0 && c < stable_t {
                                labels.insert(f.path.clone(), "stable");
                            }
                        }
                    }
                    labels
                };
                let max_sigs = match detail {
                    "minimal"  => 5usize,
                    "extended" => usize::MAX,
                    _          => 20,
                };
                let tested_names = {
                    let mut names = std::collections::HashSet::new();
                    for mf in files.values() {
                        if crate::api::is_test_path(&mf.path) {
                            for s in &mf.signatures {
                                if let Some(n) = &s.symbol_name {
                                    let base = n.strip_prefix("test_")
                                        .or_else(|| n.strip_prefix("tests_"))
                                        .unwrap_or(n.as_str());
                                    let base = base
                                        .trim_end_matches("_works")
                                        .trim_end_matches("_fails")
                                        .trim_end_matches("_success")
                                        .trim_end_matches("_error")
                                        .trim_end_matches("_ok")
                                        .trim_end_matches("_err")
                                        .trim_end_matches("_test");
                                    if !base.is_empty() { names.insert(base.to_string()); }
                                }
                            }
                        }
                    }
                    names
                };
                let skeleton: Vec<serde_json::Value> = files.values().map(|mf| {
                    let is_test = crate::api::is_test_path(&mf.path);
                    let sigs: Vec<String> = mf.signatures.iter()
                        .take(max_sigs)
                        .map(|s| {
                            if let Some(body) = &s.body {
                                let decl = s.raw.trim_end_matches('{').trim_end();
                                format!("{} {{ {} }}", decl, body)
                            } else if !is_test && s.symbol_name.as_deref()
                                .map(|n| tested_names.contains(n))
                                .unwrap_or(false)
                            {
                                format!("{} // tested", s.raw)
                            } else {
                                format!("{} // ...", s.raw)
                            }
                        })
                        .collect();
                    let heat = churn_labels.get(&mf.path).copied().unwrap_or("");
                    serde_json::json!({
                        "path":       api_state.rel(&mf.path),
                        "heat":       heat,
                        "imports":    mf.imports,
                        "signatures": sigs,
                    })
                }).collect();
                let total_sigs: usize = files.values().map(|f| f.signatures.len()).sum();
                let est_tokens: usize = skeleton.iter()
                    .map(|f| serde_json::to_string(f).unwrap_or_default().len() / 4)
                    .sum();
                let result = serde_json::json!({
                    "files":             skeleton,
                    "totalFiles":        files.len(),
                    "totalSignatures":   total_sigs,
                    "estimatedTokens":   est_tokens,
                });
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            "ranked_skeleton" => {
                let args = &call.arguments;
                let focus_str = args.get("focus").and_then(|v| v.as_str()).unwrap_or("[]");
                let focus: Vec<String> = serde_json::from_str(focus_str).unwrap_or_default();
                let budget = args.get("budget").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
                // Ensure graph is built
                let _ = api_state.rebuild_graph()?;
                let result = api_state.ranked_skeleton(&focus, budget)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // Feature 4: focused_skeleton
            // -----------------------------------------------------------------

            "focused_skeleton" => {
                let args = &call.arguments;
                let focus = args.get("focus").and_then(|v| v.as_str())
                    .ok_or("Missing required parameter: focus")?;
                let depth = args.get("depth").and_then(|v| v.as_u64()).unwrap_or(1) as usize;
                let detail = args.get("detail").and_then(|v| v.as_str()).unwrap_or("standard");

                let _ = api_state.rebuild_graph()?;
                let files = api_state.mapped_files.lock().map_err(|e| e.to_string())?;
                let graph = api_state.project_graph.lock().map_err(|e| e.to_string())?;
                let graph = graph.as_ref().ok_or("Graph not initialized")?;

                // Fuzzy-match the focus string to a module_id.
                let seed = files.keys()
                    .find(|k| *k == focus || k.ends_with(focus) || k.contains(focus))
                    .map(|k| k.clone())
                    .ok_or_else(|| format!("No file matching '{}'", focus))?;

                let neighborhood = bfs_neighborhood(&seed, depth, &graph.edges);
                let churn_map = crate::git_analysis::git_churn(&api_state.root_path, 300);
                let churn_labels = compute_churn_labels(&churn_map, files.values().map(|f| f.path.as_str()));
                let tested_names = collect_tested_names(files.values());

                let max_sigs = match detail { "minimal" => 5, "extended" => usize::MAX, _ => 20 };
                let result_files: Vec<serde_json::Value> = neighborhood.iter()
                    .filter_map(|(module_id, role)| {
                        files.get(module_id).map(|mf| render_mf(mf, role, max_sigs, &churn_labels, &tested_names, &api_state.root_path))
                    })
                    .collect();

                let est_tokens: usize = result_files.iter()
                    .map(|f| serde_json::to_string(f).unwrap_or_default().len() / 4)
                    .sum();

                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&serde_json::json!({
                        "focus": seed,
                        "depth": depth,
                        "files": result_files,
                        "totalFiles": result_files.len(),
                        "estimatedTokens": est_tokens,
                    })).unwrap_or_default())],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // Feature 5: diff_skeleton
            // -----------------------------------------------------------------

            "diff_skeleton" => {
                let args = &call.arguments;
                let from = args.get("from").and_then(|v| v.as_str()).unwrap_or("HEAD~1");
                let to   = args.get("to").and_then(|v| v.as_str()).unwrap_or("HEAD");
                let include_importers = args.get("include_importers")
                    .and_then(|v| v.as_bool()).unwrap_or(true);

                let changed = crate::git_analysis::git_diff_files(&api_state.root_path, from, to);
                if changed.is_empty() {
                    return Ok(McpToolResult {
                        content: vec![McpContent::text(serde_json::to_string(&serde_json::json!({
                            "from": from, "to": to,
                            "changedFiles": [],
                            "files": [],
                            "totalFiles": 0,
                            "estimatedTokens": 0,
                        })).unwrap_or_default())],
                        is_error: None,
                    });
                }

                let _ = api_state.rebuild_graph()?;
                let files = api_state.mapped_files.lock().map_err(|e| e.to_string())?;
                let graph = api_state.project_graph.lock().map_err(|e| e.to_string())?;
                let graph = graph.as_ref().ok_or("Graph not initialized")?;

                let churn_map = crate::git_analysis::git_churn(&api_state.root_path, 300);
                let churn_labels = compute_churn_labels(&churn_map, files.values().map(|f| f.path.as_str()));
                let tested_names = collect_tested_names(files.values());

                // Seed: changed files that exist in the skeleton.
                let changed_paths: std::collections::HashSet<String> = changed.iter()
                    .map(|(p, _)| p.clone())
                    .collect();

                let mut neighborhood: Vec<(String, &'static str)> = Vec::new();
                let mut seen: std::collections::HashSet<String> = std::collections::HashSet::new();

                for module_id in files.keys() {
                    if changed_paths.contains(module_id.as_str())
                        || changed_paths.iter().any(|p| module_id.ends_with(p.as_str()))
                    {
                        if seen.insert(module_id.clone()) {
                            neighborhood.push((module_id.clone(), "changed"));
                        }
                    }
                }

                // Optionally add 1-hop importers of changed files.
                if include_importers {
                    let seeds: Vec<String> = neighborhood.iter().map(|(id, _)| id.clone()).collect();
                    for seed in &seeds {
                        for edge in &graph.edges {
                            if &edge.target == seed && seen.insert(edge.source.clone()) {
                                neighborhood.push((edge.source.clone(), "importer"));
                            }
                        }
                    }
                }

                let result_files: Vec<serde_json::Value> = neighborhood.iter()
                    .filter_map(|(module_id, role)| {
                        files.get(module_id).map(|mf| render_mf(mf, role, 20, &churn_labels, &tested_names, &api_state.root_path))
                    })
                    .collect();

                let est_tokens: usize = result_files.iter()
                    .map(|f| serde_json::to_string(f).unwrap_or_default().len() / 4)
                    .sum();

                let changed_list: Vec<serde_json::Value> = changed.iter()
                    .map(|(p, s)| serde_json::json!({"path": p, "status": s.to_string()}))
                    .collect();

                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&serde_json::json!({
                        "from": from,
                        "to": to,
                        "changedFiles": changed_list,
                        "files": result_files,
                        "totalFiles": result_files.len(),
                        "estimatedTokens": est_tokens,
                    })).unwrap_or_default())],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // search_skeleton
            // -----------------------------------------------------------------

            "search_skeleton" => {
                let args = &call.arguments;
                let pattern = args.get("pattern").and_then(|v| v.as_str())
                    .ok_or("Missing required parameter: pattern")?;
                let detail = args.get("detail").and_then(|v| v.as_str()).unwrap_or("standard");
                let budget = args.get("budget").and_then(|v| v.as_u64()).unwrap_or(0) as usize;

                let pat_lower = pattern.to_lowercase();
                let files = api_state.mapped_files.lock().map_err(|e| e.to_string())?;
                let churn_map = crate::git_analysis::git_churn(&api_state.root_path, 300);
                let churn_labels = compute_churn_labels(&churn_map, files.values().map(|f| f.path.as_str()));
                let tested_names = collect_tested_names(files.values());
                let max_sigs = match detail { "minimal" => 5, "extended" => usize::MAX, _ => 20 };

                // Collect matches; path matches take precedence over symbol matches.
                let mut matched: Vec<(&crate::mapper::MappedFile, &'static str)> = Vec::new();
                for mf in files.values() {
                    if mf.path.to_lowercase().contains(&pat_lower) {
                        matched.push((mf, "path"));
                    } else if mf.signatures.iter().any(|s| s.raw.to_lowercase().contains(&pat_lower)) {
                        matched.push((mf, "symbol"));
                    }
                }
                // Path matches first, then alphabetical within each group.
                matched.sort_by_key(|(mf, role)| (if *role == "path" { 0u8 } else { 1u8 }, mf.path.clone()));

                let total_matched = matched.len();
                let mut result_files: Vec<serde_json::Value> = Vec::new();
                let mut tokens_used: usize = 0;

                for (mf, role) in &matched {
                    let rendered = render_mf(mf, role, max_sigs, &churn_labels, &tested_names, &api_state.root_path);
                    let est = serde_json::to_string(&rendered).unwrap_or_default().len() / 4;
                    if budget > 0 && tokens_used + est > budget {
                        break;
                    }
                    tokens_used += est;
                    result_files.push(rendered);
                }

                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&serde_json::json!({
                        "pattern": pattern,
                        "matched": total_matched,
                        "returned": result_files.len(),
                        "files": result_files,
                        "estimatedTokens": tokens_used,
                    })).unwrap_or_default())],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // Git intelligence tools
            // -----------------------------------------------------------------

            "git_churn" => {
                let limit = call.arguments.get("limit").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
                let churn = crate::git_analysis::git_churn(&api_state.root_path, limit);
                // The tool contract is "sorted by churn count"; a HashMap serialises
                // in arbitrary order, so emit an explicitly descending array (ties
                // broken by path for determinism).
                let mut ranked: Vec<(&String, &usize)> = churn.iter().collect();
                ranked.sort_by(|a, b| b.1.cmp(a.1).then_with(|| a.0.cmp(b.0)));
                let out: Vec<serde_json::Value> = ranked
                    .into_iter()
                    .map(|(file, count)| serde_json::json!({ "file": file, "commits": count }))
                    .collect();
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&out).unwrap_or_default())],
                    is_error: None,
                })
            }

            "git_cochange" => {
                let args = &call.arguments;
                let limit     = args.get("limit").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
                let min_count = args.get("min_count").and_then(|v| v.as_u64()).unwrap_or(2) as usize;
                let pairs: Vec<serde_json::Value> = crate::git_analysis::git_cochange(&api_state.root_path, limit)
                    .into_iter()
                    .filter(|p| p.count >= min_count)
                    .map(|p| serde_json::json!({
                        "fileA": p.file_a, "fileB": p.file_b,
                        "count": p.count,  "couplingScore": p.coupling_score,
                    }))
                    .collect();
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&pairs).unwrap_or_default())],
                    is_error: None,
                })
            }

            "hidden_coupling" => {
                let args = &call.arguments;
                let limit     = args.get("limit").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
                let min_count = args.get("min_count").and_then(|v| v.as_u64()).unwrap_or(2) as usize;
                let graph = api_state.rebuild_graph()?;
                // Build set of existing import edges for fast lookup
                let edge_set: std::collections::HashSet<(String, String)> = graph.edges.iter()
                    .flat_map(|e| [
                        (e.source.clone(), e.target.clone()),
                        (e.target.clone(), e.source.clone()),
                    ])
                    .collect();
                let pairs: Vec<serde_json::Value> = crate::git_analysis::git_cochange(&api_state.root_path, limit)
                    .into_iter()
                    .filter(|p| p.count >= min_count)
                    .filter(|p| !edge_set.contains(&(p.file_a.clone(), p.file_b.clone())))
                    .map(|p| serde_json::json!({
                        "fileA": p.file_a, "fileB": p.file_b,
                        "count": p.count,  "couplingScore": p.coupling_score,
                    }))
                    .collect();
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&pairs).unwrap_or_default())],
                    is_error: None,
                })
            }

            "semidiff" => {
                let args = &call.arguments;
                let commit1 = args.get("commit1").and_then(|v| v.as_str()).ok_or("Missing commit1")?.to_string();
                let commit2 = args.get("commit2").and_then(|v| v.as_str()).unwrap_or("HEAD").to_string();
                let root = &api_state.root_path;
                let changed = crate::git_analysis::git_diff_files(root, &commit1, &commit2);
                let mut result: Vec<serde_json::Value> = Vec::new();
                for (file_path, status) in &changed {
                    let fake_path = std::path::Path::new(file_path);
                    let before = if *status != 'A' {
                        crate::git_analysis::git_show_file(root, &commit1, file_path)
                            .map(|c| crate::mapper::extract_skeleton(fake_path, &c).signatures
                                .iter().map(|s| s.raw.clone()).collect::<Vec<_>>())
                            .unwrap_or_default()
                    } else { vec![] };
                    let after = if *status != 'D' {
                        crate::git_analysis::git_show_file(root, &commit2, file_path)
                            .map(|c| crate::mapper::extract_skeleton(fake_path, &c).signatures
                                .iter().map(|s| s.raw.clone()).collect::<Vec<_>>())
                            .unwrap_or_default()
                    } else { vec![] };
                    let before_set: std::collections::HashSet<&str> = before.iter().map(String::as_str).collect();
                    let after_set:  std::collections::HashSet<&str> = after.iter().map(String::as_str).collect();
                    let added:   Vec<&str> = after.iter().filter(|s| !before_set.contains(s.as_str())).map(String::as_str).collect();
                    let removed: Vec<&str> = before.iter().filter(|s| !after_set.contains(s.as_str())).map(String::as_str).collect();
                    result.push(serde_json::json!({
                        "path": file_path,
                        "status": match status { 'A' => "added", 'D' => "deleted", _ => "modified" },
                        "added":   added,
                        "removed": removed,
                    }));
                }
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            "poll_changes" => {
                let since_ms = call.arguments.get("since_ms").and_then(|v| v.as_u64()).unwrap_or(0);
                use std::time::{Duration, SystemTime, UNIX_EPOCH};
                let threshold_ms = if since_ms == 0 {
                    SystemTime::now().duration_since(UNIX_EPOCH).unwrap_or_default()
                        .as_millis().saturating_sub(60_000) as u64
                } else {
                    since_ms
                };
                let threshold = UNIX_EPOCH + Duration::from_millis(threshold_ms);
                let now_ms = SystemTime::now().duration_since(UNIX_EPOCH).unwrap_or_default().as_millis() as u64;
                let scan = crate::scanner::scan_files_with_noise_tracking(&api_state.root_path)
                    .map_err(|e| e.to_string())?;
                let changed: Vec<String> = scan.files.iter()
                    .filter(|p| !crate::scanner::is_ignored_path(p))
                    .filter_map(|p| {
                        let mtime = std::fs::metadata(p).ok()?.modified().ok()?;
                        if mtime > threshold {
                            let rel = p.strip_prefix(&api_state.root_path).unwrap_or(p)
                                .to_string_lossy().replace('\\', "/");
                            Some(rel)
                        } else { None }
                    })
                    .collect();
                let result = serde_json::json!({ "changedFiles": changed, "checkedAtMs": now_ms });
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // Surgical editing tools
            // -----------------------------------------------------------------

            "replace_content" => {
                let args = &call.arguments;
                let pattern = args.get("pattern").and_then(|v| v.as_str()).ok_or("Missing pattern")?.to_string();
                let replacement = args.get("replacement").and_then(|v| v.as_str()).ok_or("Missing replacement")?.to_string();
                let opts = crate::search::ReplaceOptions {
                    literal:       args.get("literal").and_then(|v| v.as_bool()).unwrap_or(false),
                    case_sensitive: args.get("caseSensitive").and_then(|v| v.as_bool()).unwrap_or(true),
                    dry_run:       args.get("dryRun").and_then(|v| v.as_bool()).unwrap_or(false),
                    backup:        false,
                    context_lines: args.get("contextLines").and_then(|v| v.as_u64()).unwrap_or(3) as usize,
                    file_glob:     args.get("fileGlob").and_then(|v| v.as_str()).map(String::from),
                    exclude_glob:  args.get("excludeGlob").and_then(|v| v.as_str()).map(String::from),
                    search_path:   args.get("searchPath").and_then(|v| v.as_str()).map(String::from),
                    max_per_file:  args.get("maxPerFile").and_then(|v| v.as_u64()).unwrap_or(0) as usize,
                    ..Default::default()
                };
                let result = crate::search::replace_content(&api_state.root_path, &pattern, &replacement, &opts)
                    .map_err(|e| e)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            "extract_content" => {
                let args = &call.arguments;
                let pattern = args.get("pattern").and_then(|v| v.as_str()).ok_or("Missing pattern")?.to_string();
                let groups: Vec<usize> = args.get("groups")
                    .and_then(|v| v.as_str())
                    .and_then(|s| serde_json::from_str(s).ok())
                    .unwrap_or_default();
                let opts = crate::search::ExtractOptions {
                    groups,
                    count:         args.get("count").and_then(|v| v.as_bool()).unwrap_or(false),
                    dedup:         args.get("dedup").and_then(|v| v.as_bool()).unwrap_or(false),
                    sort:          args.get("sort").and_then(|v| v.as_bool()).unwrap_or(false),
                    case_sensitive: args.get("caseSensitive").and_then(|v| v.as_bool()).unwrap_or(true),
                    file_glob:     args.get("fileGlob").and_then(|v| v.as_str()).map(String::from),
                    search_path:   args.get("searchPath").and_then(|v| v.as_str()).map(String::from),
                    limit:         args.get("limit").and_then(|v| v.as_u64()).unwrap_or(1000) as usize,
                    ..Default::default()
                };
                let result = crate::search::extract_content(&api_state.root_path, &pattern, &opts)
                    .map_err(|e| e)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            "query_context" => {
                let args = &call.arguments;
                let query = args
                    .get("query")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing query")?
                    .to_string();
                let budget = args.get("budget").and_then(|v| v.as_u64()).unwrap_or(8000) as usize;
                let max_search = args.get("maxSearchResults").and_then(|v| v.as_u64()).unwrap_or(20) as usize;
                let model_str = args.get("model").and_then(|v| v.as_str()).unwrap_or("claude").to_string();

                // Step 1: rank files by BM25 relevance to the query. Rank over the SYMBOL
                // corpus first (names + qualified names + signatures + doc-comments) so the
                // query matches code intent, not string-literal/comment noise — the key fix
                // for macro-heavy languages like C++. Fall back to raw-content BM25 only if
                // the symbol corpus finds nothing.
                let bm25_opts = crate::search::BM25Options {
                    max_results: max_search,
                    ..Default::default()
                };
                let _ = api_state.rebuild_graph();
                let sym_hits: Vec<String> = {
                    let files = api_state.mapped_files.lock().map_err(|e| e.to_string())?;
                    crate::search::bm25_search_symbols(
                        files.iter().map(|(k, v)| (k.as_str(), v)),
                        &query,
                        &bm25_opts,
                    )
                    .matches
                    .into_iter()
                    .map(|m| m.path)
                    .collect()
                };
                let focus_files: Vec<String> = if !sym_hits.is_empty() {
                    sym_hits
                } else {
                    match crate::search::bm25_search(&api_state.root_path, &query, &bm25_opts) {
                        Ok(sr) => sr.matches.into_iter().map(|m| m.path).collect(),
                        Err(_) => vec![],
                    }
                };
                // query_context injects CODE context — bias toward source files and drop
                // docs (which win BM25 on conceptual terms). Use query_docs for doc context.
                let focus_files: Vec<String> = {
                    let code: Vec<String> = focus_files
                        .iter()
                        .filter(|p| !crate::api::is_doc_path(p))
                        .cloned()
                        .collect();
                    if code.is_empty() { focus_files } else { code }
                };

                // Step 2: ranked skeleton personalised to those files
                let ranked = api_state.ranked_skeleton(&focus_files, budget)
                    .map_err(|e| e)?;

                // Step 3: build context text
                let mut context_text = format!("## Ranked Context for: {}\n\n", query);
                let total_tokens: usize = ranked.iter().map(|f| f.estimated_tokens).sum();
                let sig_count: usize = ranked.iter().map(|f| f.signatures.len()).sum();

                for f in &ranked {
                    context_text.push_str(&format!(
                        "// {} (rank: {:.4}, {} tokens)\n",
                        f.path, f.rank, f.estimated_tokens
                    ));
                    for sig in &f.signatures {
                        context_text.push_str(&format!("  {}\n", sig));
                    }
                    context_text.push('\n');
                }

                // Step 4: score the bundle
                let model = model_str
                    .parse::<crate::token_metrics::ModelFamily>()
                    .unwrap_or_default();
                let health_opts = crate::token_metrics::HealthOpts {
                    model,
                    window_size: 0,
                    key_positions: crate::token_metrics::key_positions_from_order(
                        &ranked.iter().map(|f| f.path.clone()).collect::<Vec<_>>(),
                        &focus_files,
                    ),
                    signature_count: sig_count,
                    signature_tokens: (total_tokens as f64 * 0.85) as usize, // approximate
                };
                let health = crate::token_metrics::analyze(&context_text, &health_opts);

                let result = serde_json::json!({
                    "context": context_text,
                    "filesUsed": ranked.iter().map(|f| &f.path).collect::<Vec<_>>(),
                    "focusFiles": focus_files,
                    "totalTokens": total_tokens,
                    "health": health,
                });

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&result).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "shotgun_surgery" => {
                let args = &call.arguments;
                let commits = args.get("commits").and_then(|v| v.as_u64()).unwrap_or(500) as usize;
                let max_results = args.get("maxResults").and_then(|v| v.as_u64()).unwrap_or(20) as usize;
                let min_partners = args.get("minPartners").and_then(|v| v.as_u64()).unwrap_or(3) as usize;

                let mut entries = crate::git_analysis::git_cochange_dispersion(
                    &api_state.root_path,
                    commits,
                );
                entries.retain(|e| e.partner_count >= min_partners);
                entries.truncate(max_results);

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&entries).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "context_health" => {
                let args = &call.arguments;
                let content = args
                    .get("content")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing content")?
                    .to_string();

                let model = args
                    .get("model")
                    .and_then(|v| v.as_str())
                    .and_then(|s| s.parse::<crate::token_metrics::ModelFamily>().ok())
                    .unwrap_or_default();

                let opts = crate::token_metrics::HealthOpts {
                    model,
                    window_size: args.get("windowSize").and_then(|v| v.as_u64()).unwrap_or(0) as usize,
                    signature_count: args.get("signatureCount").and_then(|v| v.as_u64()).unwrap_or(0) as usize,
                    signature_tokens: args.get("signatureTokens").and_then(|v| v.as_u64()).unwrap_or(0) as usize,
                    key_positions: args
                        .get("keyPositions")
                        .and_then(|v| v.as_str())
                        .and_then(|s| serde_json::from_str::<Vec<f64>>(s).ok())
                        .unwrap_or_default(),
                };

                let mut report = crate::token_metrics::analyze(&content, &opts);

                // Populate CARTOGRAPHER.md [commands] preset names
                let cmds = crate::token_metrics::parse_cartographer_commands(&api_state.root_path);
                if !cmds.is_empty() {
                    let preset_names: Vec<String> = cmds.into_keys().collect();
                    report.commands = Some(preset_names);
                }

                // Warn if any preset command references a file in a detected cycle
                if let Some(ref preset_names_ref) = report.commands.clone() {
                    if let Ok(graph) = api_state.rebuild_graph() {
                        let cycle_files: std::collections::HashSet<String> = graph.cycles.iter()
                            .flat_map(|c| c.nodes.iter().cloned())
                            .collect();
                        let cmd_map = crate::token_metrics::parse_cartographer_commands(&api_state.root_path);
                        for preset_name in preset_names_ref {
                            if let Some(cmd) = cmd_map.get(preset_name) {
                                let references_cycle = cycle_files.iter().any(|f| cmd.contains(f.as_str()));
                                if references_cycle {
                                    report.warnings.push(format!(
                                        "preset '{}' references a file in a dependency cycle",
                                        preset_name
                                    ));
                                }
                            }
                        }
                    }
                }

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&report).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "watch_graph" => {
                use notify::{RecursiveMode, Watcher};
                use std::sync::mpsc;
                use std::time::{Duration, Instant, SystemTime, UNIX_EPOCH};

                let args = &call.arguments;
                let root_str = args
                    .get("root")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing root")?
                    .to_string();
                let timeout_secs = args
                    .get("timeout_secs")
                    .and_then(|v| v.as_u64())
                    .unwrap_or(30)
                    .min(300);

                let watch_path = std::path::PathBuf::from(&root_str);
                let (tx, rx) = mpsc::channel();

                let mut watcher = notify::recommended_watcher(move |res: notify::Result<notify::Event>| {
                    if let Ok(event) = res {
                        let _ = tx.send(event);
                    }
                }).map_err(|e| format!("Failed to create watcher: {}", e))?;

                watcher.watch(&watch_path, RecursiveMode::Recursive)
                    .map_err(|e| format!("Failed to watch {}: {}", root_str, e))?;

                let source_extensions: std::collections::HashSet<&str> =
                    ["rs", "go", "py", "ts", "js", "dart"].iter().copied().collect();

                let deadline = Instant::now() + Duration::from_secs(timeout_secs);
                let mut event_count = 0u64;

                while Instant::now() < deadline {
                    let remaining = deadline.saturating_duration_since(Instant::now());
                    if remaining.is_zero() { break; }
                    let timeout = remaining.min(Duration::from_millis(100));
                    match rx.recv_timeout(timeout) {
                        Ok(event) => {
                            for path in &event.paths {
                                let ext = path.extension()
                                    .and_then(|e| e.to_str())
                                    .unwrap_or("");
                                if !source_extensions.contains(ext) {
                                    continue;
                                }
                                let timestamp_ms = SystemTime::now()
                                    .duration_since(UNIX_EPOCH)
                                    .unwrap_or_default()
                                    .as_millis() as u64;
                                let graph_event = GraphEvent {
                                    kind: GraphEventKind::FileReindexed,
                                    path: path.to_string_lossy().to_string(),
                                    timestamp_ms,
                                };
                                println!("{}", serde_json::to_string(&graph_event).unwrap_or_default());
                                event_count += 1;
                            }
                        }
                        Err(mpsc::RecvTimeoutError::Timeout) => continue,
                        Err(mpsc::RecvTimeoutError::Disconnected) => break,
                    }
                }

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&serde_json::json!({
                            "events_emitted": event_count,
                            "timeout_secs":   timeout_secs,
                            "root":           root_str,
                        })).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // search_in_symbol — scope a search to one function's body
            // -----------------------------------------------------------------
            "search_in_symbol" => {
                let args = &call.arguments;
                let file    = args.get("file").and_then(|v| v.as_str()).ok_or("Missing file")?;
                let symbol  = args.get("symbol").and_then(|v| v.as_str()).ok_or("Missing symbol")?;
                let pattern = args.get("pattern").and_then(|v| v.as_str()).ok_or("Missing pattern")?;
                let ctx     = args.get("context_lines").and_then(|v| v.as_u64()).unwrap_or(2) as usize;

                // 1. Locate the file in the skeleton index
                let files = api_state.mapped_files.lock().map(|g| g.clone()).unwrap_or_default();
                let mf = files.values()
                    .find(|f| f.path == file || f.path.contains(file))
                    .ok_or_else(|| format!("File not found: {}", file))?;

                // 2. Find the symbol (symbol_name, qualified_name, or raw text)
                let sig = mf.signatures.iter()
                    .find(|s| {
                        s.symbol_name.as_deref() == Some(symbol)
                            || s.qualified_name.as_deref() == Some(symbol)
                            || s.raw.contains(symbol)
                    })
                    .ok_or_else(|| format!("Symbol '{}' not found in {}", symbol, file))?;

                let sym_start = sig.line_start; // 0-indexed

                // 3. Estimate end: next symbol's line_start, fallback +500
                let sym_end = mf.signatures.iter()
                    .filter(|s| s.line_start > sym_start)
                    .map(|s| s.line_start)
                    .min()
                    .unwrap_or(sym_start + 500);

                // 4. Content search scoped to this file by glob
                let fname = std::path::Path::new(&mf.path)
                    .file_name().and_then(|n| n.to_str()).unwrap_or(file);
                let opts = crate::search::SearchOptions {
                    case_sensitive: true,
                    context_lines: ctx,
                    max_results: 500,
                    file_glob: Some(format!("**/{}", fname)),
                    ..Default::default()
                };
                let sr = crate::search::search_content(&api_state.root_path, pattern, &opts)
                    .map_err(|e| e)?;

                // 5. Filter to the symbol's estimated line range (convert 0-indexed → 1-indexed)
                let in_range: Vec<_> = sr.matches.into_iter()
                    .filter(|m| m.line_number > sym_start && m.line_number <= sym_end + 1)
                    .collect();
                let match_count = in_range.len();

                let result = serde_json::json!({
                    "file": mf.path,
                    "symbol": symbol,
                    "symbol_kind": format!("{:?}", sig.kind),
                    "symbol_line": sym_start + 1,
                    "estimated_end_line": sym_end + 1,
                    "pattern": pattern,
                    "match_count": match_count,
                    "matches": in_range,
                });
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // list_key_handlers — TUI key-binding map
            // -----------------------------------------------------------------
            "list_key_handlers" => {
                let args = &call.arguments;
                let file = args.get("file").and_then(|v| v.as_str()).ok_or("Missing file")?;
                let ctx  = args.get("context_lines").and_then(|v| v.as_u64()).unwrap_or(4) as usize;

                let files = api_state.mapped_files.lock().map(|g| g.clone()).unwrap_or_default();
                let mf = files.values()
                    .find(|f| f.path == file || f.path.contains(file))
                    .ok_or_else(|| format!("File not found: {}", file))?;
                let fname = std::path::Path::new(&mf.path)
                    .file_name().and_then(|n| n.to_str()).unwrap_or(file);
                let glob = format!("**/{}", fname);

                // Search for both dominant key-handler syntaxes
                let mut all_matches = Vec::new();
                for pattern in &[r#"case ""#, r#"== ""#] {
                    let opts = crate::search::SearchOptions {
                        case_sensitive: true,
                        context_lines: ctx,
                        max_results: 300,
                        file_glob: Some(glob.clone()),
                        ..Default::default()
                    };
                    if let Ok(sr) = crate::search::search_content(&api_state.root_path, pattern, &opts) {
                        all_matches.extend(sr.matches);
                    }
                }

                // Group by extracted key string (BTreeMap keeps keys sorted)
                let mut key_map: std::collections::BTreeMap<String, Vec<serde_json::Value>> =
                    std::collections::BTreeMap::new();
                for m in &all_matches {
                    if let Some(key) = extract_quoted_key(&m.line) {
                        key_map.entry(key).or_default().push(serde_json::json!({
                            "line":           m.line_number,
                            "text":           m.line.trim(),
                            "before_context": m.before_context,
                            "after_context":  m.after_context,
                        }));
                    }
                }

                let handlers: Vec<_> = key_map.iter().map(|(k, v)| serde_json::json!({
                    "key":         k,
                    "occurrences": v,
                })).collect();

                let result = serde_json::json!({
                    "file":          mf.path,
                    "handler_count": handlers.len(),
                    "handlers":      handlers,
                });
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // map_state_machine — state × key-handlers matrix
            // -----------------------------------------------------------------
            "map_state_machine" => {
                let args = &call.arguments;
                let file         = args.get("file").and_then(|v| v.as_str()).ok_or("Missing file")?;
                let state_var    = args.get("state_var").and_then(|v| v.as_str()).unwrap_or("m.state").to_string();
                let state_prefix = args.get("state_prefix").and_then(|v| v.as_str()).unwrap_or("State").to_string();

                let files = api_state.mapped_files.lock().map(|g| g.clone()).unwrap_or_default();
                let mf = files.values()
                    .find(|f| f.path == file || f.path.contains(file))
                    .ok_or_else(|| format!("File not found: {}", file))?;
                let fname = std::path::Path::new(&mf.path)
                    .file_name().and_then(|n| n.to_str()).unwrap_or(file);
                let glob = format!("**/{}", fname);

                // Helper: build SearchOptions for this file
                let make_opts = |max: usize| crate::search::SearchOptions {
                    case_sensitive: true,
                    max_results: max,
                    file_glob: Some(glob.clone()),
                    ..Default::default()
                };

                // 1. Find all state enum variants by searching for the prefix
                let mut known_states: Vec<String> = Vec::new();
                if let Ok(sr) = crate::search::search_content(
                    &api_state.root_path, &state_prefix, &make_opts(300))
                {
                    for m in &sr.matches {
                        let mut pos = 0usize;
                        while pos < m.line.len() {
                            if let Some(idx) = m.line[pos..].find(&state_prefix as &str) {
                                let abs = pos + idx;
                                let rest = &m.line[abs..];
                                let end = rest.find(|c: char| !c.is_alphanumeric() && c != '_')
                                    .unwrap_or(rest.len());
                                let name = &rest[..end];
                                if name.len() > state_prefix.len() {
                                    let name = name.to_string();
                                    if !known_states.contains(&name) {
                                        known_states.push(name);
                                    }
                                }
                                pos = abs + 1;
                            } else {
                                break;
                            }
                        }
                    }
                }

                // 2. Find all state guard locations: `state_var == `
                let guard_pattern = format!("{} == ", state_var);
                let mut guard_map: HashMap<String, Vec<usize>> = HashMap::new();
                if let Ok(sr) = crate::search::search_content(
                    &api_state.root_path, &guard_pattern, &make_opts(500))
                {
                    for m in &sr.matches {
                        for state in &known_states {
                            if m.line.contains(state.as_str()) {
                                guard_map.entry(state.clone()).or_default().push(m.line_number);
                            }
                        }
                    }
                }

                // 3. Collect all key handler matches
                let mut all_key_matches = Vec::new();
                for pattern in &[r#"case ""#, r#"== ""#] {
                    if let Ok(sr) = crate::search::search_content(
                        &api_state.root_path, pattern, &make_opts(500))
                    {
                        all_key_matches.extend(sr.matches);
                    }
                }

                // 4. For each state, attribute key handlers within WINDOW lines of a guard
                const WINDOW: usize = 60;
                let mut state_handlers: serde_json::Map<String, serde_json::Value> =
                    serde_json::Map::new();

                for state in &known_states {
                    let guard_lines = guard_map.get(state).cloned().unwrap_or_default();
                    let mut keys: Vec<String> = Vec::new();
                    let mut handler_details: Vec<serde_json::Value> = Vec::new();

                    for &guard_ln in &guard_lines {
                        for km in &all_key_matches {
                            if km.line_number > guard_ln && km.line_number < guard_ln + WINDOW {
                                if let Some(key) = extract_quoted_key(&km.line) {
                                    if !keys.contains(&key) {
                                        keys.push(key.clone());
                                        handler_details.push(serde_json::json!({
                                            "key":  key,
                                            "line": km.line_number,
                                            "text": km.line.trim(),
                                        }));
                                    }
                                }
                            }
                        }
                    }

                    state_handlers.insert(state.clone(), serde_json::json!({
                        "guard_lines": guard_lines,
                        "keys":        keys,
                        "handlers":    handler_details,
                    }));
                }

                let result = serde_json::json!({
                    "file":           mf.path,
                    "state_var":      state_var,
                    "state_prefix":   state_prefix,
                    "states":         known_states,
                    "state_handlers": state_handlers,
                });
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // doc_index — list all document nodes with structure + edges
            // -----------------------------------------------------------------
            "doc_index" => {
                let docs = api_state.doc_nodes()?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&docs).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // doc_context — single doc + referenced code skeletons
            // -----------------------------------------------------------------
            "doc_context" => {
                let args = &call.arguments;
                let doc_path = args.get("doc_path").and_then(|v| v.as_str())
                    .ok_or("Missing doc_path")?;
                let budget = args.get("budget").and_then(|v| v.as_u64()).unwrap_or(4000) as usize;

                // Rebuild graph so edges exist
                if let Err(e) = api_state.rebuild_graph() {
                    return Err(e);
                }

                // Find the doc in mapped_files (exact match or substring)
                let files = api_state.mapped_files.lock().map_err(|e| e.to_string())?;
                let (module_id, mf) = files.iter()
                    .find(|(_, f)| f.path == doc_path || f.path.contains(doc_path))
                    .ok_or_else(|| format!("Document not found: {}", doc_path))?;

                let doc_sigs: Vec<String> = mf.signatures.iter().map(|s| s.raw.clone()).collect();
                let doc_imports = mf.imports.clone();
                let doc_path_owned = mf.path.clone();
                let module_id_owned = module_id.clone();

                // Drop the lock before calling ranked_skeleton
                drop(files);

                // Use the doc's imports as focus files for ranked skeleton
                let focus: Vec<String> = doc_imports.clone();
                let ranked = if focus.is_empty() {
                    vec![]
                } else {
                    api_state.ranked_skeleton(&focus, budget).unwrap_or_default()
                };

                let total_tokens: usize = ranked.iter().map(|f| f.estimated_tokens).sum();

                let referenced: Vec<serde_json::Value> = ranked.iter().map(|f| {
                    serde_json::json!({
                        "path": f.path,
                        "rank": f.rank,
                        "signatureCount": f.signature_count,
                        "estimatedTokens": f.estimated_tokens,
                        "signatures": f.signatures,
                    })
                }).collect();

                let result = serde_json::json!({
                    "doc": {
                        "path": doc_path_owned,
                        "moduleId": module_id_owned,
                        "signatures": doc_sigs,
                        "imports": doc_imports,
                    },
                    "referencedFiles": referenced,
                    "totalTokens": total_tokens,
                });

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&result).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // query_docs — doc-biased context retrieval
            // -----------------------------------------------------------------
            "query_docs" => {
                let args = &call.arguments;
                let query = args.get("query").and_then(|v| v.as_str())
                    .ok_or("Missing query")?.to_string();
                let budget = args.get("budget").and_then(|v| v.as_u64()).unwrap_or(8000) as usize;
                let model_str = args.get("model").and_then(|v| v.as_str())
                    .unwrap_or("claude").to_string();

                // Rebuild graph
                if let Err(e) = api_state.rebuild_graph() {
                    return Err(e);
                }

                // Step 1: BM25 search across all files
                let bm25_opts = crate::search::BM25Options {
                    max_results: 30,
                    ..Default::default()
                };
                let bm25_result = crate::search::bm25_search(
                    &api_state.root_path, &query, &bm25_opts,
                ).unwrap_or_default();

                // Step 2: Separate into doc files and code files
                let mut doc_files: Vec<String> = Vec::new();
                let mut code_files: Vec<String> = Vec::new();
                let mut seen = std::collections::HashSet::new();

                for m in &bm25_result.matches {
                    if !seen.insert(m.path.clone()) { continue; }
                    if crate::api::is_doc_path(&m.path) {
                        doc_files.push(m.path.clone());
                    } else {
                        code_files.push(m.path.clone());
                    }
                }

                // Step 3: Follow doc cross-refs into code
                let files = api_state.mapped_files.lock().map_err(|e| e.to_string())?;
                let mut ref_code: Vec<String> = Vec::new();
                for doc_path in &doc_files {
                    if let Some(mf) = files.get(doc_path.as_str()) {
                        for imp in &mf.imports {
                            if !seen.contains(imp) && !crate::api::is_doc_path(imp) {
                                seen.insert(imp.clone());
                                ref_code.push(imp.clone());
                            }
                        }
                    }
                }
                drop(files);

                // Merge: doc imports come after direct code hits
                code_files.extend(ref_code);

                // Step 4: Build ranked skeleton — docs as primary focus, code as secondary
                let mut all_focus = doc_files.clone();
                all_focus.extend(code_files.iter().cloned());
                all_focus.truncate(30);

                let ranked = api_state.ranked_skeleton(&all_focus, budget)
                    .unwrap_or_default();

                // Step 5: Build context text — docs first, then code
                let mut doc_entries = Vec::new();
                let mut code_entries = Vec::new();
                let mut context_text = format!("## Doc Context for: {}\n\n", query);
                let mut total_tokens = 0usize;

                for f in &ranked {
                    let entry = serde_json::json!({
                        "path": f.path,
                        "rank": f.rank,
                        "signatureCount": f.signature_count,
                        "estimatedTokens": f.estimated_tokens,
                        "signatures": f.signatures,
                    });
                    total_tokens += f.estimated_tokens;

                    if crate::api::is_doc_path(&f.path) {
                        context_text.push_str(&format!(
                            "// [DOC] {} (rank: {:.4}, {} tokens)\n", f.path, f.rank, f.estimated_tokens
                        ));
                        doc_entries.push(entry);
                    } else {
                        context_text.push_str(&format!(
                            "// {} (rank: {:.4}, {} tokens)\n", f.path, f.rank, f.estimated_tokens
                        ));
                        code_entries.push(entry);
                    }
                    for sig in &f.signatures {
                        context_text.push_str(&format!("  {}\n", sig));
                    }
                    context_text.push('\n');
                }

                // Step 6: Health score
                let sig_count: usize = ranked.iter().map(|f| f.signatures.len()).sum();
                let model = model_str.parse::<crate::token_metrics::ModelFamily>().unwrap_or_default();
                let health_opts = crate::token_metrics::HealthOpts {
                    model,
                    window_size: 0,
                    key_positions: crate::token_metrics::key_positions_from_order(
                        &ranked.iter().map(|f| f.path.clone()).collect::<Vec<_>>(),
                        &doc_files,
                    ),
                    signature_count: sig_count,
                    signature_tokens: (total_tokens as f64 * 0.85) as usize,
                };
                let health = crate::token_metrics::analyze(&context_text, &health_opts);

                let result = serde_json::json!({
                    "context": context_text,
                    "docFiles": doc_entries,
                    "codeFiles": code_entries,
                    "focusDocs": doc_files,
                    "totalTokens": total_tokens,
                    "health": health,
                });

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string(&result).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "answer_question" => {
                let args = &call.arguments;
                let question = args.get("question").and_then(|v| v.as_str())
                    .ok_or("Missing required parameter: question")?.to_string();
                let max_items = args.get("maxItems").and_then(|v| v.as_u64()).unwrap_or(6) as usize;
                let budget = args.get("budget").and_then(|v| v.as_u64()).unwrap_or(8000) as usize;
                let show_body = args.get("showBody").and_then(|v| v.as_bool()).unwrap_or(true);

                if let Err(e) = api_state.rebuild_graph() {
                    return Err(e);
                }
                let files_lock = api_state.mapped_files.lock().map_err(|e| e.to_string())?;
                // The answer pipeline is built around repo-relative paths (git lookups,
                // root.join(file) reads, BM25 paths). MappedFile.path is absolute, but the
                // map key IS the repo-relative module_id — use it so BM25 candidates match.
                let mapped: Vec<crate::mapper::MappedFile> = files_lock
                    .iter()
                    .map(|(id, mf)| {
                        let mut m = mf.clone();
                        m.path = id.clone();
                        m
                    })
                    .collect();
                drop(files_lock);

                let opts = crate::answer::AnswerOptions {
                    budget,
                    max_items,
                    show_top_body: show_body,
                };

                let result = crate::answer::build_answer(
                    &api_state.root_path, &mapped, &question, &opts,
                );
                let rendered = crate::answer::render_answer(&result);
                let meta = serde_json::json!({
                    "tokensUsed": result.tokens_used,
                    "itemCount": result.items.len(),
                    "filesSearched": result.files_searched,
                    "budgetHit": result.budget_hit,
                });
                let output = format!(
                    "{}\n---\n{}",
                    rendered,
                    serde_json::to_string(&meta).unwrap_or_default()
                );
                Ok(McpToolResult {
                    content: vec![McpContent::text(output)],
                    is_error: None,
                })
            }

            "reach_symbol" => {
                let args = &call.arguments;
                let symbol = args.get("symbol").and_then(|v| v.as_str())
                    .ok_or("Missing required parameter: symbol")?.to_string();
                let file_filter = args.get("file").and_then(|v| v.as_str()).map(|s| s.to_string());
                let depth = args.get("depth").and_then(|v| v.as_u64()).unwrap_or(2) as usize;
                let budget = args.get("budget").and_then(|v| v.as_u64()).unwrap_or(6000) as usize;
                let include_tests = args.get("includeTests").and_then(|v| v.as_bool()).unwrap_or(false);
                let show_body = args.get("showBody").and_then(|v| v.as_bool()).unwrap_or(false);

                // Ensure the skeleton is current.
                if let Err(e) = api_state.rebuild_graph() {
                    return Err(e);
                }
                let files_lock = api_state.mapped_files.lock().map_err(|e| e.to_string())?;
                let mapped: Vec<crate::mapper::MappedFile> = files_lock.values().cloned().collect();
                drop(files_lock);

                let opts = crate::reach::ReachOptions {
                    depth,
                    budget,
                    file_filter,
                    include_tests,
                    show_body,
                    ..Default::default()
                };

                match crate::reach::build_reach(&api_state.root_path, &mapped, &symbol, &opts) {
                    Ok(result) => {
                        let rendered = crate::reach::render_reach(&result);
                        let meta = serde_json::json!({
                            "tokensUsed": result.tokens_used,
                            "budgetHit": result.budget_hit,
                            "callerCount": result.callers.len(),
                            "calleeCount": result.callees.len(),
                            "testCallersCollapsed": result.test_callers_collapsed,
                            "languageNote": result.language_note,
                        });
                        let output = format!(
                            "{}\n---\n{}",
                            rendered,
                            serde_json::to_string(&meta).unwrap_or_default()
                        );
                        Ok(McpToolResult {
                            content: vec![McpContent::text(output)],
                            is_error: None,
                        })
                    }
                    // Ambiguity is not a failure — return the candidate list so the
                    // caller can pick one by passing `file` (or `target`), no retry-as-error.
                    Err(crate::reach::ReachError::Ambiguous(candidates)) => {
                        let mut text = format!(
                            "Symbol '{}' is defined in {} places. Re-run with `file` (or `target`) set to one of:\n",
                            symbol,
                            candidates.len()
                        );
                        for c in &candidates {
                            text.push_str(&format!("  {}:{} — {}\n", c.file, c.line, c.sig));
                        }
                        Ok(McpToolResult {
                            content: vec![McpContent::text(text)],
                            is_error: None,
                        })
                    }
                    Err(e) => Ok(McpToolResult {
                        content: vec![McpContent::text(e.to_string())],
                        is_error: Some(true),
                    }),
                }
            }

            "switch_project" => {
                let path = call
                    .arguments
                    .get("path")
                    .and_then(|v| v.as_str())
                    .ok_or("Missing path")?;
                let msg = self.switch_project(std::path::PathBuf::from(path))?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(msg)],
                    is_error: None,
                })
            }

            _ => Err(format!("Unknown tool: {}", call.name)),
        }
    }

    pub fn get_resource(&self, uri: &str) -> Result<String, String> {
        let api_state = self.state();
        match uri {
            "codecartographer://project-graph" => {
                let graph = api_state.rebuild_graph()?;
                Ok(serde_json::to_string(&graph).unwrap_or_default())
            }
            "codecartographer://module-index" => {
                let files = api_state
                    .mapped_files
                    .lock()
                    .map_err(|e| e.to_string())?;
                Ok(serde_json::to_string(&*files).unwrap_or_default())
            }
            _ => Err(format!("Unknown resource: {}", uri)),
        }
    }

    pub fn get_prompt(
        &self,
        name: &str,
        arguments: &HashMap<String, String>,
    ) -> Result<String, String> {
        let api_state = self.state();
        match name {
            "analyze_module" => {
                let module_id = arguments
                    .get("module_id")
                    .ok_or("Missing module_id argument")?;

                let request = ModuleContextRequest {
                    module_id: module_id.clone(),
                    depth: Some(1),
                    detail_level: Some("standard".to_string()),
                    include: None,
                    format: None,
                };

                let context = api_state.get_module_context(&request)?;

                Ok(format!(
                    "Analyze the module at {}:\n\n\
                    Path: {}\n\n\
                    Imports:\n{}\n\n\
                    Signatures:\n{}\n\n\
                    Provide a summary of the module's public API and its dependencies.",
                    module_id,
                    context.path,
                    context.imports.join("\n"),
                    context
                        .signatures
                        .iter()
                        .map(|s| s.raw.clone())
                        .collect::<Vec<_>>()
                        .join("\n")
                ))
            }

            "plan_refactoring" => {
                let module_id = arguments
                    .get("module_id")
                    .ok_or("Missing module_id argument")?;
                let goal = arguments.get("goal").ok_or("Missing goal argument")?;

                let request = ModuleContextRequest {
                    module_id: module_id.clone(),
                    depth: Some(2),
                    detail_level: Some("extended".to_string()),
                    include: None,
                    format: None,
                };

                let context = api_state.get_module_context(&request)?;

                Ok(format!(
                    "Plan a refactoring of {} to achieve: {}\n\n\
                    Current module path: {}\n\n\
                    Dependencies (depth 2):\n{}\n\n\
                    Public API:\n{}\n\n\
                    Consider:\n\
                    1. How the refactoring affects each dependency\n\
                    2. Potential breaking changes\n\
                    3. Migration strategy",
                    module_id,
                    goal,
                    context.path,
                    context
                        .dependencies
                        .as_ref()
                        .map(|d| d
                            .iter()
                            .map(|d| d.module_id.clone())
                            .collect::<Vec<_>>()
                            .join(", "))
                        .unwrap_or_default(),
                    context
                        .signatures
                        .iter()
                        .map(|s| s.raw.clone())
                        .collect::<Vec<_>>()
                        .join("\n")
                ))
            }

            _ => Err(format!("Unknown prompt: {}", name)),
        }
    }

    /// Run the MCP server on stdio using JSON-RPC 2.0.
    pub fn serve(&self) {
        use std::io::{BufRead, Write};
        use std::sync::atomic::Ordering;
        let stdin = std::io::stdin();
        let stdout = std::io::stdout();
        let mut roots_requested = false;
        for line in stdin.lock().lines() {
            let line = match line {
                Ok(l) => l,
                Err(_) => break,
            };
            if line.trim().is_empty() {
                continue;
            }

            // A client->server *response* (reply to a request we sent) carries an
            // id but no method. Route it to the roots auto-follow path; never reply.
            if let Ok(v) = serde_json::from_str::<serde_json::Value>(&line) {
                if v.get("method").is_none()
                    && (v.get("result").is_some() || v.get("error").is_some())
                {
                    self.handle_client_response(&v);
                    continue;
                }
            }

            let method: String = serde_json::from_str::<serde_json::Value>(&line)
                .ok()
                .and_then(|v| v.get("method").and_then(|m| m.as_str()).map(String::from))
                .unwrap_or_default();

            let response = self.handle_jsonrpc(&line);
            if !response.is_empty() {
                let mut out = stdout.lock();
                let _ = writeln!(out, "{}", response);
                let _ = out.flush();
            }

            // Once the client has finished initializing, ask it for its workspace
            // roots (if it supports them) so we can auto-follow the session's repo
            // instead of staying pinned to our launch directory.
            if method == "notifications/initialized"
                && !roots_requested
                && self.client_supports_roots.load(Ordering::Relaxed)
            {
                roots_requested = true;
                let mut out = stdout.lock();
                let req = serde_json::json!({
                    "jsonrpc": "2.0",
                    "id": ROOTS_REQUEST_ID,
                    "method": "roots/list"
                });
                let _ = writeln!(out, "{}", req);
                let _ = out.flush();
            }
        }
    }

    fn handle_jsonrpc(&self, line: &str) -> String {
        let msg: serde_json::Value = match serde_json::from_str(line) {
            Ok(v) => v,
            Err(e) => {
                return jsonrpc_error(None, -32700, &format!("Parse error: {}", e));
            }
        };

        let id = msg.get("id").cloned();
        let method = msg
            .get("method")
            .and_then(|m| m.as_str())
            .unwrap_or("");
        let params = msg
            .get("params")
            .cloned()
            .unwrap_or_else(|| serde_json::Value::Object(Default::default()));

        // Notifications have no id — do not send a response
        if id.is_none() {
            return String::new();
        }

        match method {
            "initialize" => {
                // If the client advertises the `roots` capability, we'll ask it
                // for the workspace root after `initialized` and auto-follow it.
                if params.pointer("/capabilities/roots").is_some() {
                    self.client_supports_roots
                        .store(true, std::sync::atomic::Ordering::Relaxed);
                }
                let result = serde_json::json!({
                    "protocolVersion": "2024-11-05",
                    "capabilities": {
                        "tools": {},
                        "resources": {},
                        "prompts": {}
                    },
                    "serverInfo": self.get_server_info()
                });
                jsonrpc_ok(&id, result)
            }

            "tools/list" => {
                let result = serde_json::json!({ "tools": self.list_tools() });
                jsonrpc_ok(&id, result)
            }

            "tools/call" => {
                let name = params
                    .get("name")
                    .and_then(|v| v.as_str())
                    .unwrap_or("")
                    .to_string();
                let arguments = params
                    .get("arguments")
                    .cloned()
                    .unwrap_or_else(|| serde_json::Value::Object(Default::default()));
                let call = McpToolCall { name, arguments };
                match self.call_tool(call) {
                    Ok(result) => jsonrpc_ok(
                        &id,
                        serde_json::to_value(result).unwrap_or_default(),
                    ),
                    Err(e) => jsonrpc_error(id.as_ref(), -32603, &e),
                }
            }

            "resources/list" => {
                let result = serde_json::json!({ "resources": self.list_resources() });
                jsonrpc_ok(&id, result)
            }

            "resources/read" => {
                let uri = params
                    .get("uri")
                    .and_then(|v| v.as_str())
                    .unwrap_or("");
                match self.get_resource(uri) {
                    Ok(content) => jsonrpc_ok(
                        &id,
                        serde_json::json!({
                            "contents": [{
                                "uri": uri,
                                "mimeType": "application/json",
                                "text": content
                            }]
                        }),
                    ),
                    Err(e) => jsonrpc_error(id.as_ref(), -32603, &e),
                }
            }

            "prompts/list" => {
                let result = serde_json::json!({ "prompts": self.list_prompts() });
                jsonrpc_ok(&id, result)
            }

            "prompts/get" => {
                let name = params
                    .get("name")
                    .and_then(|v| v.as_str())
                    .unwrap_or("");
                let arguments: HashMap<String, String> = params
                    .get("arguments")
                    .and_then(|v| serde_json::from_value(v.clone()).ok())
                    .unwrap_or_default();
                match self.get_prompt(name, &arguments) {
                    Ok(content) => jsonrpc_ok(
                        &id,
                        serde_json::json!({
                            "description": name,
                            "messages": [{
                                "role": "user",
                                "content": { "type": "text", "text": content }
                            }]
                        }),
                    ),
                    Err(e) => jsonrpc_error(id.as_ref(), -32603, &e),
                }
            }

            _ => jsonrpc_error(
                id.as_ref(),
                -32601,
                &format!("Method not found: {}", method),
            ),
        }
    }
}

// ---------------------------------------------------------------------------
// Helpers shared by focused_skeleton and diff_skeleton
// ---------------------------------------------------------------------------

/// BFS over graph edges; returns every reachable module within `depth` hops
/// with a role tag: "focus" (seed), "dependency" (seed imports it), or
/// "importer" (it imports the seed).
fn bfs_neighborhood(
    seed: &str,
    depth: usize,
    edges: &[crate::api::GraphEdge],
) -> Vec<(String, &'static str)> {
    use std::collections::{HashMap, VecDeque};

    let mut result: HashMap<String, &'static str> = HashMap::new();
    result.insert(seed.to_string(), "focus");

    if depth == 0 {
        return result.into_iter().collect();
    }

    let mut queue: VecDeque<(String, usize)> = VecDeque::new();
    queue.push_back((seed.to_string(), 0));

    while let Some((module_id, hops)) = queue.pop_front() {
        if hops >= depth {
            continue;
        }
        for edge in edges {
            if edge.source == module_id {
                if !result.contains_key(&edge.target) {
                    result.insert(edge.target.clone(), "dependency");
                    queue.push_back((edge.target.clone(), hops + 1));
                }
            }
            if edge.target == module_id {
                if !result.contains_key(&edge.source) {
                    result.insert(edge.source.clone(), "importer");
                    queue.push_back((edge.source.clone(), hops + 1));
                }
            }
        }
    }

    result.into_iter().collect()
}

/// Build a path → "hot"/"stable"/empty label map from raw churn counts.
fn compute_churn_labels<'a>(
    churn_map: &std::collections::HashMap<String, usize>,
    paths: impl Iterator<Item = &'a str>,
) -> std::collections::HashMap<String, &'static str> {
    let mut counts: Vec<usize> = paths.map(|p| *churn_map.get(p).unwrap_or(&0)).collect();
    if counts.is_empty() {
        return std::collections::HashMap::new();
    }
    counts.sort_unstable();
    let n = counts.len();
    let hot_t = counts[n * 3 / 4];
    let stable_t = counts[n / 4];
    let max_c = *counts.last().unwrap_or(&0);

    let mut labels = std::collections::HashMap::new();
    if max_c > 0 && hot_t != stable_t {
        for (path, &c) in churn_map {
            if c > hot_t {
                labels.insert(path.clone(), "hot");
            } else if stable_t > 0 && c < stable_t {
                labels.insert(path.clone(), "stable");
            }
        }
    }
    labels
}

/// Collect all function/method names that have `#[test]` coverage (same
/// stripping logic as `annotate_tested` in main.rs).
fn collect_tested_names<'a>(
    files: impl Iterator<Item = &'a crate::mapper::MappedFile>,
) -> std::collections::HashSet<String> {
    let mut names = std::collections::HashSet::new();
    let strip = |n: &str| -> String {
        let base = n.strip_prefix("test_").or_else(|| n.strip_prefix("tests_")).unwrap_or(n);
        base.trim_end_matches("_works")
            .trim_end_matches("_fails")
            .trim_end_matches("_success")
            .trim_end_matches("_error")
            .trim_end_matches("_ok")
            .trim_end_matches("_err")
            .trim_end_matches("_test")
            .to_string()
    };
    for file in files {
        for n in &file.inline_test_fns {
            let b = strip(n);
            if !b.is_empty() { names.insert(b); }
        }
        if crate::api::is_test_path(&file.path) {
            for sig in &file.signatures {
                if let Some(n) = &sig.symbol_name {
                    let b = strip(n);
                    if !b.is_empty() { names.insert(b); }
                }
            }
        }
    }
    names
}

/// Render a single `MappedFile` as a JSON value for the focused/diff skeleton
/// output, applying body, tested-marker, and churn-label enrichments.
fn render_mf(
    mf: &crate::mapper::MappedFile,
    role: &str,
    max_sigs: usize,
    churn_labels: &std::collections::HashMap<String, &'static str>,
    tested_names: &std::collections::HashSet<String>,
    root: &std::path::Path,
) -> serde_json::Value {
    let rel_path = std::path::Path::new(&mf.path)
        .strip_prefix(root)
        .map(|r| r.to_string_lossy().replace('\\', "/"))
        .unwrap_or_else(|_| mf.path.clone());
    let is_test = crate::api::is_test_path(&mf.path);
    let sigs: Vec<String> = mf.signatures.iter().take(max_sigs).map(|s| {
        if let Some(body) = &s.body {
            let decl = s.raw.trim_end_matches('{').trim_end();
            format!("{} {{ {} }}", decl, body)
        } else if !is_test && s.symbol_name.as_deref().map(|n| tested_names.contains(n)).unwrap_or(false) {
            format!("{} // tested", s.raw)
        } else {
            format!("{} // ...", s.raw)
        }
    }).collect();
    serde_json::json!({
        "path":       rel_path,
        "role":       role,
        "heat":       churn_labels.get(&mf.path).copied().unwrap_or(""),
        "imports":    mf.imports,
        "signatures": sigs,
    })
}

/// Extract the first double-quoted token from a line of code.
/// e.g. `case "ctrl+c":` → Some("ctrl+c"), `key == "up"` → Some("up").
/// Returns None if no quoted token ≤ 30 chars is found.
fn extract_quoted_key(line: &str) -> Option<String> {
    let start = line.find('"')? + 1;
    let rest = &line[start..];
    let end = rest.find('"')?;
    let key = &rest[..end];
    if !key.is_empty() && key.len() <= 30 {
        Some(key.to_string())
    } else {
        None
    }
}

/// Convert a `file://` URI to a local path, decoding `%XX` escapes. Returns
/// None for non-file URIs. Handles both `file:///abs/path` and `file://host/abs`.
fn file_uri_to_path(uri: &str) -> Option<std::path::PathBuf> {
    let rest = uri.strip_prefix("file://")?;
    // Drop an optional authority (host) component: everything before the first
    // '/' that starts the absolute path.
    let path_part = match rest.find('/') {
        Some(idx) => &rest[idx..],
        None => rest,
    };
    let bytes = path_part.as_bytes();
    let hex = |c: u8| -> Option<u8> {
        match c {
            b'0'..=b'9' => Some(c - b'0'),
            b'a'..=b'f' => Some(c - b'a' + 10),
            b'A'..=b'F' => Some(c - b'A' + 10),
            _ => None,
        }
    };
    let mut out = Vec::with_capacity(bytes.len());
    let mut i = 0;
    while i < bytes.len() {
        if bytes[i] == b'%' && i + 2 < bytes.len() {
            if let (Some(h), Some(l)) = (hex(bytes[i + 1]), hex(bytes[i + 2])) {
                out.push(h * 16 + l);
                i += 3;
                continue;
            }
        }
        out.push(bytes[i]);
        i += 1;
    }
    let s = String::from_utf8(out).ok()?;
    Some(std::path::PathBuf::from(s))
}

fn jsonrpc_ok(id: &Option<serde_json::Value>, result: serde_json::Value) -> String {
    serde_json::to_string(&serde_json::json!({
        "jsonrpc": "2.0",
        "id": id,
        "result": result
    }))
    .unwrap_or_default()
}

fn jsonrpc_error(id: Option<&serde_json::Value>, code: i32, message: &str) -> String {
    serde_json::to_string(&serde_json::json!({
        "jsonrpc": "2.0",
        "id": id,
        "error": { "code": code, "message": message }
    }))
    .unwrap_or_default()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_mcp_server_info() {
        let info = McpServerInfo::default();
        assert_eq!(info.name, "Project CodeCartographer MCP Server");
        assert_eq!(info.version, "1.0.0");
    }

    #[test]
    fn test_tools_created() {
        let api_state = std::sync::Arc::new(ApiState::new(std::path::PathBuf::from("/test")));
        let server = McpServer::new(api_state);
        assert!(!server.list_tools().is_empty());
    }

    #[test]
    fn search_skeleton_tool_is_registered() {
        let api_state = std::sync::Arc::new(ApiState::new(std::path::PathBuf::from("/test")));
        let server = McpServer::new(api_state);
        assert!(server.list_tools().iter().any(|t| t.name == "search_skeleton"));
    }

    #[test]
    fn search_skeleton_requires_pattern() {
        let api_state = std::sync::Arc::new(ApiState::new(std::path::PathBuf::from("/test")));
        let server = McpServer::new(api_state);
        let call = McpToolCall {
            name: "search_skeleton".to_string(),
            arguments: serde_json::json!({}),
        };
        assert!(server.call_tool(call).is_err());
    }

    #[test]
    fn search_skeleton_empty_project_returns_zero_matches() {
        let api_state = std::sync::Arc::new(ApiState::new(std::path::PathBuf::from("/test")));
        let server = McpServer::new(api_state);
        let call = McpToolCall {
            name: "search_skeleton".to_string(),
            arguments: serde_json::json!({ "pattern": "anything" }),
        };
        let result = server.call_tool(call).unwrap();
        let text = match &result.content[0] { McpContent::Text { text } => text.clone(), _ => String::new() };
        let v: serde_json::Value = serde_json::from_str(&text).unwrap();
        assert_eq!(v["matched"], 0);
        assert_eq!(v["returned"], 0);
    }
}
