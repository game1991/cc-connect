//go:build windows

package daemon

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

type svcManager struct{}

func (*svcManager) Platform() string { return "svc" }

// escapeScPath escapes a path for use inside sc.exe's binPath= parameter.
// sc.exe binPath rules: backslash is escape char, \" is escaped quote.
// A trailing backslash before a closing quote would escape the quote itself,
// so we must remove trailing backslashes and double all remaining ones.
func escapeScPath(path string) string {
	path = strings.TrimRight(path, `\`)
	path = strings.ReplaceAll(path, `\`, `\\`)
	path = strings.ReplaceAll(path, `"`, `\"`)
	return path
}

func (m *svcManager) Install(cfg Config) error {
	if !IsAdmin() {
		return fmt.Errorf("installing as a Windows Service requires administrator privileges; " +
			"run as admin or install without admin to use Task Scheduler instead")
	}

	if err := os.MkdirAll(DefaultDataDir(), 0755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(cfg.LogFile), 0755); err != nil {
		return fmt.Errorf("create log dir: %w", err)
	}

	// If the service is already installed, delete it first (idempotent / --force).
	if installed, _ := svcIsInstalled(); installed {
		slog.Info("svc: service already exists, deleting before reinstall")
		_ = m.Stop()
		if out, err := runSc("delete", ServiceName); err != nil {
			slog.Debug("sc delete failed", "output", out, "error", err)
			return fmt.Errorf("svc: failed to delete existing service for reinstall: %w", err)
		}
		if err := pollStopped(func() bool {
			ok, _ := svcIsInstalled()
			return !ok
		}, 10*time.Second); err != nil {
			return fmt.Errorf("svc: timed out waiting for service deletion: %w", err)
		}
	}

	// Build the binPath for sc.exe.
	// sc.exe binPath= parameter: value is everything after '=' until end of line.
	// Paths with spaces must be quoted. Inside quotes: \\ is literal backslash, \" is literal quote.
	args := []string{"--service"}
	if cfg.ConfigFile != "" {
		args = append(args, fmt.Sprintf(`--config "%s"`, escapeScPath(cfg.ConfigFile)))
	}
	fullBinPath := fmt.Sprintf(`"%s" %s`, escapeScPath(cfg.BinaryPath), strings.Join(args, " "))

	out, err := runSc("create", ServiceName,
		fmt.Sprintf("binPath=%s", fullBinPath),
		fmt.Sprintf("DisplayName=%s", ServiceDisplayName),
		"start=auto")
	if err != nil {
		slog.Debug("sc create failed", "output", out, "error", err)
		return fmt.Errorf("sc create: failed to register service: %w", err)
	}

	out, err = runSc("description", ServiceName, ServiceDescription)
	if err != nil {
		slog.Warn("svc: sc description failed", "output", out, "error", err)
	}

	if err := m.Start(); err != nil {
		return fmt.Errorf("start service: %w", err)
	}
	return nil
}

func (m *svcManager) Uninstall() error {
	st, _ := m.Status()
	if st == nil || !st.Installed {
		return nil
	}
	if st.Running {
		if err := m.Stop(); err != nil {
			slog.Warn("svc: stop before uninstall failed", "error", err)
		}
	}
	if err := pollStopped(m.isNotRunning, 10*time.Second); err != nil {
		slog.Warn("svc: timed out waiting for service to stop", "error", err)
	}
	out, err := runSc("delete", ServiceName)
	if err != nil {
		slog.Debug("sc delete failed", "output", out, "error", err)
		return fmt.Errorf("sc delete: failed to remove service: %w", err)
	}
	return nil
}

func (*svcManager) Start() error {
	out, err := runSc("start", ServiceName)
	if err != nil {
		slog.Debug("sc start failed", "output", out, "error", err)
		return fmt.Errorf("sc start: %w", err)
	}
	return nil
}

func (*svcManager) Stop() error {
	out, err := runSc("stop", ServiceName)
	if err != nil {
		slog.Debug("sc stop failed", "output", out, "error", err)
		return fmt.Errorf("sc stop: %w", err)
	}
	return nil
}

func (m *svcManager) Restart() error {
	st, _ := m.Status()
	if st != nil && st.Installed && st.Running {
		if err := m.Stop(); err != nil {
			return fmt.Errorf("svc: stop before restart failed: %w", err)
		}
		if err := pollStopped(m.isNotRunning, 10*time.Second); err != nil {
			return fmt.Errorf("svc: timed out waiting for service to stop before restart: %w", err)
		}
	}
	return m.Start()
}

func (*svcManager) Status() (*Status, error) {
	st := &Status{Platform: "svc"}
	out, err := runSc("query", ServiceName)
	if err != nil {
		s := strings.ToUpper(out)
		if strings.Contains(s, "DOES NOT EXIST") || strings.Contains(s, "ERROR_1060") {
			return st, nil
		}
		slog.Debug("sc query failed", "output", out, "error", err)
		return nil, fmt.Errorf("sc query: %w", err)
	}
	st.Installed = true
	if strings.Contains(strings.ToUpper(out), "RUNNING") {
		st.Running = true
	}
	// Extract PID from sc query output — match "PID" exactly, not "PROCESS_ID"
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		lineUpper := strings.ToUpper(line)
		if lineUpper == "PID" || strings.HasPrefix(lineUpper, "PID ") || strings.HasPrefix(lineUpper, "PID\t") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				pid := 0
				n, err := fmt.Sscanf(strings.TrimSpace(parts[1]), "%d", &pid)
				if err != nil || n != 1 {
					slog.Debug("svc: failed to parse PID from sc query", "raw", parts[1])
				} else if pid > 0 {
					st.PID = pid
				}
			}
		}
	}
	return st, nil
}

func (m *svcManager) isNotRunning() bool {
	st, _ := m.Status()
	return st == nil || !st.Installed || !st.Running
}

func svcIsInstalled() (bool, error) {
	st, err := (&svcManager{}).Status()
	if err != nil {
		return false, err
	}
	return st.Installed, nil
}

// pollStopped repeatedly calls check until it returns true or timeout elapses.
func pollStopped(check func() bool, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if check() {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("timed out after %v", timeout)
}

var runSc = func(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sc.exe", args...)
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// SvcStopOnce protects close(svcStopCh) from double-close panic.
// It is set by cmd/cc-connect/svc_run_windows.go and closed by svcHandler.Execute.
var SvcStopOnce sync.Once

// IsAdmin checks whether the current process has elevated administrator
// privileges. It uses TokenElevation first (UAC context), then falls back
// to CheckTokenMembership for service accounts (LocalSystem, NetworkService)
// where TokenElevation is 0 despite having full privileges.
func IsAdmin() bool {
	// Method 1: TokenElevation (UAC elevated processes)
	token := windows.GetCurrentProcessToken()
	defer token.Close()

	var elevated uint32
	var returnedLen uint32
	err := windows.GetTokenInformation(
		token,
		windows.TokenElevation,
		(*byte)(unsafe.Pointer(&elevated)),
		uint32(unsafe.Sizeof(elevated)),
		&returnedLen,
	)
	if err == nil && elevated != 0 {
		return true
	}
	if err != nil {
		slog.Debug("svc: TokenElevation query failed, falling back to CheckTokenMembership", "error", err)
	}

	// Method 2: Token.IsMember (service accounts, non-UAC contexts)
	var sid *windows.SID
	err = windows.AllocateAndInitializeSid(
		&windows.SECURITY_NT_AUTHORITY,
		2,
		windows.SECURITY_BUILTIN_DOMAIN_RID,
		windows.DOMAIN_ALIAS_RID_ADMINS,
		0, 0, 0, 0, 0, 0,
		&sid,
	)
	if err != nil {
		slog.Warn("svc: AllocateAndInitializeSid failed, assuming non-admin", "error", err)
		return false
	}
	defer windows.FreeSid(sid)

	member, err := token.IsMember(sid)
	return err == nil && member
}
