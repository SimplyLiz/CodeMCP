use anyhow::{Context, Result};
use serde::{Deserialize, Serialize};
use std::collections::HashMap;

const UC_BASE_URL: &str = "https://api.ultracontext.ai";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UCMessage {
    #[serde(skip_serializing_if = "String::is_empty")]
    pub id: String,
    #[serde(skip_serializing_if = "is_zero")]
    pub index: usize,
    #[serde(default)]
    pub metadata: serde_json::Value,
    #[serde(flatten)]
    pub data: HashMap<String, serde_json::Value>,
}

fn is_zero(n: &usize) -> bool {
    *n == 0
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UCContext {
    #[serde(default)]
    pub id: String,
    #[serde(default)]
    pub version: u32,
    #[serde(default)]
    pub data: Vec<UCMessage>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub versions: Option<Vec<UCVersion>>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub metadata: Option<serde_json::Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub created_at: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UCVersion {
    pub version: u32,
    pub operation: String,
    pub affected: Option<Vec<String>>,
    #[serde(default)]
    pub timestamp: String,
}

#[derive(Debug, Clone)]
pub struct UCClient {
    api_key: String,
    client: reqwest::blocking::Client,
}

impl UCClient {
    pub fn new(api_key: String) -> Result<Self> {
        let client = reqwest::blocking::Client::builder()
            .timeout(std::time::Duration::from_secs(30))
            .build()?;

        Ok(Self { api_key, client })
    }

    /// Create a new context
    pub fn create_context(&self, from: Option<&str>, version: Option<u32>) -> Result<UCContext> {
        let mut body = HashMap::new();
        if let Some(from_id) = from {
            body.insert("from", serde_json::json!(from_id));
            if let Some(v) = version {
                body.insert("version", serde_json::json!(v));
            }
        }

        let response = self
            .client
            .post(&format!("{}/contexts", UC_BASE_URL))
            .bearer_auth(&self.api_key)
            .json(&body)
            .send()
            .context("Failed to create context")?;

        let status = response.status();
        let text = response.text().unwrap_or_default();

        if !status.is_success() {
            anyhow::bail!("UC API error ({}): {}\n\nThis might mean:\n1. The UC API is not yet publicly available\n2. Your API key needs activation\n3. The endpoint structure is different\n\nPlease contact UltraContext support or check https://ultracontext.ai/docs", status, text);
        }

        let ctx: UCContext =
            serde_json::from_str(&text).context("Failed to parse context response")?;
        Ok(ctx)
    }

    /// Get context with optional version and history
    pub fn get_context(
        &self,
        ctx_id: &str,
        version: Option<u32>,
        history: bool,
    ) -> Result<UCContext> {
        let mut url = format!("{}/contexts/{}", UC_BASE_URL, ctx_id);
        let mut params = vec![];

        if let Some(v) = version {
            params.push(format!("version={}", v));
        }
        if history {
            params.push("history=true".to_string());
        }

        if !params.is_empty() {
            url.push('?');
            url.push_str(&params.join("&"));
        }

        let response = self
            .client
            .get(&url)
            .bearer_auth(&self.api_key)
            .send()
            .context("Failed to get context")?;

        if !response.status().is_success() {
            let status = response.status();
            let text = response.text().unwrap_or_default();
            anyhow::bail!("UC API error ({}): {}", status, text);
        }

        let ctx: UCContext = response.json().context("Failed to parse context")?;
        Ok(ctx)
    }

    /// Append a message to context
    pub fn append(&self, ctx_id: &str, message: UCMessage) -> Result<UCContext> {
        let response = self
            .client
            .post(&format!("{}/contexts/{}", UC_BASE_URL, ctx_id))
            .bearer_auth(&self.api_key)
            .json(&message.data)
            .send()
            .context("Failed to append message")?;

        if !response.status().is_success() {
            let status = response.status();
            let text = response.text().unwrap_or_default();
            anyhow::bail!("UC API error ({}): {}", status, text);
        }

        let mut ctx: UCContext = response.json().context("Failed to parse append response")?;
        ctx.id = ctx_id.to_string();
        Ok(ctx)
    }

    /// Update a message by ID or index
    pub fn update(&self, ctx_id: &str, message: UCMessage) -> Result<UCContext> {
        let response = self
            .client
            .patch(&format!("{}/contexts/{}", UC_BASE_URL, ctx_id))
            .bearer_auth(&self.api_key)
            .json(&message)
            .send()
            .context("Failed to update message")?;

        if !response.status().is_success() {
            let status = response.status();
            let text = response.text().unwrap_or_default();
            anyhow::bail!("UC API error ({}): {}", status, text);
        }

        let ctx: UCContext = response.json().context("Failed to parse update response")?;
        Ok(ctx)
    }

    /// Delete a message by ID or index
    pub fn delete(&self, ctx_id: &str, id_or_index: &str) -> Result<UCContext> {
        let response = self
            .client
            .delete(&format!(
                "{}/contexts/{}/{}",
                UC_BASE_URL, ctx_id, id_or_index
            ))
            .bearer_auth(&self.api_key)
            .send()
            .context("Failed to delete message")?;

        if !response.status().is_success() {
            let status = response.status();
            let text = response.text().unwrap_or_default();
            anyhow::bail!("UC API error ({}): {}", status, text);
        }

        let ctx: UCContext = response.json().context("Failed to parse delete response")?;
        Ok(ctx)
    }

    /// Batch append multiple messages
    pub fn batch_append(&self, ctx_id: &str, messages: Vec<UCMessage>) -> Result<UCContext> {
        let mut ctx = self.get_context(ctx_id, None, false)?;

        for msg in messages {
            ctx = self.append(ctx_id, msg)?;
        }

        Ok(ctx)
    }

    /// List all contexts (if API supports it)
    pub fn list_contexts(&self) -> Result<Vec<UCContext>> {
        let response = self
            .client
            .get(&format!("{}/contexts", UC_BASE_URL))
            .bearer_auth(&self.api_key)
            .send()
            .context("Failed to list contexts")?;

        if !response.status().is_success() {
            let status = response.status();
            let text = response.text().unwrap_or_default();
            anyhow::bail!("UC API error ({}): {}", status, text);
        }

        let contexts: Vec<UCContext> = response.json().context("Failed to parse contexts list")?;
        Ok(contexts)
    }
}
