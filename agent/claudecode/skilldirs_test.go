package claudecode

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
)

func TestSkillDirs_UsesClaudeConfigDirAndProjectParents(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	configHome := filepath.Join(tmp, "profile-home")
	repo := filepath.Join(tmp, "repo")
	workDir := filepath.Join(repo, "nested", "pkg")

	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", configHome)

	for _, dir := range []string{
		filepath.Join(repo, "nested", "pkg"),
		filepath.Join(repo, "nested"),
		repo,
		configHome,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", dir, err)
		}
	}
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: fake\n"), 0o644); err != nil {
		t.Fatalf("write .git: %v", err)
	}

	a := &Agent{workDir: workDir}
	got := a.SkillDirs()
	want := []string{
		filepath.Join(workDir, ".claude", "skills"),
		filepath.Join(repo, "nested", ".claude", "skills"),
		filepath.Join(repo, ".claude", "skills"),
		filepath.Join(configHome, "skills"),
	}
	if len(got) != len(want) {
		t.Fatalf("len(SkillDirs()) = %d, want %d\n got=%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("SkillDirs()[%d] = %q, want %q\nfull=%v", i, got[i], want[i], got)
		}
	}
}

func TestSkillDirs_FallsBackToHomeClaudeDir(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	workDir := filepath.Join(tmp, "workspace")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("mkdir workdir: %v", err)
	}
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	if runtime.GOOS == "windows" {
		t.Setenv("USERPROFILE", home)
	}

	a := &Agent{workDir: workDir}
	got := a.SkillDirs()
	wantLast := filepath.Join(home, ".claude", "skills")
	if got[len(got)-1] != wantLast {
		t.Fatalf("last SkillDirs() = %q, want %q\nfull=%v", got[len(got)-1], wantLast, got)
	}
}

func TestDiscoverPluginSkillDirs(t *testing.T) {
	tmp := t.TempDir()
	cacheDir := filepath.Join(tmp, "cache")

	mkSkill := func(paths ...string) {
		for _, p := range paths {
			if err := os.MkdirAll(p, 0o755); err != nil {
				t.Fatalf("mkdir %s: %v", p, err)
			}
			if err := os.WriteFile(filepath.Join(p, "SKILL.md"), []byte("---\ndescription: test\n---\nbody"), 0o644); err != nil {
				t.Fatalf("write SKILL.md: %v", err)
			}
		}
	}

	// superpowers plugin: cache/publisher/superpowers/5.1.0/skills/brainstorming/
	skillDir := filepath.Join(cacheDir, "pub", "superpowers", "5.1.0", "skills")
	mkSkill(filepath.Join(skillDir, "brainstorming"))

	// plugin with .claude/skills pattern
	dotClaudeSkill := filepath.Join(cacheDir, "pub", "vercel", "1.0.0", ".claude", "skills")
	mkSkill(filepath.Join(dotClaudeSkill, "deploy"))

	// plugin with no skills dir (should be ignored)
	if err := os.MkdirAll(filepath.Join(cacheDir, "pub", "noskill", "1.0.0", "src"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := discoverPluginSkillDirs(cacheDir)
	sort.Strings(got)

	want := []string{skillDir, dotClaudeSkill}
	sort.Strings(want)

	if len(got) != len(want) {
		t.Fatalf("discoverPluginSkillDirs() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("discoverPluginSkillDirs()[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestDiscoverPluginSkillDirs_EmptyCache(t *testing.T) {
	tmp := t.TempDir()
	got := discoverPluginSkillDirs(filepath.Join(tmp, "nonexistent"))
	if len(got) != 0 {
		t.Fatalf("expected empty result for nonexistent dir, got %v", got)
	}
}

func TestSkillDirs_IncludesPluginSkillDirs(t *testing.T) {
	tmp := t.TempDir()
	home := filepath.Join(tmp, "home")
	configHome := filepath.Join(tmp, "config")
	workDir := filepath.Join(tmp, "workspace")
	cacheDir := filepath.Join(configHome, "plugins", "cache")

	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_CONFIG_DIR", configHome)

	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatal(err)
	}

	skillDir := filepath.Join(cacheDir, "pub", "superpowers", "5.1.0", "skills")
	if err := os.MkdirAll(filepath.Join(skillDir, "brainstorming"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "brainstorming", "SKILL.md"), []byte("---\ndescription: test\n---\nbody"), 0o644); err != nil {
		t.Fatal(err)
	}

	a := &Agent{workDir: workDir}
	got := a.SkillDirs()

	found := false
	for _, d := range got {
		if d == skillDir {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("SkillDirs() missing plugin skill dir %q\nfull=%v", skillDir, got)
	}
}
