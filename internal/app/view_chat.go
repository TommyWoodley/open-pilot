package app

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
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
		if title, ok := parsePilotDividerTitle(content); ok {
			prefix := strings.Repeat(" ", indent)
			out = append(out, prefix+renderPilotDividerLine(title, contentWidth))
			continue
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

func parsePilotDividerTitle(content string) (string, bool) {
	const prefix = "[[pilot-divider:"
	const suffix = "]]"
	if !strings.HasPrefix(content, prefix) || !strings.HasSuffix(content, suffix) {
		return "", false
	}
	title := strings.TrimSpace(content[len(prefix) : len(content)-len(suffix)])
	return title, true
}

func renderPilotDividerLine(title string, width int) string {
	if width <= 0 {
		return ""
	}
	lineChar := "─"
	if strings.EqualFold(strings.TrimSpace(title), "Development Work Complete") {
		lineChar = "═"
	}
	if strings.TrimSpace(title) == "" {
		return ui.HookDividerLineStyle.Render(strings.Repeat(lineChar, width))
	}
	label := " " + strings.TrimSpace(title) + " "
	labelWidth := lipgloss.Width(label)
	if labelWidth >= width {
		return ui.HookDividerTitleStyle.Render(truncateVisible(label, width))
	}
	left := (width - labelWidth) / 2
	right := width - labelWidth - left
	return ui.HookDividerLineStyle.Render(strings.Repeat(lineChar, left)) +
		ui.HookDividerTitleStyle.Render(label) +
		ui.HookDividerLineStyle.Render(strings.Repeat(lineChar, right))
}

func truncateVisible(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	runes := []rune(s)
	if width == 1 {
		return "…"
	}
	return string(runes[:width-1]) + "…"
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
