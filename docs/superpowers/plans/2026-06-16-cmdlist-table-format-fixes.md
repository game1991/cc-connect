# cmdList Table Format Fixes Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix code review findings from the `/list` markdown table format change — add unit tests for `cmdList` plain text path, and add pipe-character escaping to the card rendering path.

**Architecture:** Two focused changes: (1) new test file `core/cmdlist_test.go` covering plain text table output, pipe escaping, i18n placeholders, and active marker; (2) one-line addition in `core/engine.go` `renderListCard()` to escape `|` in display names for the card path.

**Tech Stack:** Go 1.x, standard `testing` package, existing `stubListAgent` / `stubPlatformEngine` test helpers.

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `core/cmdlist_test.go` | Create | Unit tests for `cmdList` plain text path (table format, pipe escape, i18n, marker) |
| `core/engine.go:12083` | Modify | Add `displayName = strings.ReplaceAll(displayName, "|", "│")` in `renderListCard` |

---

### Task 1: Create unit tests for cmdList plain text path

**Files:**
- Create: `core/cmdlist_test.go`
- Reference: `core/engine_test.go` (stub patterns: `stubListAgent`, `stubPlatformEngine`)

- [ ] **Step 1: Write the failing test — table header in output**

```go
package core

import (
	"strings"
	"testing"
	"time"
)

func TestCmdList_PlainText_ContainsTableHeader(t *testing.T) {
	p := &stubPlatformEngine{n: "plain"}
	sessions := []AgentSessionInfo{
		{ID: "s1", Summary: "First session", MessageCount: 3, ModifiedAt: time.Date(2026, 6, 12, 0, 20, 0, 0, time.UTC)},
	}
	e := NewEngine("test", &stubListAgent{sessions: sessions}, []Platform{p}, "", LangEnglish)
	msg := &Message{SessionKey: "test:user1", ReplyCtx: "ctx"}

	e.cmdList(p, msg, nil)

	if len(p.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(p.sent))
	}
	if !strings.Contains(p.sent[0], "| # | Time | Session | Msgs |") {
		t.Fatalf("output = %q, want table header row", p.sent[0])
	}
	if !strings.Contains(p.sent[0], "|:---|:---|:---|:---|") {
		t.Fatalf("output = %q, want table separator row", p.sent[0])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/ -run TestCmdList_PlainText_ContainsTableHeader -v`
Expected: FAIL — `go test` may fail to compile due to pre-existing `session_id_validation_test.go` signature mismatch. If so, use: `go test ./core/ -run TestCmdList_PlainText_ContainsTableHeader -v -count=1 2>&1 | head -30` and look for the test-specific output.

- [ ] **Step 3: Write the failing test — pipe character escaped**

```go
func TestCmdList_PlainText_PipeInDisplayName(t *testing.T) {
	p := &stubPlatformEngine{n: "plain"}
	sessions := []AgentSessionInfo{
		{ID: "s1", Summary: "test | pipe | name", MessageCount: 1, ModifiedAt: time.Date(2026, 6, 12, 0, 20, 0, 0, time.UTC)},
	}
	e := NewEngine("test", &stubListAgent{sessions: sessions}, []Platform{p}, "", LangEnglish)
	msg := &Message{SessionKey: "test:user1", ReplyCtx: "ctx"}

	e.cmdList(p, msg, nil)

	if len(p.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(p.sent))
	}
	// Pipe characters in display names should be replaced with fullwidth │
	if strings.Count(p.sent[0], "test │ pipe │ name") != 1 {
		t.Fatalf("output = %q, want pipe characters escaped to │", p.sent[0])
	}
	// Original half-width pipe should NOT appear in the display name portion
	// (it may appear in the table header/separator lines)
	lines := strings.Split(p.sent[0], "\n")
	for _, line := range lines {
		if strings.Contains(line, "test") && strings.Contains(line, "|") && !strings.Contains(line, "│") {
			t.Fatalf("line = %q, half-width pipe should be escaped in display name", line)
		}
	}
}
```

- [ ] **Step 4: Run test to verify it fails (should PASS since code already has the fix)**

Run: `go test ./core/ -run TestCmdList_PlainText_PipeInDisplayName -v`
Expected: PASS — pipe escaping is already implemented in the code change being reviewed.

- [ ] **Step 5: Write the failing test — empty summary uses i18n placeholder**

```go
func TestCmdList_PlainText_EmptySummaryUsesI18n(t *testing.T) {
	p := &stubPlatformEngine{n: "plain"}
	sessions := []AgentSessionInfo{
		{ID: "s1", Summary: "", MessageCount: 0, ModifiedAt: time.Date(2026, 6, 12, 0, 20, 0, 0, time.UTC)},
	}
	e := NewEngine("test", &stubListAgent{sessions: sessions}, []Platform{p}, "", LangChinese)
	msg := &Message{SessionKey: "test:user1", ReplyCtx: "ctx"}

	e.cmdList(p, msg, nil)

	if len(p.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(p.sent))
	}
	// Chinese i18n placeholder for empty summary is "（空）"
	if !strings.Contains(p.sent[0], "（空）") {
		t.Fatalf("output = %q, want i18n empty summary placeholder '（空）'", p.sent[0])
	}
}
```

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./core/ -run TestCmdList_PlainText_EmptySummaryUsesI18n -v`
Expected: PASS — `e.i18n.T(MsgListEmptySummary)` already returns localized string.

- [ ] **Step 7: Write the failing test — active session has marker**

```go
func TestCmdList_PlainText_ActiveSessionMarker(t *testing.T) {
	p := &stubPlatformEngine{n: "plain"}
	sessions := []AgentSessionInfo{
		{ID: "s1", Summary: "Active session", MessageCount: 5, ModifiedAt: time.Date(2026, 6, 12, 0, 20, 0, 0, time.UTC)},
		{ID: "s2", Summary: "Other session", MessageCount: 2, ModifiedAt: time.Date(2026, 6, 13, 0, 20, 0, 0, time.UTC)},
	}
	e := NewEngine("test", &stubListAgent{sessions: sessions}, []Platform{p}, "", LangEnglish)
	msg := &Message{SessionKey: "test:user1", ReplyCtx: "ctx"}
	session := e.sessions.GetOrCreateActive(msg.SessionKey)
	session.SetAgentSessionID("s1", "test")

	e.cmdList(p, msg, nil)

	if len(p.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(p.sent))
	}
	// Active session should have ▶ marker, inactive should have ◻
	if !strings.Contains(p.sent[0], "▶ 1") {
		t.Fatalf("output = %q, want ▶ marker for active session", p.sent[0])
	}
	if !strings.Contains(p.sent[0], "◻ 2") {
		t.Fatalf("output = %q, want ◻ marker for inactive session", p.sent[0])
	}
}
```

- [ ] **Step 8: Run test to verify it passes**

Run: `go test ./core/ -run TestCmdList_PlainText_ActiveSessionMarker -v`
Expected: PASS — marker logic already implemented.

- [ ] **Step 9: Run all new tests together**

Run: `go test ./core/ -run TestCmdList_PlainText -v`
Expected: All 4 tests PASS.

- [ ] **Step 10: Commit tests**

```bash
git add core/cmdlist_test.go
git commit -m "test: add unit tests for cmdList plain text table format

Covers table header, pipe escaping, i18n empty summary, and
active session marker for the plain text rendering path."
```

---

### Task 2: Add pipe-character escaping in renderListCard

**Files:**
- Modify: `core/engine.go:12083` (insert after the display name truncation block, before `btnType` assignment)

- [ ] **Step 1: Write the failing test — card path pipe escaping**

```go
func TestRenderListCard_PipeInDisplayNameEscaped(t *testing.T) {
	sessions := []AgentSessionInfo{
		{ID: "s1", Summary: "pipe | test | name", MessageCount: 1, ModifiedAt: time.Date(2026, 6, 12, 0, 20, 0, 0, time.UTC)},
	}
	e := NewEngine("test", &stubListAgent{sessions: sessions}, nil, "", LangEnglish)
	session := e.sessions.GetOrCreateActive("test:user1")
	session.SetAgentSessionID("s1", "test")

	card, err:= e.renderListCard("test:user1", 1)
	if err != nil {
		t.Fatalf("renderListCard error: %v", err)
	}

	// Card list items use MsgListItem format which includes the display name
	// The display name should have pipes escaped to fullwidth │
	for _, elem := range card.Elements {
		if item, ok := elem.(CardListItem); ok {
			if strings.Contains(item.Text, "|") && strings.Contains(item.Text, "pipe") && !strings.Contains(item.Text, "│") {
				t.Fatalf("list item text = %q, half-width pipe should be escaped to │", item.Text)
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./core/ -run TestRenderListCard_PipeInDisplayNameEscaped -v`
Expected: FAIL — `renderListCard` does not currently escape pipe characters.

- [ ] **Step 3: Write minimal implementation**

In `core/engine.go`, inside `renderListCard`, after the display name truncation block (after line 12083 `}` closing the else block) and before `btnType := "default"`, add:

```go
			displayName = strings.ReplaceAll(displayName, "|", "│")
```

The full context becomes:

```go
			if len([]rune(displayName)) > 40 {
				displayName = string([]rune(displayName)[:40]) + "…"
			}
			}
			displayName = strings.ReplaceAll(displayName, "|", "│")
			btnType := "default"
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./core/ -run TestRenderListCard_PipeInDisplayNameEscaped -v`
Expected: PASS.

- [ ] **Step 5: Run all new tests together**

Run: `go test ./core/ -run "TestCmdList_PlainText|TestRenderListCard_Pipe" -v`
Expected: All 5 tests PASS.

- [ ] **Step 6: Commit the fix**

```bash
git add core/engine.go
git commit -m "fix: escape pipe characters in display names for card list path

Aligns renderListCard with the plain text path which already
escapes | to │ to prevent table format breakage."
```

---

### Task 3: Final verification

- [ ] **Step 1: Run full core package build**

Run: `go build ./core/`
Expected: No errors.

- [ ] **Step 2: Run all cmdList-related tests**

Run: `go test ./core/ -run "TestCmdList|TestRenderListCard" -v`
Expected: All tests PASS (note: pre-existing `session_id_validation_test.go` compilation errors may prevent full `go test ./core/` — those are not introduced by this change).

- [ ] **Step 3: Commit any remaining changes**

If there are unstaged changes (unlikely), commit them:
```bash
git add -A
git commit -m "chore: final cleanup for cmdList table format changes"
```
