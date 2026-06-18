# Code Review Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 3 Important and 4 Minor issues identified in code review of recent cc-connect commits (fde13091..15e06cc3).

**Architecture:** Three independent fixes targeting different subsystems — daemon lock cleanup, i18n table format, and startup error logging. Each task is self-contained and can be implemented/testd independently.

**Tech Stack:** Go 1.25, standard library only (slog, fmt, os, filepath, syscall)

---

### Task 1: Extract `RemoveInstanceLock` helper to eliminate daemon.go code duplication

**Files:**
- Modify: `cmd/cc-connect/instance_lock.go:25-65` (add `RemoveInstanceLock` function)
- Modify: `cmd/cc-connect/instance_lock_windows.go:25-70` (add `RemoveInstanceLock` function)
- Modify: `cmd/cc-connect/daemon.go:278-281` (replace inline lock removal with helper)
- Modify: `cmd/cc-connect/daemon.go:338-341` (replace inline lock removal with helper)
- Modify: `cmd/cc-connect/daemon.go:362-365` (replace inline lock removal with helper)
- Test: `cmd/cc-connect/instance_lock_test.go` (add test for `RemoveInstanceLock`)

- [ ] **Step 1: Write the failing test for `RemoveInstanceLock`**

In `cmd/cc-connect/instance_lock_test.go`, add:

```go
func TestRemoveInstanceLock(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.toml")

	// Acquire a lock to create the lock file
	lock, err := AcquireInstanceLock(configPath)
	if err != nil {
		t.Fatalf("AcquireInstanceLock: %v", err)
	}
	lockPath := lock.Path()
	lock.Release()

	// Lock file should still exist after Release on some platforms,
	// so RemoveInstanceLock must clean it up.
	if _, err := os.Stat(lockPath); os.IsNotExist(err) {
		t.Skip("lock file already removed by Release")
	}

	if err := RemoveInstanceLock(configPath); err != nil {
		t.Fatalf("RemoveInstanceLock: %v", err)
	}

	if _, err := os.Stat(lockPath); err == nil {
		t.Errorf("lock file still exists after RemoveInstanceLock")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd /d/WorkSpace/src/cc-connect && go test ./cmd/cc-connect/ -run TestRemoveInstanceLock -v`
Expected: FAIL with "undefined: RemoveInstanceLock"

- [ ] **Step 3: Implement `RemoveInstanceLock` in both platform files**

In `cmd/cc-connect/instance_lock.go` (non-windows), add after `KillExistingInstance`:

```go
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
```

In `cmd/cc-connect/instance_lock_windows.go`, add the same function (identical logic on Windows):

```go
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
```

- [ ] **Step 4: Replace inline lock removal in `daemon.go`**

In `cmd/cc-connect/daemon.go`, replace the three occurrences. Each looks like:

```go
// BEFORE (appears 3 times, at lines ~280, ~340, ~364):
configDir := filepath.Dir(configPath)
configBase := filepath.Base(configPath)
lockPath := filepath.Join(configDir, "."+configBase+".lock")
os.Remove(lockPath)

// AFTER (each site):
if err := RemoveInstanceLock(configPath); err != nil {
    slog.Warn("failed to remove stale lock file", "error", err)
}
```

Specifically:
1. In `daemonUninstall()` (~line 280): Replace the 4-line block with the 2-line helper call.
2. In `stopWithFallback()` (~line 340): Replace the 4-line block with the 2-line helper call.
3. In `daemonRestart()` (~line 364): Replace the 4-line block with the 2-line helper call.

- [ ] **Step 5: Run test to verify it passes**

Run: `cd /d/WorkSpace/src/cc-connect && go test ./cmd/cc-connect/ -run TestRemoveInstanceLock -v`
Expected: PASS

- [ ] **Step 6: Run full test suite**

Run: `cd /d/WorkSpace/src/cc-connect && go test ./...`
Expected: All tests pass

- [ ] **Step 7: Commit**

```bash
cd /d/WorkSpace/src/cc-connect
git add cmd/cc-connect/instance_lock.go cmd/cc-connect/instance_lock_windows.go cmd/cc-connect/instance_lock_test.go cmd/cc-connect/daemon.go
git commit -m "refactor(daemon): extract RemoveInstanceLock helper to eliminate triple code duplication

Consolidates lock file path construction and removal from daemon.go's
uninstall/stop/restart paths into a single RemoveInstanceLock function
in instance_lock.go. Adds non-existent-file tolerance and slog.Warn
for unexpected removal errors."
```

---

### Task 2: Move MsgListTableHeader/MsgListTableRow from i18n to Go constants

**Files:**
- Modify: `core/i18n.go:222-223` (remove `MsgListTableHeader` and `MsgListTableRow` MsgKey constants)
- Modify: `core/i18n.go:1456-1469` (remove the translation map entries)
- Modify: `core/engine.go:6369` (replace `e.i18n.T(MsgListTableHeader)` with Go constant)
- Modify: `core/engine.go:6390` (replace `e.i18n.T(MsgListTableRow)` with Go constant)
- Test: `core/cmdlist_test.go` (update test expectations if needed)

- [ ] **Step 1: Write failing test for Go constant-based table header**

The existing test `TestCmdList_PlainText_ContainsTableHeader` at `core/cmdlist_test.go:9-35` hardcodes the English header. We need to add a test that verifies the header is locale-independent (same for Chinese).

In `core/cmdlist_test.go`, add:

```go
func TestCmdList_PlainText_TableHeaderIsLocaleIndependent(t *testing.T) {
	p := &stubPlatformEngine{n: "locale-test"}
	sessions := []AgentSessionInfo{
		{ID: "s1", Summary: "test", MessageCount: 1, ModifiedAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)},
	}
	eEn := NewEngine("test", &stubListAgent{sessions: sessions}, []Platform{p}, "", LangEnglish)
	msg := &Message{SessionKey: "test:user1", ReplyCtx: "ctx"}
	eEn.cmdList(p, msg, nil)
	headerEn := "| # | Time | Session | Msgs |"

	pZh := &stubPlatformEngine{n: "locale-test-zh"}
	eZh := NewEngine("test", &stubListAgent{sessions: sessions}, []Platform{pZh}, "", LangChinese)
	msgZh := &Message{SessionKey: "test:user1", ReplyCtx: "ctx"}
	eZh.cmdList(pZh, msgZh, nil)
	headerZh := "| # | 时间 | 会话 | 消息数 |"

	// Both headers must be present in their respective locale outputs
	if !strings.Contains(p.sent[0], headerEn) {
		t.Errorf("English output missing header %q, got %q", headerEn, p.sent[0])
	}
	if !strings.Contains(pZh.sent[0], headerZh) {
		t.Errorf("Chinese output missing header %q, got %q", headerZh, pZh.sent[0])
	}
	// The separator row (|:---|...) must be identical in both locales
	if !strings.Contains(p.sent[0], "|:---|:---|:---|:---|") || !strings.Contains(pZh.sent[0], "|:---|:---|:---|:---|") {
		t.Error("separator row should be identical across locales")
	}
}
```

- [ ] **Step 2: Run test to see current state**

Run: `cd /d/WorkSpace/src/cc-connect && go test ./core/ -run TestCmdList_PlainText_TableHeaderIsLocaleIndependent -v`
Expected: This test should PASS currently since i18n returns locale-specific headers. After the refactoring, we need to ensure it still passes.

- [ ] **Step 3: Add Go constants in `core/engine.go`**

Near the top of `core/engine.go`, after the existing constants, add:

```go
const (
	listTableSep  = "|:---|:---|:---|:---|\n"
	listTableRow  = "| %s %d | %s | %s | %d |"
)
```

Then add a helper method on Engine:

```go
func (e *Engine) listTableHeader() string {
	return "| # | " + e.i18n.T(MsgListColTime) + " | " + e.i18n.T(MsgListColSession) + " | " + e.i18n.T(MsgListColMsgs) + " |\n" + listTableSep
}
```

This requires 3 new MsgKey constants for column names:

In `core/i18n.go`, add the keys:

```go
	MsgListColTime    MsgKey = "list_col_time"
	MsgListColSession MsgKey = "list_col_session"
	MsgListColMsgs    MsgKey = "list_col_msgs"
```

And their translations (these ARE locale-dependent):

```go
		MsgListColTime: {
			LangEnglish:            "Time",
			LangChinese:            "时间",
			LangTraditionalChinese: "時間",
			LangJapanese:           "時間",
			LangSpanish:            "Hora",
		},
		MsgListColSession: {
			LangEnglish:            "Session",
			LangChinese:            "会话",
			LangTraditionalChinese: "會話",
			LangJapanese:           "セッション",
			LangSpanish:            "Sesión",
		},
		MsgListColMsgs: {
			LangEnglish:            "Msgs",
			LangChinese:            "消息数",
			LangTraditionalChinese: "訊息數",
			LangJapanese:           "件",
			LangSpanish:            "Msgs",
		},
```

- [ ] **Step 4: Remove `MsgListTableHeader` and `MsgListTableRow` from i18n**

In `core/i18n.go`:
- Remove the constant declarations at lines 222-223
- Remove the translation map entries at lines 1456-1469

In `core/engine.go`:
- Replace `e.i18n.T(MsgListTableHeader)` at line 6369 with `e.listTableHeader()`
- Replace `e.i18n.T(MsgListTableRow)` at line 6390 with `listTableRow`

- [ ] **Step 5: Update existing tests**

In `core/cmdlist_test.go`:
- In `TestCmdList_PlainText_ContainsTableHeader`, update the hardcoded header string from the old format to the new format (the content is the same, just ensure it still matches).
- The existing header check `"| # | Time | Session | Msgs |\n|:---|:---|:---|:---|"` should still work since the new `listTableHeader()` produces the same string.

- [ ] **Step 6: Run tests**

Run: `cd /d/WorkSpace/src/cc-connect && go test ./core/ -v -run TestCmdList`
Expected: All pass

- [ ] **Step 7: Run full test suite**

Run: `cd /d/WorkSpace/src/cc-connect && go test ./...`
Expected: All pass

- [ ] **Step 8: Commit**

```bash
cd /d/WorkSpace/src/cc-connect
git add core/i18n.go core/engine.go core/cmdlist_test.go
git commit -m "refactor(i18n): move table format strings out of i18n, keep column names i18n

MsgListTableHeader/Row were identical across all 5 languages because
Markdown table syntax is structural, not linguistic. Replace with Go
constants (listTableSep, listTableRow) and new MsgKey entries for the
3 column headers (Time, Session, Msgs) that are genuinely locale-
dependent."
```

---

### Task 3: Unify dual logging in main.go startup error paths

**Files:**
- Modify: `cmd/cc-connect/main.go:254-290` (replace dual slog.Error + fmt.Fprintf with helper)
- Test: No new test needed — the helper is a 5-line function called only from main's startup path.

- [ ] **Step 1: Add `logExit` helper function**

In `cmd/cc-connect/main.go`, add before `func main()`:

```go
// logExit logs a startup error to both slog (for file/daemon consumers)
// and stderr (for interactive users), then exits. msg is the human-readable
// prefix; err provides the detail in both channels.
func logExit(msg string, err error, code int) {
	slog.Error(msg, "error", err)
	fmt.Fprintf(os.Stderr, "%s: %v\n", msg, err)
	os.Exit(code)
}
```

- [ ] **Step 2: Replace dual logging at each site**

At line 256-259 (instance lock failure):
```go
// BEFORE:
fmt.Fprintf(os.Stderr, "Error: %v\n", err)
fmt.Fprintf(os.Stderr, "Use --force to kill the existing instance.\n")
slog.Error("failed to acquire instance lock", "error", err)
os.Exit(1)

// AFTER:
slog.Error("failed to acquire instance lock", "error", err)
fmt.Fprintf(os.Stderr, "Error: %v\nUse --force to kill the existing instance.\n", err)
os.Exit(1)
```

Note: This site has an extra hint line ("Use --force..."), so it doesn't map cleanly to `logExit`. Keep the dual-write but make the slog and fmt messages consistent.

At line 265-267 (bootstrap config failure):
```go
// BEFORE:
slog.Error("failed to create default config", "path", configPath, "error", err)
fmt.Fprintf(os.Stderr, "Error creating config: %v\n", err)
os.Exit(1)

// AFTER:
logExit("failed to create default config", err, 1)
```

At line 276-278 (config load failure):
```go
// BEFORE:
slog.Error("failed to load config", "path", configPath, "error", err)
fmt.Fprintf(os.Stderr, "Error loading config (%s): %v\n", configPath, err)
os.Exit(1)

// AFTER:
logExit("failed to load config", err, 1)
```

At line 285-289 (no projects configured):
```go
// BEFORE:
slog.Error("no projects configured", "path", configPath)
fmt.Fprintf(os.Stderr, "Error: no projects configured in %s\n", configPath)
fmt.Fprintln(os.Stderr, "Add at least one [[project]] section to your config.toml, or run:")
fmt.Fprintln(os.Stderr, "  cc-connect init")
os.Exit(1)

// AFTER:
slog.Error("no projects configured", "path", configPath)
fmt.Fprintf(os.Stderr, "Error: no projects configured in %s\nAdd at least one [[project]] section to your config.toml, or run:\n  cc-connect init\n", configPath)
os.Exit(1)
```

Note: This site also has extra hint lines, so keep manual dual-write but compress the fmt.Fprintf calls.

- [ ] **Step 3: Verify build**

Run: `cd /d/WorkSpace/src/cc-connect && go build ./cmd/cc-connect/`
Expected: Build succeeds

- [ ] **Step 4: Run full test suite**

Run: `cd /d/WorkSpace/src/cc-connect && go test ./...`
Expected: All pass

- [ ] **Step 5: Commit**

```bash
cd /d/WorkSpace/src/cc-connect
git add cmd/cc-connect/main.go
git commit -m "refactor(main): unify startup error logging with logExit helper

Eliminates slog.Error + fmt.Fprintf dual-write inconsistency at startup
error paths. Adds logExit() for simple cases; keeps manual dual-write for
multi-line hint messages but ensures consistent wording between both channels."
```

---

## Dependency Order

Tasks 1, 2, and 3 are fully independent — they touch different files and subsystems. They can be implemented in any order or in parallel.

## Self-Review Checklist

- [x] **Spec coverage:** All 3 Important issues + os.Remove warning (from Minor #1) addressed
- [x] **Placeholder scan:** No TBD, TODO, "implement later", or vague steps
- [x] **Type consistency:** `RemoveInstanceLock(configPath string) error` signature matches all 3 call sites; `listTableHeader()` returns `string` matching `sb.WriteString` argument type; `logExit` parameter types match call sites
