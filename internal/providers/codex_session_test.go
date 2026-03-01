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
	if got := extractCodexPreviewChunk(codexJSONEvent{Type: "item.completed"}, raw); got != "" {
		t.Fatalf("expected item.completed agent_message to be normalized, not chunked, got %q", got)
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

func TestNormalizeCodexEventReasoning(t *testing.T) {
	t.Parallel()

	ev := codexJSONEvent{Type: "item.completed"}
	raw := map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "item-1",
			"type": "reasoning",
			"text": "**Planning project type detection**",
		},
	}
	got, ok := normalizeCodexEvent(ev, raw)
	if !ok {
		t.Fatalf("expected reasoning event normalization")
	}
	if got.Type != EventReasoning || got.ItemType != "reasoning" || got.ItemID != "item-1" {
		t.Fatalf("unexpected normalized event: %#v", got)
	}
}

func TestNormalizeCodexEventCommandExecution(t *testing.T) {
	t.Parallel()

	ev := codexJSONEvent{Type: "item.completed"}
	raw := map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":                "item-2",
			"type":              "command_execution",
			"command":           "go test ./...",
			"aggregated_output": "ok",
			"status":            "completed",
			"exit_code":         float64(0),
		},
	}
	got, ok := normalizeCodexEvent(ev, raw)
	if !ok {
		t.Fatalf("expected command event normalization")
	}
	if got.Type != EventCommandExecution || got.Command != "go test ./..." || got.CommandStatus != "completed" {
		t.Fatalf("unexpected command normalized event: %#v", got)
	}
	if got.CommandExitCode == nil || *got.CommandExitCode != 0 {
		t.Fatalf("expected exit code 0, got %#v", got.CommandExitCode)
	}
}

func TestNormalizeCodexEventAgentMessage(t *testing.T) {
	t.Parallel()

	ev := codexJSONEvent{Type: "item.completed"}
	raw := map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "item-3",
			"type": "agent_message",
			"text": "done",
		},
	}
	got, ok := normalizeCodexEvent(ev, raw)
	if !ok {
		t.Fatalf("expected agent_message normalization")
	}
	if got.Type != EventAgentMessage || got.ItemID != "item-3" || got.Text != "done" {
		t.Fatalf("unexpected agent message normalized event: %#v", got)
	}
}

func TestNormalizeCodexEventUnknownItemSubtypeIsHandledSilently(t *testing.T) {
	t.Parallel()

	ev := codexJSONEvent{Type: "item.completed"}
	raw := map[string]any{
		"type": "item.completed",
		"item": map[string]any{
			"id":   "item-4",
			"type": "tool_call",
			"text": "patched files",
		},
	}
	got, ok := normalizeCodexEvent(ev, raw)
	if !ok {
		t.Fatalf("expected unknown item subtype to be marked handled")
	}
	if got.Type != "" {
		t.Fatalf("expected no normalized event for internal-only item subtype, got %#v", got)
	}
}

func TestNormalizeCodexEventTurnUsage(t *testing.T) {
	t.Parallel()

	ev := codexJSONEvent{Type: "turn.completed"}
	raw := map[string]any{
		"type": "turn.completed",
		"usage": map[string]any{
			"input_tokens":        float64(10),
			"cached_input_tokens": float64(2),
			"output_tokens":       float64(3),
		},
	}
	got, ok := normalizeCodexEvent(ev, raw)
	if !ok {
		t.Fatalf("expected turn usage normalization")
	}
	if got.Type != EventTurnUsage || got.UsageInputTokens != 10 || got.UsageCachedInputTokens != 2 || got.UsageOutputTokens != 3 {
		t.Fatalf("unexpected usage normalized event: %#v", got)
	}
}
