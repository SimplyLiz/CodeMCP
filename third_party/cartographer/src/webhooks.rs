// Webhook Service - Handles webhook notifications for project graph updates
// This allows external services to react to changes in the project graph in real-time
#![allow(dead_code)]

use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::sync::Mutex;
use std::time::{SystemTime, UNIX_EPOCH};

/// Webhook event types
#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq, Hash)]
pub enum WebhookEvent {
    GraphUpdated,
    ModuleChanged,
    DependenciesChanged,
}

impl WebhookEvent {
    pub fn as_str(&self) -> &str {
        match self {
            WebhookEvent::GraphUpdated => "graph_updated",
            WebhookEvent::ModuleChanged => "module_changed",
            WebhookEvent::DependenciesChanged => "dependencies_changed",
        }
    }
}

/// Webhook configuration
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Webhook {
    pub id: String,
    pub url: String,
    pub events: Vec<WebhookEvent>,
    pub enabled: bool,
    pub created_at: u64,
    pub last_triggered: Option<u64>,
}

impl Webhook {
    pub fn new(url: String, events: Vec<WebhookEvent>) -> Self {
        Self {
            id: generate_webhook_id(),
            url,
            events,
            enabled: true,
            created_at: current_timestamp(),
            last_triggered: None,
        }
    }
}

/// Webhook payload for notifications
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WebhookPayload {
    pub event: String,
    pub timestamp: u64,
    pub data: WebhookData,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(untagged)]
pub enum WebhookData {
    GraphUpdated(GraphUpdatedData),
    ModuleChanged(ModuleChangedData),
    DependenciesChanged(DependenciesChangedData),
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct GraphUpdatedData {
    pub total_files: usize,
    pub total_edges: usize,
    pub affected_modules: Vec<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ModuleChangedData {
    pub module_id: String,
    pub path: String,
    pub change_type: String,
    pub signature_count: usize,
    pub risk_level: Option<String>,
    pub is_bridge: Option<bool>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct DependenciesChangedData {
    pub module_id: String,
    pub added_dependencies: Vec<String>,
    pub removed_dependencies: Vec<String>,
}

/// Webhook service state
pub struct WebhookService {
    webhooks: Mutex<HashMap<String, Webhook>>,
    delivery_history: Mutex<Vec<WebhookDelivery>>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct WebhookDelivery {
    pub webhook_id: String,
    pub url: String,
    pub event: String,
    pub payload: String,
    pub status: WebhookDeliveryStatus,
    pub attempted_at: u64,
    pub response_code: Option<u16>,
    pub error: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq, Eq)]
pub enum WebhookDeliveryStatus {
    Pending,
    Success,
    Failed,
    RetryScheduled,
}

impl WebhookService {
    pub fn new() -> Self {
        Self {
            webhooks: Mutex::new(HashMap::new()),
            delivery_history: Mutex::new(Vec::new()),
        }
    }

    pub fn register_webhook(
        &self,
        url: String,
        events: Vec<WebhookEvent>,
    ) -> Result<Webhook, String> {
        let webhook = Webhook::new(url.clone(), events);
        let id = webhook.id.clone();

        let mut webhooks = self.webhooks.lock().map_err(|e| e.to_string())?;

        if webhooks.contains_key(&url) {
            return Err(format!("Webhook with URL {} already exists", url));
        }

        webhooks.insert(id, webhook.clone());
        Ok(webhook)
    }

    pub fn unregister_webhook(&self, webhook_id: &str) -> Result<(), String> {
        let mut webhooks = self.webhooks.lock().map_err(|e| e.to_string())?;

        if webhooks.remove(webhook_id).is_none() {
            return Err(format!("Webhook not found: {}", webhook_id));
        }

        Ok(())
    }

    pub fn list_webhooks(&self) -> Result<Vec<Webhook>, String> {
        let webhooks = self.webhooks.lock().map_err(|e| e.to_string())?;
        Ok(webhooks.values().cloned().collect())
    }

    pub fn get_webhook(&self, webhook_id: &str) -> Result<Webhook, String> {
        let webhooks = self.webhooks.lock().map_err(|e| e.to_string())?;
        webhooks
            .get(webhook_id)
            .cloned()
            .ok_or_else(|| format!("Webhook not found: {}", webhook_id))
    }

    pub fn enable_webhook(&self, webhook_id: &str) -> Result<(), String> {
        let mut webhooks = self.webhooks.lock().map_err(|e| e.to_string())?;

        let webhook = webhooks
            .get_mut(webhook_id)
            .ok_or_else(|| format!("Webhook not found: {}", webhook_id))?;

        webhook.enabled = true;
        Ok(())
    }

    pub fn disable_webhook(&self, webhook_id: &str) -> Result<(), String> {
        let mut webhooks = self.webhooks.lock().map_err(|e| e.to_string())?;

        let webhook = webhooks
            .get_mut(webhook_id)
            .ok_or_else(|| format!("Webhook not found: {}", webhook_id))?;

        webhook.enabled = false;
        Ok(())
    }

    pub fn notify_graph_updated(
        &self,
        total_files: usize,
        total_edges: usize,
        affected_modules: Vec<String>,
    ) -> Vec<Result<(), String>> {
        let data = WebhookData::GraphUpdated(GraphUpdatedData {
            total_files,
            total_edges,
            affected_modules,
        });
        self.notify(WebhookEvent::GraphUpdated, data)
    }

    pub fn notify_module_changed(
        &self,
        module_id: String,
        path: String,
        change_type: &str,
        signature_count: usize,
        risk_level: Option<String>,
        is_bridge: Option<bool>,
    ) -> Vec<Result<(), String>> {
        let data = WebhookData::ModuleChanged(ModuleChangedData {
            module_id,
            path,
            change_type: change_type.to_string(),
            signature_count,
            risk_level,
            is_bridge,
        });
        self.notify(WebhookEvent::ModuleChanged, data)
    }

    pub fn notify_dependencies_changed(
        &self,
        module_id: String,
        added: Vec<String>,
        removed: Vec<String>,
    ) -> Vec<Result<(), String>> {
        let data = WebhookData::DependenciesChanged(DependenciesChangedData {
            module_id,
            added_dependencies: added,
            removed_dependencies: removed,
        });
        self.notify(WebhookEvent::DependenciesChanged, data)
    }

    fn notify(&self, event: WebhookEvent, data: WebhookData) -> Vec<Result<(), String>> {
        let webhooks = match self.webhooks.lock() {
            Ok(h) => h,
            Err(e) => return vec![Err(e.to_string())],
        };

        let payload = WebhookPayload {
            event: event.as_str().to_string(),
            timestamp: current_timestamp(),
            data,
        };

        let payload_json = serde_json::to_string(&payload).unwrap_or_default();
        let mut results = Vec::new();

        for webhook in webhooks.values() {
            if !webhook.enabled {
                continue;
            }

            if !webhook.events.contains(&event) {
                continue;
            }

            let result = self.deliver_webhook(webhook, &payload_json);
            results.push(result);
        }

        results
    }

    fn deliver_webhook(&self, webhook: &Webhook, payload: &str) -> Result<(), String> {
        let client = reqwest::blocking::Client::builder()
            .timeout(std::time::Duration::from_secs(30))
            .build()
            .map_err(|e| e.to_string())?;

        let response = client
            .post(&webhook.url)
            .header("Content-Type", "application/json")
            .header("X-Webhook-Event", "codecartographer")
            .header("X-Webhook-Id", &webhook.id)
            .body(payload.to_string())
            .send()
            .map_err(|e| e.to_string())?;

        let status = response.status();
        if !status.is_success() {
            return Err(format!("Webhook delivery failed with status: {}", status));
        }

        let mut webhooks = self.webhooks.lock().map_err(|e| e.to_string())?;
        if let Some(w) = webhooks.get_mut(&webhook.id) {
            w.last_triggered = Some(current_timestamp());
        }

        Ok(())
    }

    pub fn get_delivery_history(
        &self,
        limit: Option<usize>,
    ) -> Result<Vec<WebhookDelivery>, String> {
        let history = self.delivery_history.lock().map_err(|e| e.to_string())?;
        let limit = limit.unwrap_or(100);
        Ok(history.iter().rev().take(limit).cloned().collect())
    }

    pub fn test_webhook(&self, webhook_id: &str) -> Result<String, String> {
        let webhook = self.get_webhook(webhook_id)?;

        let payload = WebhookPayload {
            event: "test".to_string(),
            timestamp: current_timestamp(),
            data: WebhookData::GraphUpdated(GraphUpdatedData {
                total_files: 0,
                total_edges: 0,
                affected_modules: vec![],
            }),
        };

        let payload_json = serde_json::to_string_pretty(&payload).unwrap_or_default();

        self.deliver_webhook(&webhook, &payload_json)?;

        Ok(format!(
            "Test webhook triggered successfully for {}",
            webhook.url
        ))
    }
}

fn generate_webhook_id() -> String {
    use std::time::SystemTime;
    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_nanos();
    format!("wh_{:x}", timestamp)
}

fn current_timestamp() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_webhook_creation() {
        let webhook = Webhook::new(
            "https://example.com/webhook".to_string(),
            vec![WebhookEvent::GraphUpdated],
        );
        assert!(webhook.id.starts_with("wh_"));
        assert!(webhook.enabled);
    }

    #[test]
    fn test_webhook_events() {
        assert_eq!(WebhookEvent::GraphUpdated.as_str(), "graph_updated");
        assert_eq!(WebhookEvent::ModuleChanged.as_str(), "module_changed");
    }
}
