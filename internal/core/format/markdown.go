package format

import "strings"

const (
	BlockParagraph = "paragraph"
	BlockList      = "list"
	BlockHeading   = "heading"
	BlockQuote     = "quote"
	BlockCode      = "code"
	BlockBlank     = "blank"
)

type Block struct {
	Kind string
	Text string
	Lang string
}

type InlineStyles struct {
	Code   func(string) string
	Link   func(label, url string) string
	Bold   func(string) string
	Italic func(string) string
	Strike func(string) string
}

func ParseMarkdownBlocks(input string, streaming bool) []Block {
	if input == "" {
		return nil
	}

	lines := strings.Split(input, "\n")
	blocks := make([]Block, 0, len(lines))
	paragraph := make([]string, 0, 4)
	codeLines := make([]string, 0, 8)
	inCode := false
	lang := ""

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		blocks = append(blocks, Block{Kind: BlockParagraph, Text: strings.Join(paragraph, "\n")})
		paragraph = paragraph[:0]
	}

	flushCode := func() {
		blocks = append(blocks, Block{Kind: BlockCode, Text: strings.Join(codeLines, "\n"), Lang: lang})
		codeLines = codeLines[:0]
		lang = ""
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			if inCode {
				flushCode()
				inCode = false
				continue
			}
			flushParagraph()
			inCode = true
			lang = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
			continue
		}

		if inCode {
			codeLines = append(codeLines, line)
			continue
		}

		if trimmed == "" {
			flushParagraph()
			blocks = append(blocks, Block{Kind: BlockBlank})
			continue
		}

		if headingText, ok := parseHeading(trimmed); ok {
			flushParagraph()
			blocks = append(blocks, Block{Kind: BlockHeading, Text: headingText})
			continue
		}

		if isListLine(trimmed) {
			flushParagraph()
			blocks = append(blocks, Block{Kind: BlockList, Text: trimmed})
			continue
		}

		if quoteText, ok := parseQuote(trimmed); ok {
			flushParagraph()
			blocks = append(blocks, Block{Kind: BlockQuote, Text: quoteText})
			continue
		}

		paragraph = append(paragraph, line)
	}

	flushParagraph()
	if inCode && streaming {
		flushCode()
	}
	if inCode && !streaming {
		blocks = append(blocks, Block{Kind: BlockParagraph, Text: "```" + lang + "\n" + strings.Join(codeLines, "\n")})
	}

	return blocks
}

func parseHeading(line string) (string, bool) {
	if !strings.HasPrefix(line, "#") {
		return "", false
	}
	i := 0
	for i < len(line) && line[i] == '#' {
		i++
	}
	if i == 0 || i > 3 || i >= len(line) || line[i] != ' ' {
		return "", false
	}
	return strings.TrimSpace(line[i+1:]), true
}

func isListLine(line string) bool {
	if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
		return true
	}
	if len(line) < 3 {
		return false
	}
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	return i > 0 && i+1 < len(line) && line[i] == '.' && line[i+1] == ' '
}

func parseQuote(line string) (string, bool) {
	if !strings.HasPrefix(line, "> ") {
		return "", false
	}
	return strings.TrimSpace(strings.TrimPrefix(line, "> ")), true
}

func RenderInlineCode(text string, apply func(string) string) string {
	return RenderInline(text, InlineStyles{Code: apply})
}

func RenderInline(text string, styles InlineStyles) string {
	if text == "" {
		return text
	}
	if !strings.Contains(text, "`") {
		return renderEmphasis(text, styles)
	}

	var out strings.Builder
	start := 0
	for start < len(text) {
		open := strings.IndexByte(text[start:], '`')
		if open < 0 {
			out.WriteString(renderEmphasis(text[start:], styles))
			break
		}
		open += start
		close := strings.IndexByte(text[open+1:], '`')
		if close < 0 {
			out.WriteString(renderEmphasis(text[start:], styles))
			break
		}
		close += open + 1

		out.WriteString(renderEmphasis(text[start:open], styles))
		codeText := text[open+1 : close]
		if styles.Code != nil {
			out.WriteString(styles.Code(codeText))
		} else {
			out.WriteString(codeText)
		}
		start = close + 1
	}
	return out.String()
}

func renderEmphasis(text string, styles InlineStyles) string {
	if text == "" {
		return text
	}
	text = renderLinks(text, styles)
	text = renderDelimited(text, "**", styles.Bold, styles)
	text = renderDelimited(text, "__", styles.Bold, styles)
	text = renderDelimited(text, "~~", styles.Strike, styles)
	text = renderDelimited(text, "*", styles.Italic, styles)
	text = renderDelimited(text, "_", styles.Italic, styles)
	return text
}

func renderLinks(text string, styles InlineStyles) string {
	var out strings.Builder
	i := 0
	for i < len(text) {
		openLabel := strings.IndexByte(text[i:], '[')
		if openLabel < 0 {
			out.WriteString(text[i:])
			break
		}
		openLabel += i

		closeLabel := strings.IndexByte(text[openLabel+1:], ']')
		if closeLabel < 0 {
			out.WriteString(text[i:])
			break
		}
		closeLabel += openLabel + 1

		if closeLabel+1 >= len(text) || text[closeLabel+1] != '(' {
			out.WriteString(text[i : closeLabel+1])
			i = closeLabel + 1
			continue
		}

		closeURL := strings.IndexByte(text[closeLabel+2:], ')')
		if closeURL < 0 {
			out.WriteString(text[i:])
			break
		}
		closeURL += closeLabel + 2

		label := text[openLabel+1 : closeLabel]
		url := text[closeLabel+2 : closeURL]
		if strings.TrimSpace(label) == "" || strings.TrimSpace(url) == "" {
			out.WriteString(text[i : closeURL+1])
			i = closeURL + 1
			continue
		}

		out.WriteString(text[i:openLabel])
		if styles.Link != nil {
			out.WriteString(styles.Link(label, url))
		} else {
			out.WriteString(label + " (" + url + ")")
		}
		i = closeURL + 1
	}
	return out.String()
}

func renderDelimited(text, marker string, apply func(string) string, styles InlineStyles) string {
	if apply == nil || marker == "" || len(text) < len(marker)*2 {
		return text
	}
	var out strings.Builder
	i := 0
	for i < len(text) {
		open := strings.Index(text[i:], marker)
		if open < 0 {
			out.WriteString(text[i:])
			break
		}
		open += i
		close := strings.Index(text[open+len(marker):], marker)
		if close < 0 {
			out.WriteString(text[i:])
			break
		}
		close += open + len(marker)

		out.WriteString(text[i:open])
		inner := text[open+len(marker) : close]
		if inner == "" {
			out.WriteString(text[open : close+len(marker)])
		} else {
			out.WriteString(apply(renderEmphasis(inner, styles)))
		}
		i = close + len(marker)
	}
	return out.String()
}
