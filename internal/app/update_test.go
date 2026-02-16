package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thwoodle/open-pilot/internal/config"
)

func TestUpdateQuitKeys(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())

	cases := []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune{'q'}},
		{Type: tea.KeyCtrlC},
	}

	for _, msg := range cases {
		_, cmd := m.Update(msg)
		if cmd == nil {
			t.Fatalf("expected quit command for key %q", msg.String())
		}
	}
}

func TestUpdateWindowSize(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if cmd != nil {
		t.Fatalf("expected nil command for resize, got non-nil")
	}

	nextModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model type from Update")
	}

	if nextModel.Width != 120 || nextModel.Height != 40 {
		t.Fatalf("expected dimensions 120x40, got %dx%d", nextModel.Width, nextModel.Height)
	}

	if !nextModel.Ready {
		t.Fatalf("expected model to be ready after first window size message")
	}
}

func TestPromptBlockedWithoutSession(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.Input = "hello"
	m = m.processEnter()

	if m.StatusText == "" {
		t.Fatalf("expected status message when prompt is blocked")
	}
}

func TestUpdateAcceptsSpaceInput(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())
	m.Input = "/provider"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	nextModel, ok := updated.(Model)
	if !ok {
		t.Fatalf("expected Model type from Update")
	}

	if nextModel.Input != "/provider " {
		t.Fatalf("expected space to be appended, got %q", nextModel.Input)
	}
}
