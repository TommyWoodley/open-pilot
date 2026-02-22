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
