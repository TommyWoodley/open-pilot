package app

import "testing"

func TestParseCommandValid(t *testing.T) {
	t.Parallel()

	cmd, isCommand, err := ParseCommand("/session new my session")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isCommand {
		t.Fatalf("expected command")
	}
	if cmd.Kind != "session.new" || cmd.Session != "my session" {
		t.Fatalf("unexpected parsed command: %#v", cmd)
	}
}

func TestParseCommandInvalid(t *testing.T) {
	t.Parallel()

	_, isCommand, err := ParseCommand("/provider use")
	if !isCommand {
		t.Fatalf("expected command")
	}
	if err == nil {
		t.Fatalf("expected parse error")
	}
}
