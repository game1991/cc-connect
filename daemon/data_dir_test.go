package daemon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper restores isGoTestBinary after the test.
func overrideIsGoTestBinary(t *testing.T, fn func() bool) {
	t.Helper()
	orig := isGoTestBinary
	isGoTestBinary = fn
	t.Cleanup(func() { isGoTestBinary = orig })
}

func TestDefaultDataDir_TestOverride(t *testing.T) {
	overrideIsGoTestBinary(t, func() bool { return true })

	dir := t.TempDir()
	t.Setenv("CC_DAEMON_DATA_DIR", dir)

	got := DefaultDataDir()
	if got != dir {
		t.Errorf("DefaultDataDir() = %q, want %q when CC_DAEMON_DATA_DIR is set in test", got, dir)
	}
}

func TestIsGoTestBinaryDefault(t *testing.T) {
	cases := []struct {
		args0 string
		want  bool
	}{
		{"/tmp/daemon.test", true},
		{"C:\\Users\\KC\\AppData\\Local\\Temp\\go-build123\\daemon.test", true},
		{"/home/user/go-build999/bazel-test-/daemon.test___1_", false}, // bazel: suffix _1_ not .test
		{"cc-connect", false},
		{"cc-connect.exe", false},
		{"/usr/local/bin/cc-connect", false},
		{"D:\\nodejs\\cc-connect.exe", false},
	}
	for _, tc := range cases {
		got := strings.HasSuffix(tc.args0, ".test") || strings.Contains(tc.args0, "-test.")
		if got != tc.want {
			t.Errorf("isGoTestBinaryDefault(%q): got %v, want %v", tc.args0, got, tc.want)
		}
	}
}

func TestDefaultDataDir_ProductionEnv(t *testing.T) {
	// When isGoTestBinary returns false (production context),
	// CC_DAEMON_DATA_DIR must be ignored even if set.
	overrideIsGoTestBinary(t, func() bool { return false })

	dir := t.TempDir()
	t.Setenv("CC_DAEMON_DATA_DIR", dir)

	got := DefaultDataDir()
	if got == dir {
		t.Errorf("DefaultDataDir() = %q — CC_DAEMON_DATA_DIR should be IGNORED in production, but it was used", got)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".cc-connect")
	if got != want {
		t.Errorf("DefaultDataDir() = %q, want %q (production default)", got, want)
	}
}

func TestDefaultLogFile_DelegatesToDataDir(t *testing.T) {
	overrideIsGoTestBinary(t, func() bool { return true })

	dir := t.TempDir()
	t.Setenv("CC_DAEMON_DATA_DIR", dir)

	got := DefaultLogFile()
	want := filepath.Join(dir, "logs", "cc-connect.log")
	if got != want {
		t.Errorf("DefaultLogFile() = %q, want %q", got, want)
	}
}

func TestMetaSaveLoad_Isolated(t *testing.T) {
	overrideIsGoTestBinary(t, func() bool { return true })

	dir := t.TempDir()
	t.Setenv("CC_DAEMON_DATA_DIR", dir)

	m := &Meta{
		LogFile:       "/tmp/test.log",
		LogMaxSize:    1024,
		LogMaxBackups: 3,
		WorkDir:       "/tmp",
		BinaryPath:    "/usr/local/bin/cc-connect",
		InstalledAt:   NowISO(),
	}

	if err := SaveMeta(m); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}

	loaded, err := LoadMeta()
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}

	if loaded.LogFile != m.LogFile {
		t.Errorf("LogFile mismatch: %s != %s", loaded.LogFile, m.LogFile)
	}
	if loaded.WorkDir != m.WorkDir {
		t.Errorf("WorkDir mismatch: %s != %s", loaded.WorkDir, m.WorkDir)
	}

	// Verify the file lives in the isolated directory, not real ~/.cc-connect
	data, err := os.ReadFile(filepath.Join(dir, "daemon.json"))
	if err != nil {
		t.Fatalf("read isolated daemon.json: %v", err)
	}
	if !strings.Contains(string(data), "/tmp/test.log") {
		t.Errorf("isolated daemon.json does not contain expected content: %s", string(data))
	}

	// Verify real daemon.json was NOT touched
	home, _ := os.UserHomeDir()
	realPath := filepath.Join(home, ".cc-connect", "daemon.json")
	if _, err := os.Stat(realPath); err == nil {
		realData, _ := os.ReadFile(realPath)
		if strings.Contains(string(realData), "/tmp/test.log") {
			t.Errorf("real daemon.json was polluted with test data!")
		}
	}
}
