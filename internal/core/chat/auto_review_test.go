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
	r := &cliAutoReviewRunner{
		runCmd: func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
			return "Approved: no comments", nil
		},
	}
	out, err := r.Review("/tmp/repo", "abc123")
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if !out.Approved {
		t.Fatalf("expected approved result")
	}
}

func TestReviewTreatsNonApprovedOutputAsComments(t *testing.T) {
	r := &cliAutoReviewRunner{
		runCmd: func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
			return "Line 42: possible bug", nil
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
	r.runCmd = func(_ context.Context, _ string, _ string, _ ...string) (string, error) {
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
