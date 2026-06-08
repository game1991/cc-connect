# cc-connect one-click reinstall script (PowerShell)
# Usage: powershell -File scripts\reinstall.ps1 [-Yes] [-DryRun]
#   -Yes     skip confirmation
#   -DryRun  only preview, no side effects

param(
  [switch]$Yes = $false,
  [switch]$DryRun = $false
)

$ErrorActionPreference = 'Stop'
$PKG = "@game1991/cc-connect"
$CC_DIR = "$env:USERPROFILE\.cc-connect"
$BACKUP_DIR = "$env:USERPROFILE\.cc-connect-backup"

function Run-Command {
  param([string[]]$Cmd)
  if ($DryRun) {
    Write-Host "  [DRY-RUN] $($Cmd -join ' ')" -ForegroundColor DarkGray
  } else {
    & @Cmd
  }
}

# ── Step 0: Pre-check ──────────────────────────────────────
Write-Host "=== cc-connect Reinstall ===" -ForegroundColor Cyan
Write-Host ""
Write-Host "This will:"
Write-Host "  1. Backup config.toml and crons/"
Write-Host "  2. Stop and uninstall the daemon"
Write-Host "  3. Uninstall the npm package"
Write-Host "  4. Clean runtime state"
Write-Host "  5. Reinstall via npm"
Write-Host "  6. Restore config and crons"
Write-Host "  7. Reinstall the daemon"
Write-Host ""

if (-not $Yes) {
  $reply = Read-Host "Continue? [y/N]"
  if ($reply -notmatch '^[yY]$') {
    Write-Host "Cancelled."
    exit 0
  }
}

# ── Step 1: Backup user data FIRST (before any destructive operation) ──
Write-Host ""
Write-Host "[1/7] Backing up user data..."
$timestamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
$backupPath = Join-Path $BACKUP_DIR $timestamp

if (-not $DryRun) {
  New-Item -ItemType Directory -Path $backupPath -Force | Out-Null
}

if (Test-Path "$CC_DIR\config.toml") {
  Run-Command @("Copy-Item", "$CC_DIR\config.toml", "$backupPath\config.toml", "-Force")
  Write-Host "  Backed up: config.toml"
}

if (Test-Path "$CC_DIR\crons\jobs.json") {
  if (-not $DryRun) { New-Item -ItemType Directory -Path "$backupPath\crons" -Force | Out-Null }
  Run-Command @("Copy-Item", "$CC_DIR\crons\jobs.json", "$backupPath\crons-jobs.json", "-Force")
  Write-Host "  Backed up: crons/jobs.json"
}

if (Test-Path "$CC_DIR\dir_history.json") {
  Run-Command @("Copy-Item", "$CC_DIR\dir_history.json", "$backupPath\dir_history.json", "-Force")
  Write-Host "  Backed up: dir_history.json"
}

Write-Host "  Backup location: $backupPath"

# ── Step 2: Stop & uninstall daemon + clean runtime state ──────
Write-Host ""
Write-Host "[2/7] Stopping daemon and cleaning runtime state..."

# Strategy: try cc-connect clean (fork.9+), fall back to manual steps for older binaries
$cleanOk = $false
try {
  Run-Command @("cc-connect", "clean")
  $cleanOk = $true
  Write-Host "  Cleaned via cc-connect clean" -ForegroundColor Green
} catch {
  Write-Host "  cc-connect clean not available or failed, using manual fallback..." -ForegroundColor Yellow
  # daemon uninstall includes stop logic (all fork versions)
  try { Run-Command @("cc-connect", "daemon", "uninstall") } catch {}
  # restart --force triggers PID-kill fallback for stuck processes (fork.4+)
  try { Run-Command @("cc-connect", "daemon", "restart", "--force") } catch {}
}

# Always explicitly remove residual runtime files (idempotent — safe even after clean)
Write-Host "  Removing residual files..."
Run-Command @("Remove-Item", "-Force", "$CC_DIR\daemon.json", "-ErrorAction", "SilentlyContinue")
Run-Command @("Remove-Item", "-Force", "$CC_DIR\.config.toml.lock", "-ErrorAction", "SilentlyContinue")
Run-Command @("Remove-Item", "-Force", "$CC_DIR\cc-connect-daemon.ps1", "-ErrorAction", "SilentlyContinue")
Run-Command @("Remove-Item", "-Recurse", "-Force", "$CC_DIR\run", "-ErrorAction", "SilentlyContinue")
Run-Command @("Remove-Item", "-Recurse", "-Force", "$CC_DIR\logs", "-ErrorAction", "SilentlyContinue")
Run-Command @("Remove-Item", "-Force", "$CC_DIR\dir_history.json", "-ErrorAction", "SilentlyContinue")
if (Test-Path "$CC_DIR\providers") {
  Write-Host "  Note: ~/.cc-connect/providers/ exists (not removed — review manually if needed)" -ForegroundColor DarkYellow
}

# ── Step 3: Uninstall npm package ──────────────────────────
Write-Host ""
Write-Host "[3/7] Uninstalling npm package..."
Run-Command @("npm", "uninstall", "-g", $PKG)

# ── Step 4: Verify process exit (Windows file locks) ───────
Write-Host ""
Write-Host "[4/7] Verifying process exit..."
if (-not $DryRun) {
  for ($i = 1; $i -le 15; $i++) {
    $proc = Get-Process -Name "cc-connect" -ErrorAction SilentlyContinue
    if (-not $proc) { break }
    Start-Sleep -Seconds 1
  }
  $proc = Get-Process -Name "cc-connect" -ErrorAction SilentlyContinue
  if ($proc) {
    Write-Host "  Warning: cc-connect process still running after 15s." -ForegroundColor Yellow
    Write-Host "  Attempting force kill..." -ForegroundColor Yellow
    try {
      Stop-Process -Name "cc-connect" -Force -ErrorAction SilentlyContinue
      Start-Sleep -Seconds 2
      $proc = Get-Process -Name "cc-connect" -ErrorAction SilentlyContinue
      if ($proc) {
        Write-Host "  ERROR: Could not kill cc-connect process." -ForegroundColor Red
        Write-Host "  File locks may prevent install. Please kill it manually and re-run."
      }
    } catch {}
  }
}

# ── Step 5: Reinstall ──────────────────────────────────────
Write-Host ""
Write-Host "[5/7] Installing latest version..."
try {
  Run-Command @("npm", "install", "-g", $PKG)
  if (-not $DryRun) {
    $version = & cc-connect --version 2>$null
    $binPath = (Get-Command cc-connect -ErrorAction SilentlyContinue).Source
    Write-Host "  Installed: $version" -ForegroundColor Green
    Write-Host "  Binary:     $binPath"
  }
} catch {
  Write-Host ""
  Write-Host "ERROR: npm install failed. Check your ~/.npmrc and network." -ForegroundColor Red
  Write-Host "  Your config backup is at: $backupPath"
  Write-Host "  To retry manually:"
  Write-Host "    npm install -g $PKG"
  exit 1
}

# ── Step 6: Restore user data ───────────────────────────────
Write-Host ""
Write-Host "[6/7] Restoring user data..."
if (Test-Path "$backupPath\config.toml") {
  if (-not $DryRun) { New-Item -ItemType Directory -Path $CC_DIR -Force | Out-Null }
  Run-Command @("Copy-Item", "$backupPath\config.toml", "$CC_DIR\config.toml", "-Force")
  Write-Host "  Restored: config.toml"
}

if (Test-Path "$backupPath\crons-jobs.json") {
  if (-not $DryRun) { New-Item -ItemType Directory -Path "$CC_DIR\crons" -Force | Out-Null }
  Run-Command @("Copy-Item", "$backupPath\crons-jobs.json", "$CC_DIR\crons\jobs.json", "-Force")
  Write-Host "  Restored: crons/jobs.json"
}

if (Test-Path "$backupPath\dir_history.json") {
  Run-Command @("Copy-Item", "$backupPath\dir_history.json", "$CC_DIR\dir_history.json", "-Force")
  Write-Host "  Restored: dir_history.json"
}

# ── Step 7: Reinstall daemon ───────────────────────────────
Write-Host ""
Write-Host "[7/7] Reinstalling daemon..."
try {
  Run-Command @("cc-connect", "daemon", "install")
  if (-not $DryRun) { & cc-connect daemon status }
} catch {
  Write-Host "  Warning: daemon install failed. You may need to run it manually." -ForegroundColor Yellow
}

# ── Summary ────────────────────────────────────────────────
Write-Host ""
Write-Host "=== Reinstall Complete ===" -ForegroundColor Cyan
if (-not $DryRun) {
  $finalVer = & cc-connect --version 2>$null
  $finalBin = (Get-Command cc-connect -ErrorAction SilentlyContinue).Source
  Write-Host "  Version: $finalVer"
  Write-Host "  Binary:  $finalBin"
}
Write-Host "  Config:  $CC_DIR\config.toml"
Write-Host "  Backup:  $backupPath"
Write-Host ""
