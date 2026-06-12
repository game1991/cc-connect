//go:build !windows

package main

// svcStopCh is nil on non-Windows platforms, so it never fires in select.
var svcStopCh chan struct{}

func isWindowsService() bool { return false }

func runWindowsService() {}
