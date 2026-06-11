package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/chenhg5/cc-connect/daemon"
)

func runReinstall(args []string) {
	yes := false
	for _, arg := range args {
		switch arg {
		case "--yes", "-y":
			yes = true
		}
	}

	pkg := "@game1991/cc-connect"
	dataDir := daemon.DefaultDataDir()

	fmt.Println("=== cc-connect Reinstall ===")
	fmt.Println()
	fmt.Println("This will prepare a full reinstall:")
	fmt.Println("  1. Back up config.toml, crons/, dir_history.json")
	fmt.Println("  2. Stop and uninstall the daemon")
	fmt.Println("  3. Clean runtime state")
	fmt.Println("  4. Generate a script to complete the reinstall")
	fmt.Println()

	if !yes && !confirmPrompt("Continue? [y/N] ") {
		fmt.Println("Cancelled.")
		return
	}

	// Step 1: Back up user data BEFORE any destructive operation.
	fmt.Println()
	fmt.Println("[1/3] Backing up user data...")
	timestamp := time.Now().Format("20060102-150405")
	backupDir := filepath.Join(filepath.Dir(dataDir), ".cc-connect-backup", timestamp)
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create backup directory: %v\n", err)
		os.Exit(1)
	}
	_ = os.Chmod(backupDir, 0700)

	backedUp := 0
	for _, bi := range []struct{ src, name string }{
		{filepath.Join(dataDir, "config.toml"), "config.toml"},
		{filepath.Join(dataDir, "crons", "jobs.json"), "crons-jobs.json"},
		{filepath.Join(dataDir, "dir_history.json"), "dir_history.json"},
	} {
		data, err := os.ReadFile(bi.src)
		if err != nil {
			continue
		}
		if err := os.WriteFile(filepath.Join(backupDir, bi.name), data, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to back up %s: %v\n", bi.name, err)
			continue
		}
		fmt.Printf("  Backed up: %s\n", bi.name)
		backedUp++
	}

	// Save daemon meta for Phase 2 restoration.
	if meta, err := daemon.LoadMeta(); err == nil {
		metaData, _ := json.Marshal(meta)
		if err := os.WriteFile(filepath.Join(backupDir, "daemon-meta.json"), metaData, 0600); err == nil {
			fmt.Println("  Backed up: daemon-meta.json")
			backedUp++
		}
	}

	fmt.Printf("  Backup location: %s\n", shortPath(backupDir))

	// Step 2: Clean runtime (includes daemon stop + uninstall + kill residual + remove files).
	fmt.Println()
	fmt.Println("[2/3] Cleaning runtime state...")
	cleanRuntime()

	// Step 3: Generate completion scripts into the backup directory (0700, safe from symlink attacks).
	fmt.Println()
	fmt.Println("[3/3] Generating completion script...")
	bashContent := fmt.Sprintf(bashScriptTmpl, filepath.ToSlash(backupDir), filepath.ToSlash(dataDir), pkg)
	bashPath := filepath.Join(backupDir, "reinstall.sh")
	if err := os.WriteFile(bashPath, []byte(bashContent), 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot write completion script: %v\n", err)
		os.Exit(1)
	}

	psContent := fmt.Sprintf(psScriptTmpl, filepath.ToSlash(backupDir), filepath.ToSlash(dataDir), pkg)
	psPath := filepath.Join(backupDir, "reinstall.ps1")
	_ = os.WriteFile(psPath, []byte(psContent), 0600)

	// On Unix: try exec to get one-command experience.
	if runtime.GOOS != "windows" {
		fmt.Println()
		fmt.Println("Completing reinstall...")
		if err := syscall.Exec("/usr/bin/env", []string{"env", "bash", bashPath}, os.Environ()); err != nil {
			printCompletionInstructions(bashPath, psPath)
		}
		return // unreachable if exec succeeds
	}

	printCompletionInstructions(bashPath, psPath)
}

func printCompletionInstructions(bashPath, psPath string) {
	bashDisplay := filepath.ToSlash(bashPath)
	psDisplay := filepath.ToSlash(psPath)
	isMINGW := runtime.GOOS == "windows" && os.Getenv("MSYSTEM") != ""

	if isMINGW {
		bashDisplay = toMINGWPath(bashDisplay)
	}

	fmt.Println()
	fmt.Println("=== Preparation Complete ===")
	fmt.Println()
	fmt.Println("Run ONE of the following to complete reinstall:")
	fmt.Println()
	if isMINGW {
		fmt.Printf("  bash '%s'\n", bashDisplay)
	} else if runtime.GOOS != "windows" {
		fmt.Printf("  bash '%s'\n", bashDisplay)
	} else {
		fmt.Printf("  powershell -ExecutionPolicy Bypass -File '%s'\n", psDisplay)
	}
	fmt.Println()
	fmt.Println("Alternative:")
	if isMINGW || runtime.GOOS != "windows" {
		fmt.Printf("  powershell -ExecutionPolicy Bypass -File '%s'\n", psDisplay)
	} else {
		fmt.Printf("  bash '%s'\n", toMINGWPath(bashDisplay))
	}
	fmt.Println()
}

func toMINGWPath(path string) string {
	if len(path) < 3 || path[1] != ':' {
		return path
	}
	return "/" + strings.ToLower(path[:1]) + path[2:]
}

const bashScriptTmpl = `#!/usr/bin/env bash
set -uo pipefail

BACKUP_DIR='%s'
CC_DIR='%s'
PKG="%s"

echo "=== cc-connect Reinstall (Completion) ==="
echo ""

# Safety: ensure no cc-connect is still running
if pgrep -f cc-connect > /dev/null 2>&1; then
  echo "Waiting for cc-connect process to exit..."
  for i in $(seq 1 15); do
    if ! pgrep -f cc-connect > /dev/null 2>&1; then break; fi
    sleep 1
  done
  if pgrep -f cc-connect > /dev/null 2>&1; then
    echo "ERROR: cc-connect still running. Kill it manually and re-run this script."
    exit 1
  fi
fi

# Detect npm global prefix write permission
NPM_PREFIX=$(npm prefix -g 2>/dev/null || echo "")
if [ -n "$NPM_PREFIX" ] && [ ! -w "$NPM_PREFIX" ]; then
  echo "Notice: npm global directory ($NPM_PREFIX) is not writable."
  echo "Will try with sudo. If it fails, run: sudo bash '$0'"
  echo ""
  SUDO="sudo"
else
  SUDO=""
fi

echo "[1/4] Uninstalling npm package..."
$SUDO npm uninstall -g "$PKG"

echo ""
echo "[2/4] Installing latest version..."
if $SUDO npm install -g "$PKG"; then
  VERSION=$(cc-connect --version 2>/dev/null || echo "unknown")
  echo "  Installed: $VERSION"
else
  echo ""
  echo "ERROR: npm install failed."
  echo "  Your backup is at: $BACKUP_DIR"
  exit 1
fi

echo ""
echo "[3/4] Restoring user data..."
if [ -f "$BACKUP_DIR/config.toml" ]; then
  mkdir -p "$CC_DIR"
  cp "$BACKUP_DIR/config.toml" "$CC_DIR/config.toml"
  echo "  Restored: config.toml"
fi

if [ -f "$BACKUP_DIR/crons-jobs.json" ]; then
  mkdir -p "$CC_DIR/crons"
  cp "$BACKUP_DIR/crons-jobs.json" "$CC_DIR/crons/jobs.json"
  echo "  Restored: crons/jobs.json"
fi

if [ -f "$BACKUP_DIR/dir_history.json" ]; then
  cp "$BACKUP_DIR/dir_history.json" "$CC_DIR/dir_history.json"
  echo "  Restored: dir_history.json"
fi

echo ""
echo "[4/4] Reinstalling daemon..."
DAEMON_META="$BACKUP_DIR/daemon-meta.json"
INSTALL_ARGS=""
if [ -f "$DAEMON_META" ]; then
  WORK_DIR=$(python3 -c "import json,sys; d=json.load(open(sys.argv[1])); print(d.get('work_dir',''))" "$DAEMON_META" 2>/dev/null || echo "")
  if [ -n "$WORK_DIR" ]; then
    INSTALL_ARGS="$INSTALL_ARGS --work-dir '$WORK_DIR'"
  fi
  CONFIG_FILE=$(python3 -c "import json,sys; d=json.load(open(sys.argv[1])); print(d.get('config_file',''))" "$DAEMON_META" 2>/dev/null || echo "")
  if [ -n "$CONFIG_FILE" ]; then
    INSTALL_ARGS="$INSTALL_ARGS --config '$CONFIG_FILE'"
  fi
fi

eval cc-connect daemon install $INSTALL_ARGS
cc-connect daemon status

echo ""
echo "=== Reinstall Complete ==="
echo "  Version: $(cc-connect --version 2>/dev/null || echo 'unknown')"
echo "  Config:  $CC_DIR/config.toml"
echo "  Backup:  $BACKUP_DIR"
echo ""
`

const psScriptTmpl = `$ErrorActionPreference = 'Stop'
$backupDir = '%s'
$ccDir = '%s'
$pkg = '%s'

Write-Host "=== cc-connect Reinstall (Completion) ===" -ForegroundColor Cyan
Write-Host ""

# Safety: ensure no cc-connect is still running
$proc = Get-Process -Name "cc-connect" -ErrorAction SilentlyContinue
if ($proc) {
  Write-Host "Waiting for cc-connect process to exit..." -ForegroundColor Yellow
  for ($i = 1; $i -le 15; $i++) {
    $proc = Get-Process -Name "cc-connect" -ErrorAction SilentlyContinue
    if (-not $proc) { break }
    Start-Sleep -Seconds 1
  }
  $proc = Get-Process -Name "cc-connect" -ErrorAction SilentlyContinue
  if ($proc) {
    Write-Host "ERROR: cc-connect still running. Kill it manually and re-run this script." -ForegroundColor Red
    exit 1
  }
}

Write-Host "[1/4] Uninstalling npm package..."
npm uninstall -g $pkg

Write-Host ""
Write-Host "[2/4] Installing latest version..."
try {
  npm install -g $pkg
  $version = & cc-connect --version 2>$null
  Write-Host "  Installed: $version" -ForegroundColor Green
} catch {
  Write-Host ""
  Write-Host "ERROR: npm install failed." -ForegroundColor Red
  Write-Host "  Your backup is at: $backupDir"
  exit 1
}

Write-Host ""
Write-Host "[3/4] Restoring user data..."
if (Test-Path "$backupDir\config.toml") {
  New-Item -ItemType Directory -Path $ccDir -Force | Out-Null
  Copy-Item "$backupDir\config.toml" "$ccDir\config.toml" -Force
  Write-Host "  Restored: config.toml"
}

if (Test-Path "$backupDir\crons-jobs.json") {
  New-Item -ItemType Directory -Path "$ccDir\crons" -Force | Out-Null
  Copy-Item "$backupDir\crons-jobs.json" "$ccDir\crons\jobs.json" -Force
  Write-Host "  Restored: crons/jobs.json"
}

if (Test-Path "$backupDir\dir_history.json") {
  Copy-Item "$backupDir\dir_history.json" "$ccDir\dir_history.json" -Force
  Write-Host "  Restored: dir_history.json"
}

Write-Host ""
Write-Host "[4/4] Reinstalling daemon..."
$installArgs = @("daemon", "install")
$metaPath = Join-Path $backupDir "daemon-meta.json"
if (Test-Path $metaPath) {
  $meta = Get-Content $metaPath -Raw | ConvertFrom-Json
  if ($meta.work_dir) { $installArgs += "--work-dir", $meta.work_dir }
  if ($meta.config_file) { $installArgs += "--config", $meta.config_file }
}
& cc-connect @installArgs
& cc-connect daemon status

Write-Host ""
Write-Host "=== Reinstall Complete ===" -ForegroundColor Cyan
$finalVer = & cc-connect --version 2>$null
Write-Host "  Version: $finalVer"
Write-Host "  Config:  $ccDir\config.toml"
Write-Host "  Backup:  $backupDir"
Write-Host ""
`
