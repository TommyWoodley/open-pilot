package providers

import "testing"

func TestExtractCodexSessionID(t *testing.T) {
	t.Parallel()

	raw := `OpenAI Codex
session id: 019c6744-cdb6-7fb1-a066-6b0ae0ca189c
hello`

	got := extractCodexSessionID(raw)
	if got != "019c6744-cdb6-7fb1-a066-6b0ae0ca189c" {
		t.Fatalf("unexpected session id: %q", got)
	}
}

func TestExtractCodexSessionIDMissing(t *testing.T) {
	t.Parallel()

	if got := extractCodexSessionID("no session id here"); got != "" {
		t.Fatalf("expected empty session id, got %q", got)
	}
}
