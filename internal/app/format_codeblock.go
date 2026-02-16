package app

import (
	"strings"

	"github.com/thwoodle/open-pilot/internal/ui"
)

func renderCodeBlock(block mdBlock) string {
	content := block.Text
	if block.Lang != "" {
		lang := ui.CodeBlockLangStyle.Render("[" + block.Lang + "]")
		content = lang + "\n" + content
	}
	return ui.CodeBlockStyle.Render(content)
}

func renderInlineCode(text string) string {
	if !strings.Contains(text, "`") {
		return text
	}
	parts := strings.Split(text, "`")
	if len(parts) < 3 {
		return text
	}
	var b strings.Builder
	for i, part := range parts {
		if i%2 == 1 {
			b.WriteString(ui.InlineCodeStyle.Render(part))
		} else {
			b.WriteString(part)
		}
	}
	return b.String()
}
