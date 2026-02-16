package app

import (
	"strings"

	"github.com/thwoodle/open-pilot/internal/domain"
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
		prefix := "[system]"
		switch msg.Role {
		case domain.RoleUser:
			prefix = "[you]"
		case domain.RoleAssistant:
			prefix = "[agent]"
		}
		suffix := ""
		if msg.Streaming {
			suffix = " ..."
		}
		lines = append(lines, prefix+" "+msg.Content+suffix)
	}
	if len(lines) > maxLines {
		lines = lines[len(lines)-maxLines:]
	}
	return ui.BodyStyle.Width(max(m.Width-2, 50)).Render(strings.Join(lines, "\n"))
}
