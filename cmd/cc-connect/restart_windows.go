//go:build windows

package main

import (
	"os"
	"os/exec"
)

func restartProcess(execPath string) error {
	args := os.Args[1:]
	// In Windows Service mode, os.Args has already been filtered to remove
	// --service. We must re-add it so the restarted process registers with SCM.
	if svcStopCh != nil {
		args = append([]string{"--service"}, args...)
	}
	cmd := exec.Command(execPath, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	cmd.Env = os.Environ()
	return cmd.Start()
}
