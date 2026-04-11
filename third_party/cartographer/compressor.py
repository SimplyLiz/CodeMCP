#!/usr/bin/env python3
"""
Cartographer context compressor.

Usage:
  python compressor.py [TARGET]
      Generate a deps snapshot for TARGET and save to state_key.md.

  python compressor.py [TARGET] --messages <file.json> --token-budget <N>
      Load messages from a JSON file, append cartographer context, compress
      with ContextCompressionEngine to fit N tokens, and save to state_key.md.

  python compressor.py --messages <file.json> --token-budget <N>
      Compress an existing messages file without adding cartographer context.
"""

import json
import os
import shutil
import subprocess
import sys


# ---------------------------------------------------------------------------
# Cartographer analysis
# ---------------------------------------------------------------------------

def get_cartographer_analysis(target: str) -> dict | None:
    """
    Run `cartographer deps <target> --format json` and return parsed JSON output.
    Returns None if cartographer is not available or command fails.
    """
    if not shutil.which("cartographer"):
        print("Warning: 'cartographer' not found in PATH. Skipping dependency analysis.")
        return None

    try:
        result = subprocess.run(
            ["cartographer", "deps", target, "--format", "json"],
            capture_output=True,
            text=True,
            timeout=30,
        )
        if result.returncode != 0:
            print(f"Warning: cartographer command failed: {result.stderr.strip()}")
            return None
        return json.loads(result.stdout)
    except subprocess.TimeoutExpired:
        print("Warning: cartographer command timed out.")
        return None
    except json.JSONDecodeError as e:
        print(f"Warning: Failed to parse cartographer output: {e}")
        return None
    except Exception as e:
        print(f"Warning: Unexpected error running cartographer: {e}")
        return None


def deps_to_xml(deps_output: dict) -> str:
    """Convert cartographer deps JSON to token-efficient XML."""
    node_id = deps_output.get("node_id", "")
    node_name = deps_output.get("node_name", "unknown")
    dependencies = deps_output.get("dependencies", [])

    node_type = "unknown"
    if node_id.startswith("cls:"):
        node_type = "class"
    elif node_id.startswith("fn:"):
        node_type = "function"
    elif node_id.startswith("mod:"):
        node_type = "module"

    parts = node_id.split(":")
    file_path = parts[1] if len(parts) > 1 else ""

    lines = ["<CURRENT_FOCUS>"]
    lines.append(f'  <NODE name="{node_name}" type="{node_type}" path="{file_path}">')

    if dependencies:
        lines.append(f'    <DEPS count="{len(dependencies)}">')
        for dep in dependencies:
            dep_name = dep.get("name", "")
            dep_type = dep.get("node_type", "")
            dep_path = dep.get("file_path", "")
            lines.append(f'      <DEP name="{dep_name}" type="{dep_type}" path="{dep_path}" />')
        lines.append("    </DEPS>")

    lines.append("  </NODE>")
    lines.append("</CURRENT_FOCUS>")
    return "\n".join(lines)


# ---------------------------------------------------------------------------
# CCE integration
# ---------------------------------------------------------------------------

def find_cce_dist() -> str | None:
    """
    Locate the built CCE dist directory.  Search order:
      1. CCE_DIST environment variable
      2. .cartographer/cce_dist config file (written by launch.py)
      3. Sibling directory ContextCompressionEngine/dist (dev layout)
    """
    # 1. Env var
    env = os.environ.get("CCE_DIST")
    if env and os.path.isdir(env):
        return env

    # 2. Config written by launch.py
    config_file = os.path.join(".cartographer", "cce_dist")
    if os.path.isfile(config_file):
        with open(config_file, encoding="utf-8") as f:
            path = f.read().strip()
        if os.path.isdir(path):
            return path

    # 3. Sibling-directory convention (dev layout)
    script_dir = os.path.dirname(os.path.abspath(__file__))
    sibling = os.path.join(script_dir, "..", "ContextCompressionEngine", "dist")
    sibling = os.path.normpath(sibling)
    if os.path.isdir(sibling):
        return sibling

    return None


def find_bridge_script() -> str | None:
    """Return the path to tools/cce_bridge.mjs, relative to this script."""
    script_dir = os.path.dirname(os.path.abspath(__file__))
    bridge = os.path.join(script_dir, "tools", "cce_bridge.mjs")
    return bridge if os.path.isfile(bridge) else None


def cce_compress(messages: list[dict], token_budget: int) -> list[dict] | None:
    """
    Compress a message array via the CCE bridge.
    Returns the compressed messages, or None if CCE is unavailable.
    """
    node = shutil.which("node")
    if not node:
        print("Warning: 'node' not found in PATH. Skipping CCE compression.")
        return None

    bridge = find_bridge_script()
    if not bridge:
        print("Warning: tools/cce_bridge.mjs not found. Skipping CCE compression.")
        return None

    cce_dist = find_cce_dist()
    if not cce_dist:
        print("Warning: CCE dist not found. Skipping CCE compression.")
        print("  Run launch.py to set it up, or set CCE_DIST env var.")
        return None

    payload = json.dumps({"messages": messages, "tokenBudget": token_budget})
    env = {**os.environ, "CCE_DIST": cce_dist}

    try:
        result = subprocess.run(
            [node, bridge],
            input=payload,
            capture_output=True,
            text=True,
            timeout=60,
            env=env,
        )
        if result.returncode != 0:
            print(f"Warning: CCE bridge failed: {result.stderr.strip()}")
            return None
        data = json.loads(result.stdout)
        if data.get("tokenCount") is not None:
            within = "yes" if data.get("withinBudget") else "no"
            print(
                f"  CCE: {len(messages)} → {len(data['messages'])} messages "
                f"| ~{data['tokenCount']} tokens | within budget: {within}"
            )
        return data["messages"]
    except subprocess.TimeoutExpired:
        print("Warning: CCE bridge timed out.")
        return None
    except (json.JSONDecodeError, KeyError) as e:
        print(f"Warning: Failed to parse CCE bridge output: {e}")
        return None
    except Exception as e:
        print(f"Warning: Unexpected error calling CCE bridge: {e}")
        return None


# ---------------------------------------------------------------------------
# Main pipeline
# ---------------------------------------------------------------------------

def compress_chat_log(
    target: str | None = None,
    messages_file: str | None = None,
    token_budget: int | None = None,
):
    """
    Generate a state snapshot.

    - If messages_file is given, load it as a message array.
    - If target is given, run cartographer and append it as a system message.
    - If token_budget is given and CCE is available, compress to fit.
    - Write the result to state_key.md.
    """
    messages: list[dict] = []

    # Load existing messages if provided
    if messages_file:
        try:
            with open(messages_file, encoding="utf-8") as f:
                messages = json.load(f)
            if not isinstance(messages, list):
                print(f"Error: {messages_file} must contain a JSON array.")
                sys.exit(1)
        except (OSError, json.JSONDecodeError) as e:
            print(f"Error: failed to load {messages_file}: {e}")
            sys.exit(1)

    # Append cartographer context as a system message
    if target:
        deps_output = get_cartographer_analysis(target)
        if deps_output:
            xml_block = deps_to_xml(deps_output)
            messages.append({"role": "system", "content": xml_block})
        else:
            messages.append({"role": "system", "content": "<!-- cartographer analysis unavailable -->"})

    # Compress with CCE if token budget is set
    if token_budget is not None and messages:
        compressed = cce_compress(messages, token_budget)
        if compressed is not None:
            messages = compressed

    # Serialise to state_key.md
    if not messages:
        output = "<!-- No state captured -->"
    elif messages_file or token_budget is not None:
        # Structured output: JSON array for downstream tools
        output = json.dumps(messages, indent=2, ensure_ascii=False)
    else:
        # Legacy plain-text output (no messages file, no budget)
        output = "\n\n".join(m.get("content", "") for m in messages)

    with open("state_key.md", "w", encoding="utf-8") as f:
        f.write(output)

    print("State snapshot saved to state_key.md")


# ---------------------------------------------------------------------------
# CLI
# ---------------------------------------------------------------------------

def _parse_args(argv: list[str]) -> tuple[str | None, str | None, int | None]:
    target = None
    messages_file = None
    token_budget = None
    i = 0
    while i < len(argv):
        arg = argv[i]
        if arg == "--messages" and i + 1 < len(argv):
            i += 1
            messages_file = argv[i]
        elif arg == "--token-budget" and i + 1 < len(argv):
            i += 1
            token_budget = int(argv[i])
        elif not arg.startswith("--") and target is None:
            target = arg
        i += 1
    return target, messages_file, token_budget


if __name__ == "__main__":
    _target, _messages_file, _token_budget = _parse_args(sys.argv[1:])
    compress_chat_log(
        target=_target,
        messages_file=_messages_file,
        token_budget=_token_budget,
    )
