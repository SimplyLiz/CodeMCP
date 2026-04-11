use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WebhookPayload {
    pub event: String,
    pub context_id: String,
    pub version: u32,
    pub timestamp: String,
    pub changes: ContextChanges,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContextChanges {
    pub added: Vec<String>,
    pub modified: Vec<String>,
    pub deleted: Vec<String>,
    pub total_files: usize,
}

pub struct WebhookService {
    client: reqwest::blocking::Client,
}

impl WebhookService {
    pub fn new() -> Result<Self> {
        let client = reqwest::blocking::Client::builder()
            .timeout(std::time::Duration::from_secs(10))
            .build()?;

        Ok(Self { client })
    }

    /// Notify a single agent via webhook
    pub fn notify_agent(&self, webhook_url: &str, payload: &WebhookPayload) -> Result<()> {
        let response = self.client.post(webhook_url).json(payload).send()?;

        if !response.status().is_success() {
            let status = response.status();
            let text = response.text().unwrap_or_default();
            anyhow::bail!("Webhook failed ({}): {}", status, text);
        }

        Ok(())
    }

    /// Notify all agents with webhooks
    pub fn notify_all(
        &self,
        agents: &[crate::uc_agents::AgentConfig],
        payload: &WebhookPayload,
    ) -> Vec<Result<()>> {
        agents
            .iter()
            .filter(|a| a.enabled && a.webhook_url.is_some())
            .map(|agent| {
                let url = agent.webhook_url.as_ref().unwrap();
                self.notify_agent(url, payload)
                    .map_err(|e| anyhow::anyhow!("Agent '{}' webhook failed: {}", agent.name, e))
            })
            .collect()
    }

    /// Create payload from sync operation
    pub fn create_payload(
        context_id: &str,
        version: u32,
        added: Vec<String>,
        modified: Vec<String>,
        deleted: Vec<String>,
        total_files: usize,
    ) -> WebhookPayload {
        WebhookPayload {
            event: "context.updated".to_string(),
            context_id: context_id.to_string(),
            version,
            timestamp: chrono::Utc::now().to_rfc3339(),
            changes: ContextChanges {
                added,
                modified,
                deleted,
                total_files,
            },
        }
    }
}

/// Agent-specific context format
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentContext {
    pub context_id: String,
    pub version: u32,
    pub files: HashMap<String, AgentFile>,
    pub metadata: HashMap<String, serde_json::Value>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentFile {
    pub path: String,
    pub content: String,
    pub language: Option<String>,
    pub size: usize,
}

impl AgentContext {
    /// Convert CMP memory to agent-friendly format
    pub fn from_memory(memory: &crate::memory::Memory, context_id: &str) -> Self {
        let files = memory
            .files
            .iter()
            .map(|(path, entry)| {
                let language = Self::detect_language(&entry.path);
                let file = AgentFile {
                    path: entry.path.clone(),
                    content: entry.content.clone(),
                    language,
                    size: entry.content.len(),
                };
                (path.clone(), file)
            })
            .collect();

        Self {
            context_id: context_id.to_string(),
            version: memory.version,
            files,
            metadata: HashMap::new(),
        }
    }

    fn detect_language(path: &str) -> Option<String> {
        let ext = path.rsplit('.').next()?;
        let lang = match ext {
            "rs" => "rust",
            "py" => "python",
            "js" => "javascript",
            "ts" => "typescript",
            "go" => "go",
            "java" => "java",
            "cpp" | "cc" | "cxx" => "cpp",
            "c" => "c",
            "rb" => "ruby",
            "php" => "php",
            "swift" => "swift",
            "kt" => "kotlin",
            "cs" => "csharp",
            "md" => "markdown",
            "json" => "json",
            "yaml" | "yml" => "yaml",
            "toml" => "toml",
            "xml" => "xml",
            "html" => "html",
            "css" => "css",
            "sh" => "shell",
            _ => return None,
        };
        Some(lang.to_string())
    }

    /// Export as JSON for agents
    pub fn to_json(&self) -> Result<String> {
        Ok(serde_json::to_string_pretty(self)?)
    }

    /// Export as markdown for agents
    pub fn to_markdown(&self) -> String {
        let mut md = format!("# Context: {}\n\n", self.context_id);
        md.push_str(&format!("Version: {}\n", self.version));
        md.push_str(&format!("Total Files: {}\n\n", self.files.len()));

        md.push_str("## Files\n\n");
        let mut paths: Vec<_> = self.files.keys().collect();
        paths.sort();

        for path in paths {
            let file = &self.files[path];
            let lang = file.language.as_deref().unwrap_or("text");
            md.push_str(&format!("### {}\n\n", file.path));
            md.push_str(&format!("```{}\n{}\n```\n\n", lang, file.content));
        }

        md
    }
}
