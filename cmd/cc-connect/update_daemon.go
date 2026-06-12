package main

import (
	"fmt"

	"github.com/chenhg5/cc-connect/daemon"
)

// postUpdateDaemonRestart checks if the daemon is installed and, if so,
// regenerates its launcher script and restarts it so the newly updated
// binary takes effect immediately.
func postUpdateDaemonRestart() error {
	mgr, err := daemon.NewManager()
	if err != nil {
		return nil
	}

	st, _ := mgr.Status()
	if st == nil || !st.Installed {
		return nil
	}

	meta, err := daemon.LoadMeta()
	if err != nil {
		return fmt.Errorf("load daemon meta: %w", err)
	}

	cfg := daemon.ConfigFromMeta(meta)

	switch mgr.Platform() {
	case "schtasks":
		if err := daemon.RewriteLauncherScript(cfg); err != nil {
			return fmt.Errorf("rewrite launcher script: %w", err)
		}
	case "svc":
		// Re-register the service with updated binPath via force install.
		if err := mgr.Install(cfg); err != nil {
			return fmt.Errorf("re-register service: %w", err)
		}
		return nil
	}

	if err := mgr.Restart(); err != nil {
		return fmt.Errorf("restart daemon: %w", err)
	}

	fmt.Println("  Daemon restarted with updated binary.")
	return nil
}
