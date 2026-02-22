package chat

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

type autoReviewState struct {
	active               bool
	cycle                int
	waitingForCompletion bool
}

type autoReviewResult struct {
	Approved bool
	Summary  string
}

type autoReviewRunner interface {
	ResolveBase(repoPath string) (baseSHA string, baseRef string, err error)
	Review(repoPath string, baseSHA string) (autoReviewResult, error)
}

type cliAutoReviewRunner struct {
	runCmd func(ctx context.Context, dir, name string, args ...string) (string, error)
}

func newCLIAutoReviewRunner() *cliAutoReviewRunner {
	return &cliAutoReviewRunner{runCmd: runCommandCapture}
}

func (r *cliAutoReviewRunner) ResolveBase(repoPath string) (string, string, error) {
	refs := []string{"origin/main", "origin/master"}
	for _, ref := range refs {
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		out, err := r.runCmd(ctx, repoPath, "git", "merge-base", "HEAD", ref)
		cancel()
		if err != nil {
			continue
		}
		sha := strings.TrimSpace(out)
		if sha != "" {
			return sha, ref, nil
		}
	}
	return "", "", fmt.Errorf("failed to resolve base with origin/main or origin/master")
}

func (r *cliAutoReviewRunner) Review(repoPath string, baseSHA string) (autoReviewResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	out, err := r.runCmd(ctx, repoPath, "codex", "review", fmt.Sprintf("%s...HEAD", strings.TrimSpace(baseSHA)))
	cancel()
	trimmed := strings.TrimSpace(out)
	if err != nil && trimmed == "" {
		return autoReviewResult{}, err
	}
	if trimmed == "" {
		trimmed = "Review requires changes."
	}
	return autoReviewResult{
		Approved: autoReviewApprovedFromOutput(trimmed),
		Summary:  trimmed,
	}, nil
}

func runCommandCapture(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func autoReviewApprovedFromOutput(output string) bool {
	lower := strings.ToLower(strings.TrimSpace(output))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "approved") ||
		strings.Contains(lower, "no comments") ||
		strings.Contains(lower, "no issues found")
}

func buildAutoReviewPrompt(baseRef, reviewSummary string) string {
	var b strings.Builder
	b.WriteString("Automatic review found comments. Please address all findings in the current branch, then continue normal verification and report completion with <DEVELOPMENT_WORK_COMPLETE>.\n\n")
	if strings.TrimSpace(baseRef) != "" {
		b.WriteString("Review base ref: ")
		b.WriteString(baseRef)
		b.WriteString("\n\n")
	}
	b.WriteString("Review comments:\n")
	b.WriteString(strings.TrimSpace(reviewSummary))
	return b.String()
}

func summarizeAutoReviewDetail(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	const max = 500
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

