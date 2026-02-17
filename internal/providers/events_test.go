package providers

import "testing"

func TestParseWrapperEventValid(t *testing.T) {
	t.Parallel()

	ev, err := parseWrapperEvent([]byte(`{"type":"chunk","id":"r1","text":"hello"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != EventChunk || ev.RequestID != "r1" || ev.Text != "hello" {
		t.Fatalf("unexpected event: %#v", ev)
	}
}

func TestParseWrapperEventInvalid(t *testing.T) {
	t.Parallel()

	_, err := parseWrapperEvent([]byte(`{"id":"x"}`))
	if err == nil {
		t.Fatalf("expected error for missing type")
	}
}

func TestParseWrapperEventUnknownType(t *testing.T) {
	t.Parallel()

	line := []byte(`{"type":"tool.preview","id":"r2","text":"x"}`)
	ev, err := parseWrapperEvent(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != EventUnknown {
		t.Fatalf("expected unknown event type, got %q", ev.Type)
	}
	if ev.RawType != "tool.preview" {
		t.Fatalf("expected raw type to be preserved, got %q", ev.RawType)
	}
	if ev.RawJSON != string(line) {
		t.Fatalf("expected raw json to be preserved")
	}
}
