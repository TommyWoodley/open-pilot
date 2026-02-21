package hooks

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/thwoodle/open-pilot/internal/config"
)

func TestRunExecutesCommandsInOrder(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	svc := NewService(config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "ordered",
				Triggers: []config.HookTrigger{config.HookTriggerSessionStarted},
				Execute: []string{
					"echo first > " + target,
					"echo second >> " + target,
				},
				Timeout: time.Second,
			},
		},
	}, "", "")

	result := svc.Run(context.Background(), config.HookTriggerSessionStarted, "s-1", "", nil)
	if !result.Passed {
		t.Fatalf("expected pass, got failure: %#v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read output file: %v", err)
	}
	got := strings.TrimSpace(string(data))
	if got != "first\nsecond" {
		t.Fatalf("unexpected command order output: %q", got)
	}
}

func TestRunStopsOnNonZeroExit(t *testing.T) {
	svc := NewService(config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "fail",
				Triggers: []config.HookTrigger{config.HookTriggerSessionStarted},
				Execute:  []string{"exit 7"},
				Timeout:  time.Second,
			},
		},
	}, "", "")

	result := svc.Run(context.Background(), config.HookTriggerSessionStarted, "s-1", "", nil)
	if result.Passed {
		t.Fatalf("expected failure")
	}
	if result.FailedHookID != "fail" || result.FailedCommandIndex != 1 {
		t.Fatalf("unexpected failure location: %#v", result)
	}
	if !strings.Contains(result.Reason, "exit=") {
		t.Fatalf("expected exit reason, got %q", result.Reason)
	}
}

func TestRunReturnsTimeoutReason(t *testing.T) {
	svc := NewService(config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "timeout",
				Triggers: []config.HookTrigger{config.HookTriggerSessionStarted},
				Execute:  []string{"sleep 1"},
				Timeout:  10 * time.Millisecond,
			},
		},
	}, "", "")

	result := svc.Run(context.Background(), config.HookTriggerSessionStarted, "s-1", "", nil)
	if result.Passed {
		t.Fatalf("expected timeout failure")
	}
	if result.Reason != "timeout" {
		t.Fatalf("expected timeout reason, got %q", result.Reason)
	}
}

func TestRunSetsEnvOnCommand(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "env.txt")
	svc := NewService(config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "env",
				Triggers: []config.HookTrigger{config.HookTriggerSessionStarted},
				Execute:  []string{"echo $TEST_HOOK_ENV > " + target},
				Timeout:  time.Second,
				Env: map[string]string{
					"TEST_HOOK_ENV": "works",
				},
			},
		},
	}, "", "")

	result := svc.Run(context.Background(), config.HookTriggerSessionStarted, "s-1", "", nil)
	if !result.Passed {
		t.Fatalf("expected pass, got %#v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read env output: %v", err)
	}
	if strings.TrimSpace(string(data)) != "works" {
		t.Fatalf("expected env value, got %q", string(data))
	}
}

func TestRunSetsRepoPathEnvOnCommand(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "repo.txt")
	svc := NewService(config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "repo-env",
				Triggers: []config.HookTrigger{config.HookTriggerRepoAdded},
				Execute:  []string{"echo $OPEN_PILOT_REPO_PATH > " + target},
				Timeout:  time.Second,
			},
		},
	}, "", "")

	result := svc.Run(context.Background(), config.HookTriggerRepoAdded, "s-1", "/tmp/my-repo", nil)
	if !result.Passed {
		t.Fatalf("expected pass, got %#v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read repo output: %v", err)
	}
	if strings.TrimSpace(string(data)) != "/tmp/my-repo" {
		t.Fatalf("expected repo path env, got %q", string(data))
	}
}

func TestRunFailsClosedOnLoadError(t *testing.T) {
	svc := NewService(config.HookCatalog{}, "broken yaml", "")
	result := svc.Run(context.Background(), config.HookTriggerSessionStarted, "s-1", "", nil)
	if result.Passed {
		t.Fatalf("expected fail-closed")
	}
	if !strings.Contains(result.Reason, "broken yaml") {
		t.Fatalf("expected load error in reason, got %q", result.Reason)
	}
}

func TestRunSetsBuiltinSkillsDirEnvOnCommand(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "skills-dir.txt")
	svc := NewService(config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "skills-dir-env",
				Triggers: []config.HookTrigger{config.HookTriggerProviderCodexSelected},
				Execute:  []string{"echo $OPEN_PILOT_BUILTIN_SKILLS_DIR > " + target},
				Timeout:  time.Second,
			},
		},
	}, "", "/tmp/open-pilot/skills/builtin")

	result := svc.Run(context.Background(), config.HookTriggerProviderCodexSelected, "s-1", "", nil)
	if !result.Passed {
		t.Fatalf("expected pass, got %#v", result)
	}
	data, err := os.ReadFile(target)
	if err != nil {
		t.Fatalf("read builtin skills dir output: %v", err)
	}
	if strings.TrimSpace(string(data)) != "/tmp/open-pilot/skills/builtin" {
		t.Fatalf("expected builtin skills dir env, got %q", string(data))
	}
}

func TestProviderCodexSelectedHookReplacesExistingSkills(t *testing.T) {
	work := t.TempDir()
	sourceRoot := filepath.Join(work, "skills", "builtin")
	destRoot := filepath.Join(work, "home", ".codex", "skills")
	sourceSkillDir := filepath.Join(sourceRoot, "my-skill")
	destSkillDir := filepath.Join(destRoot, "my-skill")
	if err := os.MkdirAll(sourceSkillDir, 0o755); err != nil {
		t.Fatalf("mkdir source skill: %v", err)
	}
	if err := os.MkdirAll(destSkillDir, 0o755); err != nil {
		t.Fatalf("mkdir dest skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sourceSkillDir, "SKILL.md"), []byte("new"), 0o644); err != nil {
		t.Fatalf("write source skill file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(destSkillDir, "stale.txt"), []byte("stale"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	command := "src=\"$OPEN_PILOT_BUILTIN_SKILLS_DIR\"; [ -n \"$src\" ] || exit 1; mkdir -p \"$src\"; dest=\"$HOME/.codex/skills\"; mkdir -p \"$dest\"; for d in \"$src\"/*; do [ -d \"$d\" ] || continue; skill_name=\"$(basename \"$d\")\"; rm -rf \"$dest/$skill_name\" || exit 1; cp -a \"$d\" \"$dest/$skill_name\" || exit 1; done"
	svc := NewService(config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "install-skills",
				Triggers: []config.HookTrigger{config.HookTriggerProviderCodexSelected},
				Execute:  []string{command},
				Timeout:  time.Second * 5,
			},
		},
	}, "", sourceRoot)

	t.Setenv("HOME", filepath.Join(work, "home"))
	result := svc.Run(context.Background(), config.HookTriggerProviderCodexSelected, "s-1", "", nil)
	if !result.Passed {
		t.Fatalf("expected pass, got %#v", result)
	}
	if _, err := os.Stat(filepath.Join(destSkillDir, "SKILL.md")); err != nil {
		t.Fatalf("expected copied skill file, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(destSkillDir, "stale.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected stale file to be removed, got err=%v", err)
	}
}

func TestRepoAddedOpenDevelopmentBranchCreatesNormalizedBranchFromSyncedBase(t *testing.T) {
	work := t.TempDir()
	remote := filepath.Join(work, "remote.git")
	seed := filepath.Join(work, "seed")
	repo := filepath.Join(work, "repo")

	runGit(t, work, "init", "--bare", remote)
	runGit(t, work, "clone", remote, seed)
	runGit(t, seed, "config", "user.email", "test@example.com")
	runGit(t, seed, "config", "user.name", "Test User")
	runGit(t, seed, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "seed v1")
	runGit(t, seed, "push", "-u", "origin", "main")

	runGit(t, work, "clone", remote, repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "checkout", "-b", "main", "origin/main")

	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("write seed update: %v", err)
	}
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "seed v2")
	runGit(t, seed, "push", "origin", "main")

	script := scriptPath(t)
	svc := NewService(config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "open-development-branch",
				Triggers: []config.HookTrigger{config.HookTriggerRepoAdded},
				Execute:  []string{"bash " + shellQuote(script)},
				Timeout:  time.Second * 10,
			},
		},
	}, "", "")

	result := svc.Run(context.Background(), config.HookTriggerRepoAdded, "Feature 123/ABC", repo, nil)
	if !result.Passed {
		t.Fatalf("expected pass, got %#v", result)
	}
	if got := runGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature-123-abc" {
		t.Fatalf("expected normalized session branch checked out, got %q", got)
	}
	branchTip := runGit(t, repo, "rev-parse", "feature-123-abc")
	originTip := runGit(t, repo, "rev-parse", "origin/main")
	if branchTip != originTip {
		t.Fatalf("expected new branch from latest origin/main, branch=%s origin/main=%s", branchTip, originTip)
	}
}

func TestRepoAddedOpenDevelopmentBranchUsesMasterWhenMainMissing(t *testing.T) {
	work := t.TempDir()
	remote := filepath.Join(work, "remote.git")
	seed := filepath.Join(work, "seed")
	repo := filepath.Join(work, "repo")

	runGit(t, work, "init", "--bare", remote)
	runGit(t, work, "clone", remote, seed)
	runGit(t, seed, "config", "user.email", "test@example.com")
	runGit(t, seed, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("master\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "seed master")
	runGit(t, seed, "push", "-u", "origin", "master")

	runGit(t, work, "clone", remote, repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")

	script := scriptPath(t)
	svc := NewService(config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "open-development-branch",
				Triggers: []config.HookTrigger{config.HookTriggerRepoAdded},
				Execute:  []string{"bash " + shellQuote(script)},
				Timeout:  time.Second * 10,
			},
		},
	}, "", "")

	result := svc.Run(context.Background(), config.HookTriggerRepoAdded, "Master Session", repo, nil)
	if !result.Passed {
		t.Fatalf("expected pass, got %#v", result)
	}
	if got := runGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "master-session" {
		t.Fatalf("expected master-session checked out, got %q", got)
	}
	branchTip := runGit(t, repo, "rev-parse", "master-session")
	originTip := runGit(t, repo, "rev-parse", "origin/master")
	if branchTip != originTip {
		t.Fatalf("expected new branch from latest origin/master, branch=%s origin/master=%s", branchTip, originTip)
	}
}

func TestRepoAddedOpenDevelopmentBranchExistingSessionBranchSyncsUpstreamOnly(t *testing.T) {
	work := t.TempDir()
	remote := filepath.Join(work, "remote.git")
	seed := filepath.Join(work, "seed")
	repo := filepath.Join(work, "repo")

	runGit(t, work, "init", "--bare", remote)
	runGit(t, work, "clone", remote, seed)
	runGit(t, seed, "config", "user.email", "test@example.com")
	runGit(t, seed, "config", "user.name", "Test User")
	runGit(t, seed, "checkout", "-b", "main")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("base\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "base")
	runGit(t, seed, "push", "-u", "origin", "main")

	runGit(t, work, "clone", remote, repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "checkout", "-b", "main", "origin/main")
	runGit(t, repo, "checkout", "-b", "feature-123")
	if err := os.WriteFile(filepath.Join(repo, "feature.txt"), []byte("local feature\n"), 0o644); err != nil {
		t.Fatalf("write local feature file: %v", err)
	}
	runGit(t, repo, "add", "feature.txt")
	runGit(t, repo, "commit", "-m", "feature start")
	runGit(t, repo, "push", "-u", "origin", "feature-123")

	runGit(t, seed, "fetch", "origin")
	runGit(t, seed, "checkout", "-b", "feature-123", "origin/feature-123")
	if err := os.WriteFile(filepath.Join(seed, "feature.txt"), []byte("remote feature update\n"), 0o644); err != nil {
		t.Fatalf("write seed feature update: %v", err)
	}
	runGit(t, seed, "add", "feature.txt")
	runGit(t, seed, "commit", "-m", "feature update")
	runGit(t, seed, "push", "origin", "feature-123")

	runGit(t, seed, "checkout", "main")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("main moved\n"), 0o644); err != nil {
		t.Fatalf("write seed main update: %v", err)
	}
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "main update")
	runGit(t, seed, "push", "origin", "main")

	script := scriptPath(t)
	svc := NewService(config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "open-development-branch",
				Triggers: []config.HookTrigger{config.HookTriggerRepoAdded},
				Execute:  []string{"bash " + shellQuote(script)},
				Timeout:  time.Second * 10,
			},
		},
	}, "", "")

	result := svc.Run(context.Background(), config.HookTriggerRepoAdded, "Feature 123", repo, nil)
	if !result.Passed {
		t.Fatalf("expected pass, got %#v", result)
	}
	if got := runGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "feature-123" {
		t.Fatalf("expected feature-123 checked out, got %q", got)
	}
	featureTip := runGit(t, repo, "rev-parse", "feature-123")
	originFeatureTip := runGit(t, repo, "rev-parse", "origin/feature-123")
	originMainTip := runGit(t, repo, "rev-parse", "origin/main")
	if featureTip != originFeatureTip {
		t.Fatalf("expected feature-123 to match upstream tip, local=%s upstream=%s", featureTip, originFeatureTip)
	}
	if featureTip == originMainTip {
		t.Fatalf("expected feature branch not to be forced to main tip")
	}
}

func TestRepoAddedOpenDevelopmentBranchNoRemoteIsNoop(t *testing.T) {
	repo := t.TempDir()
	runGit(t, repo, "init")
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("local\n"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	runGit(t, repo, "add", "README.md")
	runGit(t, repo, "commit", "-m", "local")

	script := scriptPath(t)
	svc := NewService(config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:         "open-development-branch",
				SourcePath: "/tmp/open-development-branch.yaml",
				Triggers:   []config.HookTrigger{config.HookTriggerRepoAdded},
				Execute:    []string{"bash " + shellQuote(script)},
				Timeout:    time.Second * 10,
			},
		},
	}, "", "")

	result := svc.Run(context.Background(), config.HookTriggerRepoAdded, "No Remote Session", repo, nil)
	if !result.Passed {
		t.Fatalf("expected pass/noop, got %#v", result)
	}
	if got := runGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "master" && got != "main" {
		t.Fatalf("expected to remain on initial branch, got %q", got)
	}
}

func TestRepoAddedOpenDevelopmentBranchNoMainOrMasterIsNoop(t *testing.T) {
	work := t.TempDir()
	remote := filepath.Join(work, "remote.git")
	seed := filepath.Join(work, "seed")
	repo := filepath.Join(work, "repo")

	runGit(t, work, "init", "--bare", remote)
	runGit(t, work, "clone", remote, seed)
	runGit(t, seed, "config", "user.email", "test@example.com")
	runGit(t, seed, "config", "user.name", "Test User")
	runGit(t, seed, "checkout", "-b", "develop")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("develop\n"), 0o644); err != nil {
		t.Fatalf("write seed file: %v", err)
	}
	runGit(t, seed, "add", "README.md")
	runGit(t, seed, "commit", "-m", "develop")
	runGit(t, seed, "push", "-u", "origin", "develop")

	runGit(t, work, "clone", remote, repo)
	runGit(t, repo, "config", "user.email", "test@example.com")
	runGit(t, repo, "config", "user.name", "Test User")
	runGit(t, repo, "checkout", "-b", "develop", "origin/develop")

	script := scriptPath(t)
	svc := NewService(config.HookCatalog{
		Hooks: []config.HookDefinition{
			{
				ID:       "open-development-branch",
				Triggers: []config.HookTrigger{config.HookTriggerRepoAdded},
				Execute:  []string{"bash " + shellQuote(script)},
				Timeout:  time.Second * 10,
			},
		},
	}, "", "")

	result := svc.Run(context.Background(), config.HookTriggerRepoAdded, "Develop Session", repo, nil)
	if !result.Passed {
		t.Fatalf("expected pass/noop, got %#v", result)
	}
	if got := runGit(t, repo, "rev-parse", "--abbrev-ref", "HEAD"); got != "develop" {
		t.Fatalf("expected develop to remain checked out, got %q", got)
	}
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	fullArgs := append([]string{"-c", "commit.gpgsign=false"}, args...)
	cmd := exec.Command("git", fullArgs...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s failed in %s: %v\n%s", strings.Join(args, " "), dir, err, string(out))
	}
	return strings.TrimSpace(string(out))
}

func scriptPath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("resolve caller path")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", ".."))
	return filepath.Join(root, "hooks", "scripts", "open-development-branch.sh")
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}
