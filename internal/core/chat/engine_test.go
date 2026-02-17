package chat

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/thwoodle/open-pilot/internal/config"
	"github.com/thwoodle/open-pilot/internal/core/command"
	"github.com/thwoodle/open-pilot/internal/core/session"
	"github.com/thwoodle/open-pilot/internal/providers"
)

type fakeManager struct {
	events chan providers.Event
}

func (f *fakeManager) SendPrompt(context.Context, string, string, string, string, string) error {
	return nil
}
func (f *fakeManager) Events() <-chan providers.Event { return f.events }
func (f *fakeManager) StopAll(context.Context) error  { return nil }

func TestPromptBlockedWithoutSession(t *testing.T) {
	store := session.NewStore()
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())
	eng.SendPrompt("hello")
	if eng.StatusText == "" {
		t.Fatalf("expected status text when prompt blocked")
	}
}

func TestRunCommandSessionNewSetsDefaultProvider(t *testing.T) {
	store := session.NewStore()
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())
	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "demo"})
	s := store.ActiveSession()
	if s == nil {
		t.Fatalf("expected active session")
	}
	if s.ProviderID != "codex" {
		t.Fatalf("expected provider codex, got %q", s.ProviderID)
	}
}

func TestRunCommandSessionNewRejectsDuplicateName(t *testing.T) {
	store := session.NewStore()
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())
	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "demo"})
	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "demo"})

	if len(store.SessionOrder) != 1 {
		t.Fatalf("expected duplicate name to be rejected")
	}
	if !strings.Contains(eng.StatusText, "already exists") {
		t.Fatalf("expected duplicate-name status text, got %q", eng.StatusText)
	}
}

func TestRunCommandSessionDeleteByName(t *testing.T) {
	store := session.NewStore()
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())
	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "demo"})
	eng.RunCommand(command.Command{Kind: command.KindSessionDelete, SessionID: "demo"})

	if len(store.SessionOrder) != 0 {
		t.Fatalf("expected session to be deleted")
	}
}

func TestHandleProviderErrorKeepsMessageConcise(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventError,
		SessionID: s.ID,
		Message:   "network down",
		Err:       errors.New("exit status 1"),
	})

	msgs := store.ActiveSession().Messages
	if len(msgs) == 0 {
		t.Fatalf("expected system message")
	}
	got := msgs[len(msgs)-1].Content
	if got != "Provider error: network down" {
		t.Fatalf("expected concise provider error, got %q", got)
	}
}

func TestHandleProviderFinalAppliesToPendingMessage(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	idx := store.AppendAssistantStreaming("codex", "repo-1")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())
	eng.pending["req-1"] = pendingRef{SessionID: s.ID, Index: idx}

	eng.HandleProviderEvent(providers.Event{Type: providers.EventFinal, SessionID: s.ID, RequestID: "req-1", Text: "final"})
	if got := store.ActiveSession().Messages[idx].Content; got != "final" {
		t.Fatalf("expected final text, got %q", got)
	}
	if store.ActiveSession().Messages[idx].Streaming {
		t.Fatalf("expected streaming=false after final")
	}
}

func TestRunCommandProviderStatus(t *testing.T) {
	store := session.NewStore()
	store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())
	eng.RunCommand(command.Command{Kind: command.KindProviderStatus})
	if !strings.Contains(eng.StatusText, "provider=") {
		t.Fatalf("expected provider status text, got %q", eng.StatusText)
	}
}

func TestHandleProviderReadyDoesNotClearBusyWhenPendingRequestExists(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	idx := store.AppendAssistantStreaming("codex", "repo-1")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())
	eng.pending["req-1"] = pendingRef{SessionID: s.ID, Index: idx}
	eng.ProviderState = "busy"
	eng.StatusText = "Sending prompt..."

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventReady,
		SessionID: s.ID,
		Message:   "provider ready",
	})

	if eng.ProviderState != "busy" {
		t.Fatalf("expected provider state to remain busy while pending, got %q", eng.ProviderState)
	}
}
