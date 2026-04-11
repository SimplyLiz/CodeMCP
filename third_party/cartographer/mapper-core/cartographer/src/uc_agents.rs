use anyhow::Result;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs;
use std::path::Path;

const AGENTS_CONFIG_FILE: &str = ".cartographer_agents.json";

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct AgentConfig {
    pub id: String,
    pub name: String,
    pub agent_type: AgentType,
    pub context_id: String,
    pub api_key: Option<String>,
    pub webhook_url: Option<String>,
    pub enabled: bool,
    pub created_at: String,
}

#[derive(Debug, Clone, Serialize, Deserialize, PartialEq)]
#[serde(rename_all = "lowercase")]
pub enum AgentType {
    Cursor,
    Copilot,
    Claude,
    Custom,
}

impl std::fmt::Display for AgentType {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        match self {
            AgentType::Cursor => write!(f, "cursor"),
            AgentType::Copilot => write!(f, "copilot"),
            AgentType::Claude => write!(f, "claude"),
            AgentType::Custom => write!(f, "custom"),
        }
    }
}

#[derive(Debug, Default, Serialize, Deserialize)]
pub struct AgentsRegistry {
    pub agents: HashMap<String, AgentConfig>,
}

impl AgentsRegistry {
    pub fn load(root: &Path) -> Result<Self> {
        let path = root.join(AGENTS_CONFIG_FILE);
        if path.exists() {
            let data = fs::read_to_string(&path)?;
            Ok(serde_json::from_str(&data)?)
        } else {
            Ok(Self::default())
        }
    }

    pub fn save(&self, root: &Path) -> Result<()> {
        let path = root.join(AGENTS_CONFIG_FILE);
        let data = serde_json::to_string_pretty(self)?;
        fs::write(path, data)?;
        Ok(())
    }

    pub fn add_agent(&mut self, agent: AgentConfig) {
        self.agents.insert(agent.id.clone(), agent);
    }

    pub fn remove_agent(&mut self, agent_id: &str) -> Option<AgentConfig> {
        self.agents.remove(agent_id)
    }

    pub fn get_agent(&self, agent_id: &str) -> Option<&AgentConfig> {
        self.agents.get(agent_id)
    }

    pub fn list_agents(&self) -> Vec<&AgentConfig> {
        let mut agents: Vec<_> = self.agents.values().collect();
        agents.sort_by(|a, b| a.name.cmp(&b.name));
        agents
    }

    pub fn enable_agent(&mut self, agent_id: &str) -> Result<()> {
        if let Some(agent) = self.agents.get_mut(agent_id) {
            agent.enabled = true;
            Ok(())
        } else {
            anyhow::bail!("Agent not found: {}", agent_id)
        }
    }

    pub fn disable_agent(&mut self, agent_id: &str) -> Result<()> {
        if let Some(agent) = self.agents.get_mut(agent_id) {
            agent.enabled = false;
            Ok(())
        } else {
            anyhow::bail!("Agent not found: {}", agent_id)
        }
    }
}

pub struct AgentService {
    root: std::path::PathBuf,
}

impl AgentService {
    pub fn new(root: &Path) -> Self {
        Self {
            root: root.to_path_buf(),
        }
    }

    pub fn add_agent(
        &self,
        name: &str,
        agent_type: AgentType,
        context_id: &str,
        api_key: Option<String>,
        webhook_url: Option<String>,
    ) -> Result<AgentConfig> {
        let mut registry = AgentsRegistry::load(&self.root)?;

        let agent = AgentConfig {
            id: uuid::Uuid::new_v4().to_string(),
            name: name.to_string(),
            agent_type,
            context_id: context_id.to_string(),
            api_key,
            webhook_url,
            enabled: true,
            created_at: chrono::Utc::now().to_rfc3339(),
        };

        registry.add_agent(agent.clone());
        registry.save(&self.root)?;

        println!("✓ Agent '{}' added ({})", name, agent.id);
        Ok(agent)
    }

    pub fn remove_agent(&self, agent_id: &str) -> Result<()> {
        let mut registry = AgentsRegistry::load(&self.root)?;

        if let Some(agent) = registry.remove_agent(agent_id) {
            registry.save(&self.root)?;
            println!("✓ Agent '{}' removed", agent.name);
            Ok(())
        } else {
            anyhow::bail!("Agent not found: {}", agent_id)
        }
    }

    pub fn list_agents(&self) -> Result<Vec<AgentConfig>> {
        let registry = AgentsRegistry::load(&self.root)?;
        Ok(registry.list_agents().into_iter().cloned().collect())
    }

    pub fn enable_agent(&self, agent_id: &str) -> Result<()> {
        let mut registry = AgentsRegistry::load(&self.root)?;
        registry.enable_agent(agent_id)?;
        registry.save(&self.root)?;
        println!("✓ Agent enabled");
        Ok(())
    }

    pub fn disable_agent(&self, agent_id: &str) -> Result<()> {
        let mut registry = AgentsRegistry::load(&self.root)?;
        registry.disable_agent(agent_id)?;
        registry.save(&self.root)?;
        println!("✓ Agent disabled");
        Ok(())
    }

    pub fn print_agents_table(&self) -> Result<()> {
        let agents = self.list_agents()?;

        if agents.is_empty() {
            println!("No agents configured. Use 'cartographer agents add' to add one.");
            return Ok(());
        }

        println!("\nConfigured Agents:");
        println!("============================================");
        println!("{:<36} {:<15} {:<10} {:<8}", "ID", "Name", "Type", "Status");
        println!("--------------------------------------------");

        for agent in agents {
            let status = if agent.enabled { "enabled" } else { "disabled" };
            println!(
                "{:<36} {:<15} {:<10} {:<8}",
                agent.id, agent.name, agent.agent_type, status
            );
        }

        println!("============================================\n");
        Ok(())
    }

    pub fn get_agent_details(&self, agent_id: &str) -> Result<AgentConfig> {
        let registry = AgentsRegistry::load(&self.root)?;
        registry
            .get_agent(agent_id)
            .cloned()
            .ok_or_else(|| anyhow::anyhow!("Agent not found: {}", agent_id))
    }

    pub fn print_agent_details(&self, agent_id: &str) -> Result<()> {
        let agent = self.get_agent_details(agent_id)?;

        println!("\nAgent Details:");
        println!("============================================");
        println!("ID:          {}", agent.id);
        println!("Name:        {}", agent.name);
        println!("Type:        {}", agent.agent_type);
        println!("Context ID:  {}", agent.context_id);
        println!(
            "Status:      {}",
            if agent.enabled { "enabled" } else { "disabled" }
        );
        println!("Created:     {}", agent.created_at);

        if let Some(key) = &agent.api_key {
            println!("API Key:     {}...{}", &key[..8], &key[key.len() - 4..]);
        }

        if let Some(webhook) = &agent.webhook_url {
            println!("Webhook:     {}", webhook);
        }

        println!("============================================\n");
        Ok(())
    }
}
