package command

import (
	"strings"
	"testing"
)

func TestParseSessionNew(t *testing.T) {
	cmd, isCommand, err := Parse("/session new demo")
	if err != nil || !isCommand {
		t.Fatalf("expected command parse success, err=%v isCommand=%v", err, isCommand)
	}
	if cmd.Kind != KindSessionNew || cmd.Session != "demo" {
		t.Fatalf("unexpected command: %#v", cmd)
	}
}

func TestParseSessionUseWithSpaces(t *testing.T) {
	cmd, isCommand, err := Parse("/session use my project")
	if err != nil || !isCommand {
		t.Fatalf("expected command parse success, err=%v isCommand=%v", err, isCommand)
	}
	if cmd.Kind != KindSessionUse || cmd.SessionID != "my project" {
		t.Fatalf("unexpected command: %#v", cmd)
	}
}

func TestParseSessionDeleteAliases(t *testing.T) {
	tests := []string{
		"/session delete demo",
		"/session remove demo",
		"/session destroy demo",
	}
	for _, input := range tests {
		cmd, isCommand, err := Parse(input)
		if err != nil || !isCommand {
			t.Fatalf("expected command parse success for %q, err=%v isCommand=%v", input, err, isCommand)
		}
		if cmd.Kind != KindSessionDelete || cmd.SessionID != "demo" {
			t.Fatalf("unexpected command for %q: %#v", input, cmd)
		}
	}
}

func TestParseInvalid(t *testing.T) {
	_, isCommand, err := Parse("/provider use")
	if !isCommand || err == nil {
		t.Fatalf("expected parse error for invalid provider command")
	}
}

func TestHelpTextContainsCommands(t *testing.T) {
	h := HelpText()
	if !strings.Contains(h, "/session new") || !strings.Contains(h, "/session delete") || !strings.Contains(h, "/provider status") {
		t.Fatalf("expected command help text to contain core commands")
	}
}
