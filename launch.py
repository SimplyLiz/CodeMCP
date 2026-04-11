#!/usr/bin/env python3
"""
Cartographer Installation Script
Builds and installs the cartographer binary for Linux, macOS, and Windows.
"""

import os
import platform
import shutil
import subprocess
import sys

BINARY_NAME = "cartographer"
CARGO_DIR = os.path.join("mapper-core", "cartographer")

# Default location for ContextCompressionEngine relative to this script.
# Users can override with --cce-path <dir>.
_SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
DEFAULT_CCE_DIR = os.path.normpath(os.path.join(_SCRIPT_DIR, "..", "ContextCompressionEngine"))


def step(n: int, total: int, msg: str):
    print(f"[{n}/{total}] {msg}...")


def check_cargo():
    if not shutil.which("cargo"):
        print("Rust not found. Install it first:")
        print("  curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh")
        sys.exit(1)
    print("  Rust found")


def check_node() -> bool:
    """Return True if Node.js 20+ is available; print a warning and return False otherwise."""
    node = shutil.which("node")
    if not node:
        print("  Node.js not found — CCE compression will be unavailable.")
        print("  Install Node.js 20+ from https://nodejs.org to enable it.")
        return False
    result = subprocess.run([node, "--version"], capture_output=True, text=True)
    version_str = result.stdout.strip().lstrip("v")
    try:
        major = int(version_str.split(".")[0])
    except ValueError:
        major = 0
    if major < 20:
        print(f"  Node.js {version_str} found, but 20+ is required for CCE.")
        print("  Upgrade Node.js to enable CCE compression.")
        return False
    print(f"  Node.js {version_str} found")
    return True


def setup_cce(cce_dir: str) -> bool:
    """
    Build ContextCompressionEngine and save its dist path.
    Returns True on success, False if CCE is unavailable/skipped.
    """
    if not os.path.isdir(cce_dir):
        print(f"  ContextCompressionEngine not found at: {cce_dir}")
        print("  CCE compression will be unavailable.")
        print(f"  Pass --cce-path <dir> to specify its location, or clone it to {cce_dir}")
        return False

    pkg = os.path.join(cce_dir, "package.json")
    if not os.path.isfile(pkg):
        print(f"  {cce_dir} does not look like a valid CCE directory (no package.json).")
        return False

    npm = shutil.which("npm")
    if not npm:
        print("  npm not found — cannot build CCE.")
        return False

    # Install deps
    print(f"  Installing CCE dependencies in {cce_dir}...")
    r = subprocess.run([npm, "install"], cwd=cce_dir)
    if r.returncode != 0:
        print("  npm install failed.")
        return False

    # Build
    dist_dir = os.path.join(cce_dir, "dist")
    if not os.path.isdir(dist_dir):
        print("  Building CCE...")
        r = subprocess.run([npm, "run", "build"], cwd=cce_dir)
        if r.returncode != 0:
            print("  CCE build failed.")
            return False
    else:
        print("  CCE already built")

    # Persist the dist path so compressor.py can find it
    config_dir = os.path.join(_SCRIPT_DIR, ".cartographer")
    os.makedirs(config_dir, exist_ok=True)
    config_file = os.path.join(config_dir, "cce_dist")
    with open(config_file, "w", encoding="utf-8") as f:
        f.write(dist_dir)
    print(f"  CCE dist path saved to .cartographer/cce_dist")
    return True


def build():
    result = subprocess.run(
        ["cargo", "build", "--release"],
        cwd=CARGO_DIR,
    )
    if result.returncode != 0:
        print("Build failed.")
        sys.exit(1)
    print("  Build successful")


def get_binary_src() -> str:
    if platform.system() == "Windows":
        return os.path.join(CARGO_DIR, "target", "release", f"{BINARY_NAME}.exe")
    return os.path.join(CARGO_DIR, "target", "release", BINARY_NAME)


def get_install_dir() -> str:
    if platform.system() == "Windows":
        local_app = os.environ.get("LOCALAPPDATA", os.path.expanduser("~"))
        return os.path.join(local_app, "Programs", BINARY_NAME)
    return os.path.join(os.path.expanduser("~"), ".local", "bin")


def install_binary(src: str, install_dir: str) -> str:
    os.makedirs(install_dir, exist_ok=True)
    dest_name = f"{BINARY_NAME}.exe" if platform.system() == "Windows" else BINARY_NAME
    dest = os.path.join(install_dir, dest_name)
    shutil.copy2(src, dest)
    if platform.system() != "Windows":
        os.chmod(dest, 0o755)
    print(f"  Binary installed: {dest}")
    return install_dir


def update_path(install_dir: str):
    system = platform.system()

    if system == "Windows":
        # Inform the user; modifying system PATH on Windows requires elevation
        print(f"  Add to PATH manually: {install_dir}")
        print("  Or run: [System.Environment]::SetEnvironmentVariable('PATH', $env:PATH + ';{install_dir}', 'User')")
        return

    export_line = f'export PATH="{install_dir}:$PATH"'
    shell = os.environ.get("SHELL", "")
    candidates = []
    if "zsh" in shell:
        candidates = [os.path.expanduser("~/.zshrc"), os.path.expanduser("~/.bashrc")]
    else:
        candidates = [os.path.expanduser("~/.bashrc"), os.path.expanduser("~/.zshrc")]

    for rc in candidates:
        if os.path.exists(rc):
            with open(rc, "r") as f:
                content = f.read()
            if install_dir in content:
                print(f"  PATH already set in {rc}")
                return
            with open(rc, "a") as f:
                f.write(f"\n{export_line}\n")
            print(f"  PATH updated in {rc}")
            return

    # Fallback: write to the first candidate
    rc = candidates[0]
    with open(rc, "a") as f:
        f.write(f"\n{export_line}\n")
    print(f"  PATH updated in {rc}")


def verify(install_dir: str):
    dest_name = f"{BINARY_NAME}.exe" if platform.system() == "Windows" else BINARY_NAME
    binary_path = os.path.join(install_dir, dest_name)
    result = subprocess.run([binary_path, "--version"], capture_output=True, text=True)
    if result.returncode == 0:
        print(f"  {result.stdout.strip()}")
    else:
        print("  Restart your terminal for PATH changes to take effect")


def main():
    # Parse optional --cce-path argument
    cce_dir = DEFAULT_CCE_DIR
    args = sys.argv[1:]
    for i, arg in enumerate(args):
        if arg == "--cce-path" and i + 1 < len(args):
            cce_dir = os.path.abspath(args[i + 1])

    print("=" * 48)
    print("  Cartographer Installation")
    print("=" * 48)
    print()

    total = 6
    step(1, total, "Checking Rust")
    check_cargo()
    print()

    step(2, total, "Building Cartographer (this may take a few minutes)")
    build()
    print()

    step(3, total, "Installing")
    src = get_binary_src()
    install_dir = get_install_dir()
    install_binary(src, install_dir)
    update_path(install_dir)
    print()

    step(4, total, "Verifying Cartographer")
    verify(install_dir)
    print()

    step(5, total, "Checking Node.js (required for CCE compression)")
    node_ok = check_node()
    print()

    step(6, total, "Setting up ContextCompressionEngine")
    if node_ok:
        setup_cce(cce_dir)
    else:
        print("  Skipped (Node.js unavailable)")
    print()

    print("=" * 48)
    print("  Installation complete!")
    print("=" * 48)
    print()
    print("Next steps:")
    print("  1. Restart your terminal (if needed)")
    print("  2. Set your UltraContext API key:")
    print("       cartographer init --cloud --project my-project")
    print("  3. Generate your first context:")
    print("       cartographer source")
    print("  4. Compress a conversation with CCE:")
    print("       python compressor.py --messages chat.json --token-budget 8000")
    print("  5. Push to cloud:")
    print("       cartographer push")
    print("  6. Start MCP server:")
    print("       cartographer serve")
    print()


if __name__ == "__main__":
    main()
