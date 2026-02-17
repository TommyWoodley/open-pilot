package format

import (
	"strings"
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

func TestBuildTranscriptLinesMultilineBodyKeepsContinuationIndent(t *testing.T) {
	lines := BuildTranscriptLines([]domain.Message{{Role: domain.RoleSystem, Content: "first\nsecond"}}, Styles{})
	if len(lines) < 2 {
		t.Fatalf("expected multiline transcript output")
	}
	if lines[0] != "[system] first" {
		t.Fatalf("unexpected first line: %q", lines[0])
	}
	if lines[1] != "         second" { // len("[system] ") = 9
		t.Fatalf("expected continuation line to align with body column, got %q", lines[1])
	}
}

func TestBuildTranscriptLinesContinuationIndentWithStyledPrefix(t *testing.T) {
	lines := BuildTranscriptLines([]domain.Message{{Role: domain.RoleSystem, Content: "one\ntwo"}}, Styles{
		SystemPrefix: func(s string) string { return "\x1b[33m" + s + "\x1b[0m" },
	})
	if len(lines) < 2 {
		t.Fatalf("expected multiline transcript output")
	}
	if !strings.HasSuffix(lines[1], "two") {
		t.Fatalf("expected continuation to include body text, got %q", lines[1])
	}
	if lines[1] != "         two" { // visible width should ignore ANSI style bytes
		t.Fatalf("expected continuation indent to ignore ANSI bytes, got %q", lines[1])
	}
}

func TestRenderInlineBoldItalicStrike(t *testing.T) {
	got := RenderInline("a **b** __c__ *d* _e_ ~~f~~", InlineStyles{
		Bold: func(s string) string { return "<b>" + s + "</b>" },
		Italic: func(s string) string {
			return "<i>" + s + "</i>"
		},
		Strike: func(s string) string {
			return "<s>" + s + "</s>"
		},
	})
	checks := []string{"<b>b</b>", "<b>c</b>", "<i>d</i>", "<i>e</i>", "<s>f</s>"}
	for _, c := range checks {
		if !strings.Contains(got, c) {
			t.Fatalf("expected %q in %q", c, got)
		}
	}
}

func TestRenderInlineCodeTakesPrecedence(t *testing.T) {
	got := RenderInline("x `**not-bold**` y **bold**", InlineStyles{
		Code: func(s string) string { return "<code>" + s + "</code>" },
		Bold: func(s string) string { return "<b>" + s + "</b>" },
	})
	if !strings.Contains(got, "<code>**not-bold**</code>") {
		t.Fatalf("expected code span to remain literal for emphasis markers, got %q", got)
	}
	if !strings.Contains(got, "<b>bold</b>") {
		t.Fatalf("expected bold outside code span, got %q", got)
	}
}

func TestRenderInlineUnmatchedDelimitersRemainLiteral(t *testing.T) {
	input := "a **b and _c and ~~d"
	got := RenderInline(input, InlineStyles{
		Bold:   func(s string) string { return "<b>" + s + "</b>" },
		Italic: func(s string) string { return "<i>" + s + "</i>" },
		Strike: func(s string) string { return "<s>" + s + "</s>" },
	})
	if got != input {
		t.Fatalf("expected unmatched delimiters to remain literal, got %q", got)
	}
}

func TestRenderInlineMixedMarkers(t *testing.T) {
	got := RenderInline("**Bold _and italic_** plus ~~strike~~", InlineStyles{
		Bold:   func(s string) string { return "<b>" + s + "</b>" },
		Italic: func(s string) string { return "<i>" + s + "</i>" },
		Strike: func(s string) string { return "<s>" + s + "</s>" },
	})
	if !strings.Contains(got, "<b>Bold <i>and italic</i></b>") {
		t.Fatalf("expected nested bold+italic output, got %q", got)
	}
	if !strings.Contains(got, "<s>strike</s>") {
		t.Fatalf("expected strike output, got %q", got)
	}
}

func TestRenderInlineLinkUsesRenderer(t *testing.T) {
	got := RenderInline("see [Docs](https://example.com)", InlineStyles{
		Link: func(label, url string) string {
			return "<a href=\"" + url + "\">" + label + "</a>"
		},
	})
	if !strings.Contains(got, "<a href=\"https://example.com\">Docs</a>") {
		t.Fatalf("expected link callback output, got %q", got)
	}
}

func TestRenderInlineLinkFallbackWhenRendererMissing(t *testing.T) {
	got := RenderInline("see [Docs](https://example.com)", InlineStyles{})
	if !strings.Contains(got, "Docs (https://example.com)") {
		t.Fatalf("expected fallback link rendering, got %q", got)
	}
}

func TestRenderInlineLinkAndEmphasis(t *testing.T) {
	got := RenderInline("**[Docs](https://example.com)**", InlineStyles{
		Link: func(label, url string) string {
			return label + " (" + url + ")"
		},
		Bold: func(s string) string { return "<b>" + s + "</b>" },
	})
	if !strings.Contains(got, "<b>Docs (https://example.com)</b>") {
		t.Fatalf("expected link inside bold rendering, got %q", got)
	}
}

func TestRenderInlineLinkInsideCodeStaysLiteral(t *testing.T) {
	got := RenderInline("`[Docs](https://example.com)`", InlineStyles{
		Code: func(s string) string { return "<code>" + s + "</code>" },
		Link: func(label, url string) string {
			return "<a>" + label + "</a>"
		},
	})
	if got != "<code>[Docs](https://example.com)</code>" {
		t.Fatalf("expected literal link markdown inside code, got %q", got)
	}
}

func TestRenderInlineMalformedLinksRemainLiteral(t *testing.T) {
	inputs := []string{
		"[Docs](",
		"[Docs]url)",
		"[](https://example.com)",
		"[Docs]()",
	}
	for _, input := range inputs {
		got := RenderInline(input, InlineStyles{
			Link: func(label, url string) string { return "X" },
		})
		if got != input {
			t.Fatalf("expected malformed link to remain literal. input=%q got=%q", input, got)
		}
	}
}

func TestParseMarkdownBlocksQuote(t *testing.T) {
	blocks := ParseMarkdownBlocks("> hello", false)
	if len(blocks) != 1 {
		t.Fatalf("expected single block, got %d", len(blocks))
	}
	if blocks[0].Kind != BlockQuote || blocks[0].Text != "hello" {
		t.Fatalf("expected quote block 'hello', got kind=%q text=%q", blocks[0].Kind, blocks[0].Text)
	}
}

func TestParseMarkdownBlocksListWithQuoteMarkerStaysList(t *testing.T) {
	blocks := ParseMarkdownBlocks("- > quoted", false)
	if len(blocks) != 1 {
		t.Fatalf("expected single block, got %d", len(blocks))
	}
	if blocks[0].Kind != BlockList {
		t.Fatalf("expected list block, got %q", blocks[0].Kind)
	}
}

func TestStreamingPlaceholderCallbackUsedForEmptyStreamingMessage(t *testing.T) {
	msg := domain.Message{Role: domain.RoleAssistant, Streaming: true}
	rendered := FormatMessageForTranscript(msg, Styles{
		StreamingPlaceholder: func() string { return ".." },
	})
	if rendered.Body != ".." {
		t.Fatalf("expected placeholder callback output, got %q", rendered.Body)
	}
}

func TestStreamingSuffixCallbackUsedForStreamingWithContent(t *testing.T) {
	msg := domain.Message{Role: domain.RoleAssistant, Content: "hello", Streaming: true}
	rendered := FormatMessageForTranscript(msg, Styles{
		StreamingSuffix: func() string { return "..." },
	})
	if !strings.Contains(rendered.Body, "hello\n...") {
		t.Fatalf("expected streaming suffix callback output, got %q", rendered.Body)
	}
}

func TestNoStreamingCallbacksOnFinalizedMessage(t *testing.T) {
	msg := domain.Message{Role: domain.RoleAssistant, Content: "done", Streaming: false}
	rendered := FormatMessageForTranscript(msg, Styles{
		StreamingPlaceholder: func() string { return "X" },
		StreamingSuffix:      func() string { return "Y" },
	})
	if rendered.Body != "done" {
		t.Fatalf("expected finalized body unchanged, got %q", rendered.Body)
	}
}
