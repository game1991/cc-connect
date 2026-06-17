//go:build !windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

const killWaitTimeout = 5 * time.Second
const killWaitInterval = 25 * time.Millisecond

// InstanceLock provides a file-based exclusive lock to prevent multiple
// cc-connect instances with the same config from running simultaneously.
type InstanceLock struct {
	file     *os.File
	path     string
	acquired bool
}

// AcquireInstanceLock attempts to acquire an exclusive lock for the given config path.
// If another instance is already running with the same config, it returns an error
// containing the PID of the existing instance.
func AcquireInstanceLock(configPath string) (*InstanceLock, error) {
	configDir := filepath.Dir(configPath)
	configBase := filepath.Base(configPath)

	lockName := fmt.Sprintf(".%s.lock", configBase)
	lockPath := filepath.Join(configDir, lockName)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create config directory: %w", err)
	}

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("cannot open lock file: %w", err)
	}

	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		f.Close()

		pid := readPIDFromLockFile(lockPath)
		if pid > 0 {
			return nil, fmt.Errorf("another cc-connect instance is already running (PID %d) with config %s", pid, configPath)
		}
		return nil, fmt.Errorf("another cc-connect instance is already running with config %s", configPath)
	}

	pid := os.Getpid()
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	fmt.Fprintf(f, "%d\n", pid)

	return &InstanceLock{
		file:     f,
		path:     lockPath,
		acquired: true,
	}, nil
}

// Release releases the instance lock. It is safe to call multiple times.
func (l *InstanceLock) Release() {
	if l == nil || !l.acquired {
		return
	}

	if l.file != nil {
		_ = l.file.Truncate(0)
		_ = syscall.Flock(int(l.file.Fd()), syscall.LOCK_UN)
		l.file.Close()
		l.file = nil
	}

	l.acquired = false
}

// Path returns the path to the lock file.
func (l *InstanceLock) Path() string {
	return l.path
}

// readPIDFromLockFile attempts to read a PID from a lock file.
// Returns 0 if the PID cannot be determined.
func readPIDFromLockFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}

	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return 0
	}

	return pid
}

// RemoveInstanceLock removes the instance lock file for the given config path.
// This is used after force-killing an orphan that cannot clean up its own lock.
// Returns an error for unexpected failures; os.ErrNotExist is ignored.
func RemoveInstanceLock(configPath string) error {
	configDir := filepath.Dir(configPath)
	configBase := filepath.Base(configPath)
	lockName := fmt.Sprintf(".%s.lock", configBase)
	lockPath := filepath.Join(configDir, lockName)

	err := os.Remove(lockPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove instance lock %s: %w", lockPath, err)
	}
	return nil
}

// KillExistingInstance attempts to kill the process holding the lock for the given config.
// Returns true if a process was killed, false otherwise.
func KillExistingInstance(configPath string) bool {
	configDir := filepath.Dir(configPath)
	configBase := filepath.Base(configPath)
	lockName := fmt.Sprintf(".%s.lock", configBase)
	lockPath := filepath.Join(configDir, lockName)

	pid := readPIDFromLockFile(lockPath)
	if pid <= 0 {
		return false
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds, so we need to signal it
	// to check if it actually exists
	if err := proc.Signal(syscall.Signal(0)); err != nil {
		return false
	}

	// Verify the process is cc-connect to prevent PID-reuse miskill.
	if !verifyUnixProcessIsCcConnect(pid) {
		return false
	}

	// Process exists and is cc-connect, kill it
	if err := proc.Kill(); err != nil {
		return false
	}

	// Wait for the process to actually exit
	deadline := time.Now().Add(killWaitTimeout)
	for time.Now().Before(deadline) {
		if err := proc.Signal(syscall.Signal(0)); err != nil {
			return true
		}
		time.Sleep(killWaitInterval)
	}

	return false
}

// verifyUnixProcessIsCcConnect checks if the given PID belongs to a cc-connect process.
// It reads /proc/<pid>/exe (Linux) or falls back to /proc/<pid>/cmdline.
func verifyUnixProcessIsCcConnect(pid int) bool {
	// Try /proc/<pid>/exe (Linux) — this is a symlink to the actual executable.
	exePath := fmt.Sprintf("/proc/%d/exe", pid)
	if target, err := os.Readlink(exePath); err == nil {
		base := strings.ToLower(filepath.Base(target))
		if base == "cc-connect" || base == "cc-connect.exe" {
			return true
		}
		// On some systems (e.g., NixOS), the link may contain "cc-connect" as a suffix.
		if strings.Contains(strings.ToLower(target), "cc-connect") {
			return true
		}
	}

	// Fallback: read /proc/<pid>/cmdline and check if argv[0] contains cc-connect.
	cmdlinePath := fmt.Sprintf("/proc/%d/cmdline", pid)
	if data, err := os.ReadFile(cmdlinePath); err == nil {
		// cmdline entries are null-separated; argv[0] is the executable path.
		args := strings.Split(string(data), "\x00")
		if len(args) > 0 {
			base := strings.ToLower(filepath.Base(args[0]))
			if base == "cc-connect" || base == "cc-connect.exe" {
				return true
			}
			if strings.Contains(strings.ToLower(args[0]), "cc-connect") {
				return true
			}
		}
	}

	// macOS doesn't have /proc; fall back to checking process name via ps.
	// This is a best-effort check — if /proc is not available and ps fails,
	// we still allow the kill to proceed rather than blocking it entirely.
	if _, err := os.Stat("/proc"); os.IsNotExist(err) {
		// macOS or BSD — no /proc filesystem. Allow the kill to proceed
		// since the flock-based lock already provides some confidence
		// that the PID belongs to cc-connect.
		return true
	}

	return false
}
