package app

import (
	"fmt"

	"github.com/thwoodle/open-pilot/internal/ui"
)

// View renders the top-level application layout.
func (m Model) View() string {
	header := ui.HeaderStyle.Render("open-pilot")
	sessionBar := m.renderSessionBar()
	chat := m.renderTranscript()
	input := ui.InputStyle.Width(max(m.Width-2, 50)).Render("> " + m.Input)
	suggestions := m.renderSuggestions()
	status := m.renderStatus()

	if suggestions != "" {
		return fmt.Sprintf("%s\n%s\n\n%s\n\n%s\n%s\n%s", header, sessionBar, chat, input, suggestions, status)
	}
	return fmt.Sprintf("%s\n%s\n\n%s\n\n%s\n%s", header, sessionBar, chat, input, status)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
