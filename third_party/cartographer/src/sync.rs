use crate::memory::{hash_content, FileEntry, Memory};
use crate::scanner::{is_source_file, scan_files_with_noise_tracking, IgnoredFile};
use anyhow::Result;
use std::fs;
use std::path::Path;
use std::time::SystemTime;

/// Result of a sync operation with noise tracking
pub struct SyncResult {
    pub memory: Memory,
    pub ignored_noise: Vec<IgnoredFile>,
}

pub struct SyncService {
    root: std::path::PathBuf,
}

impl SyncService {
    pub fn new(root: &Path) -> Self {
        Self {
            root: root.to_path_buf(),
        }
    }

    /// Full scan - builds memory from scratch (legacy, no noise tracking)
    #[allow(dead_code)]
    pub fn full_scan(&self) -> Result<Memory> {
        let result = self.full_scan_with_noise()?;
        Ok(result.memory)
    }

    /// Full scan with noise tracking
    pub fn full_scan_with_noise(&self) -> Result<SyncResult> {
        let mut memory = Memory::default();
        memory.version = 1;

        let scan_result = scan_files_with_noise_tracking(&self.root)?;
        let source_files: Vec<_> = scan_result.files.into_iter().filter(|p| is_source_file(p)).collect();
        let entries = self.parse_files(&source_files);
        memory.patch(entries);

        Ok(SyncResult {
            memory,
            ignored_noise: scan_result.ignored_noise,
        })
    }

    /// Incremental sync - only updates dirty files (legacy, no noise tracking)
    #[allow(dead_code)]
    pub fn incremental_sync(&self, memory: Memory) -> Result<Memory> {
        let result = self.incremental_sync_with_noise(memory)?;
        Ok(result.memory)
    }

    /// Incremental sync with noise tracking
    pub fn incremental_sync_with_noise(&self, mut memory: Memory) -> Result<SyncResult> {
        let scan_result = scan_files_with_noise_tracking(&self.root)?;
        let files: Vec<_> = scan_result.files.into_iter().filter(|p| is_source_file(p)).collect();

        // Get file paths with modification times
        let current: Vec<_> = files
            .iter()
            .filter_map(|p| {
                let modified = fs::metadata(p)
                    .and_then(|m| m.modified())
                    .ok()?
                    .duration_since(SystemTime::UNIX_EPOCH)
                    .ok()?
                    .as_secs();
                Some((p.clone(), modified))
            })
            .collect();

        // Find dirty files
        let dirty = memory.get_dirty_files(&current);

        if dirty.is_empty() {
            println!("✓ No changes detected");
            return Ok(SyncResult {
                memory,
                ignored_noise: scan_result.ignored_noise,
            });
        }

        println!("⟳ Syncing {} changed file(s)...", dirty.len());

        // Parse only dirty files
        let updates = self.parse_files(&dirty);
        memory.patch(updates);

        // Remove deleted files
        let existing: Vec<String> = current
            .iter()
            .map(|(p, _)| {
                p.strip_prefix(&self.root)
                    .unwrap_or(p)
                    .to_string_lossy()
                    .replace('\\', "/")
            })
            .collect();
        memory.remove_deleted(&existing);

        Ok(SyncResult {
            memory,
            ignored_noise: scan_result.ignored_noise,
        })
    }

    /// Force-include specific ignored files back into the scan
    #[allow(dead_code)]
    pub fn include_ignored_files(&self, memory: &mut Memory, ignored: &[IgnoredFile]) {
        let paths: Vec<_> = ignored.iter().map(|i| self.root.join(&i.path)).collect();
        let entries = self.parse_files(&paths);
        memory.patch(entries);
    }

    fn parse_files(&self, files: &[std::path::PathBuf]) -> Vec<FileEntry> {
        files
            .iter()
            .filter_map(|path| {
                let content = read_text_file(path)?;
                let modified = fs::metadata(path)
                    .and_then(|m| m.modified())
                    .ok()?
                    .duration_since(SystemTime::UNIX_EPOCH)
                    .ok()?
                    .as_secs();

                let rel_path = path
                    .strip_prefix(&self.root)
                    .unwrap_or(path)
                    .to_string_lossy()
                    .replace('\\', "/");

                Some(FileEntry {
                    path: rel_path,
                    content: content.clone(),
                    modified,
                    hash: hash_content(&content),
                })
            })
            .collect()
    }
}

fn read_text_file(path: &Path) -> Option<String> {
    let content = fs::read(path).ok()?;
    let check_len = content.len().min(8192);
    if content[..check_len].contains(&0) {
        return None;
    }
    String::from_utf8(content).ok()
}
