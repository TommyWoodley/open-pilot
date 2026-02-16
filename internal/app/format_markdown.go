package app

import coreformat "github.com/thwoodle/open-pilot/internal/core/format"

const (
	mdBlockParagraph = coreformat.BlockParagraph
	mdBlockList      = coreformat.BlockList
	mdBlockHeading   = coreformat.BlockHeading
	mdBlockCode      = coreformat.BlockCode
	mdBlockBlank     = coreformat.BlockBlank
)

type mdBlock = coreformat.Block

func parseMarkdownBlocks(input string, streaming bool) []mdBlock {
	return coreformat.ParseMarkdownBlocks(input, streaming)
}

func parseHeading(line string) (string, bool) {
	blocks := parseMarkdownBlocks(line, false)
	if len(blocks) == 1 && blocks[0].Kind == mdBlockHeading {
		return blocks[0].Text, true
	}
	return "", false
}

func isListLine(line string) bool {
	blocks := parseMarkdownBlocks(line, false)
	return len(blocks) == 1 && blocks[0].Kind == mdBlockList
}
