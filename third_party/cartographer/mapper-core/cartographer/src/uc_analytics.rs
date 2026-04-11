use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::Path;

const ANALYTICS_FILE: &str = ".cartographer_analytics.json";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct FileAccessLog {
    pub path: String,
    pub access_count: usize,
    pub last_accessed: String,
    pub total_tokens: usize,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct SessionLog {
    pub session_id: String,
    pub started_at: String,
    pub ended_at: Option<String>,
    pub files_accessed: Vec<String>,
    pub total_tokens: usize,
    pub agent_type: Option<String>,
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct Analytics {
    pub file_access: HashMap<String, FileAccessLog>,
    pub sessions: Vec<SessionLog>,
    pub total_syncs: usize,
    pub total_tokens_used: usize,
    pub last_updated: String,
}

impl Analytics {
    pub fn load(root: &Path) -> Result<Self> {
        let path = root.join(ANALYTICS_FILE);
        if path.exists() {
            let data = fs::read_to_string(&path)?;
            Ok(serde_json::from_str(&data)?)
        } else {
            Ok(Self::default())
        }
    }

    pub fn save(&self, root: &Path) -> Result<()> {
        let path = root.join(ANALYTICS_FILE);
        let data = serde_json::to_string_pretty(self)?;
        fs::write(path, data)?;
        Ok(())
    }

    pub fn record_file_access(&mut self, path: &str, tokens: usize) {
        let entry = self
            .file_access
            .entry(path.to_string())
            .or_insert(FileAccessLog {
                path: path.to_string(),
                access_count: 0,
                last_accessed: chrono::Utc::now().to_rfc3339(),
                total_tokens: 0,
            });

        entry.access_count += 1;
        entry.last_accessed = chrono::Utc::now().to_rfc3339();
        entry.total_tokens += tokens;
        self.total_tokens_used += tokens;
        self.last_updated = chrono::Utc::now().to_rfc3339();
    }

    pub fn start_session(&mut self, agent_type: Option<String>) -> String {
        let session_id = uuid::Uuid::new_v4().to_string();
        let session = SessionLog {
            session_id: session_id.clone(),
            started_at: chrono::Utc::now().to_rfc3339(),
            ended_at: None,
            files_accessed: Vec::new(),
            total_tokens: 0,
            agent_type,
        };

        self.sessions.push(session);
        self.last_updated = chrono::Utc::now().to_rfc3339();
        session_id
    }

    pub fn end_session(&mut self, session_id: &str) {
        if let Some(session) = self
            .sessions
            .iter_mut()
            .find(|s| s.session_id == session_id)
        {
            session.ended_at = Some(chrono::Utc::now().to_rfc3339());
            self.last_updated = chrono::Utc::now().to_rfc3339();
        }
    }

    pub fn record_sync(&mut self) {
        self.total_syncs += 1;
        self.last_updated = chrono::Utc::now().to_rfc3339();
    }

    pub fn get_most_accessed_files(&self, limit: usize) -> Vec<&FileAccessLog> {
        let mut files: Vec<_> = self.file_access.values().collect();
        files.sort_by(|a, b| b.access_count.cmp(&a.access_count));
        files.into_iter().take(limit).collect()
    }

    pub fn get_recent_sessions(&self, limit: usize) -> Vec<&SessionLog> {
        let mut sessions: Vec<_> = self.sessions.iter().collect();
        sessions.sort_by(|a, b| b.started_at.cmp(&a.started_at));
        sessions.into_iter().take(limit).collect()
    }

    pub fn calculate_context_health(&self) -> ContextHealth {
        let total_files = self.file_access.len();
        let accessed_files = self
            .file_access
            .values()
            .filter(|f| f.access_count > 0)
            .count();
        let unused_files = total_files - accessed_files;

        let avg_tokens_per_file = if total_files > 0 {
            self.total_tokens_used / total_files
        } else {
            0
        };

        let health_score = if total_files > 0 {
            ((accessed_files as f64 / total_files as f64) * 100.0) as u8
        } else {
            100
        };

        ContextHealth {
            total_files,
            accessed_files,
            unused_files,
            total_tokens_used: self.total_tokens_used,
            avg_tokens_per_file,
            health_score,
            total_syncs: self.total_syncs,
            total_sessions: self.sessions.len(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ContextHealth {
    pub total_files: usize,
    pub accessed_files: usize,
    pub unused_files: usize,
    pub total_tokens_used: usize,
    pub avg_tokens_per_file: usize,
    pub health_score: u8,
    pub total_syncs: usize,
    pub total_sessions: usize,
}

impl ContextHealth {
    pub fn print(&self) {
        println!("\nContext Health Report:");
        println!("============================================");
        println!(
            "Health Score:        {}% {}",
            self.health_score,
            self.health_emoji()
        );
        println!("Total Files:         {}", self.total_files);
        println!(
            "Accessed Files:      {} ({:.1}%)",
            self.accessed_files,
            (self.accessed_files as f64 / self.total_files as f64) * 100.0
        );
        println!(
            "Unused Files:        {} ({:.1}%)",
            self.unused_files,
            (self.unused_files as f64 / self.total_files as f64) * 100.0
        );
        println!(
            "Total Tokens Used:   {}",
            format_tokens(self.total_tokens_used)
        );
        println!(
            "Avg Tokens/File:     {}",
            format_tokens(self.avg_tokens_per_file)
        );
        println!("Total Syncs:         {}", self.total_syncs);
        println!("Total Sessions:      {}", self.total_sessions);
        println!("============================================\n");

        if self.health_score < 50 {
            println!("⚠️  Low health score detected!");
            println!("Recommendation: Run 'cartographer optimize' to remove unused files.\n");
        }
    }

    fn health_emoji(&self) -> &'static str {
        match self.health_score {
            90..=100 => "🟢",
            70..=89 => "🟡",
            50..=69 => "🟠",
            _ => "🔴",
        }
    }
}

pub struct AnalyticsService {
    root: std::path::PathBuf,
}

impl AnalyticsService {
    pub fn new(root: &Path) -> Self {
        Self {
            root: root.to_path_buf(),
        }
    }

    pub fn print_dashboard(&self) -> Result<()> {
        let analytics = Analytics::load(&self.root)?;
        let health = analytics.calculate_context_health();

        health.print();

        println!("Most Accessed Files:");
        println!("============================================");
        let top_files = analytics.get_most_accessed_files(10);
        if top_files.is_empty() {
            println!("No file access data yet.");
        } else {
            for (i, file) in top_files.iter().enumerate() {
                println!(
                    "{}. {} ({} accesses, {})",
                    i + 1,
                    file.path,
                    file.access_count,
                    format_tokens(file.total_tokens)
                );
            }
        }
        println!("============================================\n");

        println!("Recent Sessions:");
        println!("============================================");
        let recent = analytics.get_recent_sessions(5);
        if recent.is_empty() {
            println!("No session data yet.");
        } else {
            for session in recent {
                let status = if session.ended_at.is_some() {
                    "completed"
                } else {
                    "active"
                };
                let agent = session.agent_type.as_deref().unwrap_or("unknown");
                println!(
                    "Session {} ({}) - {} files, {}",
                    &session.session_id[..8],
                    agent,
                    session.files_accessed.len(),
                    status
                );
            }
        }
        println!("============================================\n");

        Ok(())
    }

    pub fn optimize_suggestions(&self) -> Result<Vec<String>> {
        let analytics = Analytics::load(&self.root)?;
        let mut suggestions = Vec::new();

        let unused: Vec<_> = analytics
            .file_access
            .values()
            .filter(|f| f.access_count == 0)
            .map(|f| f.path.clone())
            .collect();

        if !unused.is_empty() {
            suggestions.push(format!(
                "Remove {} unused files to reduce context size",
                unused.len()
            ));
        }

        let large_files: Vec<_> = analytics
            .file_access
            .values()
            .filter(|f| f.total_tokens > 5000)
            .collect();

        if !large_files.is_empty() {
            suggestions.push(format!(
                "Consider splitting {} large files (>5k tokens)",
                large_files.len()
            ));
        }

        if analytics.total_tokens_used > 100_000 {
            suggestions.push(
                "High token usage detected. Consider using skeleton maps more often.".to_string(),
            );
        }

        Ok(suggestions)
    }
}

fn format_tokens(tokens: usize) -> String {
    if tokens >= 1_000_000 {
        format!("{:.1}M tokens", tokens as f64 / 1_000_000.0)
    } else if tokens >= 1_000 {
        format!("{:.1}k tokens", tokens as f64 / 1_000.0)
    } else {
        format!("{} tokens", tokens)
    }
}
