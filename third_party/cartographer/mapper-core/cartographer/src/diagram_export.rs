//! Render a diagram to SVG/PNG by shelling out to an external converter.
//!
//! We pick the converter based on the *source* format (Mermaid vs DOT) — not
//! on a user-visible flag — so callers just say "write to foo.svg" and we do
//! the right thing:
//!
//!   Mermaid + .svg/.png  → `mmdc` (Mermaid CLI, npm-installed)
//!   DOT     + .svg/.png  → `dot`  (Graphviz binary)
//!
//! If the target extension isn't `.svg`/`.png`, we treat the write as a
//! passthrough — the caller's diagram text lands at `target` unchanged.

use std::io::Write;
use std::path::Path;
use std::process::{Command, Stdio};

use crate::diagram::DiagramFormat;

/// What `export_diagram` did, so the CLI can print a matching status line.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ExportKind {
    /// Wrote the diagram source straight to disk (no converter invoked).
    Source,
    /// Rendered via `mmdc` (Mermaid → SVG or PNG).
    MermaidSvg,
    MermaidPng,
    /// Rendered via Graphviz `dot`.
    DotSvg,
    DotPng,
}

/// Write `content` (diagram source in `source_format`) to `target`, converting
/// to SVG/PNG on the way if the target extension calls for it.
///
/// Errors if a converter is needed but the binary is missing from `$PATH` —
/// returning a message that tells the user how to install it.
pub fn export_diagram(
    content: &str,
    source_format: DiagramFormat,
    target: &Path,
) -> Result<ExportKind, String> {
    let ext = target
        .extension()
        .and_then(|e| e.to_str())
        .map(|s| s.to_lowercase())
        .unwrap_or_default();

    match (source_format, ext.as_str()) {
        (_, "svg") | (_, "png") => convert(content, source_format, target, &ext),
        _ => {
            std::fs::write(target, content).map_err(|e| e.to_string())?;
            Ok(ExportKind::Source)
        }
    }
}

fn convert(
    content: &str,
    source_format: DiagramFormat,
    target: &Path,
    ext: &str,
) -> Result<ExportKind, String> {
    match source_format {
        DiagramFormat::Mermaid => export_mermaid(content, target, ext),
        DiagramFormat::Dot => export_dot(content, target, ext),
        // ASCII trees are text-only by design — there's no sensible converter
        // that turns them into a raster/vector. Tell the user to pick a
        // different format explicitly instead of silently writing the text.
        DiagramFormat::Ascii => Err(
            "ASCII diagrams can't be rendered to .svg/.png — use `--format mermaid` or `--format dot` for image output, or write to a text extension.".to_string(),
        ),
    }
}

fn export_mermaid(content: &str, target: &Path, ext: &str) -> Result<ExportKind, String> {
    // mmdc reads from a file and writes to a path; it can't read stdin.
    let tmp = tempfile(".mmd")?;
    std::fs::write(&tmp, content).map_err(|e| e.to_string())?;

    let status = Command::new("mmdc")
        .args([
            "-i",
            tmp.to_str().ok_or("tempfile path not UTF-8")?,
            "-o",
            target.to_str().ok_or("target path not UTF-8")?,
        ])
        .status()
        .map_err(|_| {
            "`mmdc` not found on PATH. Install via `npm install -g @mermaid-js/mermaid-cli`."
                .to_string()
        })?;

    // Remove the tmp regardless of outcome — leaving `.mmd` files around on
    // failure mostly confuses users; the error message below is enough.
    let _ = std::fs::remove_file(&tmp);

    if !status.success() {
        return Err(format!("mmdc exited with status {}", status));
    }
    Ok(if ext == "png" { ExportKind::MermaidPng } else { ExportKind::MermaidSvg })
}

fn export_dot(content: &str, target: &Path, ext: &str) -> Result<ExportKind, String> {
    // `dot` accepts stdin — no tempfile needed.
    let mut child = Command::new("dot")
        .args(["-T", ext, "-o", target.to_str().ok_or("target path not UTF-8")?])
        .stdin(Stdio::piped())
        .spawn()
        .map_err(|_| {
            "`dot` not found on PATH. Install Graphviz (e.g. `brew install graphviz`).".to_string()
        })?;

    {
        let stdin = child
            .stdin
            .as_mut()
            .ok_or_else(|| "could not open dot stdin".to_string())?;
        stdin
            .write_all(content.as_bytes())
            .map_err(|e| e.to_string())?;
    }

    let status = child.wait().map_err(|e| e.to_string())?;
    if !status.success() {
        return Err(format!("dot exited with status {}", status));
    }
    Ok(if ext == "png" { ExportKind::DotPng } else { ExportKind::DotSvg })
}

/// Allocate a unique path in the system temp dir with the given extension.
/// We avoid pulling in the `tempfile` crate for one call — the path is used
/// immediately and removed in the happy path.
fn tempfile(ext: &str) -> Result<std::path::PathBuf, String> {
    let mut path = std::env::temp_dir();
    let pid = std::process::id();
    let nanos = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .map(|d| d.as_nanos())
        .unwrap_or(0);
    path.push(format!("cartographer-{}-{}{}", pid, nanos, ext));
    Ok(path)
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn passthrough_for_non_image_extension() {
        let dir = std::env::temp_dir();
        let target = dir.join(format!("cartographer-test-{}.mmd", std::process::id()));
        let kind = export_diagram("graph TD\n    A --> B", DiagramFormat::Mermaid, &target).unwrap();
        assert_eq!(kind, ExportKind::Source);
        let written = std::fs::read_to_string(&target).unwrap();
        assert!(written.contains("graph TD"));
        let _ = std::fs::remove_file(&target);
    }

    #[test]
    fn passthrough_for_dot_source_with_dot_extension() {
        let dir = std::env::temp_dir();
        let target = dir.join(format!("cartographer-test-{}.dot", std::process::id()));
        let kind = export_diagram("digraph G { A -> B }", DiagramFormat::Dot, &target).unwrap();
        assert_eq!(kind, ExportKind::Source);
        let _ = std::fs::remove_file(&target);
    }

    // mmdc / dot may not be installed in CI, so we don't drive actual
    // conversion in unit tests. The shell-out paths are exercised manually.
}
