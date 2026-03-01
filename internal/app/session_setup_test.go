package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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

	if !m.SessionSetupActive {
		t.Fatalf("expected session setup mode to be active")
	}
	if m.SessionSetupAutoReviewEnabled {
		t.Fatalf("expected default auto-review setup option to be disabled")
	}
	if m.Input != "/session add-repo " {
		t.Fatalf("expected prefilled add-repo input, got %q", m.Input)
	}
}

func TestSessionSetupConfirmEnabledPersistsOnSession(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.runCommand(Command{Kind: "session.new", Session: "demo"})
	if !m.SessionSetupActive {
		t.Fatalf("expected setup mode active")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	next := updated.(Model)
	if !next.SessionSetupAutoReviewEnabled {
		t.Fatalf("expected selection to toggle enabled")
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	confirmed := updated.(Model)
	if confirmed.SessionSetupActive {
		t.Fatalf("expected setup mode to close after confirmation")
	}

	s := confirmed.activeSession()
	if s == nil {
		t.Fatalf("expected active session")
	}
	if !s.AutoReviewLoopEnabled {
		t.Fatalf("expected session auto-review loop option enabled after confirmation")
	}
}

func TestSessionSetupEscKeepsDisabledDefault(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.runCommand(Command{Kind: "session.new", Session: "demo"})
	if !m.SessionSetupActive {
		t.Fatalf("expected setup mode active")
	}

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	next := updated.(Model)
	if next.SessionSetupActive {
		t.Fatalf("expected setup mode to close on escape")
	}

	s := next.activeSession()
	if s == nil {
		t.Fatalf("expected active session")
	}
	if s.AutoReviewLoopEnabled {
		t.Fatalf("expected default auto-review loop to remain disabled")
	}
}
