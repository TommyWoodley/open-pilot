package app

import (
	"fmt"

	"github.com/thwoodle/open-pilot/internal/ui"
)

// View renders the top-level application layout.
func (m Model) View() string {
	header := ui.HeaderStyle.Render("open-pilot")

	bodyText := "Waiting for agent session..."
	if m.StatusText != "" {
		bodyText = fmt.Sprintf("Status: %s", m.StatusText)
	}
	body := ui.BodyStyle.Width(max(m.Width, 50)).Render(bodyText)

	footer := ui.FooterStyle.Render("q quit")

	return fmt.Sprintf("%s\n\n%s\n\n%s", header, body, footer)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
