package app

import (
	coreformat "github.com/thwoodle/open-pilot/internal/core/format"
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
	return coreformat.RenderInlineCode(text, func(s string) string {
		return ui.InlineCodeStyle.Render(s)
	})
}
