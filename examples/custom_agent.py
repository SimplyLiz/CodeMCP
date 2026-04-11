#!/usr/bin/env python3
"""
Custom Agent Example - Pulls Context from UltraContext

This demonstrates how to build a custom AI agent that:
1. Receives webhook notifications from CMP
2. Pulls the latest context from UltraContext
3. Uses the context with an AI model (OpenAI, Anthropic, etc.)
"""

import os
import requests
from typing import Dict, List, Optional

class UCAgent:
    """Custom agent that reads from UltraContext"""
    
    def __init__(self, uc_api_key: str, context_id: str):
        self.uc_api_key = uc_api_key
        self.context_id = context_id
        self.base_url = "https://api.ultracontext.ai"
        self.headers = {
            "Authorization": f"Bearer {uc_api_key}",
            "Content-Type": "application/json"
        }
        self.context_cache = None
    
    def pull_context(self, version: Optional[int] = None) -> Dict:
        """Pull context from UltraContext"""
        url = f"{self.base_url}/contexts/{self.context_id}"
        if version is not None:
            url += f"?version={version}"
        
        response = requests.get(url, headers=self.headers)
        response.raise_for_status()
        
        self.context_cache = response.json()
        return self.context_cache
    
    def get_files(self) -> Dict[str, str]:
        """Extract files from context"""
        if not self.context_cache:
            self.pull_context()
        
        files = {}
        for msg in self.context_cache.get('data', []):
            if msg.get('type') == 'file':
                files[msg['path']] = msg['content']
        
        return files
    
    def build_prompt(self, user_query: str) -> str:
        """Build a prompt with context for AI model"""
        files = self.get_files()
        
        prompt = "You are an AI assistant with access to the following codebase:\n\n"
        
        # Add file tree
        prompt += "## File Structure\n"
        for path in sorted(files.keys()):
            prompt += f"- {path}\n"
        
        prompt += "\n## Files\n\n"
        
        # Add file contents
        for path, content in sorted(files.items()):
            ext = path.split('.')[-1] if '.' in path else 'txt'
            prompt += f"### {path}\n```{ext}\n{content}\n```\n\n"
        
        prompt += f"\n## User Query\n{user_query}\n"
        
        return prompt
    
    def handle_webhook(self, payload: Dict):
        """Handle webhook notification from CMP"""
        print(f"📥 Webhook received: {payload['event']}")
        print(f"Context: {payload['context_id']}")
        print(f"Version: {payload['version']}")
        
        changes = payload.get('changes', {})
        print(f"Changes: +{len(changes.get('added', []))} ~{len(changes.get('modified', []))} -{len(changes.get('deleted', []))}")
        
        # Pull latest context
        print("Pulling latest context...")
        self.pull_context()
        print(f"✓ Context updated ({len(self.get_files())} files)")
    
    def chat(self, user_query: str, model: str = "gpt-4") -> str:
        """Chat with AI using context"""
        prompt = self.build_prompt(user_query)
        
        # Here you would call your AI model
        # Example with OpenAI:
        # import openai
        # response = openai.ChatCompletion.create(
        #     model=model,
        #     messages=[{"role": "user", "content": prompt}]
        # )
        # return response.choices[0].message.content
        
        print(f"\n📝 Prompt built ({len(prompt)} chars)")
        print(f"Files included: {len(self.get_files())}")
        return "AI response would go here"


def example_usage():
    """Example usage of custom agent"""
    
    # Get credentials
    uc_api_key = os.getenv("ULTRA_CONTEXT")
    if not uc_api_key:
        print("❌ Set ULTRA_CONTEXT environment variable")
        return
    
    # Load context ID from CMP config
    import json
    try:
        with open(".cartographer_uc_config.json") as f:
            config = json.load(f)
            context_id = config["context_id"]
    except FileNotFoundError:
        print("❌ No .cartographer_uc_config.json found. Run 'cartographer init --cloud' first.")
        return
    
    # Create agent
    agent = UCAgent(uc_api_key, context_id)
    
    # Pull context
    print("Pulling context from UltraContext...")
    agent.pull_context()
    
    # Get files
    files = agent.get_files()
    print(f"✓ Loaded {len(files)} files")
    
    # List files
    print("\nFiles in context:")
    for path in sorted(files.keys())[:10]:
        size = len(files[path])
        print(f"  - {path} ({size} bytes)")
    if len(files) > 10:
        print(f"  ... and {len(files) - 10} more")
    
    # Example chat
    print("\n" + "="*60)
    print("Example: Chat with AI using context")
    print("="*60)
    
    query = "What is the main purpose of this codebase?"
    print(f"\nUser: {query}")
    
    response = agent.chat(query)
    print(f"\nAgent: {response}")
    
    # Example webhook handling
    print("\n" + "="*60)
    print("Example: Handle webhook notification")
    print("="*60)
    
    webhook_payload = {
        "event": "context.updated",
        "context_id": context_id,
        "version": 1,
        "timestamp": "2026-01-22T14:00:00Z",
        "changes": {
            "added": ["new_file.rs"],
            "modified": ["main.rs"],
            "deleted": [],
            "total_files": len(files) + 1
        }
    }
    
    agent.handle_webhook(webhook_payload)


if __name__ == "__main__":
    example_usage()
