//! Environment doctor.
//!
//! CodeCartographer's core features (map, skeleton, search, health) are
//! self-contained, but a few capabilities shell out to external binaries:
//! `git` for history analysis, and `mmdc` / `dot` for rendering diagrams to
//! SVG/PNG. Those failures otherwise surface only when you run the feature and
//! hit a missing binary. `doctor` reports availability up front, with an
//! install hint for anything missing, so `install.sh` / `launch.py` can show
//! the user exactly what they still need.

// Some accessors are convenience API for future callers (e.g. an MCP
// `renderer_status` tool); they aren't all exercised by the binary yet.
#![allow(dead_code)]

use std::path::Path;

/// A single external tool CodeCartographer can use, and how to get it.
pub struct ToolCheck {
    pub name: &'static str,
    pub purpose: &'static str,
    pub install: &'static str,
    /// `true` when a whole feature area degrades without it (git intelligence);
    /// `false` for tools that only affect one optional output format.
    pub recommended: bool,
}

/// The external tools we probe, in report order. This is the single source of
/// truth for tool metadata — keep the install hints here in sync with the ones
/// `diagram_export` prints on a failed render.
pub fn external_tools() -> Vec<ToolCheck> {
    vec![
        ToolCheck {
            name: "git",
            purpose: "git intelligence — hotspots, cochange, semidiff, churn, ownership",
            install: "https://git-scm.com/downloads",
            recommended: true,
        },
        ToolCheck {
            name: "mmdc",
            purpose: "export Mermaid diagrams to SVG/PNG (diagram -o out.svg)",
            install: "npm install -g @mermaid-js/mermaid-cli",
            recommended: false,
        },
        ToolCheck {
            name: "dot",
            purpose: "export DOT diagrams to SVG/PNG (diagram --format dot -o out.png)",
            install: "brew install graphviz",
            recommended: false,
        },
    ]
}

/// True if `bin` is an executable on `$PATH`. Side-effect free — scans `$PATH`
/// rather than spawning the tool, so it can't hang and has no output.
pub fn is_on_path(bin: &str) -> bool {
    let Some(paths) = std::env::var_os("PATH") else {
        return false;
    };
    std::env::split_paths(&paths).any(|dir| {
        let candidate = dir.join(bin);
        if is_executable(&candidate) {
            return true;
        }
        // Windows: the launcher may be `git.exe`, `mmdc.cmd`, etc.
        ["exe", "cmd", "bat"]
            .iter()
            .any(|ext| is_executable(&candidate.with_extension(ext)))
    })
}

fn is_executable(p: &Path) -> bool {
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        std::fs::metadata(p)
            .map(|m| m.is_file() && m.permissions().mode() & 0o111 != 0)
            .unwrap_or(false)
    }
    #[cfg(not(unix))]
    {
        p.is_file()
    }
}

/// Outcome of a doctor run, so callers (a `--strict` CI gate, install scripts)
/// can decide what a missing tool means for them.
pub struct DoctorReport {
    pub missing_recommended: usize,
    pub missing_optional: usize,
}

/// Run every check and print a human-readable report. Returns the missing
/// counts; printing is a side effect so this doubles as the CLI handler body.
pub fn run() -> DoctorReport {
    println!("CodeCartographer — environment check\n");

    let mut missing_recommended = 0;
    let mut missing_optional = 0;

    for tool in external_tools() {
        let found = is_on_path(tool.name);
        // ✓ present · ✗ missing recommended · ○ missing optional
        let mark = if found {
            "✓"
        } else if tool.recommended {
            "✗"
        } else {
            "○"
        };
        println!("  {mark} {:<5}  {}", tool.name, tool.purpose);
        if !found {
            println!("        └ install: {}", tool.install);
            if tool.recommended {
                missing_recommended += 1;
            } else {
                missing_optional += 1;
            }
        }
    }

    println!();
    if missing_recommended == 0 && missing_optional == 0 {
        println!("✅ All external tools present.");
    } else {
        println!(
            "{missing_recommended} recommended and {missing_optional} optional tool(s) missing (see install hints above)."
        );
        println!("   Core features — map, skeleton, search, health — work without them.");
    }

    DoctorReport {
        missing_recommended,
        missing_optional,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn tool_list_is_populated_and_consistent() {
        let tools = external_tools();
        assert!(tools.iter().any(|t| t.name == "mmdc"));
        assert!(tools.iter().any(|t| t.name == "dot"));
        assert!(tools.iter().any(|t| t.name == "git" && t.recommended));
        // Every tool must carry a non-empty install hint.
        assert!(tools.iter().all(|t| !t.install.is_empty()));
    }

    #[test]
    fn is_on_path_finds_a_ubiquitous_binary_and_rejects_nonsense() {
        // `sh` exists on every unix CI runner; the random name never does.
        #[cfg(unix)]
        assert!(is_on_path("sh"));
        assert!(!is_on_path("codecartographer_no_such_binary_zzz"));
    }
}
