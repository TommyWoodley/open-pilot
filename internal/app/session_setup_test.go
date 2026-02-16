package app

import (
	"strings"
	"testing"

	"github.com/thwoodle/open-pilot/internal/config"
)

func TestSessionNewSetsDefaultProviderCodex(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.runCommand(Command{Kind: "session.new", Session: "demo"})

	s := m.activeSession()
	if s == nil {
		t.Fatalf("expected active session")
	}
	if s.ProviderID != "codex" {
		t.Fatalf("expected default provider codex, got %q", s.ProviderID)
	}
}

func TestSessionNewPrefillsAddRepoInput(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.runCommand(Command{Kind: "session.new", Session: "demo"})

	if m.Input != "/session add-repo " {
		t.Fatalf("expected prefilled add-repo input, got %q", m.Input)
	}
}

func TestPromptStillBlockedWithoutSetup(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.Input = "hello"
	m = m.processEnter()

	if !strings.Contains(m.StatusText, "/session new <name>") {
		t.Fatalf("expected guided setup message, got %q", m.StatusText)
	}
}
