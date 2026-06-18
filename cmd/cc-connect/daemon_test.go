package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/chenhg5/cc-connect/daemon"
)

func TestParseDaemonInstallArgs_ConfigSetsWorkDir(t *testing.T) {
	cfg, force, err := parseDaemonInstallArgs([]string{"--config", "/tmp/example/config.toml"})
	if err != nil {
		t.Fatalf("parseDaemonInstallArgs returned error: %v", err)
	}
	if force {
		t.Fatalf("force = true, want false")
	}

	want := "/tmp/example"
	if filepath.ToSlash(cfg.WorkDir) != want {
		t.Fatalf("cfg.WorkDir = %q, want %q", cfg.WorkDir, want)
	}
}

func TestParseDaemonInstallArgs_ConfigEqualsFormSetsWorkDir(t *testing.T) {
	cfg, _, err := parseDaemonInstallArgs([]string{"--config=/tmp/example/config.toml"})
	if err != nil {
		t.Fatalf("parseDaemonInstallArgs returned error: %v", err)
	}

	want := "/tmp/example"
	if filepath.ToSlash(cfg.WorkDir) != want {
		t.Fatalf("cfg.WorkDir = %q, want %q", cfg.WorkDir, want)
	}
}

func TestParseDaemonInstallArgs_NoCaptureSecretsFlag(t *testing.T) {
	os.Unsetenv("CC_DAEMON_NO_CAPTURE_SECRETS")

	cfg, _, err := parseDaemonInstallArgs([]string{"--no-capture-secrets"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.NoCaptureSecrets {
		t.Fatal("flag should set NoCaptureSecrets=true")
	}

	cfg2, _, err := parseDaemonInstallArgs(nil)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg2.NoCaptureSecrets {
		t.Fatal("default must be false when flag and env are unset")
	}
}

func TestParseDaemonInstallArgs_NoCaptureSecretsEnv(t *testing.T) {
	for _, v := range []string{"1", "true", "TRUE", "yes", "on"} {
		t.Run("truthy="+v, func(t *testing.T) {
			t.Setenv("CC_DAEMON_NO_CAPTURE_SECRETS", v)
			cfg, _, err := parseDaemonInstallArgs(nil)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !cfg.NoCaptureSecrets {
				t.Fatalf("env=%q should opt out", v)
			}
		})
	}
	for _, v := range []string{"0", "false", "", "no", "off"} {
		t.Run("falsy="+v, func(t *testing.T) {
			t.Setenv("CC_DAEMON_NO_CAPTURE_SECRETS", v)
			cfg, _, err := parseDaemonInstallArgs(nil)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if cfg.NoCaptureSecrets {
				t.Fatalf("env=%q should NOT opt out", v)
			}
		})
	}
}

func TestParseDaemonInstallArgs_NoCaptureSecretsFlagAndEnvCombine(t *testing.T) {
	// OR semantics: env=truthy + flag=present → still true.
	t.Setenv("CC_DAEMON_NO_CAPTURE_SECRETS", "1")
	cfg, _, err := parseDaemonInstallArgs([]string{"--no-capture-secrets", "--force"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg.NoCaptureSecrets {
		t.Fatal("flag+env both should leave NoCaptureSecrets=true")
	}
	// env=truthy without flag → still true.
	cfg2, _, err := parseDaemonInstallArgs([]string{"--force"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !cfg2.NoCaptureSecrets {
		t.Fatal("env=1 alone should opt out")
	}
}

func TestParseDaemonInstallArgs_WorkDirOverridesConfig(t *testing.T) {
	cfg, force, err := parseDaemonInstallArgs([]string{
		"--config", "/tmp/example/config.toml",
		"--work-dir", "/tmp/override",
		"--force",
	})
	if err != nil {
		t.Fatalf("parseDaemonInstallArgs returned error: %v", err)
	}
	if !force {
		t.Fatalf("force = false, want true")
	}

	want := "/tmp/override"
	if filepath.ToSlash(cfg.WorkDir) != want {
		t.Fatalf("cfg.WorkDir = %q, want %q", cfg.WorkDir, want)
	}
}

func TestMetaConfigPath_WithConfigFile(t *testing.T) {
	meta := &daemon.Meta{
		WorkDir:    "/tmp/workdir",
		ConfigFile: "/custom/path/my-config.toml",
	}
	got := metaConfigPath(meta)
	want := "/custom/path/my-config.toml"
	if got != want {
		t.Errorf("metaConfigPath = %q, want %q", got, want)
	}
}

func TestMetaConfigPath_FallbackToWorkDir(t *testing.T) {
	meta := &daemon.Meta{
		WorkDir: "/tmp/workdir",
	}
	got := metaConfigPath(meta)
	want := filepath.Join("/tmp/workdir", "config.toml")
	if got != want {
		t.Errorf("metaConfigPath = %q, want %q", got, want)
	}
}

func TestMetaConfigPath_EmptyConfigFile(t *testing.T) {
	meta := &daemon.Meta{
		WorkDir:    "/tmp/workdir",
		ConfigFile: "",
	}
	got := metaConfigPath(meta)
	want := filepath.Join("/tmp/workdir", "config.toml")
	if got != want {
		t.Errorf("metaConfigPath = %q, want %q", got, want)
	}
}

func TestMetaConfigPath_BothEmpty(t *testing.T) {
	meta := &daemon.Meta{}
	got := metaConfigPath(meta)
	want := filepath.Join("", "config.toml")
	if got != want {
		t.Errorf("metaConfigPath = %q, want %q", got, want)
	}
}

// ── stopWithFallback tests ──────────────────────────────────

type stubManager struct {
	stopErr   error
	status    *daemon.Status
	statusErr error
}

func (s *stubManager) Install(daemon.Config) error { return nil }
func (s *stubManager) Uninstall() error            { return nil }
func (s *stubManager) Start() error                { return nil }
func (s *stubManager) Stop() error                 { return s.stopErr }
func (s *stubManager) Restart() error              { return nil }
func (s *stubManager) Status() (*daemon.Status, error) { return s.status, s.statusErr }
func (s *stubManager) Platform() string             { return "test" }

func TestStopWithFallback_StopSucceeds_ProcessGone(t *testing.T) {
	err := stopWithFallback(
		func() (daemon.Manager, error) {
			return &stubManager{stopErr: nil, status: &daemon.Status{Installed: true}}, nil
		},
		func() (*daemon.Meta, error) {
			return &daemon.Meta{WorkDir: "/tmp/work", ConfigFile: "/tmp/work/config.toml"}, nil
		},
		func(string) bool { return false }, // kill returns false → process already gone
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStopWithFallback_StopSucceeds_ProcessStillAlive(t *testing.T) {
	var stderr bytes.Buffer
	err := stopWithFallback(
		func() (daemon.Manager, error) {
			return &stubManager{stopErr: nil, status: &daemon.Status{Installed: true}}, nil
		},
		func() (*daemon.Meta, error) {
			return &daemon.Meta{WorkDir: "/tmp/work", ConfigFile: "/tmp/work/config.toml"}, nil
		},
		func(string) bool { return true }, // kill returns true → process was still alive
		&stderr,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(stderr.String(), "Warning:") {
		t.Errorf("expected warning about process still running, got %q", stderr.String())
	}
}

func TestStopWithFallback_LoadMetaFails(t *testing.T) {
	err := stopWithFallback(
		func() (daemon.Manager, error) {
			return &stubManager{stopErr: fmt.Errorf("schtasks failed"), status: &daemon.Status{Installed: true}}, nil
		},
		func() (*daemon.Meta, error) {
			return nil, fmt.Errorf("no daemon.json")
		},
		func(string) bool { return false },
		&bytes.Buffer{},
	)
	if err != nil {
		t.Fatalf("should return nil when loadMeta fails (assume platform stop worked): %v", err)
	}
}

func TestStopWithFallback_NotInstalled(t *testing.T) {
	err := stopWithFallback(
		func() (daemon.Manager, error) {
			return &stubManager{
				status:    &daemon.Status{Installed: false},
				statusErr: nil,
			}, nil
		},
		func() (*daemon.Meta, error) { return &daemon.Meta{}, nil },
		func(string) bool { return false },
		&bytes.Buffer{},
	)
	if err == nil {
		t.Fatal("expected error when service is not installed")
	}
	if !strings.Contains(err.Error(), "not installed") {
		t.Errorf("error should mention not installed, got %q", err.Error())
	}
}

func TestStopWithFallback_PropagatesPlatformStopError(t *testing.T) {
	stopErr := fmt.Errorf("access denied")
	var stderr bytes.Buffer
	err := stopWithFallback(
		func() (daemon.Manager, error) {
			return &stubManager{stopErr: stopErr, status: &daemon.Status{Installed: true}}, nil
		},
		func() (*daemon.Meta, error) {
			return &daemon.Meta{WorkDir: "/tmp/work", ConfigFile: "/tmp/work/config.toml"}, nil
		},
		func(string) bool { return false },
		&stderr,
	)
	if err != nil {
		t.Errorf("stopWithFallback returned error: %v", err)
	}
	if !bytes.Contains(stderr.Bytes(), []byte("Platform stop reported")) {
		t.Errorf("stderr should contain 'Platform stop reported', got: %q", stderr.String())
	}
}

func TestStopWithFallback_NewManagerFails(t *testing.T) {
	err := stopWithFallback(
		func() (daemon.Manager, error) { return nil, fmt.Errorf("unsupported platform") },
		func() (*daemon.Meta, error) { return &daemon.Meta{}, nil },
		func(string) bool { return false },
		&bytes.Buffer{},
	)
	if err == nil {
		t.Fatal("expected error when NewManager fails")
	}
	if !strings.Contains(err.Error(), "unsupported platform") {
		t.Errorf("error should propagate original message, got %q", err.Error())
	}
}
