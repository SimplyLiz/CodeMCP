// Layer Configuration - Define architectural boundaries
// Example layers.toml:
// [layers]
// ui = ["components", "pages", "hooks"]
// services = ["api", "auth", "validators"]
// db = ["models", "migrations", "repositories"]
// utils = ["helpers", "constants", "types"]
//
// [allowed_flows]
// ui -> services
// services -> db
// ui -> utils
// services -> utils

use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::path::Path;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LayerConfig {
    pub layers: HashMap<String, Vec<String>>,
    pub allowed_flows: Option<Vec<LayerFlow>>,
    pub strict_mode: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LayerFlow {
    pub from: String,
    pub to: String,
}

impl Default for LayerConfig {
    fn default() -> Self {
        Self {
            layers: HashMap::new(),
            allowed_flows: None,
            strict_mode: false,
        }
    }
}

impl LayerConfig {
    pub fn from_file(path: &Path) -> Result<Self, String> {
        let content = std::fs::read_to_string(path)
            .map_err(|e| format!("Failed to read layers config: {}", e))?;

        Self::from_toml(&content)
    }

    pub fn from_toml(content: &str) -> Result<Self, String> {
        let mut config = LayerConfig::default();

        let mut current_section = String::new();

        for line in content.lines() {
            let line = line.trim();

            if line.is_empty() || line.starts_with('#') {
                continue;
            }

            if line.starts_with('[') && line.ends_with(']') {
                current_section = line.trim_matches('[').trim_matches(']').to_string();
                continue;
            }

            match current_section.as_str() {
                "layers" => {
                    if let Some((key, value)) = line.split_once('=') {
                        let key = key.trim();
                        let folders: Vec<String> = value
                            .split(',')
                            .map(|s| s.trim().trim_matches(|c| c == '"' || c == '[' || c == ']').to_string())
                            .filter(|s| !s.is_empty())
                            .collect();

                        for folder in &folders {
                            config.layers.insert(folder.clone(), vec![key.to_string()]);
                        }
                    }
                }
                "allowed_flows" => {
                    if let Some((from, to)) = line.split_once("->") {
                        let from = from.trim().to_string();
                        let to = to.trim().to_string();

                        if config.allowed_flows.is_none() {
                            config.allowed_flows = Some(Vec::new());
                        }

                        if let Some(ref mut flows) = config.allowed_flows {
                            flows.push(LayerFlow { from, to });
                        }
                    }
                }
                _ => {}
            }
        }

        Ok(config)
    }

    pub fn get_layer(&self, path: &str) -> Option<&String> {
        let path_lower = path.to_lowercase();

        for (folder, layers) in &self.layers {
            if path_lower.contains(&folder.to_lowercase()) {
                return layers.first();
            }
        }

        None
    }

    pub fn is_flow_allowed(&self, from_layer: &str, to_layer: &str) -> bool {
        if from_layer == to_layer {
            return true;
        }

        if let Some(ref flows) = self.allowed_flows {
            for flow in flows {
                if flow.from == from_layer && flow.to == to_layer {
                    return true;
                }
            }
            false
        } else {
            true
        }
    }

    pub fn is_strict(&self) -> bool {
        self.strict_mode
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct LayerViolation {
    pub source_path: String,
    pub target_path: String,
    pub source_layer: String,
    pub target_layer: String,
    pub violation_type: LayerViolationType,
    pub severity: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum LayerViolationType {
    BackCall,
    SkipCall,
    CircularCrossLayer,
    DirectForeignImport,
}

impl LayerViolationType {
    pub fn as_str(&self) -> &str {
        match self {
            LayerViolationType::BackCall => "back_call",
            LayerViolationType::SkipCall => "skip_call",
            LayerViolationType::CircularCrossLayer => "circular_cross_layer",
            LayerViolationType::DirectForeignImport => "direct_foreign_import",
        }
    }

    pub fn severity(&self) -> &str {
        match self {
            LayerViolationType::BackCall => "CRITICAL",
            LayerViolationType::SkipCall => "HIGH",
            LayerViolationType::CircularCrossLayer => "HIGH",
            LayerViolationType::DirectForeignImport => "MEDIUM",
        }
    }
}

pub fn detect_layer_violations(
    edges: &[(String, String)],
    config: &LayerConfig,
) -> Vec<LayerViolation> {
    let mut violations = Vec::new();

    for (source, target) in edges {
        let source_layer = config.get_layer(source);
        let target_layer = config.get_layer(target);

        match (source_layer, target_layer) {
            (Some(sl), Some(tl)) if sl != tl => {
                if !config.is_flow_allowed(sl, tl) {
                    let (violation_type, severity) = if tl < sl {
                        (LayerViolationType::BackCall, "CRITICAL")
                    } else {
                        (LayerViolationType::SkipCall, "HIGH")
                    };

                    violations.push(LayerViolation {
                        source_path: source.clone(),
                        target_path: target.clone(),
                        source_layer: sl.clone(),
                        target_layer: tl.clone(),
                        violation_type,
                        severity: severity.to_string(),
                    });
                }
            }
            (Some(sl), None) => {
                if config.is_strict() {
                    violations.push(LayerViolation {
                        source_path: source.clone(),
                        target_path: target.clone(),
                        source_layer: sl.clone(),
                        target_layer: "unlayered".to_string(),
                        violation_type: LayerViolationType::DirectForeignImport,
                        severity: "MEDIUM".to_string(),
                    });
                }
            }
            _ => {}
        }
    }

    violations
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_layer_config_parse() {
        let toml = r#"
[layers]
ui = ["components", "pages"]
services = ["api", "auth"]
db = ["models", "repos"]

[allowed_flows]
ui -> services
services -> db
"#;

        let config = LayerConfig::from_toml(toml).unwrap();

        assert!(config.layers.contains_key("components"));
        assert!(config.layers.contains_key("models"));

        assert!(config.is_flow_allowed("ui", "services"));
        assert!(!config.is_flow_allowed("db", "ui"));
    }
}
