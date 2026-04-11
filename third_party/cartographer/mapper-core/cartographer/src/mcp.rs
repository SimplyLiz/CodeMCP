// MCP Server - Exposes Project Cartographer via Model Context Protocol
// This allows AI tools and agents to interact with Cartographer using MCP

use crate::api::{ApiState, ModuleContextRequest};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

macro_rules! mcprop {
    ($type:literal, $desc:literal) => {
        McpProperty {
            type_: $type.to_string(),
            description: $desc.to_string(),
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

/// MCP Tool definition
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct McpTool {
    pub name: String,
    pub description: String,
    pub input_schema: McpInputSchema,
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
    pub version: String,
    pub capabilities: McpCapabilities,
}

impl Default for McpServerInfo {
    fn default() -> Self {
        Self {
            name: "Project Cartographer MCP Server".to_string(),
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

/// MCP Tool Call response
#[derive(Debug, Serialize)]
pub struct McpToolResult {
    pub content: Vec<McpContent>,
    pub is_error: Option<bool>,
}

#[derive(Debug, Serialize)]
#[serde(untagged)]
pub enum McpContent {
    Text { text: String },
    Image { data: String, mime_type: String },
    Resource { resource: McpResource },
}

impl McpContent {
    pub fn text(content: String) -> Self {
        McpContent::Text { text: content }
    }
}

/// MCP Server implementation
pub struct McpServer {
    api_state: std::sync::Arc<ApiState>,
    tools: Vec<McpTool>,
    resources: Vec<McpResource>,
    prompts: Vec<McpPrompt>,
}

impl McpServer {
    pub fn new(api_state: std::sync::Arc<ApiState>) -> Self {
        let tools = Self::create_tools();
        let resources = Self::create_resources();
        let prompts = Self::create_prompts();

        Self {
            api_state,
            tools,
            resources,
            prompts,
        }
    }

    fn create_tools() -> Vec<McpTool> {
        vec![
            McpTool {
                name: "get_module_context".to_string(),
                description:
                    "Get the public API surface of a specific module with optional dependencies"
                        .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert(
                            "module_id".to_string(),
                            McpProperty {
                                type_: "string".to_string(),
                                description:
                                    "Unique identifier for the module (file path or module name)"
                                        .to_string(),
                            },
                        );
                        props.insert(
                            "depth".to_string(),
                            McpProperty {
                                type_: "number".to_string(),
                                description: "Depth of transitive dependencies (0 = module only)"
                                    .to_string(),
                            },
                        );
                        props.insert(
                            "detail_level".to_string(),
                            mcprop!("string", "Level of detail: minimal, standard, extended"),
                        );
                        props
                    },
                    required: vec!["module_id".to_string()],
                },
            },
            McpTool {
                name: "get_symbol_context".to_string(),
                description: "Get context for a specific symbol within a module".to_string(),
                input_schema: mcinput!(
                    "module_id" => "string" => "Module containing the symbol",
                    "symbol_name" => "string" => "Name of the symbol to retrieve",
                    "detail_level" => "string" => "Level of detail: minimal, standard, extended"
                ),
            },
            McpTool {
                name: "get_project_graph".to_string(),
                description: "Get the full project dependency graph".to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
            },
            McpTool {
                name: "get_dependencies".to_string(),
                description: "Get direct/transitive dependencies of a module".to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert(
                            "module_id".to_string(),
                            mcprop!("string", "Module to get dependencies for"),
                        );
                        props.insert(
                            "depth".to_string(),
                            mcprop!("number", "Dependency depth (default 1)"),
                        );
                        props
                    },
                    required: vec!["module_id".to_string()],
                },
            },
            McpTool {
                name: "get_dependents".to_string(),
                description: "Get modules that depend on a given module".to_string(),
                input_schema: mcinput!(
                    "module_id" => "string" => "Module to get dependents for"
                ),
            },
            McpTool {
                name: "search_project".to_string(),
                description: "Search for modules matching a pattern".to_string(),
                input_schema: mcinput!(
                    "query" => "string" => "Search pattern",
                    "query_type" => "string" => "Type: node or edge"
                ),
            },
            McpTool {
                name: "get_blast_radius".to_string(),
                description: "Get files and symbols affected by changing a target module. \
                              Each related entry includes lip_uris — the LIP symbol URIs \
                              (lip://local/<path>#<symbol>) of public symbols in that file — \
                              so CKB can drill into any affected symbol without a second lookup."
                    .to_string(),
                input_schema: mcinput!(
                    "target" => "string" => "File path or symbol name",
                    "max_related" => "number" => "Maximum related items (default 10)"
                ),
            },
            McpTool {
                name: "get_evolution".to_string(),
                description: "Get architectural health trend, debt indicators, and recommendations. \
                              Useful for understanding how code quality is trending."
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
            },
            McpTool {
                name: "watch_status".to_string(),
                description: "Check whether files changed since the last `cartographer watch` \
                              cycle. Returns { lastChangedMs, changedFiles } or \
                              { watching: false } if watch is not running."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
            },
            McpTool {
                name: "set_compression_level".to_string(),
                description: "Configure compression level for responses".to_string(),
                input_schema: mcinput!(
                    "level" => "string" => "Compression level: minimal, standard, aggressive"
                ),
            },
            McpTool {
                name: "find_files".to_string(),
                description: "Find files matching a glob pattern (like find). Returns path, \
                              language, and size. Use instead of find/ls tool calls."
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
            },
            McpTool {
                name: "search_content".to_string(),
                description: "Search for text or regex patterns across project files (like grep). \
                              Returns matching lines with optional context. Use this instead of \
                              grep/find tool calls."
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
            },

            // -----------------------------------------------------------------
            // Architectural analysis
            // -----------------------------------------------------------------
            McpTool {
                name: "get_health".to_string(),
                description: "Return the architectural health score and summary counts (cycles, \
                              bridges, god modules, layer violations)."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
            },
            McpTool {
                name: "get_cycles".to_string(),
                description: "Return all circular dependency cycles with severity and a suggested \
                              pivot node to break each cycle."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
            },
            McpTool {
                name: "check_layers".to_string(),
                description: "Check the project against its layers.toml architectural layer \
                              config. Returns violations with source/target layer and severity."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
            },
            McpTool {
                name: "unreferenced_symbols".to_string(),
                description: "Return public symbols that appear unreferenced across the project \
                              (dead-code candidates). Heuristic — does not account for dynamic \
                              dispatch or external consumers."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: HashMap::new(),
                    required: vec![],
                },
            },
            McpTool {
                name: "simulate_change".to_string(),
                description: "Predict the architectural impact of changing a module: affected \
                              modules, cycle risk, layer violations, and health delta."
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
            },

            // -----------------------------------------------------------------
            // Context / skeleton
            // -----------------------------------------------------------------
            McpTool {
                name: "skeleton_map".to_string(),
                description: "Return a compressed skeleton of every project file: imports and \
                              public signatures only. Ideal for giving a model a full structural \
                              overview within a token budget."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("detail".to_string(), mcprop!("string", "Detail level: minimal, standard, or extended (default standard)"));
                        props
                    },
                    required: vec![],
                },
            },
            McpTool {
                name: "ranked_skeleton".to_string(),
                description: "Return a token-budget-aware skeleton ranked by PageRank. Optionally \
                              personalise to a set of focus files so the most relevant modules \
                              surface first."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("focus".to_string(), mcprop!("string", "JSON array of focus file paths for personalization, e.g. [\"src/api.rs\"]"));
                        props.insert("budget".to_string(), mcprop!("number", "Max tokens to include (0 = unlimited)"));
                        props
                    },
                    required: vec![],
                },
            },

            // -----------------------------------------------------------------
            // Git intelligence
            // -----------------------------------------------------------------
            McpTool {
                name: "git_churn".to_string(),
                description: "Return per-file commit counts over recent git history. High-churn \
                              files are hotspot candidates."
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
            },
            McpTool {
                name: "git_cochange".to_string(),
                description: "Return file pairs that frequently change together (temporal \
                              coupling). High coupling score = files that almost always change \
                              in the same commit."
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
            },
            McpTool {
                name: "hidden_coupling".to_string(),
                description: "Return file pairs that co-change frequently but have NO import \
                              edge — implicit coupling invisible in the static graph."
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
            },
            McpTool {
                name: "semidiff".to_string(),
                description: "Return a function-level semantic diff between two commits: which \
                              public signatures were added, removed, or changed."
                    .to_string(),
                input_schema: mcinput!(
                    "commit1" => "string" => "Base commit SHA or ref (e.g. HEAD~1)",
                    "commit2" => "string" => "Target commit SHA or ref (default HEAD)"
                ),
            },
            McpTool {
                name: "poll_changes".to_string(),
                description: "Return project files modified since a given epoch-millisecond \
                              timestamp. Use 0 to get files changed in the last 60 seconds."
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
            },

            // -----------------------------------------------------------------
            // Surgical editing
            // -----------------------------------------------------------------
            McpTool {
                name: "replace_content".to_string(),
                description: "Find-and-replace across project files (sed-like). Supports regex \
                              with $1/$2 capture group references. Use dry_run=true to preview \
                              changes as a diff before writing."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("pattern".to_string(), mcprop!("string", "Regex pattern to search for"));
                        props.insert("replacement".to_string(), mcprop!("string", "Replacement string; supports $0 (whole match) and $1/$2 (capture groups)"));
                        props.insert("dryRun".to_string(), mcprop!("boolean", "Preview changes without writing to disk (default false)"));
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
            },
            McpTool {
                name: "extract_content".to_string(),
                description: "Extract capture-group values from regex matches across project \
                              files (awk-like). Use count=true for frequency tables, \
                              groups=[1,2] for specific capture groups."
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
            },
            // PKG retrieval — full query → rank → score pipeline
            // -----------------------------------------------------------------
            McpTool {
                name: "query_context".to_string(),
                description: "Full retrieval pipeline for code-question context injection. \
                              Given a natural-language query or symbol name: (1) searches \
                              the codebase for matching files, (2) uses PageRank personalised \
                              to those files to build a token-budget-aware skeleton, \
                              (3) scores the bundle with context_health. Returns the ready-to-inject \
                              context string plus health metadata. Use this instead of calling \
                              search_content + ranked_skeleton + context_health separately."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("query".to_string(), mcprop!("string", "Natural language question or symbol/pattern to search for"));
                        props.insert("budget".to_string(), mcprop!("number", "Max tokens for the skeleton portion (default: 8000)"));
                        props.insert("model".to_string(), mcprop!("string", "Target model family for health scoring: claude (default), gpt4, llama, gpt35"));
                        props.insert("maxSearchResults".to_string(), mcprop!("number", "Max search hits used as focus seeds (default: 20)"));
                        props
                    },
                    required: vec!["query".to_string()],
                },
            },
            // Shotgun surgery / co-change dispersion
            // -----------------------------------------------------------------
            McpTool {
                name: "shotgun_surgery".to_string(),
                description: "Detect shotgun surgery candidates — files whose changes scatter \
                              across many unrelated modules. Computes co-change dispersion \
                              (arXiv:2504.18511): partner count and Shannon entropy over the \
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
            },
            // Context quality
            // -----------------------------------------------------------------
            McpTool {
                name: "context_health".to_string(),
                description: "Analyse the quality of an LLM context bundle. Returns a \
                              composite health score (0–100, graded A–F) plus per-metric \
                              breakdown: signal density, compression density, position health, \
                              entity density, utilisation headroom, and dedup ratio. Warnings \
                              and recommendations are included when thresholds are breached. \
                              Pair with ranked_skeleton to produce high-scoring context bundles."
                    .to_string(),
                input_schema: McpInputSchema {
                    type_: "object".to_string(),
                    properties: {
                        let mut props = HashMap::new();
                        props.insert("content".to_string(), mcprop!("string", "The context text to score (e.g. a ranked_skeleton output)"));
                        props.insert("model".to_string(), mcprop!("string", "Target model family: claude (default, 200K), gpt4 (128K), llama (128K), gpt35 (16K)"));
                        props.insert("windowSize".to_string(), mcprop!("number", "Override context window size in tokens (0 = use model default)"));
                        props.insert("signatureCount".to_string(), mcprop!("number", "Number of symbol signatures in the content (improves entity density scoring)"));
                        props.insert("signatureTokens".to_string(), mcprop!("number", "Tokens occupied by signature text (improves signal density scoring)"));
                        props.insert("keyPositions".to_string(), mcprop!("string", "JSON array of 0.0–1.0 relative positions of key modules in the output"));
                        props
                    },
                    required: vec!["content".to_string()],
                },
            },
        ]
    }

    fn create_resources() -> Vec<McpResource> {
        vec![
            McpResource {
                uri: "cartographer://project-graph".to_string(),
                name: "project_graph".to_string(),
                description: "Full project dependency graph in JSON format".to_string(),
                mime_type: Some("application/json".to_string()),
            },
            McpResource {
                uri: "cartographer://module-index".to_string(),
                name: "module_index".to_string(),
                description: "Index of all mapped modules with their signatures".to_string(),
                mime_type: Some("application/json".to_string()),
            },
        ]
    }

    fn create_prompts() -> Vec<McpPrompt> {
        vec![
            McpPrompt {
                name: "analyze_module".to_string(),
                description: "Generate a prompt for analyzing a specific module".to_string(),
                arguments: vec![McpArgument {
                    name: "module_id".to_string(),
                    description: "Module to analyze".to_string(),
                    required: true,
                }],
            },
            McpPrompt {
                name: "plan_refactoring".to_string(),
                description: "Generate a prompt for planning refactoring of a module".to_string(),
                arguments: vec![
                    McpArgument {
                        name: "module_id".to_string(),
                        description: "Module to refactor".to_string(),
                        required: true,
                    },
                    McpArgument {
                        name: "goal".to_string(),
                        description: "Refactoring goal".to_string(),
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

    pub fn call_tool(&self, call: McpToolCall) -> Result<McpToolResult, String> {
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

                let response = self.api_state.get_module_context(&request)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string_pretty(&response).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "get_project_graph" => {
                let graph = self.api_state.rebuild_graph()?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string_pretty(&graph).unwrap_or_default(),
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

                let deps = self
                    .api_state
                    .get_dependencies_internal(module_id, depth)?
                    .ok_or("No dependencies found")?;

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string_pretty(&deps).unwrap_or_default(),
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

                let dependents = self.api_state.get_dependents(module_id)?;

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string_pretty(&dependents).unwrap_or_default(),
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

                let results = self.api_state.search_graph(query, query_type)?;

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string_pretty(&results).unwrap_or_default(),
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

                self.api_state.set_compression_level(level);

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

                let graph = self.api_state.rebuild_graph()?;

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

                let graph = self.api_state.rebuild_graph()?;

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

                let graph = self.api_state.rebuild_graph()?;

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

                let mut response = self.api_state.get_module_context(&request)?;
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
                        serde_json::to_string_pretty(&response).unwrap_or_default(),
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
                let graph = self.api_state.rebuild_graph()?;

                let node = graph
                    .nodes
                    .iter()
                    .find(|n| n.module_id == target || n.path.contains(target))
                    .ok_or_else(|| format!("Target not found: {}", target))?;
                let module_id = node.module_id.clone();

                let deps = self
                    .api_state
                    .get_dependencies_internal(&module_id, 1)?
                    .unwrap_or_default();

                let dependents = self.api_state.get_dependents(&module_id)?;

                // Pre-fetch mapped_files once for LIP URI extraction.
                let files_snapshot = self.api_state.mapped_files.lock()
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
                        serde_json::to_string_pretty(&result).unwrap_or_default(),
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
                    crate::search::find_files(&self.api_state.root_path, &pattern, limit, &crate::search::FindOptions::default())
                        .map_err(|e| e)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string_pretty(&result).unwrap_or_default(),
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

                let result = self.api_state.search_content(&pattern, &opts)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string_pretty(&result).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "get_evolution" => {
                let days = call.arguments
                    .get("days")
                    .and_then(|v| v.as_u64())
                    .map(|d| d as u32);
                let result = self.api_state.get_evolution(days)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string_pretty(&result).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            "watch_status" => {
                let state_path = self.api_state.root_path.join(".cartographer_watch_state.json");
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
                let graph = self.api_state.rebuild_graph()?;
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
                    content: vec![McpContent::text(serde_json::to_string_pretty(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            "get_cycles" => {
                let graph = self.api_state.rebuild_graph()?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string_pretty(&graph.cycles).unwrap_or_default())],
                    is_error: None,
                })
            }

            "check_layers" => {
                let graph = self.api_state.rebuild_graph()?;
                let result = serde_json::json!({
                    "violations":     graph.layer_violations,
                    "violationCount": graph.layer_violations.len(),
                });
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string_pretty(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            "unreferenced_symbols" => {
                let graph = self.api_state.rebuild_graph()?;
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
                    content: vec![McpContent::text(serde_json::to_string_pretty(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            "simulate_change" => {
                let args = &call.arguments;
                let module_id = args.get("module_id").and_then(|v| v.as_str()).ok_or("Missing module_id")?.to_string();
                let new_sig = args.get("new_signature").and_then(|v| v.as_str()).map(str::to_string);
                let rem_sig = args.get("remove_signature").and_then(|v| v.as_str()).map(str::to_string);
                // Ensure graph is built before simulate_change
                let _ = self.api_state.rebuild_graph()?;
                let result = self.api_state.simulate_change(&module_id, new_sig.as_deref(), rem_sig.as_deref())?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string_pretty(&result).unwrap_or_default())],
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
                let _ = self.api_state.rebuild_graph()?;
                let files = self.api_state.mapped_files.lock().map_err(|e| e.to_string())?;
                let max_sigs = match detail {
                    "minimal"  => 5usize,
                    "extended" => usize::MAX,
                    _          => 20,
                };
                let skeleton: Vec<serde_json::Value> = files.values().map(|mf| {
                    let sigs: Vec<&str> = mf.signatures.iter()
                        .take(max_sigs)
                        .map(|s| s.raw.as_str())
                        .collect();
                    serde_json::json!({
                        "path":       mf.path,
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
                    content: vec![McpContent::text(serde_json::to_string_pretty(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            "ranked_skeleton" => {
                let args = &call.arguments;
                let focus_str = args.get("focus").and_then(|v| v.as_str()).unwrap_or("[]");
                let focus: Vec<String> = serde_json::from_str(focus_str).unwrap_or_default();
                let budget = args.get("budget").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
                // Ensure graph is built
                let _ = self.api_state.rebuild_graph()?;
                let result = self.api_state.ranked_skeleton(&focus, budget)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string_pretty(&result).unwrap_or_default())],
                    is_error: None,
                })
            }

            // -----------------------------------------------------------------
            // Git intelligence tools
            // -----------------------------------------------------------------

            "git_churn" => {
                let limit = call.arguments.get("limit").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
                let churn = crate::git_analysis::git_churn(&self.api_state.root_path, limit);
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string_pretty(&churn).unwrap_or_default())],
                    is_error: None,
                })
            }

            "git_cochange" => {
                let args = &call.arguments;
                let limit     = args.get("limit").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
                let min_count = args.get("min_count").and_then(|v| v.as_u64()).unwrap_or(2) as usize;
                let pairs: Vec<serde_json::Value> = crate::git_analysis::git_cochange(&self.api_state.root_path, limit)
                    .into_iter()
                    .filter(|p| p.count >= min_count)
                    .map(|p| serde_json::json!({
                        "fileA": p.file_a, "fileB": p.file_b,
                        "count": p.count,  "couplingScore": p.coupling_score,
                    }))
                    .collect();
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string_pretty(&pairs).unwrap_or_default())],
                    is_error: None,
                })
            }

            "hidden_coupling" => {
                let args = &call.arguments;
                let limit     = args.get("limit").and_then(|v| v.as_u64()).unwrap_or(0) as usize;
                let min_count = args.get("min_count").and_then(|v| v.as_u64()).unwrap_or(2) as usize;
                let graph = self.api_state.rebuild_graph()?;
                // Build set of existing import edges for fast lookup
                let edge_set: std::collections::HashSet<(String, String)> = graph.edges.iter()
                    .flat_map(|e| [
                        (e.source.clone(), e.target.clone()),
                        (e.target.clone(), e.source.clone()),
                    ])
                    .collect();
                let pairs: Vec<serde_json::Value> = crate::git_analysis::git_cochange(&self.api_state.root_path, limit)
                    .into_iter()
                    .filter(|p| p.count >= min_count)
                    .filter(|p| !edge_set.contains(&(p.file_a.clone(), p.file_b.clone())))
                    .map(|p| serde_json::json!({
                        "fileA": p.file_a, "fileB": p.file_b,
                        "count": p.count,  "couplingScore": p.coupling_score,
                    }))
                    .collect();
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string_pretty(&pairs).unwrap_or_default())],
                    is_error: None,
                })
            }

            "semidiff" => {
                let args = &call.arguments;
                let commit1 = args.get("commit1").and_then(|v| v.as_str()).ok_or("Missing commit1")?.to_string();
                let commit2 = args.get("commit2").and_then(|v| v.as_str()).unwrap_or("HEAD").to_string();
                let root = &self.api_state.root_path;
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
                    content: vec![McpContent::text(serde_json::to_string_pretty(&result).unwrap_or_default())],
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
                let scan = crate::scanner::scan_files_with_noise_tracking(&self.api_state.root_path)
                    .map_err(|e| e.to_string())?;
                let changed: Vec<String> = scan.files.iter()
                    .filter(|p| !crate::scanner::is_ignored_path(p))
                    .filter_map(|p| {
                        let mtime = std::fs::metadata(p).ok()?.modified().ok()?;
                        if mtime > threshold {
                            let rel = p.strip_prefix(&self.api_state.root_path).unwrap_or(p)
                                .to_string_lossy().replace('\\', "/");
                            Some(rel)
                        } else { None }
                    })
                    .collect();
                let result = serde_json::json!({ "changedFiles": changed, "checkedAtMs": now_ms });
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string_pretty(&result).unwrap_or_default())],
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
                let result = crate::search::replace_content(&self.api_state.root_path, &pattern, &replacement, &opts)
                    .map_err(|e| e)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string_pretty(&result).unwrap_or_default())],
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
                let result = crate::search::extract_content(&self.api_state.root_path, &pattern, &opts)
                    .map_err(|e| e)?;
                Ok(McpToolResult {
                    content: vec![McpContent::text(serde_json::to_string_pretty(&result).unwrap_or_default())],
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

                // Step 1: search for files matching the query
                let search_opts = crate::search::SearchOptions {
                    case_sensitive: false,
                    max_results: max_search,
                    ..Default::default()
                };
                let focus_files: Vec<String> = match crate::search::search_content(
                    &self.api_state.root_path,
                    &query,
                    &search_opts,
                ) {
                    Ok(sr) => {
                        let mut seen = std::collections::HashSet::new();
                        sr.matches.into_iter()
                            .filter_map(|m| if seen.insert(m.path.clone()) { Some(m.path) } else { None })
                            .collect()
                    }
                    Err(_) => vec![],
                };

                // Step 2: ranked skeleton personalised to those files
                let ranked = self.api_state.ranked_skeleton(&focus_files, budget)
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
                        serde_json::to_string_pretty(&result).unwrap_or_default(),
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
                    &self.api_state.root_path,
                    commits,
                );
                entries.retain(|e| e.partner_count >= min_partners);
                entries.truncate(max_results);

                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string_pretty(&entries).unwrap_or_default(),
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

                let report = crate::token_metrics::analyze(&content, &opts);
                Ok(McpToolResult {
                    content: vec![McpContent::text(
                        serde_json::to_string_pretty(&report).unwrap_or_default(),
                    )],
                    is_error: None,
                })
            }

            _ => Err(format!("Unknown tool: {}", call.name)),
        }
    }

    pub fn get_resource(&self, uri: &str) -> Result<String, String> {
        match uri {
            "cartographer://project-graph" => {
                let graph = self.api_state.rebuild_graph()?;
                Ok(serde_json::to_string_pretty(&graph).unwrap_or_default())
            }
            "cartographer://module-index" => {
                let files = self
                    .api_state
                    .mapped_files
                    .lock()
                    .map_err(|e| e.to_string())?;
                Ok(serde_json::to_string_pretty(&*files).unwrap_or_default())
            }
            _ => Err(format!("Unknown resource: {}", uri)),
        }
    }

    pub fn get_prompt(
        &self,
        name: &str,
        arguments: &HashMap<String, String>,
    ) -> Result<String, String> {
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

                let context = self.api_state.get_module_context(&request)?;

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

                let context = self.api_state.get_module_context(&request)?;

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
        let stdin = std::io::stdin();
        let stdout = std::io::stdout();
        for line in stdin.lock().lines() {
            let line = match line {
                Ok(l) => l,
                Err(_) => break,
            };
            if line.trim().is_empty() {
                continue;
            }
            let response = self.handle_jsonrpc(&line);
            if response.is_empty() {
                continue; // notifications — no response
            }
            let mut out = stdout.lock();
            let _ = writeln!(out, "{}", response);
            let _ = out.flush();
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
        assert_eq!(info.name, "Project Cartographer MCP Server");
        assert_eq!(info.version, "1.0.0");
    }

    #[test]
    fn test_tools_created() {
        let api_state = std::sync::Arc::new(ApiState::new(std::path::PathBuf::from("/test")));
        let server = McpServer::new(api_state);
        assert!(!server.list_tools().is_empty());
    }
}
