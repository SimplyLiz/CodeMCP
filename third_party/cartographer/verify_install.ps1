Write-Host "========================================" -ForegroundColor Cyan
Write-Host "CMP Installation Verification" -ForegroundColor Cyan
Write-Host "========================================" -ForegroundColor Cyan
Write-Host ""

$allPassed = $true

Write-Host "[Test 1] Checking if CMP is in PATH..." -ForegroundColor Yellow
$cmpPath = Get-Command cmp -ErrorAction SilentlyContinue
if ($cmpPath) {
    Write-Host "PASS: CMP found at $($cmpPath.Source)" -ForegroundColor Green
} else {
    Write-Host "FAIL: CMP not found in PATH" -ForegroundColor Red
    $allPassed = $false
}
Write-Host ""

Write-Host "[Test 2] Checking CMP version..." -ForegroundColor Yellow
$version = cmp --version 2>&1
if ($LASTEXITCODE -eq 0) {
    Write-Host "PASS: $version" -ForegroundColor Green
} else {
    Write-Host "FAIL: Could not get version" -ForegroundColor Red
    $allPassed = $false
}
Write-Host ""

Write-Host "[Test 3] Checking help command..." -ForegroundColor Yellow
$help = cmp --help 2>&1
if ($help -match "Memory Unit") {
    Write-Host "PASS: Help command works" -ForegroundColor Green
} else {
    Write-Host "FAIL: Help command output unexpected" -ForegroundColor Red
    $allPassed = $false
}
Write-Host ""

Write-Host "[Test 4] Checking UC commands..." -ForegroundColor Yellow
$ucCommands = @("init", "push", "pull", "history", "branch", "diff", "agents", "analytics", "optimize")
$ucPassed = $true
foreach ($cmd in $ucCommands) {
    $cmdHelp = cmp $cmd --help 2>&1
    if ($LASTEXITCODE -ne 0) {
        Write-Host "  Command '$cmd' not found" -ForegroundColor Red
        $ucPassed = $false
        $allPassed = $false
    }
}
if ($ucPassed) {
    Write-Host "PASS: All UC commands available" -ForegroundColor Green
}
Write-Host ""

Write-Host "[Test 5] Checking UC API key..." -ForegroundColor Yellow
if (Test-Path ".env.local") {
    $envContent = Get-Content ".env.local" -Raw
    if ($envContent -match "ULTRA_CONTEXT=") {
        Write-Host "PASS: .env.local found with UC API key" -ForegroundColor Green
    } else {
        Write-Host "WARNING: .env.local exists but no ULTRA_CONTEXT key" -ForegroundColor Yellow
    }
} else {
    Write-Host "WARNING: .env.local not found (optional)" -ForegroundColor Yellow
}
Write-Host ""

Write-Host "========================================" -ForegroundColor Cyan
if ($allPassed) {
    Write-Host "All Tests Passed!" -ForegroundColor Green
    Write-Host ""
    Write-Host "CMP is ready to use!" -ForegroundColor Green
    Write-Host ""
    Write-Host "Try these commands:" -ForegroundColor Cyan
    Write-Host "  cmp --version" -ForegroundColor White
    Write-Host "  cmp --help" -ForegroundColor White
    Write-Host "  cmp map" -ForegroundColor White
} else {
    Write-Host "Some Tests Failed" -ForegroundColor Red
    Write-Host ""
    Write-Host "Please:" -ForegroundColor Yellow
    Write-Host "  1. Restart your terminal" -ForegroundColor White
    Write-Host "  2. Run .\install.ps1 again" -ForegroundColor White
}
Write-Host "========================================" -ForegroundColor Cyan
