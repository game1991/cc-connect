//go:build !windows

package daemon

// IsAdmin is a no-op stub on non-Windows platforms.
// On Windows the real implementation lives in svcmanager_windows.go.
func IsAdmin() bool { return true }
