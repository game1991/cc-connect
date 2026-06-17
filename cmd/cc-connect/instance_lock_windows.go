//go:build windows

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const killWaitTimeout = 5 * time.Second

const processQueryLimitedInformation = 0x1000
const processSynchronize = 0x100000

type InstanceLock struct {
	handle   syscall.Handle
	path     string
	acquired bool
}

func AcquireInstanceLock(configPath string) (*InstanceLock, error) {
	configDir := filepath.Dir(configPath)
	configBase := filepath.Base(configPath)
	lockName := fmt.Sprintf(".%s.lock", configBase)
	lockPath := filepath.Join(configDir, lockName)

	if err := os.MkdirAll(configDir, 0755); err != nil {
		return nil, fmt.Errorf("cannot create config directory: %w", err)
	}

	pathPtr, err := syscall.UTF16PtrFromString(lockPath)
	if err != nil {
		return nil, fmt.Errorf("cannot convert lock path: %w", err)
	}

	handle, createErr := syscall.CreateFile(
		pathPtr,
		syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ,
		nil,
		syscall.OPEN_ALWAYS,
		syscall.FILE_ATTRIBUTE_NORMAL,
		0,
	)

	if createErr != nil {
		pid := readPIDFromLockFile(lockPath)
		if pid > 0 {
			return nil, fmt.Errorf("another cc-connect instance is already running (PID %d) with config %s", pid, configPath)
		}
		return nil, fmt.Errorf("another cc-connect instance is already running with config %s", configPath)
	}

	pid := os.Getpid()
	syscall.SetFilePointer(handle, 0, nil, syscall.FILE_BEGIN)
	syscall.SetEndOfFile(handle)
	var written uint32
	syscall.WriteFile(handle, []byte(fmt.Sprintf("%d\n", pid)), &written, nil)
	syscall.FlushFileBuffers(handle)

	return &InstanceLock{
		handle:   handle,
		path:     lockPath,
		acquired: true,
	}, nil
}

func (l *InstanceLock) Release() {
	if l == nil || !l.acquired {
		return
	}
	if l.handle != 0 {
		syscall.SetFilePointer(l.handle, 0, nil, syscall.FILE_BEGIN)
		syscall.SetEndOfFile(l.handle)
		syscall.CloseHandle(l.handle)
		l.handle = 0
	}
	l.acquired = false
}

func (l *InstanceLock) Path() string {
	return l.path
}

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

func KillExistingInstance(configPath string) bool {
	configDir := filepath.Dir(configPath)
	configBase := filepath.Base(configPath)
	lockName := fmt.Sprintf(".%s.lock", configBase)
	lockPath := filepath.Join(configDir, lockName)

	pid := readPIDFromLockFile(lockPath)
	if pid <= 0 {
		return false
	}

	// Open process with TERMINATE (for TerminateProcess), QUERY (for
	// image name verification), and SYNCHRONIZE (for WaitForSingleObject)
	// access rights.
	handle, err := syscall.OpenProcess(
		syscall.PROCESS_TERMINATE|processQueryLimitedInformation|processSynchronize,
		false, uint32(pid),
	)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(handle)

	// Verify the process is cc-connect to prevent PID-reuse miskill.
	exePath, err := queryFullProcessImageName(handle)
	if err == nil && !isCcConnectBinary(exePath) {
		return false
	}

	if err := syscall.TerminateProcess(handle, 1); err != nil {
		return false
	}

	// On Windows, terminated processes become zombies: OpenProcess still
	// succeeds on the dead handle. Use WaitForSingleObject on the process
	// handle instead — it transitions to signaled state when the process
	// actually exits.
	timeoutMs := uint32(killWaitTimeout.Milliseconds())
	event, _ := syscall.WaitForSingleObject(handle, timeoutMs)
	return event == syscall.WAIT_OBJECT_0
}

func queryFullProcessImageName(handle syscall.Handle) (string, error) {
	var size uint32 = syscall.MAX_PATH
	buf := make([]uint16, size)
	// QueryFullProcessImageNameW is available since Windows Vista.
	modkernel32 := syscall.NewLazyDLL("kernel32.dll")
	proc := modkernel32.NewProc("QueryFullProcessImageNameW")
	rc, _, e := proc.Call(
		uintptr(handle),
		0, // name format: Win32 path
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(unsafe.Pointer(&size)),
	)
	if rc == 0 {
		return "", e
	}
	return syscall.UTF16ToString(buf[:size]), nil
}

func isCcConnectBinary(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return base == "cc-connect.exe" || base == "cc-connect"
}

func readPIDFromLockFile(path string) int {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0
	}
	var pid int
	if _, err := fmt.Sscanf(string(data), "%d", &pid); err != nil {
		return 0
	}
	return pid
}
