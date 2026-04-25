mod api;
mod call_graph;
mod diagram;
mod diagram_export;
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
mod uc_agents;
mod uc_analytics;
mod uc_client;
mod uc_sync;
mod uc_webhooks;
mod webhooks;

use anyhow::{Context, Result};
use arboard::Clipboard;
use clap::{Parser, Subcommand, ValueEnum};
use formatter::{estimate_tokens, format_token_count, get_formatter, OutputTarget};
use mapper::{extract_skeleton, MappedFile};
use memory::Memory;
use notify_debouncer_mini::{new_debouncer, notify::RecursiveMode, DebouncedEventKind};
use scanner::{is_ignored_path, scan_files_with_noise_tracking, IgnoredFile};
use std::collections::{HashMap, HashSet};
use std::fs::{self, File};
use std::io::{self, BufWriter, Write};
use std::path::{Path, PathBuf};
use std::sync::mpsc::channel;
use std::time::Duration;
use sync::SyncService;
use uc_agents::{AgentService, AgentType};
use uc_analytics::AnalyticsService;
use uc_sync::UCSyncService;
use uc_webhooks::{AgentContext, WebhookService};

const TOKEN_THRESHOLD_GREEN: usize = 10_000;
const TOKEN_THRESHOLD_YELLOW: usize = 30_000;
const WATCH_DEBOUNCE_MS: u64 = 500;

#[derive(Parser)]
#[command(name = "cartographer")]
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
        /// Auto-push to UC cloud after each detected change
        #[arg(long)]
        push: bool,
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
    /// Initialize UC cloud sync
    Init {
        #[arg(long)]
        cloud: bool,
        #[arg(long, value_name = "NAME")]
        project: Option<String>,
    },
    /// Push local context to UC
    Push,
    /// Pull context from UC
    Pull {
        #[arg(long, value_name = "VERSION")]
        version: Option<u32>,
    },
    /// Show UC context history
    History,
    /// Create a context branch
    Branch {
        #[arg(value_name = "NAME")]
        name: String,
        #[arg(long, value_name = "VERSION")]
        from: Option<u32>,
    },
    /// Diff between two versions
    Diff {
        #[arg(value_name = "V1")]
        v1: u32,
        #[arg(value_name = "V2")]
        v2: u32,
    },
    /// Manage AI agents
    Agents {
        #[command(subcommand)]
        command: AgentCommands,
    },
    /// View analytics dashboard
    Analytics,
    /// Get optimization suggestions
    Optimize,
    /// Export context for agents
    Export {
        #[arg(value_name = "FORMAT", default_value = "json")]
        format: String,
        #[arg(short = 'o', long, value_name = "FILE")]
        output: Option<PathBuf>,
    },
    /// Notify agents of context update
    Notify,
    /// Initialize Cartographer with CKB integration
    InitCkb {
        #[arg(long, value_name = "CKB_URL")]
        ckb_url: Option<String>,
        #[arg(long, value_name = "WEBHOOK_URL")]
        webhook_url: Option<String>,
    },
    /// Health check - shows architectural health score
    Health,
    /// Simulate how a change will impact the architecture
    Simulate {
        #[arg(long, value_name = "MODULE")]
        module: String,
        #[arg(long, value_name = "SIGNATURE")]
        new_signature: Option<String>,
        #[arg(long, value_name = "REMOVE")]
        remove_signature: Option<String>,
    },
    /// Show architecture evolution over time
    Evolution {
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
    Serve,
    /// Show project and cloud sync status
    Status,
    /// Manage global cartographer configuration
    Config {
        /// Set the UC API key globally
        #[arg(long, value_name = "KEY")]
        api_key: Option<String>,
        /// Set the default output target globally (claude, cursor, raw)
        #[arg(long, value_name = "TARGET")]
        default_target: Option<String>,
        /// Print current global configuration
        #[arg(long)]
        show: bool,
    },
    /// Show temporal coupling pairs from git history
    Cochange {
        /// Number of commits to analyse
        #[arg(long, default_value = "500")]
        commits: usize,
        /// Minimum co-change count to display
        #[arg(long, default_value = "5")]
        min_count: usize,
    },
    /// Show hotspot files (high churn × high complexity)
    Hotspots {
        /// Number of commits to analyse
        #[arg(long, default_value = "500")]
        commits: usize,
        /// Number of results to show
        #[arg(long, default_value = "15")]
        top: usize,
    },
    /// Show files with high co-change dispersion (shotgun surgery candidates)
    Shotgun {
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
    Dead,
    /// Export dependency graph as a diagram
    Diagram {
        /// Output format: mermaid, dot, or ascii (aliases: tree, text)
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
    },
    /// Generate llms.txt index for this project
    Llmstxt {
        /// Write to file instead of stdout
        #[arg(short = 'o', long, value_name = "FILE")]
        output: Option<PathBuf>,
    },
    /// Generate CLAUDE.md architecture guide
    Claudemd {
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
    /// CI gate: exit non-zero if cycles or layer violations are found
    Check,
    /// Ranked skeleton context pruned to a token budget (personalized PageRank)
    Context {
        /// Focus files for personalization (repeatable)
        #[arg(long = "focus", value_name = "FILE")]
        focus: Vec<String>,
        /// Maximum tokens to include (0 = unlimited)
        #[arg(long, default_value = "8000")]
        budget: usize,
        /// Also search for this pattern and bundle results into the context output
        #[arg(long, value_name = "PATTERN")]
        query: Option<String>,
    },
    /// Show symbol-level analysis (unreferenced public exports)
    Symbols {
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
    },
}

#[derive(Subcommand)]
enum AgentCommands {
    /// List all configured agents
    List,
    /// Add a new agent
    Add {
        #[arg(value_name = "NAME")]
        name: String,
        #[arg(short = 't', long = "type", value_name = "TYPE")]
        agent_type: String,
        #[arg(long, value_name = "KEY")]
        api_key: Option<String>,
        #[arg(long, value_name = "URL")]
        webhook: Option<String>,
    },
    /// Remove an agent
    Remove {
        #[arg(value_name = "ID")]
        id: String,
    },
    /// Show agent details
    Show {
        #[arg(value_name = "ID")]
        id: String,
    },
    /// Enable an agent
    Enable {
        #[arg(value_name = "ID")]
        id: String,
    },
    /// Disable an agent
    Disable {
        #[arg(value_name = "ID")]
        id: String,
    },
}

fn main() -> Result<()> {
    let cli = Cli::parse();
    let cwd = std::env::current_dir().context("Failed to get current directory")?;
    // Resolve target: CLI flag > per-repo .cartographer/config.toml > global config > claude
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
        Some(Commands::Watch { path, push }) => {
            let root = resolve_path(&cwd, path.or(cli.path.clone()))?;
            live_watch_mode(&root, &cwd, target, push)
        }
        Some(Commands::Copy { path }) => {
            let root = resolve_path(&cwd, path.or(cli.path.clone()))?;
            copy_mode(&root, target, &ignore_set)
        }
        Some(Commands::Sync { path }) => {
            let root = resolve_path(&cwd, path.or(cli.path.clone()))?;
            sync_mode(&root, &cwd, target, cli.copy)
        }
        Some(Commands::Init { cloud, project }) => {
            let root = resolve_path(&cwd, cli.path)?;
            if cloud {
                init_cloud_mode(&root, project.as_deref())
            } else {
                init_local_mode(&root)
            }
        }
        Some(Commands::Push) => {
            let root = resolve_path(&cwd, cli.path)?;
            push_mode(&root)
        }
        Some(Commands::Pull { version }) => {
            let root = resolve_path(&cwd, cli.path)?;
            pull_mode(&root, version)
        }
        Some(Commands::History) => {
            let root = resolve_path(&cwd, cli.path)?;
            history_mode(&root)
        }
        Some(Commands::Branch { name, from }) => {
            let root = resolve_path(&cwd, cli.path)?;
            branch_mode(&root, &name, from)
        }
        Some(Commands::Diff { v1, v2 }) => {
            let root = resolve_path(&cwd, cli.path)?;
            diff_mode(&root, v1, v2)
        }
        Some(Commands::Agents { command }) => {
            let root = resolve_path(&cwd, cli.path)?;
            agents_mode(&root, command)
        }
        Some(Commands::Analytics) => {
            let root = resolve_path(&cwd, cli.path)?;
            analytics_mode(&root)
        }
        Some(Commands::Optimize) => {
            let root = resolve_path(&cwd, cli.path)?;
            optimize_mode(&root)
        }
        Some(Commands::Export { format, output }) => {
            let root = resolve_path(&cwd, cli.path)?;
            export_mode(&root, &format, output.as_deref())
        }
        Some(Commands::Notify) => {
            let root = resolve_path(&cwd, cli.path)?;
            notify_mode(&root)
        }
        Some(Commands::InitCkb {
            ckb_url,
            webhook_url,
        }) => {
            let root = resolve_path(&cwd, cli.path)?;
            init_ckb_mode(&root, ckb_url.as_deref(), webhook_url.as_deref())
        }
        Some(Commands::Health) => {
            let root = resolve_path(&cwd, cli.path)?;
            health_mode(&root)
        }
        Some(Commands::Simulate {
            module,
            new_signature,
            remove_signature,
        }) => {
            let root = resolve_path(&cwd, cli.path)?;
            simulate_mode(
                &root,
                &module,
                new_signature.as_deref(),
                remove_signature.as_deref(),
            )
        }
        Some(Commands::Evolution { days }) => {
            let root = resolve_path(&cwd, cli.path)?;
            evolution_mode(&root, days)
        }
        Some(Commands::Deps { target, format }) => {
            let root = resolve_path(&cwd, cli.path)?;
            deps_mode(&root, &target, &format)
        }
        Some(Commands::Serve) => {
            let root = resolve_path(&cwd, cli.path)?;
            mcp_serve_mode(&root)
        }
        Some(Commands::Status) => {
            let root = resolve_path(&cwd, cli.path)?;
            status_mode(&root)
        }
        Some(Commands::Config {
            api_key,
            default_target,
            show,
        }) => config_mode(api_key, default_target, show),
        Some(Commands::Cochange {
            commits,
            min_count,
        }) => {
            let root = resolve_path(&cwd, cli.path)?;
            cochange_mode(&root, commits, min_count)
        }
        Some(Commands::Hotspots { commits, top }) => {
            let root = resolve_path(&cwd, cli.path)?;
            hotspots_mode(&root, commits, top)
        }
        Some(Commands::Shotgun { commits, top, min_partners }) => {
            let root = resolve_path(&cwd, cli.path)?;
            shotgun_mode(&root, commits, top, min_partners)
        }
        Some(Commands::Dead) => {
            let root = resolve_path(&cwd, cli.path)?;
            dead_mode(&root)
        }
        Some(Commands::Diagram {
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
        }) => {
            let root = resolve_path(&cwd, cli.path)?;
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
            )
        }
        Some(Commands::Llmstxt { output }) => {
            let root = resolve_path(&cwd, cli.path)?;
            llmstxt_mode(&root, output.as_deref())
        }
        Some(Commands::Claudemd { output }) => {
            let root = resolve_path(&cwd, cli.path)?;
            claudemd_mode(&root, output.as_deref())
        }
        Some(Commands::Semidiff { commit1, commit2 }) => {
            let root = resolve_path(&cwd, cli.path)?;
            semidiff_mode(&root, &commit1, &commit2)
        }
        Some(Commands::Check) => {
            let root = resolve_path(&cwd, cli.path)?;
            check_mode(&root)
        }
        Some(Commands::Context { focus, budget, query }) => {
            let root = resolve_path(&cwd, cli.path)?;
            context_mode(&root, &focus, budget, query.as_deref())
        }
        Some(Commands::Symbols { unreferenced }) => {
            let root = resolve_path(&cwd, cli.path)?;
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
        Some(Commands::ContextHealth { file, model, window, format }) => {
            context_health_mode(file.as_deref(), &model, window, &format)
        }
        None => {
            let root = resolve_path(&cwd, cli.path)?;
            source_mode(&root, &cwd, target, cli.copy, &ignore_set)
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

/// Record per-file token costs and sync count into the analytics log.
/// Non-fatal — analytics are best-effort.
fn record_analytics(root: &Path, memory: &Memory) {
    if let Ok(mut analytics) = uc_analytics::Analytics::load(root) {
        for (path, entry) in &memory.files {
            let tokens = entry.content.len() / 4;
            analytics.record_file_access(path, tokens);
        }
        analytics.record_sync();
        let _ = analytics.save(root);
    }
}

/// After a watch-detected change, do an incremental sync + UC push.
/// Errors are printed but never propagate — the watcher must keep running.
fn watch_push(root: &Path) {
    let existing = Memory::load(root).unwrap_or_default();
    let service = SyncService::new(root);
    match service.incremental_sync_with_noise(existing) {
        Ok(result) => {
            let memory = result.memory;
            if memory.save(root).is_err() {
                eprintln!("[{}] watch --push: failed to save memory", chrono_time());
                return;
            }
            record_analytics(root, &memory);
            match push_mode(root) {
                Ok(_) => println!("[{}] Pushed to cloud", chrono_time()),
                Err(e) => eprintln!("[{}] Push failed: {}", chrono_time(), e),
            }
        }
        Err(e) => eprintln!("[{}] watch --push: sync error: {}", chrono_time(), e),
    }
}

fn live_watch_mode(root: &Path, output_dir: &Path, target: OutputTarget, push: bool) -> Result<()> {
    println!("LIVE WATCHER: Monitoring {}...", root.display());
    println!("============================================");
    println!("  Mode: Skeleton Map ONLY (lightweight)");
    println!("  Debounce: {}ms", WATCH_DEBOUNCE_MS);
    println!("  Auto-push: {}", if push { "enabled" } else { "disabled (use --push to enable)" });
    println!("  Full source: Use 'cartographer copy' when needed");
    println!("============================================");
    println!("Press Ctrl+C to stop\n");

    // Cache: rel_path → (content_hash, MappedFile) for incremental re-extraction.
    let mut extract_cache: HashMap<String, (u64, MappedFile)> = HashMap::new();

    // Initial skeleton map generation
    let (mapped_files, ignored) = generate_skeleton_map_incremental(root, &mut extract_cache)?;
    let output = format_map_output(&mapped_files, target);
    let tokens = estimate_tokens(&output);

    // Write lightweight map file
    let formatter = get_formatter(target);
    let map_filename = format!("cartographer_map.{}", formatter.extension());
    let map_path = output_dir.join(&map_filename);
    fs::write(&map_path, &output)?;

    print_cartographer_report(mapped_files.len(), &ignored);
    println!("Map: {} | {}", map_filename, format_token_count(tokens));
    println!("Watching for changes...\n");

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
                        && !e.path.ends_with(".cartographer_memory.json")
                        && !e.path.ends_with("context.xml")
                        && !e.path.ends_with("context.md")
                        && !e.path.ends_with("context.json")
                        && !is_ignored_path(&e.path)
                });

                if relevant {
                    // Regenerate skeleton map (incremental — skips unchanged files)
                    match generate_skeleton_map_incremental(root, &mut extract_cache) {
                        Ok((files, _)) => {
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
                                root.join(".cartographer_watch_state.json"),
                                serde_json::to_string_pretty(&sentinel).unwrap_or_default(),
                            );
                        }
                        Err(e) => eprintln!("Error updating map: {}", e),
                    }
                    if push {
                        watch_push(root);
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

fn generate_skeleton_map(root: &Path) -> Result<(Vec<MappedFile>, Vec<IgnoredFile>)> {
    let scan_result = scan_files_with_noise_tracking(root)?;
    let mut mapped_files: Vec<MappedFile> = Vec::new();

    for path in &scan_result.files {
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

    print_cartographer_report(memory.files.len(), &ignored);

    // Generate output to memory only (NOT to disk)
    let formatter = get_formatter(target);
    let output = formatter.format(&memory);
    let tokens = estimate_tokens(&output);

    println!(
        "Generated: {} files, {}",
        memory.files.len(),
        format_token_count(tokens)
    );

    // Token budget check then copy to clipboard
    if tokens > TOKEN_THRESHOLD_YELLOW {
        println!("\nHIGH COST WARNING");
        println!(
            "Token count: {} | Estimated cost: ~${:.2}",
            format_token_count(tokens),
            estimate_cost(tokens)
        );
        println!("Recommend using `cartographer map` first or targeting a specific folder.\n");
        print!("[?] Copy to clipboard anyway? (y/N) ");
        io::stdout().flush()?;
        let mut input = String::new();
        io::stdin().read_line(&mut input)?;
        if input.trim().eq_ignore_ascii_case("y") || input.trim().eq_ignore_ascii_case("yes") {
            copy_to_clipboard(&output)?;
        } else {
            println!("Cancelled. No data written to disk or clipboard.");
        }
    } else if tokens > TOKEN_THRESHOLD_GREEN {
        println!("\nMODERATE COST");
        println!(
            "Token count: {} | Estimated cost: ~${:.2}",
            format_token_count(tokens),
            estimate_cost(tokens)
        );
        print!("[?] Copy to clipboard? (Y/n) ");
        io::stdout().flush()?;
        let mut input = String::new();
        io::stdin().read_line(&mut input)?;
        let input = input.trim();
        if input.is_empty() || input.eq_ignore_ascii_case("y") || input.eq_ignore_ascii_case("yes")
        {
            copy_to_clipboard(&output)?;
        } else {
            println!("Cancelled. No data written to disk or clipboard.");
        }
    } else {
        // Green zone - copy directly
        copy_to_clipboard(&output)?;
    }

    Ok(())
}

// =============================================================================
// MAP MODE - One-shot skeleton map generation
// =============================================================================

fn map_mode(root: &Path, output_dir: &Path, target: OutputTarget, copy: bool) -> Result<()> {
    println!("MAP MODE: Scanning {}...", root.display());

    let (mapped_files, ignored) = generate_skeleton_map(root)?;
    print_cartographer_report(mapped_files.len(), &ignored);

    let output = format_map_output(&mapped_files, target);
    let tokens = estimate_tokens(&output);

    let formatter = get_formatter(target);
    let filename = format!("cartographer_map.{}", formatter.extension());
    fs::write(output_dir.join(&filename), &output)?;

    println!(
        "Generated: {} | {} files, {}",
        filename,
        mapped_files.len(),
        format_token_count(tokens)
    );
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
    println!("SOURCE MODE: Scanning {}...", root.display());

    let service = SyncService::new(root);
    let result = service.full_scan_with_noise()?;
    let mut memory = result.memory;
    let ignored = result.ignored_noise;

    if !ignore_set.is_empty() {
        memory.files.retain(|path, _| {
            let filename = path.rsplit('/').next().unwrap_or(path);
            !ignore_set.contains(filename) && !ignore_set.contains(path)
        });
        println!("User-ignored {} file(s)", ignore_set.len());
    }

    print_cartographer_report(memory.files.len(), &ignored);
    let memory = handle_ignored_consent(&service, memory, &ignored)?;
    memory.save(output_dir)?;
    record_analytics(root, &memory);
    let output = write_output(output_dir, &memory, target)?;
    let tokens = estimate_tokens(&output);
    println!(
        "Generated context ({} files, {})",
        memory.files.len(),
        format_token_count(tokens)
    );
    handle_token_budget_copy(&output, tokens, copy)?;
    Ok(())
}

// =============================================================================
// SYNC MODE - Incremental update
// =============================================================================

fn sync_mode(root: &Path, output_dir: &Path, target: OutputTarget, copy: bool) -> Result<()> {
    println!("SYNC MODE: Scanning {}...", root.display());

    let service = SyncService::new(root);
    let existing = Memory::load(output_dir).unwrap_or_default();
    let result = service.incremental_sync_with_noise(existing)?;
    let memory = result.memory;
    let ignored = result.ignored_noise;

    print_cartographer_report(memory.files.len(), &ignored);
    let memory = handle_ignored_consent(&service, memory, &ignored)?;
    memory.save(output_dir)?;
    record_analytics(root, &memory);
    let output = write_output(output_dir, &memory, target)?;
    let tokens = estimate_tokens(&output);
    println!(
        "Synced context ({} files, {})",
        memory.files.len(),
        format_token_count(tokens)
    );
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
        out.push_str(&format!("<file path=\"{}\">\n", escape_xml(&file.path)));
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
        out.push_str(&format!(
            "## {}\n\n`{}\n{}\n`\n\n",
            file.path,
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
// CMP Report
// =============================================================================

fn print_cartographer_report(included_count: usize, ignored: &[IgnoredFile]) {
    println!();
    println!("CMP REPORT:");
    println!("============================================");
    println!("  Included: {} files (Source Code)", included_count);
    if ignored.is_empty() {
        println!("  Ignored Noise: None");
    } else {
        let noise_names: Vec<&str> = ignored
            .iter()
            .take(5)
            .map(|i| i.path.rsplit('/').next().unwrap_or(&i.path))
            .collect();
        let display = if ignored.len() > 5 {
            format!(
                "{}, ... (+{} more)",
                noise_names.join(", "),
                ignored.len() - 5
            )
        } else {
            noise_names.join(", ")
        };
        let total_tokens: usize = ignored.iter().map(|i| i.estimated_tokens).sum();
        println!(
            "  Ignored Noise: {} (saved ~{})",
            display,
            format_token_count(total_tokens)
        );
    }
    println!("============================================");
}

// =============================================================================
// Token Budget Check
// =============================================================================

fn handle_token_budget_copy(content: &str, tokens: usize, auto_copy: bool) -> Result<()> {
    if tokens > TOKEN_THRESHOLD_YELLOW {
        println!("\nHIGH COST WARNING");
        println!(
            "Token count: {} | Estimated cost: ~${:.2}",
            format_token_count(tokens),
            estimate_cost(tokens)
        );
        println!("Recommend using `cartographer map` first or targeting a specific folder.\n");
        print!("[?] Proceed with copy? (y/N) ");
        io::stdout().flush()?;
        let mut input = String::new();
        io::stdin().read_line(&mut input)?;
        if input.trim().eq_ignore_ascii_case("y") || input.trim().eq_ignore_ascii_case("yes") {
            copy_to_clipboard(content)?;
        } else {
            println!("Not copied (file still saved to disk)");
        }
    } else if tokens > TOKEN_THRESHOLD_GREEN {
        println!("\nMODERATE COST");
        println!(
            "Token count: {} | Estimated cost: ~${:.2}",
            format_token_count(tokens),
            estimate_cost(tokens)
        );
        print!("[?] Proceed with copy? (Y/n) ");
        io::stdout().flush()?;
        let mut input = String::new();
        io::stdin().read_line(&mut input)?;
        let input = input.trim();
        if input.is_empty() || input.eq_ignore_ascii_case("y") || input.eq_ignore_ascii_case("yes")
        {
            copy_to_clipboard(content)?;
        } else {
            println!("Not copied (file still saved to disk)");
        }
    } else {
        if auto_copy {
            copy_to_clipboard(content)?;
        } else {
            print!("[?] Copy to clipboard? (Y/n) ");
            io::stdout().flush()?;
            let mut input = String::new();
            io::stdin().read_line(&mut input)?;
            let input = input.trim();
            if input.is_empty()
                || input.eq_ignore_ascii_case("y")
                || input.eq_ignore_ascii_case("yes")
            {
                copy_to_clipboard(content)?;
            } else {
                println!("Saved to disk only");
            }
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
            println!("Copied to clipboard");
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

fn handle_ignored_consent(
    service: &SyncService,
    mut memory: Memory,
    ignored: &[IgnoredFile],
) -> Result<Memory> {
    if ignored.is_empty() {
        return Ok(memory);
    }
    let total_tokens: usize = ignored.iter().map(|i| i.estimated_tokens).sum();
    print!(
        "\n[?] Force-include {} ignored files? (y/N) ",
        ignored.len()
    );
    io::stdout().flush()?;
    let mut input = String::new();
    io::stdin().read_line(&mut input)?;
    if input.trim().eq_ignore_ascii_case("y") || input.trim().eq_ignore_ascii_case("yes") {
        println!(
            "WARNING: Adding ~{} of noise!",
            format_token_count(total_tokens)
        );
        service.include_ignored_files(&mut memory, ignored);
        println!("Force-included {} files", ignored.len());
    } else {
        println!("Keeping noise files excluded (recommended)");
    }
    Ok(memory)
}

// =============================================================================
// UC CLOUD SYNC MODES
// =============================================================================

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
    let repo_cfg_path = cwd.join(".cartographer").join("config.toml");
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

// =============================================================================
// UC CLOUD SYNC MODES
// =============================================================================

fn get_uc_api_key() -> Result<String> {
    // 1. Environment variable
    if let Ok(key) = std::env::var("ULTRA_CONTEXT") {
        return Ok(key);
    }

    // 2. .env.local in current directory
    if let Ok(content) = fs::read_to_string(".env.local") {
        for line in content.lines() {
            if line.starts_with("ULTRA_CONTEXT=") {
                if let Some(key) = line.strip_prefix("ULTRA_CONTEXT=") {
                    return Ok(key.trim().to_string());
                }
            }
        }
    }

    // 3. Global config (~/.config/cartographer/config.toml)
    let global = global_config::GlobalConfig::load();
    if let Some(key) = global.api.key {
        if !key.is_empty() {
            return Ok(key);
        }
    }

    anyhow::bail!(
        "UC API key not found.\n  Set ULTRA_CONTEXT env var, add to .env.local, or run:\n  cartographer config --api-key <key>"
    )
}

fn init_local_mode(root: &Path) -> Result<()> {
    let config_path = root.join(".cartographer").join("config.toml");
    if config_path.exists() {
        println!("Config already exists at: {}", config_path.display());
        println!("Edit it directly or run 'cartographer init --cloud' to enable cloud sync.");
        return Ok(());
    }
    let config_dir = config_path.parent().unwrap();
    fs::create_dir_all(config_dir)?;

    let project_name = root
        .file_name()
        .and_then(|n| n.to_str())
        .unwrap_or("my-project");

    let config_content = format!(
        r#"# Cartographer Configuration
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
    println!("Initialized .cartographer/config.toml");
    println!("Project: {}", project_name);
    println!();
    println!("Next steps:");
    println!("  cartographer source          — generate context");
    println!("  cartographer init --cloud    — enable UC cloud sync");
    println!("  Edit {} to configure layers and defaults", config_path.display());
    Ok(())
}

fn status_mode(root: &Path) -> Result<()> {
    println!("Cartographer Status");
    println!("============================================");
    println!("Root: {}", root.display());
    println!();

    // Local memory
    let memory = Memory::load(root).unwrap_or_default();
    if memory.files.is_empty() {
        println!("Local memory: not initialized (run 'cartographer source')");
    } else {
        println!("Tracked files:  {}", memory.files.len());
        println!("Memory version: {}", memory.version);
        if memory.last_sync > 0 {
            println!("Last scanned:   {}", format_timestamp(memory.last_sync));
        }
    }
    println!();

    // Cloud sync state
    match uc_sync::UCConfig::load(root) {
        Ok(config) => {
            println!("Cloud context:  {}", config.context_id);
            println!("Cloud version:  {}", config.last_version);
            println!("Last pushed:    {}", format_timestamp(config.last_sync));

            // Detect unpushed local changes
            let mut unpushed = 0usize;
            let mut new_local = 0usize;
            for (path, entry) in &memory.files {
                match config.file_hashes.get(path) {
                    None => new_local += 1,
                    Some(&h) if h != entry.hash => unpushed += 1,
                    _ => {}
                }
            }
            let deleted_remote = config
                .file_hashes
                .keys()
                .filter(|k| !memory.files.contains_key(*k))
                .count();

            if unpushed == 0 && new_local == 0 && deleted_remote == 0 {
                println!("Sync status:    up to date");
            } else {
                println!(
                    "Sync status:    {} modified, {} new, {} deleted (not yet pushed)",
                    unpushed, new_local, deleted_remote
                );
            }
        }
        Err(_) => {
            println!("Cloud sync:     not configured (run 'cartographer init --cloud')");
        }
    }
    println!();

    // Global config
    let global = global_config::GlobalConfig::load();
    let key_status = if global.api.key.is_some() {
        "configured"
    } else {
        "not set (run 'cartographer config --api-key <key>')"
    };
    println!("Global API key:  {}", key_status);
    let target_status = global
        .defaults
        .target
        .as_deref()
        .unwrap_or("claude (default)");
    println!("Global target:   {}", target_status);

    // Per-repo config
    let repo_cfg = root.join(".cartographer").join("config.toml");
    if repo_cfg.exists() {
        println!("Repo config:     {}", repo_cfg.display());
    } else {
        println!("Repo config:     not present (run 'cartographer init')");
    }

    // .cartographerignore
    let ignore_path = root.join(".cartographerignore");
    if ignore_path.exists() {
        let pattern_count = fs::read_to_string(&ignore_path)
            .unwrap_or_default()
            .lines()
            .filter(|l| !l.trim().is_empty() && !l.trim().starts_with('#'))
            .count();
        println!(".cartographerignore: {} pattern(s)", pattern_count);
    } else {
        println!(".cartographerignore: not present");
    }

    println!("============================================");
    Ok(())
}

fn config_mode(
    api_key: Option<String>,
    default_target: Option<String>,
    show: bool,
) -> Result<()> {
    if api_key.is_none() && default_target.is_none() && !show {
        println!("Usage:");
        println!("  cartographer config --show");
        println!("  cartographer config --api-key <key>");
        println!("  cartographer config --default-target <claude|cursor|raw>");
        return Ok(());
    }

    if show {
        let global = global_config::GlobalConfig::load();
        let path = global_config::GlobalConfig::config_path()
            .map(|p| p.display().to_string())
            .unwrap_or_else(|| "(unknown)".into());
        println!("Global config: {}", path);
        println!(
            "  api.key:          {}",
            global
                .api
                .key
                .as_deref()
                .map(|k| {
                    // Show only last 4 chars for security
                    if k.len() > 4 {
                        format!("{}...{}", &k[..4], &k[k.len() - 4..])
                    } else {
                        "****".into()
                    }
                })
                .unwrap_or_else(|| "(not set)".into())
        );
        println!(
            "  defaults.target:  {}",
            global.defaults.target.as_deref().unwrap_or("(not set, defaults to claude)")
        );
        return Ok(());
    }

    let mut global = global_config::GlobalConfig::load();
    let mut changed = false;

    if let Some(key) = api_key {
        global.api.key = Some(key);
        changed = true;
        println!("API key saved.");
    }
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

fn init_cloud_mode(root: &Path, project_name: Option<&str>) -> Result<()> {
    let api_key = get_uc_api_key()?;
    let service = UCSyncService::new(api_key, root)?;

    let project = project_name.unwrap_or_else(|| {
        root.file_name()
            .and_then(|n| n.to_str())
            .unwrap_or("my-project")
    });

    service.init(project)?;
    Ok(())
}

fn push_mode(root: &Path) -> Result<()> {
    let api_key = get_uc_api_key()?;
    let service = UCSyncService::new(api_key, root)?;

    // Load local memory
    let memory = Memory::load(root).context("No local memory found. Run 'cartographer source' first.")?;

    // Track what changed for webhook notification
    let old_config = uc_sync::UCConfig::load(root).ok();
    let old_files: HashSet<String> = old_config
        .as_ref()
        .map(|c| c.file_hashes.keys().cloned().collect())
        .unwrap_or_default();

    let new_files: HashSet<String> = memory.files.keys().cloned().collect();

    let added: Vec<String> = new_files.difference(&old_files).cloned().collect();
    let deleted: Vec<String> = old_files.difference(&new_files).cloned().collect();

    // Detect modified files by comparing hashes
    let mut modified: Vec<String> = Vec::new();
    if let Some(ref old_cfg) = old_config {
        for (path, entry) in &memory.files {
            if let Some(&old_hash) = old_cfg.file_hashes.get(path) {
                if old_hash != entry.hash {
                    modified.push(path.clone());
                }
            }
        }
    }

    // Push to UC
    let config = service.push(&memory)?;
    record_analytics(root, &memory);

    // Notify agents via webhooks
    let agent_service = AgentService::new(root);
    if let Ok(agents) = agent_service.list_agents() {
        if !agents.is_empty() {
            println!("\nNotifying {} agent(s)...", agents.len());

            let webhook_service = WebhookService::new()?;
            let payload = uc_webhooks::WebhookService::create_payload(
                &config.context_id,
                config.last_version,
                added.clone(),
                modified.clone(),
                deleted.clone(),
                memory.files.len(),
            );

            let results = webhook_service.notify_all(&agents, &payload);
            let success_count = results.iter().filter(|r| r.is_ok()).count();
            let fail_count = results.len() - success_count;

            if success_count > 0 {
                println!("✓ Notified {} agent(s)", success_count);
            }
            if fail_count > 0 {
                println!("⚠️  {} agent(s) failed to notify", fail_count);
                for (i, result) in results.iter().enumerate() {
                    if let Err(e) = result {
                        println!("  - Agent {}: {}", i + 1, e);
                    }
                }
            }
        }
    }

    Ok(())
}

fn pull_mode(root: &Path, version: Option<u32>) -> Result<()> {
    let api_key = get_uc_api_key()?;
    let service = UCSyncService::new(api_key, root)?;

    let memory = service.pull(version)?;
    memory.save(root)?;

    println!(
        "✓ Memory saved to {}",
        root.join(".cartographer_memory.json").display()
    );
    Ok(())
}

fn history_mode(root: &Path) -> Result<()> {
    let api_key = get_uc_api_key()?;
    let service = UCSyncService::new(api_key, root)?;

    let history = service.history()?;

    if history.is_empty() {
        println!("No version history available.");
        return Ok(());
    }

    println!("\nContext Version History:");
    println!("============================================");
    for version in history {
        let affected = version
            .affected
            .as_ref()
            .map(|a: &Vec<String>| format!(" (affected: {})", a.len()))
            .unwrap_or_default();
        println!(
            "v{} - {} - {}{}",
            version.version, version.operation, version.timestamp, affected
        );
    }
    println!("============================================\n");

    Ok(())
}

fn branch_mode(root: &Path, name: &str, from_version: Option<u32>) -> Result<()> {
    let api_key = get_uc_api_key()?;
    let service = UCSyncService::new(api_key, root)?;

    service.branch(name, from_version)?;
    Ok(())
}

fn diff_mode(root: &Path, v1: u32, v2: u32) -> Result<()> {
    let api_key = get_uc_api_key()?;
    let service = UCSyncService::new(api_key, root)?;

    let diff = service.diff(v1, v2)?;
    diff.print();

    Ok(())
}

fn agents_mode(root: &Path, command: AgentCommands) -> Result<()> {
    let agent_service = AgentService::new(root);

    match command {
        AgentCommands::List => {
            agent_service.print_agents_table()?;
        }
        AgentCommands::Add {
            name,
            agent_type,
            api_key,
            webhook,
        } => {
            let config = uc_sync::UCConfig::load(root)?;

            let agent_type_enum = match agent_type.to_lowercase().as_str() {
                "cursor" => AgentType::Cursor,
                "copilot" => AgentType::Copilot,
                "claude" => AgentType::Claude,
                "custom" => AgentType::Custom,
                _ => anyhow::bail!("Invalid agent type. Use: cursor, copilot, claude, custom"),
            };

            agent_service.add_agent(
                &name,
                agent_type_enum,
                &config.context_id,
                api_key,
                webhook,
            )?;
        }
        AgentCommands::Remove { id } => {
            agent_service.remove_agent(&id)?;
        }
        AgentCommands::Show { id } => {
            agent_service.print_agent_details(&id)?;
        }
        AgentCommands::Enable { id } => {
            agent_service.enable_agent(&id)?;
        }
        AgentCommands::Disable { id } => {
            agent_service.disable_agent(&id)?;
        }
    }

    Ok(())
}

fn analytics_mode(root: &Path) -> Result<()> {
    let service = AnalyticsService::new(root);
    service.print_dashboard()?;
    Ok(())
}

fn optimize_mode(root: &Path) -> Result<()> {
    let service = AnalyticsService::new(root);
    let suggestions = service.optimize_suggestions()?;

    if suggestions.is_empty() {
        println!("✓ Context is already optimized!");
        return Ok(());
    }

    println!("\nOptimization Suggestions:");
    println!("============================================");
    for (i, suggestion) in suggestions.iter().enumerate() {
        println!("{}. {}", i + 1, suggestion);
    }
    println!("============================================\n");

    Ok(())
}

fn export_mode(root: &Path, format: &str, output: Option<&Path>) -> Result<()> {
    let memory = Memory::load(root).context("No local memory found. Run 'cartographer source' first.")?;
    let config = uc_sync::UCConfig::load(root)?;

    let agent_context = AgentContext::from_memory(&memory, &config.context_id);

    let content = match format.to_lowercase().as_str() {
        "json" => agent_context.to_json()?,
        "markdown" | "md" => agent_context.to_markdown(),
        _ => anyhow::bail!("Unknown format: {}. Use 'json' or 'markdown'", format),
    };

    if let Some(output_path) = output {
        fs::write(output_path, &content)?;
        println!("✓ Exported to: {}", output_path.display());
    } else {
        println!("{}", content);
    }

    Ok(())
}

fn notify_mode(root: &Path) -> Result<()> {
    let memory = Memory::load(root).context("No local memory found. Run 'cartographer source' first.")?;
    let config = uc_sync::UCConfig::load(root)?;
    let agent_service = AgentService::new(root);

    let agents = agent_service.list_agents()?;
    if agents.is_empty() {
        println!("No agents configured. Use 'cartographer agents add' to add one.");
        return Ok(());
    }

    let webhook_agents: Vec<_> = agents.iter().filter(|a| a.webhook_url.is_some()).collect();
    if webhook_agents.is_empty() {
        println!("No agents with webhooks configured.");
        return Ok(());
    }

    println!(
        "Notifying {} agent(s) with webhooks...",
        webhook_agents.len()
    );

    let webhook_service = WebhookService::new()?;
    let payload = uc_webhooks::WebhookService::create_payload(
        &config.context_id,
        config.last_version,
        vec![],
        memory.files.keys().cloned().collect(),
        vec![],
        memory.files.len(),
    );

    let results = webhook_service.notify_all(&agents, &payload);
    let success_count = results.iter().filter(|r| r.is_ok()).count();
    let fail_count = results.len() - success_count;

    if success_count > 0 {
        println!("✓ Notified {} agent(s)", success_count);
    }
    if fail_count > 0 {
        println!("⚠️  {} agent(s) failed", fail_count);
        for result in results.iter() {
            if let Err(e) = result {
                println!("  - {}", e);
            }
        }
    }

    Ok(())
}

fn init_ckb_mode(root: &Path, ckb_url: Option<&str>, webhook_url: Option<&str>) -> Result<()> {
    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║      Cartographer v1.0.0 - CKB Integration Setup           ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();

    let config_path = root.join(".cartographer").join("config.toml");
    let config_dir = config_path.parent().unwrap();
    std::fs::create_dir_all(config_dir)?;

    let ckb_url = ckb_url.unwrap_or("http://localhost:8080");
    let webhook_url = webhook_url.unwrap_or("http://localhost:8081/webhook");

    let config_content = format!(
        r#"# Cartographer Configuration
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
    println!("  2. Run 'cartographer map' to generate initial graph");
    println!("  3. Run 'cartographer health' to see architectural health");
    println!();
    println!("🔗 CKB Integration:");
    println!("  - CKB URL: {}", ckb_url);
    println!("  - Webhook URL: {}", webhook_url);
    println!();
    println!("✅ Cartographer is ready to integrate with CKB!");

    Ok(())
}

fn health_mode(root: &Path) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, scan_files_with_noise_tracking};

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         Cartographer - Architectural Health Report          ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();

    let result = scan_files_with_noise_tracking(root)?;
    let files = result.files;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = files
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

    println!(
        "📊 Health Score: {:.1}/100",
        graph.metadata.health_score.unwrap_or(0.0)
    );
    println!();

    println!("📈 Statistics:");
    println!("  - Files: {}", graph.metadata.total_files);
    println!("  - Dependencies: {}", graph.metadata.total_edges);
    println!("  - Bridges: {}", graph.metadata.bridge_count.unwrap_or(0));
    println!("  - Cycles: {}", graph.metadata.cycle_count.unwrap_or(0));
    println!(
        "  - God Modules: {}",
        graph.metadata.god_module_count.unwrap_or(0)
    );
    println!(
        "  - Layer Violations: {}",
        graph.metadata.layer_violation_count.unwrap_or(0)
    );
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

    if graph.metadata.health_score.unwrap_or(100.0) < 70.0 {
        println!("⚠️  Architectural health is below acceptable threshold.");
        println!("   Run 'cartographer map --detail extended' for more information.");
    } else {
        println!("✅ Architecture looks healthy!");
    }

    Ok(())
}

fn simulate_mode(
    root: &Path,
    module: &str,
    new_signature: Option<&str>,
    remove_signature: Option<&str>,
) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, scan_files_with_noise_tracking};

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         Predictive Impact Analysis                         ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();

    let result = scan_files_with_noise_tracking(root)?;
    let files = result.files;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = files
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

    let change = state
        .simulate_change(module, new_signature, remove_signature)
        .map_err(|e| anyhow::anyhow!(e))?;

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

    Ok(())
}

fn evolution_mode(root: &Path, days: Option<u32>) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, scan_files_with_noise_tracking};

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         Architecture Evolution Report                      ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();

    let result = scan_files_with_noise_tracking(root)?;
    let files = result.files;
    let mapped_files: std::collections::HashMap<String, crate::mapper::MappedFile> = files
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

    let max_raw = graph
        .nodes
        .iter()
        .map(|n| {
            let c = *churn.get(&n.path).unwrap_or(&0);
            c * n.signature_count
        })
        .max()
        .unwrap_or(1)
        .max(1) as f64;

    let mut hotspot_count = 0usize;
    for node in &mut graph.nodes {
        let c = *churn.get(&node.path).unwrap_or(&0);
        node.churn = Some(c);
        let score = ((c * node.signature_count) as f64 / max_raw * 100.0).round();
        node.hotspot_score = Some(score);
        if score >= 20.0 {
            hotspot_count += 1;
        }
    }
    graph.metadata.hotspot_count = Some(hotspot_count);

    let known: std::collections::HashSet<&str> =
        graph.nodes.iter().map(|n| n.path.as_str()).collect();
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
            if let Some(d) = disp_map.get(node.path.as_str()) {
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
            if let Some(owner) = ownership.get(&node.path) {
                node.owner = Some(owner.clone());
            }
        }
    }
}

// =============================================================================
// COCHANGE MODE — Temporal coupling analysis from git history
// =============================================================================

fn cochange_mode(root: &Path, commits: usize, min_count: usize) -> Result<()> {
    let pairs = crate::git_analysis::git_cochange(root, commits);

    println!("╔═══════════════════════════════════════════════════════════╗");
    println!("║         Temporal Coupling Analysis                         ║");
    println!("╚═══════════════════════════════════════════════════════════╝");
    println!();
    println!("Last {} commits", commits);
    println!();

    let mut filtered: Vec<_> = pairs.iter().filter(|p| p.count >= min_count).collect();
    filtered.sort_by(|a, b| {
        b.coupling_score
            .partial_cmp(&a.coupling_score)
            .unwrap_or(std::cmp::Ordering::Equal)
    });

    if filtered.is_empty() {
        println!("No co-change pairs found with count >= {}.", min_count);
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

// =============================================================================
// HOTSPOTS MODE — High churn × high complexity files
// =============================================================================

fn hotspots_mode(root: &Path, commits: usize, top: usize) -> Result<()> {
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

    let churn = crate::git_analysis::git_churn(root, commits);

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
        println!(
            "  [{}] {} | churn: {} commits | sigs: {} | hotspot: {:.1}",
            label, path, c, sigs, score
        );
    }

    Ok(())
}

// =============================================================================
// SHOTGUN MODE — Co-change dispersion / shotgun surgery detection
// =============================================================================

fn shotgun_mode(root: &Path, commits: usize, top: usize, min_partners: usize) -> Result<()> {
    let mut entries = crate::git_analysis::git_cochange_dispersion(root, commits);

    entries.retain(|e| e.partner_count >= min_partners);
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

fn dead_mode(root: &Path) -> Result<()> {
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
) -> Result<()> {
    use crate::api::ApiState;
    use crate::mapper::extract_skeleton;
    use crate::scanner::{is_ignored_path, scan_files_with_noise_tracking};

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
        let cg = call_graph::build_file_call_graph(&abs, &source)
            .map_err(|e| anyhow::anyhow!(e))?
            .ok_or_else(|| anyhow::anyhow!(
                "call-graph extraction not supported for this file type (expected .rs / .py): {}",
                abs.display()
            ))?;
        let graph = call_graph::to_project_graph(&cg, &abs);
        let fmt = diagram::DiagramFormat::parse(format).map_err(|e| anyhow::anyhow!(e))?;
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

    // Git-backed overlays (co-change, owner coloring, hotspot sizing) need
    // the enrichment pass. Skip it when nothing downstream consumes it — the
    // git calls run in ~100ms on a warm repo, but there's no reason to pay
    // the cost for a plain top-N diagram.
    if cochange_threshold.is_some() || color_by_owner {
        enrich_with_git(&mut graph, root);
    }

    let fmt = diagram::DiagramFormat::parse(format).map_err(|e| anyhow::anyhow!(e))?;
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
        "# {}\n\n> Codebase index generated by Cartographer. {} modules.\n\n## Key Modules\n\n",
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
    content.push_str("Built with [Cartographer](https://github.com/SimplyLiz/Cartographer) v1.3.0\n");

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
         <!-- Auto-generated by Cartographer v1.3.0. Re-run: cartographer claudemd -->\n\n\
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
        cartographer serve       # Start MCP server\n\
        cartographer health      # Health report\n\
        cartographer hotspots    # Churn × complexity\n\
        cartographer dead        # Dead code candidates\n\
        cartographer semidiff HEAD~1  # What changed last commit\n\
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
/// and a git-HEAD-keyed persistent cache (.cartographer_cache.json).
fn build_mapped_files_cached(root: &Path) -> anyhow::Result<HashMap<String, MappedFile>> {
    use rayon::prelude::*;
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

    let cache_path = root.join(".cartographer_cache.json");

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

fn mcp_serve_mode(root: &Path) -> Result<()> {
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

    let server = McpServer::new(state);
    server.serve();
    Ok(())
}

// =============================================================================
// CHECK MODE — CI gate: non-zero exit on cycles or layer violations
// =============================================================================

fn check_mode(root: &Path) -> Result<()> {
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

fn context_mode(root: &Path, focus: &[String], budget: usize, query: Option<&str>) -> Result<()> {
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

    state.rebuild_graph().map_err(|e| anyhow::anyhow!(e))?;

    let ranked = state.ranked_skeleton(focus, budget).map_err(|e| anyhow::anyhow!(e))?;

    let total_tokens: usize = ranked.iter().map(|f| f.estimated_tokens).sum();

    eprintln!(
        "Ranked context: {} files, ~{} tokens (budget: {})",
        ranked.len(),
        total_tokens,
        if budget == 0 { "unlimited".to_string() } else { budget.to_string() }
    );
    if !focus.is_empty() {
        eprintln!("Focus: {}", focus.join(", "));
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
) -> Result<()> {
    use token_metrics::{HealthOpts, ModelFamily};

    let content = if let Some(path) = file {
        fs::read_to_string(path).with_context(|| format!("Reading {}", path.display()))?
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
    Ok(())
}
