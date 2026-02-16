package app

import (
	"strings"

	"github.com/thwoodle/open-pilot/internal/ui"
)

func (m Model) renderTranscript() string {
	allLines := m.buildTranscriptLines()
	if len(allLines) == 0 {
		return ui.BodyStyle.Width(max(m.Width-2, 50)).Render("No messages yet. Start with /session new <name>")
	}

	visible := m.transcriptVisibleLines()
	total := len(allLines)
	maxScrollable := max(total-visible, 0)

	scroll := m.TranscriptScroll
	if m.AutoFollowTranscript {
		scroll = 0
	}
	if scroll < 0 {
		scroll = 0
	}
	if scroll > maxScrollable {
		scroll = maxScrollable
	}

	start := max(total-visible-scroll, 0)
	end := min(start+visible, total)

	return ui.BodyStyle.Width(max(m.Width-2, 50)).Render(strings.Join(allLines[start:end], "\n"))
}

func (m Model) buildTranscriptLines() []string {
	s := m.activeSession()
	if s == nil || len(s.Messages) == 0 {
		return nil
	}

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
	return lines
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
