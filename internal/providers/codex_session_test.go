package providers

import "testing"

func TestParseCodexJSONLine(t *testing.T) {
	t.Parallel()

	ev, raw, ok := parseCodexJSONLine([]byte(`{"type":"thread.started","thread_id":"thread-1"}`))
	if !ok {
		t.Fatalf("expected parse success")
	}
	if ev.Type != "thread.started" || ev.ThreadID != "thread-1" {
		t.Fatalf("unexpected event: %#v", ev)
	}
	if raw == nil {
		t.Fatalf("expected raw map to be present")
	}
}

func TestParseCodexJSONLineIgnoresNonJSON(t *testing.T) {
	t.Parallel()

	if _, _, ok := parseCodexJSONLine([]byte("OpenAI Codex v0.x")); ok {
		t.Fatalf("expected non-JSON line to be ignored")
	}
}

func TestSummarizeCodexFailurePriority(t *testing.T) {
	t.Parallel()

	if got := summarizeCodexFailure("turn failed", "last error", "stderr error"); got != "turn failed" {
		t.Fatalf("expected turn.failed to win, got %q", got)
	}
	if got := summarizeCodexFailure("", "last error", "stderr error"); got != "last error" {
		t.Fatalf("expected fallback to last error, got %q", got)
	}
	if got := summarizeCodexFailure("", "", "stderr error"); got != "stderr error" {
		t.Fatalf("expected fallback to stderr, got %q", got)
	}
}

func TestExtractCodexPreviewChunk(t *testing.T) {
	t.Parallel()

	ev := codexJSONEvent{Type: "response.output_text.delta", Delta: "hel"}
	if got := extractCodexPreviewChunk(ev, nil); got != "hel" {
		t.Fatalf("expected chunk from delta, got %q", got)
	}

	raw := map[string]any{
		"type": "response.output_text.delta",
		"data": map[string]any{"text": "lo"},
	}
	if got := extractCodexPreviewChunk(codexJSONEvent{Type: "response.output_text.delta"}, raw); got != "lo" {
		t.Fatalf("expected nested text chunk, got %q", got)
	}

	ev = codexJSONEvent{Type: "response.output_text.partial", Text: " there"}
	if got := extractCodexPreviewChunk(ev, nil); got != " there" {
		t.Fatalf("expected chunk from non-delta preview event, got %q", got)
	}

	raw = map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"type": "agent_message",
			"text": "final message",
		},
	}
	if got := extractCodexPreviewChunk(codexJSONEvent{Type: "item.completed"}, raw); got != "final message\n\n" {
		t.Fatalf("expected chunk from item.completed agent_message, got %q", got)
	}
}

func TestIsPreviewEventType(t *testing.T) {
	t.Parallel()

	if !isPreviewEventType("response.output_text.delta") {
		t.Fatalf("expected delta preview type")
	}
	if !isPreviewEventType("response.message.partial") {
		t.Fatalf("expected message preview type")
	}
	if isPreviewEventType("turn.failed") {
		t.Fatalf("did not expect failed type to be preview")
	}
	if isPreviewEventType("thread.started") {
		t.Fatalf("did not expect thread.started to be preview")
	}
}

func TestExtractCompletedAgentMessage(t *testing.T) {
	t.Parallel()

	raw := map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"type": "agent_message",
			"text": "  done  ",
		},
	}
	if got := extractCompletedAgentMessage(raw); got != "done" {
		t.Fatalf("expected trimmed message, got %q", got)
	}
}
