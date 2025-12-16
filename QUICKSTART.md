# CKB Quick Start Guide

Get CKB running in 5 minutes. Just copy and paste the commands for your operating system.

---

## macOS

### Step 1: Install Go (if not already installed)

Open Terminal and run:

```bash
# Install Homebrew if you don't have it
/bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"

# Install Go
brew install go
```

### Step 2: Download and Build CKB

```bash
# Clone the repository
git clone https://github.com/anthropics/ckb.git
cd ckb

# Build CKB
go build -o ckb ./cmd/ckb

# Make it available everywhere (optional)
sudo cp ckb /usr/local/bin/
```

### Step 3: Set Up Your Project

Navigate to your code project and run:

```bash
cd /path/to/your/project

# Initialize CKB
ckb init
```

### Step 4: Generate Code Index (for Go projects)

```bash
# Install the Go indexer
go install github.com/sourcegraph/scip-go/cmd/scip-go@latest

# Generate the index
~/go/bin/scip-go --repository-root=.
```

### Step 5: Verify It Works

```bash
ckb status
```

You should see output showing your backends and symbol count.

---

## Linux (Ubuntu/Debian)

### Step 1: Install Go (if not already installed)

Open Terminal and run:

```bash
# Update package list
sudo apt update

# Install Go
sudo apt install -y golang-go

# Or install latest Go manually:
wget https://go.dev/dl/go1.21.5.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.21.5.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### Step 2: Install Git (if not already installed)

```bash
sudo apt install -y git
```

### Step 3: Download and Build CKB

```bash
# Clone the repository
git clone https://github.com/anthropics/ckb.git
cd ckb

# Build CKB
go build -o ckb ./cmd/ckb

# Make it available everywhere (optional)
sudo cp ckb /usr/local/bin/
```

### Step 4: Set Up Your Project

Navigate to your code project and run:

```bash
cd /path/to/your/project

# Initialize CKB
ckb init
```

### Step 5: Generate Code Index (for Go projects)

```bash
# Install the Go indexer
go install github.com/sourcegraph/scip-go/cmd/scip-go@latest

# Generate the index
~/go/bin/scip-go --repository-root=.
```

### Step 6: Verify It Works

```bash
ckb status
```

---

## Windows

### Step 1: Install Go

1. Download Go from: https://go.dev/dl/
2. Run the installer (e.g., `go1.21.5.windows-amd64.msi`)
3. Click "Next" through the installer
4. Restart your terminal/PowerShell after installation

### Step 2: Install Git

1. Download Git from: https://git-scm.com/download/win
2. Run the installer
3. Use default options (click "Next" through everything)
4. Restart your terminal/PowerShell after installation

### Step 3: Download and Build CKB

Open PowerShell and run:

```powershell
# Clone the repository
git clone https://github.com/anthropics/ckb.git
cd ckb

# Build CKB
go build -o ckb.exe ./cmd/ckb
```

### Step 4: Add CKB to Your PATH (optional)

```powershell
# Create a bin folder in your home directory
mkdir $HOME\bin -Force

# Copy CKB there
cp ckb.exe $HOME\bin\

# Add to PATH (run this once)
[Environment]::SetEnvironmentVariable("Path", $env:Path + ";$HOME\bin", "User")
```

Restart PowerShell for the PATH change to take effect.

### Step 5: Set Up Your Project

Navigate to your code project and run:

```powershell
cd C:\path\to\your\project

# Initialize CKB
ckb init
```

### Step 6: Generate Code Index (for Go projects)

```powershell
# Install the Go indexer
go install github.com/sourcegraph/scip-go/cmd/scip-go@latest

# Generate the index
& "$HOME\go\bin\scip-go.exe" --repository-root=.
```

### Step 7: Verify It Works

```powershell
ckb status
```

---

## Using CKB

Once set up, here are the most common commands:

### Search for Code

```bash
# Find a function or type by name
ckb search MyFunction

# Find with more results
ckb search MyFunction --limit 20
```

### Find References

```bash
# Find where a symbol is used
ckb refs MyFunction
```

### Check System Status

```bash
# See what backends are available
ckb status
```

### Get Architecture Overview

```bash
# See high-level code structure
ckb arch
```

### Run Diagnostics

```bash
# Check for issues
ckb doctor
```

---

## Using with Claude Code

CKB integrates directly with Claude Code via MCP (Model Context Protocol).

### Step 1: Find Your CKB Path

```bash
# macOS/Linux
which ckb
# or if not in PATH:
echo $(pwd)/ckb

# Windows PowerShell
Get-Command ckb | Select-Object -ExpandProperty Source
# or if not in PATH:
echo "$PWD\ckb.exe"
```

### Step 2: Configure Claude Code

Add CKB to your Claude Code MCP configuration:

**macOS/Linux** - Edit `~/.config/claude-code/mcp.json`:

```json
{
  "mcpServers": {
    "ckb": {
      "command": "/path/to/ckb",
      "args": ["mcp"],
      "cwd": "/path/to/your/project"
    }
  }
}
```

**Windows** - Edit `%APPDATA%\claude-code\mcp.json`:

```json
{
  "mcpServers": {
    "ckb": {
      "command": "C:\\path\\to\\ckb.exe",
      "args": ["mcp"],
      "cwd": "C:\\path\\to\\your\\project"
    }
  }
}
```

### Step 3: Restart Claude Code

Close and reopen Claude Code. CKB tools will now be available.

---

## Starting the HTTP API (Optional)

If you want to access CKB via HTTP:

```bash
# Start the server
ckb serve --port 8080

# In another terminal, test it:
curl http://localhost:8080/health
```

---

## Troubleshooting

### "command not found: ckb"

CKB isn't in your PATH. Either:
- Use the full path: `/path/to/ckb status`
- Or add it to your PATH (see installation steps above)

### "no SCIP index found"

You need to generate the code index:

```bash
# For Go projects:
go install github.com/sourcegraph/scip-go/cmd/scip-go@latest
~/go/bin/scip-go --repository-root=.
```

### "permission denied"

On macOS/Linux, you may need to make CKB executable:

```bash
chmod +x ckb
```

### Go not found after installation

Restart your terminal, or run:

```bash
# macOS/Linux
source ~/.bashrc  # or ~/.zshrc

# Windows: Close and reopen PowerShell
```

---

## Getting Help

```bash
# See all commands
ckb --help

# Get help for a specific command
ckb search --help
```

---

## Next Steps

- Read the full [README](README.md) for more details
- Check the [API documentation](API-README.md) for HTTP API usage
- Run `ckb doctor` to diagnose any issues
