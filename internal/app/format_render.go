package app

import (
	"os"
	"strings"

	"github.com/thwoodle/open-pilot/internal/core/format"
	"github.com/thwoodle/open-pilot/internal/ui"
)

func (m Model) transcriptStyles() format.Styles {
	dots := m.generationDots()
	return format.Styles{
		UserPrefix: func(s string) string {
			return ui.TranscriptUserPrefixStyle.Render(s)
		},
		AgentPrefix: func(s string) string {
			return ui.TranscriptAgentPrefixStyle.Render(s)
		},
		SystemPrefix: func(s string) string {
			return ui.TranscriptSystemPrefixStyle.Render(s)
		},
		Heading: func(s string) string {
			return ui.MarkdownHeadingStyle.Render(s)
		},
		List: func(s string) string {
			return ui.MarkdownListStyle.Render(s)
		},
		Quote: func(s string) string {
			return ui.MarkdownQuoteStyle.Render("│ " + s)
		},
		Link: func(label, url string) string {
			if !canUseOSC8() {
				return label + " (" + url + ")"
			}
			styledLabel := ui.MarkdownLinkStyle.Render(label)
			return osc8Link(styledLabel, url)
		},
		InlineCode: func(s string) string {
			return ui.InlineCodeStyle.Render(s)
		},
		Bold: func(s string) string {
			return ui.MarkdownBoldStyle.Render(s)
		},
		Italic: func(s string) string {
			return ui.MarkdownItalicStyle.Render(s)
		},
		Strike: func(s string) string {
			return ui.MarkdownStrikeStyle.Render(s)
		},
		CodeBlock: func(lang, text string) string {
			content := text
			if lang != "" {
				content = ui.CodeBlockLangStyle.Render("["+lang+"]") + "\n" + content
			}
			return ui.CodeBlockStyle.Render(content)
		},
		StreamingPlaceholder: func() string {
			return dots
		},
		StreamingSuffix: func() string {
			return dots
		},
	}
}

func canUseOSC8() bool {
	if os.Getenv("OPEN_PILOT_DISABLE_OSC8") == "1" {
		return false
	}
	if os.Getenv("TERM") == "dumb" {
		return false
	}
	return true
}

func osc8Link(label, url string) string {
	esc := "\x1b"
	st := esc + "\\"
	url = strings.TrimSpace(url)
	if url == "" {
		return label
	}
	return esc + "]8;;" + url + st + label + esc + "]8;;" + st
}
