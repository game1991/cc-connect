//go:build windows

package main

import (
	"log/slog"
	"os"
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
	status <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}

	// Filter --service from args before main reads them.
	os.Args = filterArgs(os.Args, "--service")

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
