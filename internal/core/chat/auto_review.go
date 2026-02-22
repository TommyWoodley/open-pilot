package chat

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type autoReviewState struct {
	active               bool
	runID                int
	cycle                int
	running              bool
	waitingForCompletion bool
	progressLines        []string
}

type autoReviewResult struct {
	Approved bool
	Summary  string
}

type autoReviewRunner interface {
	ResolveBase(repoPath string) (baseSHA string, baseRef string, err error)
	Review(repoPath string, baseSHA string, onOutput func(string)) (autoReviewResult, error)
}

type cliAutoReviewRunner struct {
	runCmd       func(ctx context.Context, dir, name string, args ...string) (string, error)
	runReviewCmd func(ctx context.Context, dir, name string, args []string, onLine func(string)) (string, error)
	logf         func(format string, args ...any)
}

func newCLIAutoReviewRunner() *cliAutoReviewRunner {
	return &cliAutoReviewRunner{
		runCmd:       runCommandCapture,
		runReviewCmd: runCommandStream,
		logf:         logAutoReviewf,
	}
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

func (r *cliAutoReviewRunner) Review(repoPath string, baseSHA string, onOutput func(string)) (autoReviewResult, error) {
	revision := fmt.Sprintf("%s...HEAD", strings.TrimSpace(baseSHA))
	args := []string{"review", revision}
	commandLabel := "codex review " + revision
	reviewable, err := r.hasReviewableDiff(repoPath, revision)
	if err != nil {
		return autoReviewResult{}, err
	}
	if !reviewable {
		args = []string{"review", "--uncommitted"}
		commandLabel = "codex review --uncommitted"
		hasWorkingTree, err := r.hasReviewableWorkingTreeChanges(repoPath)
		if err != nil {
			return autoReviewResult{}, err
		}
		if !hasWorkingTree {
			return autoReviewResult{
				Approved: true,
				Summary:  "No reviewable changes found.",
			}, nil
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	if r.logf != nil {
		r.logf("auto-review run repo=%s command=%q", repoPath, commandLabel)
	}
	streamLine := func(line string) {
		if strings.TrimSpace(line) == "" {
			return
		}
		if onOutput != nil {
			onOutput(line)
		}
		if r.logf != nil {
			r.logf("auto-review output %s", line)
		}
	}
	var out string
	if r.runReviewCmd != nil {
		out, err = r.runReviewCmd(ctx, repoPath, "codex", args, streamLine)
	} else {
		out, err = r.runCmd(ctx, repoPath, "codex", args...)
		scanner := bufio.NewScanner(strings.NewReader(out))
		for scanner.Scan() {
			streamLine(scanner.Text())
		}
	}
	cancel()
	if r.logf != nil {
		r.logf("auto-review done err=%v", err)
	}
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

func (r *cliAutoReviewRunner) hasReviewableDiff(repoPath, revision string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	_, err := r.runCmd(ctx, repoPath, "git", "diff", "--quiet", revision)
	cancel()
	if err == nil {
		return false, nil
	}
	exitCode, ok := commandExitCode(err)
	if ok && exitCode == 1 {
		return true, nil
	}
	return false, err
}

func (r *cliAutoReviewRunner) hasReviewableWorkingTreeChanges(repoPath string) (bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	trackedOut, err := r.runCmd(ctx, repoPath, "git", "diff", "--name-only", "HEAD", "--", ".", ":(exclude).gitignore")
	cancel()
	if err != nil {
		return false, err
	}
	if outputHasReviewablePaths(trackedOut) {
		return true, nil
	}

	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Second)
	untrackedOut, err := r.runCmd(ctx, repoPath, "git", "ls-files", "--others", "--exclude-standard", "--", ".")
	cancel()
	if err != nil {
		return false, err
	}
	return outputHasReviewablePaths(untrackedOut), nil
}

func outputHasReviewablePaths(output string) bool {
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if filepath.Base(line) == ".gitignore" {
			continue
		}
		return true
	}
	return false
}

func commandExitCode(err error) (int, bool) {
	if err == nil {
		return 0, false
	}
	var exitCoder interface{ ExitCode() int }
	if errors.As(err, &exitCoder) {
		return exitCoder.ExitCode(), true
	}
	return 0, false
}

func runCommandCapture(ctx context.Context, dir, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runCommandStream(ctx context.Context, dir, name string, args []string, onLine func(string)) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", err
	}
	if err := cmd.Start(); err != nil {
		return "", err
	}

	lineCh := make(chan string, 32)
	var wg sync.WaitGroup
	var readErrOnce sync.Once
	var readErr error

	readPipe := func(r io.Reader) {
		defer wg.Done()
		scanner := bufio.NewScanner(r)
		buf := make([]byte, 0, 64*1024)
		scanner.Buffer(buf, 1024*1024)
		for scanner.Scan() {
			lineCh <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			readErrOnce.Do(func() {
				readErr = err
			})
		}
	}

	wg.Add(2)
	go readPipe(stdout)
	go readPipe(stderr)
	go func() {
		wg.Wait()
		close(lineCh)
	}()

	var b strings.Builder
	first := true
	for line := range lineCh {
		if !first {
			b.WriteByte('\n')
		}
		first = false
		b.WriteString(line)
		if onLine != nil {
			onLine(line)
		}
	}

	waitErr := cmd.Wait()
	if readErr != nil {
		return strings.TrimSpace(b.String()), readErr
	}
	return strings.TrimSpace(b.String()), waitErr
}

func autoReviewApprovedFromOutput(output string) bool {
	lower := strings.ToLower(strings.TrimSpace(output))
	if lower == "" {
		return false
	}
	return strings.Contains(lower, "approved") ||
		strings.Contains(lower, "no comments") ||
		strings.Contains(lower, "no issues found") ||
		strings.Contains(lower, "no code changes") ||
		strings.Contains(lower, "nothing to flag") ||
		strings.Contains(lower, "nothing to review") ||
		strings.Contains(lower, "no reviewable changes")
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
	return text
}

var autoReviewLogMu sync.Mutex

func logAutoReviewf(format string, args ...any) {
	path := strings.TrimSpace(os.Getenv("OPEN_PILOT_CODEX_DEBUG_LOG"))
	if path == "" {
		path = filepath.Join(os.TempDir(), "open-pilot-codex-debug.log")
	}

	autoReviewLogMu.Lock()
	defer autoReviewLogMu.Unlock()

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()

	line := fmt.Sprintf(format, args...)
	_, _ = fmt.Fprintf(f, "%s [auto-review] %s\n", time.Now().UTC().Format(time.RFC3339Nano), line)
}
