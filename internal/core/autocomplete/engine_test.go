package autocomplete

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestApplyRootCompletion(t *testing.T) {
	var e Engine
	out := e.Apply("/pro", Options{})
	if out != "/provider " {
		t.Fatalf("expected /provider completion, got %q", out)
	}
}

func TestSuggestionsIncludeSessionNames(t *testing.T) {
	var e Engine
	s := e.Suggestions("/session u", Options{SessionNames: []string{"test-session"}})
	found := false
	for _, v := range s {
		if v == "/session use test-session" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected session suggestion")
	}
}

func TestSuggestionsIncludeSessionDeleteByName(t *testing.T) {
	var e Engine
	s := e.Suggestions("/session d", Options{SessionNames: []string{"test-session"}})
	found := false
	for _, v := range s {
		if v == "/session delete test-session" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected session delete suggestion")
	}
}

func TestApplyHooksRunCompletion(t *testing.T) {
	var e Engine
	out := e.Apply("/hooks r", Options{})
	if out != "/hooks run " {
		t.Fatalf("expected /hooks run completion, got %q", out)
	}
}

func TestApplySessionAddRepoPathCompletionCaseInsensitiveAndShortestFirst(t *testing.T) {
	tmp := t.TempDir()
	entries := []string{"repo", "RepoAlpha", "repobeta"}
	for _, name := range entries {
		if err := os.Mkdir(filepath.Join(tmp, name), 0o755); err != nil {
			t.Fatalf("mkdir %q failed: %v", name, err)
		}
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	var e Engine
	out := e.Apply("/session add-repo rep", Options{})
	if out != "/session add-repo repo/" {
		t.Fatalf("expected shortest completion first, got %q", out)
	}
}

func TestApplySessionAddRepoPathCompletionDoesNotAppendSpace(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, "repo-alpha"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	var e Engine
	out := e.Apply("/session add-repo rep", Options{})
	if strings.HasSuffix(out, " ") {
		t.Fatalf("expected no trailing space for path completion, got %q", out)
	}
}

func TestSuggestionsSessionAddRepoPathCompletionCaseInsensitive(t *testing.T) {
	tmp := t.TempDir()
	if err := os.Mkdir(filepath.Join(tmp, "RepoAlpha"), 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	var e Engine
	s := e.Suggestions("/session add-repo rep", Options{})
	want := "/session add-repo RepoAlpha/"
	for _, v := range s {
		if v == want {
			return
		}
	}
	t.Fatalf("expected %q in suggestions, got %v", want, s)
}

func TestSuggestionsSessionAddRepoPathOrderMatchesApply(t *testing.T) {
	tmp := t.TempDir()
	parent := filepath.Join(tmp, "parent")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatalf("mkdir parent failed: %v", err)
	}
	for _, name := range []string{"z", "aaa"} {
		if err := os.Mkdir(filepath.Join(parent, name), 0o755); err != nil {
			t.Fatalf("mkdir %q failed: %v", name, err)
		}
	}
	child := filepath.Join(parent, "child")
	if err := os.Mkdir(child, 0o755); err != nil {
		t.Fatalf("mkdir child failed: %v", err)
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(child); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	var e Engine
	input := "/session add-repo ../"
	gotApply := e.Apply(input, Options{})
	gotSuggestions := e.Suggestions(input, Options{})
	if len(gotSuggestions) == 0 {
		t.Fatalf("expected non-empty suggestions")
	}
	expectedFirst := strings.TrimSuffix(gotApply, " ")
	if gotSuggestions[0] != expectedFirst {
		t.Fatalf("expected first suggestion %q to match first apply result %q; all suggestions=%v", gotSuggestions[0], expectedFirst, gotSuggestions)
	}
}

func TestPathCompletionOptionsHidesDotfilesUnlessPrefixStartsWithDot(t *testing.T) {
	tmp := t.TempDir()
	for _, name := range []string{".hidden", "visible"} {
		if err := os.Mkdir(filepath.Join(tmp, name), 0o755); err != nil {
			t.Fatalf("mkdir %q failed: %v", name, err)
		}
	}

	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})

	noDot := pathCompletionOptions("")
	for _, v := range noDot {
		if strings.Contains(v, ".hidden") {
			t.Fatalf("did not expect hidden entry without dot prefix, got %v", noDot)
		}
	}

	withDot := pathCompletionOptions(".")
	foundHidden := false
	for _, v := range withDot {
		if strings.Contains(v, ".hidden") {
			foundHidden = true
			break
		}
	}
	if !foundHidden {
		t.Fatalf("expected hidden entry with dot prefix, got %v", withDot)
	}
}
