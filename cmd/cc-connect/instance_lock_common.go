package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func instanceLockPath(configPath string) string {
	configDir := filepath.Dir(configPath)
	configBase := filepath.Base(configPath)
	lockName := fmt.Sprintf(".%s.lock", configBase)
	return filepath.Join(configDir, lockName)
}

// RemoveInstanceLock removes the instance lock file for the given config path.
// This is used after force-killing an orphan that cannot clean up its own lock.
// Returns an error for unexpected failures; os.ErrNotExist is ignored.
func RemoveInstanceLock(configPath string) error {
	lockPath := instanceLockPath(configPath)

	err := os.Remove(lockPath)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove instance lock %s: %w", lockPath, err)
	}
	return nil
}
