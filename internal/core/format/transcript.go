package format

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/thwoodle/open-pilot/internal/domain"
)

type RenderedMessage struct {
	Prefix string
	Body   string
}

type Styles struct {
	UserPrefix           func(string) string
	AgentPrefix          func(string) string
	SystemPrefix         func(string) string
	Heading              func(string) string
	List                 func(string) string
	Quote                func(string) string
	Link                 func(label, url string) string
	InlineCode           func(string) string
	Bold                 func(string) string
	Italic               func(string) string
	Strike               func(string) string
	CodeBlock            func(lang, text string) string
	StreamingPlaceholder func() string
	StreamingSuffix      func() string
}

func FormatMessageForTranscript(msg domain.Message, styles Styles) RenderedMessage {
	prefix := "[pilot]"
	prefixRender := styles.SystemPrefix
	switch msg.Role {
	case domain.RoleUser:
		prefix = "[you]"
		prefixRender = styles.UserPrefix
	case domain.RoleAssistant:
		prefix = "[agent]"
		prefixRender = styles.AgentPrefix
	}
	if prefixRender != nil {
		prefix = prefixRender(prefix)
	}

	blocks := ParseMarkdownBlocks(msg.Content, msg.Streaming)
	lines := make([]string, 0, len(blocks)+1)
	for _, block := range blocks {
		switch block.Kind {
		case BlockParagraph:
			for _, l := range strings.Split(block.Text, "\n") {
				lines = append(lines, RenderInline(l, InlineStyles{
					Code:   styles.InlineCode,
					Link:   styles.Link,
					Bold:   styles.Bold,
					Italic: styles.Italic,
					Strike: styles.Strike,
				}))
			}
		case BlockHeading:
			txt := RenderInline(block.Text, InlineStyles{
				Code:   styles.InlineCode,
				Link:   styles.Link,
				Bold:   styles.Bold,
				Italic: styles.Italic,
				Strike: styles.Strike,
			})
			if styles.Heading != nil {
				txt = styles.Heading(txt)
			}
			lines = append(lines, txt)
		case BlockList:
			txt := RenderInline(block.Text, InlineStyles{
				Code:   styles.InlineCode,
				Link:   styles.Link,
				Bold:   styles.Bold,
				Italic: styles.Italic,
				Strike: styles.Strike,
			})
			if styles.List != nil {
				txt = styles.List(txt)
			}
			lines = append(lines, txt)
		case BlockQuote:
			txt := RenderInline(block.Text, InlineStyles{
				Code:   styles.InlineCode,
				Link:   styles.Link,
				Bold:   styles.Bold,
				Italic: styles.Italic,
				Strike: styles.Strike,
			})
			if styles.Quote != nil {
				txt = styles.Quote(txt)
			}
			lines = append(lines, txt)
		case BlockCode:
			if styles.CodeBlock != nil {
				lines = append(lines, styles.CodeBlock(block.Lang, block.Text))
			} else {
				lines = append(lines, block.Text)
			}
		case BlockBlank:
			lines = append(lines, "")
		}
	}

	body := strings.Join(lines, "\n")
	if msg.Streaming {
		if body == "" {
			if styles.StreamingPlaceholder != nil {
				body = styles.StreamingPlaceholder()
			} else {
				body = "..."
			}
		} else {
			if styles.StreamingSuffix != nil {
				body += "\n" + styles.StreamingSuffix()
			} else {
				body += "\n..."
			}
		}
	}

	return RenderedMessage{Prefix: prefix, Body: body}
}

func BuildTranscriptLines(messages []domain.Message, styles Styles) []string {
	if len(messages) == 0 {
		return nil
	}
	lines := make([]string, 0, len(messages))
	for _, msg := range messages {
		formatted := FormatMessageForTranscript(msg, styles)
		if formatted.Body == "" {
			lines = append(lines, formatted.Prefix)
		} else {
			bodyLines := strings.Split(formatted.Body, "\n")
			lines = append(lines, formatted.Prefix+" "+bodyLines[0])
			continuationIndent := strings.Repeat(" ", visibleTextWidth(formatted.Prefix)+1)
			for i := 1; i < len(bodyLines); i++ {
				lines = append(lines, continuationIndent+bodyLines[i])
			}
		}
		lines = append(lines, "")
	}
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

var sgrANSIRegex = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func visibleTextWidth(s string) int {
	plain := sgrANSIRegex.ReplaceAllString(s, "")
	return utf8.RuneCountInString(plain)
}
