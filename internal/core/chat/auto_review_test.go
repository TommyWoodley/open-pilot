package chat

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveBaseUsesOriginMainFirst(t *testing.T) {
	calls := 0
	r := &cliAutoReviewRunner{
		runCmd: func(_ context.Context, _ string, _ string, args ...string) (string, error) {
			calls++
			if len(args) != 3 || args[0] != "merge-base" || args[1] != "HEAD" || args[2] != "origin/main" {
				t.Fatalf("unexpected args: %#v", args)
			}
			return "abc123\n", nil
		},
	}
	base, ref, err := r.ResolveBase("/tmp/repo")
	if err != nil {
		t.Fatalf("resolve base: %v", err)
	}
	if base != "abc123" || ref != "origin/main" {
		t.Fatalf("unexpected base/ref: %q %q", base, ref)
	}
	if calls != 1 {
		t.Fatalf("expected one call, got %d", calls)
	}
}

func TestResolveBaseFallsBackToOriginMaster(t *testing.T) {
	calls := 0
	r := &cliAutoReviewRunner{
		runCmd: func(_ context.Context, _ string, _ string, args ...string) (string, error) {
			calls++
			if args[2] == "origin/main" {
				return "", errors.New("no main")
			}
			if args[2] == "origin/master" {
				return "def456\n", nil
			}
			t.Fatalf("unexpected args: %#v", args)
			return "", nil
		},
	}
	base, ref, err := r.ResolveBase("/tmp/repo")
	if err != nil {
		t.Fatalf("resolve base: %v", err)
	}
	if base != "def456" || ref != "origin/master" {
		t.Fatalf("unexpected base/ref: %q %q", base, ref)
	}
	if calls != 2 {
		t.Fatalf("expected two calls, got %d", calls)
	}
}

func TestResolveBaseFailsWhenNoBaseFound(t *testing.T) {
	r := &cliAutoReviewRunner{
		runCmd: func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
			return "", errors.New("missing")
		},
	}
	_, _, err := r.ResolveBase("/tmp/repo")
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestReviewTreatsApprovedOutputAsApproved(t *testing.T) {
	calls := 0
	r := &cliAutoReviewRunner{
		runCmd: func(_ context.Context, _ string, name string, args ...string) (string, error) {
			calls++
			if name == "git" {
				if len(args) >= 2 && args[0] == "diff" && args[1] == "--quiet" {
					return "", &fakeExitError{code: 1}
				}
				return "", nil
			}
			if name == "codex" {
				return "Approved: no comments", nil
			}
			t.Fatalf("unexpected command: %s %#v", name, args)
			return "", nil
		},
	}
	out, err := r.Review("/tmp/repo", "abc123")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !out.Approved {
		t.Fatalf("expected approved result")
	}
	if calls == 0 {
		t.Fatalf("expected review commands to run")
	}
}

func TestReviewTreatsNonApprovedOutputAsComments(t *testing.T) {
	r := &cliAutoReviewRunner{
		runCmd: func(_ context.Context, _ string, name string, args ...string) (string, error) {
			if name == "git" {
				if len(args) >= 2 && args[0] == "diff" && args[1] == "--quiet" {
					return "", &fakeExitError{code: 1}
				}
				return "", nil
			}
			if name == "codex" {
				return "Line 42: possible bug", nil
			}
			t.Fatalf("unexpected command: %s %#v", name, args)
			return "", nil
		},
	}
	out, err := r.Review("/tmp/repo", "abc123")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if out.Approved {
		t.Fatalf("expected comments-needed result")
	}
}

func TestReviewWritesDebugLog(t *testing.T) {
	logPath := filepath.Join(t.TempDir(), "codex-debug.log")
	t.Setenv("OPEN_PILOT_CODEX_DEBUG_LOG", logPath)

	r := newCLIAutoReviewRunner()
	r.runCmd = func(_ context.Context, _ string, name string, args ...string) (string, error) {
		if name == "git" {
			if len(args) >= 2 && args[0] == "diff" && args[1] == "--quiet" {
				return "", &fakeExitError{code: 1}
			}
			return "", nil
		}
		return "line one\nline two\n", nil
	}

	if _, err := r.Review("/tmp/repo", "abc123"); err != nil {
		t.Fatalf("review: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	text := string(data)
	if text == "" {
		t.Fatalf("expected log output")
	}
	if !containsAll(text, "auto-review run", "codex review abc123...HEAD", "line one", "line two") {
		t.Fatalf("expected review command and output in log, got %q", text)
	}
}

func TestReviewUsesWorkingTreeWhenNoCommittedDiff(t *testing.T) {
	commands := make([]string, 0, 8)
	r := &cliAutoReviewRunner{
		runCmd: func(_ context.Context, _ string, name string, args ...string) (string, error) {
			commands = append(commands, name+" "+strings.Join(args, " "))
			if name == "git" {
				if len(args) >= 2 && args[0] == "diff" && args[1] == "--quiet" {
					return "", nil
				}
				if len(args) >= 4 && args[0] == "diff" && args[1] == "--name-only" && args[2] == "HEAD" {
					return "internal/core/chat/auto_review.go\n", nil
				}
				if len(args) >= 2 && args[0] == "ls-files" && args[1] == "--others" {
					return "", nil
				}
			}
			if name == "codex" {
				if len(args) != 2 || args[0] != "review" || args[1] != "--uncommitted" {
					t.Fatalf("expected codex working-tree review args with --uncommitted, got %#v", args)
				}
				return "no issues found", nil
			}
			t.Fatalf("unexpected command: %s %#v", name, args)
			return "", nil
		},
	}

	out, err := r.Review("/tmp/repo", "abc123")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !out.Approved {
		t.Fatalf("expected approved result, got %#v", out)
	}
	if !containsAll(strings.Join(commands, "\n"), "git diff --quiet abc123...HEAD", "codex review --uncommitted") {
		t.Fatalf("missing expected commands: %v", commands)
	}
}

func TestReviewSkipsGitignoreOnlyChanges(t *testing.T) {
	r := &cliAutoReviewRunner{
		runCmd: func(_ context.Context, _ string, name string, args ...string) (string, error) {
			if name == "git" {
				if len(args) >= 2 && args[0] == "diff" && args[1] == "--quiet" {
					return "", nil
				}
				if len(args) >= 4 && args[0] == "diff" && args[1] == "--name-only" && args[2] == "HEAD" {
					return "", nil
				}
				if len(args) >= 2 && args[0] == "ls-files" && args[1] == "--others" {
					return ".gitignore\n", nil
				}
			}
			if name == "codex" {
				t.Fatalf("did not expect codex review to run for .gitignore-only changes")
			}
			t.Fatalf("unexpected command: %s %#v", name, args)
			return "", nil
		},
	}

	out, err := r.Review("/tmp/repo", "abc123")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !out.Approved {
		t.Fatalf("expected approved for no reviewable changes")
	}
	if out.Summary == "" {
		t.Fatalf("expected terminal summary for no reviewable changes")
	}
}

func TestAutoReviewApprovedFromOutputTreatsNoChangePhrasesAsApproved(t *testing.T) {
	cases := []string{
		"There are no code changes in the provided range.",
		"There is nothing to flag as a regression.",
		"Nothing to review in this diff.",
		"No reviewable changes found.",
	}
	for _, text := range cases {
		if !autoReviewApprovedFromOutput(text) {
			t.Fatalf("expected approved for %q", text)
		}
	}
}

type fakeExitError struct {
	code int
}

func (e *fakeExitError) Error() string {
	return "exit status"
}

func (e *fakeExitError) ExitCode() int {
	return e.code
}

func containsAll(haystack string, needles ...string) bool {
	for _, needle := range needles {
		if needle == "" {
			continue
		}
		if !strings.Contains(haystack, needle) {
			return false
		}
	}
	return true
}
