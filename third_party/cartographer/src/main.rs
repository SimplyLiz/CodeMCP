mod answer;
mod api;
mod call_graph;
mod reach;
mod class_graph;
mod cross_call;
mod diagram;
mod diagram_export;
mod doctor;
mod extractor;
mod formatter;
mod html_export;
mod token_metrics;
mod git_analysis;
mod global_config;
mod layers;
mod mapper;
mod mcp;
mod memory;
mod scanner;
mod search;
mod sync;
mod webhooks;

use anyhow::{Context, Result};
use arboard::Clipboard;
use clap::{Parser, Subcommand, ValueEnum};
use formatter::{estimate_tokens, format_token_count, get_formatter, OutputTarget};
use mapper::{extract_skeleton, MappedFile};
use memory::Memory;
use notify_debouncer_mini::{new_debouncer, notify::RecursiveMode, DebouncedEventKind};
use rayon::prelude::*;
use scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking, IgnoredFile};
use std::collections::{HashMap, HashSet};
use std::fs::{self, File};
use std::io::{self, BufWriter, Write};
use std::path::{Path, PathBuf};
use std::sync::mpsc::channel;
use std::time::Duration;
use sync::SyncService;

const WATCH_DEBOUNCE_MS: u64 = 500;

#[derive(Parser)]
#[command(name = "codecartographer")]
#[command(about = "Memory Unit - Deterministic codebase mapper for AI context injection")]
#[command(version)]
struct Cli {
    #[command(subcommand)]
    command: Option<Commands>,

    /// Target folder to scan (defaults to current directory)
    #[arg(value_name = "PATH")]
    path: Option<PathBuf>,

    #[arg(short, long)]
    target: Option<Target>,

    #[arg(short, long)]
    copy: bool,

    #[arg(long = "ignore", value_name = "FILE")]
    ignore_files: Vec<String>,
}

#[derive(Clone, ValueEnum, Default)]
enum Target {
    Raw,
    #[default]
    Claude,
    Cursor,
}

impl From<Target> for OutputTarget {
    fn from(t: Target) -> Self {
        match t {
            Target::Raw => OutputTarget::Raw,
            Target::Claude => OutputTarget::Claude,
            Target::Cursor => OutputTarget::Cursor,
        }
    }
}

#[derive(Subcommand)]
enum Commands {
    /// Mode A: Skeleton map (imports + signatures only)
    Map {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
    },
    /// Mode B: Full source code (saves to disk)
    Source {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
    },
    /// Live watcher - keeps skeleton map updated, NO full source to disk
    Watch {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
    },
    /// Copy full source to clipboard (ephemeral - no disk write)
    Copy {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
    },
    /// Incremental sync
    Sync {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
    },
    /// Initialize CodeCartographer in the current project
    Init {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
    },
    /// Initialize CodeCartographer with CKB integration
    InitCkb {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        #[arg(long, value_name = "CKB_URL")]
        ckb_url: Option<String>,
        #[arg(long, value_name = "WEBHOOK_URL")]
        webhook_url: Option<String>,
    },
    /// Health check - shows architectural health score
    Health {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Compare current health against a git ref (e.g. main, HEAD~1)
        #[arg(long, value_name = "REF")]
        compare: Option<String>,
        /// Roll the graph up to directory granularity at the given path depth
        /// (e.g. 2 folds `a/b/c/f.rs` into `a/b`). Bridges/cycles/god modules then
        /// describe subsystems, not files — the comprehensible view on huge trees.
        #[arg(long, value_name = "DEPTH")]
        rollup: Option<usize>,
        /// Emit machine-readable JSON
        #[arg(long)]
        json: bool,
    },
    /// Simulate how a change will impact the architecture
    Simulate {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Target module (required unless --staged or --diff is given)
        #[arg(long, value_name = "MODULE")]
        module: Option<String>,
        #[arg(long, value_name = "SIGNATURE")]
        new_signature: Option<String>,
        #[arg(long, value_name = "REMOVE")]
        remove_signature: Option<String>,
        /// Analyse all staged changes (git diff --cached)
        #[arg(long)]
        staged: bool,
        /// Analyse changes relative to a git ref (e.g. main, HEAD~1)
        #[arg(long, value_name = "REF")]
        diff: Option<String>,
        /// Exit with status 1 if any simulated change would create a cycle
        #[arg(long)]
        fail_on_cycle: bool,
        /// Emit machine-readable JSON
        #[arg(long)]
        json: bool,
    },
    /// Show architecture evolution over time
    Evolution {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        #[arg(long, value_name = "DAYS")]
        days: Option<u32>,
    },
    /// Show dependencies of a target module as JSON
    Deps {
        #[arg(value_name = "TARGET")]
        target: String,
        #[arg(long, default_value = "json")]
        format: String,
    },
    /// Start MCP server (stdio JSON-RPC transport)
    Serve {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Expose only a preset toolset ("core" = the ~12 highest-value discovery tools).
        /// Defaults to all tools. Falls back to the CARTOGRAPHER_PRESET env var.
        #[arg(long, value_name = "PRESET")]
        preset: Option<String>,
    },
    /// Show project status
    Status {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
    },
    /// Manage global codecartographer configuration
    Config {
        /// Set the default output target globally (claude, cursor, raw)
        #[arg(long, value_name = "TARGET")]
        default_target: Option<String>,
        /// Print current global configuration
        #[arg(long)]
        show: bool,
    },
    /// Show temporal coupling pairs from git history
    Cochange {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Number of commits to analyse
        #[arg(long, default_value = "500")]
        commits: usize,
        /// Minimum co-change count to display
        #[arg(long, default_value = "5")]
        min_count: usize,
        /// Cluster files into implicit modules via co-change community detection
        #[arg(long)]
        cluster: bool,
        /// Coupling-score threshold for community edges (0.0–1.0; default 0.5)
        #[arg(long, default_value = "0.5")]
        threshold: f64,
        /// Emit machine-readable JSON
        #[arg(long)]
        json: bool,
    },
    /// Show TODO/FIXME/HACK density across source files
    Todo {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Number of results to show
        #[arg(long, default_value = "20")]
        top: usize,
        /// Emit machine-readable JSON
        #[arg(long)]
        json: bool,
    },
    /// Show hotspot files (high churn × high complexity)
    Hotspots {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Number of commits to analyse
        #[arg(long, default_value = "500")]
        commits: usize,
        /// Number of results to show
        #[arg(long, default_value = "15")]
        top: usize,
        /// Only show hotspots that have no sibling test file
        #[arg(long)]
        untested: bool,
        /// Show dominant owner column (who changed each file most)
        #[arg(long)]
        by_author: bool,
        /// Show bus-factor column (unique author count — lower = higher risk)
        #[arg(long)]
        bus_factor: bool,
        /// Emit machine-readable JSON
        #[arg(long)]
        json: bool,
    },
    /// Show files with high co-change dispersion (shotgun surgery candidates)
    Shotgun {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Number of commits to analyse
        #[arg(long, default_value = "500")]
        commits: usize,
        /// Number of results to show
        #[arg(long, default_value = "20")]
        top: usize,
        /// Minimum distinct co-change partners to include
        #[arg(long, default_value = "3")]
        min_partners: usize,
    },
    /// Find dead code candidates (unreachable in dependency graph)
    Dead {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Emit machine-readable JSON
        #[arg(long)]
        json: bool,
    },
    /// Export dependency graph as a diagram
    Diagram {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Output format: mermaid, dot, ascii, sequence, class, quadrant, er
        #[arg(long, default_value = "mermaid")]
        format: String,
        /// Write output to file instead of stdout
        #[arg(short = 'o', long, value_name = "FILE")]
        output: Option<PathBuf>,
        /// Maximum nodes to include (trims least-connected)
        #[arg(long, default_value = "60")]
        max_nodes: usize,
        /// Anchor the diagram on a module (module_id or path suffix).
        /// Shows the neighborhood via undirected BFS to `--depth`.
        #[arg(long, value_name = "MODULE")]
        focus: Option<String>,
        /// BFS depth when `--focus` is set
        #[arg(long, default_value = "2")]
        depth: usize,
        /// Render the blast radius of a module: epicenter + direct deps +
        /// direct dependents. Overrides `--focus` when both are set.
        #[arg(long, value_name = "MODULE")]
        blast_radius: Option<String>,
        /// Overlay co-change edges (dotted purple) for pairs with
        /// `coupling_score >= THRESHOLD`. Omit to disable.
        #[arg(long, value_name = "THRESHOLD")]
        cochange_threshold: Option<f64>,
        /// Render the documentation subgraph: all doc files
        /// (Markdown/YAML/TOML/JSON) plus the code they reference. Yields a
        /// doc-map view distinct from the import-graph top-N default.
        #[arg(long)]
        docs_only: bool,
        /// Collapse the graph to folder granularity at the given path depth.
        /// `1` groups by top-level folder (src/, tests/, …); `2` groups by
        /// second-level folder (src/api/, src/db/, …). Drops self-loops from
        /// intra-folder edges and aggregates inter-folder edges.
        #[arg(long, value_name = "DEPTH")]
        group_by_folder: Option<usize>,
        /// Color nodes by dominant git author (replaces role-based colors).
        /// Nodes without an `owner` field stay white. Requires git history.
        #[arg(long)]
        color_by_owner: bool,
        /// Render the function-level call graph for a single source file
        /// (Rust or Python). Nodes are functions/methods; edges are
        /// caller→callee relations for calls we can resolve to a function
        /// defined in the same file. External / stdlib calls are dropped.
        #[arg(long, value_name = "FILE")]
        call_graph: Option<PathBuf>,
        /// Cross-file sequence trace entry point: FILE::FUNCTION
        /// (e.g. `src/main.rs::diagram_mode`). Requires `--format sequence`.
        /// Spike feature — resolution is heuristic; edges marked `(~)` are
        /// lower-confidence matches.
        #[arg(long, value_name = "FILE::FUNCTION")]
        entry: Option<String>,
    },
    /// Generate llms.txt index for this project
    Llmstxt {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Write to file instead of stdout
        #[arg(short = 'o', long, value_name = "FILE")]
        output: Option<PathBuf>,
    },
    /// Generate CLAUDE.md architecture guide
    Claudemd {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Write to file instead of stdout
        #[arg(short = 'o', long, value_name = "FILE")]
        output: Option<PathBuf>,
    },
    /// Show semantic diff (function-level) between two commits
    Semidiff {
        #[arg(value_name = "COMMIT1")]
        commit1: String,
        #[arg(value_name = "COMMIT2", default_value = "HEAD")]
        commit2: String,
    },
    /// List languages detected in the project and their file counts
    Languages {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Emit machine-readable JSON
        #[arg(long)]
        json: bool,
    },
    /// CI gate: exit non-zero if cycles or layer violations are found
    Check {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
    },
    /// Check the environment for the external tools diagrams and git-analysis need
    /// (git, mmdc, dot) and report anything missing with install hints.
    Doctor {
        /// Exit non-zero if a recommended tool (git) is missing — for CI.
        #[arg(long)]
        strict: bool,
    },
    /// Manage architectural layer definitions (layers.toml)
    Layers {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        #[command(subcommand)]
        command: LayerCommands,
    },
    /// Show the shortest import path between two source files
    Path {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Starting file (repo-relative path or module id)
        #[arg(long, value_name = "FROM")]
        from: String,
        /// Destination file (repo-relative path or module id)
        #[arg(long, value_name = "TO")]
        to: String,
        /// Emit machine-readable JSON
        #[arg(long)]
        json: bool,
    },
    /// Ranked skeleton context pruned to a token budget (personalized PageRank)
    Context {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Focus files for personalization (repeatable)
        #[arg(long = "focus", value_name = "FILE")]
        focus: Vec<String>,
        /// Maximum tokens to include (0 = unlimited)
        #[arg(long, default_value = "8000")]
        budget: usize,
        /// Also search for this pattern and bundle results into the context output
        #[arg(long, value_name = "PATTERN")]
        query: Option<String>,
        /// Task description: boosts files whose signatures overlap this text (TF-IDF re-ranking)
        #[arg(long, value_name = "DESCRIPTION")]
        for_task: Option<String>,
    },
    /// Show symbol-level analysis (unreferenced public exports)
    Symbols {
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
        /// Show only unreferenced public exports
        #[arg(long)]
        unreferenced: bool,
    },
    /// Search for text or regex pattern across project files (grep-like)
    Search {
        /// Pattern to search for (regex by default)
        #[arg(value_name = "PATTERN")]
        pattern: String,
        /// Additional patterns OR'd with the primary (repeatable, like grep -e)
        #[arg(short = 'e', long = "regexp", value_name = "PATTERN")]
        extra_patterns: Vec<String>,
        /// Treat pattern as a literal string (no regex metacharacters)
        #[arg(long)]
        literal: bool,
        /// Case-insensitive matching
        #[arg(short = 'i', long)]
        ignore_case: bool,
        /// Invert match — show lines that do NOT match
        #[arg(short = 'v', long)]
        invert_match: bool,
        /// Whole-word matching (wraps pattern in \b…\b)
        #[arg(short = 'w', long)]
        word_regexp: bool,
        /// Print only the matched portion of each line
        #[arg(short = 'o', long)]
        only_matching: bool,
        /// Print only file names that have matches
        #[arg(short = 'l', long)]
        files_with_matches: bool,
        /// Print only file names that have NO matches
        #[arg(long)]
        files_without_match: bool,
        /// Print match count per file
        #[arg(short = 'c', long)]
        count: bool,
        /// Lines of context after each match
        #[arg(short = 'A', long, value_name = "N", default_value = "0")]
        after_context: usize,
        /// Lines of context before each match
        #[arg(short = 'B', long, value_name = "N", default_value = "0")]
        before_context: usize,
        /// Lines of context before and after (sets both -A and -B)
        #[arg(short = 'C', long, value_name = "N", default_value = "0")]
        context: usize,
        /// Include only files matching this glob (e.g. "*.rs")
        #[arg(long, value_name = "GLOB")]
        glob: Option<String>,
        /// Exclude files matching this glob
        #[arg(long, value_name = "GLOB")]
        exclude: Option<String>,
        /// Restrict search to this repo-relative subdirectory
        #[arg(long, value_name = "SUBDIR")]
        path: Option<String>,
        /// Maximum results (0 = unlimited)
        #[arg(long, default_value = "100")]
        limit: usize,
        /// Include vendor/generated/noise files (bypass ignore filter)
        #[arg(long)]
        no_ignore: bool,
    },
    /// Find files by name or path glob (e.g. "*.rs" or "src/**/*.go")
    Find {
        /// Glob pattern
        #[arg(value_name = "PATTERN")]
        pattern: String,
        /// Files modified within this duration (e.g. "24h", "7d", "30m")
        #[arg(long, value_name = "DURATION")]
        modified_since: Option<String>,
        /// Files newer than this file's modification time
        #[arg(long, value_name = "FILE")]
        newer: Option<String>,
        /// Minimum file size in bytes
        #[arg(long, value_name = "BYTES")]
        min_size: Option<u64>,
        /// Maximum file size in bytes
        #[arg(long, value_name = "BYTES")]
        max_size: Option<u64>,
        /// Maximum directory depth (0 = root files only)
        #[arg(long, value_name = "N")]
        max_depth: Option<usize>,
        /// Maximum files to return (0 = unlimited)
        #[arg(long, default_value = "50")]
        limit: usize,
        /// Include vendor/generated/noise files (bypass ignore filter)
        #[arg(long)]
        no_ignore: bool,
    },
    /// Find-and-replace across project files (sed-like)
    Replace {
        /// Regex pattern to search for
        #[arg(value_name = "PATTERN")]
        pattern: String,
        /// Replacement string; supports $0 (whole match) and $1/$2 (capture groups)
        #[arg(value_name = "REPLACEMENT")]
        replacement: String,
        /// Treat pattern as a literal string (no regex metacharacters)
        #[arg(long)]
        literal: bool,
        /// Case-insensitive matching
        #[arg(short = 'i', long)]
        ignore_case: bool,
        /// Whole-word matching (wraps pattern in \b…\b)
        #[arg(short = 'w', long)]
        word_regexp: bool,
        /// Show what would change without writing to disk
        #[arg(long)]
        dry_run: bool,
        /// Write a .bak backup before modifying each file
        #[arg(long)]
        backup: bool,
        /// Context lines in diff output
        #[arg(short = 'C', long, value_name = "N", default_value = "3")]
        context: usize,
        /// Restrict to files matching this glob (e.g. "*.rs")
        #[arg(long, value_name = "GLOB")]
        glob: Option<String>,
        /// Exclude files matching this glob
        #[arg(long, value_name = "GLOB")]
        exclude: Option<String>,
        /// Restrict to this repo-relative subdirectory
        #[arg(long, value_name = "SUBDIR")]
        path: Option<String>,
        /// Max replacements per file (0 = unlimited)
        #[arg(long, value_name = "N", default_value = "0")]
        max_per_file: usize,
        /// Include vendor/generated/noise files (bypass ignore filter)
        #[arg(long)]
        no_ignore: bool,
    },
    /// Extract capture-group values from regex matches (awk-like)
    Extract {
        /// Regex pattern (use groups like `(foo)` to capture substrings)
        #[arg(value_name = "PATTERN")]
        pattern: String,
        /// Capture group index to extract (repeatable; 0 = whole match)
        #[arg(long = "group", short = 'g', value_name = "N")]
        groups: Vec<usize>,
        /// Separator between groups when multiple are selected
        #[arg(long, value_name = "SEP", default_value = "\t")]
        sep: String,
        /// Output format: text, json, csv, or tsv
        #[arg(long, value_name = "FMT", default_value = "text")]
        format: String,
        /// Aggregate: count occurrences per unique value
        #[arg(long)]
        count: bool,
        /// Deduplicate extracted values
        #[arg(long)]
        dedup: bool,
        /// Sort output (ascending; with --count sorts by frequency descending)
        #[arg(long)]
        sort: bool,
        /// Case-insensitive matching
        #[arg(short = 'i', long)]
        ignore_case: bool,
        /// Restrict to files matching this glob
        #[arg(long, value_name = "GLOB")]
        glob: Option<String>,
        /// Exclude files matching this glob
        #[arg(long, value_name = "GLOB")]
        exclude: Option<String>,
        /// Restrict to this repo-relative subdirectory
        #[arg(long, value_name = "SUBDIR")]
        path: Option<String>,
        /// Max total results (0 = unlimited)
        #[arg(long, default_value = "1000")]
        limit: usize,
        /// Include vendor/generated/noise files (bypass ignore filter)
        #[arg(long)]
        no_ignore: bool,
    },
    /// Query-driven context retrieval: search → PageRank → context_health in one step
    Query {
        /// Natural language question or symbol/pattern to search for
        #[arg(value_name = "QUERY")]
        query: String,
        /// Token budget for the skeleton (default: 8000)
        #[arg(long, default_value = "8000")]
        budget: usize,
        /// Target model family: claude, gpt4, llama, gpt35 (default: claude)
        #[arg(long, default_value = "claude")]
        model: String,
        /// Output format: text (default) or json
        #[arg(long, default_value = "text")]
        format: String,
        /// Max search hits used as PageRank focus seeds (default: 20)
        #[arg(long, default_value = "20")]
        max_seeds: usize,
    },
    /// Score the quality of an LLM context bundle (signal density, entropy, position health)
    ContextHealth {
        /// Read context from this file (default: stdin)
        #[arg(value_name = "FILE")]
        file: Option<PathBuf>,
        /// Target model family for window size: claude, gpt4, llama, gpt35 (default: claude)
        #[arg(long, default_value = "claude")]
        model: String,
        /// Override context window size in tokens (0 = use model default)
        #[arg(long, default_value = "0")]
        window: usize,
        /// Output format: text (default) or json
        #[arg(long, default_value = "text")]
        format: String,
        /// Exit 1 if token count exceeds this threshold (CI gate; 0 = disabled)
        #[arg(long, default_value = "0", value_name = "TOKENS")]
        fail_if_over: usize,
    },
    /// Save or compare architecture snapshots
    Snapshot {
        #[command(subcommand)]
        command: SnapshotCommands,
    },
    /// Question-driven evidence chain — assembles minimum context to answer a question [EXPERIMENTAL]
    Answer {
        /// Natural language question about the codebase
        #[arg(value_name = "QUESTION")]
        question: String,
        /// Maximum items in the evidence chain (default 6)
        #[arg(long, default_value = "6")]
        max_items: usize,
        /// Token budget (default 8000)
        #[arg(long, default_value = "8000")]
        budget: usize,
        /// Do not show the body of the top-scored item
        #[arg(long)]
        no_body: bool,
        /// Drill into evidence item N with reach, appended below the answer chain
        #[arg(long = "then", value_name = "N")]
        then_item: Option<usize>,
        /// Project path (defaults to current directory)
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
    },
    /// Semantic graph traversal — AI-optimized context tree from a symbol [EXPERIMENTAL]
    Reach {
        /// Symbol name(s) to start from. Provide two for intersection view.
        #[arg(value_name = "SYMBOL", num_args = 1..)]
        symbols: Vec<String>,
        /// Disambiguate to a specific file (single-symbol mode only)
        #[arg(long, value_name = "FILE")]
        file: Option<String>,
        /// Graph traversal depth (default 2)
        #[arg(long, default_value = "2")]
        depth: usize,
        /// Token budget — hard cap on output size (default 6000)
        #[arg(long, default_value = "6000")]
        budget: usize,
        /// Include test call sites (default: collapse and count them)
        #[arg(long)]
        include_tests: bool,
        /// Include the function body of the root symbol (up to 40 lines)
        #[arg(long)]
        show_body: bool,
        /// Output format: compact (default) or json
        #[arg(long, default_value = "compact")]
        format: String,
        /// Project path to scan (defaults to current directory)
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
    },
    /// Re-run the install script to upgrade to the latest build
    Update,
}

#[derive(Subcommand, Debug)]
enum SnapshotCommands {
    /// Save current architecture snapshot with a tag
    Save {
        /// Tag to identify this snapshot (e.g. "v1.0.0" or "before-refactor")
        #[arg(value_name = "TAG")]
        tag: String,
        /// Repository path (default: current directory)
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
    },
    /// Diff two saved snapshots
    Diff {
        /// First snapshot tag
        #[arg(value_name = "TAG1")]
        tag1: String,
        /// Second snapshot tag
        #[arg(value_name = "TAG2")]
        tag2: String,
        /// Emit machine-readable JSON
        #[arg(long)]
        json: bool,
    },
    /// List saved snapshots
    List {
        /// Repository path (default: current directory)
        #[arg(value_name = "PATH")]
        path: Option<PathBuf>,
    },
}


#[derive(Subcommand, Debug)]
enum LayerCommands {
    /// Auto-propose a layers.toml from the current import graph
    Init {
        /// Write the proposed file here instead of printing to stdout
        #[arg(short = 'o', long, value_name = "FILE")]
        output: Option<PathBuf>,
    },
    /// Validate source files against layers.toml and report violations
    Validate {
        /// Path to layers.toml (default: ./layers.toml or .codecartographer/layers.toml)
        #[arg(long, value_name = "FILE")]
        config: Option<PathBuf>,
        /// Emit machine-readable JSON
        #[arg(long)]
        json: bool,
    },
    /// Show the layer graph (collapsed, not file-level)
    Diagram {
        /// Path to layers.toml (default: auto-detected)
        #[arg(long, value_name = "FILE")]
        config: Option<PathBuf>,
        /// Output format: mermaid (default) or dot
        #[arg(long, default_value = "mermaid")]
        format: String,
    },
    /// Suggest improvements to an existing layers.toml
    Suggest {
        /// Path to layers.toml (default: auto-detected)
        #[arg(long, value_name = "FILE")]
        config: Option<PathBuf>,
    },
}

fn main() -> Result<()> {
    // Deeply nested ASTs (macro-generated C initializers, etc.) recurse the
    // tree-sitter walkers far enough to overflow a worker's default ~2 MB stack —
    // the Linux kernel aborts the whole process this way. Give rayon workers a
    // large stack. Must run before any par_iter; best-effort (no-op if the global
    // pool already exists).
    let _ = rayon::ThreadPoolBuilder::new()
        .stack_size(256 * 1024 * 1024)
        .build_global();
    let cli = Cli::parse();
    let cwd = std::env::current_dir().context("Failed to get current directory")?;
    // Resolve target: CLI flag > per-repo .codecartographer/config.toml > global config > claude
    let target = resolve_target(cli.target, &cwd);
    let ignore_set: HashSet<String> = cli.ignore_files.into_iter().collect();

    match cli.command {
        Some(Commands::Map { path }) => {
            let root = resolve_path(&cwd, path.or(cli.path.clone()))?;
            map_mode(&root, &cwd, target, cli.copy)
        }
        Some(Commands::Source { path }) => {
            let root = resolve_path(&cwd, path.or(cli.path.clone()))?;
            source_mode(&root, &cwd, target, cli.copy, &ignore_set)
        }
        Some(Commands::Watch { path }) => {
            let root = resolve_path(&cwd, path.or(cli.path.clone()))?;
            live_watch_mode(&root, &cwd, target)
        }
        Some(Commands::Copy { path }) => {
            let root = resolve_path(&cwd, path.or(cli.path.clone()))?;
            copy_mode(&root, target, &ignore_set)
        }
        Some(Commands::Sync { path }) => {
            let root = resolve_path(&cwd, path.or(cli.path.clone()))?;
            sync_mode(&root, &cwd, target, cli.copy)
        }
        Some(Commands::Init { path }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            init_local_mode(&root)
        }
        Some(Commands::InitCkb {
            path,
            ckb_url,
            webhook_url,
        }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            init_ckb_mode(&root, ckb_url.as_deref(), webhook_url.as_deref())
        }
        Some(Commands::Health { path, compare, rollup, json }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            health_mode(&root, compare.as_deref(), rollup, json)
        }
        Some(Commands::Simulate {
            path,
            module,
            new_signature,
            remove_signature,
            staged,
            diff,
            fail_on_cycle,
            json,
        }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            simulate_mode(
                &root,
                module.as_deref(),
                new_signature.as_deref(),
                remove_signature.as_deref(),
                staged,
                diff.as_deref(),
                fail_on_cycle,
                json,
            )
        }
        Some(Commands::Evolution { path, days }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            evolution_mode(&root, days)
        }
        Some(Commands::Deps { target, format }) => {
            let root = resolve_path(&cwd, cli.path)?;
            deps_mode(&root, &target, &format)
        }
        Some(Commands::Serve { path, preset }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            let preset = preset.or_else(|| std::env::var("CARTOGRAPHER_PRESET").ok());
            mcp_serve_mode(&root, preset.as_deref())
        }
        Some(Commands::Status { path }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            status_mode(&root)
        }
        Some(Commands::Config {
            default_target,
            show,
        }) => config_mode(default_target, show),
        Some(Commands::Cochange {
            path,
            commits,
            min_count,
            cluster,
            threshold,
            json,
        }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            cochange_mode(&root, commits, min_count, cluster, threshold, json)
        }
        Some(Commands::Todo { path, top, json }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            todo_mode(&root, top, json)
        }
        Some(Commands::Hotspots { path, commits, top, untested, by_author, bus_factor, json }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            hotspots_mode(&root, commits, top, untested, by_author, bus_factor, json)
        }
        Some(Commands::Shotgun { path, commits, top, min_partners }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            shotgun_mode(&root, commits, top, min_partners)
        }
        Some(Commands::Dead { path, json }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            dead_mode(&root, json)
        }
        Some(Commands::Diagram {
            path,
            format,
            output,
            max_nodes,
            focus,
            depth,
            blast_radius,
            cochange_threshold,
            docs_only,
            group_by_folder,
            color_by_owner,
            call_graph,
            entry,
        }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            diagram_mode(
                &root,
                &format,
                output.as_deref(),
                max_nodes,
                focus.as_deref(),
                depth,
                blast_radius.as_deref(),
                cochange_threshold,
                docs_only,
                group_by_folder,
                color_by_owner,
                call_graph.as_deref(),
                entry.as_deref(),
            )
        }
        Some(Commands::Llmstxt { path, output }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            llmstxt_mode(&root, output.as_deref())
        }
        Some(Commands::Claudemd { path, output }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            claudemd_mode(&root, output.as_deref())
        }
        Some(Commands::Semidiff { commit1, commit2 }) => {
            let root = resolve_path(&cwd, cli.path)?;
            semidiff_mode(&root, &commit1, &commit2)
        }
        Some(Commands::Languages { path, json }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            languages_mode(&root, json)
        }
        Some(Commands::Layers { path, command }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            layers_mode(&root, command)
        }
        Some(Commands::Check { path }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            check_mode(&root)
        }
        Some(Commands::Doctor { strict }) => {
            let report = doctor::run();
            if strict && report.missing_recommended > 0 {
                std::process::exit(1);
            }
            Ok(())
        }
        Some(Commands::Path { path, from, to, json }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            path_mode(&root, &from, &to, json)
        }
        Some(Commands::Context { path, focus, budget, query, for_task }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            context_mode(&root, &focus, budget, query.as_deref(), for_task.as_deref())
        }
        Some(Commands::Symbols { path, unreferenced }) => {
            let root = resolve_path(&cwd, path.or(cli.path))?;
            symbols_mode(&root, unreferenced)
        }
        Some(Commands::Search {
            pattern, extra_patterns, literal, ignore_case, invert_match,
            word_regexp, only_matching, files_with_matches, files_without_match,
            count, after_context, before_context, context, glob, exclude,
            path, limit, no_ignore,
        }) => {
            let root = resolve_path(&cwd, cli.path)?;
            search_mode(
                &root, &pattern, extra_patterns, literal, ignore_case,
                invert_match, word_regexp, only_matching, files_with_matches,
                files_without_match, count, after_context, before_context,
                context, glob.as_deref(), exclude.as_deref(),
                path.as_deref(), limit, no_ignore,
            )
        }
        Some(Commands::Find { pattern, modified_since, newer, min_size, max_size, max_depth, limit, no_ignore }) => {
            let root = resolve_path(&cwd, cli.path)?;
            find_mode(&root, &pattern, modified_since.as_deref(), newer.as_deref(), min_size, max_size, max_depth, limit, no_ignore)
        }
        Some(Commands::Replace {
            pattern, replacement, literal, ignore_case, word_regexp, dry_run,
            backup, context, glob, exclude, path, max_per_file, no_ignore,
        }) => {
            let root = resolve_path(&cwd, cli.path)?;
            replace_mode(
                &root, &pattern, &replacement, literal, ignore_case, word_regexp,
                dry_run, backup, context, glob.as_deref(), exclude.as_deref(),
                path.as_deref(), max_per_file, no_ignore,
            )
        }
        Some(Commands::Extract {
            pattern, groups, sep, format, count, dedup, sort, ignore_case,
            glob, exclude, path, limit, no_ignore,
        }) => {
            let root = resolve_path(&cwd, cli.path)?;
            extract_mode(
                &root, &pattern, &groups, &sep, &format, count, dedup, sort,
                ignore_case, glob.as_deref(), exclude.as_deref(),
                path.as_deref(), limit, no_ignore,
            )
        }
        Some(Commands::Query { query, budget, model, format, max_seeds }) => {
            let root = resolve_path(&cwd, cli.path)?;
            query_mode(&root, &query, budget, &model, &format, max_seeds)
        }
        Some(Commands::ContextHealth { file, model, window, format, fail_if_over }) => {
            context_health_mode(file.as_deref(), &model, window, &format, fail_if_over)
        }
        Some(Commands::Snapshot { command }) => {
            let cwd2 = cwd.clone();
            snapshot_mode(&cwd2, command)
        }
        Some(Commands::Answer { question, max_items, budget, no_body, then_item, path }) => {
            let root = resolve_path(&cwd, path)?;
            answer_mode(&root, &question, max_items, budget, !no_body, then_item)
        }
        Some(Commands::Reach { symbols, file, depth, budget, include_tests, show_body, format, path }) => {
            let root = resolve_path(&cwd, path)?;
            let sym_refs: Vec<&str> = symbols.iter().map(|s| s.as_str()).collect();
            reach_mode(&root, &sym_refs, file.as_deref(), depth, budget, include_tests, show_body, &format)
        }
        Some(Commands::Update) => {
            update_mode()
        }
        None => {
            let root = resolve_path(&cwd, cli.path)?;
            overview_mode(&root, &cwd, target, cli.copy, &ignore_set)
        }
    }
}

fn resolve_path(cwd: &Path, path: Option<PathBuf>) -> Result<PathBuf> {
    match path {
        Some(p) => {
            let resolved = if p.is_absolute() { p } else { cwd.join(&p) };
            if !resolved.exists() {
                anyhow::bail!("Path does not exist: {}", resolved.display());
            }
            if !resolved.is_dir() {
                anyhow::bail!("Path is not a directory: {}", resolved.display());
            }
            Ok(resolved)
        }
        None => Ok(cwd.to_path_buf()),
    }
}

// =============================================================================
// LIVE WATCH MODE - Lightweight skeleton map only, NO full source to disk
// =============================================================================

fn live_watch_mode(root: &Path, output_dir: &Path, target: OutputTarget) -> Result<()> {
    println!("LIVE WATCHER: Monitoring {}...", root.display());
    println!("============================================");
    println!("  Mode: Skeleton Map ONLY (lightweight)");
    println!("  Debounce: {}ms", WATCH_DEBOUNCE_MS);
    println!("  Full source: Use 'codecartographer copy' when needed");
    println!("============================================");
    println!("Press Ctrl+C to stop\n");

    // Cache: rel_path → (content_hash, MappedFile) for incremental re-extraction.
    let mut extract_cache: HashMap<String, (u64, MappedFile)> = HashMap::new();

    // Initial skeleton map generation
    let (mut mapped_files, ignored) = generate_skeleton_map_incremental(root, &mut extract_cache)?;
    annotate_tested(&mut mapped_files);
    annotate_churn(&mut mapped_files, root);
    let output = format_map_output(&mapped_files, target);
    let tokens = estimate_tokens(&output);

    // Write lightweight map file
    let formatter = get_formatter(target);
    let map_filename = format!("codecartographer_map.{}", formatter.extension());
    let map_path = output_dir.join(&map_filename);
    fs::write(&map_path, &output)?;

    print_codecartographer_report(mapped_files.len(), &ignored, tokens, &map_filename);
    println!("  Watching for changes...\n");

    // Setup file watcher with 500ms debounce
    let (tx, rx) = channel();
    let mut debouncer = new_debouncer(Duration::from_millis(WATCH_DEBOUNCE_MS), tx)?;
    debouncer.watcher().watch(root, RecursiveMode::Recursive)?;

    loop {
        match rx.recv() {
            Ok(Ok(events)) => {
                // Filter out irrelevant events (our own output, ignored paths)
                let relevant = events.iter().any(|e| {
                    e.kind == DebouncedEventKind::Any
                        && !e.path.ends_with(&map_filename)
                        && !e.path.ends_with(".codecartographer_memory.json")
                        && !e.path.ends_with("context.xml")
                        && !e.path.ends_with("context.md")
                        && !e.path.ends_with("context.json")
                        && !is_ignored_path(&e.path)
                });

                if relevant {
                    // Regenerate skeleton map (incremental — skips unchanged files)
                    match generate_skeleton_map_incremental(root, &mut extract_cache) {
                        Ok((mut files, _)) => {
                            annotate_tested(&mut files);
                            annotate_churn(&mut files, root);
                            let output = format_map_output(&files, target);
                            let tokens = estimate_tokens(&output);
                            if fs::write(&map_path, &output).is_ok() {
                                println!(
                                    "[{}] Map updated: {} files, {}",
                                    chrono_time(),
                                    files.len(),
                                    format_token_count(tokens)
                                );
                            }
                            // Write watch-state sentinel so MCP clients can poll for changes.
                            let now_ms = std::time::SystemTime::now()
                                .duration_since(std::time::UNIX_EPOCH)
                                .unwrap_or_default()
                                .as_millis() as u64;
                            let changed_paths: Vec<String> = events
                                .iter()
                                .filter(|e| e.kind == DebouncedEventKind::Any)
                                .filter_map(|e| {
                                    e.path.strip_prefix(root).ok()
                                        .map(|r| r.to_string_lossy().replace('\\', "/"))
                                })
                                .collect();
                            let sentinel = serde_json::json!({
                                "watching": true,
                                "lastChangedMs": now_ms,
                                "changedFiles": changed_paths,
                            });
                            let _ = fs::write(
                                root.join(".codecartographer_watch_state.json"),
                                serde_json::to_string_pretty(&sentinel).unwrap_or_default(),
                            );
                        }
                        Err(e) => eprintln!("Error updating map: {}", e),
                    }
                }
            }
            Ok(Err(e)) => eprintln!("Watch error: {:?}", e),
            Err(e) => {
                eprintln!("Channel error: {}", e);
                break;
            }
        }
    }
    Ok(())
}

// ---------------------------------------------------------------------------
// Feature 2: test-coverage markers
// ---------------------------------------------------------------------------

/// Marks each non-test-function signature as `tested = true` when a test function
/// whose name (after stripping a `test_` prefix) matches the symbol name exists.
///
/// Sources for test function names:
/// 1. Separate test files (path matches `is_test_path`).
/// 2. Inline `#[test]` functions detected during Rust extraction (sig.tested == true
///    on the test function itself, used here as a two-pass sentinel then cleared).
fn annotate_tested(files: &mut Vec<MappedFile>) {
    use std::collections::HashSet;

    // Collect candidate names from test files and inline #[test] functions.
    let mut tested_names: HashSet<String> = HashSet::new();

    let strip = |n: &str| -> String {
        let base = n
            .strip_prefix("test_")
            .or_else(|| n.strip_prefix("tests_"))
            .unwrap_or(n);
        base.trim_end_matches("_works")
            .trim_end_matches("_fails")
            .trim_end_matches("_success")
            .trim_end_matches("_error")
            .trim_end_matches("_ok")
            .trim_end_matches("_err")
            .trim_end_matches("_test")
            .to_string()
    };

    for file in files.iter() {
        // Inline #[test] / #[cfg(test)] names preserved through tree-sitter override.
        for name in &file.inline_test_fns {
            let base = strip(name);
            if !base.is_empty() {
                tested_names.insert(base);
            }
        }
        // Separate test-file signatures.
        if crate::api::is_test_path(&file.path) {
            for sig in &file.signatures {
                if let Some(name) = &sig.symbol_name {
                    let base = strip(name);
                    if !base.is_empty() {
                        tested_names.insert(base);
                    }
                }
            }
        }
    }

    // Mark matching non-test signatures.
    for file in files.iter_mut() {
        if crate::api::is_test_path(&file.path) {
            continue;
        }
        for sig in &mut file.signatures {
            sig.tested = sig
                .symbol_name
                .as_deref()
                .map(|n| tested_names.contains(n))
                .unwrap_or(false);
        }
    }
}

// ---------------------------------------------------------------------------
// Feature 3: churn annotations
// ---------------------------------------------------------------------------

/// Labels each MappedFile with "hot" (top-quartile commit count) or "stable"
/// (bottom-quartile) based on git churn over the last 300 commits.
fn annotate_churn(files: &mut Vec<MappedFile>, root: &Path) {
    let churn = crate::git_analysis::git_churn(root, 300);
    if churn.is_empty() {
        return;
    }

    let mut counts: Vec<usize> = files
        .iter()
        .map(|f| *churn.get(&f.path).unwrap_or(&0))
        .collect();
    counts.sort_unstable();

    let n = counts.len();
    let hot_threshold = counts[n * 3 / 4]; // 75th percentile
    let stable_threshold = counts[n / 4];   // 25th percentile
    let max_count = *counts.last().unwrap_or(&0);

    // Only label when there is real spread; flat distributions carry no signal.
    if max_count == 0 || hot_threshold == stable_threshold {
        return;
    }

    for file in files.iter_mut() {
        let c = *churn.get(&file.path).unwrap_or(&0);
        if c > hot_threshold {
            file.churn_label = Some("hot".to_string());
        } else if stable_threshold > 0 && c < stable_threshold {
            file.churn_label = Some("stable".to_string());
        }
    }
}

fn generate_skeleton_map(root: &Path) -> Result<(Vec<MappedFile>, Vec<IgnoredFile>)> {
    let scan_result = scan_files_with_noise_tracking(root)?;
    let mut mapped_files: Vec<MappedFile> = Vec::new();

    for path in &scan_result.files {
        if !is_source_file(path) { continue; }
        if let Some(content) = read_text_file(path) {
            let rel_path = path.strip_prefix(root).unwrap_or(path);
            let skeleton = extract_skeleton(rel_path, &content);
            if !skeleton.imports.is_empty() || !skeleton.signatures.is_empty() {
                mapped_files.push(skeleton);
            }
        }
    }

    Ok((mapped_files, scan_result.ignored_noise))
}

fn hash_content(s: &str) -> u64 {
    use std::hash::{Hash, Hasher};
    use std::collections::hash_map::DefaultHasher;
    let mut h = DefaultHasher::new();
    s.hash(&mut h);
    h.finish()
}

/// Like `generate_skeleton_map` but skips re-extraction for files whose content
/// hash hasn't changed since the last call. Used by watch mode.
fn generate_skeleton_map_incremental(
    root: &Path,
    cache: &mut HashMap<String, (u64, MappedFile)>,
) -> Result<(Vec<MappedFile>, Vec<IgnoredFile>)> {
    let scan_result = scan_files_with_noise_tracking(root)?;
    let mut mapped_files: Vec<MappedFile> = Vec::new();

    for path in &scan_result.files {
        if let Some(content) = read_text_file(path) {
            let rel_path = path.strip_prefix(root).unwrap_or(path);
            let rel_str = rel_path.to_string_lossy().to_string();
            let hash = hash_content(&content);

            let skeleton = if let Some((cached_hash, cached_file)) = cache.get(&rel_str) {
                if *cached_hash == hash {
                    cached_file.clone()
                } else {
                    let s = extract_skeleton(rel_path, &content);
                    cache.insert(rel_str, (hash, s.clone()));
                    s
                }
            } else {
                let s = extract_skeleton(rel_path, &content);
                cache.insert(rel_str, (hash, s.clone()));
                s
            };

            if !skeleton.imports.is_empty() || !skeleton.signatures.is_empty() {
                mapped_files.push(skeleton);
            }
        }
    }

    Ok((mapped_files, scan_result.ignored_noise))
}

fn chrono_time() -> String {
    use std::time::SystemTime;
    let now = SystemTime::now()
        .duration_since(SystemTime::UNIX_EPOCH)
        .unwrap_or_default();
    let secs = now.as_secs() % 86400;
    let hours = (secs / 3600) % 24;
    let mins = (secs % 3600) / 60;
    let secs = secs % 60;
    format!("{:02}:{:02}:{:02}", hours, mins, secs)
}

// =============================================================================
// COPY MODE - Ephemeral full source to clipboard (NO disk write)
// =============================================================================

fn copy_mode(root: &Path, target: OutputTarget, ignore_set: &HashSet<String>) -> Result<()> {
    println!("COPY MODE: Generating full source (ephemeral)...");

    let service = SyncService::new(root);
    let result = service.full_scan_with_noise()?;
    let mut memory = result.memory;
    let ignored = result.ignored_noise;

    // Apply user ignores
    if !ignore_set.is_empty() {
        memory.files.retain(|path, _| {
            let filename = path.rsplit('/').next().unwrap_or(path);
            !ignore_set.contains(filename) && !ignore_set.contains(path)
        });
    }

    // Generate output to memory only (NOT to disk)
    let formatter = get_formatter(target);
    let output = formatter.format(&memory);
    let tokens = estimate_tokens(&output);
    print_codecartographer_report(memory.files.len(), &ignored, tokens, "(clipboard)");

    println!(
        "  Tokens  : {}  (API input cost ~${:.2} if billed per token)",
        format_token_count(tokens),
        estimate_cost(tokens)
    );
    copy_to_clipboard(&output)?;
    Ok(())
}

// =============================================================================
// MAP MODE - One-shot skeleton map generation
// =============================================================================

fn overview_mode(
    root: &Path,
    cwd: &Path,
    target: OutputTarget,
    copy: bool,
    ignore_set: &HashSet<String>,
) -> Result<()> {
    let project_name = root.file_name().and_then(|n| n.to_str()).unwrap_or("project");

    // Map estimate: skeleton extraction
    let (mut mapped_files, _) = generate_skeleton_map(root)?;
    annotate_tested(&mut mapped_files);
    annotate_churn(&mut mapped_files, root);
    let map_output = format_map_output(&mapped_files, target);
    let map_tokens = estimate_tokens(&map_output);

    // Source estimate: full content (source files only, same as source_mode)
    let service = SyncService::new(root);
    let result = service.full_scan_with_noise()?;
    let mut memory = result.memory;
    if !ignore_set.is_empty() {
        memory.files.retain(|path, _| {
            let filename = path.rsplit('/').next().unwrap_or(path);
            !ignore_set.contains(filename) && !ignore_set.contains(path)
        });
    }
    let formatter = get_formatter(target);
    let source_tokens = estimate_tokens(&formatter.format(&memory));
    let file_count = memory.files.len();

    println!();
    println!("  Project : {}", project_name);
    println!("  Files   : {} source files", file_count);
    println!();
    println!("  map     {:<14} signatures & structure only   (recommended)", format_token_count(map_tokens));
    println!("  source  {:<14} full file content", format_token_count(source_tokens));
    println!("  diagram                render the dependency graph (Mermaid)");
    println!();
    print!("What would you like to do? [map/source/diagram/quit]: ");
    io::stdout().flush()?;

    let mut input = String::new();
    io::stdin().read_line(&mut input)?;
    println!();

    match input.trim() {
        "map"     => map_mode(root, cwd, target, copy),
        "source"  => source_mode(root, cwd, target, copy, ignore_set),
        "diagram" => {
            use crate::diagram;
            let graph = {
                use crate::scanner::is_source_file;
                let scan = scan_files_with_noise_tracking(root)?;
                let mapped: std::collections::HashMap<String, MappedFile> = scan.files.iter()
                    .filter(|p| is_source_file(p))
                    .filter_map(|p| {
                        let content = std::fs::read_to_string(p).ok()?;
                        let rel = p.strip_prefix(root).unwrap_or(p).to_string_lossy().replace('\\', "/");
                        Some((rel, extract_skeleton(p, &content)))
                    })
                    .collect();
                let state = crate::api::ApiState::new(root.to_path_buf());
                *state.mapped_files.lock().unwrap() = mapped;
                state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?
            };
            let opts = diagram::RenderOptions {
                format: diagram::DiagramFormat::Mermaid,
                focus: None, depth: 2, max_nodes: 60,
                show_cochange: None, blast_radius: None,
                docs_only: false, group_by_folder_depth: None,
                color_by_owner: false,
            };
            let rendered = diagram::render(&graph, &opts).map_err(|e| anyhow::anyhow!(e))?;
            if rendered.node_count == 0 {
                eprintln!("No nodes to diagram — not enough import relationships detected in this project.");
            } else {
                println!("{}", rendered.diagram);
            }
            Ok(())
        }
        "quit" | "q" | "" => Ok(()),
        other => {
            eprintln!("Unknown option: {}", other);
            Ok(())
        }
    }
}

fn map_mode(root: &Path, output_dir: &Path, target: OutputTarget, copy: bool) -> Result<()> {
    let project = root.file_name().and_then(|n| n.to_str()).unwrap_or("project");
    println!("  Scanning {}...\n", project);

    let (mut mapped_files, ignored) = generate_skeleton_map(root)?;
    annotate_tested(&mut mapped_files);
    annotate_churn(&mut mapped_files, root);
    let output = format_map_output(&mapped_files, target);
    let tokens = estimate_tokens(&output);

    let formatter = get_formatter(target);
    let filename = format!("codecartographer_map.{}", formatter.extension());
    fs::write(output_dir.join(&filename), &output)?;

    print_codecartographer_report(mapped_files.len(), &ignored, tokens, &filename);
    handle_token_budget_copy(&output, tokens, copy)?;
    Ok(())
}

// =============================================================================
// SOURCE MODE - Full source to disk (legacy behavior)
// =============================================================================

fn source_mode(
    root: &Path,
    output_dir: &Path,
    target: OutputTarget,
    copy: bool,
    ignore_set: &HashSet<String>,
) -> Result<()> {
    let project = root.file_name().and_then(|n| n.to_str()).unwrap_or("project");
    println!("  Scanning {}...\n", project);

    let service = SyncService::new(root);
    let result = service.full_scan_with_noise()?;
    let mut memory = result.memory;
    let ignored = result.ignored_noise;

    if !ignore_set.is_empty() {
        memory.files.retain(|path, _| {
            let filename = path.rsplit('/').next().unwrap_or(path);
            !ignore_set.contains(filename) && !ignore_set.contains(path)
        });
    }

    memory.save(output_dir)?;
    let output = write_output(output_dir, &memory, target)?;
    let tokens = estimate_tokens(&output);
    let formatter = get_formatter(target);
    let filename = format!("context.{}", formatter.extension());

    print_codecartographer_report(memory.files.len(), &ignored, tokens, &filename);
    handle_token_budget_copy(&output, tokens, copy)?;
    Ok(())
}

// =============================================================================
// SYNC MODE - Incremental update
// =============================================================================

fn sync_mode(root: &Path, output_dir: &Path, target: OutputTarget, copy: bool) -> Result<()> {
    let project = root.file_name().and_then(|n| n.to_str()).unwrap_or("project");
    println!("  Scanning {}...\n", project);

    let service = SyncService::new(root);
    let existing = Memory::load(output_dir).unwrap_or_default();
    let result = service.incremental_sync_with_noise(existing)?;
    let memory = result.memory;
    let ignored = result.ignored_noise;

    memory.save(output_dir)?;
    let output = write_output(output_dir, &memory, target)?;
    let tokens = estimate_tokens(&output);
    let formatter = get_formatter(target);
    let filename = format!("context.{}", formatter.extension());
    print_codecartographer_report(memory.files.len(), &ignored, tokens, &filename);
    handle_token_budget_copy(&output, tokens, copy)?;
    Ok(())
}

// =============================================================================
// Formatting helpers
// =============================================================================

fn format_map_output(files: &[MappedFile], target: OutputTarget) -> String {
    match target {
        OutputTarget::Claude => format_map_xml(files),
        OutputTarget::Cursor => format_map_markdown(files),
        OutputTarget::Raw => format_map_json(files),
    }
}

fn format_map_xml(files: &[MappedFile]) -> String {
    let mut out = String::from("<context type=\"skeleton_map\">\n<project_map>\n");
    for file in files {
        let heat_attr = match file.churn_label.as_deref() {
            Some(label) => format!(" heat=\"{}\"", label),
            None => String::new(),
        };
        out.push_str(&format!("<file path=\"{}\"{}>\n", escape_xml(&file.path), heat_attr));
        out.push_str(&escape_xml(&file.format()));
        out.push_str("</file>\n");
    }
    out.push_str("</project_map>\n</context>");
    out
}

fn format_map_markdown(files: &[MappedFile]) -> String {
    let mut out = String::from("# Project Skeleton Map\n\n");
    for file in files {
        let ext = file.path.rsplit('.').next().unwrap_or("txt");
        let heat = file.churn_label.as_deref().map(|l| format!("  <!-- {} -->", l)).unwrap_or_default();
        out.push_str(&format!(
            "## {}{}\n\n`{}\n{}\n`\n\n",
            file.path,
            heat,
            ext,
            file.format()
        ));
    }
    out
}

fn format_map_json(files: &[MappedFile]) -> String {
    let json_files: Vec<_> = files.iter().map(|f| serde_json::json!({"path": f.path, "imports": f.imports, "signatures": f.signatures})).collect();
    serde_json::to_string_pretty(&json_files).unwrap_or_default()
}

// =============================================================================
// CodeCartographer Report
// =============================================================================

fn print_codecartographer_report(included_count: usize, ignored: &[IgnoredFile], tokens: usize, filename: &str) {
    println!(
        "  {} files · {} · {}",
        included_count,
        format_token_count(tokens),
        filename
    );
    if !ignored.is_empty() {
        let noise_names: Vec<&str> = ignored
            .iter()
            .take(5)
            .map(|i| i.path.rsplit('/').next().unwrap_or(&i.path))
            .collect();
        let display = if ignored.len() > 5 {
            format!("{}, +{} more", noise_names.join(", "), ignored.len() - 5)
        } else {
            noise_names.join(", ")
        };
        let total_tokens: usize = ignored.iter().map(|i| i.estimated_tokens).sum();
        println!(
            "  Filtered: {} (saved {})",
            display,
            format_token_count(total_tokens)
        );
    }
    println!();
}

// =============================================================================
// Token Budget Check
// =============================================================================

fn handle_token_budget_copy(content: &str, _tokens: usize, auto_copy: bool) -> Result<()> {
    if auto_copy {
        copy_to_clipboard(content)?;
    } else {
        print!("  Copy to clipboard? [Y/n] ");
        io::stdout().flush()?;
        let mut input = String::new();
        io::stdin().read_line(&mut input)?;
        let input = input.trim();
        if input.is_empty() || input.eq_ignore_ascii_case("y") || input.eq_ignore_ascii_case("yes") {
            copy_to_clipboard(content)?;
        } else {
            println!("  Saved to disk.");
        }
    }
    Ok(())
}


fn estimate_cost(tokens: usize) -> f64 {
    (tokens as f64 / 1000.0) * 0.01
}

// =============================================================================
// Helper Functions
// =============================================================================

fn write_output(root: &Path, memory: &Memory, target: OutputTarget) -> Result<String> {
    let formatter = get_formatter(target);
    let output = formatter.format(memory);
    let filename = format!("context.{}", formatter.extension());
    let file = File::create(root.join(&filename))?;
    let mut writer = BufWriter::new(file);
    write!(writer, "{}", output)?;
    writer.flush()?;
    Ok(output)
}

fn copy_to_clipboard(content: &str) -> Result<()> {
    match Clipboard::new() {
        Ok(mut clipboard) => {
            clipboard
                .set_text(content.to_string())
                .context("Failed to copy to clipboard")?;
            println!("\nCopied to clipboard");
            Ok(())
        }
        Err(e) => {
            eprintln!("Clipboard unavailable: {}", e);
            Ok(())
        }
    }
}

fn read_text_file(path: &Path) -> Option<String> {
    let content = fs::read(path).ok()?;
    let check_len = content.len().min(8192);
    if content[..check_len].contains(&0) {
        return None;
    }
    String::from_utf8(content).ok()
}

fn escape_xml(s: &str) -> String {
    s.replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
}

// =============================================================================
// TARGET RESOLUTION
// =============================================================================

/// Per-repo config subset — only the [defaults] section we care about.
#[derive(serde::Deserialize, Default)]
struct RepoConfigFile {
    #[serde(default)]
    defaults: RepoDefaults,
}

#[derive(serde::Deserialize, Default)]
struct RepoDefaults {
    target: Option<String>,
}

/// Resolve output target: CLI flag > per-repo config > global config > claude.
fn resolve_target(cli_target: Option<Target>, cwd: &Path) -> OutputTarget {
    if let Some(t) = cli_target {
        return t.into();
    }
    // Per-repo config
    let repo_cfg_path = cwd.join(".codecartographer").join("config.toml");
    if let Ok(content) = fs::read_to_string(repo_cfg_path) {
        if let Ok(cfg) = toml::from_str::<RepoConfigFile>(&content) {
            if let Some(ref t) = cfg.defaults.target {
                if let Ok(ot) = t.parse::<OutputTarget>() {
                    return ot;
                }
            }
        }
    }
    // Global config
    let global = global_config::GlobalConfig::load();
    if let Some(ref t) = global.defaults.target {
        if let Ok(ot) = t.parse::<OutputTarget>() {
            return ot;
        }
    }
    OutputTarget::Claude
}


fn init_local_mode(root: &Path) -> Result<()> {
    let config_path = root.join(".codecartographer").join("config.toml");
    if config_path.exists() {
        println!("Config already exists at: {}", config_path.display());
        println!("Edit it directly to adjust defaults and layer config.");
        return Ok(());
    }
    let config_dir = config_path.parent().unwrap();
    fs::create_dir_all(config_dir)?;

    let project_name = root
        .file_name()
        .and_then(|n| n.to_str())
        .unwrap_or("my-project");

    let config_content = format!(
        r#"# CodeCartographer Configuration
version = "1.0.0"
project = "{}"

[defaults]
# Output target for this repo: claude, cursor, or raw
# Overrides global config; --target flag overrides this.
target = "claude"

[layers]
# Define your architectural layers here
# ui = ["components", "pages", "hooks"]
# services = ["api", "auth"]
# db = ["models", "repositories"]

[allowed_flows]
# Define allowed dependency flows
# ui -> services
# services -> db
"#,
        project_name
    );

    fs::write(&config_path, config_content)?;
    println!("Initialized .codecartographer/config.toml");
    println!("Project: {}", project_name);
    println!();
    println!("Next steps:");
    println!("  codecartographer source          — generate context");
    println!("  Edit {} to configure layers and defaults", config_path.display());
    Ok(())
}

fn status_mode(root: &Path) -> Result<()> {
    println!("CodeCartographer Status");
    println!("============================================");
    println!("Root: {}", root.display());
    println!();

    // Local memory
    let memory = Memory::load(root).unwrap_or_default();
    if memory.files.is_empty() {
        println!("Local memory: not initialized (run 'codecartographer source')");
    } else {
        println!("Tracked files:  {}", memory.files.len());
        println!("Memory version: {}", memory.version);
        if memory.last_sync > 0 {
            println!("Last scanned:   {}", format_timestamp(memory.last_sync));
        }
    }
    println!();

    // Global config
    let global = global_config::GlobalConfig::load();
    let target_status = global
        .defaults
        .target
        .as_deref()
        .unwrap_or("claude (default)");
    println!("Global target:   {}", target_status);

    // Per-repo config
    let repo_cfg = root.join(".codecartographer").join("config.toml");
    if repo_cfg.exists() {
        println!("Repo config:     {}", repo_cfg.display());
    } else {
        println!("Repo config:     not present (run 'codecartographer init')");
    }

    // .codecartographerignore
    let ignore_path = root.join(".codecartographerignore");
    if ignore_path.exists() {
        let pattern_count = fs::read_to_string(&ignore_path)
            .unwrap_or_default()
            .lines()
            .filter(|l| !l.trim().is_empty() && !l.trim().starts_with('#'))
            .count();
        println!(".codecartographerignore: {} pattern(s)", pattern_count);
    } else {
        println!(".codecartographerignore: not present");
    }

    println!("============================================");
    Ok(())
}

fn config_mode(
    default_target: Option<String>,
    show: bool,
) -> Result<()> {
    if default_target.is_none() && !show {
        println!("Usage:");
        println!("  codecartographer config --show");
        println!("  codecartographer config --default-target <claude|cursor|raw>");
        return Ok(());
    }

    if show {
        let global = global_config::GlobalConfig::load();
        let path = global_config::GlobalConfig::config_path()
            .map(|p| p.display().to_string())
            .unwrap_or_else(|| "(unknown)".into());
        println!("Global config: {}", path);
        println!(
            "  defaults.target:  {}",
            global.defaults.target.as_deref().unwrap_or("(not set, defaults to claude)")
        );
        return Ok(());
    }

    let mut global = global_config::GlobalConfig::load();
    let mut changed = false;

    if let Some(t) = default_target {
        // Validate
        t.parse::<OutputTarget>()
            .map_err(|e| anyhow::anyhow!("{}", e))?;
        global.defaults.target = Some(t.clone());
        changed = true;
        println!("Default target set to '{}'.", t);
    }

    if changed {
        global.save()?;
        let path = global_config::GlobalConfig::config_path()
            .map(|p| p.display().to_string())
            .unwrap_or_else(|| "(unknown)".into());
        println!("Saved to {}", path);
    }

    Ok(())
}

fn format_timestamp(secs: u64) -> String {
    if secs == 0 {
        return "never".into();
    }
    use std::time::{Duration, UNIX_EPOCH};
    let dt = UNIX_EPOCH + Duration::from_secs(secs);
    match dt.elapsed() {
        Ok(elapsed) => {
            let s = elapsed.as_secs();
            if s < 60 {
                format!("{s}s ago")
            } else if s < 3600 {
                format!("{}m ago", s / 60)
            } else if s < 86400 {
                format!("{}h ago", s / 3600)
            } else {
                format!("{}d ago", s / 86400)
            }
        }
        Err(_) => "unknown".into(),
    }
}

fn init_ckb_mode(root: &Path, ckb_url: Option<&str>, webhook_url: Option<&str>) -> Result<()> {
    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║      CodeCartographer v1.0.0 - CKB Integration Setup           ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();

    let config_path = root.join(".codecartographer").join("config.toml");
    let config_dir = config_path.parent().unwrap();
    std::fs::create_dir_all(config_dir)?;

    let ckb_url = ckb_url.unwrap_or("http://localhost:8080");
    let webhook_url = webhook_url.unwrap_or("http://localhost:8081/webhook");

    let config_content = format!(
        r#"# CodeCartographer Configuration
version = "1.0.0"

[ckb]
url = "{}"
enabled = true

[webhooks]
enabled = true
url = "{}"
events = ["graph_updated", "module_changed", "layer_violation"]

[layers]
# Define your architectural layers here
# Example:
# ui = ["components", "pages", "hooks"]
# services = ["api", "auth"]
# db = ["models", "repositories"]

[allowed_flows]
# Define allowed dependency flows
# Example:
# ui -> services
# services -> db
"#,
        ckb_url, webhook_url
    );

    std::fs::write(&config_path, config_content)?;

    println!("✓ Created configuration at: {}", config_path.display());
    println!();
    println!("📋 Next steps:");
    println!("  1. Add layer definitions to {}", config_path.display());
    println!("  2. Run 'codecartographer map' to generate initial graph");
    println!("  3. Run 'codecartographer health' to see architectural health");
    println!();
    println!("🔗 CKB Integration:");
    println!("  - CKB URL: {}", ckb_url);
    println!("  - Webhook URL: {}", webhook_url);
    println!();
    println!("✅ CodeCartographer is ready to integrate with CKB!");

    Ok(())
}

fn health_mode(root: &Path, compare: Option<&str>, rollup: Option<usize>, json_out: bool) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    if !json_out {
        println!("╔═══════════════════════════════════════════════════════════╗");
        println!("║         CodeCartographer - Architectural Health Report          ║");
        println!("╚═══════════════════════════════════════════════════════════╝");
        println!();
    }

    let result = scan_files_with_noise_tracking(root)?;
    let files = result.files;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    // Directory-level rollup: fold files into subsystems before analysis. --compare
    // operates on the file-level graph only, so the two don't combine.
    let rollup_depth = rollup.filter(|d| *d > 0);
    if rollup_depth.is_some() && compare.is_some() {
        anyhow::bail!("--rollup and --compare cannot be combined (compare is file-level)");
    }
    let graph = match rollup_depth {
        Some(depth) => state.rebuild_graph_rolled_up(depth).map_err(|e| anyhow::anyhow!(e))?,
        None => state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?,
    };
    if let Some(depth) = rollup_depth {
        if !json_out {
            println!(
                "📁 Directory rollup (depth {depth}): {} subsystems, {} cross-subsystem deps\n",
                graph.metadata.total_files, graph.metadata.total_edges
            );
        }
    }
    let score_now = graph.metadata.health_score.unwrap_or(0.0);
    let bridges_now = graph.metadata.bridge_count.unwrap_or(0);
    let cycles_now = graph.metadata.cycle_count.unwrap_or(0);
    let god_now = graph.metadata.god_module_count.unwrap_or(0);
    let violations_now = graph.metadata.layer_violation_count.unwrap_or(0);

    // --compare mode: compute the same metrics at the given ref and show delta.
    if let Some(git_ref) = compare {
        let old_graph = health_graph_at_ref(root, git_ref)?;
        let score_old = old_graph.metadata.health_score.unwrap_or(0.0);
        let bridges_old = old_graph.metadata.bridge_count.unwrap_or(0);
        let cycles_old = old_graph.metadata.cycle_count.unwrap_or(0);
        let god_old = old_graph.metadata.god_module_count.unwrap_or(0);
        let violations_old = old_graph.metadata.layer_violation_count.unwrap_or(0);

        if json_out {
            let out = serde_json::json!({
                "base_ref": git_ref,
                "base": { "score": score_old, "bridges": bridges_old, "cycles": cycles_old,
                           "god_modules": god_old, "layer_violations": violations_old },
                "head": { "score": score_now, "bridges": bridges_now, "cycles": cycles_now,
                          "god_modules": god_now, "layer_violations": violations_now },
                "delta": {
                    "score": score_now - score_old,
                    "bridges": bridges_now as i64 - bridges_old as i64,
                    "cycles": cycles_now as i64 - cycles_old as i64,
                    "god_modules": god_now as i64 - god_old as i64,
                    "layer_violations": violations_now as i64 - violations_old as i64,
                }
            });
            println!("{}", serde_json::to_string_pretty(&out)?);
            return Ok(());
        }

        fn delta_str(now: f64, old: f64) -> String {
            let d = now - old;
            if d > 0.01 { format!("+{:.1}", d) }
            else if d < -0.01 { format!("{:.1}", d) }
            else { "±0".to_string() }
        }
        fn idelta(now: usize, old: usize) -> String {
            match now.cmp(&old) {
                std::cmp::Ordering::Greater => format!("+{}", now - old),
                std::cmp::Ordering::Less    => format!("-{}", old - now),
                std::cmp::Ordering::Equal   => "±0".to_string(),
            }
        }

        println!("Comparing HEAD → {}", git_ref);
        println!();
        println!(
            "  Health Score:      {:.1} → {:.1}  ({})",
            score_old, score_now, delta_str(score_now, score_old)
        );
        println!(
            "  Bridges:           {} → {}  ({})",
            bridges_old, bridges_now, idelta(bridges_now, bridges_old)
        );
        println!(
            "  Cycles:            {} → {}  ({})",
            cycles_old, cycles_now, idelta(cycles_now, cycles_old)
        );
        println!(
            "  God Modules:       {} → {}  ({})",
            god_old, god_now, idelta(god_now, god_old)
        );
        println!(
            "  Layer Violations:  {} → {}  ({})",
            violations_old, violations_now, idelta(violations_now, violations_old)
        );
        return Ok(());
    }

    if json_out {
        let out = serde_json::json!({
            "score": score_now,
            "files": graph.metadata.total_files,
            "dependencies": graph.metadata.total_edges,
            "bridges": bridges_now,
            "cycles": cycles_now,
            "god_modules": god_now,
            "layer_violations": violations_now,
        });
        println!("{}", serde_json::to_string_pretty(&out)?);
        return Ok(());
    }

    println!(
        "📊 Health Score: {:.1}/100",
        score_now
    );
    println!();

    println!("📈 Statistics:");
    println!("  - Files: {}", graph.metadata.total_files);
    println!("  - Dependencies: {}", graph.metadata.total_edges);
    println!("  - Bridges: {}", bridges_now);
    println!("  - Cycles: {}", cycles_now);
    println!("  - God Modules: {}", god_now);
    println!("  - Layer Violations: {}", violations_now);
    println!();

    if !graph.cycles.is_empty() {
        println!("🔴 Critical Issues (Cycles):");
        for (i, cycle) in graph.cycles.iter().take(3).enumerate() {
            println!(
                "  {}. {} - {}",
                i + 1,
                cycle.severity,
                cycle.nodes.join(" -> ")
            );
        }
        println!();
    }

    // Bridge remediation hints: for each bridge, show who depends on it and suggest action.
    let bridge_nodes: Vec<_> = graph
        .nodes
        .iter()
        .filter(|n| n.is_bridge == Some(true))
        .collect();

    if !bridge_nodes.is_empty() {
        // Pre-compute fan-in (callers) and fan-out (deps) from the edge list.
        let mut fan_in: std::collections::HashMap<&str, Vec<&str>> =
            std::collections::HashMap::new();
        let mut fan_out: std::collections::HashMap<&str, Vec<&str>> =
            std::collections::HashMap::new();
        for edge in &graph.edges {
            fan_in
                .entry(edge.target.as_str())
                .or_default()
                .push(edge.source.as_str());
            fan_out
                .entry(edge.source.as_str())
                .or_default()
                .push(edge.target.as_str());
        }

        // Sort bridges by score descending so the worst ones appear first.
        let mut bridges_sorted = bridge_nodes;
        bridges_sorted.sort_by(|a, b| {
            b.bridge_score
                .unwrap_or(0.0)
                .partial_cmp(&a.bridge_score.unwrap_or(0.0))
                .unwrap_or(std::cmp::Ordering::Equal)
        });

        println!("🌉 Bridge Remediation Hints (top {}):", bridges_sorted.len().min(5));
        for node in bridges_sorted.iter().take(5) {
            let callers = fan_in.get(node.module_id.as_str()).map(|v| v.len()).unwrap_or(0);
            let deps    = fan_out.get(node.module_id.as_str()).map(|v| v.len()).unwrap_or(0);
            let risk    = node.risk_level.as_deref().unwrap_or("?");

            // Group caller directories to suggest split domains.
            let caller_dirs: std::collections::BTreeSet<&str> = fan_in
                .get(node.module_id.as_str())
                .map(|callers| {
                    callers
                        .iter()
                        .filter_map(|p| p.rsplit_once('/').map(|(dir, _)| dir))
                        .collect()
                })
                .unwrap_or_default();

            let is_entry = crate::api::is_entry_point_path(&node.path);
            // C/C++/Obj-C implementation units are compiled directly, never
            // #included, so "0 importers" is the normal case — not dead code.
            let is_impl_unit = matches!(
                node.path.rsplit('.').next().unwrap_or(""),
                "cpp" | "cc" | "cxx" | "c++" | "c" | "m" | "mm"
            );
            let suggestion = if callers == 0 && is_entry {
                "no callers — entry point; this is expected".to_string()
            } else if callers == 0 && is_impl_unit {
                "no importers — translation unit compiled directly, not #included; expected".to_string()
            } else if callers == 0 {
                "no callers — likely dead bridge, consider removal".to_string()
            } else if caller_dirs.len() >= 2 {
                let dirs: Vec<&str> = caller_dirs.iter().copied().take(3).collect();
                format!(
                    "callers span {} domains ({}) — split by domain or introduce a façade",
                    caller_dirs.len(),
                    dirs.join(", ")
                )
            } else if deps >= 5 {
                format!("imports {deps} modules — too many dependencies; extract a sub-layer")
            } else {
                format!("{callers} callers, {deps} deps — monitor; may be an intentional mediator")
            };

            println!(
                "  [{risk}] {} — {callers} callers, {deps} deps. {suggestion}",
                node.path
            );
        }
        println!();
    }

    if score_now < 70.0 {
        println!("⚠️  Architectural health is below acceptable threshold.");
        println!("   Run 'codecartographer map --detail extended' for more information.");
    } else {
        println!("✅ Architecture looks healthy!");
    }

    Ok(())
}

/// Build a ProjectGraphResponse using file contents from a git ref.
fn health_graph_at_ref(
    root: &Path,
    git_ref: &str,
) -> Result<crate::api::ProjectGraphResponse> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::is_source_file;

    // List all files at the ref.
    let ls = std::process::Command::new("git")
        .arg("-C").arg(root)
        .args(["ls-tree", "--name-only", "-r", git_ref])
        .output()
        .map_err(|e| anyhow::anyhow!("git ls-tree: {}", e))?;
    if !ls.status.success() {
        anyhow::bail!(
            "git ls-tree failed for ref '{}': {}",
            git_ref,
            String::from_utf8_lossy(&ls.stderr).trim()
        );
    }

    let rel_paths: Vec<String> = String::from_utf8_lossy(&ls.stdout)
        .lines()
        .map(str::trim)
        .filter(|l| !l.is_empty())
        .filter(|l| is_source_file(std::path::Path::new(l)))
        .map(str::to_owned)
        .collect();

    let mut mapped_files = std::collections::HashMap::new();
    for rel in rel_paths {
        let blob = format!("{}:{}", git_ref, rel);
        let out = std::process::Command::new("git")
            .arg("-C").arg(root)
            .args(["show", &blob])
            .output();
        if let Ok(out) = out {
            if out.status.success() {
                if let Ok(content) = String::from_utf8(out.stdout) {
                    let path = root.join(&rel);
                    let mf = extract_skeleton(&path, &content);
                    mapped_files.insert(rel, mf);
                }
            }
        }
    }

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }
    state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))
}

fn simulate_mode(
    root: &Path,
    module: Option<&str>,
    new_signature: Option<&str>,
    remove_signature: Option<&str>,
    staged: bool,
    diff_ref: Option<&str>,
    fail_on_cycle: bool,
    json_out: bool,
) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    if !json_out {
        println!("╔═══════════════════════════════════════════════════════════╗");
        println!("║         Predictive Impact Analysis                         ║");
        println!("╚═══════════════════════════════════════════════════════════╝");
        println!();
    }

    let result = scan_files_with_noise_tracking(root)?;
    let files = result.files;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    // --staged / --diff mode: analyse every changed source file.
    if staged || diff_ref.is_some() {
        let changed = git_changed_source_files(root, staged, diff_ref)?;
        if changed.is_empty() {
            println!("No source file changes detected.");
            return Ok(());
        }

        // Optionally narrow to a single module.
        let targets: Vec<String> = if let Some(m) = module {
            changed.into_iter().filter(|f| f == m).collect()
        } else {
            changed
        };

        if targets.is_empty() {
            println!("No changes in the requested module.");
            return Ok(());
        }

        let mut any_cycle = false;
        let mut total_affected: std::collections::HashSet<String> =
            std::collections::HashSet::new();
        let mut max_callers = 0usize;
        let mut per_module: Vec<serde_json::Value> = Vec::new();

        for target in &targets {
            match state.simulate_change(target, None, None) {
                Ok(change) => {
                    if change.predicted_impact.will_create_cycle {
                        any_cycle = true;
                    }
                    max_callers = max_callers.max(change.predicted_impact.callers_count);
                    for m in &change.predicted_impact.affected_modules {
                        total_affected.insert(m.clone());
                    }
                    if json_out {
                        per_module.push(serde_json::json!({
                            "module": target,
                            "callers": change.predicted_impact.callers_count,
                            "risk": change.predicted_impact.risk_level,
                            "will_create_cycle": change.predicted_impact.will_create_cycle,
                        }));
                    } else {
                        let cycle_tag = if change.predicted_impact.will_create_cycle {
                            " ⚠️  CYCLE"
                        } else {
                            ""
                        };
                        println!(
                            "  {} — {} callers, risk {}{}",
                            target,
                            change.predicted_impact.callers_count,
                            change.predicted_impact.risk_level,
                            cycle_tag
                        );
                    }
                }
                Err(e) => {
                    if json_out {
                        per_module.push(serde_json::json!({"module": target, "error": e.to_string()}));
                    } else {
                        println!("  {} — skipped ({})", target, e);
                    }
                }
            }
        }

        if json_out {
            let out = serde_json::json!({
                "mode": if staged { "staged" } else { "diff" },
                "modules": per_module,
                "total_unique_affected": total_affected.len(),
                "any_cycle": any_cycle,
            });
            println!("{}", serde_json::to_string_pretty(&out)?);
        } else {
            println!();
            println!(
                "📦 Total unique affected modules: {}",
                total_affected.len()
            );
            if any_cycle {
                println!("⚠️  WARNING: One or more changes would create a circular dependency.");
            }
        }
        if fail_on_cycle && any_cycle {
            std::process::exit(1);
        }
        return Ok(());
    }

    // Single-module mode (original behaviour).
    let module = module.ok_or_else(|| {
        anyhow::anyhow!("--module is required unless --staged or --diff <ref> is used")
    })?;

    let change = state
        .simulate_change(module, new_signature, remove_signature)
        .map_err(|e| anyhow::anyhow!(e))?;

    if json_out {
        let out = serde_json::json!({
            "module": change.target_module,
            "new_signature": change.new_signature,
            "removed_signature": change.removed_signature,
            "risk": change.predicted_impact.risk_level,
            "health_impact": change.predicted_impact.health_impact,
            "callers": change.predicted_impact.callers_count,
            "callees": change.predicted_impact.callees_count,
            "will_create_cycle": change.predicted_impact.will_create_cycle,
            "affected_modules": change.predicted_impact.affected_modules,
        });
        println!("{}", serde_json::to_string_pretty(&out)?);
        if fail_on_cycle && change.predicted_impact.will_create_cycle {
            std::process::exit(1);
        }
        return Ok(());
    }

    println!("🎯 Target: {}", change.target_module);
    println!();
    println!("📊 Impact Analysis:");
    println!("   Risk Level: {}", change.predicted_impact.risk_level);
    println!(
        "   Health Impact: {:.1}",
        change.predicted_impact.health_impact
    );
    println!(
        "   Direct Callers: {}",
        change.predicted_impact.callers_count
    );
    println!(
        "   Direct Callees: {}",
        change.predicted_impact.callees_count
    );
    println!();

    if change.predicted_impact.will_create_cycle {
        println!("⚠️  WARNING: This change will create a circular dependency!");
    }

    if !change.predicted_impact.layer_violations.is_empty() {
        println!(
            "🚨 Layer Violations: {}",
            change.predicted_impact.layer_violations.len()
        );
        for v in &change.predicted_impact.layer_violations {
            println!(
                "   - {} -> {} ({})",
                v.source_layer,
                v.target_layer,
                v.violation_type.as_str()
            );
        }
    }

    if !change.predicted_impact.affected_modules.is_empty() {
        println!(
            "📦 Affected Modules ({}):",
            change.predicted_impact.affected_modules.len()
        );
        for m in change.predicted_impact.affected_modules.iter().take(5) {
            println!("   - {}", m);
        }
        if change.predicted_impact.affected_modules.len() > 5 {
            println!(
                "   ... and {} more",
                change.predicted_impact.affected_modules.len() - 5
            );
        }
    }

    if fail_on_cycle && change.predicted_impact.will_create_cycle {
        std::process::exit(1);
    }

    Ok(())
}

/// Returns true when `rel_path` has a recognisable sibling test file next to it.
///
/// Checks patterns like `foo_test.rs`, `test_foo.py`, `foo.test.ts`, `foo.spec.ts`,
/// and a `tests/` or `__tests__/` directory containing a file that starts with the
/// stem of `rel_path`.
fn has_sibling_test(root: &Path, rel_path: &str) -> bool {
    let p = std::path::Path::new(rel_path);
    let stem = match p.file_stem().and_then(|s| s.to_str()) {
        Some(s) => s,
        None => return false,
    };
    let ext = p.extension().and_then(|e| e.to_str()).unwrap_or("");
    let dir = p.parent().unwrap_or(std::path::Path::new(""));

    // Inline sibling patterns: foo_test.rs, test_foo.py, foo.test.ts, foo.spec.ts
    let candidates = [
        format!("{}_test.{}", stem, ext),
        format!("test_{}.{}", stem, ext),
        format!("{}.test.{}", stem, ext),
        format!("{}.spec.{}", stem, ext),
        format!("{}Tests.{}", stem, ext),
        format!("{}Test.{}", stem, ext),
    ];
    for c in &candidates {
        if root.join(dir).join(c).exists() {
            return true;
        }
    }

    // tests/ or __tests__/ sibling directory containing a file whose name starts with the stem.
    for test_dir in &["tests", "__tests__", "test"] {
        let candidate_dir = root.join(dir).join(test_dir);
        if candidate_dir.is_dir() {
            if let Ok(rd) = std::fs::read_dir(&candidate_dir) {
                for entry in rd.flatten() {
                    let name = entry.file_name();
                    let name_str = name.to_string_lossy();
                    if name_str.starts_with(stem) || name_str.contains(&format!("_{}", stem)) {
                        return true;
                    }
                }
            }
        }
    }

    false
}

/// Returns repo-relative paths of source files changed in the working tree.
/// `staged=true`  → `git diff --cached --name-only`
/// `diff_ref=Some` → `git diff <ref> --name-only`
fn git_changed_source_files(
    root: &Path,
    staged: bool,
    diff_ref: Option<&str>,
) -> Result<Vec<String>> {
    use crate::scanner::is_source_file;

    let mut cmd = std::process::Command::new("git");
    cmd.arg("-C").arg(root).arg("diff").arg("--name-only");
    if staged {
        cmd.arg("--cached");
    }
    if let Some(r) = diff_ref {
        cmd.arg(r);
    }

    let out = cmd.output().map_err(|e| anyhow::anyhow!("git: {}", e))?;
    if !out.status.success() {
        anyhow::bail!(
            "git diff failed: {}",
            String::from_utf8_lossy(&out.stderr).trim()
        );
    }

    let paths = String::from_utf8_lossy(&out.stdout)
        .lines()
        .map(str::trim)
        .filter(|l| !l.is_empty())
        .filter(|l| is_source_file(Path::new(l)))
        .map(str::to_owned)
        .collect();

    Ok(paths)
}

fn evolution_mode(root: &Path, days: Option<u32>) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         Architecture Evolution Report                      ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();

    let result = scan_files_with_noise_tracking(root)?;
    let files = result.files;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let evolution = state.get_evolution(days).map_err(|e| anyhow::anyhow!(e))?;

    println!("📈 Current Status:");
    if let Some(snapshot) = evolution.snapshots.first() {
        println!("   Health Score: {:.1}/100", snapshot.health_score);
        println!("   Files: {}", snapshot.total_files);
        println!("   Dependencies: {}", snapshot.total_edges);
        println!("   Bridges: {}", snapshot.bridge_count);
        println!();
        println!("📊 Trend: {}", evolution.health_trend);
        println!();
    }

    if !evolution.debt_indicators.is_empty() {
        println!("⚠️  Debt Indicators:");
        for debt in &evolution.debt_indicators {
            println!("   • {}", debt);
        }
        println!();
    }

    println!("💡 Recommendations:");
    for rec in &evolution.recommendations {
        println!("   • {}", rec);
    }

    Ok(())
}

// =============================================================================
// DEPS MODE - Show dependencies of a target module as JSON
// =============================================================================

fn deps_mode(root: &Path, target: &str, _format: &str) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    let result = scan_files_with_noise_tracking(root)?;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    // Populate project_graph so get_dependencies_internal can traverse edges
    state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;

    let nodes = state
        .search_graph(target, None)
        .map_err(|e| anyhow::anyhow!(e))?;

    let node = nodes
        .first()
        .ok_or_else(|| anyhow::anyhow!("Target not found: {}", target))?;

    let deps = state
        .get_dependencies_internal(&node.module_id, 1)
        .map_err(|e| anyhow::anyhow!(e))?
        .unwrap_or_default();

    let output = serde_json::json!({
        "node_id": node.module_id,
        "node_name": node.path,
        "dependencies": deps,
    });

    println!("{}", serde_json::to_string_pretty(&output)?);
    Ok(())
}

// =============================================================================
// GIT ENRICHMENT — Populate hotspot/cochange data on a ProjectGraphResponse.
// Lives here (not in api.rs) because git_analysis is a binary-only module.
// =============================================================================

fn enrich_with_git(graph: &mut crate::api::ProjectGraphResponse, root: &Path) {
    let churn = crate::git_analysis::git_churn(root, 500);
    if churn.is_empty() {
        return;
    }

    // git_churn / git_cochange / git_ownership all return repo-relative paths
    // (output of `git log --name-only`). node.module_id is also repo-relative
    // (stripped from root), while node.path is the absolute filesystem path.
    // Always key on module_id for git lookups.
    let max_raw = graph
        .nodes
        .iter()
        .map(|n| {
            let c = *churn.get(&n.module_id).unwrap_or(&0);
            c * n.signature_count
        })
        .max()
        .unwrap_or(1)
        .max(1) as f64;

    let mut hotspot_count = 0usize;
    for node in &mut graph.nodes {
        let c = *churn.get(&node.module_id).unwrap_or(&0);
        node.churn = Some(c);
        let score = ((c * node.signature_count) as f64 / max_raw * 100.0).round();
        node.hotspot_score = Some(score);
        if score >= 20.0 {
            hotspot_count += 1;
        }
    }
    graph.metadata.hotspot_count = Some(hotspot_count);

    let known: std::collections::HashSet<&str> =
        graph.nodes.iter().map(|n| n.module_id.as_str()).collect();
    graph.cochange_pairs = crate::git_analysis::git_cochange(root, 500)
        .into_iter()
        .filter(|p| known.contains(p.file_a.as_str()) && known.contains(p.file_b.as_str()))
        .map(|p| crate::api::CoChangePair {
            file_a: p.file_a,
            file_b: p.file_b,
            count: p.count,
            coupling_score: p.coupling_score,
        })
        .collect();

    // Co-change dispersion — shotgun surgery signal.
    let dispersion = crate::git_analysis::git_cochange_dispersion(root, 500);
    if !dispersion.is_empty() {
        let disp_map: std::collections::HashMap<&str, &crate::git_analysis::CoChangeDispersion> =
            dispersion.iter().map(|d| (d.file.as_str(), d)).collect();
        for node in &mut graph.nodes {
            if let Some(d) = disp_map.get(node.module_id.as_str()) {
                node.cochange_partners = Some(d.partner_count);
                node.cochange_entropy = Some((d.entropy * 100.0).round() / 100.0);
            }
        }
    }

    // Dominant-author ownership. One git call, no coupling to churn/cochange
    // so it stays available even on repos where the other signals are empty.
    let ownership = crate::git_analysis::git_ownership(root, 500);
    if !ownership.is_empty() {
        for node in &mut graph.nodes {
            if let Some(owner) = ownership.get(&node.module_id) {
                node.owner = Some(owner.clone());
            }
        }
    }
}

// =============================================================================
// COCHANGE MODE — Temporal coupling analysis from git history
// =============================================================================

fn cochange_mode(
    root: &Path,
    commits: usize,
    min_count: usize,
    cluster: bool,
    threshold: f64,
    json_out: bool,
) -> Result<()> {
    let pairs = crate::git_analysis::git_cochange(root, commits);

    let mut filtered: Vec<_> = pairs.iter().filter(|p| p.count >= min_count).collect();
    filtered.sort_by(|a, b| {
        b.coupling_score
            .partial_cmp(&a.coupling_score)
            .unwrap_or(std::cmp::Ordering::Equal)
    });

    if cluster {
        return cochange_cluster_mode(root, &pairs, threshold, json_out);
    }

    if json_out {
        let items: Vec<_> = filtered.iter().map(|p| serde_json::json!({
            "file_a": p.file_a,
            "file_b": p.file_b,
            "count": p.count,
            "coupling_score": (p.coupling_score * 1000.0).round() / 1000.0,
        })).collect();
        println!("{}", serde_json::to_string_pretty(&serde_json::json!({
            "commits": commits,
            "min_count": min_count,
            "pairs": items,
        }))?);
        return Ok(());
    }

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         Temporal Coupling Analysis                         ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();
    println!("Last {} commits", commits);
    println!();

    if filtered.is_empty() {
        println!("No co-change pairs found with count >= {}.", min_count);
        if min_count > 2 {
            println!("Tip: try a lower threshold — e.g. `codecartographer cochange --min-count 2`");
        }
        return Ok(());
    }

    for pair in &filtered {
        println!(
            "  {} ↔ {} | coupled {} times | score: {:.2}",
            pair.file_a, pair.file_b, pair.count, pair.coupling_score
        );
    }

    println!();
    println!("Note: High coupling score with no import link = hidden dependency.");

    Ok(())
}

fn cochange_cluster_mode(
    _root: &Path,
    pairs: &[crate::git_analysis::CoChangePair],
    threshold: f64,
    json_out: bool,
) -> Result<()> {
    // Union-Find over files that are strongly coupled (score >= threshold).
    // Each connected component is an "implicit module" — files that behave
    // as a unit at the change level regardless of declared structure.
    let mut parent: std::collections::HashMap<String, String> = HashMap::new();

    fn find(parent: &mut std::collections::HashMap<String, String>, x: &str) -> String {
        if parent.get(x).map(|p| p == x).unwrap_or(true) {
            parent.insert(x.to_string(), x.to_string());
            return x.to_string();
        }
        let p = parent[x].clone();
        let root = find(parent, &p);
        parent.insert(x.to_string(), root.clone());
        root
    }

    fn union(parent: &mut std::collections::HashMap<String, String>, a: &str, b: &str) {
        let ra = find(parent, a);
        let rb = find(parent, b);
        if ra != rb {
            parent.insert(rb, ra);
        }
    }

    // Seed every file that appears in any pair.
    for p in pairs {
        parent.entry(p.file_a.clone()).or_insert_with(|| p.file_a.clone());
        parent.entry(p.file_b.clone()).or_insert_with(|| p.file_b.clone());
    }

    // Union pairs above threshold.
    for p in pairs {
        if p.coupling_score >= threshold {
            union(&mut parent, &p.file_a, &p.file_b);
        }
    }

    // Collect communities: root → sorted member list, plus max internal coupling score.
    let mut communities: std::collections::HashMap<String, Vec<String>> = HashMap::new();
    let keys: Vec<String> = parent.keys().cloned().collect();
    for key in &keys {
        let root = find(&mut parent, key);
        communities.entry(root).or_default().push(key.clone());
    }

    // Compute max coupling score inside each community.
    let mut community_scores: std::collections::HashMap<String, f64> = HashMap::new();
    for p in pairs {
        if p.coupling_score >= threshold {
            let root = find(&mut parent, &p.file_a);
            let entry = community_scores.entry(root).or_insert(0.0);
            if p.coupling_score > *entry {
                *entry = p.coupling_score;
            }
        }
    }

    // Sort communities: largest first, singletons last.
    let mut groups: Vec<(String, Vec<String>, f64)> = communities
        .into_iter()
        .filter(|(_, members)| members.len() > 1)
        .map(|(root, mut members)| {
            members.sort();
            let score = community_scores.get(&root).copied().unwrap_or(0.0);
            (root, members, score)
        })
        .collect();
    groups.sort_by(|a, b| b.1.len().cmp(&a.1.len()).then_with(|| {
        b.2.partial_cmp(&a.2).unwrap_or(std::cmp::Ordering::Equal)
    }));

    if json_out {
        let items: Vec<_> = groups.iter().enumerate().map(|(i, (_, members, score))| {
            serde_json::json!({
                "community": i + 1,
                "size": members.len(),
                "max_coupling": (*score * 1000.0).round() / 1000.0,
                "files": members,
            })
        }).collect();
        println!("{}", serde_json::to_string_pretty(&serde_json::json!({
            "threshold": threshold,
            "communities": items,
        }))?);
        return Ok(());
    }

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         Co-Change Community Detection                      ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();
    println!("Coupling threshold: {:.2}  |  {} implicit module(s) found", threshold, groups.len());
    println!();

    if groups.is_empty() {
        println!("No communities found above threshold {:.2}.", threshold);
        println!("Try lowering --threshold (e.g. --threshold 0.3).");
        return Ok(());
    }

    for (i, (_, members, score)) in groups.iter().enumerate() {
        println!("  Community {} ({} files, max coupling: {:.2})", i + 1, members.len(), score);
        for m in members {
            println!("    {}", m);
        }
        println!();
    }

    println!("Communities are groups of files that always change together.");
    println!("Compare against your declared module structure to find hidden coupling.");

    Ok(())
}

// =============================================================================
// HOTSPOTS MODE — High churn × high complexity files
// =============================================================================

fn todo_mode(root: &Path, top: usize, json_out: bool) -> Result<()> {
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    let result = scan_files_with_noise_tracking(root)?;

    // Regex patterns for debt markers (case-insensitive).
    let markers = ["TODO", "FIXME", "HACK", "XXX", "WORKAROUND", "NOCOMMIT"];

    struct FileDebt {
        path: String,
        counts: Vec<(&'static str, usize)>,
        total: usize,
    }

    let mut debts: Vec<FileDebt> = result
        .files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            let upper = content.to_uppercase();
            let counts: Vec<(&'static str, usize)> = markers
                .iter()
                .map(|&m| {
                    let count = upper.match_indices(m).count();
                    (m, count)
                })
                .filter(|(_, c)| *c > 0)
                .collect();
            let total: usize = counts.iter().map(|(_, c)| c).sum();
            if total == 0 {
                return None;
            }
            Some(FileDebt { path: rel, counts, total })
        })
        .collect();

    debts.sort_by(|a, b| b.total.cmp(&a.total));

    let grand_total: usize = debts.iter().map(|d| d.total).sum();

    if json_out {
        let items: Vec<_> = debts.iter().take(top).map(|d| {
            let breakdown: serde_json::Map<String, serde_json::Value> = d.counts.iter()
                .map(|(k, v)| (k.to_string(), serde_json::json!(v)))
                .collect();
            serde_json::json!({
                "path": d.path,
                "total": d.total,
                "breakdown": breakdown,
            })
        }).collect();
        println!("{}", serde_json::to_string_pretty(&serde_json::json!({
            "grand_total": grand_total,
            "files_with_debt": debts.len(),
            "results": items,
        }))?);
        return Ok(());
    }

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         TODO / FIXME / HACK Density                       ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();
    println!("{} markers across {} files", grand_total, debts.len());
    println!();

    if debts.is_empty() {
        println!("No debt markers found.");
        return Ok(());
    }

    for debt in debts.iter().take(top) {
        let breakdown: Vec<String> = debt
            .counts
            .iter()
            .map(|(k, v)| format!("{}×{}", v, k))
            .collect();
        println!(
            "  {:>4}  {}  [{}]",
            debt.total,
            debt.path,
            breakdown.join(", ")
        );
    }

    Ok(())
}

fn hotspots_mode(
    root: &Path,
    commits: usize,
    top: usize,
    untested: bool,
    by_author: bool,
    bus_factor: bool,
    json_out: bool,
) -> Result<()> {
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    let result = scan_files_with_noise_tracking(root)?;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let churn = crate::git_analysis::git_churn(root, commits);
    let owners = if by_author || json_out {
        crate::git_analysis::git_ownership(root, commits)
    } else {
        HashMap::new()
    };
    let bus = if bus_factor || json_out {
        crate::git_analysis::git_bus_factor(root, commits)
    } else {
        HashMap::new()
    };

    // Compute raw hotspot = churn * sig_count for each file
    let mut scores: Vec<(String, usize, usize, f64)> = mapped_files
        .iter()
        .map(|(path, mf)| {
            let c = *churn.get(path.as_str()).unwrap_or(&0);
            let sigs = mf.signatures.len();
            let raw = (c * sigs) as f64;
            (path.clone(), c, sigs, raw)
        })
        .filter(|(_, c, sigs, _)| *c > 0 && *sigs > 0)
        .collect();

    // Normalize to 0–100
    let max_raw = scores
        .iter()
        .map(|(_, _, _, r)| *r)
        .fold(0.0_f64, f64::max);
    if max_raw > 0.0 {
        for s in &mut scores {
            s.3 = (s.3 / max_raw) * 100.0;
        }
    }

    scores.sort_by(|a, b| b.3.partial_cmp(&a.3).unwrap_or(std::cmp::Ordering::Equal));

    // --untested: keep only files that have no recognisable sibling test file.
    if untested {
        scores.retain(|(path, _, _, _)| !has_sibling_test(root, path));
    }

    if json_out {
        let items: Vec<_> = scores.iter().take(top).map(|(path, c, sigs, score)| {
            let severity = if *score > 80.0 { "CRITICAL" }
                else if *score > 50.0 { "HIGH" }
                else if *score > 20.0 { "MODERATE" }
                else { "LOW" };
            let mut obj = serde_json::json!({
                "path": path,
                "churn": c,
                "signatures": sigs,
                "score": (*score * 10.0).round() / 10.0,
                "severity": severity,
            });
            if let Some(owner) = owners.get(path.as_str()) {
                obj["owner"] = serde_json::Value::String(owner.clone());
            }
            if let Some(&bf) = bus.get(path.as_str()) {
                obj["bus_factor"] = serde_json::Value::Number(bf.into());
            }
            obj
        }).collect();
        println!("{}", serde_json::to_string_pretty(&serde_json::json!({
            "commits": commits,
            "untested_only": untested,
            "hotspots": items,
        }))?);
        return Ok(());
    }

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         Hotspot Analysis                                   ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();
    println!("Last {} commits  |  top {}", commits, top);
    println!();

    if scores.is_empty() {
        println!("No hotspots found (no git history or no source files).");
        return Ok(());
    }

    for (path, c, sigs, score) in scores.iter().take(top) {
        let label = if *score > 80.0 {
            "CRITICAL"
        } else if *score > 50.0 {
            "HIGH    "
        } else if *score > 20.0 {
            "MODERATE"
        } else {
            "LOW     "
        };
        let owner_col = if by_author {
            let o = owners.get(path.as_str()).map(|s| s.as_str()).unwrap_or("unknown");
            format!(" | owner: {}", o)
        } else {
            String::new()
        };
        let bus_col = if bus_factor {
            let bf = bus.get(path.as_str()).copied().unwrap_or(0);
            format!(" | authors: {}", bf)
        } else {
            String::new()
        };
        println!(
            "  [{}] {} | churn: {} commits | sigs: {} | hotspot: {:.1}{}{}",
            label, path, c, sigs, score, owner_col, bus_col
        );
    }

    Ok(())
}

// =============================================================================
// SHOTGUN MODE — Co-change dispersion / shotgun surgery detection
// =============================================================================

fn shotgun_mode(root: &Path, commits: usize, top: usize, min_partners: usize) -> Result<()> {
    use crate::scanner::{IGNORED_FILES, NOISE_LOCK_FILES, NOISE_MAP_EXTENSIONS};
    let mut entries = crate::git_analysis::git_cochange_dispersion(root, commits);

    entries.retain(|e| {
        if e.partner_count < min_partners {
            return false;
        }
        let filename = e.file.rsplit('/').next().unwrap_or(&e.file);
        if IGNORED_FILES.contains(&filename) || NOISE_LOCK_FILES.contains(&filename) {
            return false;
        }
        if let Some(ext) = filename.rsplit('.').next() {
            if NOISE_MAP_EXTENSIONS.contains(&ext) {
                return false;
            }
        }
        true
    });
    entries.truncate(top);

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         Shotgun Surgery Detection                          ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();
    println!("Last {} commits  |  min {} partners  |  top {}", commits, min_partners, top);
    println!();

    if entries.is_empty() {
        println!("No shotgun surgery candidates found.");
        return Ok(());
    }

    for e in &entries {
        let tier = if e.dispersion_score >= 60.0 {
            "HIGH    "
        } else if e.dispersion_score >= 30.0 {
            "MODERATE"
        } else {
            "LOW     "
        };
        println!(
            "  [{}] {:<55} partners: {:>3}  entropy: {:.2}  score: {:.0}",
            tier, e.file, e.partner_count, e.entropy, e.dispersion_score
        );
    }

    println!();
    println!(
        "High entropy + many partners = changes scatter across unrelated modules (shotgun surgery)."
    );

    Ok(())
}

// =============================================================================
// DEAD MODE — Dead code candidates (unreachable in dependency graph)
// =============================================================================

fn dead_mode(root: &Path, json_out: bool) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    let result = scan_files_with_noise_tracking(root)?;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let graph = state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;

    let dead: Vec<_> = graph
        .nodes
        .iter()
        .filter(|n| n.role.as_deref() == Some("dead"))
        .collect();

    let entry: Vec<_> = graph
        .nodes
        .iter()
        .filter(|n| n.role.as_deref() == Some("entry"))
        .collect();

    if json_out {
        let out = serde_json::json!({
            "total_files": graph.metadata.total_files,
            "dead": dead.iter().map(|n| serde_json::json!({"path": n.path, "signatures": n.signature_count})).collect::<Vec<_>>(),
            "entry_points": entry.iter().map(|n| &n.path).collect::<Vec<_>>(),
        });
        println!("{}", serde_json::to_string_pretty(&out)?);
        return Ok(());
    }

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         Dead Code Candidates                               ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();
    println!(
        "Dead code count: {}  |  total files: {}",
        graph.metadata.dead_code_count.unwrap_or(0),
        graph.metadata.total_files
    );
    println!();

    if dead.is_empty() {
        println!("No dead code candidates found.");
    } else {
        println!("Unreachable (in_degree = 0, not entry pattern):");
        for node in &dead {
            println!("  - {} ({} symbols)", node.path, node.signature_count);
        }
    }

    println!();
    println!("Entry points (in_degree = 0, not imported but likely intentional):");
    if entry.is_empty() {
        println!("  (none detected)");
    } else {
        for node in &entry {
            println!("  - {}", node.path);
        }
    }

    println!();
    println!("Note: Confidence is limited by static import analysis. Verify before deleting.");

    Ok(())
}

// =============================================================================
// DIAGRAM MODE — Export dependency graph as Mermaid or DOT
// =============================================================================

#[allow(clippy::too_many_arguments)]
fn diagram_mode(
    root: &Path,
    format: &str,
    output: Option<&Path>,
    max_nodes: usize,
    focus: Option<&str>,
    depth: usize,
    blast_radius: Option<&str>,
    cochange_threshold: Option<f64>,
    docs_only: bool,
    group_by_folder_depth: Option<usize>,
    color_by_owner: bool,
    call_graph_target: Option<&Path>,
    entry: Option<&str>,
) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    // ── Cross-file sequence trace (spike) ────────────────────────────────────
    // `--entry FILE::FUNCTION --format sequence` traces call edges across files
    // using simple-name + import-graph resolution. Requires a full project scan.
    if let Some(entry_spec) = entry {
        let fmt = diagram::DiagramFormat::parse(format).map_err(|e| anyhow::anyhow!(e))?;
        if fmt != diagram::DiagramFormat::Sequence {
            anyhow::bail!("--entry requires --format sequence");
        }
        let (entry_file, entry_fn) = entry_spec.split_once("::").ok_or_else(|| {
            anyhow::anyhow!("--entry must be FILE::FUNCTION (e.g. src/main.rs::diagram_mode)")
        })?;
        let abs_entry = if Path::new(entry_file).is_absolute() {
            Path::new(entry_file).to_path_buf()
        } else {
            root.join(entry_file)
        };

        // Build full mapped-file index for symbol resolution.
        let result = scan_files_with_noise_tracking(root)?;
        let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = result
            .files
            .par_iter()
            .filter(|p| !is_ignored_path(p) && is_source_file(p))
            .filter_map(|p| {
                let content = std::fs::read_to_string(p).ok()?;
                let mapped = extract_skeleton(p, &content);
                let rel = p.strip_prefix(root).unwrap_or(p).to_string_lossy().replace('\\', "/");
                Some((rel, mapped))
            })
            .collect();

        let trace = crate::cross_call::trace_from_entry(
            &abs_entry,
            entry_fn,
            &mapped_files,
            root,
            depth,
        ).map_err(|e| anyhow::anyhow!(e))?;

        let rendered = diagram::render_cross_sequence(&trace);

        if let Some(out_path) = output {
            let kind = diagram_export::export_diagram(&rendered.diagram, fmt, out_path)
                .map_err(|e| anyhow::anyhow!(e))?;
            let verb = match kind {
                diagram_export::ExportKind::Source => "Cross-file sequence written to",
                diagram_export::ExportKind::MermaidSvg => "Cross-file sequence SVG exported to",
                diagram_export::ExportKind::MermaidPng => "Cross-file sequence PNG exported to",
                _ => "Cross-file sequence exported to",
            };
            println!("{}: {}", verb, out_path.display());
        } else {
            println!("{}", rendered.diagram);
        }
        eprintln!(
            "Cross-file trace: {} modules, {} resolved edges, {} unmatched calls",
            rendered.node_count,
            trace.steps.len(),
            trace.unmatched.len(),
        );
        if !trace.unmatched.is_empty() {
            eprintln!("Unmatched (first 5): {:?}", &trace.unmatched[..trace.unmatched.len().min(5)]);
        }
        return Ok(());
    }

    // Call-graph mode is a completely separate code path: we parse a single
    // file, build the function-level graph, and hand it to the same renderer
    // via a shim ProjectGraphResponse. Import-graph options that don't apply
    // (cochange, docs_only, folder collapse, git enrichment) are silently
    // ignored — the alternative is a noisy error that blocks `--call-graph
    // foo.rs --mermaid -o out.mmd`, which is what most callers actually want.
    if let Some(file) = call_graph_target {
        let abs = if file.is_absolute() { file.to_path_buf() } else { root.join(file) };
        let source = std::fs::read_to_string(&abs)
            .map_err(|e| anyhow::anyhow!("cannot read {}: {}", abs.display(), e))?;
        let fmt = diagram::DiagramFormat::parse(format).map_err(|e| anyhow::anyhow!(e))?;

        // Sequence diagrams render directly from the call graph without going
        // through to_project_graph — they need call order, not a node set.
        if fmt == diagram::DiagramFormat::Sequence {
            let cg = call_graph::build_file_call_graph(&abs, &source)
                .map_err(|e| anyhow::anyhow!(e))?
                .ok_or_else(|| anyhow::anyhow!(
                    "sequence diagram not supported for this file type (expected .rs / .py): {}",
                    abs.display()
                ))?;
            let rendered = diagram::render_sequence(&cg);
            if let Some(out_path) = output {
                let kind = diagram_export::export_diagram(&rendered.diagram, fmt, out_path)
                    .map_err(|e| anyhow::anyhow!(e))?;
                let verb = match kind {
                    diagram_export::ExportKind::Source => "Sequence diagram written to",
                    diagram_export::ExportKind::MermaidSvg => "Sequence diagram SVG exported to",
                    diagram_export::ExportKind::MermaidPng => "Sequence diagram PNG exported to",
                    _ => "Sequence diagram exported to",
                };
                println!("{}: {}", verb, out_path.display());
            } else {
                println!("{}", rendered.diagram);
            }
            eprintln!(
                "Sequence: {} participants, {} messages, {} unresolved external calls dropped",
                cg.functions.len(), cg.calls.len(), cg.unresolved_count
            );
            return Ok(());
        }

        // Class diagrams render from a dedicated class extractor.
        if fmt == diagram::DiagramFormat::Class {
            let cg = class_graph::build_class_graph(&abs, &source)
                .map_err(|e| anyhow::anyhow!(e))?
                .ok_or_else(|| anyhow::anyhow!(
                    "class diagram not supported for this file type (supported: .rs .py .ts .tsx .go): {}",
                    abs.display()
                ))?;
            let rendered = diagram::render_class(&cg);
            if let Some(out_path) = output {
                let kind = diagram_export::export_diagram(&rendered.diagram, fmt, out_path)
                    .map_err(|e| anyhow::anyhow!(e))?;
                let verb = match kind {
                    diagram_export::ExportKind::Source => "Class diagram written to",
                    diagram_export::ExportKind::MermaidSvg => "Class diagram SVG exported to",
                    diagram_export::ExportKind::MermaidPng => "Class diagram PNG exported to",
                    _ => "Class diagram exported to",
                };
                println!("{}: {}", verb, out_path.display());
            } else {
                println!("{}", rendered.diagram);
            }
            eprintln!(
                "Class diagram: {} classes, {} relationships",
                cg.classes.len(), cg.relationships.len()
            );
            return Ok(());
        }

        // ER diagrams use the same class extractor as class diagrams.
        if fmt == diagram::DiagramFormat::Er {
            let cg = class_graph::build_class_graph(&abs, &source)
                .map_err(|e| anyhow::anyhow!(e))?
                .ok_or_else(|| anyhow::anyhow!(
                    "er diagram not supported for this file type (supported: .rs .py .ts .tsx .go): {}",
                    abs.display()
                ))?;
            let rendered = diagram::render_er(&cg);
            if let Some(out_path) = output {
                let kind = diagram_export::export_diagram(&rendered.diagram, fmt, out_path)
                    .map_err(|e| anyhow::anyhow!(e))?;
                let verb = match kind {
                    diagram_export::ExportKind::Source => "ER diagram written to",
                    diagram_export::ExportKind::MermaidSvg => "ER diagram SVG exported to",
                    diagram_export::ExportKind::MermaidPng => "ER diagram PNG exported to",
                    _ => "ER diagram exported to",
                };
                println!("{}: {}", verb, out_path.display());
            } else {
                println!("{}", rendered.diagram);
            }
            eprintln!(
                "ER diagram: {} entities, {} relationships",
                cg.classes.len(), rendered.node_count
            );
            return Ok(());
        }

        let cg = call_graph::build_file_call_graph(&abs, &source)
            .map_err(|e| anyhow::anyhow!(e))?
            .ok_or_else(|| anyhow::anyhow!(
                "call-graph extraction not supported for this file type (expected .rs / .py): {}",
                abs.display()
            ))?;
        let graph = call_graph::to_project_graph(&cg, &abs);
        let opts = diagram::RenderOptions {
            format: fmt,
            focus,
            depth,
            max_nodes,
            show_cochange: None,
            blast_radius,
            docs_only: false,
            group_by_folder_depth: None,
            color_by_owner: false,
        };
        let rendered = diagram::render(&graph, &opts).map_err(|e| anyhow::anyhow!(e))?;

        if let Some(out_path) = output {
            let ext = out_path.extension().and_then(|e| e.to_str()).unwrap_or("");
            if ext.eq_ignore_ascii_case("html") {
                let html = html_export::render_html(&graph, &rendered.included);
                fs::write(out_path, &html)?;
                println!("Interactive call-graph HTML written to: {}", out_path.display());
            } else {
                let kind = diagram_export::export_diagram(&rendered.diagram, fmt, out_path)
                    .map_err(|e| anyhow::anyhow!(e))?;
                let verb = match kind {
                    diagram_export::ExportKind::Source => "Call graph written to",
                    diagram_export::ExportKind::MermaidSvg | diagram_export::ExportKind::DotSvg => {
                        "Call-graph SVG exported to"
                    }
                    diagram_export::ExportKind::MermaidPng | diagram_export::ExportKind::DotPng => {
                        "Call-graph PNG exported to"
                    }
                };
                println!("{}: {}", verb, out_path.display());
            }
        } else {
            println!("{}", rendered.diagram);
        }

        eprintln!(
            "Call graph: {} functions, {} edges, {} unresolved external calls",
            cg.functions.len(), cg.calls.len(), cg.unresolved_count
        );
        if rendered.truncated {
            eprintln!("(truncated to {} nodes — raise --max-nodes for more)", max_nodes);
        }
        return Ok(());
    }

    let result = scan_files_with_noise_tracking(root)?;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let mut graph = state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;

    let fmt = diagram::DiagramFormat::parse(format).map_err(|e| anyhow::anyhow!(e))?;

    // Git-backed overlays (co-change, owner coloring, hotspot sizing, churn) need
    // the enrichment pass. Skip it when nothing downstream consumes it — the
    // git calls run in ~100ms on a warm repo, but there's no reason to pay
    // the cost for a plain top-N diagram.
    if cochange_threshold.is_some() || color_by_owner || fmt == diagram::DiagramFormat::Quadrant {
        enrich_with_git(&mut graph, root);
    }
    let opts = diagram::RenderOptions {
        format: fmt,
        focus,
        depth,
        max_nodes,
        show_cochange: cochange_threshold,
        blast_radius,
        docs_only,
        group_by_folder_depth,
        color_by_owner,
    };
    let rendered = diagram::render(&graph, &opts).map_err(|e| anyhow::anyhow!(e))?;

    if rendered.node_count == 0 {
        if graph.nodes.is_empty() {
            eprintln!("No nodes to diagram — no source files were found under this path.");
            eprintln!("Point at a directory that contains source: codecartographer diagram <path>");
        } else {
            eprintln!(
                "No nodes to diagram — {} files were scanned but none share a rendered edge.",
                graph.nodes.len()
            );
            eprintln!("The import graph has no resolvable relationships to draw at this granularity.");
        }
        return Ok(());
    }

    if let Some(out_path) = output {
        // `.html` → interactive explorer (self-contained page).
        // `.svg` / `.png` → shell out to mmdc (Mermaid) or dot (Graphviz).
        // Anything else → write the diagram source verbatim.
        let ext = out_path.extension().and_then(|e| e.to_str()).unwrap_or("");
        if ext.eq_ignore_ascii_case("html") {
            let html = html_export::render_html(&graph, &rendered.included);
            fs::write(out_path, &html)?;
            println!("Interactive HTML written to: {}", out_path.display());
        } else {
            let kind = diagram_export::export_diagram(&rendered.diagram, fmt, out_path)
                .map_err(|e| anyhow::anyhow!(e))?;
            let verb = match kind {
                diagram_export::ExportKind::Source => "Diagram written to",
                diagram_export::ExportKind::MermaidSvg | diagram_export::ExportKind::DotSvg => {
                    "SVG exported to"
                }
                diagram_export::ExportKind::MermaidPng | diagram_export::ExportKind::DotPng => {
                    "PNG exported to"
                }
            };
            println!("{}: {}", verb, out_path.display());
        }
        if rendered.truncated {
            println!("(truncated to {} nodes — raise --max-nodes for more)", max_nodes);
        }
    } else {
        println!("{}", rendered.diagram);
        if rendered.truncated {
            eprintln!("(truncated to {} nodes — raise --max-nodes for more)", max_nodes);
        }
    }

    Ok(())
}

// =============================================================================
// LLMSTXT MODE — Generate llms.txt index for the project
// =============================================================================

fn llmstxt_mode(root: &Path, output: Option<&Path>) -> Result<()> {
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, scan_files_with_noise_tracking};

    // Detect project name
    let project_name = detect_project_name(root);

    let result = scan_files_with_noise_tracking(root)?;
    let mut mapped: Vec<(String, crate::mapper::MappedFile)> = result
        .files
        .iter()
        .filter(|p| !is_ignored_path(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mf = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mf))
        })
        .collect();

    // Sort: entry points first, then by signature count descending
    mapped.sort_by(|(pa, ma), (pb, mb)| {
        let ea = crate::api::is_entry_point_path(pa);
        let eb = crate::api::is_entry_point_path(pb);
        if ea != eb {
            return eb.cmp(&ea); // entry points first
        }
        mb.signatures.len().cmp(&ma.signatures.len())
    });

    let total_files = mapped.len();
    let mut content = format!(
        "# {}\n\n> Codebase index generated by CodeCartographer. {} modules.\n\n## Key Modules\n\n",
        project_name, total_files
    );

    for (rel, mf) in &mapped {
        let sig_count = mf.signatures.len();
        if sig_count == 0 {
            continue;
        }
        let desc = if crate::api::is_entry_point_path(rel) {
            format!("Entry point — {} symbols", sig_count)
        } else {
            format!("{} symbols", sig_count)
        };
        content.push_str(&format!("- [{}]({}): {}\n", rel, rel, desc));
    }

    content.push_str("\n## Ignored\n\n");
    content.push_str("Built with [CodeCartographer](https://github.com/SimplyLiz/CodeCartographer) v1.3.0\n");

    if let Some(out_path) = output {
        fs::write(out_path, &content)?;
        println!("llms.txt written to: {}", out_path.display());
    } else {
        print!("{}", content);
    }

    Ok(())
}

fn detect_project_name(root: &Path) -> String {
    // Try Cargo.toml
    let cargo = root.join("Cargo.toml");
    if cargo.exists() {
        if let Ok(text) = std::fs::read_to_string(&cargo) {
            for line in text.lines() {
                let line = line.trim();
                if line.starts_with("name") {
                    if let Some(val) = line.splitn(2, '=').nth(1) {
                        let name = val.trim().trim_matches('"').trim_matches('\'').to_string();
                        if !name.is_empty() {
                            return name;
                        }
                    }
                }
            }
        }
    }
    // Try package.json
    let pkg = root.join("package.json");
    if pkg.exists() {
        if let Ok(text) = std::fs::read_to_string(&pkg) {
            if let Ok(v) = serde_json::from_str::<serde_json::Value>(&text) {
                if let Some(name) = v["name"].as_str() {
                    return name.to_string();
                }
            }
        }
    }
    // Fall back to directory name
    root.file_name()
        .and_then(|n| n.to_str())
        .unwrap_or("project")
        .to_string()
}

// =============================================================================
// CLAUDEMD MODE — Generate CLAUDE.md architecture guide
// =============================================================================

fn claudemd_mode(root: &Path, output: Option<&Path>) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, scan_files_with_noise_tracking};

    let project_name = detect_project_name(root);

    let result = scan_files_with_noise_tracking(root)?;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .iter()
        .filter(|p| !is_ignored_path(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let mut graph = state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;
    enrich_with_git(&mut graph, root);

    // Language summary: sort by count
    let mut langs: Vec<(String, usize)> = graph.metadata.languages.iter()
        .map(|(k, v)| (k.clone(), *v))
        .collect();
    langs.sort_by(|a, b| b.1.cmp(&a.1));
    let total_lang: usize = langs.iter().map(|(_, c)| c).sum();
    let lang_str = langs
        .iter()
        .map(|(lang, count)| {
            let pct = if total_lang > 0 { *count * 100 / total_lang } else { 0 };
            format!("{} ({}%)", lang, pct)
        })
        .collect::<Vec<_>>()
        .join(", ");

    let mut doc = format!(
        "# Architecture Guide — {}\n\
         <!-- Auto-generated by CodeCartographer v1.3.0. Re-run: codecartographer claudemd -->\n\n\
         ## Overview\n\
         - **Files**: {} | **Dependencies**: {} | **Health**: {:.0}/100\n\
         - **Languages**: {}\n\n",
        project_name,
        graph.metadata.total_files,
        graph.metadata.total_edges,
        graph.metadata.health_score.unwrap_or(0.0),
        lang_str
    );

    // Entry points
    let entries: Vec<_> = graph.nodes.iter()
        .filter(|n| n.role.as_deref() == Some("entry"))
        .collect();
    if !entries.is_empty() {
        doc.push_str("## Key Entry Points\n");
        for n in &entries {
            doc.push_str(&format!("- `{}` — {} symbols\n", n.path, n.signature_count));
        }
        doc.push('\n');
    }

    // Core modules (most-depended-upon)
    let mut core_nodes: Vec<_> = graph.nodes.iter()
        .filter(|n| n.role.as_deref() == Some("core"))
        .collect();
    core_nodes.sort_by(|a, b| b.signature_count.cmp(&a.signature_count));
    if !core_nodes.is_empty() {
        doc.push_str("## Core Modules (most-depended-upon)\n");
        for n in core_nodes.iter().take(10) {
            doc.push_str(&format!(
                "- `{}` — {} symbols, role: core\n",
                n.path, n.signature_count
            ));
        }
        doc.push('\n');
    }

    // Hotspots
    let mut hotspot_nodes: Vec<_> = graph.nodes.iter()
        .filter(|n| n.hotspot_score.map(|s| s > 20.0).unwrap_or(false))
        .collect();
    hotspot_nodes.sort_by(|a, b| {
        b.hotspot_score.unwrap_or(0.0)
            .partial_cmp(&a.hotspot_score.unwrap_or(0.0))
            .unwrap_or(std::cmp::Ordering::Equal)
    });
    if !hotspot_nodes.is_empty() {
        doc.push_str("## Hotspots\n");
        for n in hotspot_nodes.iter().take(5) {
            doc.push_str(&format!(
                "- `{}` — changed {}x, {} symbols (hotspot: {:.0})\n",
                n.path,
                n.churn.unwrap_or(0),
                n.signature_count,
                n.hotspot_score.unwrap_or(0.0)
            ));
        }
        doc.push('\n');
    }

    // Architectural issues
    let has_cycles = !graph.cycles.is_empty();
    let cochange_issues: Vec<_> = graph.cochange_pairs.iter()
        .filter(|p| p.coupling_score >= 0.7)
        .collect();

    if has_cycles || !cochange_issues.is_empty() {
        doc.push_str("## Architectural Issues\n");
        if has_cycles {
            doc.push_str("### Circular Dependencies\n");
            for cycle in graph.cycles.iter().take(5) {
                doc.push_str(&format!(
                    "- {} ({})\n",
                    cycle.nodes.join(" → "),
                    cycle.severity
                ));
            }
            doc.push('\n');
        }
        if !cochange_issues.is_empty() {
            doc.push_str("### Hidden Coupling (no import, always co-change)\n");
            for pair in cochange_issues.iter().take(5) {
                doc.push_str(&format!(
                    "- `{}` ↔ `{}` — coupled {} times (score: {:.2})\n",
                    pair.file_a, pair.file_b, pair.count, pair.coupling_score
                ));
            }
            doc.push('\n');
        }
    }

    // Quick reference
    doc.push_str("## Quick Reference\n```\n\
        codecartographer serve       # Start MCP server\n\
        codecartographer health      # Health report\n\
        codecartographer hotspots    # Churn × complexity\n\
        codecartographer dead        # Dead code candidates\n\
        codecartographer semidiff HEAD~1  # What changed last commit\n\
        ```\n");

    if let Some(out_path) = output {
        fs::write(out_path, &doc)?;
        println!("CLAUDE.md written to: {}", out_path.display());
    } else {
        print!("{}", doc);
    }

    Ok(())
}

// =============================================================================
// SEMIDIFF MODE — Semantic (function-level) diff between two commits
// =============================================================================

fn semidiff_mode(root: &Path, commit1: &str, commit2: &str) -> Result<()> {
    use crate::mapper::extract_skeleton;

    let changed = crate::git_analysis::git_diff_files(root, commit1, commit2);

    if changed.is_empty() {
        println!("No files changed between {} and {}.", commit1, commit2);
        return Ok(());
    }

    println!("Semantic diff: {} → {}", commit1, commit2);
    println!();

    for (path, status) in &changed {
        let status_label = match status {
            'A' => "added",
            'D' => "deleted",
            _ => "modified",
        };
        println!("{} ({})", path, status_label);

        let fake_path = std::path::Path::new(path);

        let before_sigs: Vec<String> = if *status != 'A' {
            crate::git_analysis::git_show_file(root, commit1, path)
                .map(|content| {
                    let mf = extract_skeleton(fake_path, &content);
                    mf.signatures.into_iter().map(|s| s.raw).collect()
                })
                .unwrap_or_default()
        } else {
            vec![]
        };

        let after_sigs: Vec<String> = if *status != 'D' {
            crate::git_analysis::git_show_file(root, commit2, path)
                .map(|content| {
                    let mf = extract_skeleton(fake_path, &content);
                    mf.signatures.into_iter().map(|s| s.raw).collect()
                })
                .unwrap_or_default()
        } else {
            vec![]
        };

        let before_set: std::collections::HashSet<&str> =
            before_sigs.iter().map(|s| s.as_str()).collect();
        let after_set: std::collections::HashSet<&str> =
            after_sigs.iter().map(|s| s.as_str()).collect();

        let mut any = false;
        for sig in &after_sigs {
            if !before_set.contains(sig.as_str()) {
                println!("  + {}", sig);
                any = true;
            }
        }
        for sig in &before_sigs {
            if !after_set.contains(sig.as_str()) {
                println!("  - {}", sig);
                any = true;
            }
        }
        if !any {
            println!("  (no signature changes)");
        }
        println!();
    }

    Ok(())
}

// =============================================================================
// SHARED HELPER: parallel file scan + persistent cache
// =============================================================================

/// Scan and extract skeleton for every project file, with a parallel rayon scan
/// and a git-HEAD-keyed persistent cache (.codecartographer_cache.json).
fn build_mapped_files_cached(root: &Path) -> anyhow::Result<HashMap<String, MappedFile>> {
    use serde::{Deserialize, Serialize};

    #[derive(Serialize, Deserialize)]
    struct MapCache {
        head: String,
        files: HashMap<String, MappedFile>,
    }

    // Compute git HEAD (empty string if not a git repo)
    let head: String = std::process::Command::new("git")
        .args(["-C", &root.to_string_lossy(), "rev-parse", "HEAD"])
        .output()
        .ok()
        .and_then(|o| if o.status.success() { Some(String::from_utf8_lossy(&o.stdout).trim().to_string()) } else { None })
        .unwrap_or_default();

    let cache_path = root.join(".codecartographer_cache.json");

    // Try cache hit
    if !head.is_empty() {
        if let Ok(raw) = std::fs::read_to_string(&cache_path) {
            if let Ok(cached) = serde_json::from_str::<MapCache>(&raw) {
                if cached.head == head {
                    return Ok(cached.files);
                }
            }
        }
    }

    // Parallel scan
    let scan = scan_files_with_noise_tracking(root).context("file scan failed")?;
    let result: HashMap<String, MappedFile> = scan.files
        .par_iter()
        .filter(|p| !is_ignored_path(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p.strip_prefix(root).unwrap_or(p)
                .to_string_lossy().replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    // Write cache
    if !head.is_empty() {
        if let Ok(json) = serde_json::to_string(&MapCache { head, files: result.clone() }) {
            let _ = std::fs::write(&cache_path, json);
        }
    }

    Ok(result)
}

// =============================================================================
// MCP SERVE MODE - Start MCP server with stdio JSON-RPC transport
// =============================================================================

fn mcp_serve_mode(root: &Path, preset: Option<&str>) -> Result<()> {
    use crate::api::ApiState;
    use crate::mcp::McpServer;
    use std::sync::Arc;

    let mapped_files = build_mapped_files_cached(root)?;

    let state = Arc::new(ApiState::new(root.to_path_buf()));
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    // Pre-populate graph so dependency tools work from first call
    state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;
    // Prime the mtime baseline so the first tool call can already detect changes.
    state.refresh_if_stale();

    let server = McpServer::with_preset(state, preset);
    server.serve();
    Ok(())
}

// =============================================================================
// CHECK MODE — CI gate: non-zero exit on cycles or layer violations
// =============================================================================

fn languages_mode(root: &Path, json_out: bool) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    let result = scan_files_with_noise_tracking(root)?;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let total = mapped_files.len();
    let skipped = result.files.len().saturating_sub(total);

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let graph = state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;
    let mut langs: Vec<(String, usize)> = graph.metadata.languages.into_iter().collect();
    langs.sort_by(|a, b| b.1.cmp(&a.1));

    if json_out {
        let out = serde_json::json!({
            "total_source_files": total,
            "skipped_non_source": skipped,
            "languages": langs.iter().map(|(l, c)| serde_json::json!({"language": l, "files": c})).collect::<Vec<_>>(),
        });
        println!("{}", serde_json::to_string_pretty(&out)?);
        return Ok(());
    }

    println!("Languages ({} source files, {} skipped non-source):", total, skipped);
    println!();
    for (lang, count) in &langs {
        let bar: String = "█".repeat((*count * 30 / total.max(1)).max(1));
        println!("  {:>6}  {:<12}  {}", count, lang, bar);
    }

    Ok(())
}

// =============================================================================
// LAYERS MODE — manage architectural layer definitions
// =============================================================================

fn layers_mode(root: &Path, command: LayerCommands) -> Result<()> {
    match command {
        LayerCommands::Init { output } => layers_init(root, output.as_deref()),
        LayerCommands::Validate { config, json } => layers_validate(root, config.as_deref(), json),
        LayerCommands::Diagram { config, format } => layers_diagram(root, config.as_deref(), &format),
        LayerCommands::Suggest { config } => layers_suggest(root, config.as_deref()),
    }
}

/// Build a graph and a per-directory dependency map (used by init and suggest).
fn build_dir_graph(
    root: &Path,
) -> Result<(
    crate::api::ProjectGraphResponse,
    // dir → set of dirs it imports from
    std::collections::HashMap<String, std::collections::BTreeSet<String>>,
)> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    let result = scan_files_with_noise_tracking(root)?;
    let mapped: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mf = extract_skeleton(p, &content);
            let rel = p.strip_prefix(root).unwrap_or(p)
                .to_string_lossy().replace('\\', "/");
            Some((rel, mf))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    { let mut f = state.mapped_files.lock().unwrap(); *f = mapped; }
    let graph = state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;

    // dir of a module = first path component, or "." for root files
    let dir_of = |module_id: &str| -> String {
        match module_id.split_once('/') {
            Some((d, _)) => d.to_owned(),
            None => ".".to_owned(),
        }
    };

    let mut dir_deps: std::collections::HashMap<String, std::collections::BTreeSet<String>> =
        std::collections::HashMap::new();
    for node in &graph.nodes {
        dir_deps.entry(dir_of(&node.module_id)).or_default();
    }
    for edge in &graph.edges {
        let src_dir = dir_of(&edge.source);
        let tgt_dir = dir_of(&edge.target);
        if src_dir != tgt_dir {
            dir_deps.entry(src_dir).or_default().insert(tgt_dir);
        }
    }

    Ok((graph, dir_deps))
}

/// Load layers.toml from the given path or auto-detect.
fn load_layer_config(root: &Path, explicit: Option<&Path>) -> Result<crate::layers::LayerConfig> {
    use crate::layers::LayerConfig;
    if let Some(p) = explicit {
        return LayerConfig::from_file(p).map_err(|e| anyhow::anyhow!(e));
    }
    for candidate in &["layers.toml", ".codecartographer/layers.toml"] {
        let p = root.join(candidate);
        if p.exists() {
            return LayerConfig::from_file(&p).map_err(|e| anyhow::anyhow!(e));
        }
    }
    anyhow::bail!("No layers.toml found. Run 'codecartographer layers init' to generate one.")
}

// ── init ─────────────────────────────────────────────────────────────────────

fn layers_init(root: &Path, output: Option<&Path>) -> Result<()> {
    let (graph, dir_deps) = build_dir_graph(root)?;

    // Directories ordered by how many other dirs depend on them (most-imported = lowest layer).
    let dirs: Vec<String> = {
        let mut all: Vec<String> = dir_deps.keys().cloned().collect();
        all.sort();
        all
    };

    if dirs.is_empty() {
        anyhow::bail!("No source files found.");
    }

    // Count how many dirs import each dir (in-degree in the dir graph).
    let mut in_degree: std::collections::HashMap<&str, usize> =
        dirs.iter().map(|d| (d.as_str(), 0)).collect();
    for deps in dir_deps.values() {
        for dep in deps {
            *in_degree.entry(dep.as_str()).or_insert(0) += 1;
        }
    }

    // Topological order: dirs with nothing importing them first (presentation),
    // deeply imported dirs last (infrastructure).
    let mut topo: Vec<&str> = dirs.iter().map(|d| d.as_str()).collect();
    topo.sort_by_key(|d| std::cmp::Reverse(*in_degree.get(d).unwrap_or(&0)));

    // Assign names based on rank.
    let n = topo.len();
    let layer_name = |rank: usize| -> &'static str {
        match (rank * 4) / n.max(1) {
            0 => "presentation",
            1 => "domain",
            2 => "service",
            _ => "infrastructure",
        }
    };

    // Group dirs into named layers.
    let mut layer_to_dirs: std::collections::BTreeMap<&str, Vec<&str>> =
        std::collections::BTreeMap::new();
    for (rank, &dir) in topo.iter().enumerate() {
        layer_to_dirs
            .entry(layer_name(rank))
            .or_default()
            .push(dir);
    }

    // Infer allowed flows from actual inter-layer edges.
    let dir_to_layer: std::collections::HashMap<&str, &str> = topo
        .iter()
        .enumerate()
        .map(|(rank, &dir)| (dir, layer_name(rank)))
        .collect();

    let mut allowed: std::collections::BTreeSet<(&str, &str)> =
        std::collections::BTreeSet::new();
    for (src_dir, deps) in &dir_deps {
        let sl = dir_to_layer.get(src_dir.as_str()).copied().unwrap_or("?");
        for dep in deps {
            let tl = dir_to_layer.get(dep.as_str()).copied().unwrap_or("?");
            if sl != tl {
                allowed.insert((sl, tl));
            }
        }
    }

    // Compose the layers.toml.
    let mut out = String::new();
    out.push_str("# Generated by `codecartographer layers init`\n");
    out.push_str("# Edit layer names and folder assignments to match your architecture.\n\n");

    out.push_str("[layers]\n");
    // Emit in a fixed order so the output is stable.
    let layer_order = ["presentation", "domain", "service", "infrastructure"];
    for &layer in &layer_order {
        if let Some(dirs) = layer_to_dirs.get(layer) {
            let quoted: Vec<String> = dirs.iter().map(|d| format!("\"{}\"", d)).collect();
            out.push_str(&format!("{} = [{}]\n", layer, quoted.join(", ")));
        }
    }

    out.push_str("\n[allowed_flows]\n");
    for (from, to) in &allowed {
        out.push_str(&format!("{} -> {}\n", from, to));
    }

    // Summary stats.
    let total_files = graph.metadata.total_files;
    out.push_str(&format!(
        "\n# Stats: {} source files across {} directories\n",
        total_files,
        dirs.len()
    ));

    match output {
        Some(p) => {
            std::fs::write(p, &out)?;
            println!("Wrote proposed layers.toml to {}", p.display());
            println!("Review and edit, then run `codecartographer layers validate`.");
        }
        None => {
            print!("{}", out);
        }
    }

    Ok(())
}

// ── validate ─────────────────────────────────────────────────────────────────

fn layers_validate(root: &Path, config_path: Option<&Path>, json_out: bool) -> Result<()> {
    use crate::api::ApiState;
    use crate::layers::detect_layer_violations;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    let config = load_layer_config(root, config_path)?;

    let result = scan_files_with_noise_tracking(root)?;
    let mapped: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mf = extract_skeleton(p, &content);
            let rel = p.strip_prefix(root).unwrap_or(p)
                .to_string_lossy().replace('\\', "/");
            Some((rel, mf))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    { let mut f = state.mapped_files.lock().unwrap(); *f = mapped; }
    let graph = state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;

    let edge_tuples: Vec<(String, String)> = graph
        .edges
        .iter()
        .map(|e| (e.source.clone(), e.target.clone()))
        .collect();

    let violations = detect_layer_violations(&edge_tuples, &config);

    if json_out {
        let v_json: Vec<_> = violations.iter().map(|v| serde_json::json!({
            "source": v.source_path,
            "target": v.target_path,
            "source_layer": v.source_layer,
            "target_layer": v.target_layer,
            "type": v.violation_type.as_str(),
            "severity": v.severity,
        })).collect();
        println!("{}", serde_json::to_string_pretty(&serde_json::json!({
            "total_files": graph.metadata.total_files,
            "violations": v_json,
        }))?);
        if !violations.is_empty() { std::process::exit(1); }
        return Ok(());
    }

    if violations.is_empty() {
        println!(
            "✅ No layer violations ({} files, {} edges checked).",
            graph.metadata.total_files,
            graph.metadata.total_edges
        );
        return Ok(());
    }

    eprintln!(
        "❌ {} layer violation{} found:\n",
        violations.len(),
        if violations.len() == 1 { "" } else { "s" }
    );
    for v in &violations {
        eprintln!(
            "  [{}] {} ({}) → {} ({})",
            v.violation_type.as_str().to_uppercase(),
            v.source_path, v.source_layer,
            v.target_path, v.target_layer,
        );
    }
    std::process::exit(1);
}

// ── diagram ──────────────────────────────────────────────────────────────────

fn layers_diagram(root: &Path, config_path: Option<&Path>, format: &str) -> Result<()> {
    let config = load_layer_config(root, config_path)?;
    let (_, dir_deps) = build_dir_graph(root)?;

    // Invert config.layers: folder → layer name.
    // config.layers is already stored as folder → [layer_name] by the parser.
    let dir_to_layer: std::collections::HashMap<String, String> = config
        .layers
        .iter()
        .map(|(folder, names)| {
            (folder.clone(), names.first().cloned().unwrap_or_default())
        })
        .collect();

    // Aggregate edges at the layer level.
    let mut layer_edges: std::collections::BTreeMap<(&str, &str), usize> =
        std::collections::BTreeMap::new();
    let unknown = "unlayered".to_string();
    for (src_dir, deps) in &dir_deps {
        let sl = dir_to_layer.get(src_dir).unwrap_or(&unknown);
        for dep in deps {
            let tl = dir_to_layer.get(dep).unwrap_or(&unknown);
            if sl != tl {
                *layer_edges.entry((sl.as_str(), tl.as_str())).or_insert(0) += 1;
            }
        }
    }

    // Collect unique layers.
    let mut layers: std::collections::BTreeSet<&str> = std::collections::BTreeSet::new();
    for (from, to) in layer_edges.keys() {
        layers.insert(from);
        layers.insert(to);
    }

    match format {
        "dot" => {
            println!("digraph layers {{");
            println!("  rankdir=TB;");
            println!("  node [shape=box, style=filled, fillcolor=lightblue];");
            for layer in &layers {
                println!("  \"{}\" [label=\"{}\"];", layer, layer);
            }
            for ((from, to), count) in &layer_edges {
                let is_allowed = config.is_flow_allowed(from, to);
                let color = if is_allowed { "black" } else { "red" };
                println!(
                    "  \"{}\" -> \"{}\" [label=\"{} dirs\", color={}];",
                    from, to, count, color
                );
            }
            println!("}}");
        }
        _ => {
            // Mermaid (default)
            println!("graph TD");
            for layer in &layers {
                let safe = layer.replace('-', "_");
                println!("  {}[{}]", safe, layer);
            }
            for ((from, to), count) in &layer_edges {
                let is_allowed = config.is_flow_allowed(from, to);
                let arrow = if is_allowed { "-->" } else { "-.->|VIOLATION|" };
                let sf = from.replace('-', "_");
                let st = to.replace('-', "_");
                println!("  {} {} {}[\"{} dirs\"]", sf, arrow, st, count);
            }
        }
    }

    Ok(())
}

// ── suggest ──────────────────────────────────────────────────────────────────

fn layers_suggest(root: &Path, config_path: Option<&Path>) -> Result<()> {
    use crate::layers::detect_layer_violations;

    let config = load_layer_config(root, config_path)?;
    let (graph, dir_deps) = build_dir_graph(root)?;

    let edge_tuples: Vec<(String, String)> = graph
        .edges
        .iter()
        .map(|e| (e.source.clone(), e.target.clone()))
        .collect();

    let violations = detect_layer_violations(&edge_tuples, &config);

    // Invert config.layers to get folder → layer.
    let dir_to_layer: std::collections::HashMap<String, String> = config
        .layers
        .iter()
        .map(|(folder, names)| (folder.clone(), names.first().cloned().unwrap_or_default()))
        .collect();

    // 1. Unlayered directories — suggest assigning them.
    let unlayered: Vec<&str> = dir_deps
        .keys()
        .filter(|d| !dir_to_layer.contains_key(d.as_str()) && d.as_str() != ".")
        .map(String::as_str)
        .collect();

    if !unlayered.is_empty() {
        println!("📂 Unassigned directories (not in any layer):");
        for d in &unlayered {
            println!("  {}", d);
        }
        println!(
            "  → Add them to [layers] in layers.toml to enable violation detection.\n"
        );
    }

    // 2. Current violations grouped by type.
    if violations.is_empty() {
        println!("✅ No violations with the current configuration.");
    } else {
        // Group violations by (source_layer, target_layer).
        let mut by_pair: std::collections::BTreeMap<(&str, &str), usize> =
            std::collections::BTreeMap::new();
        for v in &violations {
            *by_pair
                .entry((v.source_layer.as_str(), v.target_layer.as_str()))
                .or_insert(0) += 1;
        }

        println!("⚠️  Active violations ({} total):", violations.len());
        for ((sl, tl), count) in &by_pair {
            println!(
                "  {} → {}  ({} edge{})",
                sl, tl, count, if *count == 1 { "" } else { "s" }
            );
            println!(
                "    → To allow: add `{} -> {}` to [allowed_flows]",
                sl, tl
            );
            println!(
                "    → To fix:   move offending files into a layer where the import is permitted"
            );
        }
        println!();
    }

    // 3. Allowed flows that are never actually used.
    if let Some(ref flows) = config.allowed_flows {
        let dir_to_layer_str: std::collections::HashMap<String, String> = config
            .layers
            .iter()
            .map(|(k, v)| (k.clone(), v.first().cloned().unwrap_or_default()))
            .collect();

        let mut used_flows: std::collections::BTreeSet<(&str, &str)> =
            std::collections::BTreeSet::new();
        for edge in &graph.edges {
            let sl = dir_of_module(&edge.source);
            let tl = dir_of_module(&edge.target);
            let sl_layer = dir_to_layer_str.get(&sl);
            let tl_layer = dir_to_layer_str.get(&tl);
            if let (Some(sl), Some(tl)) = (sl_layer, tl_layer) {
                if sl != tl {
                    used_flows.insert((sl.as_str(), tl.as_str()));
                }
            }
        }

        let unused: Vec<&crate::layers::LayerFlow> = flows
            .iter()
            .filter(|f| !used_flows.contains(&(f.from.as_str(), f.to.as_str())))
            .collect();

        if !unused.is_empty() {
            println!("🧹 Unused allowed_flows (no actual edges traverse them):");
            for flow in unused {
                println!("  {} -> {}  (safe to remove)", flow.from, flow.to);
            }
            println!();
        }
    }

    Ok(())
}

fn dir_of_module(module_id: &str) -> String {
    match module_id.split_once('/') {
        Some((d, _)) => d.to_owned(),
        None => ".".to_owned(),
    }
}

fn check_mode(root: &Path) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    let result = scan_files_with_noise_tracking(root)?;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let graph = state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;

    let cycle_count = graph.metadata.cycle_count.unwrap_or(0);
    let violation_count = graph.metadata.layer_violation_count.unwrap_or(0);

    let mut failed = false;

    if cycle_count > 0 {
        eprintln!("FAIL: {} circular dependenc{}", cycle_count, if cycle_count == 1 { "y" } else { "ies" });
        for cycle in graph.cycles.iter().take(5) {
            eprintln!("  {} ({})", cycle.nodes.join(" -> "), cycle.severity);
        }
        failed = true;
    }

    if violation_count > 0 {
        eprintln!("FAIL: {} layer violation{}", violation_count, if violation_count == 1 { "" } else { "s" });
        for v in graph.layer_violations.iter().take(5) {
            eprintln!("  {} -> {} ({} -> {})", v.source_path, v.target_path, v.source_layer, v.target_layer);
        }
        failed = true;
    }

    if failed {
        std::process::exit(1);
    }

    println!(
        "OK: {} files, {} dependencies, health {:.0}/100",
        graph.metadata.total_files,
        graph.metadata.total_edges,
        graph.metadata.health_score.unwrap_or(0.0)
    );
    Ok(())
}

// =============================================================================
// PATH MODE — shortest import path between two source files (BFS)
// =============================================================================

fn path_mode(root: &Path, from: &str, to: &str, json_out: bool) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    let result = scan_files_with_noise_tracking(root)?;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let graph = state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;

    // Accept either a repo-relative path or a module_id (they're the same in practice).
    // Do a fuzzy match: exact > suffix match.
    let resolve_node = |query: &str| -> Option<String> {
        graph.nodes.iter()
            .find(|n| n.module_id == query || n.path == query)
            .or_else(|| graph.nodes.iter().find(|n| n.path.ends_with(query)))
            .map(|n| n.module_id.clone())
    };

    let start = resolve_node(from)
        .ok_or_else(|| anyhow::anyhow!("No module matching '{}' found", from))?;
    let end = resolve_node(to)
        .ok_or_else(|| anyhow::anyhow!("No module matching '{}' found", to))?;

    if start == end {
        if json_out {
            println!("{}", serde_json::to_string_pretty(&serde_json::json!({
                "from": from, "to": to, "hops": 0, "path": [start]
            }))?);
        } else {
            println!("{} (same file)", start);
        }
        return Ok(());
    }

    // Build adjacency list (directed: import A→B means A depends on B).
    let mut adj: std::collections::HashMap<&str, Vec<&str>> =
        std::collections::HashMap::new();
    for edge in &graph.edges {
        adj.entry(edge.source.as_str())
            .or_default()
            .push(edge.target.as_str());
    }

    // BFS from start → end.
    let mut queue: std::collections::VecDeque<Vec<&str>> = std::collections::VecDeque::new();
    let mut visited: std::collections::HashSet<&str> = std::collections::HashSet::new();
    queue.push_back(vec![start.as_str()]);
    visited.insert(start.as_str());
    let mut found_path: Option<Vec<String>> = None;

    'bfs: while let Some(current_path) = queue.pop_front() {
        let tail = *current_path.last().unwrap();
        for &neighbor in adj.get(tail).into_iter().flatten() {
            if !visited.contains(neighbor) {
                let mut new_path = current_path.clone();
                new_path.push(neighbor);
                if neighbor == end.as_str() {
                    found_path = Some(new_path.iter().map(|&s| s.to_owned()).collect());
                    break 'bfs;
                }
                visited.insert(neighbor);
                queue.push_back(new_path);
            }
        }
    }

    match found_path {
        Some(hops) => {
            if json_out {
                println!("{}", serde_json::to_string_pretty(&serde_json::json!({
                    "from": from,
                    "to": to,
                    "hops": hops.len() - 1,
                    "path": hops,
                }))?);
            } else {
                println!("Shortest path ({} hop{}):", hops.len() - 1, if hops.len() == 2 { "" } else { "s" });
                for (i, step) in hops.iter().enumerate() {
                    if i == 0 {
                        println!("  {}", step);
                    } else {
                        println!("  └─ imports ─▶ {}", step);
                    }
                }
            }
        }
        None => {
            if json_out {
                println!("{}", serde_json::to_string_pretty(&serde_json::json!({
                    "from": from, "to": to, "hops": null, "path": null,
                    "error": "no import path found"
                }))?);
            } else {
                println!("No import path found from '{}' to '{}'.", from, to);
                println!("(The dependency may be indirect via a different direction, or there is no connection.)");
            }
        }
    }

    Ok(())
}

// =============================================================================
// SEARCH MODE — grep-like text/regex search across project files
// =============================================================================

#[allow(clippy::too_many_arguments)]
fn search_mode(
    root: &Path,
    pattern: &str,
    extra_patterns: Vec<String>,
    literal: bool,
    ignore_case: bool,
    invert_match: bool,
    word_regexp: bool,
    only_matching: bool,
    files_with_matches: bool,
    files_without_match: bool,
    count: bool,
    after_context: usize,
    before_context: usize,
    context: usize,
    glob: Option<&str>,
    exclude: Option<&str>,
    path: Option<&str>,
    limit: usize,
    no_ignore: bool,
) -> Result<()> {
    use crate::search::{search_content, SearchOptions};

    let opts = SearchOptions {
        literal,
        case_sensitive: !ignore_case,
        context_lines: context,
        before_context,
        after_context,
        max_results: limit,
        file_glob: glob.map(|s| s.to_string()),
        exclude_glob: exclude.map(|s| s.to_string()),
        extra_patterns,
        invert_match,
        word_regexp,
        only_matching,
        files_with_matches,
        files_without_match,
        count_only: count,
        no_ignore,
        search_path: path.map(|s| s.to_string()),
    };

    let result = search_content(root, pattern, &opts).map_err(|e| anyhow::anyhow!(e))?;

    eprintln!(
        "Search {:?} — {} match(es) across {} file(s){}",
        pattern, result.total_matches, result.files_searched,
        if result.truncated { " [truncated]" } else { "" }
    );

    // -l
    if opts.files_with_matches {
        for f in &result.files_with_matches { println!("{}", f); }
        return Ok(());
    }
    // --files-without-match
    if opts.files_without_match {
        for f in &result.files_without_match { println!("{}", f); }
        return Ok(());
    }
    // -c
    if opts.count_only {
        for fc in &result.file_counts { println!("{}:{}", fc.path, fc.count); }
        return Ok(());
    }

    if result.matches.is_empty() { return Ok(()); }

    eprintln!();
    let mut cur_file = String::new();
    for m in &result.matches {
        if m.path != cur_file {
            if !cur_file.is_empty() { println!(); }
            println!("{}:", m.path);
            cur_file = m.path.clone();
        }
        for ctx in &m.before_context {
            println!("  {:>5}-{}", ctx.line_number, ctx.line);
        }
        if opts.only_matching {
            for t in &m.matched_texts { println!("  {:>5}:{}", m.line_number, t); }
        } else {
            println!("  {:>5}:{}", m.line_number, m.line);
        }
        for ctx in &m.after_context {
            println!("  {:>5}-{}", ctx.line_number, ctx.line);
        }
    }

    Ok(())
}

// =============================================================================
// FIND MODE — find files by glob + optional mtime/size/depth filters
// =============================================================================

fn find_mode(
    root: &Path,
    pattern: &str,
    modified_since: Option<&str>,
    newer: Option<&str>,
    min_size: Option<u64>,
    max_size: Option<u64>,
    max_depth: Option<usize>,
    limit: usize,
    no_ignore: bool,
) -> Result<()> {
    use crate::search::{find_files, FindOptions};

    let modified_since_secs = modified_since.map(parse_duration_secs).transpose()?;

    let opts = FindOptions {
        modified_since_secs,
        newer_than: newer.map(|s| s.to_string()),
        min_size_bytes: min_size,
        max_size_bytes: max_size,
        max_depth,
        no_ignore,
    };

    let result = find_files(root, pattern, limit, &opts).map_err(|e| anyhow::anyhow!(e))?;

    eprintln!(
        "Find {:?} — {} file(s){}",
        pattern, result.total_matches,
        if result.truncated { " [truncated]" } else { "" }
    );

    if result.files.is_empty() { return Ok(()); }

    eprintln!();
    for f in &result.files {
        let lang = f.language.as_deref().unwrap_or("");
        let size = fmt_size(f.size_bytes);
        let mtime = f.modified.as_deref().unwrap_or("");
        if lang.is_empty() {
            println!("  {}  ({})  {}", f.path, size, mtime);
        } else {
            println!("  {}  [{}, {}]  {}", f.path, lang, size, mtime);
        }
    }

    Ok(())
}

fn parse_duration_secs(s: &str) -> Result<u64> {
    let (num, mul) = if let Some(n) = s.strip_suffix('d') {
        (n, 86400u64)
    } else if let Some(n) = s.strip_suffix('h') {
        (n, 3600)
    } else if let Some(n) = s.strip_suffix('m') {
        (n, 60)
    } else if let Some(n) = s.strip_suffix('s') {
        (n, 1)
    } else {
        (s, 1)
    };
    let n: u64 = num.parse().context("invalid duration (use: 24h, 7d, 30m, 3600s)")?;
    Ok(n * mul)
}

fn fmt_size(bytes: u64) -> String {
    if bytes >= 1024 * 1024 {
        format!("{:.1}M", bytes as f64 / (1024.0 * 1024.0))
    } else if bytes >= 1024 {
        format!("{:.1}K", bytes as f64 / 1024.0)
    } else {
        format!("{}B", bytes)
    }
}

// =============================================================================
// CONTEXT MODE — Ranked skeleton pruned to token budget (personalized PageRank)
// =============================================================================

fn context_mode(
    root: &Path,
    focus: &[String],
    budget: usize,
    query: Option<&str>,
    for_task: Option<&str>,
) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, scan_files_with_noise_tracking};

    let result = scan_files_with_noise_tracking(root)?;
    let total_file_count = result.files.iter().filter(|p| !is_ignored_path(p)).count();
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .iter()
        .filter(|p| !is_ignored_path(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;

    let mut ranked = state.ranked_skeleton(focus, budget).map_err(|e| anyhow::anyhow!(e))?;

    // --for-task: boost files whose signatures match the task description via TF-IDF.
    if let Some(task) = for_task {
        let task_terms: std::collections::HashSet<String> = task
            .split(|c: char| !c.is_alphanumeric() && c != '_')
            .filter(|t| t.len() >= 3)
            .map(|t| t.to_lowercase())
            .collect();
        if !task_terms.is_empty() {
            // Score each file by term overlap with its path + signatures.
            let mut task_scores: Vec<f64> = ranked
                .iter()
                .map(|f| {
                    let text = format!("{} {}", f.path, f.signatures.join(" ")).to_lowercase();
                    let hits = task_terms
                        .iter()
                        .filter(|t| text.contains(t.as_str()))
                        .count();
                    hits as f64 / task_terms.len() as f64
                })
                .collect();
            // Normalise task scores to [0, 1].
            let max_ts = task_scores.iter().cloned().fold(0.0_f64, f64::max);
            if max_ts > 0.0 {
                for s in &mut task_scores {
                    *s /= max_ts;
                }
            }
            // Blend: 60% PageRank + 40% task overlap.
            let max_rank = ranked.iter().map(|f| f.rank).fold(0.0_f64, f64::max);
            for (f, ts) in ranked.iter_mut().zip(task_scores.iter()) {
                let norm_rank = if max_rank > 0.0 { f.rank / max_rank } else { 0.0 };
                f.rank = 0.6 * norm_rank + 0.4 * ts;
            }
            ranked.sort_by(|a, b| b.rank.partial_cmp(&a.rank).unwrap_or(std::cmp::Ordering::Equal));
        }
    }

    let total_tokens: usize = ranked.iter().map(|f| f.estimated_tokens).sum();
    let dropped = total_file_count.saturating_sub(ranked.len());

    eprintln!(
        "Ranked context: {} files, ~{} tokens (budget: {})",
        ranked.len(),
        total_tokens,
        if budget == 0 { "unlimited".to_string() } else { budget.to_string() }
    );
    if budget > 0 && dropped > 0 {
        eprintln!("Dropped: {} files (lowest PageRank, exceeded budget)", dropped);
    }
    if !focus.is_empty() {
        eprintln!("Focus: {}", focus.join(", "));
    }
    if let Some(task) = for_task {
        eprintln!("Task: {:?}", task);
    }
    if let Some(q) = query {
        eprintln!("Query: {:?}", q);
    }
    eprintln!();

    // Print ranked skeleton
    println!("## Ranked Architecture Skeleton\n");
    for f in &ranked {
        println!("// {} (rank: {:.4}, {} tokens)", f.path, f.rank, f.estimated_tokens);
        for sig in &f.signatures {
            println!("  {}", sig);
        }
        println!();
    }

    // If --query was given, bundle matching lines below the skeleton
    if let Some(q) = query {
        use crate::search::{search_content, SearchOptions};
        let opts = SearchOptions {
            case_sensitive: false, // case-insensitive for context queries
            context_lines: 2,
            max_results: 50,
            ..Default::default()
        };
        match search_content(root, q, &opts) {
            Ok(sr) if !sr.matches.is_empty() => {
                println!("## Search Results for {:?}\n", q);
                let mut cur_file = String::new();
                for m in &sr.matches {
                    if m.path != cur_file {
                        if !cur_file.is_empty() {
                            println!();
                        }
                        println!("// {}", m.path);
                        cur_file = m.path.clone();
                    }
                    for ctx in &m.before_context {
                        println!("  {:>4}  {}", ctx.line_number, ctx.line);
                    }
                    println!("  {:>4}> {}", m.line_number, m.line);
                    for ctx in &m.after_context {
                        println!("  {:>4}  {}", ctx.line_number, ctx.line);
                    }
                }
                println!();
                eprintln!(
                    "Search: {} match(es) in {} file(s){}",
                    sr.total_matches,
                    sr.files_searched,
                    if sr.truncated { " [truncated]" } else { "" }
                );
            }
            Ok(_) => {
                eprintln!("Search: no matches for {:?}", q);
            }
            Err(e) => {
                eprintln!("Search error: {}", e);
            }
        }
    }

    Ok(())
}

// =============================================================================
// SYMBOLS MODE — Symbol-level analysis (unreferenced public exports)
// =============================================================================

fn symbols_mode(root: &Path, unreferenced_only: bool) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, scan_files_with_noise_tracking};

    let result = scan_files_with_noise_tracking(root)?;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .iter()
        .filter(|p| !is_ignored_path(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p
                .strip_prefix(root)
                .unwrap_or(p)
                .to_string_lossy()
                .replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }

    let graph = state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;

    let unreferenced_count = graph.metadata.unreferenced_exports_count.unwrap_or(0);

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         Symbol Analysis                                    ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();
    println!("Total files:         {}", graph.metadata.total_files);
    println!("Unreferenced exports: {} (heuristic — verify before removing)", unreferenced_count);
    println!();

    let nodes_with_unref: Vec<_> = graph
        .nodes
        .iter()
        .filter(|n| {
            n.unreferenced_exports
                .as_ref()
                .map(|v| !v.is_empty())
                .unwrap_or(false)
        })
        .collect();

    if unreferenced_only || true {
        if nodes_with_unref.is_empty() {
            println!("No unreferenced public exports found.");
        } else {
            println!("Unreferenced public exports by file:");
            for node in &nodes_with_unref {
                if let Some(exports) = &node.unreferenced_exports {
                    println!("  {}:", node.path);
                    for sym in exports {
                        println!("    - {}", sym);
                    }
                }
            }
        }
    }

    println!();
    println!("Note: Uses import-token heuristic. Does not account for dynamic dispatch,");
    println!("reflection, or external consumers of library crates.");
    println!("      `pub extern \"C\"` / FFI exports are excluded automatically.");

    Ok(())
}

// =============================================================================
// REPLACE MODE — sed-like find-and-replace
// =============================================================================

#[allow(clippy::too_many_arguments)]
fn replace_mode(
    root: &Path,
    pattern: &str,
    replacement: &str,
    literal: bool,
    ignore_case: bool,
    word_regexp: bool,
    dry_run: bool,
    backup: bool,
    context: usize,
    glob: Option<&str>,
    exclude: Option<&str>,
    search_path: Option<&str>,
    max_per_file: usize,
    no_ignore: bool,
) -> Result<()> {
    use crate::search::{replace_content, ReplaceOptions};

    let opts = ReplaceOptions {
        literal,
        case_sensitive: !ignore_case,
        word_regexp,
        dry_run,
        backup,
        context_lines: context,
        file_glob: glob.map(str::to_string),
        exclude_glob: exclude.map(str::to_string),
        search_path: search_path.map(str::to_string),
        no_ignore,
        max_per_file,
    };

    let result = replace_content(root, pattern, replacement, &opts)
        .map_err(|e| anyhow::anyhow!(e))?;

    if result.changes.is_empty() {
        println!("No matches found.");
        return Ok(());
    }

    if dry_run {
        println!("DRY RUN — no files will be written\n");
    }

    for change in &result.changes {
        let action = if dry_run { "would change" } else { "changed" };
        println!(
            "{} — {} replacement{}",
            change.path,
            change.replacements,
            if change.replacements == 1 { "" } else { "s" }
        );
        println!("({} {})", action, change.path);

        for line in &change.diff {
            match line.kind.as_str() {
                "removed"   => println!("\x1b[31m- {:>4} {}\x1b[0m", line.line_number, line.content),
                "added"     => println!("\x1b[32m+ {:>4} {}\x1b[0m", line.line_number, line.content),
                "context"   => println!("  {:>4} {}", line.line_number, line.content),
                "separator" => println!("  ..."),
                _           => {}
            }
        }
        println!();
    }

    println!(
        "Summary: {} file{} {} — {} replacement{} total{}",
        result.files_changed,
        if result.files_changed == 1 { "" } else { "s" },
        if dry_run { "would be changed" } else { "changed" },
        result.total_replacements,
        if result.total_replacements == 1 { "" } else { "s" },
        if backup && !dry_run { " (.bak backups written)" } else { "" },
    );

    Ok(())
}

// =============================================================================
// EXTRACT MODE — awk-like capture group extraction
// =============================================================================

#[allow(clippy::too_many_arguments)]
fn extract_mode(
    root: &Path,
    pattern: &str,
    groups: &[usize],
    sep: &str,
    format: &str,
    count: bool,
    dedup: bool,
    sort: bool,
    ignore_case: bool,
    glob: Option<&str>,
    exclude: Option<&str>,
    search_path: Option<&str>,
    limit: usize,
    no_ignore: bool,
) -> Result<()> {
    use crate::search::{extract_content, ExtractOptions};

    let opts = ExtractOptions {
        groups: groups.to_vec(),
        separator: sep.to_string(),
        format: format.to_string(),
        count,
        dedup,
        sort,
        case_sensitive: !ignore_case,
        file_glob: glob.map(str::to_string),
        exclude_glob: exclude.map(str::to_string),
        search_path: search_path.map(str::to_string),
        no_ignore,
        limit,
    };

    let result = extract_content(root, pattern, &opts)
        .map_err(|e| anyhow::anyhow!(e))?;

    if result.total == 0 {
        println!("No matches found.");
        return Ok(());
    }

    match format {
        "json" => {
            println!("{}", serde_json::to_string_pretty(&result).unwrap_or_default());
        }
        "csv" | "tsv" => {
            let delim = if format == "csv" { "," } else { "\t" };
            if count {
                for entry in &result.counts {
                    // escape commas for CSV
                    let val = if format == "csv" {
                        format!("\"{}\"", entry.value.replace('"', "\"\""))
                    } else {
                        entry.value.clone()
                    };
                    println!("{}{}{}", val, delim, entry.count);
                }
            } else {
                for m in &result.matches {
                    let row: Vec<String> = if format == "csv" {
                        m.groups.iter().map(|g| format!("\"{}\"", g.replace('"', "\"\""))).collect()
                    } else {
                        m.groups.clone()
                    };
                    println!("{}", row.join(delim));
                }
            }
        }
        _ => {
            // text (default)
            if count {
                for entry in &result.counts {
                    println!("{:>6}  {}", entry.count, entry.value);
                }
            } else {
                for m in &result.matches {
                    println!("{}", m.groups.join(sep));
                }
            }
        }
    }

    if format != "json" {
        eprintln!(
            "\n{} match{} from {} file{}{}",
            result.total,
            if result.total == 1 { "" } else { "es" },
            result.files_searched,
            if result.files_searched == 1 { "" } else { "s" },
            if result.truncated { " (truncated)" } else { "" },
        );
    }

    Ok(())
}

fn query_mode(
    root: &Path,
    query: &str,
    budget: usize,
    model: &str,
    format: &str,
    max_seeds: usize,
) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, scan_files_with_noise_tracking};
    use crate::search::{bm25_search, BM25Options};
    use token_metrics::{HealthOpts, ModelFamily};

    // Step 1: BM25 + regex search to find focus files
    let bm25_opts = BM25Options {
        max_results: max_seeds,
        ..Default::default()
    };
    let bm25_hits = bm25_search(root, query, &bm25_opts).unwrap_or_default();

    let search_opts = crate::search::SearchOptions {
        case_sensitive: false,
        max_results: max_seeds,
        ..Default::default()
    };
    let regex_hits = crate::search::search_content(root, query, &search_opts)
        .unwrap_or_else(|_| crate::search::SearchResult {
            matches: vec![],
            total_matches: 0,
            files_searched: 0,
            truncated: false,
            files_with_matches: vec![],
            files_without_match: vec![],
            file_counts: vec![],
        });

    // Merge: BM25 first (ranked), then any additional regex-only hits
    let mut seen = std::collections::HashSet::new();
    let mut focus_files: Vec<String> = Vec::new();
    for m in &bm25_hits.matches {
        if seen.insert(m.path.clone()) {
            focus_files.push(m.path.clone());
        }
    }
    for m in &regex_hits.matches {
        if seen.insert(m.path.clone()) {
            focus_files.push(m.path.clone());
        }
        if focus_files.len() >= max_seeds { break; }
    }

    // Code-question retrieval: bias focus seeds toward actual source files.
    // Raw-content BM25 otherwise latches onto data/doc files (e.g. Godot's
    // doc/classes/*.xml) that mention the query terms but carry no code — they
    // seed the skeleton with 0-signature noise and bury the real implementation.
    // Fall back to the unfiltered list if nothing source-like matched.
    let code_seeds: Vec<String> = focus_files
        .iter()
        .filter(|p| crate::scanner::is_source_file(std::path::Path::new(p.as_str())))
        .cloned()
        .collect();
    if !code_seeds.is_empty() {
        focus_files = code_seeds;
    }

    eprintln!("Query: {:?}", query);
    eprintln!("Focus seeds: {} file(s) ({} BM25, {} regex)", focus_files.len(), bm25_hits.matches.len(), regex_hits.total_matches);

    // Step 2: build mapped files + ranked skeleton
    let scan = scan_files_with_noise_tracking(root)?;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = scan.files.iter()
        .filter(|p| !is_ignored_path(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p.strip_prefix(root).unwrap_or(p).to_string_lossy().replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    { let mut files = state.mapped_files.lock().unwrap(); *files = mapped_files; }
    state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;

    let ranked = state.ranked_skeleton(&focus_files, budget).map_err(|e| anyhow::anyhow!(e))?;
    let total_tokens: usize = ranked.iter().map(|f| f.estimated_tokens).sum();
    let sig_count: usize = ranked.iter().map(|f| f.signatures.len()).sum();

    // Step 3: build context text
    let mut context_text = format!("## Ranked Context for: {}\n\n", query);
    for f in &ranked {
        context_text.push_str(&format!("// {} (rank: {:.4}, {} tokens)\n", f.path, f.rank, f.estimated_tokens));
        for sig in &f.signatures {
            context_text.push_str(&format!("  {}\n", sig));
        }
        context_text.push('\n');
    }

    // Step 4: health score
    let model_family: ModelFamily = model.parse().unwrap_or_default();
    let health_opts = HealthOpts {
        model: model_family,
        window_size: 0,
        key_positions: token_metrics::key_positions_from_order(
            &ranked.iter().map(|f| f.path.clone()).collect::<Vec<_>>(),
            &focus_files,
        ),
        signature_count: sig_count,
        signature_tokens: (total_tokens as f64 * 0.85) as usize,
    };
    let health = token_metrics::analyze(&context_text, &health_opts);

    if format == "json" {
        let out = serde_json::json!({
            "query": query,
            "context": context_text,
            "filesUsed": ranked.iter().map(|f| &f.path).collect::<Vec<_>>(),
            "focusFiles": focus_files,
            "totalTokens": total_tokens,
            "health": health,
        });
        println!("{}", serde_json::to_string_pretty(&out)?);
        return Ok(());
    }

    // Text output
    println!("{}", context_text);

    let score_bar = {
        let filled = (health.score / 5.0).round() as usize;
        let empty = 20usize.saturating_sub(filled);
        format!("[{}{}]", "█".repeat(filled), "░".repeat(empty))
    };
    eprintln!();
    eprintln!("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
    eprintln!("  {} files  ~{} tokens  health: {:.0}/100 {} grade {}",
        ranked.len(), total_tokens, health.score, score_bar, health.grade);
    for w in &health.warnings {
        eprintln!("  ⚠  {}", w);
    }
    eprintln!("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");

    Ok(())
}

fn context_health_mode(
    file: Option<&std::path::Path>,
    model: &str,
    window: usize,
    format: &str,
    fail_if_over: usize,
) -> Result<()> {
    use token_metrics::{HealthOpts, ModelFamily};

    let content = if let Some(path) = file {
        fs::read_to_string(path).with_context(|| format!("Reading {}", path.display()))?
    } else if std::io::IsTerminal::is_terminal(&std::io::stdin()) {
        // No file arg and stdin is a terminal — fall back to codecartographer_map.xml in cwd.
        let map_path = std::path::Path::new("codecartographer_map.xml");
        if map_path.exists() {
            eprintln!("(reading codecartographer_map.xml)");
            fs::read_to_string(map_path).context("Reading codecartographer_map.xml")?
        } else {
            anyhow::bail!(
                "No input: pass a file (`codecartographer context-health FILE`) or pipe content, \
                 or run `codecartographer map` first to generate codecartographer_map.xml"
            );
        }
    } else {
        use io::Read;
        let mut buf = String::new();
        io::stdin().read_to_string(&mut buf)?;
        buf
    };

    let model_family: ModelFamily = model.parse().unwrap_or_default();
    let opts = HealthOpts {
        model: model_family,
        window_size: window,
        // Without positional info, position_health uses its neutral default (0.5).
        key_positions: Vec::new(),
        // Token-level signal info not available from raw stdin input; leave at 0
        // so signal_density/entity_density reflect a pessimistic baseline.
        signature_count: 0,
        signature_tokens: 0,
    };

    let report = token_metrics::analyze(&content, &opts);

    if format == "json" {
        println!("{}", serde_json::to_string_pretty(&report)?);
        return Ok(());
    }

    // Text output
    let score_bar = {
        let filled = (report.score / 5.0).round() as usize;
        let empty  = 20usize.saturating_sub(filled);
        format!("[{}{}]", "█".repeat(filled), "░".repeat(empty))
    };

    println!("\nContext Health Report");
    println!("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
    println!(
        "  Score: {:.1}/100  Grade: {}  {}",
        report.score, report.grade, score_bar
    );
    println!(
        "  Tokens: {}  /  window: {} ({:.1}% utilisation)",
        format_token_count(report.token_count),
        format_token_count(report.window_size),
        report.utilization_pct,
    );
    println!("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
    let m = &report.metrics;
    println!("  Metrics");
    println!("    Signal density        {:.1}%  (symbol tokens / total)", m.signal_density * 100.0);
    println!("    Compression density   {:.1}%  (entropy proxy, higher = denser)", m.compression_density * 100.0);
    println!("    Position health       {:.1}%  (key-module U-bias score)", m.position_health * 100.0);
    println!("    Entity density        {:.1}%  (symbols per 1K tokens)", m.entity_density * 100.0);
    println!("    Utilisation headroom  {:.1}%  (window buffer score)", m.utilization_headroom * 100.0);
    println!("    Dedup ratio           {:.1}%  (unique-line fraction)", m.dedup_ratio * 100.0);

    if !report.warnings.is_empty() {
        println!("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
        println!("  Warnings");
        for w in &report.warnings {
            println!("  ⚠  {}", w);
        }
    }

    if !report.recommendations.is_empty() {
        println!("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");
        println!("  Recommendations");
        for r in &report.recommendations {
            println!("  →  {}", r);
        }
    }

    println!("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━");

    if fail_if_over > 0 && report.token_count > fail_if_over {
        eprintln!(
            "context-health: FAIL — {} tokens exceeds budget of {} (--fail-if-over)",
            report.token_count, fail_if_over
        );
        std::process::exit(1);
    }

    Ok(())
}

// =============================================================================
// SNAPSHOT MODE — save / diff / list architecture snapshots
// =============================================================================

fn snapshot_mode(cwd: &Path, command: SnapshotCommands) -> Result<()> {
    match command {
        SnapshotCommands::Save { tag, path } => {
            let root = resolve_path(cwd, path)?;
            snapshot_save(&root, &tag)
        }
        SnapshotCommands::Diff { tag1, tag2, json } => {
            snapshot_diff(cwd, &tag1, &tag2, json)
        }
        SnapshotCommands::List { path } => {
            let root = resolve_path(cwd, path)?;
            snapshot_list(&root)
        }
    }
}

fn snapshot_dir(root: &Path) -> PathBuf {
    root.join(".codecartographer").join("snapshots")
}

fn snapshot_path(root: &Path, tag: &str) -> PathBuf {
    snapshot_dir(root).join(format!("{}.json", tag.replace(['/', '\\', ' '], "_")))
}

fn snapshot_save(root: &Path, tag: &str) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, is_source_file, scan_files_with_noise_tracking};

    let result = scan_files_with_noise_tracking(root)?;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = result
        .files
        .par_iter()
        .filter(|p| !is_ignored_path(p) && is_source_file(p))
        .filter_map(|p| {
            let content = std::fs::read_to_string(p).ok()?;
            let mapped = extract_skeleton(p, &content);
            let rel = p.strip_prefix(root).unwrap_or(p).to_string_lossy().replace('\\', "/");
            Some((rel, mapped))
        })
        .collect();

    let state = ApiState::new(root.to_path_buf());
    {
        let mut files = state.mapped_files.lock().unwrap();
        *files = mapped_files;
    }
    let graph = state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;

    let snap = serde_json::json!({
        "tag": tag,
        "saved_at": chrono::Local::now().to_rfc3339(),
        "health_score": graph.metadata.health_score.unwrap_or(0.0),
        "total_files": graph.metadata.total_files,
        "total_edges": graph.metadata.total_edges,
        "bridge_count": graph.metadata.bridge_count.unwrap_or(0),
        "cycle_count": graph.metadata.cycle_count.unwrap_or(0),
        "god_module_count": graph.metadata.god_module_count.unwrap_or(0),
        "layer_violation_count": graph.metadata.layer_violation_count.unwrap_or(0),
    });

    let dir = snapshot_dir(root);
    fs::create_dir_all(&dir)
        .with_context(|| format!("Creating snapshot dir {}", dir.display()))?;
    let path = snapshot_path(root, tag);
    fs::write(&path, serde_json::to_string_pretty(&snap)?)
        .with_context(|| format!("Writing snapshot {}", path.display()))?;

    println!("Snapshot saved: {} → {}", tag, path.display());
    println!("  Health: {:.1}/100  Files: {}  Edges: {}  Cycles: {}  Bridges: {}",
        graph.metadata.health_score.unwrap_or(0.0),
        graph.metadata.total_files,
        graph.metadata.total_edges,
        graph.metadata.cycle_count.unwrap_or(0),
        graph.metadata.bridge_count.unwrap_or(0),
    );
    Ok(())
}

fn snapshot_diff(cwd: &Path, tag1: &str, tag2: &str, json_out: bool) -> Result<()> {
    // Search for snapshot files relative to cwd
    let path1 = snapshot_path(cwd, tag1);
    let path2 = snapshot_path(cwd, tag2);
    let s1: serde_json::Value = serde_json::from_str(&fs::read_to_string(&path1)
        .with_context(|| format!("Reading snapshot '{}' from {}", tag1, path1.display()))?)
        .context("Parsing snapshot")?;
    let s2: serde_json::Value = serde_json::from_str(&fs::read_to_string(&path2)
        .with_context(|| format!("Reading snapshot '{}' from {}", tag2, path2.display()))?)
        .context("Parsing snapshot")?;

    macro_rules! f64v {
        ($v:expr, $k:expr) => { $v[$k].as_f64().unwrap_or(0.0) };
    }
    macro_rules! u64v {
        ($v:expr, $k:expr) => { $v[$k].as_u64().unwrap_or(0) };
    }

    let score1 = f64v!(s1, "health_score");
    let score2 = f64v!(s2, "health_score");
    let files1 = u64v!(s1, "total_files");
    let files2 = u64v!(s2, "total_files");
    let edges1 = u64v!(s1, "total_edges");
    let edges2 = u64v!(s2, "total_edges");
    let bridges1 = u64v!(s1, "bridge_count");
    let bridges2 = u64v!(s2, "bridge_count");
    let cycles1 = u64v!(s1, "cycle_count");
    let cycles2 = u64v!(s2, "cycle_count");
    let god1 = u64v!(s1, "god_module_count");
    let god2 = u64v!(s2, "god_module_count");
    let violations1 = u64v!(s1, "layer_violation_count");
    let violations2 = u64v!(s2, "layer_violation_count");

    if json_out {
        println!("{}", serde_json::to_string_pretty(&serde_json::json!({
            "from": tag1,
            "to": tag2,
            "delta": {
                "health_score": score2 - score1,
                "total_files": files2 as i64 - files1 as i64,
                "total_edges": edges2 as i64 - edges1 as i64,
                "bridge_count": bridges2 as i64 - bridges1 as i64,
                "cycle_count": cycles2 as i64 - cycles1 as i64,
                "god_module_count": god2 as i64 - god1 as i64,
                "layer_violation_count": violations2 as i64 - violations1 as i64,
            }
        }))?);
        return Ok(());
    }

    fn fmt_delta_f(a: f64, b: f64) -> String {
        let d = b - a;
        if d > 0.05 { format!("+{:.1}", d) }
        else if d < -0.05 { format!("{:.1}", d) }
        else { "±0.0".into() }
    }
    fn fmt_delta_u(a: u64, b: u64) -> String {
        match b.cmp(&a) {
            std::cmp::Ordering::Greater => format!("+{}", b - a),
            std::cmp::Ordering::Less    => format!("-{}", a - b),
            std::cmp::Ordering::Equal   => "0".into(),
        }
    }

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         Snapshot Diff                                      ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();
    println!("  From: {}  →  To: {}", tag1, tag2);
    println!();
    println!("  {:25} {:>10}  {:>10}  {:>10}", "Metric", tag1, tag2, "Delta");
    println!("  {}", "─".repeat(60));
    println!("  {:25} {:>10.1}  {:>10.1}  {:>10}", "Health Score",    score1,    score2,    fmt_delta_f(score1, score2));
    println!("  {:25} {:>10}  {:>10}  {:>10}", "Files",            files1,    files2,    fmt_delta_u(files1, files2));
    println!("  {:25} {:>10}  {:>10}  {:>10}", "Edges",            edges1,    edges2,    fmt_delta_u(edges1, edges2));
    println!("  {:25} {:>10}  {:>10}  {:>10}", "Bridges",          bridges1,  bridges2,  fmt_delta_u(bridges1, bridges2));
    println!("  {:25} {:>10}  {:>10}  {:>10}", "Cycles",           cycles1,   cycles2,   fmt_delta_u(cycles1, cycles2));
    println!("  {:25} {:>10}  {:>10}  {:>10}", "God Modules",      god1,      god2,      fmt_delta_u(god1, god2));
    println!("  {:25} {:>10}  {:>10}  {:>10}", "Layer Violations", violations1, violations2, fmt_delta_u(violations1, violations2));
    println!();
    Ok(())
}

fn snapshot_list(root: &Path) -> Result<()> {
    let dir = snapshot_dir(root);
    if !dir.exists() {
        println!("No snapshots found ({})", dir.display());
        return Ok(());
    }

    let mut entries: Vec<_> = fs::read_dir(&dir)?
        .filter_map(|e| e.ok())
        .filter(|e| e.path().extension().map(|x| x == "json").unwrap_or(false))
        .collect();
    entries.sort_by_key(|e| e.file_name());

    if entries.is_empty() {
        println!("No snapshots found in {}", dir.display());
        return Ok(());
    }

    println!("Snapshots in {}:", dir.display());
    println!();
    for entry in entries {
        let path = entry.path();
        let content = fs::read_to_string(&path).unwrap_or_default();
        if let Ok(v) = serde_json::from_str::<serde_json::Value>(&content) {
            let tag = v["tag"].as_str().unwrap_or("?");
            let saved = v["saved_at"].as_str().unwrap_or("?");
            let score = v["health_score"].as_f64().unwrap_or(0.0);
            let files = v["total_files"].as_u64().unwrap_or(0);
            println!("  {:30} saved: {}  health: {:.1}/100  files: {}", tag, saved, score, files);
        } else {
            println!("  {} (unreadable)", path.display());
        }
    }
    println!();
    Ok(())
}

// =============================================================================
// UPDATE MODE — re-run install.sh to upgrade
// =============================================================================

fn answer_mode(
    root: &Path,
    question: &str,
    max_items: usize,
    budget: usize,
    show_top_body: bool,
    then_item: Option<usize>,
) -> Result<()> {
    use answer::{build_answer, render_answer, AnswerOptions};

    let scan_result = scan_files_with_noise_tracking(root)?;
    let mut mapped: Vec<MappedFile> = Vec::new();
    for path in &scan_result.files {
        if !is_source_file(path) {
            continue;
        }
        if let Some(content) = read_text_file(path) {
            let rel = path.strip_prefix(root).unwrap_or(path);
            let mf = extract_skeleton(rel, &content);
            if !mf.imports.is_empty() || !mf.signatures.is_empty() {
                mapped.push(mf);
            }
        }
    }

    let opts = AnswerOptions {
        budget,
        max_items,
        show_top_body,
    };

    let result = build_answer(root, &mapped, question, &opts);

    if result.items.is_empty() {
        eprintln!("answer: no relevant symbols found for query \"{question}\"");
        eprintln!("  ({} files searched)", result.files_searched);
        return Ok(());
    }

    print!("{}", render_answer(&result));
    eprintln!(
        "\n[answer] {} tokens · {} item(s) · {} files searched{}",
        result.tokens_used,
        result.items.len(),
        result.files_searched,
        if result.budget_hit { " [budget hit]" } else { "" },
    );

    // --then N: drill into evidence item N via reach, appended below.
    if let Some(n) = then_item {
        if n == 0 || n > result.items.len() {
            eprintln!(
                "answer --then {n}: no item #{n} (chain has {} item{})",
                result.items.len(),
                if result.items.len() == 1 { "" } else { "s" }
            );
            return Ok(());
        }
        use reach::{build_reach, render_reach, ReachOptions};
        let item = &result.items[n - 1];
        eprintln!("\n── expanding item #{n}: {}", item.name);
        let reach_opts = ReachOptions {
            file_filter: Some(item.file.clone()),
            ..Default::default()
        };
        match build_reach(root, &mapped, &item.name, &reach_opts) {
            Ok(rr) => {
                print!("{}", render_reach(&rr));
                eprintln!(
                    "[reach] {} tokens · {} caller(s) · {} callee(s)",
                    rr.tokens_used, rr.callers.len(), rr.callees.len()
                );
            }
            Err(e) => eprintln!("reach: {e}"),
        }
    }

    Ok(())
}

fn reach_mode(
    root: &Path,
    symbols: &[&str],
    file_filter: Option<&str>,
    depth: usize,
    budget: usize,
    include_tests: bool,
    show_body: bool,
    format: &str,
) -> Result<()> {
    use reach::{build_reach, render_reach, render_multi_reach, ReachOptions};

    // Scan project files to build the mapped skeleton.
    let scan_result = scan_files_with_noise_tracking(root)?;
    let mut mapped: Vec<MappedFile> = Vec::new();
    for path in &scan_result.files {
        if !is_source_file(path) {
            continue;
        }
        if let Some(content) = read_text_file(path) {
            let rel = path.strip_prefix(root).unwrap_or(path);
            let mf = extract_skeleton(rel, &content);
            if !mf.imports.is_empty() || !mf.signatures.is_empty() {
                mapped.push(mf);
            }
        }
    }

    // Single-symbol: existing behaviour unchanged.
    if symbols.len() == 1 {
        let opts = ReachOptions {
            depth,
            budget,
            file_filter: file_filter.map(|s| s.to_string()),
            include_tests,
            show_body,
            ..Default::default()
        };
        match build_reach(root, &mapped, symbols[0], &opts) {
            Ok(result) => {
                match format {
                    "json" => {
                        let json = serde_json::json!({
                            "root": {
                                "name": result.root.name,
                                "file": result.root.file,
                                "line": result.root.line,
                                "sig": result.root.sig,
                                "kind": format!("{:?}", result.root.kind),
                            },
                            "callers": result.callers.iter().map(|c| serde_json::json!({
                                "file": c.file,
                                "line": c.line,
                                "snippet": c.snippet,
                                "tag": c.tag.as_ref().map(|t| t.label()),
                                "is_test": c.is_test,
                            })).collect::<Vec<_>>(),
                            "test_callers_collapsed": result.test_callers_collapsed,
                            "callees": result.callees.iter().map(|c| serde_json::json!({
                                "name": c.name,
                                "file": c.file,
                                "line": c.line,
                                "sig": c.sig,
                            })).collect::<Vec<_>>(),
                            "depth2": result.depth2.iter().map(|n| serde_json::json!({
                                "name": n.name,
                                "file": n.file,
                                "line": n.line,
                                "sig": n.sig,
                            })).collect::<Vec<_>>(),
                            "tokens_used": result.tokens_used,
                            "budget_hit": result.budget_hit,
                            "language_note": result.language_note,
                        });
                        println!("{}", serde_json::to_string_pretty(&json).unwrap_or_default());
                    }
                    _ => {
                        print!("{}", render_reach(&result));
                        eprintln!(
                            "\n[reach] {} tokens · depth {} · {} caller(s) · {} callee(s){}",
                            result.tokens_used,
                            depth,
                            result.callers.len(),
                            result.callees.len(),
                            if result.budget_hit { " [budget hit]" } else { "" },
                        );
                    }
                }
                Ok(())
            }
            Err(e) => {
                eprintln!("reach: {e}");
                std::process::exit(1);
            }
        }
    } else {
        // Multi-symbol: run build_reach for each, merge and render.
        // --file and --format json are not supported in multi-symbol mode.
        let mut results = Vec::new();
        for &sym in symbols {
            let opts = ReachOptions {
                depth,
                budget,
                file_filter: None,
                include_tests,
                show_body,
                ..Default::default()
            };
            match build_reach(root, &mapped, sym, &opts) {
                Ok(r) => results.push(r),
                Err(e) => eprintln!("reach: {sym}: {e}"),
            }
        }
        if results.is_empty() {
            eprintln!("reach: no symbols resolved");
            std::process::exit(1);
        }
        print!("{}", render_multi_reach(&results));
        let total: usize = results.iter().map(|r| r.tokens_used).sum();
        let resolved = results.len();
        eprintln!(
            "\n[reach] {} tokens combined · {} of {} symbol(s) resolved",
            total, resolved, symbols.len()
        );
        Ok(())
    }
}

fn update_mode() -> Result<()> {
    // Find the install script relative to the binary's own location or from the
    // well-known repo layout (script lives two directories above the binary in
    // target/release/).
    let binary = std::env::current_exe().unwrap_or_default();
    let candidates = [
        // Running from target/release/ inside the repo
        binary.parent().and_then(|p| p.parent()).and_then(|p| p.parent())
            .map(|p| p.join("install.sh")),
        // Installed to ~/.local/bin — script not here; fall through
        None,
    ];

    for candidate in candidates.into_iter().flatten() {
        if candidate.exists() {
            println!("Running {} …", candidate.display());
            let status = std::process::Command::new("bash")
                .arg(&candidate)
                .status()
                .with_context(|| format!("Failed to run {}", candidate.display()))?;
            if status.success() {
                println!("Update complete.");
            } else {
                anyhow::bail!("install.sh exited with status {}", status);
            }
            return Ok(());
        }
    }

    println!("install.sh not found next to this binary.");
    println!("To update manually, clone the repo and run: bash install.sh");
    println!("Or run: cargo install --path mapper-core/CodeCartographer");
    Ok(())
}
