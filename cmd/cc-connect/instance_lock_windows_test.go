//go:build windows

package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireInstanceLock_Success(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")

	lock, err := AcquireInstanceLock(cfg)
	if err != nil {
		t.Fatalf("AcquireInstanceLock: %v", err)
	}
	if lock == nil || !lock.acquired {
		t.Fatal("expected acquired lock")
	}
	defer lock.Release()

	if lock.Path() == "" {
		t.Fatal("expected non-empty lock path")
	}
}

func TestAcquireInstanceLock_AlreadyLocked(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "config.toml")

	first, err := AcquireInstanceLock(cfg)
	if err != nil {
		t.Fatalf("first AcquireInstanceLock: %v", err)
	}
	defer first.Release()

	_, err = AcquireInstanceLock(cfg)
	if err == nil {
		t.Fatal("second AcquireInstanceLock should fail while lock held")
	}
}

func TestRemoveInstanceLock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	// Acquire a lock to create the lock file
	lock, err := AcquireInstanceLock(configPath)
	if err != nil {
		t.Fatalf("AcquireInstanceLock: %v", err)
	}
	lockPath := lock.Path()
	lock.Release()

	// Lock file should still exist after Release on some platforms,
	// so RemoveInstanceLock must clean it up.
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Skip("lock file already removed by Release")
	}

	if err := RemoveInstanceLock(configPath); err != nil {
		t.Fatalf("RemoveInstanceLock: %v", err)
	}

	if _, err := os.Stat(lockPath); err == nil {
		t.Errorf("lock file still exists after RemoveInstanceLock")
	}
}
