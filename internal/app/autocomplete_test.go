package app

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thwoodle/open-pilot/internal/config"
)

func TestAutocompleteOneWordAtATimeRoot(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.Input = "/pro"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	nextModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model type from Update")
	}

	if nextModel.Input != "/provider " {
		t.Fatalf("expected one-word completion to /provider, got %q", nextModel.Input)
	}
}

func TestAutocompleteOneWordAtATimeSubcommand(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.Input = "/provider u"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	nextModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model type from Update")
	}

	if nextModel.Input != "/provider use " {
		t.Fatalf("expected one-word completion to use, got %q", nextModel.Input)
	}
}

func TestAutocompleteCompletesCurrentToken(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.Input = "/provider use c"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	nextModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model type from Update")
	}
	if nextModel.Input != "/provider use codex " {
		t.Fatalf("expected first completion codex, got %q", nextModel.Input)
	}
}

func TestSuggestionsRenderUnderInput(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.Width = 100
	m.Input = "/session "

	view := m.View()
	if !strings.Contains(view, "Suggestions:") {
		t.Fatalf("expected suggestions header in view")
	}
	if !strings.Contains(view, "/session list") {
		t.Fatalf("expected matching command suggestions in view")
	}
}
