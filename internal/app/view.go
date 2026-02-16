package app

import (
	"fmt"

	"github.com/thwoodle/open-pilot/internal/ui"
)

// View renders the top-level application layout.
func (m Model) View() string {
	header := ui.HeaderStyle.Render("open-pilot")
	chat := m.renderTranscript()
	input := ui.InputStyle.Width(max(m.Width-2, 50)).Render("> " + m.Input)
	status := m.renderStatus()

	return fmt.Sprintf("%s\n\n%s\n\n%s\n%s", header, chat, input, status)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
