//go:build windows

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/chenhg5/cc-connect/daemon"

	"golang.org/x/sys/windows/svc"
)

const svcShutdownTimeout = 25 * time.Second

// svcStopCh is closed by svcHandler when SCM requests stop/shutdown.
// Nil (default) blocks forever in select, which is correct for non-service mode.
// Write: runWindowsService (make). Close: svcHandler.Execute (via svcStopOnce).
// Read: main.go signal select (<-svcStopCh).
var svcStopCh chan struct{}

// isWindowsService returns true when the current process was launched by SCM.
func isWindowsService() bool {
	ok, _ := svc.IsWindowsService()
	return ok
}

// runWindowsService is the service entry point. It tells SCM we are a service,
// then runs the normal main logic while listening for SCM stop/shutdown signals.
func runWindowsService() {
	svcStopCh = make(chan struct{})
	if err := svc.Run(daemon.ServiceName, &svcHandler{}); err != nil {
		slog.Error("svc.Run failed", "error", err)
		os.Exit(1)
	}
}

// svcHandler implements svc.Handler.
type svcHandler struct{}

func (h *svcHandler) Execute(args []string, r <-chan svc.ChangeRequest, status chan<- svc.Status) (bool, uint32) {
	// Filter --service from args before main reads them.
	os.Args = filterArgs(os.Args, "--service")

	// When running as a Windows Service under LocalSystem, the user's home
	// directory and PATH are not available. Fix the environment BEFORE
	// reporting Running so that log file and PATH are set up before any
	// slog.Error calls from runMain().
	fixSvcEnvironment()

	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	done := make(chan struct{})
	go func() {
		defer close(done)
		runMain()
	}()

	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Stop, svc.Shutdown:
				slog.Info("Windows Service stop requested")
				status <- svc.Status{State: svc.StopPending, WaitHint: uint32(svcShutdownTimeout / time.Millisecond)}
				// Signal runMain to shut down; use Once to prevent double-close panic.
				daemon.SvcStopOnce.Do(func() { close(svcStopCh) })

				select {
				case <-done:
				case <-time.After(svcShutdownTimeout):
					slog.Error("graceful shutdown timed out, SCM will force-terminate")
				}
				return false, 0

			case svc.Interrogate:
				status <- c.CurrentStatus

			default:
				slog.Debug("svc: unhandled change request", "cmd", c.Cmd)
			}

		case <-done:
			return false, 0
		}
	}
}

func filterArgs(args []string, remove string) []string {
	filtered := make([]string, 0, len(args))
	for _, a := range args {
		if a != remove {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

// fixSvcEnvironment restores the user's home directory, PATH, and log
// location when the service runs under LocalSystem. It reads daemon.json
// (written by "daemon install") for the captured user environment.
func fixSvcEnvironment() {
	// Only fix when running as SYSTEM (USERPROFILE points to systemprofile).
	up := os.Getenv("USERPROFILE")
	if !strings.Contains(strings.ToLower(up), "system32") &&
		!strings.Contains(strings.ToLower(up), "systemprofile") {
		return
	}

	// Locate daemon.json via --config path or default data dir.
	var configFile string
	for i := 0; i < len(os.Args); i++ {
		if (os.Args[i] == "--config" || os.Args[i] == "-config") && i+1 < len(os.Args) {
			configFile = os.Args[i+1]
			break
		}
		if prefix, ok := strings.CutPrefix(os.Args[i], "--config="); ok {
			configFile = prefix
			break
		}
	}

	var dataDir string
	if configFile != "" {
		dataDir = filepath.Dir(filepath.FromSlash(configFile))
	} else {
		// Fallback: assume default data dir relative to SystemDrive.
		// The ps1 launcher passes --config, so this path is unlikely.
		dataDir = filepath.Join(os.Getenv("SystemDrive"), "Users", ".cc-connect")
	}

	metaPath := filepath.Join(dataDir, "daemon.json")
	data, err := os.ReadFile(metaPath)
	if err != nil {
		slog.Warn("svc: could not read daemon.json, environment not restored", "path", metaPath, "error", err)
		return
	}

	var meta daemon.Meta
	if err := json.Unmarshal(data, &meta); err != nil {
		slog.Warn("svc: could not parse daemon.json", "error", err)
		return
	}

	// Restore home directory from daemon.json (takes precedence over --config derivation).
	if meta.HomeDir != "" {
		hd := filepath.FromSlash(meta.HomeDir)
		os.Setenv("USERPROFILE", hd)
		os.Setenv("HOME", hd)
		drive := filepath.VolumeName(hd)
		os.Setenv("HOMEDRIVE", drive)
		os.Setenv("HOMEPATH", strings.TrimPrefix(hd, drive))
		slog.Info("svc: restored home dir from daemon.json", "path", hd)
	} else if configFile != "" {
		// Derive from --config as fallback.
		homeDir := filepath.Dir(dataDir)
		os.Setenv("USERPROFILE", homeDir)
		os.Setenv("HOME", homeDir)
		drive := filepath.VolumeName(homeDir)
		os.Setenv("HOMEDRIVE", drive)
		os.Setenv("HOMEPATH", strings.TrimPrefix(homeDir, drive))
	}

	// Restore log file location from daemon.json.
	logFile := meta.LogFile
	if logFile == "" {
		// Fallback to derived path if log_file was not captured.
		logFile = filepath.ToSlash(filepath.Join(dataDir, "logs", "cc-connect.log"))
	}
	logFile = filepath.FromSlash(logFile)
	os.Setenv("CC_LOG_FILE", logFile)

	// Ensure the log directory exists (SYSTEM has full filesystem access).
	if err := os.MkdirAll(filepath.Dir(logFile), 0755); err != nil {
		fmt.Fprintf(os.Stderr, "svc: failed to create log dir %s: %v\n", filepath.Dir(logFile), err)
	}

	// Restore PATH so agent binaries (claude, codex, etc.) are findable.
	if meta.EnvPATH != "" {
		os.Setenv("PATH", meta.EnvPATH)
		slog.Info("svc: restored PATH from daemon.json", "len", len(meta.EnvPATH))
	}
}
