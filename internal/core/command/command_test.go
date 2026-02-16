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

func TestParseInvalid(t *testing.T) {
	_, isCommand, err := Parse("/provider use")
	if !isCommand || err == nil {
		t.Fatalf("expected parse error for invalid provider command")
	}
}

func TestHelpTextContainsCommands(t *testing.T) {
	h := HelpText()
	if !strings.Contains(h, "/session new") || !strings.Contains(h, "/provider status") {
		t.Fatalf("expected command help text to contain core commands")
	}
}
