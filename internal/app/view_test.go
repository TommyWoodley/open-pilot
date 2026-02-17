package app

import (
	"strings"
	"testing"

	"github.com/thwoodle/open-pilot/internal/config"
	"github.com/thwoodle/open-pilot/internal/domain"
)

func TestViewContainsCoreSections(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.Width = 80
	view := m.View()

	checks := []string{
		"open-pilot",
		"session=none",
		"> ",
	}

	for _, expected := range checks {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected view to contain %q", expected)
		}
	}
}

func TestStatusShowsSessionName(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	s := m.createSession("my-session-name")
	m.ActiveSessionID = s.ID

	view := m.View()
	if !strings.Contains(view, "session=my-session-name") {
		t.Fatalf("expected status to include session name, got: %s", view)
	}
}

func TestViewRendersMarkdownAndCodeBlockContent(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID
	s.Messages = append(s.Messages, domain.Message{
		ID:      "msg-1",
		Role:    domain.RoleAssistant,
		Content: "# Heading\n- item\n```go\nfmt.Println(\"x\")\n```",
	})
	m.Width = 100
	m.Height = 24

	view := m.View()
	checks := []string{
		"[agent]",
		"Heading",
		"- item",
		"[go]",
		"fmt.Println(\"x\")",
	}
	for _, expected := range checks {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected view to contain %q", expected)
		}
	}
}

func TestViewRendersInlineEmphasisContent(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID
	s.Messages = append(s.Messages, domain.Message{
		ID:   "msg-2",
		Role: domain.RoleAssistant,
		Content: "This repo is a **Cisco DLP scanning service** plus " +
			"its _deployment_ and ~~legacy~~ stack.",
	})
	m.Width = 100
	m.Height = 24

	view := m.View()
	checks := []string{
		"Cisco DLP scanning service",
		"deployment",
		"legacy",
	}
	for _, expected := range checks {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected view to contain %q", expected)
		}
	}
}

func TestViewRendersLinkFallbackAndQuoteBlock(t *testing.T) {
	t.Setenv("OPEN_PILOT_DISABLE_OSC8", "1")

	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID
	s.Messages = append(s.Messages, domain.Message{
		ID:      "msg-3",
		Role:    domain.RoleAssistant,
		Content: "> Blockquote text\n[Link text](https://example.com)",
	})
	m.Width = 100
	m.Height = 24

	view := m.View()
	checks := []string{
		"Blockquote text",
		"Link text (https://example.com)",
		"│",
	}
	for _, expected := range checks {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected view to contain %q", expected)
		}
	}
}

func TestSessionListRendersWithoutActiveSession(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	_ = m.useSession(s.ID)
	m.ActiveSessionID = ""

	m.runCommand(Command{Kind: "session.list"})
	view := m.View()

	if !strings.Contains(view, "demo") {
		t.Fatalf("expected session list in view without active session, got: %s", view)
	}
}

func TestViewRendersDotsPlaceholderForEmptyStreamingMessage(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID
	s.Messages = append(s.Messages, domain.Message{
		ID:        "msg-stream-1",
		Role:      domain.RoleAssistant,
		Content:   "",
		Streaming: true,
	})
	m.Width = 100
	m.Height = 24

	m.GeneratingTick = 0
	lines1 := m.buildTranscriptLines()
	m.GeneratingTick = 1
	lines2 := m.buildTranscriptLines()
	m.GeneratingTick = 2
	lines3 := m.buildTranscriptLines()

	if len(lines1) == 0 || !strings.HasSuffix(lines1[0], ".") {
		t.Fatalf("expected single-dot placeholder, got lines=%v", lines1)
	}
	if len(lines2) == 0 || !strings.HasSuffix(lines2[0], "..") {
		t.Fatalf("expected two-dot placeholder, got lines=%v", lines2)
	}
	if len(lines3) == 0 || !strings.HasSuffix(lines3[0], "...") {
		t.Fatalf("expected three-dot placeholder, got lines=%v", lines3)
	}
}

func TestViewRendersDotsSuffixForStreamingMessageWithContent(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID
	s.Messages = append(s.Messages, domain.Message{
		ID:        "msg-stream-2",
		Role:      domain.RoleAssistant,
		Content:   "partial text",
		Streaming: true,
	})
	m.Width = 100
	m.Height = 24
	m.GeneratingTick = 2

	lines := m.buildTranscriptLines()
	if len(lines) < 2 || !strings.Contains(lines[0], "partial text") {
		t.Fatalf("expected streamed content in transcript lines, got %v", lines)
	}
	if lines[1] != "..." {
		t.Fatalf("expected animated suffix dots line, got %v", lines)
	}
}

func TestStatusShowsDotsWhenBusy(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.ProviderState = "busy"
	m.GeneratingTick = 1

	status := m.renderStatus()
	if !strings.Contains(status, "state=busy..") {
		t.Fatalf("expected status dots while busy, got %q", status)
	}
}
