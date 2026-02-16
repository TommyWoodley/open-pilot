package app

import (
	"strings"

	"github.com/thwoodle/open-pilot/internal/ui"
)

func (m Model) renderSuggestions() string {
	suggestions := m.commandSuggestions(m.Input)
	if len(suggestions) == 0 {
		return ""
	}

	lines := append([]string{"Suggestions:"}, suggestions...)
	return ui.SuggestionStyle.Width(max(m.Width-2, 50)).Render(strings.Join(lines, "\n"))
}
