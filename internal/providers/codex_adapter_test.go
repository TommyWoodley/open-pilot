package providers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/thwoodle/open-pilot/internal/config"
)

func TestManagerUsesBuiltInCodexAdapter(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Providers["codex"] = config.ProviderConfig{
		ID:      "codex",
		Command: "open-pilot-codex-wrapper",
	}

	svc := NewManager(cfg).(*service)
	adapter, _, err := svc.adapterForLocked("codex")
	if err != nil {
		t.Fatalf("adapterForLocked returned error: %v", err)
	}

	codexAdapter, ok := adapter.(*codexCLIAdapter)
	if !ok {
		t.Fatalf("expected *codexCLIAdapter, got %T", adapter)
	}
	if codexAdapter.binary != "codex" {
		t.Fatalf("expected codex binary fallback, got %q", codexAdapter.binary)
	}
}

func TestCodexAdapterFirstPromptStoresThreadID(t *testing.T) {
	env := setupFakeCodex(t, "success", "thread-first", "hello from assistant")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-1", SessionID: "sess-1", RepoPath: env.repoDir, Text: "hello"}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	ev := waitEventType(t, events, EventFinal)
	if ev.Text != "hello from assistant" {
		t.Fatalf("expected clean assistant text, got %q", ev.Text)
	}

	adapter.mu.Lock()
	h := adapter.handles[handle]
	adapter.mu.Unlock()
	if h == nil {
		t.Fatalf("expected active handle")
	}
	h.mu.Lock()
	got := h.codexID
	h.mu.Unlock()
	if got != "thread-first" {
		t.Fatalf("expected thread ID thread-first, got %q", got)
	}
}

func TestCodexAdapterEmitsThreadIDInStatusEvent(t *testing.T) {
	env := setupFakeCodex(t, "success", "thread-event", "hello from assistant")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-1", SessionID: "sess-1", RepoPath: env.repoDir, Text: "hello"}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	var status Event
	for {
		ev := waitEventType(t, events, EventStatus)
		if ev.ProviderThreadID != "" {
			status = ev
			break
		}
	}
	if status.ProviderThreadID != "thread-event" {
		t.Fatalf("expected provider thread id thread-event, got %q", status.ProviderThreadID)
	}
}

func TestCodexAdapterSubsequentPromptUsesResume(t *testing.T) {
	env := setupFakeCodex(t, "success", "thread-resume", "ok")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-1", SessionID: "sess-1", RepoPath: env.repoDir, Text: "first prompt"}); err != nil {
		t.Fatalf("send #1 failed: %v", err)
	}
	_ = waitEventType(t, events, EventFinal)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-2", SessionID: "sess-1", RepoPath: env.repoDir, Text: "second prompt"}); err != nil {
		t.Fatalf("send #2 failed: %v", err)
	}
	_ = waitEventType(t, events, EventFinal)

	raw, err := os.ReadFile(env.argsFile)
	if err != nil {
		t.Fatalf("read args file failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 invocations, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "exec --json") || !strings.Contains(lines[0], "--output-last-message") {
		t.Fatalf("expected first call to be plain exec with output-last-message, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "--skip-git-repo-check") {
		t.Fatalf("expected first call to include skip-git-repo-check, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "--sandbox workspace-write") {
		t.Fatalf("expected first call to force workspace-write sandbox, got %q", lines[0])
	}
	if !strings.Contains(lines[0], "--add-dir") {
		t.Fatalf("expected first call to include add-dir writable roots, got %q", lines[0])
	}
	if !strings.Contains(lines[1], "exec ") || !strings.Contains(lines[1], " resume ") {
		t.Fatalf("expected second call to use resume, got %q", lines[1])
	}
	if !strings.Contains(lines[1], "--skip-git-repo-check") {
		t.Fatalf("expected second call to include skip-git-repo-check, got %q", lines[1])
	}
	if !strings.Contains(lines[1], "--sandbox workspace-write") {
		t.Fatalf("expected second call to force workspace-write sandbox, got %q", lines[1])
	}
	if !strings.Contains(lines[1], "--add-dir") {
		t.Fatalf("expected second call to include add-dir writable roots, got %q", lines[1])
	}
	if strings.Contains(lines[1], "--output-last-message") {
		t.Fatalf("expected resume call to omit output-last-message, got %q", lines[1])
	}
	if strings.Contains(lines[1], " -- thread-resume second prompt") {
		t.Fatalf("expected resume call to pass session/prompt positionally without -- separator, got %q", lines[1])
	}
	if !strings.Contains(lines[1], "thread-resume second prompt") {
		t.Fatalf("expected resume call to include thread id and prompt, got %q", lines[1])
	}
}

func TestCodexAdapterStartWithThreadIDUsesResumeOnFirstPrompt(t *testing.T) {
	env := setupFakeCodex(t, "success", "thread-start", "ok")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, err := adapter.Start(context.Background(), StartRequest{
		SessionID:        "sess-1",
		Provider:         "codex",
		RepoPath:         env.repoDir,
		ProviderThreadID: "thread-start",
	})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	events := adapter.Events(handle)
	_ = waitEventType(t, events, EventReady)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-1", SessionID: "sess-1", RepoPath: env.repoDir, Text: "hello"}); err != nil {
		t.Fatalf("send failed: %v", err)
	}
	_ = waitEventType(t, events, EventFinal)

	raw, err := os.ReadFile(env.argsFile)
	if err != nil {
		t.Fatalf("read args file failed: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
	if len(lines) < 1 {
		t.Fatalf("expected at least 1 invocation, got %d", len(lines))
	}
	if !strings.Contains(lines[0], " resume ") || !strings.Contains(lines[0], "thread-start") {
		t.Fatalf("expected first call to use resume with provided thread id, got %q", lines[0])
	}
}

func TestCodexAdapterSubsequentPromptUsesResumeWhenSecondResponseIsDelayed(t *testing.T) {
	env := setupFakeCodex(t, "delayed_success", "thread-resume-delayed", "ok")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-1", SessionID: "sess-1", RepoPath: env.repoDir, Text: "first prompt"}); err != nil {
		t.Fatalf("send #1 failed: %v", err)
	}
	_ = waitEventType(t, events, EventFinal)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-2", SessionID: "sess-1", RepoPath: env.repoDir, Text: "second prompt"}); err != nil {
		t.Fatalf("send #2 failed: %v", err)
	}
	_ = waitEventType(t, events, EventFinal)
}

func TestCodexAdapterFailureEmitsSingleConciseError(t *testing.T) {
	env := setupFakeCodex(t, "fail", "", "")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-1", SessionID: "sess-1", RepoPath: env.repoDir, Text: "hello"}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	ev := waitEventType(t, events, EventError)
	if ev.Message != "network down" {
		t.Fatalf("expected concise turn.failed message, got %q", ev.Message)
	}
	if strings.Contains(strings.ToLower(ev.Message), "reconnecting") {
		t.Fatalf("unexpected reconnect noise in error: %q", ev.Message)
	}
}

func TestCodexAdapterNoFinalMessageEmitsError(t *testing.T) {
	env := setupFakeCodex(t, "empty", "thread-empty", "")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-1", SessionID: "sess-1", RepoPath: env.repoDir, Text: "hello"}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	ev := waitEventType(t, events, EventError)
	if ev.Message != "codex returned no assistant message" {
		t.Fatalf("unexpected error message: %q", ev.Message)
	}
}

func TestCodexAdapterAgentMessageWithoutOutputFileDoesNotEmitError(t *testing.T) {
	env := setupFakeCodex(t, "agent_message_only", "thread-agent", "")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-1", SessionID: "sess-1", RepoPath: env.repoDir, Text: "hello"}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	ev := waitEventType(t, events, EventAgentMessage)
	if strings.TrimSpace(ev.Text) == "" {
		t.Fatalf("expected agent message text")
	}
	assertNoEventTypeWithin(t, events, EventError, 300*time.Millisecond)
}

func TestCodexAdapterStreamsPreviewChunks(t *testing.T) {
	env := setupFakeCodex(t, "stream", "thread-stream", "hello world")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-1", SessionID: "sess-1", RepoPath: env.repoDir, Text: "hello"}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	chunk1 := waitEventType(t, events, EventChunk)
	chunk2 := waitEventType(t, events, EventChunk)
	final := waitEventType(t, events, EventFinal)

	if chunk1.Text+chunk2.Text != "hello world" {
		t.Fatalf("expected preview chunks to build full text, got %q + %q", chunk1.Text, chunk2.Text)
	}
	if final.Text != "hello world" {
		t.Fatalf("expected final clean text, got %q", final.Text)
	}
}

func TestCodexAdapterFailureFallsBackToStderrMessage(t *testing.T) {
	env := setupFakeCodex(t, "fail_stderr", "", "")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-1", SessionID: "sess-1", RepoPath: env.repoDir, Text: "hello"}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	ev := waitEventType(t, events, EventError)
	if ev.Message != "authentication required; run codex login" {
		t.Fatalf("expected stderr fallback message, got %q", ev.Message)
	}
}

func TestCodexAdapterEmitsUnknownEventsAndLogsPayload(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "provider-debug.log")
	t.Setenv("OPEN_PILOT_PROVIDER_DEBUG_LOG", logPath)
	env := setupFakeCodex(t, "unknown_raw_event", "thread-unknown", "hello")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-unknown", SessionID: "sess-1", RepoPath: env.repoDir, Text: "hello"}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	unknown := waitEventType(t, events, EventUnknown)
	if unknown.RawType != "tool.preview" {
		t.Fatalf("expected unknown raw type tool.preview, got %q", unknown.RawType)
	}
	if !strings.Contains(unknown.RawJSON, `"type":"tool.preview"`) {
		t.Fatalf("expected raw json payload, got %q", unknown.RawJSON)
	}
	_ = waitEventType(t, events, EventFinal)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read provider debug log failed: %v", err)
	}
	if !strings.Contains(string(data), `"raw_type":"tool.preview"`) {
		t.Fatalf("expected unknown event diagnostic in log, got %q", string(data))
	}
}

func TestCodexAdapterIgnoresToolCallItemCompleted(t *testing.T) {
	env := setupFakeCodex(t, "unknown_event", "thread-unknown", "hello")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-tool-call", SessionID: "sess-1", RepoPath: env.repoDir, Text: "hello"}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	timer := time.NewTimer(providerEventWaitTimeout)
	defer timer.Stop()
	sawToolCall := false
	for {
		select {
		case <-timer.C:
			t.Fatalf("timed out waiting for final event")
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("event channel closed before final")
			}
			if ev.Type == EventUnknown {
				t.Fatalf("expected tool_call item.completed to be handled, got unknown event: %#v", ev)
			}
			if ev.Type == EventToolCall {
				sawToolCall = true
				if ev.Text != "patched files" {
					t.Fatalf("expected tool_call text, got %#v", ev)
				}
			}
			if ev.Type == EventFinal {
				if !sawToolCall {
					t.Fatalf("expected tool_call event before final")
				}
				return
			}
			if ev.Type == EventError {
				t.Fatalf("unexpected provider error: %#v", ev)
			}
		}
	}
}

func TestCodexAdapterEmitsReasoningAndCommandLifecycleEvents(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "provider-debug.log")
	t.Setenv("OPEN_PILOT_PROVIDER_DEBUG_LOG", logPath)
	env := setupFakeCodex(t, "lifecycle", "thread-life", "done")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-life", SessionID: "sess-1", RepoPath: env.repoDir, Text: "hello"}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	reasoning := waitEventType(t, events, EventReasoning)
	if !strings.Contains(reasoning.Text, "Planning") {
		t.Fatalf("expected reasoning text, got %#v", reasoning)
	}

	cmdStart := waitEventType(t, events, EventCommandExecution)
	if cmdStart.CommandStatus != "in_progress" || cmdStart.Command != "go test ./..." {
		t.Fatalf("expected command start event, got %#v", cmdStart)
	}

	cmdDone := waitEventType(t, events, EventCommandExecution)
	if cmdDone.CommandStatus != "completed" || cmdDone.CommandExitCode == nil || *cmdDone.CommandExitCode != 0 {
		t.Fatalf("expected command completion event, got %#v", cmdDone)
	}

	usage := waitEventType(t, events, EventTurnUsage)
	if usage.UsageInputTokens != 10 || usage.UsageCachedInputTokens != 2 || usage.UsageOutputTokens != 3 {
		t.Fatalf("expected usage event, got %#v", usage)
	}

	_ = waitEventType(t, events, EventFinal)
}

func TestCodexAdapterHandlesLargeStdoutJSONLine(t *testing.T) {
	env := setupFakeCodex(t, "large_json_line", "thread-large", "ok")

	adapter := newCodexCLIAdapter(env.binary).(*codexCLIAdapter)
	handle, events := startHandle(t, adapter, env.repoDir)

	if err := adapter.Send(context.Background(), handle, PromptRequest{ID: "req-large", SessionID: "sess-1", RepoPath: env.repoDir, Text: "hello"}); err != nil {
		t.Fatalf("send failed: %v", err)
	}

	_ = waitEventType(t, events, EventFinal)
	assertNoEventTypeWithin(t, events, EventError, 300*time.Millisecond)
}

type fakeCodexEnv struct {
	binary   string
	repoDir  string
	argsFile string
}

const providerEventWaitTimeout = 10 * time.Second

func setupFakeCodex(t *testing.T, mode, threadID, message string) fakeCodexEnv {
	t.Helper()

	tmp := t.TempDir()
	argsFile := filepath.Join(tmp, "args.log")
	repoDir := filepath.Join(tmp, "repo")
	if err := os.Mkdir(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo failed: %v", err)
	}

	script := filepath.Join(tmp, "fake-codex")
	content := `#!/usr/bin/env bash
set -eu
if [ -n "${OPEN_PILOT_ARGS_FILE:-}" ]; then
  printf '%s\n' "$*" >> "$OPEN_PILOT_ARGS_FILE"
fi
out_file=""
prev=""
for arg in "$@"; do
  if [ "$prev" = "--output-last-message" ]; then
    out_file="$arg"
  fi
  prev="$arg"
done
mode="${OPEN_PILOT_MODE:-success}"
thread_id="${OPEN_PILOT_THREAD_ID:-thread-123}"
last_message="${OPEN_PILOT_LAST_MESSAGE:-hello}"
if [ "$mode" = "success" ]; then
  printf '{"type":"thread.started","thread_id":"%s"}\n' "$thread_id"
  printf '{"type":"response.output_text.delta","delta":"%s"}\n' "$last_message"
  if [ -n "$out_file" ]; then
    printf '%s' "$last_message" > "$out_file"
  fi
  exit 0
fi
if [ "$mode" = "delayed_success" ]; then
  case "$*" in
    *" resume "*) sleep 4 ;;
  esac
  printf '{"type":"thread.started","thread_id":"%s"}\n' "$thread_id"
  printf '{"type":"response.output_text.delta","delta":"%s"}\n' "$last_message"
  if [ -n "$out_file" ]; then
    printf '%s' "$last_message" > "$out_file"
  fi
  exit 0
fi
if [ "$mode" = "fail" ]; then
  printf '{"type":"error","message":"Reconnecting... 1/5"}\n'
  printf '{"type":"turn.failed","error":{"message":"network down"}}\n'
  exit 1
fi
if [ "$mode" = "empty" ]; then
  printf '{"type":"thread.started","thread_id":"%s"}\n' "$thread_id"
  : > "$out_file"
  exit 0
fi
if [ "$mode" = "agent_message_only" ]; then
  printf '{"type":"thread.started","thread_id":"%s"}\n' "$thread_id"
  printf '{"type":"item.completed","item":{"id":"item_0","type":"agent_message","text":"hello from agent"}}\n'
  : > "$out_file"
  exit 0
fi
if [ "$mode" = "stream" ]; then
  printf '{"type":"thread.started","thread_id":"%s"}\n' "$thread_id"
  printf '{"type":"response.output_text.delta","delta":"hello "}\n'
  printf '{"type":"response.output_text.delta","delta":"world"}\n'
  printf '%s' "$last_message" > "$out_file"
  exit 0
fi
if [ "$mode" = "fail_stderr" ]; then
  printf 'authentication required; run codex login\n' >&2
  exit 1
fi
if [ "$mode" = "unknown_event" ]; then
  printf '{"type":"thread.started","thread_id":"%s"}\n' "$thread_id"
  printf '{"type":"item.completed","item":{"type":"tool_call","text":"patched files"}}\n'
  printf '{"type":"response.output_text.delta","delta":"%s"}\n' "$last_message"
  printf '%s' "$last_message" > "$out_file"
  exit 0
fi
if [ "$mode" = "unknown_raw_event" ]; then
  printf '{"type":"thread.started","thread_id":"%s"}\n' "$thread_id"
  printf '{"type":"tool.preview","text":"still unknown"}\n'
  printf '{"type":"response.output_text.delta","delta":"%s"}\n' "$last_message"
  printf '%s' "$last_message" > "$out_file"
  exit 0
fi
if [ "$mode" = "lifecycle" ]; then
  printf '{"type":"thread.started","thread_id":"%s"}\n' "$thread_id"
  printf '{"type":"turn.started"}\n'
  printf '{"type":"item.completed","item":{"id":"item-r","type":"reasoning","text":"**Planning**"}}\n'
  printf '{"type":"item.started","item":{"id":"item-c","type":"command_execution","command":"go test ./...","aggregated_output":"","exit_code":null,"status":"in_progress"}}\n'
  printf '{"type":"item.completed","item":{"id":"item-c","type":"command_execution","command":"go test ./...","aggregated_output":"ok\\n","exit_code":0,"status":"completed"}}\n'
  printf '{"type":"response.output_text.delta","delta":"%s"}\n' "$last_message"
  printf '{"type":"turn.completed","usage":{"input_tokens":10,"cached_input_tokens":2,"output_tokens":3}}\n'
  printf '%s' "$last_message" > "$out_file"
  exit 0
fi
if [ "$mode" = "large_json_line" ]; then
  printf '{"type":"thread.started","thread_id":"%s"}\n' "$thread_id"
  big_payload=$(printf 'x%.0s' $(seq 1 70000))
  printf '{"type":"item.completed","item":{"id":"item-big","type":"command_execution","aggregated_output":"%s"}}\n' "$big_payload"
  printf '{"type":"response.output_text.delta","delta":"%s"}\n' "$last_message"
  printf '%s' "$last_message" > "$out_file"
  exit 0
fi
printf 'unknown mode: %s\n' "$mode" >&2
exit 2
`
	if err := os.WriteFile(script, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake codex script failed: %v", err)
	}

	t.Setenv("OPEN_PILOT_MODE", mode)
	t.Setenv("OPEN_PILOT_THREAD_ID", threadID)
	t.Setenv("OPEN_PILOT_LAST_MESSAGE", message)
	t.Setenv("OPEN_PILOT_ARGS_FILE", argsFile)

	return fakeCodexEnv{binary: script, repoDir: repoDir, argsFile: argsFile}
}

func startHandle(t *testing.T, adapter *codexCLIAdapter, repoDir string) (SessionHandle, <-chan Event) {
	t.Helper()

	handle, err := adapter.Start(context.Background(), StartRequest{SessionID: "sess-1", Provider: "codex", RepoPath: repoDir})
	if err != nil {
		t.Fatalf("start failed: %v", err)
	}
	events := adapter.Events(handle)
	_ = waitEventType(t, events, EventReady)
	return handle, events
}

func waitEventType(t *testing.T, events <-chan Event, eventType string) Event {
	t.Helper()
	timer := time.NewTimer(providerEventWaitTimeout)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			t.Fatalf("timed out waiting for event %q after %s", eventType, providerEventWaitTimeout)
		case ev, ok := <-events:
			if !ok {
				t.Fatalf("event channel closed while waiting for %q", eventType)
			}
			if ev.Type == eventType {
				return ev
			}
			if ev.Type == EventError && eventType != EventError {
				t.Fatalf("received error event while waiting for %q: %s (%v)", eventType, ev.Message, ev.Err)
			}
		}
	}
}

func assertNoEventTypeWithin(t *testing.T, events <-chan Event, eventType string, d time.Duration) {
	t.Helper()
	timer := time.NewTimer(d)
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			return
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Type == eventType {
				t.Fatalf("unexpected event %q: %#v", eventType, ev)
			}
		}
	}
}
