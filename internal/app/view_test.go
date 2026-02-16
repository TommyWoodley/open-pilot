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
