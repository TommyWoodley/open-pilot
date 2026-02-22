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
	sends []fakeSendPromptCall
	err   error
}

type fakeSendPromptCall struct {
	providerID string
	sessionID  string
	repoPath   string
	requestID  string
	prompt     string
}

type fakeHooks struct {
	result       corehooks.RunResult
	calls        int
	lastTrigger  config.HookTrigger
	lastRepoPath string
}

type fakeAutoReviewRunner struct {
	base         string
	ref          string
	baseErr      error
	results      []autoReviewResult
	reviewErr    error
	resolveCalls int
	reviewCalls  int
}

func (f *fakeHooks) Run(_ context.Context, trigger config.HookTrigger, _ string, _ string, repoPath string, _ func(corehooks.ProgressUpdate)) corehooks.RunResult {
	f.calls++
	f.lastTrigger = trigger
	f.lastRepoPath = repoPath
	return f.result
}

func (f *fakeAutoReviewRunner) ResolveBase(string) (string, string, error) {
	f.resolveCalls++
	if f.baseErr != nil {
		return "", "", f.baseErr
	}
	return f.base, f.ref, nil
}

func (f *fakeAutoReviewRunner) Review(string, string) (autoReviewResult, error) {
	f.reviewCalls++
	if f.reviewErr != nil {
		return autoReviewResult{}, f.reviewErr
	}
	if len(f.results) == 0 {
		return autoReviewResult{Approved: true, Summary: "approved"}, nil
	}
	out := f.results[0]
	f.results = f.results[1:]
	return out, nil
}

func (f *fakeManager) SendPrompt(_ context.Context, providerID, sessionID, repoPath, requestID, prompt string) error {
	f.sends = append(f.sends, fakeSendPromptCall{
		providerID: providerID,
		sessionID:  sessionID,
		repoPath:   repoPath,
		requestID:  requestID,
		prompt:     prompt,
	})
	return f.err
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

func TestHandleProviderAgentMessageSameItemIDDifferentRequestDoesNotOverwrite(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventAgentMessage,
		SessionID: s.ID,
		RequestID: "req-1",
		ItemID:    "item_0",
		Text:      "first turn",
	})
	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventAgentMessage,
		SessionID: s.ID,
		RequestID: "req-2",
		ItemID:    "item_0",
		Text:      "second turn",
	})

	msgs := store.ActiveSession().Messages
	if len(msgs) != 2 {
		t.Fatalf("expected two distinct messages, got %d", len(msgs))
	}
	if msgs[0].Content != "first turn" || msgs[1].Content != "second turn" {
		t.Fatalf("unexpected contents: %#v", msgs)
	}
}

func TestHandleProviderAgentMessageDevelopmentWorkCompleteRunsHooksForEachTag(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	cfg := config.Default()
	cfg.BuiltinHooks = config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "dev-work-complete",
				Triggers: []config.HookTrigger{config.HookTriggerDevelopmentWorkComplete},
				Execute:  []string{"echo ok"},
				Timeout:  time.Second,
			},
		},
	}
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, cfg)
	h := &fakeHooks{result: corehooks.RunResult{Passed: true}}
	eng.Hooks = h

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventAgentMessage,
		SessionID: s.ID,
		Text:      "done [DEVELOPMENT_WORK_COMPLETE] and again [<DEVELOPMENT_WORK_COMPLETE>]",
	})

	if h.calls != 2 {
		t.Fatalf("expected two hook runs for two tags, got %d", h.calls)
	}
	if h.lastTrigger != config.HookTriggerDevelopmentWorkComplete {
		t.Fatalf("expected development.work.complete trigger, got %q", h.lastTrigger)
	}
}

func TestHandleProviderFinalDevelopmentWorkCompleteRunsHook(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	idx := store.AppendAssistantStreaming("codex", "repo-1")
	cfg := config.Default()
	cfg.BuiltinHooks = config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "dev-work-complete",
				Triggers: []config.HookTrigger{config.HookTriggerDevelopmentWorkComplete},
				Execute:  []string{"echo ok"},
				Timeout:  time.Second,
			},
		},
	}
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, cfg)
	h := &fakeHooks{result: corehooks.RunResult{Passed: true}}
	eng.Hooks = h
	eng.pending["req-1"] = pendingRef{SessionID: s.ID, Index: idx}

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventFinal,
		SessionID: s.ID,
		RequestID: "req-1",
		Text:      "all done <DEVELOPMENT_WORK_COMPLETE>",
	})

	if h.calls != 1 {
		t.Fatalf("expected one hook run for final tag, got %d", h.calls)
	}
	if h.lastTrigger != config.HookTriggerDevelopmentWorkComplete {
		t.Fatalf("expected development.work.complete trigger, got %q", h.lastTrigger)
	}
}

func TestAutoReviewStartsOnDevelopmentWorkCompleteTag(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	s.ProviderID = "codex"
	if err := store.AddRepoToActiveSession(t.TempDir(), "repo"); err != nil {
		t.Fatalf("add repo: %v", err)
	}
	mgr := &fakeManager{events: make(chan providers.Event)}
	eng := NewEngine(store, mgr, config.Default())
	eng.autoReviewRunner = &fakeAutoReviewRunner{
		base: "abc123",
		ref:  "origin/main",
		results: []autoReviewResult{
			{Approved: true, Summary: "approved"},
		},
	}

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventFinal,
		SessionID: s.ID,
		Text:      "<DEVELOPMENT_WORK_COMPLETE>",
	})

	msgs := store.ActiveSession().Messages
	joined := ""
	for _, m := range msgs {
		joined += m.Content + "\n"
	}
	if !strings.Contains(joined, "[[pilot-divider:Automatic Review]]") {
		t.Fatalf("expected automatic review divider, got %q", joined)
	}
	if !strings.Contains(joined, "Cycle: 1/5") || !strings.Contains(joined, "State: Review approved") {
		t.Fatalf("expected cycle/approved state headings, got %q", joined)
	}
}

func TestAutoReviewCommentsPathResumesAgentAndWaitsForNextCompletion(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	s.ProviderID = "codex"
	repoPath := t.TempDir()
	if err := store.AddRepoToActiveSession(repoPath, "repo"); err != nil {
		t.Fatalf("add repo: %v", err)
	}
	mgr := &fakeManager{events: make(chan providers.Event)}
	eng := NewEngine(store, mgr, config.Default())
	eng.autoReviewRunner = &fakeAutoReviewRunner{
		base: "abc123",
		ref:  "origin/main",
		results: []autoReviewResult{
			{Approved: false, Summary: "- fix x"},
			{Approved: true, Summary: "approved"},
		},
	}

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventFinal,
		SessionID: s.ID,
		Text:      "<DEVELOPMENT_WORK_COMPLETE>",
	})
	if len(mgr.sends) != 1 {
		t.Fatalf("expected one remediation prompt send, got %d", len(mgr.sends))
	}
	if !strings.Contains(mgr.sends[0].prompt, "- fix x") {
		t.Fatalf("expected remediation prompt to contain review summary, got %q", mgr.sends[0].prompt)
	}

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventFinal,
		SessionID: s.ID,
		Text:      "<DEVELOPMENT_WORK_COMPLETE>",
	})
	msgs := store.ActiveSession().Messages
	joined := ""
	for _, m := range msgs {
		joined += m.Content + "\n"
	}
	if !strings.Contains(joined, "Cycle: 2/5") || !strings.Contains(joined, "State: Review approved") {
		t.Fatalf("expected second cycle approval, got %q", joined)
	}
}

func TestAutoReviewStopsAtFiveCyclesWithSystemMessage(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	s.ProviderID = "codex"
	repoPath := t.TempDir()
	if err := store.AddRepoToActiveSession(repoPath, "repo"); err != nil {
		t.Fatalf("add repo: %v", err)
	}
	mgr := &fakeManager{events: make(chan providers.Event)}
	eng := NewEngine(store, mgr, config.Default())
	eng.autoReviewRunner = &fakeAutoReviewRunner{
		base: "abc123",
		ref:  "origin/main",
		results: []autoReviewResult{
			{Approved: false, Summary: "needs changes"},
			{Approved: false, Summary: "needs changes"},
			{Approved: false, Summary: "needs changes"},
			{Approved: false, Summary: "needs changes"},
			{Approved: false, Summary: "needs changes"},
		},
	}

	for i := 0; i < 5; i++ {
		eng.HandleProviderEvent(providers.Event{
			Type:      providers.EventFinal,
			SessionID: s.ID,
			Text:      "<DEVELOPMENT_WORK_COMPLETE>",
		})
	}

	if len(mgr.sends) != 4 {
		t.Fatalf("expected remediation prompt on first four cycles, got %d", len(mgr.sends))
	}
	msgs := store.ActiveSession().Messages
	joined := ""
	for _, m := range msgs {
		joined += m.Content + "\n"
	}
	if !strings.Contains(joined, "State: Max cycles reached (5)") {
		t.Fatalf("expected max cycle terminal state, got %q", joined)
	}
}

func TestAutoReviewReviewFailureRendersErrorState(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	s.ProviderID = "codex"
	if err := store.AddRepoToActiveSession(t.TempDir(), "repo"); err != nil {
		t.Fatalf("add repo: %v", err)
	}
	mgr := &fakeManager{events: make(chan providers.Event)}
	eng := NewEngine(store, mgr, config.Default())
	eng.autoReviewRunner = &fakeAutoReviewRunner{
		baseErr: errors.New("no base"),
	}

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventFinal,
		SessionID: s.ID,
		Text:      "<DEVELOPMENT_WORK_COMPLETE>",
	})

	msgs := store.ActiveSession().Messages
	joined := ""
	for _, m := range msgs {
		joined += m.Content + "\n"
	}
	if !strings.Contains(joined, "State: Error") || !strings.Contains(joined, "no base") {
		t.Fatalf("expected error state with reason, got %q", joined)
	}
}

func TestAutoReviewMultipleTagsInOneMessageStartsSingleCycle(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	s.ProviderID = "codex"
	repoPath := t.TempDir()
	if err := store.AddRepoToActiveSession(repoPath, "repo"); err != nil {
		t.Fatalf("add repo: %v", err)
	}
	mgr := &fakeManager{events: make(chan providers.Event)}
	runner := &fakeAutoReviewRunner{
		base: "abc123",
		ref:  "origin/main",
		results: []autoReviewResult{
			{Approved: false, Summary: "needs changes"},
		},
	}
	eng := NewEngine(store, mgr, config.Default())
	eng.autoReviewRunner = runner

	eng.HandleProviderEvent(providers.Event{
		Type:      providers.EventAgentMessage,
		SessionID: s.ID,
		Text:      "done [DEVELOPMENT_WORK_COMPLETE] and [<DEVELOPMENT_WORK_COMPLETE>]",
	})

	if runner.reviewCalls != 1 {
		t.Fatalf("expected single review cycle start, got %d", runner.reviewCalls)
	}
	if len(mgr.sends) != 1 {
		t.Fatalf("expected single remediation prompt, got %d", len(mgr.sends))
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

func TestHandleProviderCommandExecutionGroupsSequentialExploreCommands(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())
	start := time.Unix(1000, 0)
	eng.nowFn = func() time.Time { return start }

	eng.HandleProviderEvent(providers.Event{
		Type:          providers.EventCommandExecution,
		SessionID:     s.ID,
		RequestID:     "req-1",
		ItemID:        "item-ls",
		Command:       "ls -la",
		CommandStatus: "in_progress",
	})
	eng.nowFn = func() time.Time { return start.Add(200 * time.Millisecond) }
	zero := 0
	eng.HandleProviderEvent(providers.Event{
		Type:            providers.EventCommandExecution,
		SessionID:       s.ID,
		RequestID:       "req-1",
		ItemID:          "item-ls",
		Command:         "ls -la",
		CommandStatus:   "completed",
		CommandExitCode: &zero,
	})
	eng.nowFn = func() time.Time { return start.Add(200 * time.Millisecond) }
	eng.HandleProviderEvent(providers.Event{
		Type:          providers.EventCommandExecution,
		SessionID:     s.ID,
		RequestID:     "req-1",
		ItemID:        "item-rg",
		Command:       "rg --files",
		CommandStatus: "in_progress",
	})
	eng.nowFn = func() time.Time { return start.Add(500 * time.Millisecond) }
	eng.HandleProviderEvent(providers.Event{
		Type:            providers.EventCommandExecution,
		SessionID:       s.ID,
		RequestID:       "req-1",
		ItemID:          "item-rg",
		Command:         "rg --files",
		CommandStatus:   "completed",
		CommandExitCode: &zero,
	})

	msgs := store.ActiveSession().Messages
	if len(msgs) != 1 {
		t.Fatalf("expected one grouped message, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "Explored 2 commands for 500ms") {
		t.Fatalf("expected grouped explore summary, got %q", msgs[0].Content)
	}
}

func TestHandleProviderCommandExecutionStartsNewExploreGroupAfterNonExploreCommand(t *testing.T) {
	store := session.NewStore()
	s := store.CreateSession("demo")
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, config.Default())
	start := time.Unix(1000, 0)
	eng.nowFn = func() time.Time { return start }

	eng.HandleProviderEvent(providers.Event{
		Type:          providers.EventCommandExecution,
		SessionID:     s.ID,
		RequestID:     "req-1",
		ItemID:        "item-find",
		Command:       "find . -maxdepth 1",
		CommandStatus: "in_progress",
	})
	eng.nowFn = func() time.Time { return start.Add(100 * time.Millisecond) }
	zero := 0
	eng.HandleProviderEvent(providers.Event{
		Type:            providers.EventCommandExecution,
		SessionID:       s.ID,
		RequestID:       "req-1",
		ItemID:          "item-find",
		Command:         "find . -maxdepth 1",
		CommandStatus:   "completed",
		CommandExitCode: &zero,
	})

	eng.nowFn = func() time.Time { return start.Add(120 * time.Millisecond) }
	eng.HandleProviderEvent(providers.Event{
		Type:          providers.EventCommandExecution,
		SessionID:     s.ID,
		RequestID:     "req-1",
		ItemID:        "item-test",
		Command:       "go test ./...",
		CommandStatus: "in_progress",
	})
	eng.nowFn = func() time.Time { return start.Add(240 * time.Millisecond) }
	eng.HandleProviderEvent(providers.Event{
		Type:            providers.EventCommandExecution,
		SessionID:       s.ID,
		RequestID:       "req-1",
		ItemID:          "item-test",
		Command:         "go test ./...",
		CommandStatus:   "completed",
		CommandExitCode: &zero,
	})

	eng.nowFn = func() time.Time { return start.Add(240 * time.Millisecond) }
	eng.HandleProviderEvent(providers.Event{
		Type:          providers.EventCommandExecution,
		SessionID:     s.ID,
		RequestID:     "req-1",
		ItemID:        "item-rg",
		Command:       "rg --files",
		CommandStatus: "in_progress",
	})
	eng.nowFn = func() time.Time { return start.Add(440 * time.Millisecond) }
	eng.HandleProviderEvent(providers.Event{
		Type:            providers.EventCommandExecution,
		SessionID:       s.ID,
		RequestID:       "req-1",
		ItemID:          "item-rg",
		Command:         "rg --files",
		CommandStatus:   "completed",
		CommandExitCode: &zero,
	})

	msgs := store.ActiveSession().Messages
	if len(msgs) != 3 {
		t.Fatalf("expected separate summaries around non-explore command, got %d", len(msgs))
	}
	if !strings.Contains(msgs[0].Content, "Explored for 100ms") {
		t.Fatalf("expected first explore summary, got %q", msgs[0].Content)
	}
	if !strings.Contains(msgs[1].Content, "Ran `go test ./...` for 120ms") {
		t.Fatalf("expected non-explore summary, got %q", msgs[1].Content)
	}
	if !strings.Contains(msgs[2].Content, "Explored for 200ms") {
		t.Fatalf("expected second explore summary to be separate group, got %q", msgs[2].Content)
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

func TestSessionUseRunsRepoSelectedHooksWhenActiveRepoExists(t *testing.T) {
	store := session.NewStore()
	cfg := config.Default()
	cfg.BuiltinHooks = config.HookCatalog{
		Hooks: []config.HookDefinition{
			{ID: "open-development-branch", Triggers: []config.HookTrigger{config.HookTriggerRepoSelected}, Execute: []string{"echo ok"}, Timeout: time.Second},
		},
	}
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, cfg)
	h := &fakeHooks{
		result: corehooks.RunResult{Passed: true},
	}
	eng.Hooks = h

	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "one"})
	eng.RunCommand(command.Command{Kind: command.KindSessionAddRepo, RepoPath: t.TempDir()})
	s1 := store.ActiveSession()
	if s1 == nil || s1.ActiveRepoID == "" {
		t.Fatalf("expected first session with active repo")
	}
	repoPath := store.ActiveRepo().Path
	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "two"})
	callsAfterSetup := h.calls

	eng.RunCommand(command.Command{Kind: command.KindSessionUse, SessionID: "one"})

	if h.calls != callsAfterSetup+1 {
		t.Fatalf("expected /session use to run repo.selected hooks once, before=%d after=%d", callsAfterSetup, h.calls)
	}
	if h.lastTrigger != config.HookTriggerRepoSelected {
		t.Fatalf("expected repo.selected trigger, got %q", h.lastTrigger)
	}
	if h.lastRepoPath != repoPath {
		t.Fatalf("expected repo path %q, got %q", repoPath, h.lastRepoPath)
	}
}

func TestSessionAddRepoRunsRepoSelectedHooks(t *testing.T) {
	store := session.NewStore()
	cfg := config.Default()
	cfg.BuiltinHooks = config.HookCatalog{
		Hooks: []config.HookDefinition{
			{ID: "open-development-branch", Triggers: []config.HookTrigger{config.HookTriggerRepoSelected}, Execute: []string{"echo ok"}, Timeout: time.Second},
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
	if h.lastTrigger != config.HookTriggerRepoSelected {
		t.Fatalf("expected repo.selected trigger, got %q", h.lastTrigger)
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

func TestSessionRepoUseRunsRepoSelectedHooks(t *testing.T) {
	store := session.NewStore()
	cfg := config.Default()
	cfg.BuiltinHooks = config.HookCatalog{
		Hooks: []config.HookDefinition{
			{ID: "open-development-branch", Triggers: []config.HookTrigger{config.HookTriggerRepoSelected}, Execute: []string{"echo ok"}, Timeout: time.Second},
		},
	}
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, cfg)
	h := &fakeHooks{
		result: corehooks.RunResult{Passed: true},
	}
	eng.Hooks = h

	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "demo"})
	eng.RunCommand(command.Command{Kind: command.KindSessionAddRepo, RepoPath: t.TempDir()})
	eng.RunCommand(command.Command{Kind: command.KindSessionAddRepo, RepoPath: t.TempDir()})

	s := store.ActiveSession()
	if s == nil || len(s.Repos) < 2 {
		t.Fatalf("expected session with at least two repos")
	}
	repoID := s.Repos[0].ID
	repoPath := s.Repos[0].Path
	callsBeforeRepoUse := h.calls

	eng.RunCommand(command.Command{Kind: command.KindSessionRepoUse, RepoID: repoID})

	if h.calls != callsBeforeRepoUse+1 {
		t.Fatalf("expected one additional hook run on /session repo use, before=%d after=%d", callsBeforeRepoUse, h.calls)
	}
	if h.lastTrigger != config.HookTriggerRepoSelected {
		t.Fatalf("expected repo.selected trigger, got %q", h.lastTrigger)
	}
	if h.lastRepoPath != repoPath {
		t.Fatalf("expected repo path %q, got %q", repoPath, h.lastRepoPath)
	}
}

func TestProviderUseCodexRunsProviderCodexSelectedHooks(t *testing.T) {
	store := session.NewStore()
	cfg := config.Default()
	cfg.BuiltinHooks = config.HookCatalog{
		Hooks: []config.HookDefinition{
			{ID: "install-codex-skills", Triggers: []config.HookTrigger{config.HookTriggerProviderCodexSelected}, Execute: []string{"echo ok"}, Timeout: time.Second},
		},
	}
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, cfg)
	h := &fakeHooks{result: corehooks.RunResult{Passed: true}}
	eng.Hooks = h
	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "demo"})
	callsAfterSessionNew := h.calls

	eng.RunCommand(command.Command{Kind: command.KindProviderUse, ProviderID: "codex"})
	if h.calls != callsAfterSessionNew+1 {
		t.Fatalf("expected one additional hook run on provider use codex, before=%d after=%d", callsAfterSessionNew, h.calls)
	}
	if h.lastTrigger != config.HookTriggerProviderCodexSelected {
		t.Fatalf("expected provider.codex.selected trigger, got %q", h.lastTrigger)
	}
}

func TestSessionNewRunsProviderCodexSelectedHooksWhenCodexDefault(t *testing.T) {
	store := session.NewStore()
	cfg := config.Default()
	cfg.BuiltinHooks = config.HookCatalog{
		Hooks: []config.HookDefinition{
			{ID: "session-started", Triggers: []config.HookTrigger{config.HookTriggerSessionStarted}, Execute: []string{"echo ok"}, Timeout: time.Second},
			{ID: "install-codex-skills", Triggers: []config.HookTrigger{config.HookTriggerProviderCodexSelected}, Execute: []string{"echo ok"}, Timeout: time.Second},
		},
	}
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, cfg)
	h := &fakeHooks{result: corehooks.RunResult{Passed: true}}
	eng.Hooks = h

	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "demo"})
	if h.calls != 2 {
		t.Fatalf("expected two hook runs on session new with codex default (session.started + provider.codex.selected), got %d", h.calls)
	}
}

func TestProviderCodexSelectedHookFailureBlocksPrompts(t *testing.T) {
	store := session.NewStore()
	cfg := config.Default()
	cfg.BuiltinHooks = config.HookCatalog{
		Hooks: []config.HookDefinition{
			{ID: "install-codex-skills", Triggers: []config.HookTrigger{config.HookTriggerProviderCodexSelected}, Execute: []string{"echo ok"}, Timeout: time.Second},
		},
	}
	eng := NewEngine(store, &fakeManager{events: make(chan providers.Event)}, cfg)
	h := &fakeHooks{
		result: corehooks.RunResult{
			Passed:             false,
			HooksMatched:       1,
			FailedHookID:       "install-codex-skills",
			FailedCommandIndex: 1,
			Reason:             "exit=1",
			PerHookResults: []corehooks.HookResult{
				{HookID: "install-codex-skills", Passed: false, Reason: "exit=1"},
			},
		},
	}
	eng.Hooks = h
	eng.RunCommand(command.Command{Kind: command.KindSessionNew, Session: "demo"})
	eng.RunCommand(command.Command{Kind: command.KindProviderUse, ProviderID: "codex"})

	s := store.ActiveSession()
	if s == nil || !s.HooksBlocked {
		t.Fatalf("expected hooks to block session on provider.codex.selected failure")
	}
	eng.SendPrompt("hello")
	if !strings.Contains(eng.StatusText, "Hooks blocked") {
		t.Fatalf("expected hooks blocked status, got %q", eng.StatusText)
	}
}
