//go:build windows

package daemon

import (
	"errors"
	"strings"
	"testing"
	"time"
)

func TestIsAdmin(t *testing.T) {
	result := IsAdmin()
	t.Logf("IsAdmin() = %v", result)
}

func TestSvcManager_Status_NotInstalled(t *testing.T) {
	origRunSc := runSc
	t.Cleanup(func() { runSc = origRunSc })

	runSc = func(args ...string) (string, error) {
		return "[SC] EnumQueryServicesStatus: OpenService FAILED 1060:\n\nThe specified service does not exist asan installed service.", errors.New("exit status 1")
	}

	st, err := (&svcManager{}).Status()
	if err != nil {
		t.Fatalf("Status() returned error: %v", err)
	}
	if st.Installed {
		t.Error("Installed should be false for non-existent service")
	}
	if st.Running {
		t.Error("Running should be false for non-existent service")
	}
	if st.Platform != "svc" {
		t.Errorf("Platform = %q, want %q", st.Platform, "svc")
	}
}

func TestSvcManager_Status_Running(t *testing.T) {
	origRunSc := runSc
	t.Cleanup(func() { runSc = origRunSc })

	runSc = func(args ...string) (string, error) {
		return "STATE              : 4  RUNNING\nPID            : 1234\nPROCESS_ID         : 0", nil
	}

	st, err := (&svcManager{}).Status()
	if err != nil {
		t.Fatalf("Status() returned error: %v", err)
	}
	if !st.Installed {
		t.Error("Installed should be true")
	}
	if !st.Running {
		t.Error("Running should be true")
	}
	if st.PID != 1234 {
		t.Errorf("PID = %d, want 1234 (should ignore PROCESS_ID line)", st.PID)
	}
}

func TestSvcManager_Status_Stopped(t *testing.T) {
	origRunSc := runSc
	t.Cleanup(func() { runSc = origRunSc })

	runSc = func(args ...string) (string, error) {
		return "STATE              : 1  STOPPED", nil
	}

	st, err := (&svcManager{}).Status()
	if err != nil {
		t.Fatalf("Status() returned error: %v", err)
	}
	if !st.Installed {
		t.Error("Installed should be true")
	}
	if st.Running {
		t.Error("Running should be false for STOPPED state")
	}
}

func TestSvcManager_Status_UnexpectedError(t *testing.T) {
	origRunSc := runSc
	t.Cleanup(func() { runSc = origRunSc })

	runSc = func(args ...string) (string, error) {
		return "[SC] OpenSCManager FAILED 5: Access is denied.", errors.New("exit status 1")
	}

	st, err := (&svcManager{}).Status()
	if err == nil {
		t.Fatal("Status() should return error for access denied")
	}
	if st != nil {
		t.Errorf("Status() should return nil status on error, got %v", st)
	}
}

func TestSvcIsInstalled(t *testing.T) {
	origRunSc := runSc
	t.Cleanup(func() { runSc = origRunSc })

	runSc = func(args ...string) (string, error) {
		return "STATE              : 4  RUNNING", nil
	}

	ok, err := svcIsInstalled()
	if err != nil {
		t.Fatalf("svcIsInstalled() error: %v", err)
	}
	if !ok {
		t.Error("svcIsInstalled() should return true for running service")
	}
}

func TestPollStopped(t *testing.T) {
	count := 0
	check := func() bool {
		count++
		return count > 3
	}

	err := pollStopped(check, 5*time.Second)
	if err != nil {
		t.Fatalf("pollStopped() error: %v", err)
	}
	if count < 4 {
		t.Errorf("expected at least 4 checks, got %d", count)
	}
}

func TestPollStopped_Timeout(t *testing.T) {
	err := pollStopped(func() bool { return false }, 100*time.Millisecond)
	if err == nil {
		t.Fatal("pollStopped() should timeout")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("error should mention timeout, got: %v", err)
	}
}

func TestSvcManager_Platform(t *testing.T) {
	if got := (&svcManager{}).Platform(); got != "svc" {
		t.Errorf("Platform() = %q, want %q", got, "svc")
	}
}

func TestEscapeScPath(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"simple", `C:\app\bin.exe`, `C:\\app\\bin.exe`},
		{"trailing_slash", `C:\app\`, `C:\\app`},
		{"spaces", `C:\Program Files\app.exe`, `C:\\Program Files\\app.exe`},
		{"double_trailing", `C:\app\\`, `C:\\app`},
		{"no_backslash", `app.exe`, `app.exe`},
		{"quote", `C:\"evil"\app.exe`, `C:\\\"evil\"\\app.exe`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := escapeScPath(tt.in)
			if got != tt.want {
				t.Errorf("escapeScPath(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestEnvKeyRe(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"PATH", true},
		{"MY_VAR", true},
		{"_private", true},
		{"a1b2", true},
		{"1invalid", false},
		{"has space", false},
		{"has;semicolon", false},
		{"has\newline", false},
		{"", false},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got := envKeyRe.MatchString(tt.key)
			if got != tt.want {
				t.Errorf("envKeyRe.MatchString(%q) = %v, want %v", tt.key, got, tt.want)
			}
		})
	}
}

func TestProtectedEnvKeysCaseInsensitive(t *testing.T) {
	cases := []string{"ErrorActionPreference", "ERRORACTIONPREFERENCE", "erroractionpreference", "PsModulePath", "PSMODULEPATH"}
	for _, key := range cases {
		if !protectedEnvKeys[strings.ToLower(key)] {
			t.Errorf("protectedEnvKeys[%q] = false, want true (case-insensitive)", key)
		}
	}
}

func TestWritePowerShellEnv_SkipsNewlineValue(t *testing.T) {
	var sb strings.Builder
	writePowerShellEnv(&sb, "MY_VAR", "value\nwith\nnewlines")
	if sb.Len() > 0 {
		t.Errorf("writePowerShellEnv should skip values with newlines, but wrote: %q", sb.String())
	}
}
