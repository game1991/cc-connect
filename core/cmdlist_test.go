package core

import (
	"strings"
	"testing"
	"time"
)

func TestCmdList_PlainText_ContainsTableHeader(t *testing.T) {
	p := &stubPlatformEngine{n: "plain"}
	sessions := []AgentSessionInfo{
		{ID: "s1", Summary: "First session", MessageCount: 3, ModifiedAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)},
		{ID: "s2", Summary: "Second session", MessageCount: 5, ModifiedAt: time.Date(2026, 6, 2, 14, 30, 0, 0, time.UTC)},
	}
	e := NewEngine("test", &stubListAgent{sessions: sessions}, []Platform{p}, "", LangEnglish)
	msg := &Message{SessionKey: "test:user1", ReplyCtx: "ctx"}

	e.cmdList(p, msg, nil)

	if len(p.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(p.sent))
	}
	got := p.sent[0]

	// English table header
	header := "| # | Time | Session | Msgs |\n|:---|:---|:---|:---|"
	if !strings.Contains(got, header) {
		t.Errorf("plain text output = %q, want table header containing %q", got, header)
	}

	// Separator row with :--- markers must appear
	if !strings.Contains(got, "|:---|") {
		t.Errorf("plain text output missing separator row with |:---|, got %q", got)
	}
}

func TestCmdList_PlainText_PipeInDisplayName(t *testing.T) {
	p := &stubPlatformEngine{n: "plain"}
	// Summary contains pipe character that should be escaped
	sessions := []AgentSessionInfo{
		{ID: "s1", Summary: "fix bug | refactor code", MessageCount: 2, ModifiedAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)},
	}
	e := NewEngine("test", &stubListAgent{sessions: sessions}, []Platform{p}, "", LangEnglish)
	msg := &Message{SessionKey: "test:user1", ReplyCtx: "ctx"}

	e.cmdList(p, msg, nil)

	if len(p.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(p.sent))
	}
	got := p.sent[0]

	// Fullwidth pipe │ should replace ASCII pipe | in display names
	if !strings.Contains(got, "fix bug │ refactor code") {
		t.Errorf("plain text output = %q, want pipe in display name escaped to fullwidth │", got)
	}

	// Verify original ASCII pipe | does NOT appear in the data row area
	// (the header and separator contain | but data rows should use │ for display names)
	lines := strings.Split(got, "\n")
	for i, line := range lines {
		// Skip header lines (first two) and switch-hint line — they legitimately contain |
		if i < 2 {
			continue
		}
		// Skip lines that are part of table structure (separator or switch hint)
		if strings.HasPrefix(line, "|:---") || strings.Contains(line, "/switch") || strings.TrimSpace(line) == "" {
			continue
		}
		// Data row: the display name cell (3rd column) should have │ not |
		// We check that "fix bug | refactor" does not appear (unescaped)
		if strings.Contains(line, "fix bug | refactor") {
			t.Errorf("data row %d = %q, contains unescaped pipe in display name", i, line)
		}
	}
}

func TestCmdList_PlainText_EmptySummaryUsesI18n(t *testing.T) {
	p := &stubPlatformEngine{n: "plain"}
	// Session with empty summary
	sessions := []AgentSessionInfo{
		{ID: "s1", Summary: "", MessageCount: 0, ModifiedAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)},
	}
	e := NewEngine("test", &stubListAgent{sessions: sessions}, []Platform{p}, "", LangEnglish)
	msg := &Message{SessionKey: "test:user1", ReplyCtx: "ctx"}

	e.cmdList(p, msg, nil)

	if len(p.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(p.sent))
	}
	got := p.sent[0]

	// English i18n placeholder for empty summary is "(empty)"
	if !strings.Contains(got, "(empty)") {
		t.Errorf("plain text output = %q, want i18n empty summary placeholder (empty)", got)
	}

	// Also verify Chinese locale
	pZh := &stubPlatformEngine{n: "plain-zh"}
	eZh := NewEngine("test", &stubListAgent{sessions: sessions}, []Platform{pZh}, "", LangChinese)
	msgZh := &Message{SessionKey: "test:user1", ReplyCtx: "ctx"}

	eZh.cmdList(pZh, msgZh, nil)

	if len(pZh.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(pZh.sent))
	}
	gotZh := pZh.sent[0]
	if !strings.Contains(gotZh, "（空）") {
		t.Errorf("Chinese plain text output = %q, want i18n empty summary placeholder （空）", gotZh)
	}
}

func TestCmdList_PlainText_ActiveSessionMarker(t *testing.T) {
	p := &stubPlatformEngine{n: "plain"}
	sessions := []AgentSessionInfo{
		{ID: "s1", Summary: "Active session", MessageCount: 5, ModifiedAt: time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)},
		{ID: "s2", Summary: "Inactive session", MessageCount: 3, ModifiedAt: time.Date(2026, 6, 2, 14, 0, 0, 0, time.UTC)},
	}
	e := NewEngine("test", &stubListAgent{sessions: sessions}, []Platform{p}, "", LangEnglish)
	msg := &Message{SessionKey: "test:user1", ReplyCtx: "ctx"}

	// Set s1 as the active session so it gets the ▶ marker
	session := e.sessions.GetOrCreateActive(msg.SessionKey)
	session.SetAgentSessionID("s1", "test")

	e.cmdList(p, msg, nil)

	if len(p.sent) != 1 {
		t.Fatalf("sent messages = %d, want 1", len(p.sent))
	}
	got := p.sent[0]

	// The active session (s1) should have the ▶ marker
	if !strings.Contains(got, "▶ 1 |") {
		t.Errorf("plain text output = %q, want ▶ marker for active session s1", got)
	}

	// The inactive session (s2) should have the ◻ marker
	if !strings.Contains(got, "◻ 2 |") {
		t.Errorf("plain text output = %q, want ◻ marker for inactive session s2", got)
	}
}
