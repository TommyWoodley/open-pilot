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
