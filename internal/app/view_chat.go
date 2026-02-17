package app

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/thwoodle/open-pilot/internal/core/format"
	"github.com/thwoodle/open-pilot/internal/ui"
)

func (m Model) renderTranscript() string {
	allLines := m.displayTranscriptLines()
	if len(allLines) == 0 {
		text := "No messages yet. Start with /session new <name>"
		if m.activeSession() == nil && strings.TrimSpace(m.StatusText) != "" && m.StatusText != "No agent connected" {
			text = m.StatusText
		}
		return ui.BodyStyle.Width(max(m.Width-2, 50)).Render(text)
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

func (m Model) displayTranscriptLines() []string {
	raw := m.buildTranscriptLines()
	if len(raw) == 0 {
		return nil
	}
	return wrapTranscriptLines(raw, m.transcriptInnerWidth())
}

func (m Model) buildTranscriptLines() []string {
	s := m.activeSession()
	if s == nil || len(s.Messages) == 0 {
		return nil
	}
	return format.BuildTranscriptLines(s.Messages, m.transcriptStyles())
}

func (m Model) transcriptInnerWidth() int {
	outer := max(m.Width-2, 50)
	inner := outer - ui.BodyStyle.GetHorizontalFrameSize()
	if inner < 1 {
		return 1
	}
	return inner
}

func wrapTranscriptLines(lines []string, width int) []string {
	if width < 1 {
		return append([]string(nil), lines...)
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		indent := leadingSpaces(line)
		content := strings.TrimLeft(line, " ")
		if content == "" {
			out = append(out, line)
			continue
		}
		contentWidth := width - indent
		if contentWidth < 1 {
			contentWidth = 1
		}
		wrapped := ansi.Hardwrap(content, contentWidth, false)
		parts := strings.Split(wrapped, "\n")
		prefix := strings.Repeat(" ", indent)
		for _, p := range parts {
			out = append(out, prefix+p)
		}
	}
	return out
}

func leadingSpaces(s string) int {
	n := 0
	for n < len(s) && s[n] == ' ' {
		n++
	}
	return n
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
