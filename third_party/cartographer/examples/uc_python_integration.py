#!/usr/bin/env python3
"""
CMP + UltraContext Python Integration Example

This demonstrates how to:
1. Read CMP-generated context
2. Push to UltraContext
3. Pull from UltraContext
4. Use with AI frameworks (OpenAI, Anthropic, etc.)
"""

import os
import json
import requests
from typing import Dict, List, Optional

UC_BASE_URL = "https://api.ultracontext.ai/v1"


class UCClient:
    """UltraContext API client"""
    
    def __init__(self, api_key: str):
        self.api_key = api_key
        self.headers = {
            "Authorization": f"Bearer {api_key}",
            "Content-Type": "application/json"
        }
    
    def create_context(self, from_ctx: Optional[str] = None, version: Optional[int] = None) -> Dict:
        """Create a new context"""
        body = {}
        if from_ctx:
            body["from"] = from_ctx
            if version is not None:
                body["version"] = version
        
        response = requests.post(
            f"{UC_BASE_URL}/contexts",
            headers=self.headers,
            json=body
        )
        response.raise_for_status()
        return response.json()
    
    def get_context(self, ctx_id: str, version: Optional[int] = None, history: bool = False) -> Dict:
        """Get context with optional version and history"""
        params = {}
        if version is not None:
            params["version"] = version
        if history:
            params["history"] = "true"
        
        response = requests.get(
            f"{UC_BASE_URL}/contexts/{ctx_id}",
            headers=self.headers,
            params=params
        )
        response.raise_for_status()
        return response.json()
    
    def append(self, ctx_id: str, message: Dict) -> Dict:
        """Append a message to context"""
        response = requests.post(
            f"{UC_BASE_URL}/contexts/{ctx_id}/messages",
            headers=self.headers,
            json=message
        )
        response.raise_for_status()
        return response.json()
    
    def update(self, ctx_id: str, message: Dict) -> Dict:
        """Update a message"""
        response = requests.patch(
            f"{UC_BASE_URL}/contexts/{ctx_id}/messages",
            headers=self.headers,
            json=message
        )
        response.raise_for_status()
        return response.json()
    
    def delete(self, ctx_id: str, msg_id: str) -> Dict:
        """Delete a message"""
        response = requests.delete(
            f"{UC_BASE_URL}/contexts/{ctx_id}/messages/{msg_id}",
            headers=self.headers
        )
        response.raise_for_status()
        return response.json()


class CMPUCIntegration:
    """Integration between CMP and UltraContext"""
    
    def __init__(self, api_key: str):
        self.client = UCClient(api_key)
        self.config_file = ".cartographer_uc_config.json"
    
    def load_config(self) -> Dict:
        """Load CMP UC configuration"""
        if not os.path.exists(self.config_file):
            raise FileNotFoundError("No UC config found. Run 'cartographer init --cloud' first.")
        
        with open(self.config_file, 'r') as f:
            return json.load(f)
    
    def save_config(self, config: Dict):
        """Save CMP UC configuration"""
        with open(self.config_file, 'w') as f:
            json.dump(config, f, indent=2)
    
    def load_cartographer_memory(self) -> Dict:
        """Load Cartographer memory file"""
        memory_file = ".cartographer_memory.json"
        if not os.path.exists(memory_file):
            raise FileNotFoundError("No Cartographer memory found. Run 'cartographer source' first.")
        
        with open(memory_file, 'r') as f:
            return json.load(f)
    
    def init_project(self, project_name: str) -> Dict:
        """Initialize UC sync for project"""
        print(f"Initializing UC sync for '{project_name}'...")
        
        # Create new context
        ctx = self.client.create_context()
        
        # Add project metadata
        metadata = {
            "type": "project_metadata",
            "project_name": project_name,
            "initialized_at": "2025-01-22T00:00:00Z"
        }
        self.client.append(ctx["id"], metadata)
        
        # Save config
        config = {
            "context_id": ctx["id"],
            "project_name": project_name,
            "last_version": ctx["version"],
            "last_sync": 0,
            "file_message_map": {}
        }
        self.save_config(config)
        
        print(f"✓ UC context created: {ctx['id']}")
        return config
    
    def push_to_uc(self):
        """Push Cartographer memory to UC"""
        config = self.load_config()
        memory = self.load_cartographer_memory()
        
        ctx_id = config["context_id"]
        files = memory.get("files", {})
        
        print(f"Pushing {len(files)} files to UC context {ctx_id}...")
        
        updated = 0
        new = 0
        
        for path, entry in files.items():
            msg_data = {
                "type": "file",
                "path": path,
                "content": entry["content"],
                "modified": entry["modified"],
                "hash": entry["hash"]
            }
            
            if path in config["file_message_map"]:
                # Update existing
                msg_id = config["file_message_map"][path]
                msg_data["id"] = msg_id
                self.client.update(ctx_id, msg_data)
                updated += 1
            else:
                # Append new
                result = self.client.append(ctx_id, msg_data)
                if result.get("data"):
                    last_msg = result["data"][-1]
                    config["file_message_map"][path] = last_msg["id"]
                new += 1
        
        # Update config
        ctx = self.client.get_context(ctx_id)
        config["last_version"] = ctx["version"]
        self.save_config(config)
        
        print(f"✓ Push complete: {new} new, {updated} updated")
        print(f"✓ UC version: {config['last_version']}")
    
    def pull_from_uc(self, version: Optional[int] = None):
        """Pull UC context to local memory"""
        config = self.load_config()
        ctx_id = config["context_id"]
        
        print(f"Pulling from UC context {ctx_id}...")
        if version is not None:
            print(f"Target version: {version}")
        
        ctx = self.client.get_context(ctx_id, version)
        
        # Convert to Cartographer memory format
        memory = {
            "version": ctx["version"],
            "files": {},
            "last_sync": 0
        }
        
        for msg in ctx.get("data", []):
            if msg.get("type") == "file":
                path = msg["path"]
                memory["files"][path] = {
                    "path": path,
                    "content": msg["content"],
                    "modified": msg["modified"],
                    "hash": msg["hash"]
                }
        
        # Save memory
        with open(".cartographer_memory.json", 'w') as f:
            json.dump(memory, f, indent=2)
        
        print(f"✓ Pulled {len(memory['files'])} files (version {ctx['version']})")
    
    def get_history(self) -> List[Dict]:
        """Get context version history"""
        config = self.load_config()
        ctx = self.client.get_context(config["context_id"], history=True)
        return ctx.get("versions", [])
    
    def create_branch(self, branch_name: str, from_version: Optional[int] = None) -> Dict:
        """Create a context branch"""
        config = self.load_config()
        
        print(f"Creating branch '{branch_name}' from context {config['context_id']}...")
        
        new_ctx = self.client.create_context(config["context_id"], from_version)
        
        branch_config = {
            "context_id": new_ctx["id"],
            "project_name": f"{config['project_name']}-{branch_name}",
            "last_version": new_ctx["version"],
            "last_sync": 0,
            "file_message_map": {}
        }
        
        # Save branch config
        branch_file = f".cartographer_uc_config.{branch_name}.json"
        with open(branch_file, 'w') as f:
            json.dump(branch_config, f, indent=2)
        
        print(f"✓ Branch created: {new_ctx['id']}")
        print(f"✓ Config saved to {branch_file}")
        
        return branch_config


def example_usage():
    """Example usage of CMP + UC integration"""
    
    # Get API key from environment
    api_key = os.getenv("ULTRA_CONTEXT")
    if not api_key:
        print("❌ Set ULTRA_CONTEXT environment variable")
        return
    
    integration = CMPUCIntegration(api_key)
    
    # Example 1: Initialize project
    print("\n=== Example 1: Initialize Project ===")
    try:
        config = integration.init_project("my-python-project")
        print(f"Context ID: {config['context_id']}")
    except Exception as e:
        print(f"Already initialized or error: {e}")
    
    # Example 2: Push to UC
    print("\n=== Example 2: Push to UC ===")
    try:
        integration.push_to_uc()
    except Exception as e:
        print(f"Error: {e}")
    
    # Example 3: View history
    print("\n=== Example 3: View History ===")
    try:
        history = integration.get_history()
        for version in history:
            print(f"v{version['version']} - {version['operation']} - {version['timestamp']}")
    except Exception as e:
        print(f"Error: {e}")
    
    # Example 4: Create branch
    print("\n=== Example 4: Create Branch ===")
    try:
        integration.create_branch("feature-x")
    except Exception as e:
        print(f"Error: {e}")
    
    # Example 5: Pull from UC
    print("\n=== Example 5: Pull from UC ===")
    try:
        integration.pull_from_uc()
    except Exception as e:
        print(f"Error: {e}")


def example_with_openai():
    """Example: Use UC context with OpenAI"""
    import openai
    
    api_key = os.getenv("ULTRA_CONTEXT")
    if not api_key:
        print("❌ Set ULTRA_CONTEXT environment variable")
        return
    
    integration = CMPUCIntegration(api_key)
    config = integration.load_config()
    
    # Get context from UC
    ctx = integration.client.get_context(config["context_id"])
    
    # Convert to OpenAI messages format
    messages = []
    for msg in ctx.get("data", []):
        if msg.get("type") == "file":
            messages.append({
                "role": "system",
                "content": f"File: {msg['path']}\n\n{msg['content']}"
            })
    
    # Add user query
    messages.append({
        "role": "user",
        "content": "Explain the main architecture of this codebase"
    })
    
    # Call OpenAI (requires openai package and API key)
    # response = openai.ChatCompletion.create(
    #     model="gpt-4",
    #     messages=messages
    # )
    # print(response.choices[0].message.content)
    
    print(f"✓ Prepared {len(messages)} messages for OpenAI")


if __name__ == "__main__":
    example_usage()
    # example_with_openai()
