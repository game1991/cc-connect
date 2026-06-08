#!/usr/bin/env bash
# cc-connect one-click reinstall script
# Usage: bash scripts/reinstall.sh [--yes] [--dry-run]
#   --yes     skip confirmation
#   --dry-run only preview, no side effects

set -uo pipefail

YES=false
DRYRUN=false
for arg in "$@"; do
  case "$arg" in
    --yes|-y)    YES=true ;;
    --dry-run)   DRYRUN=true ;;
  esac
done

PKG="@game1991/cc-connect"
CC_DIR="$HOME/.cc-connect"
BACKUP_DIR="$HOME/.cc-connect-backup"

run() {
  if $DRYRUN; then
    echo "  [DRY-RUN] $*"
  else
    "$@"
  fi
}

# ── Step 0: Pre-check ──────────────────────────────────────
echo "=== cc-connect Reinstall ==="
echo ""
echo "This will:"
echo "  1. Backup config.toml and crons/"
echo "  2. Stop and uninstall the daemon"
echo "  3. Uninstall the npm package"
echo "  4. Clean runtime state"
echo "  5. Reinstall via npm"
echo "  6. Restore config and crons"
echo "  7. Reinstall the daemon"
echo ""

if ! $YES; then
  read -r -p "Continue? [y/N] " reply
  reply="${reply%$'\r'}"
  case "$reply" in
    y|Y) ;;
    *)   echo "Cancelled."; exit 0 ;;
  esac
fi

# ── Step 1: Backup user data FIRST (before any destructive operation) ──
echo ""
echo "[1/7] Backing up user data..."
TIMESTAMP=$(date +%Y%m%d-%H%M%S-%N 2>/dev/null || date +%Y%m%d-%H%M%S)
run mkdir -p "$BACKUP_DIR/$TIMESTAMP"

if [ -f "$CC_DIR/config.toml" ]; then
  run cp "$CC_DIR/config.toml" "$BACKUP_DIR/$TIMESTAMP/config.toml"
  echo "  Backed up: config.toml"
fi

if [ -f "$CC_DIR/crons/jobs.json" ]; then
  run mkdir -p "$BACKUP_DIR/$TIMESTAMP/crons"
  run cp "$CC_DIR/crons/jobs.json" "$BACKUP_DIR/$TIMESTAMP/crons-jobs.json"
  echo "  Backed up: crons/jobs.json"
fi

if [ -f "$CC_DIR/dir_history.json" ]; then
  run cp "$CC_DIR/dir_history.json" "$BACKUP_DIR/$TIMESTAMP/dir_history.json"
  echo "  Backed up: dir_history.json"
fi

echo "  Backup location: $BACKUP_DIR/$TIMESTAMP"

# ── Step 2: Stop & uninstall daemon + clean runtime state ──────
echo ""
echo "[2/7] Stopping daemon and cleaning runtime state..."

# Strategy: try cc-connect clean (fork.9+), fall back to manual steps for older binaries
CLEAN_OK=false
if cc-connect clean 2>/dev/null; then
  CLEAN_OK=true
  echo "  Cleaned via cc-connect clean"
else
  echo "  cc-connect clean not available or failed, using manual fallback..."
  # daemon uninstall includes stop logic (all fork versions)
  run cc-connect daemon uninstall 2>/dev/null || true
  # restart --force triggers PID-kill fallback for stuck processes (fork.4+)
  run cc-connect daemon restart --force 2>/dev/null || true
fi

# Always explicitly remove residual runtime files (idempotent — safe even after clean)
echo "  Removing residual files..."
run rm -f "$CC_DIR/daemon.json" "$CC_DIR/.config.toml.lock" "$CC_DIR/cc-connect-daemon.ps1"
run rm -rf "$CC_DIR/run/" "$CC_DIR/logs/"
run rm -f "$CC_DIR/dir_history.json"
[ -d "$CC_DIR/providers" ] && echo "  Note: ~/.cc-connect/providers/ exists (not removed — review manually if needed)"

# ── Step 3: Uninstall npm package ──────────────────────────
echo ""
echo "[3/7] Uninstalling npm package..."
run npm uninstall -g "$PKG"

# ── Step 4: Verify process exit ────────────────────────────
echo ""
echo "[4/7] Verifying process exit..."
if $DRYRUN; then
  echo "  [DRY-RUN] verify process exit (would poll up to 15s)"
else
  for i in $(seq 1 15); do
    if ! pgrep -f "cc-connect" > /dev/null 2>&1; then
      break
    fi
    sleep 1
  done
  if pgrep -f "cc-connect" > /dev/null 2>&1; then
    echo "  Warning: cc-connect process still running after 15s."
    echo "  Attempting force kill..."
    if command -v taskkill > /dev/null 2>&1; then
      # Windows (Git Bash / MINGW)
      taskkill //F //IM cc-connect.exe > /dev/null 2>&1 || true
    else
      # Unix
      pkill -f cc-connect 2>/dev/null || true
    fi
    sleep 2
    if pgrep -f "cc-connect" > /dev/null 2>&1; then
      echo "  ERROR: Could not kill cc-connect process."
      echo "  File locks may prevent install. Please kill it manually and re-run."
    fi
  fi
fi

# ── Step 5: Reinstall ──────────────────────────────────────
echo ""
echo "[5/7] Installing latest version..."
if run npm install -g "$PKG"; then
  VERSION=$(cc-connect --version 2>/dev/null || echo "unknown")
  if ! $DRYRUN; then
    WHICH_CC=$(command -v cc-connect 2>/dev/null || which cc-connect 2>/dev/null || echo "unknown")
    echo "  Installed: $VERSION"
    echo "  Binary:    $WHICH_CC"
  fi
else
  echo ""
  echo "ERROR: npm install failed. Check your ~/.npmrc and network."
  echo "  Your config backup is at: $BACKUP_DIR/$TIMESTAMP"
  echo "  To retry manually:"
  echo "    npm install -g $PKG"
  exit 1
fi

# ── Step 6: Restore user data ───────────────────────────────
echo ""
echo "[6/7] Restoring user data..."
if [ -f "$BACKUP_DIR/$TIMESTAMP/config.toml" ]; then
  run mkdir -p "$CC_DIR"
  run cp "$BACKUP_DIR/$TIMESTAMP/config.toml" "$CC_DIR/config.toml"
  echo "  Restored: config.toml"
fi

if [ -f "$BACKUP_DIR/$TIMESTAMP/crons-jobs.json" ]; then
  run mkdir -p "$CC_DIR/crons"
  run cp "$BACKUP_DIR/$TIMESTAMP/crons-jobs.json" "$CC_DIR/crons/jobs.json"
  echo "  Restored: crons/jobs.json"
fi

# ── Step 7: Reinstall daemon ───────────────────────────────
echo ""
echo "[7/7] Reinstalling daemon..."
if run cc-connect daemon install; then
  run cc-connect daemon status
else
  echo "  Warning: daemon install failed. You may need to run it manually."
fi

# ── Summary ────────────────────────────────────────────────
echo ""
echo "=== Reinstall Complete ==="
echo "  Version: $(cc-connect --version 2>/dev/null || echo 'unknown')"
echo "  Config:  $CC_DIR/config.toml"
echo "  Backup:  $BACKUP_DIR/$TIMESTAMP"
echo ""
