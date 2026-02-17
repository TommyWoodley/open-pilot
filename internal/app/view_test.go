package app

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/thwoodle/open-pilot/internal/config"
	"github.com/thwoodle/open-pilot/internal/domain"
	"github.com/thwoodle/open-pilot/internal/providers"
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
	if strings.TrimSpace(lines[1]) != "..." {
		t.Fatalf("expected animated suffix dots line, got %v", lines)
	}
}

func TestStatusBusyDoesNotShowDots(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.ProviderState = "busy"
	m.GeneratingTick = 1

	status := m.renderStatus()
	if !strings.Contains(status, "state=busy") {
		t.Fatalf("expected busy state in status, got %q", status)
	}
	if strings.Contains(status, "state=busy..") || strings.Contains(status, "state=busy...") {
		t.Fatalf("expected no animated dots in status, got %q", status)
	}
}

func TestUnknownProviderEventAppearsInTranscript(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID
	m.Width = 100
	m.Height = 24

	m.handleProviderEvent(providers.Event{
		Type:      providers.EventUnknown,
		SessionID: s.ID,
		Provider:  "codex",
		RawType:   "item.completed",
	})
	view := m.View()
	if !strings.Contains(view, "Unhandled provider event 'item.completed' (details logged).") {
		t.Fatalf("expected unknown provider event system message in transcript, got: %s", view)
	}
}

func TestCommandAndReasoningEventsAppearInTranscript(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID
	m.Width = 100
	m.Height = 24

	m.handleProviderEvent(providers.Event{
		Type:      providers.EventReasoning,
		SessionID: s.ID,
		Text:      "**Planning project type detection**",
	})
	zero := 0
	m.handleProviderEvent(providers.Event{
		Type:            providers.EventCommandExecution,
		SessionID:       s.ID,
		Command:         "go test ./...",
		CommandStatus:   "completed",
		CommandExitCode: &zero,
		CommandOutput:   "ok package/a",
	})

	view := m.View()
	checks := []string{
		"[agent-thought] Planning project type detection",
		"Ran go test ./... for",
	}
	for _, expected := range checks {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected transcript to contain %q, got: %s", expected, view)
		}
	}
	if strings.Contains(view, "Command output:") || strings.Contains(view, "ok package/a") {
		t.Fatalf("did not expect verbose command output in transcript, got: %s", view)
	}
}

func TestViewFitsConfiguredHeight(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.Width = 120
	m.Height = 24
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID
	for i := 0; i < 60; i++ {
		s.Messages = append(s.Messages, domain.Message{
			ID:      fmt.Sprintf("msg-fit-%d", i),
			Role:    domain.RoleAssistant,
			Content: "line",
		})
	}

	view := m.View()
	if h := lipgloss.Height(view); h > m.Height {
		t.Fatalf("expected rendered view height <= %d, got %d", m.Height, h)
	}
}

func TestWrappedCommandOutputKeepsContinuationIndent(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID
	m.Width = 90
	m.Height = 24
	longLine := "go: writing stat cache: open /Users/thwoodle/go/pkg/mod/cache/download/github.com/thwoodle/open-pilot/@v/v0.0.0-20260217093314-837a977a3d15.info421941505.tmp: operation not permitted"
	s.Messages = append(s.Messages, domain.Message{
		ID:      "msg-cmd-wrap",
		Role:    domain.RoleSystem,
		Content: "Command output:\n" + longLine,
	})

	lines := m.displayTranscriptLines()
	if len(lines) < 2 {
		t.Fatalf("expected wrapped output to span multiple lines, got lines=%v", lines)
	}
	if !strings.HasPrefix(lines[0], "[pilot] Command output:") {
		t.Fatalf("expected first line to include pilot prefix, got %q", lines[0])
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "" {
			continue
		}
		if !strings.HasPrefix(lines[i], "        ") { // len("[pilot] ") = 8
			t.Fatalf("expected continuation line %d to keep pilot-body indent, got %q", i, lines[i])
		}
	}
}

func TestHookDividerTokenRendersStyledCenteredLine(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID
	m.Width = 100
	m.Height = 24
	s.Messages = append(s.Messages, domain.Message{
		ID:      "msg-hooks-divider",
		Role:    domain.RoleSystem,
		Content: "Hooks 0/1\n[[pilot-divider:Hooks 0/1]]",
	})

	lines := m.displayTranscriptLines()
	joined := strings.Join(lines, "\n")
	if strings.Contains(joined, "[[pilot-divider:") {
		t.Fatalf("expected divider token to be rendered, got %q", joined)
	}
	if !strings.Contains(joined, "Hooks 0/1") {
		t.Fatalf("expected divider title to be visible, got %q", joined)
	}
}
