//go:build windows

package main

import (
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
