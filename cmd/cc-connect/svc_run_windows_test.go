//go:build windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilterArgs_RemovesTarget(t *testing.T) {
	input := []string{"cc-connect.exe", "--service", "--config", "x.toml"}
	want := []string{"cc-connect.exe", "--config", "x.toml"}
	got := filterArgs(input, "--service")
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d; got %v", len(got), len(want), got)
	}
	for i, v := range got {
		if v != want[i] {
			t.Errorf("got[%d] = %q, want %q", i, v, want[i])
		}
	}
}

func TestFilterArgs_NoTarget(t *testing.T) {
	input := []string{"cc-connect.exe", "--config", "x.toml"}
	got := filterArgs(input, "--service")
	if len(got) != len(input) {
		t.Fatalf("len = %d, want %d; got %v", len(got), len(input), got)
	}
	for i, v := range got {
		if v != input[i] {
			t.Errorf("got[%d] = %q, want %q", i, v, input[i])
		}
	}
}

func TestFilterArgs_MultipleTarget(t *testing.T) {
	input := []string{"cc-connect.exe", "--service", "--service"}
	got := filterArgs(input, "--service")
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1; got %v", len(got), got)
	}
	if got[0] != "cc-connect.exe" {
		t.Errorf("got[0] = %q, want %q", got[0], "cc-connect.exe")
	}
}

func TestFilterArgs_Empty(t *testing.T) {
	got := filterArgs(nil, "--service")
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0; got %v", len(got), got)
	}
}

func TestFilterArgs_ExactMatch(t *testing.T) {
	input := []string{"cc-connect.exe", "--service-mode", "--service"}
	got := filterArgs(input, "--service")
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2; got %v", len(got), got)
	}
	// "--service-mode" should NOT be filtered (prefix but not exact match)
	if got[1] != "--service-mode" {
		t.Errorf("got[1] = %q, want %q", got[1], "--service-mode")
	}
}

func TestFixSvcEnvironmentWritesDiagFile(t *testing.T) {
	// Set SYSTEM account marker so fixSvcEnvironment actually runs.
	origUserProfile := os.Getenv("USERPROFILE")
	os.Setenv("USERPROFILE", `C:\Windows\System32\config\systemprofile`)
	defer os.Setenv("USERPROFILE", origUserProfile)

	// Create a temp directory to simulate dataDir.
	dir := t.TempDir()
	configFile := filepath.Join(dir, "config.toml")

	// Write a minimal daemon.json with EnvExtra.
	metaContent := `{"log_file":"","work_dir":"` + filepath.ToSlash(dir) + `","home_dir":"C:/Users/TestUser","env_path":"C:/bin","env_extra":{"ANTHROPIC_API_KEY":"sk-test-key","HTTPS_PROXY":"http://proxy:8080"},"installed_at":"2026-01-01T00:00:00Z"}`
	metaPath := filepath.Join(dir, "daemon.json")
	if err := os.WriteFile(metaPath, []byte(metaContent), 0644); err != nil {
		t.Fatal(err)
	}

	// Modify os.Args so fixSvcEnvironment can parse --config.
	origArgs := os.Args
	os.Args = []string{"cc-connect.exe", "--config", configFile}
	defer func() { os.Args = origArgs }()

	fixSvcEnvironment()

	// Verify HOME was restored.
	if os.Getenv("HOME") != `C:\Users\TestUser` {
		t.Errorf("HOME = %q, want %q", os.Getenv("HOME"), `C:\Users\TestUser`)
	}

	// Verify EnvExtra was restored.
	if os.Getenv("ANTHROPIC_API_KEY") != "sk-test-key" {
		t.Errorf("ANTHROPIC_API_KEY = %q, want %q", os.Getenv("ANTHROPIC_API_KEY"), "sk-test-key")
	}
	if os.Getenv("HTTPS_PROXY") != "http://proxy:8080" {
		t.Errorf("HTTPS_PROXY = %q, want %q", os.Getenv("HTTPS_PROXY"), "http://proxy:8080")
	}

	// Verify diagnostic file was created.
	diagPath := filepath.Join(dir, "svc-env-fix.log")
	data, err := os.ReadFile(diagPath)
	if err != nil {
		t.Fatalf("diagnostic file not found at %s: %v", diagPath, err)
	}
	content := string(data)
	if !strings.Contains(content, "fixSvcEnvironment started") {
		t.Errorf("diagnostic file should contain 'fixSvcEnvironment started', got:\n%s", content)
	}
	if !strings.Contains(content, "fixSvcEnvironment completed") {
		t.Errorf("diagnostic file should contain 'fixSvcEnvironment completed', got:\n%s", content)
	}
	if !strings.Contains(content, "restored EnvExtra, count=2") {
		t.Errorf("diagnostic file should mention EnvExtra count, got:\n%s", content)
	}
}
