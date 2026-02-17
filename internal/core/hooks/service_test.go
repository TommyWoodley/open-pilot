package hooks

import (
	"context"
	"os"
	"path/filepath"
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
	}, "")

	result := svc.Run(context.Background(), config.HookTriggerSessionStarted, "s-1", "")
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
	}, "")

	result := svc.Run(context.Background(), config.HookTriggerSessionStarted, "s-1", "")
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
	}, "")

	result := svc.Run(context.Background(), config.HookTriggerSessionStarted, "s-1", "")
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
	}, "")

	result := svc.Run(context.Background(), config.HookTriggerSessionStarted, "s-1", "")
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
	}, "")

	result := svc.Run(context.Background(), config.HookTriggerRepoAdded, "s-1", "/tmp/my-repo")
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
	svc := NewService(config.HookCatalog{}, "broken yaml")
	result := svc.Run(context.Background(), config.HookTriggerSessionStarted, "s-1", "")
	if result.Passed {
		t.Fatalf("expected fail-closed")
	}
	if !strings.Contains(result.Reason, "broken yaml") {
		t.Fatalf("expected load error in reason, got %q", result.Reason)
	}
}
