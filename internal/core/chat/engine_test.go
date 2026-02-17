package chat

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thwoodle/open-pilot/internal/config"
	"github.com/thwoodle/open-pilot/internal/core/command"
	corehooks "github.com/thwoodle/open-pilot/internal/core/hooks"
	"github.com/thwoodle/open-pilot/internal/core/session"
	"github.com/thwoodle/open-pilot/internal/providers"
)

type fakeManager struct {
	events chan providers.Event
}

type fakeHooks struct {
	result       corehooks.RunResult
	calls        int
	lastTrigger  config.HookTrigger
	lastRepoPath string
}

func (f *fakeHooks) Run(_ context.Context, trigger config.HookTrigger, _ string, repoPath string, _ func(corehooks.ProgressUpdate)) corehooks.RunResult {
	f.calls++
	f.lastTrigger = trigger
	f.lastRepoPath = repoPath
	return f.result
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
	start := time.Unix(1000, 0)
	eng.nowFn = func() time.Time { return start }

	eng.HandleProviderEvent(providers.Event{
		Type:          providers.EventCommandExecution,
		SessionID:     s.ID,
		ItemID:        "item-c",
		Command:       "go test ./...",
		CommandStatus: "in_progress",
	})
	eng.nowFn = func() time.Time { return start.Add(1200 * time.Millisecond) }
	zero := 0
	eng.HandleProviderEvent(providers.Event{
		Type:            providers.EventCommandExecution,
		SessionID:       s.ID,
		ItemID:          "item-c",
		Command:         "go test ./...",
		CommandStatus:   "completed",
		CommandExitCode: &zero,
		CommandOutput:   "ok package/a\nok package/b",
	})

	msgs := store.ActiveSession().Messages
	if len(msgs) != 1 {
		t.Fatalf("expected one upserted command message, got %d", len(msgs))
	}
	joined := msgs[0].Content
	if !strings.Contains(joined, "Ran `go test ./...` for 1.2s") {
		t.Fatalf("expected completed command message, got %q", joined)
	}
	if strings.Contains(joined, "Running ") {
		t.Fatalf("expected running line to be replaced by final summary, got %q", joined)
	}
	if strings.Contains(joined, "Command output:") || strings.Contains(joined, "ok package/a") {
		t.Fatalf("did not expect verbose command output in transcript, got %q", joined)
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
		ItemID:          "item-fail",
		Command:         "echo test",
		CommandStatus:   "failed",
		CommandExitCode: &one,
		CommandOutput:   output,
	})

	msgs := store.ActiveSession().Messages
	if len(msgs) == 0 {
		t.Fatalf("expected command message")
	}
	last := msgs[len(msgs)-1].Content
	if !strings.Contains(last, "failed") || !strings.Contains(last, "Error: line") {
		t.Fatalf("expected failed summary with teaser, got %q", last)
	}
}

func TestHandleProviderCommandExecutionUpsertsByItemID(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())

	eng.HandleProviderEvent(providers.Event{
		Type:          providers.EventCommandExecution,
		SessionID:     s.ID,
		ItemID:        "item-2",
		Command:       "go test ./...",
		CommandStatus: "in_progress",
	})
	zero := 0
	eng.HandleProviderEvent(providers.Event{
		Type:            providers.EventCommandExecution,
		SessionID:       s.ID,
		ItemID:          "item-2",
		Command:         "go test ./...",
		CommandStatus:   "completed",
		CommandExitCode: &zero,
		CommandOutput:   "ok package/a",
	})

	msgs := store.ActiveSession().Messages
	if len(msgs) != 1 {
		t.Fatalf("expected one upserted message for same item id, got %d", len(msgs))
	}
	got := msgs[0].Content
	if !strings.Contains(got, "Ran `go test ./...` for") {
		t.Fatalf("expected completed command content, got %q", got)
	}
	if strings.Contains(got, "Command output:") || strings.Contains(got, "ok package/a") {
		t.Fatalf("did not expect full command output in summary message, got %q", got)
	}
}

func TestHandleProviderReasoningUpsertsByItemID(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventReasoning,
		SessionID: s.ID,
		ItemID:    "item-r",
		Text:      "**Planning**",
	})
	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventReasoning,
		SessionID: s.ID,
		ItemID:    "item-r",
		Text:      "**Planning with more detail**",
	})

	msgs := store.ActiveSession().Messages
	if len(msgs) != 1 {
		t.Fatalf("expected one reasoning message for same item id, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "[agent-thought] Planning with more detail") {
		t.Fatalf("expected latest reasoning text to overwrite prior, got %q", msgs[0].Content)
	}
}

func TestHandleProviderAgentMessageClearsPendingPlaceholderAndUsesItemID(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	idx := store.AppendAssistantStreaming("codex", "repo-1")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())
	eng.pending["req-1"] = pendingRef{SessionID: s.ID, Index: idx}

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventAgentMessage,
		SessionID: s.ID,
		RequestID: "req-1",
		ItemID:    "item-1",
		Text:      "first message",
	})
	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventAgentMessage,
		SessionID: s.ID,
		RequestID: "req-1",
		ItemID:    "item-2",
		Text:      "final message",
	})

	msgs := store.ActiveSession().Messages
	if len(msgs) != 2 {
		t.Fatalf("expected placeholder removed and two agent messages kept, got %d", len(msgs))
	}
	if msgs[0].Content != "first message" {
		t.Fatalf("unexpected first message content: %q", msgs[0].Content)
	}
	if msgs[1].Content != "final message" {
		t.Fatalf("unexpected second message content: %q", msgs[1].Content)
	}
	if _, ok := eng.pending["req-1"]; ok {
		t.Fatalf("expected pending request to be cleared after itemized message flow")
	}
}

func TestHandleProviderCommandExecutionUsesExploredSummaryForDiscoveryCommands(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())
	start := time.Unix(1000, 0)
	eng.nowFn = func() time.Time { return start }

	eng.HandleProviderEvent(providers.Event{
		Type:          providers.EventCommandExecution,
		SessionID:     s.ID,
		ItemID:        "item-ls",
		Command:       "/bin/bash -lc 'ls -la && rg --files'",
		CommandStatus: "in_progress",
	})
	eng.nowFn = func() time.Time { return start.Add(400 * time.Millisecond) }
	zero := 0
	eng.HandleProviderEvent(providers.Event{
		Type:            providers.EventCommandExecution,
		SessionID:       s.ID,
		ItemID:          "item-ls",
		Command:         "/bin/bash -lc 'ls -la && rg --files'",
		CommandStatus:   "completed",
		CommandExitCode: &zero,
	})

	msgs := store.ActiveSession().Messages
	if len(msgs) != 1 {
		t.Fatalf("expected single upserted message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "Explored for 400ms") {
		t.Fatalf("expected explored summary, got %q", msgs[0].Content)
	}
	if strings.Contains(msgs[0].Content, "ls -la") || strings.Contains(msgs[0].Content, "rg --files") {
		t.Fatalf("did not expect explored summary to include command text, got %q", msgs[0].Content)
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

func TestSessionNewRunsStartupHooksAndBlocksPromptOnFailure(t *testing.T) {
	store := session.NewStore()
	cfg := config.Default()
	cfg.BuiltinHooks = config.HookCatalog{
		Hooks: []config.HookDefinition{
			{ID: "ensure-branch", Triggers: []config.HookTrigger{config.HookTriggerSessionStarted}, Execute: []string{"echo ok"}, Timeout: time.Second},
		},
	}
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, cfg)
	eng.Hooks = &fakeHooks{
		result: corehooks.RunResult{
			Passed:             false,
			HooksMatched:       1,
			FailedHookID:       "ensure-branch",
			FailedCommandIndex: 1,
			Reason:             "exit=1",
			PerHookResults: []corehooks.HookResult{
				{HookID: "ensure-branch", Passed: false, Reason: "exit=1"},
			},
		},
	}

	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "demo"})
	s := store.ActiveSession()
	if s == nil {
		t.Fatalf("expected active session")
	}
	if !s.HooksBlocked {
		t.Fatalf("expected hooks to block session on failure")
	}

	eng.SendPrompt("hello")
	if !strings.Contains(eng.StatusText, "Hooks blocked") {
		t.Fatalf("expected hooks blocked status, got %q", eng.StatusText)
	}
}

func TestHooksRunClearsBlockedStateOnSuccess(t *testing.T) {
	store := session.NewStore()
	cfg := config.Default()
	cfg.BuiltinHooks = config.HookCatalog{
		Hooks: []config.HookDefinition{
			{ID: "ensure-branch", Triggers: []config.HookTrigger{config.HookTriggerSessionStarted}, Execute: []string{"echo ok"}, Timeout: time.Second},
		},
	}
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, cfg)
	h := &fakeHooks{
		result: corehooks.RunResult{
			Passed:       false,
			HooksMatched: 1,
			Reason:       "exit=1",
			PerHookResults: []corehooks.HookResult{
				{HookID: "ensure-branch", Passed: false, Reason: "exit=1"},
			},
		},
	}
	eng.Hooks = h
	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "demo"})

	h.result = corehooks.RunResult{
		Passed:       true,
		HooksMatched: 1,
		PerHookResults: []corehooks.HookResult{
			{HookID: "ensure-branch", Passed: true},
		},
	}
	eng.RunCommand(command.Command{Kind: command.KindHooksRun})

	s := store.ActiveSession()
	if s == nil {
		t.Fatalf("expected active session")
	}
	if s.HooksBlocked {
		t.Fatalf("expected hooks to be unblocked")
	}
	if eng.StatusText != "Hooks passed" {
		t.Fatalf("expected hooks passed status, got %q", eng.StatusText)
	}
}

func TestSessionUseDoesNotAutoRunHooks(t *testing.T) {
	store := session.NewStore()
	cfg := config.Default()
	cfg.BuiltinHooks = config.HookCatalog{
		Hooks: []config.HookDefinition{
			{ID: "ensure-branch", Triggers: []config.HookTrigger{config.HookTriggerSessionStarted}, Execute: []string{"echo ok"}, Timeout: time.Second},
		},
	}
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, cfg)
	h := &fakeHooks{
		result: corehooks.RunResult{Passed: true},
	}
	eng.Hooks = h

	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "one"})
	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "two"})
	callsAfterNew := h.calls
	eng.RunCommand(command.Command{Kind: command.KindSessionUse, SessionID: "one"})

	if h.calls != callsAfterNew {
		t.Fatalf("expected /session use to not run hooks, before=%d after=%d", callsAfterNew, h.calls)
	}
}

func TestSessionAddRepoRunsRepoAddedHooks(t *testing.T) {
	store := session.NewStore()
	cfg := config.Default()
	cfg.BuiltinHooks = config.HookCatalog{
		Hooks: []config.HookDefinition{
			{ID: "sync-main-or-master-on-repo-add", Triggers: []config.HookTrigger{config.HookTriggerRepoAdded}, Execute: []string{"echo ok"}, Timeout: time.Second},
		},
	}
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, cfg)
	h := &fakeHooks{
		result: corehooks.RunResult{Passed: true},
	}
	eng.Hooks = h
	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "demo"})
	callsAfterSessionNew := h.calls

	eng.RunCommand(command.Command{Kind: command.KindSessionAddRepo, RepoPath: t.TempDir()})

	if h.calls != callsAfterSessionNew+1 {
		t.Fatalf("expected one additional hook run on add-repo, before=%d after=%d", callsAfterSessionNew, h.calls)
	}
	if h.lastTrigger != config.HookTriggerRepoAdded {
		t.Fatalf("expected repo.added trigger, got %q", h.lastTrigger)
	}
	if strings.TrimSpace(h.lastRepoPath) == "" {
		t.Fatalf("expected repo path to be passed to hook runner")
	}
	msgs := store.ActiveSession().Messages
	if len(msgs) == 0 {
		t.Fatalf("expected hook progress message")
	}
	last := msgs[len(msgs)-1]
	if last.Role != "system" {
		t.Fatalf("expected hook progress to render as pilot/system message, got role=%q", last.Role)
	}
}
