//go:build windows

package daemon

import (
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
		`& 'D:\tools\cc-connect.exe' --config`,
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
