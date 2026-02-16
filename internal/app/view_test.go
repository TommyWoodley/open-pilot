package app

import (
	"strings"
	"testing"
)

func TestViewContainsCoreSections(t *testing.T) {
	t.Parallel()

	m := NewModel()
	m.Width = 80
	view := m.View()

	checks := []string{
		"open-pilot",
		"Status:",
		"q quit",
	}

	for _, expected := range checks {
		if !strings.Contains(view, expected) {
			t.Fatalf("expected view to contain %q", expected)
		}
	}
}
