package app

import "strings"

const (
	mdBlockParagraph = "paragraph"
	mdBlockList      = "list"
	mdBlockHeading   = "heading"
	mdBlockCode      = "code"
	mdBlockBlank     = "blank"
)

type mdBlock struct {
	Kind string
	Text string
	Lang string
}

func parseMarkdownBlocks(input string, streaming bool) []mdBlock {
	if input == "" {
		return nil
	}

	lines := strings.Split(input, "\n")
	blocks := make([]mdBlock, 0, len(lines))
	paragraph := make([]string, 0, 4)
	codeLines := make([]string, 0, 8)
	inCode := false
	lang := ""

	flushParagraph := func() {
		if len(paragraph) == 0 {
			return
		}
		blocks = append(blocks, mdBlock{Kind: mdBlockParagraph, Text: strings.Join(paragraph, "\n")})
		paragraph = paragraph[:0]
	}

	flushCode := func() {
		blocks = append(blocks, mdBlock{Kind: mdBlockCode, Text: strings.Join(codeLines, "\n"), Lang: lang})
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
			blocks = append(blocks, mdBlock{Kind: mdBlockBlank})
			continue
		}

		if headingText, ok := parseHeading(trimmed); ok {
			flushParagraph()
			blocks = append(blocks, mdBlock{Kind: mdBlockHeading, Text: headingText})
			continue
		}

		if isListLine(trimmed) {
			flushParagraph()
			blocks = append(blocks, mdBlock{Kind: mdBlockList, Text: trimmed})
			continue
		}

		paragraph = append(paragraph, line)
	}

	flushParagraph()
	if inCode && streaming {
		flushCode()
	}
	if inCode && !streaming {
		// Preserve unclosed fences as plain text on finalized render.
		blocks = append(blocks, mdBlock{Kind: mdBlockParagraph, Text: "```" + lang + "\n" + strings.Join(codeLines, "\n")})
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
