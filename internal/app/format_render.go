package app

import (
	"github.com/thwoodle/open-pilot/internal/core/format"
	"github.com/thwoodle/open-pilot/internal/domain"
	"github.com/thwoodle/open-pilot/internal/ui"
)

type renderedMessage = format.RenderedMessage

func formatMessageForTranscript(msg domain.Message) renderedMessage {
	return format.FormatMessageForTranscript(msg, transcriptStyles())
}

func transcriptStyles() format.Styles {
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
		InlineCode: func(s string) string {
			return ui.InlineCodeStyle.Render(s)
		},
		CodeBlock: func(lang, text string) string {
			content := text
			if lang != "" {
				content = ui.CodeBlockLangStyle.Render("["+lang+"]") + "\n" + content
			}
			return ui.CodeBlockStyle.Render(content)
		},
	}
}
