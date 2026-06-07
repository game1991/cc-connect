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
const killWaitInterval = 25 * time.Millisecond

const processQueryLimitedInformation = 0x1000

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

func KillExistingInstance(configPath string) bool {
	configDir := filepath.Dir(configPath)
	configBase := filepath.Base(configPath)
	lockName := fmt.Sprintf(".%s.lock", configBase)
	lockPath := filepath.Join(configDir, lockName)

	pid := readPIDFromLockFile(lockPath)
	if pid <= 0 {
		return false
	}

	// Open process with both TERMINATE and QUERY access so we can
	// verify the image name on the same handle before terminating.
	handle, err := syscall.OpenProcess(
		syscall.PROCESS_TERMINATE|processQueryLimitedInformation,
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

	deadline := time.Now().Add(killWaitTimeout)
	for time.Now().Before(deadline) {
		_, err := syscall.OpenProcess(processQueryLimitedInformation, false, uint32(pid))
		if err != nil {
			return true
		}
		time.Sleep(killWaitInterval)
	}
	return false
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
