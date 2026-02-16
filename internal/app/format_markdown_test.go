package app

import "testing"

func TestParseMarkdownBlocksFencedCode(t *testing.T) {
	t.Parallel()

	input := "# Title\n\n```go\nfmt.Println(\"x\")\n```\n"
	blocks := parseMarkdownBlocks(input, false)
	if len(blocks) < 3 {
		t.Fatalf("expected multiple markdown blocks, got %d", len(blocks))
	}

	foundHeading := false
	foundCode := false
	for _, b := range blocks {
		if b.Kind == mdBlockHeading && b.Text == "Title" {
			foundHeading = true
		}
		if b.Kind == mdBlockCode {
			foundCode = true
			if b.Lang != "go" {
				t.Fatalf("expected go code fence language, got %q", b.Lang)
			}
		}
	}
	if !foundHeading || !foundCode {
		t.Fatalf("expected heading and code blocks")
	}
}

func TestParseMarkdownBlocksListAndInlineCode(t *testing.T) {
	t.Parallel()

	input := "- one\n- two with `code`"
	blocks := parseMarkdownBlocks(input, false)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 list blocks, got %d", len(blocks))
	}
	if blocks[0].Kind != mdBlockList || blocks[1].Kind != mdBlockList {
		t.Fatalf("expected list blocks")
	}

	rendered := renderInlineCode(blocks[1].Text)
	if rendered == blocks[1].Text {
		t.Fatalf("expected inline code rendering to modify output")
	}
}

func TestParseMarkdownBlocksUnclosedFenceStreaming(t *testing.T) {
	t.Parallel()

	input := "```go\nfmt.Println(1)"
	blocks := parseMarkdownBlocks(input, true)
	if len(blocks) != 1 {
		t.Fatalf("expected single code block for streaming unclosed fence, got %d", len(blocks))
	}
	if blocks[0].Kind != mdBlockCode {
		t.Fatalf("expected code block, got %q", blocks[0].Kind)
	}
}

func TestParseMarkdownBlocksUnclosedFenceFinalized(t *testing.T) {
	t.Parallel()

	input := "```go\nfmt.Println(1)"
	blocks := parseMarkdownBlocks(input, false)
	if len(blocks) != 1 {
		t.Fatalf("expected single paragraph for finalized unclosed fence, got %d", len(blocks))
	}
	if blocks[0].Kind != mdBlockParagraph {
		t.Fatalf("expected paragraph fallback, got %q", blocks[0].Kind)
	}
}
