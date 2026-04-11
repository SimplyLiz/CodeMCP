use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::fs;
use std::path::PathBuf;

/// Global user-level config at ~/.config/cartographer/config.toml.
/// Applies across all repos; per-repo .cartographer/config.toml overrides it.
#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct GlobalConfig {
    #[serde(default)]
    pub api: ApiConfig,
    #[serde(default)]
    pub defaults: DefaultsConfig,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct ApiConfig {
    pub key: Option<String>,
}

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct DefaultsConfig {
    /// Output target: "claude", "cursor", or "raw"
    pub target: Option<String>,
}

impl GlobalConfig {
    pub fn config_path() -> Option<PathBuf> {
        let home = std::env::var("HOME").ok()?;
        Some(
            PathBuf::from(home)
                .join(".config")
                .join("cartographer")
                .join("config.toml"),
        )
    }

    pub fn load() -> Self {
        Self::try_load().unwrap_or_default()
    }

    fn try_load() -> Option<Self> {
        let path = Self::config_path()?;
        let content = fs::read_to_string(path).ok()?;
        toml::from_str(&content).ok()
    }

    pub fn save(&self) -> Result<()> {
        let path = Self::config_path()
            .ok_or_else(|| anyhow::anyhow!("Could not determine home directory"))?;
        if let Some(parent) = path.parent() {
            fs::create_dir_all(parent)?;
        }
        let content = toml::to_string_pretty(self)?;
        fs::write(path, content)?;
        Ok(())
    }
}
