package main

import (
	"fmt"
	"os"
	"path/filepath"
)

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
