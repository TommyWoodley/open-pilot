package config

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestLoadBuiltinHooksParsesValidFile(t *testing.T) {
	dir := t.TempDir()
	writeHookFile(t, dir, "a.yaml", `
version: 1
id: ensure-main-up-to-date
triggers:
  - session.started
execute:
  - git fetch --prune
timeout: 45s
env:
  GIT_TERMINAL_PROMPT: "0"
`)

	catalog, err := LoadBuiltinHooks(dir)
	if err != nil {
		t.Fatalf("expected load success, got error: %v", err)
	}
	if len(catalog.Hooks) != 1 {
		t.Fatalf("expected one hook, got %d", len(catalog.Hooks))
	}
	hook := catalog.Hooks[0]
	if hook.ID != "ensure-main-up-to-date" {
		t.Fatalf("unexpected hook id: %q", hook.ID)
	}
	if len(hook.Triggers) != 1 || hook.Triggers[0] != HookTriggerSessionStarted {
		t.Fatalf("unexpected triggers: %#v", hook.Triggers)
	}
	if len(hook.Execute) != 1 || hook.Execute[0] != "git fetch --prune" {
		t.Fatalf("unexpected execute: %#v", hook.Execute)
	}
	if hook.Timeout.String() != "45s" {
		t.Fatalf("unexpected timeout: %s", hook.Timeout)
	}
}

func TestLoadBuiltinHooksParsesRepoAddedTrigger(t *testing.T) {
	dir := t.TempDir()
	writeHookFile(t, dir, "a.yaml", `
version: 1
id: repo-added-hook
triggers:
  - repo.added
execute:
  - echo ok
`)

	catalog, err := LoadBuiltinHooks(dir)
	if err != nil {
		t.Fatalf("expected load success, got error: %v", err)
	}
	if len(catalog.Hooks) != 1 {
		t.Fatalf("expected one hook, got %d", len(catalog.Hooks))
	}
	if catalog.Hooks[0].Triggers[0] != HookTriggerRepoAdded {
		t.Fatalf("unexpected trigger: %q", catalog.Hooks[0].Triggers[0])
	}
}

func TestLoadBuiltinHooksParsesProviderCodexSelectedTrigger(t *testing.T) {
	dir := t.TempDir()
	writeHookFile(t, dir, "a.yaml", `
version: 1
id: provider-codex-hook
triggers:
  - provider.codex.selected
execute:
  - echo ok
`)

	catalog, err := LoadBuiltinHooks(dir)
	if err != nil {
		t.Fatalf("expected load success, got error: %v", err)
	}
	if len(catalog.Hooks) != 1 {
		t.Fatalf("expected one hook, got %d", len(catalog.Hooks))
	}
	if catalog.Hooks[0].Triggers[0] != HookTriggerProviderCodexSelected {
		t.Fatalf("unexpected trigger: %q", catalog.Hooks[0].Triggers[0])
	}
}

func TestLoadBuiltinHooksRejectsDuplicateID(t *testing.T) {
	dir := t.TempDir()
	content := `
version: 1
id: duplicate-id
triggers:
  - session.started
execute:
  - echo ok
`
	writeHookFile(t, dir, "a.yaml", content)
	writeHookFile(t, dir, "b.yaml", content)

	_, err := LoadBuiltinHooks(dir)
	if err == nil || !strings.Contains(err.Error(), "duplicate hook id") {
		t.Fatalf("expected duplicate id error, got: %v", err)
	}
}

func TestLoadBuiltinHooksRejectsUnsupportedTrigger(t *testing.T) {
	dir := t.TempDir()
	writeHookFile(t, dir, "a.yaml", `
version: 1
id: invalid-trigger
triggers:
  - prompt.before_send
execute:
  - echo ok
`)

	_, err := LoadBuiltinHooks(dir)
	if err == nil || !strings.Contains(err.Error(), "unsupported trigger") {
		t.Fatalf("expected unsupported trigger error, got: %v", err)
	}
}

func TestLoadBuiltinHooksRejectsMissingRequiredFields(t *testing.T) {
	dir := t.TempDir()
	writeHookFile(t, dir, "a.yaml", `
version: 1
id: missing-fields
`)

	_, err := LoadBuiltinHooks(dir)
	if err == nil || !strings.Contains(err.Error(), "triggers is required") {
		t.Fatalf("expected missing required fields error, got: %v", err)
	}
}

func TestLoadBuiltinHooksRejectsInvalidTimeout(t *testing.T) {
	dir := t.TempDir()
	writeHookFile(t, dir, "a.yaml", `
version: 1
id: bad-timeout
triggers:
  - session.started
execute:
  - echo ok
timeout: nope
`)

	_, err := LoadBuiltinHooks(dir)
	if err == nil || !strings.Contains(err.Error(), "invalid timeout") {
		t.Fatalf("expected invalid timeout error, got: %v", err)
	}
}

func TestLoadBuiltinHooksRejectsEmptyExecuteEntry(t *testing.T) {
	dir := t.TempDir()
	writeHookFile(t, dir, "a.yaml", `
version: 1
id: empty-command
triggers:
  - session.started
execute:
  -
`)

	_, err := LoadBuiltinHooks(dir)
	if err == nil || (!strings.Contains(err.Error(), "cannot be empty") && !strings.Contains(err.Error(), "expected list item")) {
		t.Fatalf("expected empty execute error, got: %v", err)
	}
}

func TestBuiltinHooksIncludeOpenDevelopmentBranch(t *testing.T) {
	dir := Default().BuiltinHooksDir
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("builtin hooks dir missing: %v", err)
	}
	catalog, err := LoadBuiltinHooks(dir)
	if err != nil {
		t.Fatalf("load builtin hooks: %v", err)
	}
	ids := make([]string, 0, len(catalog.Hooks))
	for _, h := range catalog.Hooks {
		ids = append(ids, h.ID)
	}
	if !slices.Contains(ids, "open-development-branch") {
		t.Fatalf("expected builtin hook id open-development-branch, got ids=%v", ids)
	}
}

func writeHookFile(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
		t.Fatalf("write hook file %s: %v", path, err)
	}
}
