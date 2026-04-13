# CMP Installation Script for Windows
# Run this script to install CMP globally

Write-Host "========================================" -ForegroundColor Cyan
Write-Host "CMP Installation Script" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Check if Rust is installed
Write-Host "[1/4] Checking Rust installation..." -ForegroundColor Yellow
if (!(Get-Command cargo -ErrorAction SilentlyContinue)) {
    Write-Host "❌ Rust not found. Please install Rust first:" -ForegroundColor Red
    Write-Host "   https://rustup.rs/" -ForegroundColor Red
    exit 1
}
Write-Host "✓ Rust found" -ForegroundColor Green
Write-Host ""

# Build CMP
Write-Host "[2/4] Building CMP (this may take a few minutes)..." -ForegroundColor Yellow
Push-Location cmp
$buildResult = cargo build --release 2>&1
Pop-Location

if ($LASTEXITCODE -ne 0) {
    Write-Host "❌ Build failed" -ForegroundColor Red
    Write-Host $buildResult
    exit 1
}
Write-Host "✓ Build successful" -ForegroundColor Green
Write-Host ""

# Create bin directory
Write-Host "[3/4] Installing CMP..." -ForegroundColor Yellow
$binPath = "$env:USERPROFILE\.local\bin"
New-Item -ItemType Directory -Path $binPath -Force | Out-Null

# Copy binary
Copy-Item "cmp\target\release\cmp.exe" "$binPath\cmp.exe" -Force
Write-Host "✓ Binary copied to: $binPath\cmp.exe" -ForegroundColor Green
Write-Host ""

# Add to PATH
Write-Host "[4/4] Updating PATH..." -ForegroundColor Yellow
$currentPath = [Environment]::GetEnvironmentVariable("PATH", [EnvironmentVariableTarget]::User)
if ($currentPath -notlike "*$binPath*") {
    [Environment]::SetEnvironmentVariable("PATH", "$currentPath;$binPath", [EnvironmentVariableTarget]::User)
    Write-Host "✓ Added to PATH: $binPath" -ForegroundColor Green
} else {
    Write-Host "✓ Already in PATH: $binPath" -ForegroundColor Green
}
Write-Host ""

# Verify installation
Write-Host "========================================" -ForegroundColor Cyan
Write-Host "Installation Complete!" -ForegroundColor Green
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

# Refresh PATH for current session
$env:PATH = [Environment]::GetEnvironmentVariable("PATH", [EnvironmentVariableTarget]::User)

# Test command
Write-Host "Testing installation..." -ForegroundColor Yellow
$version = & cmp --version 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "✓ CMP is working: $version" -ForegroundColor Green
} else {
    Write-Host "⚠️  Please restart your terminal for PATH changes to take effect" -ForegroundColor Yellow
}
Write-Host ""

Write-Host "Next steps:" -ForegroundColor Cyan
Write-Host "  1. Restart your terminal (if needed)" -ForegroundColor White
Write-Host "  2. Set your UC API key:" -ForegroundColor White
Write-Host "     echo 'ULTRA_CONTEXT=uc_live_your_key' > .env.local" -ForegroundColor Gray
Write-Host "  3. Initialize your project:" -ForegroundColor White
Write-Host "     cmp init --cloud --project my-project" -ForegroundColor Gray
Write-Host "  4. Start using CMP:" -ForegroundColor White
Write-Host "     cmp source && cmp push" -ForegroundColor Gray
Write-Host ""
Write-Host "Documentation: UC_INTEGRATION.md" -ForegroundColor Cyan
Write-Host "Quick Start: QUICKSTART.md" -ForegroundColor Cyan
Write-Host ""
