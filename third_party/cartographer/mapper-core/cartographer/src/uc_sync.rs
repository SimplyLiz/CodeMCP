use crate::memory::{FileEntry, Memory};
use crate::uc_client::{UCClient, UCMessage, UCVersion};
use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::Path;

const UC_CONFIG_FILE: &str = ".cartographer_uc_config.json";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct UCConfig {
    pub context_id: String,
    pub project_name: String,
    pub last_version: u32,
    pub last_sync: u64,
    pub file_hashes: HashMap<String, u64>, // file_path -> hash (for change detection)
}

impl UCConfig {
    pub fn load(root: &Path) -> Result<Self> {
        let path = root.join(UC_CONFIG_FILE);
        if path.exists() {
            let data = fs::read_to_string(&path)?;
            Ok(serde_json::from_str(&data)?)
        } else {
            anyhow::bail!("No UC config found. Run 'cartographer init --cloud' first.")
        }
    }

    pub fn save(&self, root: &Path) -> Result<()> {
        let path = root.join(UC_CONFIG_FILE);
        let data = serde_json::to_string_pretty(self)?;
        fs::write(path, data)?;
        Ok(())
    }
}

pub struct UCSyncService {
    client: UCClient,
    root: std::path::PathBuf,
}

impl UCSyncService {
    pub fn new(api_key: String, root: &Path) -> Result<Self> {
        let client = UCClient::new(api_key)?;
        Ok(Self {
            client,
            root: root.to_path_buf(),
        })
    }

    /// Initialize UC sync for this project
    pub fn init(&self, project_name: &str) -> Result<UCConfig> {
        println!("Initializing UC sync for '{}'...", project_name);

        // Create new context in UC
        let ctx = self.client.create_context(None, None)?;

        // Add project metadata as first message
        let mut metadata = HashMap::new();
        metadata.insert("type".to_string(), serde_json::json!("project_metadata"));
        metadata.insert("project_name".to_string(), serde_json::json!(project_name));
        metadata.insert(
            "cartographer_version".to_string(),
            serde_json::json!(env!("CARGO_PKG_VERSION")),
        );
        metadata.insert(
            "initialized_at".to_string(),
            serde_json::json!(chrono::Utc::now().to_rfc3339()),
        );

        let msg = UCMessage {
            id: String::new(),
            index: 0,
            metadata: serde_json::json!({}),
            data: metadata,
        };

        self.client.append(&ctx.id, msg)?;

        let config = UCConfig {
            context_id: ctx.id.clone(),
            project_name: project_name.to_string(),
            last_version: ctx.version,
            last_sync: std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_secs(),
            file_hashes: HashMap::new(),
        };

        config.save(&self.root)?;

        println!("✓ UC context created: {}", ctx.id);
        println!("✓ Config saved to {}", UC_CONFIG_FILE);

        Ok(config)
    }

    /// Push local memory to UC (append-only with change detection)
    pub fn push(&self, memory: &Memory) -> Result<UCConfig> {
        let mut config = UCConfig::load(&self.root)?;

        println!(
            "Pushing {} files to UC context {}...",
            memory.files.len(),
            config.context_id
        );

        let mut new_count = 0;
        let mut updated_count = 0;
        let mut deleted_count = 0;

        let current_files: std::collections::HashSet<_> = memory.files.keys().collect();

        // Detect changes
        let mut added_files = Vec::new();
        let mut modified_files = Vec::new();

        for (path, entry) in &memory.files {
            match config.file_hashes.get(path) {
                None => {
                    // New file
                    added_files.push((path.clone(), entry));
                    new_count += 1;
                }
                Some(&old_hash) if old_hash != entry.hash => {
                    // Modified file
                    modified_files.push((path.clone(), entry));
                    updated_count += 1;
                }
                _ => {
                    // Unchanged file - skip
                }
            }
        }

        // Detect deleted files
        let mut deleted_files = Vec::new();
        for path in config.file_hashes.keys() {
            if !current_files.contains(path) {
                deleted_files.push(path.clone());
                deleted_count += 1;
            }
        }

        // Append changes to UC (append-only model)
        // 1. Append new files
        for (path, entry) in &added_files {
            let mut msg_data = self.file_entry_to_message(entry);
            msg_data.insert("operation".to_string(), serde_json::json!("add"));

            let msg = UCMessage {
                id: String::new(),
                index: 0,
                metadata: serde_json::json!({}),
                data: msg_data,
            };
            self.client.append(&config.context_id, msg)?;
            config.file_hashes.insert(path.clone(), entry.hash);
        }

        // 2. Append modified files (as updates)
        for (path, entry) in &modified_files {
            let mut msg_data = self.file_entry_to_message(entry);
            msg_data.insert("operation".to_string(), serde_json::json!("update"));

            let msg = UCMessage {
                id: String::new(),
                index: 0,
                metadata: serde_json::json!({}),
                data: msg_data,
            };
            self.client.append(&config.context_id, msg)?;
            config.file_hashes.insert(path.clone(), entry.hash);
        }

        // 3. Append deletion markers
        for path in &deleted_files {
            let mut msg_data = HashMap::new();
            msg_data.insert("type".to_string(), serde_json::json!("file"));
            msg_data.insert("path".to_string(), serde_json::json!(path));
            msg_data.insert("operation".to_string(), serde_json::json!("delete"));

            let msg = UCMessage {
                id: String::new(),
                index: 0,
                metadata: serde_json::json!({}),
                data: msg_data,
            };
            self.client.append(&config.context_id, msg)?;
            config.file_hashes.remove(path);
        }

        // Update config
        let ctx = self.client.get_context(&config.context_id, None, false)?;
        config.last_version = ctx.version;
        config.last_sync = std::time::SystemTime::now()
            .duration_since(std::time::UNIX_EPOCH)
            .unwrap()
            .as_secs();
        config.save(&self.root)?;

        println!(
            "✓ Push complete: {} new, {} updated, {} deleted",
            new_count, updated_count, deleted_count
        );
        println!("✓ UC version: {}", config.last_version);

        Ok(config)
    }

    /// Pull UC context to local memory
    pub fn pull(&self, version: Option<u32>) -> Result<Memory> {
        let config = UCConfig::load(&self.root)?;

        println!("Pulling from UC context {}...", config.context_id);
        if let Some(v) = version {
            println!("Target version: {}", v);
        }

        let ctx = self
            .client
            .get_context(&config.context_id, version, false)?;

        let mut memory = Memory::default();
        memory.version = ctx.version;

        for msg in &ctx.data {
            if let Some(entry) = self.message_to_file_entry(msg) {
                memory.files.insert(entry.path.clone(), entry);
            }
        }

        println!(
            "✓ Pulled {} files (version {})",
            memory.files.len(),
            ctx.version
        );

        Ok(memory)
    }

    /// Get context history
    pub fn history(&self) -> Result<Vec<UCVersion>> {
        let config = UCConfig::load(&self.root)?;
        let ctx = self.client.get_context(&config.context_id, None, true)?;

        Ok(ctx.versions.unwrap_or_default())
    }

    /// Create a branch from current or specific version
    pub fn branch(&self, branch_name: &str, from_version: Option<u32>) -> Result<UCConfig> {
        let config = UCConfig::load(&self.root)?;

        println!(
            "Creating branch '{}' from context {}...",
            branch_name, config.context_id
        );

        let new_ctx = self
            .client
            .create_context(Some(&config.context_id), from_version)?;

        let branch_config = UCConfig {
            context_id: new_ctx.id.clone(),
            project_name: format!("{}-{}", config.project_name, branch_name),
            last_version: new_ctx.version,
            last_sync: std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap()
                .as_secs(),
            file_hashes: HashMap::new(),
        };

        // Save branch config with different name
        let branch_config_path = self
            .root
            .join(format!(".cartographer_uc_config.{}.json", branch_name));
        let data = serde_json::to_string_pretty(&branch_config)?;
        fs::write(branch_config_path, data)?;

        println!("✓ Branch created: {}", new_ctx.id);
        println!("✓ Config saved to .cartographer_uc_config.{}.json", branch_name);

        Ok(branch_config)
    }

    /// Diff between two versions
    pub fn diff(&self, v1: u32, v2: u32) -> Result<ContextDiff> {
        let config = UCConfig::load(&self.root)?;

        let ctx1 = self
            .client
            .get_context(&config.context_id, Some(v1), false)?;
        let ctx2 = self
            .client
            .get_context(&config.context_id, Some(v2), false)?;

        let files1: HashMap<String, FileEntry> = ctx1
            .data
            .iter()
            .filter_map(|msg| self.message_to_file_entry(msg))
            .map(|e| (e.path.clone(), e))
            .collect();

        let files2: HashMap<String, FileEntry> = ctx2
            .data
            .iter()
            .filter_map(|msg| self.message_to_file_entry(msg))
            .map(|e| (e.path.clone(), e))
            .collect();

        let mut added = Vec::new();
        let mut modified = Vec::new();
        let mut deleted = Vec::new();

        for (path, entry2) in &files2 {
            match files1.get(path) {
                None => added.push(path.clone()),
                Some(entry1) if entry1.hash != entry2.hash => modified.push(path.clone()),
                _ => {}
            }
        }

        for path in files1.keys() {
            if !files2.contains_key(path) {
                deleted.push(path.clone());
            }
        }

        Ok(ContextDiff {
            from_version: v1,
            to_version: v2,
            added,
            modified,
            deleted,
        })
    }

    fn file_entry_to_message(&self, entry: &FileEntry) -> HashMap<String, serde_json::Value> {
        let mut data = HashMap::new();
        data.insert("type".to_string(), serde_json::json!("file"));
        data.insert("path".to_string(), serde_json::json!(entry.path));
        data.insert("content".to_string(), serde_json::json!(entry.content));
        data.insert("modified".to_string(), serde_json::json!(entry.modified));
        data.insert("hash".to_string(), serde_json::json!(entry.hash));
        data
    }

    fn message_to_file_entry(&self, msg: &UCMessage) -> Option<FileEntry> {
        let msg_type = msg.data.get("type")?.as_str()?;
        if msg_type != "file" {
            return None;
        }

        Some(FileEntry {
            path: msg.data.get("path")?.as_str()?.to_string(),
            content: msg.data.get("content")?.as_str()?.to_string(),
            modified: msg.data.get("modified")?.as_u64()?,
            hash: msg.data.get("hash")?.as_u64()?,
        })
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContextDiff {
    pub from_version: u32,
    pub to_version: u32,
    pub added: Vec<String>,
    pub modified: Vec<String>,
    pub deleted: Vec<String>,
}

impl ContextDiff {
    pub fn print(&self) {
        println!(
            "\nContext Diff: v{} → v{}",
            self.from_version, self.to_version
        );
        println!("============================================");

        if !self.added.is_empty() {
            println!("\n+ Added ({}):", self.added.len());
            for path in &self.added {
                println!("  + {}", path);
            }
        }

        if !self.modified.is_empty() {
            println!("\n~ Modified ({}):", self.modified.len());
            for path in &self.modified {
                println!("  ~ {}", path);
            }
        }

        if !self.deleted.is_empty() {
            println!("\n- Deleted ({}):", self.deleted.len());
            for path in &self.deleted {
                println!("  - {}", path);
            }
        }

        if self.added.is_empty() && self.modified.is_empty() && self.deleted.is_empty() {
            println!("No changes");
        }
    }
}
