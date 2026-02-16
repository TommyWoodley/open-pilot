package app

import (
	"errors"
	"testing"

	"github.com/thwoodle/open-pilot/internal/config"
	"github.com/thwoodle/open-pilot/internal/domain"
	"github.com/thwoodle/open-pilot/internal/providers"
)

func TestHandleProviderErrorKeepsMessageConcise(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID

	m.handleProviderEvent(providers.Event{
		Type:    providers.EventError,
		Message: "network down",
		Err:     errors.New("exit status 1"),
	})

	s = m.activeSession()
	if s == nil || len(s.Messages) == 0 {
		t.Fatalf("expected system message")
	}
	got := s.Messages[len(s.Messages)-1].Content
	if got != "Provider error: network down" {
		t.Fatalf("expected concise provider error, got %q", got)
	}
}

func TestFinalEventReplacesPreviewContent(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID
	s.Messages = append(s.Messages, domain.Message{
		ID:        "msg-1",
		Role:      domain.RoleAssistant,
		Content:   "preview",
		Streaming: true,
	})
	m.pending["req-1"] = len(s.Messages) - 1

	m.handleProviderEvent(providers.Event{
		Type:      providers.EventFinal,
		RequestID: "req-1",
		Text:      "final answer",
	})

	s = m.activeSession()
	got := s.Messages[len(s.Messages)-1]
	if got.Content != "final answer" {
		t.Fatalf("expected final content replacement, got %q", got.Content)
	}
	if got.Streaming {
		t.Fatalf("expected message to stop streaming")
	}
}
