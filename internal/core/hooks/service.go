package hooks

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"time"

	"github.com/thwoodle/open-pilot/internal/config"
)

type Service interface {
	Run(ctx context.Context, trigger config.HookTrigger, sessionID, repoPath string) RunResult
}

type RunResult struct {
	Passed             bool
	HooksMatched       int
	FailedHookID       string
	FailedCommandIndex int
	Reason             string
	HookLoadError      string
	PerHookResults     []HookResult
	StartedAt          time.Time
	CompletedAt        time.Time
}

type HookResult struct {
	HookID string
	Passed bool
	Reason string
}

type runner struct {
	catalog   config.HookCatalog
	loadError string
}

func NewService(catalog config.HookCatalog, loadError string) Service {
	return &runner{
		catalog:   catalog,
		loadError: loadError,
	}
}

func (r *runner) Run(ctx context.Context, trigger config.HookTrigger, sessionID, repoPath string) RunResult {
	if ctx == nil {
		ctx = context.Background()
	}
	result := RunResult{
		Passed:         true,
		HookLoadError:  r.loadError,
		PerHookResults: make([]HookResult, 0),
		StartedAt:      time.Now(),
	}
	if r.loadError != "" {
		result.Passed = false
		result.Reason = "hook configuration error: " + r.loadError
		result.CompletedAt = time.Now()
		return result
	}

	hooks := r.catalog.HooksFor(trigger)
	result.HooksMatched = len(hooks)
	for _, hook := range hooks {
		hookResult := HookResult{HookID: hook.ID}
		for i, command := range hook.Execute {
			cmdCtx, cancel := context.WithTimeout(ctx, hook.Timeout)
			cmd := exec.CommandContext(cmdCtx, "bash", "-lc", command)
			cmd.Env = append(os.Environ(), runtimeEnv(sessionID, repoPath)...)
			cmd.Env = append(cmd.Env, envToList(hook.Env)...)
			if err := cmd.Run(); err != nil {
				ctxErr := cmdCtx.Err()
				cancel()
				hookResult.Passed = false
				switch {
				case errors.Is(ctxErr, context.DeadlineExceeded):
					hookResult.Reason = "timeout"
				case errors.Is(ctxErr, context.Canceled):
					hookResult.Reason = "cancelled"
				default:
					var exitErr *exec.ExitError
					if errors.As(err, &exitErr) {
						hookResult.Reason = fmt.Sprintf("exit=%d", exitErr.ExitCode())
					} else {
						hookResult.Reason = "start error"
					}
				}
				result.PerHookResults = append(result.PerHookResults, hookResult)
				result.Passed = false
				result.FailedHookID = hook.ID
				result.FailedCommandIndex = i + 1
				result.Reason = hookResult.Reason
				result.CompletedAt = time.Now()
				return result
			}
			cancel()
		}
		hookResult.Passed = true
		result.PerHookResults = append(result.PerHookResults, hookResult)
	}
	result.CompletedAt = time.Now()
	return result
}

func runtimeEnv(sessionID, repoPath string) []string {
	out := make([]string, 0, 2)
	if sessionID != "" {
		out = append(out, "OPEN_PILOT_SESSION_ID="+sessionID)
	}
	if repoPath != "" {
		out = append(out, "OPEN_PILOT_REPO_PATH="+repoPath)
	}
	return out
}

func envToList(env map[string]string) []string {
	if len(env) == 0 {
		return nil
	}
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(env))
	for _, k := range keys {
		v := env[k]
		out = append(out, k+"="+v)
	}
	return out
}
