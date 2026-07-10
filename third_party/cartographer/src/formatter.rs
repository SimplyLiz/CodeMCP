use crate::memory::{FileEntry, Memory};
use tiktoken_rs::cl100k_base;

#[derive(Debug, Clone, Copy, Default)]
pub enum OutputTarget {
    #[default]
    Raw,
    Claude,
    Cursor,
}

impl std::str::FromStr for OutputTarget {
    type Err = String;

    fn from_str(s: &str) -> Result<Self, Self::Err> {
        match s.to_lowercase().as_str() {
            "raw" => Ok(Self::Raw),
            "claude" => Ok(Self::Claude),
            "cursor" => Ok(Self::Cursor),
            _ => Err(format!("Unknown target: {}. Use: raw, claude, cursor", s)),
        }
    }
}

pub trait Formatter {
    fn format(&self, memory: &Memory) -> String;
    fn extension(&self) -> &'static str;
}

pub struct RawFormatter;
pub struct ClaudeFormatter;
pub struct CursorFormatter;

/// Estimate token count using cl100k_base (GPT-4/Claude tokenizer)
pub fn estimate_tokens(text: &str) -> usize {
    cl100k_base()
        .map(|bpe| bpe.encode_with_special_tokens(text).len())
        .unwrap_or_else(|_| text.len() / 4) // Fallback: ~4 chars per token
}

/// Format token count for display
pub fn format_token_count(tokens: usize) -> String {
    if tokens >= 1_000_000 {
        format!("~{:.1}M tokens", tokens as f64 / 1_000_000.0)
    } else if tokens >= 1_000 {
        format!("~{:.1}k tokens", tokens as f64 / 1_000.0)
    } else {
        format!("~{} tokens", tokens)
    }
}

impl Formatter for RawFormatter {
    fn format(&self, memory: &Memory) -> String {
        serde_json::to_string_pretty(&memory.files).unwrap_or_default()
    }

    fn extension(&self) -> &'static str {
        "json"
    }
}

impl Formatter for ClaudeFormatter {
    fn format(&self, memory: &Memory) -> String {
        let mut out = String::new();
        out.push_str("<context>\n");
        out.push_str("<project_map>\n");

        let mut entries: Vec<&FileEntry> = memory.files.values().collect();
        entries.sort_by(|a, b| a.path.cmp(&b.path));

        for entry in entries {
            out.push_str(&format!("<file path=\"{}\">\n", escape_xml(&entry.path)));
            out.push_str(&escape_xml(&entry.content));
            if !entry.content.ends_with('\n') {
                out.push('\n');
            }
            out.push_str("</file>\n");
        }

        out.push_str("</project_map>\n");
        out.push_str("</context>");
        out
    }

    fn extension(&self) -> &'static str {
        "xml"
    }
}

impl Formatter for CursorFormatter {
    fn format(&self, memory: &Memory) -> String {
        let mut out = String::new();
        out.push_str("# Project Context\n\n");
        out.push_str("## File Structure\n\n");

        let mut entries: Vec<&FileEntry> = memory.files.values().collect();
        entries.sort_by(|a, b| a.path.cmp(&b.path));

        // Tree view
        for entry in &entries {
            out.push_str(&format!("- `{}`\n", entry.path));
        }

        out.push_str("\n## File Contents\n\n");

        for entry in entries {
            let ext = entry.path.rsplit('.').next().unwrap_or("txt");
            out.push_str(&format!("### {}\n\n```{}\n", entry.path, ext));
            out.push_str(&entry.content);
            if !entry.content.ends_with('\n') {
                out.push('\n');
            }
            out.push_str("```\n\n");
        }

        out
    }

    fn extension(&self) -> &'static str {
        "md"
    }
}

pub fn get_formatter(target: OutputTarget) -> Box<dyn Formatter> {
    match target {
        OutputTarget::Raw => Box::new(RawFormatter),
        OutputTarget::Claude => Box::new(ClaudeFormatter),
        OutputTarget::Cursor => Box::new(CursorFormatter),
    }
}

fn escape_xml(s: &str) -> String {
    s.replace('&', "&amp;")
        .replace('<', "&lt;")
        .replace('>', "&gt;")
        .replace('"', "&quot;")
}
