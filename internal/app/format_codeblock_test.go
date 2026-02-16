package app

import (
	"strings"
	"testing"
)

func TestRenderCodeBlockPreservesContentAndLang(t *testing.T) {
	t.Parallel()

	block := mdBlock{Kind: mdBlockCode, Lang: "go", Text: "if x {\n    y()\n}"}
	rendered := renderCodeBlock(block)

	if !strings.Contains(rendered, "[go]") {
		t.Fatalf("expected rendered code block to include lang label")
	}
	if !strings.Contains(rendered, "    y()") {
		t.Fatalf("expected indentation to be preserved in code block")
	}
}

func TestRenderInlineCode(t *testing.T) {
	t.Parallel()

	raw := "use `fmt.Println` here"
	rendered := renderInlineCode(raw)
	if rendered == raw {
		t.Fatalf("expected inline code styling to change output")
	}
	if !strings.Contains(rendered, "fmt.Println") {
		t.Fatalf("expected inline code content preserved")
	}
}
