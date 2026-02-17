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

func TestHandleProviderUnknownEventAddsDedupedSystemMessage(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())

	ev := providers.Event{Type: providers.EventUnknown, SessionID: s.ID, Provider: "codex", RawType: "item.completed"}
	eng.HandleProviderEvent(ev)
	eng.HandleProviderEvent(ev)

	msgs := store.ActiveSession().Messages
	count := 0
	for _, m := range msgs {
		if strings.Contains(m.Content, "Unhandled provider event 'item.completed'") {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one deduped unknown-event system message, got %d", count)
	}
}

func TestHandleProviderUnknownEventDifferentTypesEachVisible(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())

	eng.HandleProviderEvent(providers.Event{Type: providers.EventUnknown, SessionID: s.ID, Provider: "codex", RawType: "item.completed"})
	eng.HandleProviderEvent(providers.Event{Type: providers.EventUnknown, SessionID: s.ID, Provider: "codex", RawType: "tool.preview"})

	msgs := store.ActiveSession().Messages
	foundItemCompleted := false
	foundToolPreview := false
	for _, m := range msgs {
		if strings.Contains(m.Content, "Unhandled provider event 'item.completed'") {
			foundItemCompleted = true
		}
		if strings.Contains(m.Content, "Unhandled provider event 'tool.preview'") {
			foundToolPreview = true
		}
	}
	if !foundItemCompleted || !foundToolPreview {
		t.Fatalf("expected both unknown event types to be shown, found item=%v tool=%v", foundItemCompleted, foundToolPreview)
	}
}

func TestHandleProviderReasoningRendersConciseLine(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventReasoning,
		SessionID: s.ID,
		Text:      "**Planning project type detection**",
	})

	msgs := store.ActiveSession().Messages
	if len(msgs) == 0 || !strings.Contains(msgs[len(msgs)-1].Content, "[agent-thought] Planning project type detection") {
		t.Fatalf("expected concise reasoning system message, got %#v", msgs)
	}
}

func TestHandleProviderCommandLifecycleAndOutput(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())

	eng.HandleProviderEvent(providers.Event{
		Type:          providers.EventCommandExecution,
		SessionID:     s.ID,
		Command:       "go test ./...",
		CommandStatus: "in_progress",
	})
	zero := 0
	eng.HandleProviderEvent(providers.Event{
		Type:            providers.EventCommandExecution,
		SessionID:       s.ID,
		Command:         "go test ./...",
		CommandStatus:   "completed",
		CommandExitCode: &zero,
		CommandOutput:   "ok package/a\nok package/b",
	})

	msgs := store.ActiveSession().Messages
	all := make([]string, 0, len(msgs))
	for _, m := range msgs {
		all = append(all, m.Content)
	}
	joined := strings.Join(all, "\n")
	if !strings.Contains(joined, "Running command: go test ./...") {
		t.Fatalf("expected running command message, got %q", joined)
	}
	if !strings.Contains(joined, "Command completed (exit=0): go test ./...") {
		t.Fatalf("expected completed command message, got %q", joined)
	}
	if !strings.Contains(joined, "Command output:\nok package/a\nok package/b") {
		t.Fatalf("expected command output summary, got %q", joined)
	}
}

func TestHandleProviderCommandOutputTruncation(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())

	output := strings.Repeat("line\n", 20)
	one := 1
	eng.HandleProviderEvent(providers.Event{
		Type:            providers.EventCommandExecution,
		SessionID:       s.ID,
		Command:         "echo test",
		CommandStatus:   "failed",
		CommandExitCode: &one,
		CommandOutput:   output,
	})

	msgs := store.ActiveSession().Messages
	if len(msgs) < 2 {
		t.Fatalf("expected command status + output messages")
	}
	last := msgs[len(msgs)-1].Content
	if !strings.Contains(last, "... (truncated)") {
		t.Fatalf("expected truncated marker in command output, got %q", last)
	}
}

func TestHandleProviderTurnUsageIsNotRendered(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())
	before := len(store.ActiveSession().Messages)

	eng.HandleProviderEvent(providers.Event{
		Type:                   providers.EventTurnUsage,
		SessionID:              s.ID,
		UsageInputTokens:       1,
		UsageCachedInputTokens: 2,
		UsageOutputTokens:      3,
	})

	after := len(store.ActiveSession().Messages)
	if after != before {
		t.Fatalf("expected usage event to be log-only (no transcript message), before=%d after=%d", before, after)
	}
}
