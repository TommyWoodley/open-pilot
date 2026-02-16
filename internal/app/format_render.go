package app

import (
	"strings"

	"github.com/thwoodle/open-pilot/internal/domain"
	"github.com/thwoodle/open-pilot/internal/ui"
)

type renderedMessage struct {
	Prefix string
	Body   string
}

func formatMessageForTranscript(msg domain.Message) renderedMessage {
	prefix := "[system]"
	prefixStyle := ui.TranscriptSystemPrefixStyle
	switch msg.Role {
	case domain.RoleUser:
		prefix = "[you]"
		prefixStyle = ui.TranscriptUserPrefixStyle
	case domain.RoleAssistant:
		prefix = "[agent]"
		prefixStyle = ui.TranscriptAgentPrefixStyle
	}

	blocks := parseMarkdownBlocks(msg.Content, msg.Streaming)
	lines := make([]string, 0, len(blocks)+1)
	for _, block := range blocks {
		switch block.Kind {
		case mdBlockParagraph:
			for _, l := range strings.Split(block.Text, "\n") {
				lines = append(lines, renderInlineCode(l))
			}
		case mdBlockHeading:
			lines = append(lines, ui.MarkdownHeadingStyle.Render(renderInlineCode(block.Text)))
		case mdBlockList:
			lines = append(lines, ui.MarkdownListStyle.Render(renderInlineCode(block.Text)))
		case mdBlockCode:
			lines = append(lines, renderCodeBlock(block))
		case mdBlockBlank:
			lines = append(lines, "")
		}
	}

	body := strings.Join(lines, "\n")
	if msg.Streaming {
		if body == "" {
			body = "..."
		} else {
			body += "\n..."
		}
	}

	return renderedMessage{
		Prefix: prefixStyle.Render(prefix),
		Body:   body,
	}
}
