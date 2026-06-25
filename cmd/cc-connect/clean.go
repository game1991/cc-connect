package main

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/chenhg5/cc-connect/daemon"
)

func runClean(args []string) {
	if len(args) > 0 && args[0] == "reset" {
		cleanReset()
		return
	}
	cleanRuntime()
}

// cleanRuntime stops the daemon, kills residual processes, removes
// runtime/ephemeral files, and preserves config.toml and crons/.
// It is safe to run even when the daemon is not installed.
func cleanRuntime() {
	dataDir := daemon.DefaultDataDir()

	fmt.Println("Cleaning cc-connect runtime state...")

	// 1. Snapshot daemon metadata BEFORE any destructive operations.
	//    This ensures killResidualProcess can find the config path even
	//    after the service is uninstalled and daemon.json is deleted.
	var metaSnapshot *daemon.Meta
	if m, err := daemon.LoadMeta(); err == nil {
		metaSnapshot = m
	}

	// 2. Stop the daemon — gracefully handle "not installed".
	stopDaemonForClean()

	// 3. Kill any residual process via instance lock PID.
	//    Uses the snapshotted meta to resolve the config path.
	killResidualProcessWithMeta(metaSnapshot)

	// 4. Wait for process exit and file-handle release (especially on Windows).
	time.Sleep(1 * time.Second)

	// 5. Uninstall the daemon service — only if installed.
	//    This also deletes daemon.json (last destructive operation on meta).
	uninstallDaemonForClean()

	// 6. Remove runtime/ephemeral files.
	//    daemon.json and cc-connect-daemon.ps1 are explicitly listed because
	//    they may remain if the daemon was never installed. os.RemoveAll on
	//    a non-existent path is safe.
	removed := 0
	failed := []string{}

	cleanTargets := []struct {
		path string
		desc string
	}{
		{filepath.Join(dataDir, "daemon.json"), "daemon metadata"},
		{filepath.Join(dataDir, ".config.toml.lock"), "instance lock"},
		{filepath.Join(dataDir, "cc-connect-daemon.ps1"), "daemon launcher script"},
		{filepath.Join(dataDir, "run"), "runtime directory"},
		{filepath.Join(dataDir, "logs"), "log directory"},
		{filepath.Join(dataDir, "dir_history.json"), "directory history"},
	}

	for _, ct := range cleanTargets {
		if _, err := os.Stat(ct.path); os.IsNotExist(err) {
			continue
		}
		if err := os.RemoveAll(ct.path); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to remove %s: %v\n", ct.desc, err)
			failed = append(failed, ct.desc)
		} else {
			fmt.Printf("  Removed: %s\n", ct.desc)
			removed++
		}
	}

	// 6. Remove empty data directory if nothing remains.
	if entries, _ := os.ReadDir(dataDir); len(entries) == 0 {
		_ = os.Remove(dataDir)
		fmt.Printf("  Removed empty directory: %s\n", shortPath(dataDir))
	}

	// 7. Summary.
	fmt.Printf("\nClean complete. Removed %d item(s).\n", removed)
	if len(failed) > 0 {
		fmt.Fprintf(os.Stderr, "Warning: could not remove: %s\n", strings.Join(failed, ", "))
	}
	fmt.Println("Preserved: config.toml, crons/")
}

// cleanReset performs a full reset: back up user data + clean + remove everything.
func cleanReset() {
	dataDir := daemon.DefaultDataDir()

	// 1. Back up user data BEFORE clean (cleanRuntime deletes dir_history.json).
	backupDir := filepath.Join(filepath.Dir(dataDir), ".cc-connect-backup", time.Now().Format("20060102-150405"))
	if err := os.MkdirAll(backupDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Error: cannot create backup directory: %v\n", err)
		os.Exit(1)
	}
	_ = os.Chmod(backupDir, 0700)

	backedUp := 0
	type backupItem struct {
		src  string
		name string
	}
	items := []backupItem{
		{filepath.Join(dataDir, "config.toml"), "config.toml"},
		{filepath.Join(dataDir, "crons", "jobs.json"), "crons-jobs.json"},
		{filepath.Join(dataDir, "dir_history.json"), "dir_history.json"},
	}

	for _, bi := range items {
		data, err := os.ReadFile(bi.src)
		if err != nil {
			continue
		}
		dst := filepath.Join(backupDir, bi.name)
		if err := os.WriteFile(dst, data, 0600); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to back up %s: %v\n", bi.name, err)
			continue
		}
		if len(data) == 0 {
			fmt.Printf("  Backed up (empty): %s\n", bi.name)
		} else {
			fmt.Printf("  Backed up: %s (%d bytes)\n", bi.name, len(data))
		}
		backedUp++
	}

	if backedUp > 0 {
		fmt.Printf("  Backup directory: %s\n", shortPath(backupDir))
	} else {
		fmt.Println("  No user data to back up.")
	}

	// 2. Run clean.
	cleanRuntime()

	// 3. Confirm with user — reset deletes config and cron jobs.
	fmt.Println("\nReset will remove config.toml and crons/ (backups have been created).")
	if !confirmPrompt("Continue with reset? [y/N] ") {
		fmt.Println("Reset cancelled.")
		return
	}

	// 4. Remove remaining user data.
	remaining, _ := os.ReadDir(dataDir)
	removed := 0
	for _, e := range remaining {
		p := filepath.Join(dataDir, e.Name())
		if err := os.RemoveAll(p); err != nil {
			fmt.Fprintf(os.Stderr, "  Warning: failed to remove %s: %v\n", e.Name(), err)
		} else {
			fmt.Printf("  Removed: %s\n", e.Name())
			removed++
		}
	}

	// 5. Remove data directory if empty.
	if entries, _ := os.ReadDir(dataDir); len(entries) == 0 {
		_ = os.Remove(dataDir)
		fmt.Printf("  Removed empty directory: %s\n", shortPath(dataDir))
	}

	// 6. Summary.
	fmt.Printf("\nReset complete. Backed up %d file(s), removed %d item(s).\n", backedUp, removed)
	if backedUp > 0 {
		fmt.Printf("To restore: cp %s/config.toml ~/.cc-connect/config.toml\n", shortPath(backupDir))
		if _, err := os.Stat(filepath.Join(backupDir, "crons-jobs.json")); err == nil {
			fmt.Printf("            cp %s/crons-jobs.json ~/.cc-connect/crons/jobs.json\n", shortPath(backupDir))
		}
	}
}

// stopDaemonForClean stops the daemon if it is installed. Errors are
// printed as warnings; the process always continues.
func stopDaemonForClean() {
	if err := stopWithFallback(daemon.NewManager, daemon.LoadMeta, KillExistingInstance, os.Stderr); err != nil {
		if !strings.Contains(err.Error(), "not installed") {
			fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
		}
	} else {
		fmt.Println("  Daemon stopped.")
	}
}

// uninstallDaemonForClean uninstalls the daemon service if it is installed.
func uninstallDaemonForClean() {
	mgr, err := daemon.NewManager()
	if err != nil {
		return
	}
	st, _ := mgr.Status()
	if st == nil || !st.Installed {
		return
	}
	if err := mgr.Uninstall(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: uninstall failed: %v\n", err)
	} else {
		fmt.Println("  Daemon service uninstalled.")
		daemon.RemoveMeta()
	}
}

// killResidualProcessWithMeta attempts to kill any lingering cc-connect process
// via the instance lock PID. It uses the pre-snapshotted meta to resolve the
// actual config path, falling back to the default path if meta is nil.
func killResidualProcessWithMeta(meta *daemon.Meta) {
	configPath := filepath.Join(daemon.DefaultDataDir(), "config.toml")
	if meta != nil && meta.ConfigFile != "" {
		configPath = meta.ConfigFile
	}
	if KillExistingInstance(configPath) {
		fmt.Println("  Killed residual process via instance lock PID.")
		time.Sleep(500 * time.Millisecond)
	}
}

// confirmPrompt prints a prompt and reads a single-character y/N response.
// It uses raw byte reading to avoid MINGW \r issues with bufio.ReadString.
// In non-interactive environments (EOF on stdin), it returns false.
func confirmPrompt(prompt string) bool {
	fmt.Print(prompt)
	reader := bufio.NewReader(os.Stdin)
	b, err := reader.ReadByte()
	if err != nil {
		// EOF or read error — non-interactive, default to "no"
		fmt.Println()
		return false
	}
	// Drain the rest of the line
	_, _ = reader.ReadString('\n')
	return b == 'y' || b == 'Y'
}

// shortPath replaces the home directory prefix with ~ for display.
func shortPath(path string) string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return path
	}
	if runtime.GOOS == "windows" {
		path = filepath.ToSlash(path)
		homeDir = filepath.ToSlash(homeDir)
	}
	if strings.HasPrefix(path, homeDir+"/") {
		return "~" + path[len(homeDir):]
	}
	return path
}
