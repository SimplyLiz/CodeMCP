use anyhow::Result;
use std::collections::HashSet;
use std::fs;
use std::path::{Path, PathBuf};
use walkdir::WalkDir;

pub const IGNORED_DIRS: &[&str] = &[
    "node_modules",
    ".git",
    "target",
    "dist",
    "vendor",
    // Vendored / bundled third-party source. These are checked into many repos
    // (e.g. Godot's `thirdparty/`) so .gitignore never excludes them, yet they
    // dominate every structural metric with code the team does not own.
    "thirdparty",
    "third_party",
    "third-party",
    "3rdparty",
    "external",
    "externals",
    ".next",
    "build",
    "out",
    ".env",
    "__pycache__",
    ".venv",
    "venv",
    ".idea",
    ".vscode",
    "coverage",
    ".nuxt",
];

// =============================================================================
// CONTEXT SAFETY: "Villain" Hard-Coded Noise Blacklist
// These files are universally noise for LLMs - they burn tokens and crash context
// =============================================================================

/// Lock files - The #1 enemy of context windows
pub const NOISE_LOCK_FILES: &[&str] = &[
    // JavaScript ecosystem
    "package-lock.json",
    "yarn.lock",
    "pnpm-lock.yaml",
    "bun.lockb",
    // Rust
    "Cargo.lock",
    // Python
    "poetry.lock",
    "Pipfile.lock",
    // Ruby
    "Gemfile.lock",
    // PHP
    "composer.lock",
    // .NET
    "packages.lock.json",
    // Go
    "go.sum",
];

/// Log file extensions
pub const NOISE_LOG_EXTENSIONS: &[&str] = &["log"];

/// Source map extensions
pub const NOISE_MAP_EXTENSIONS: &[&str] = &["map"];

/// Minified JS pattern suffix
pub const NOISE_MINIFIED_JS_SUFFIX: &str = ".min.js";

/// Minified CSS pattern suffix
pub const NOISE_MINIFIED_CSS_SUFFIX: &str = ".min.css";

/// Binary/image extensions to always ignore
pub const NOISE_BINARY_EXTENSIONS: &[&str] = &["png", "jpg", "jpeg", "ico", "gif", "webp", "bmp"];

/// Directories that are pure noise (already in IGNORED_DIRS but explicit for reporting)
#[allow(dead_code)]
pub const NOISE_DIRS: &[&str] = &["dist", "build", ".next", "out", "target"];

/// SVG size threshold in bytes (only ignore if > 2KB)
pub const SVG_SIZE_THRESHOLD: u64 = 2048;

/// Represents a file that was ignored due to noise filtering
#[derive(Debug, Clone)]
pub struct IgnoredFile {
    #[allow(dead_code)]
    pub path: String,
    #[allow(dead_code)]
    pub reason: NoiseReason,
    pub estimated_tokens: usize,
}

/// Why a file was ignored
#[derive(Debug, Clone)]
pub enum NoiseReason {
    LockFile,
    LogFile,
    SourceMap,
    MinifiedJs,
    MinifiedCss,
    BinaryImage,
    #[allow(dead_code)]
    LargeSvg(u64), // size in bytes
    #[allow(dead_code)]
    NoiseDirectory(String),
}

impl NoiseReason {
    #[allow(dead_code)]
    pub fn description(&self) -> String {
        match self {
            NoiseReason::LockFile => "Lock file".to_string(),
            NoiseReason::LogFile => "Log file".to_string(),
            NoiseReason::SourceMap => "Source map".to_string(),
            NoiseReason::MinifiedJs => "Minified JS".to_string(),
            NoiseReason::MinifiedCss => "Minified CSS".to_string(),
            NoiseReason::BinaryImage => "Binary image".to_string(),
            NoiseReason::LargeSvg(size) => format!("Large SVG ({}KB)", size / 1024),
            NoiseReason::NoiseDirectory(dir) => format!("In {} folder", dir),
        }
    }
}

/// Result of scanning with noise tracking
#[derive(Debug, Default)]
pub struct ScanResult {
    pub files: Vec<PathBuf>,
    pub ignored_noise: Vec<IgnoredFile>,
}

// SECURITY: Hard-blocked files - NEVER include these
pub const IGNORED_FILES: &[&str] = &[
    // System
    ".DS_Store",
    // Secrets & credentials
    ".env",
    ".env.local",
    ".env.production",
    ".env.development",
    "id_rsa",
    "id_rsa.pub",
    "id_ed25519",
    "id_ed25519.pub",
    "id_dsa",
    "*.pem",
    "*.key",
    "*.p12",
    "*.pfx",
    "secrets.yaml",
    "secrets.yml",
    "secrets.json",
    ".npmrc",
    ".pypirc",
    "credentials",
    "credentials.json",
    "service-account.json",
    // Output files
    "context.xml",
    "context.json",
    "context.md",
    "codecartographer_map.xml",
    "codecartographer_map.md",
    "codecartographer_map.json",
    ".codecartographer_memory.json",
    ".codecartographer_memory.json",
    // CodeCartographer runtime state files
    ".codecartographer_cache.json",
    ".codecartographer_cache.json",
    ".codecartographer_watch_state.json",
    ".codecartographer_history.json",
];

// Patterns for extension-based blocking
pub const BLOCKED_EXTENSIONS: &[&str] = &["pem", "key", "p12", "pfx", "jks", "keystore"];

pub const BLOCKED_PATTERNS: &[&str] = &[
    "id_rsa",
    "id_dsa",
    "id_ed25519",
    "id_ecdsa",
    "aws_access",
    "aws_secret",
    "credentials",
];

// =============================================================================
// .codecartographerignore support
// =============================================================================

/// A compiled pattern from .codecartographerignore
pub struct CodeCartographerIgnorePattern {
    pub negate: bool,
    filename_only: bool,
    regex: regex::Regex,
}

impl CodeCartographerIgnorePattern {
    fn parse(line: &str) -> Option<Self> {
        let line = line.trim();
        if line.is_empty() || line.starts_with('#') {
            return None;
        }
        let (negate, pattern) = if let Some(rest) = line.strip_prefix('!') {
            (true, rest)
        } else {
            (false, line)
        };
        // No '/' in pattern (ignoring trailing slash) → match filename only
        let filename_only = !pattern.trim_end_matches('/').contains('/');
        let pattern = pattern.trim_end_matches('/');
        let regex = glob_to_regex(pattern)?;
        Some(Self {
            negate,
            filename_only,
            regex,
        })
    }

    fn matches(&self, rel_path: &str) -> bool {
        if self.filename_only {
            let name = rel_path.rsplit('/').next().unwrap_or(rel_path);
            self.regex.is_match(name)
        } else {
            self.regex.is_match(rel_path)
        }
    }
}

fn glob_to_regex(pattern: &str) -> Option<regex::Regex> {
    let mut out = String::from("^");
    let chars: Vec<char> = pattern.chars().collect();
    let mut i = 0;
    while i < chars.len() {
        match chars[i] {
            '*' if i + 1 < chars.len() && chars[i + 1] == '*' => {
                out.push_str(".*");
                i += 2;
                if i < chars.len() && chars[i] == '/' {
                    i += 1;
                }
            }
            '*' => {
                out.push_str("[^/]*");
                i += 1;
            }
            '?' => {
                out.push_str("[^/]");
                i += 1;
            }
            '.' | '+' | '^' | '$' | '{' | '}' | '(' | ')' | '|' | '[' | ']' | '\\' => {
                out.push('\\');
                out.push(chars[i]);
                i += 1;
            }
            c => {
                out.push(c);
                i += 1;
            }
        }
    }
    out.push('$');
    regex::Regex::new(&out).ok()
}

/// Load .codecartographerignore from the repo root.
pub fn load_codecartographer_ignore(root: &Path) -> Vec<CodeCartographerIgnorePattern> {
    let path = root.join(".codecartographerignore");
    fs::read_to_string(path)
        .unwrap_or_default()
        .lines()
        .filter_map(CodeCartographerIgnorePattern::parse)
        .collect()
}

/// Load .gitignore from the repo root.  Same pattern syntax — reuses the
/// existing CodeCartographerIgnorePattern parser since .gitignore is compatible.
pub fn load_gitignore(root: &Path) -> Vec<CodeCartographerIgnorePattern> {
    let path = root.join(".gitignore");
    fs::read_to_string(path)
        .unwrap_or_default()
        .lines()
        .filter_map(CodeCartographerIgnorePattern::parse)
        .collect()
}

fn is_codecartographer_ignored(rel_path: &str, patterns: &[CodeCartographerIgnorePattern]) -> bool {
    let mut ignored = false;
    for p in patterns {
        if p.matches(rel_path) {
            ignored = !p.negate;
        }
    }
    ignored
}

// =============================================================================
// Source-file classification
// =============================================================================

/// Extensions that indicate analysable source code.  Files whose extension is
/// not in this list (e.g. `.md`, `.toml`, `LICENSE`, `Makefile`) are excluded
/// from structural analysis commands (hotspots, dead, simulate, health, check).
#[allow(dead_code)]
pub const SOURCE_EXTENSIONS: &[&str] = &[
    "rs", "py", "js", "mjs", "cjs", "ts", "tsx", "jsx",
    "go", "java", "kt", "kts", "swift", "c", "cc", "cpp", "cxx",
    "h", "hpp", "cs", "rb", "php", "scala", "clj", "cljs",
    "elm", "hs", "ml", "mli", "ex", "exs", "lua", "r", "dart",
    "vue", "svelte", "zig", "nim", "cr",
];

#[allow(dead_code)]
pub fn is_source_file(path: &Path) -> bool {
    path.extension()
        .and_then(|e| e.to_str())
        .map(|ext| {
            let lower = ext.to_lowercase();
            SOURCE_EXTENSIONS.contains(&lower.as_str())
        })
        .unwrap_or(false)
}

// =============================================================================

/// Legacy function for backward compatibility
#[allow(dead_code)]
pub fn scan_files(root: &Path) -> Result<Vec<PathBuf>> {
    let result = scan_files_with_noise_tracking(root)?;
    Ok(result.files)
}

/// Scan files with noise tracking - returns both clean files and ignored noise
pub fn scan_files_with_noise_tracking(root: &Path) -> Result<ScanResult> {
    let ignored_dirs: HashSet<&str> = IGNORED_DIRS.iter().copied().collect();
    let ignored_files: HashSet<&str> = IGNORED_FILES.iter().copied().collect();
    // .gitignore patterns are processed first; .codecartographerignore is appended
    // so its rules (including negations) take precedence over .gitignore.
    let mut ignore_patterns = load_gitignore(root);
    ignore_patterns.extend(load_codecartographer_ignore(root));

    let mut result = ScanResult::default();

    let all_entries: Vec<_> = WalkDir::new(root)
        .into_iter()
        .filter_entry(|e| !should_skip_dir(e, &ignored_dirs))
        .filter_map(|e| e.ok())
        .filter(|e| e.file_type().is_file())
        .collect();

    for entry in all_entries {
        let path = entry.path();

        // Check security blocks first (these are never included, not even reported)
        if is_blocked_file(path, &ignored_files) {
            continue;
        }

        // Check .gitignore + .codecartographerignore patterns (silently excluded)
        let rel = path
            .strip_prefix(root)
            .unwrap_or(path)
            .to_string_lossy()
            .replace('\\', "/");
        if is_codecartographer_ignored(&rel, &ignore_patterns) {
            continue;
        }

        // Check noise patterns
        if let Some(ignored) = check_noise_file(path, root) {
            result.ignored_noise.push(ignored);
            continue;
        }

        result.files.push(entry.into_path());
    }

    result.files.sort();
    result
        .ignored_noise
        .sort_by(|a, b| b.estimated_tokens.cmp(&a.estimated_tokens));
    Ok(result)
}

/// Check if a file is noise and should be ignored (but reported)
fn check_noise_file(path: &Path, root: &Path) -> Option<IgnoredFile> {
    let filename = path.file_name().and_then(|n| n.to_str()).unwrap_or("");
    let rel_path = path
        .strip_prefix(root)
        .unwrap_or(path)
        .to_string_lossy()
        .replace('\\', "/");

    // Check lock files
    if NOISE_LOCK_FILES.contains(&filename) {
        let tokens = estimate_file_tokens(path);
        return Some(IgnoredFile {
            path: rel_path,
            reason: NoiseReason::LockFile,
            estimated_tokens: tokens,
        });
    }

    // Check extension-based noise
    if let Some(ext) = path.extension().and_then(|e| e.to_str()) {
        let ext_lower = ext.to_lowercase();

        // Binary images - always ignore
        if NOISE_BINARY_EXTENSIONS.contains(&ext_lower.as_str()) {
            return Some(IgnoredFile {
                path: rel_path,
                reason: NoiseReason::BinaryImage,
                estimated_tokens: 0,
            });
        }

        // Log files
        if NOISE_LOG_EXTENSIONS.contains(&ext_lower.as_str()) {
            let tokens = estimate_file_tokens(path);
            return Some(IgnoredFile {
                path: rel_path,
                reason: NoiseReason::LogFile,
                estimated_tokens: tokens,
            });
        }

        // Source maps
        if NOISE_MAP_EXTENSIONS.contains(&ext_lower.as_str()) {
            let tokens = estimate_file_tokens(path);
            return Some(IgnoredFile {
                path: rel_path,
                reason: NoiseReason::SourceMap,
                estimated_tokens: tokens,
            });
        }

        // Large SVGs (> 2KB)
        if ext_lower == "svg" {
            if let Ok(metadata) = fs::metadata(path) {
                let size = metadata.len();
                if size > SVG_SIZE_THRESHOLD {
                    let tokens = estimate_file_tokens(path);
                    return Some(IgnoredFile {
                        path: rel_path,
                        reason: NoiseReason::LargeSvg(size),
                        estimated_tokens: tokens,
                    });
                }
            }
        }
    }

    // Check minified JS
    if filename.ends_with(NOISE_MINIFIED_JS_SUFFIX) {
        let tokens = estimate_file_tokens(path);
        return Some(IgnoredFile {
            path: rel_path,
            reason: NoiseReason::MinifiedJs,
            estimated_tokens: tokens,
        });
    }

    // Check minified CSS
    if filename.ends_with(NOISE_MINIFIED_CSS_SUFFIX) {
        let tokens = estimate_file_tokens(path);
        return Some(IgnoredFile {
            path: rel_path,
            reason: NoiseReason::MinifiedCss,
            estimated_tokens: tokens,
        });
    }

    None
}

/// Estimate tokens for a file (rough: ~4 chars per token)
fn estimate_file_tokens(path: &Path) -> usize {
    fs::metadata(path)
        .map(|m| (m.len() as usize) / 4)
        .unwrap_or(0)
}

fn is_blocked_file(path: &Path, ignored_files: &HashSet<&str>) -> bool {
    let filename = path.file_name().and_then(|n| n.to_str()).unwrap_or("");

    // Direct filename match
    if ignored_files.contains(filename) {
        return true;
    }

    // Extension-based blocking
    if let Some(ext) = path.extension().and_then(|e| e.to_str()) {
        if BLOCKED_EXTENSIONS.contains(&ext) {
            return true;
        }
    }

    // Pattern-based blocking (contains check)
    let lower = filename.to_lowercase();
    for pattern in BLOCKED_PATTERNS {
        if lower.contains(pattern) {
            return true;
        }
    }

    false
}

fn should_skip_dir(entry: &walkdir::DirEntry, ignored: &HashSet<&str>) -> bool {
    entry
        .file_name()
        .to_str()
        .map(|s| ignored.contains(s))
        .unwrap_or(false)
}

pub fn is_ignored_path(path: &Path) -> bool {
    path.components().any(|c| {
        c.as_os_str()
            .to_str()
            .map(|s| IGNORED_DIRS.contains(&s) || IGNORED_FILES.contains(&s))
            .unwrap_or(false)
    })
}
