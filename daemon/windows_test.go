//go:build windows

package daemon

import (
	"os"
	"strings"
	"testing"
)

func TestStrictPowerShellStopsOnCmdletErrors(t *testing.T) {
	script := strictPowerShell("Write-Output 'ok'")
	if !strings.HasPrefix(script, "$ErrorActionPreference = 'Stop'\n") {
		t.Fatalf("strictPowerShell() missing stop prelude:\n%s", script)
	}
	if !strings.Contains(script, "Write-Output 'ok'") {
		t.Fatalf("strictPowerShell() missing original script:\n%s", script)
	}
}

func TestBuildWindowsTaskScriptEnvAndWorkdir(t *testing.T) {
	cfg := Config{
		LogFile:    `C:\Users\me\.cc-connect\logs\cc-connect.log`,
		LogMaxSize: 999,
		BinaryPath: `D:\tools\cc-connect.exe`,
		ConfigFile: `C:\Users\me\.cc-connect\config.toml`,
		WorkDir:    `C:\Users\me\.cc-connect`,
		EnvPATH:    `D:\tools;C:\Windows`,
		EnvExtra:   map[string]string{"FOO": "bar", "BAZ": `"qux"`},
	}
	script := buildWindowsTaskScript(cfg)

	for _, want := range []string{
		`$env:CC_LOG_FILE = 'C:\Users\me\.cc-connect\logs\cc-connect.log'`,
		`$env:CC_LOG_MAX_SIZE = '999'`,
		`$env:PATH = 'D:\tools;C:\Windows'`,
		`$env:FOO = 'bar'`,
		`$env:BAZ = '"qux"'`,
		`Set-Location -LiteralPath 'C:\Users\me\.cc-connect'`,
		`$ccConnectExe = 'D:\tools\cc-connect.exe'`,
		`if (-not (Test-Path -LiteralPath $ccConnectExe))`,
		`& $ccConnectExe --config`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("task script missing %q:\n%s", want, script)
		}
	}
}

func TestDeleteWindowsTaskUsesSingleCall(t *testing.T) {
	orig := runPowerShell
	t.Cleanup(func() { runPowerShell = orig })

	var calls []string
	runPowerShell = func(s string) (string, error) {
		calls = append(calls, s)
		return "", nil
	}

	if err := deleteWindowsTask(); err != nil {
		t.Fatalf("deleteWindowsTask() error = %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected 1 PowerShell call (get+unregister combined), got %d", len(calls))
	}
	if !strings.Contains(calls[0], "Unregister-ScheduledTask") {
		t.Fatalf("call should unregister task:\n%s", calls[0])
	}
}

func TestCreateWindowsTaskRegisters(t *testing.T) {
	orig := runPowerShell
	t.Cleanup(func() { runPowerShell = orig })

	var script string
	runPowerShell = func(s string) (string, error) {
		script = s
		return "", nil
	}

	if err := createWindowsTask(`C:\Users\me\.cc-connect\cc-connect-daemon.ps1`); err != nil {
		t.Fatalf("createWindowsTask() error = %v", err)
	}
	for _, want := range []string{
		`New-ScheduledTaskAction`,
		`Register-ScheduledTask`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("script missing %q:\n%s", want, script)
		}
	}
}

func TestPowerShellLiteralEscapesSingleQuotes(t *testing.T) {
	got := powerShellLiteral(`C:\Users\O'Brien\.cc-connect`)
	want := `'C:\Users\O''Brien\.cc-connect'`
	if got != want {
		t.Fatalf("powerShellLiteral() = %q, want %q", got, want)
	}
}

func TestBuildWindowsTaskScript_DropsInvalidEnvName(t *testing.T) {
	cfg := Config{
		BinaryPath: "x", WorkDir: "y", LogFile: "l", LogMaxSize: 1, EnvPATH: "p",
		EnvExtra: map[string]string{"FOO BAR": "v", "OK": "ok"},
	}
	script := buildWindowsTaskScript(cfg)
	if strings.Contains(script, "FOO BAR") {
		t.Errorf("invalid env name leaked: %s", script)
	}
	if !strings.Contains(script, "$env:OK = 'ok'") {
		t.Errorf("valid env missing: %s", script)
	}
}

func TestBuildWindowsTaskScript_DropsEmptyValue(t *testing.T) {
	cfg := Config{
		BinaryPath: "x", WorkDir: "y", LogFile: "l", LogMaxSize: 1, EnvPATH: "p",
		EnvExtra: map[string]string{"EMPTY": "", "OK": "ok"},
	}
	script := buildWindowsTaskScript(cfg)
	if strings.Contains(script, "$env:EMPTY") {
		t.Errorf("empty value should be skipped: %s", script)
	}
}

// TestSchtasksInstall_TightensExistingScriptFrom0644 covers the upgrade
// path: os.WriteFile truncates in-place, so a script left by an earlier
// cc-connect version must be overwritten with the regenerated script.
//
// NOTE: On Windows, POSIX perm bits have weak semantics — only the
// read-only flag 0400 is honored, and 0600/0644/0666 all stat as 0666
// (see windows.go). Unlike the systemd/launchd counterparts, this test
// cannot assert the tightened 0600 mode via os.Stat; it verifies the
// legacy content is overwritten with the new cfg instead. Real access
// control is the file ACL under %USERPROFILE%.
func TestSchtasksInstall_TightensExistingScriptFrom0644(t *testing.T) {
	t.Setenv("USERPROFILE", t.TempDir())

	orig := runPowerShell
	t.Cleanup(func() { runPowerShell = orig })
	runPowerShell = func(script string) (string, error) { return "", nil }

	if err := os.MkdirAll(DefaultDataDir(), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	scriptPath := windowsTaskScriptPath()
	seed := []byte("$env:OLD = 'leftover'\r\n")
	if err := os.WriteFile(scriptPath, seed, 0o644); err != nil {
		t.Fatalf("seed legacy script: %v", err)
	}
	// Confirm seed content is in place. POSIX perm check skipped: on
	// Windows os.WriteFile(..., 0o644) stats as 0666 (only 0400 honored).
	if got, _ := os.ReadFile(scriptPath); string(got) != string(seed) {
		t.Fatalf("precondition: seeded content mismatch")
	}

	mgr := &schtasksManager{}
	cfg := Config{
		BinaryPath: "C:\\cc.exe",
		WorkDir:    t.TempDir(),
		LogFile:    "C:\\cc.log",
		LogMaxSize: 1024,
		EnvPATH:    "C:\\bin",
		EnvExtra:   map[string]string{"CUSTOM_TOKEN": "captured"},
	}
	if err := mgr.Install(cfg); err != nil {
		t.Fatalf("Install: %v", err)
	}
	got, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script after reinstall: %v", err)
	}
	if strings.Contains(string(got), "$env:OLD = 'leftover'") {
		t.Errorf("legacy content not overwritten:\n%s", got)
	}
	if !strings.Contains(string(got), "$env:CUSTOM_TOKEN = 'captured'") {
		t.Errorf("new env (CUSTOM_TOKEN) not written:\n%s", got)
	}
}
