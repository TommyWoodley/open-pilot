package app

import (
	"testing"

	"github.com/thwoodle/open-pilot/internal/config"
)

func TestSessionLifecycleCommands(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())

	m.runCommand(Command{Kind: "session.new", Session: "demo"})
	s := m.activeSession()
	if s == nil {
		t.Fatalf("expected active session")
	}

	if err := m.addRepoToActiveSession("/tmp", "tmp-repo"); err != nil {
		t.Fatalf("add repo failed: %v", err)
	}

	repo := m.activeRepo()
	if repo == nil {
		t.Fatalf("expected active repo")
	}

	m.runCommand(Command{Kind: "provider.use", ProviderID: "codex"})
	if s.ProviderID != "codex" {
		t.Fatalf("expected provider codex, got %q", s.ProviderID)
	}
}
