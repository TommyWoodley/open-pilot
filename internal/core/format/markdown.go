package format

import "strings"

const (
	BlockParagraph = "paragraph"
	BlockList      = "list"
	BlockHeading   = "heading"
	BlockCode      = "code"
	BlockBlank     = "blank"
)

type Block struct {
	Kind string
	Text string
	Lang string
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

func RenderInlineCode(text string, apply func(string) string) string {
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
			b.WriteString(apply(part))
		} else {
			b.WriteString(part)
		}
	}
	return b.String()
}
