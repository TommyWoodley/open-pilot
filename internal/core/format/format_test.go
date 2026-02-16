package format

import (
	"testing"

	"github.com/thwoodle/open-pilot/internal/domain"
)

func TestParseMarkdownBlocksFencedCode(t *testing.T) {
	input := "# Title\n\n```go\nfmt.Println(\"x\")\n```\n"
	blocks := ParseMarkdownBlocks(input, false)
	if len(blocks) < 3 {
		t.Fatalf("expected multiple markdown blocks, got %d", len(blocks))
	}

	foundHeading := false
	foundCode := false
	for _, b := range blocks {
		if b.Kind == BlockHeading && b.Text == "Title" {
			foundHeading = true
		}
		if b.Kind == BlockCode {
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
	input := "- one\n- two with `code`"
	blocks := ParseMarkdownBlocks(input, false)
	if len(blocks) != 2 {
		t.Fatalf("expected 2 list blocks, got %d", len(blocks))
	}
	if blocks[0].Kind != BlockList || blocks[1].Kind != BlockList {
		t.Fatalf("expected list blocks")
	}

	rendered := RenderInlineCode(blocks[1].Text, func(s string) string { return "[" + s + "]" })
	if rendered == blocks[1].Text {
		t.Fatalf("expected inline code rendering to modify output")
	}
}

func TestParseMarkdownBlocksUnclosedFenceStreaming(t *testing.T) {
	input := "```go\nfmt.Println(1)"
	blocks := ParseMarkdownBlocks(input, true)
	if len(blocks) != 1 {
		t.Fatalf("expected single code block for streaming unclosed fence, got %d", len(blocks))
	}
	if blocks[0].Kind != BlockCode {
		t.Fatalf("expected code block, got %q", blocks[0].Kind)
	}
}

func TestParseMarkdownBlocksUnclosedFenceFinalized(t *testing.T) {
	input := "```go\nfmt.Println(1)"
	blocks := ParseMarkdownBlocks(input, false)
	if len(blocks) != 1 {
		t.Fatalf("expected single paragraph for finalized unclosed fence, got %d", len(blocks))
	}
	if blocks[0].Kind != BlockParagraph {
		t.Fatalf("expected paragraph fallback, got %q", blocks[0].Kind)
	}
}

func TestBuildTranscriptLines(t *testing.T) {
	lines := BuildTranscriptLines([]domain.Message{{Role: domain.RoleAssistant, Content: "hello"}}, Styles{})
	if len(lines) == 0 {
		t.Fatalf("expected transcript lines")
	}
}
