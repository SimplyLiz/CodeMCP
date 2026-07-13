use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::time::SystemTime;

const MEMORY_FILE: &str = ".codecartographer_memory.json";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FileEntry {
    pub path: String,
    pub content: String,
    pub modified: u64,
    pub hash: u64,
}

#[derive(Debug, Default, Clone, Serialize, Deserialize)]
pub struct Memory {
    pub version: u32,
    pub files: HashMap<String, FileEntry>,
    pub last_sync: u64,
}

impl Memory {
    pub fn load(root: &Path) -> Result<Self> {
        let path = root.join(MEMORY_FILE);
        if path.exists() {
            let data = fs::read_to_string(&path)?;
            Ok(serde_json::from_str(&data)?)
        } else {
            Ok(Self::default())
        }
    }

    pub fn save(&self, root: &Path) -> Result<()> {
        let path = root.join(MEMORY_FILE);
        let data = serde_json::to_string_pretty(self)?;

        let tmp_name = format!(
            ".{}.tmp-{}-{}",
            MEMORY_FILE,
            std::process::id(),
            SystemTime::now()
                .duration_since(SystemTime::UNIX_EPOCH)
                .map(|d| d.as_nanos())
                .unwrap_or(0)
        );
        let tmp_path = root.join(tmp_name);

        #[cfg(unix)]
        let mut file = {
            use std::fs::OpenOptions;
            use std::os::unix::fs::OpenOptionsExt;

            OpenOptions::new()
                .write(true)
                .create_new(true)
                .mode(0o600)
                .open(&tmp_path)?
        };

        #[cfg(not(unix))]
        let mut file = std::fs::OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&tmp_path)?;

        use std::io::Write;
        file.write_all(data.as_bytes())?;
        file.sync_all()?;
        drop(file);

        fs::rename(&tmp_path, &path)?;
        Ok(())
    }

    pub fn get_dirty_files(&self, current_files: &[(PathBuf, u64)]) -> Vec<PathBuf> {
        let mut dirty = Vec::new();

        for (path, modified) in current_files {
            let rel_path = path.to_string_lossy().replace('\\', "/");
            match self.files.get(&rel_path) {
                Some(entry) if entry.modified >= *modified => continue,
                _ => dirty.push(path.clone()),
            }
        }
        dirty
    }

    pub fn patch(&mut self, updates: Vec<FileEntry>) {
        for entry in updates {
            self.files.insert(entry.path.clone(), entry);
        }
        self.last_sync = SystemTime::now()
            .duration_since(SystemTime::UNIX_EPOCH)
            .map(|d| d.as_secs())
            .unwrap_or(0);
    }

    pub fn remove_deleted(&mut self, existing_paths: &[String]) {
        let existing: std::collections::HashSet<_> = existing_paths.iter().collect();
        self.files.retain(|k, _| existing.contains(k));
    }
}

pub fn hash_content(content: &str) -> u64 {
    // FNV-1a: stable across processes and Rust versions (DefaultHasher is not)
    let mut hash: u64 = 14695981039346656037;
    for byte in content.bytes() {
        hash ^= byte as u64;
        hash = hash.wrapping_mul(1099511628211);
    }
    hash
}
