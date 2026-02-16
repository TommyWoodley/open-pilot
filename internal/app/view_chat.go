package app

import (
	"strings"

	"github.com/thwoodle/open-pilot/internal/ui"
)

func (m Model) renderTranscript() string {
	s := m.activeSession()
	if s == nil || len(s.Messages) == 0 {
		return ui.BodyStyle.Width(max(m.Width-2, 50)).Render("No messages yet. Start with /session new <name>")
	}

	maxLines := max(m.Height-8, 6)
	lines := make([]string, 0, len(s.Messages))
	for _, msg := range s.Messages {
		formatted := formatMessageForTranscript(msg)
		if formatted.Body == "" {
			lines = append(lines, formatted.Prefix)
		} else {
			bodyLines := strings.Split(formatted.Body, "\n")
			lines = append(lines, formatted.Prefix+" "+bodyLines[0])
			for i := 1; i < len(bodyLines); i++ {
				lines = append(lines, bodyLines[i])
			}
		}
		lines = append(lines, "")
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return ui.BodyStyle.Width(max(m.Width-2, 50)).Render(strings.Join(lines, "\n"))
}
