package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const defaultHookTimeout = 30 * time.Second

type HookTrigger string

const HookTriggerSessionStarted HookTrigger = "session.started"
const HookTriggerRepoAdded HookTrigger = "repo.added"
const HookTriggerProviderCodexSelected HookTrigger = "provider.codex.selected"

type HookDefinition struct {
	Version     int
	ID          string
	Description string
	Triggers    []HookTrigger
	Execute     []string
	Timeout     time.Duration
	Env         map[string]string
	SourcePath  string
}

type HookCatalog struct {
	Hooks []HookDefinition
}

func (c HookCatalog) HooksFor(trigger HookTrigger) []HookDefinition {
	out := make([]HookDefinition, 0, len(c.Hooks))
	for _, hook := range c.Hooks {
		for _, t := range hook.Triggers {
			if t == trigger {
				out = append(out, hook)
				break
			}
		}
	}
	return out
}

func LoadBuiltinHooks(dir string) (HookCatalog, error) {
	if strings.TrimSpace(dir) == "" {
		return HookCatalog{}, fmt.Errorf("hooks directory is required")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return HookCatalog{}, fmt.Errorf("read hooks directory: %w", err)
	}

	catalog := HookCatalog{Hooks: make([]HookDefinition, 0)}
	seen := make(map[string]string)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		path := filepath.Join(dir, name)
		hook, err := loadHookFile(path)
		if err != nil {
			return HookCatalog{}, err
		}
		if prev, ok := seen[hook.ID]; ok {
			return HookCatalog{}, fmt.Errorf("duplicate hook id %q in %s and %s", hook.ID, prev, path)
		}
		seen[hook.ID] = path
		catalog.Hooks = append(catalog.Hooks, hook)
	}
	return catalog, nil
}

func loadHookFile(path string) (HookDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return HookDefinition{}, fmt.Errorf("read hook file %s: %w", path, err)
	}
	hook, err := parseHookYAML(string(data))
	if err != nil {
		return HookDefinition{}, fmt.Errorf("parse hook file %s: %w", path, err)
	}
	hook.SourcePath = path
	if err := validateHook(hook); err != nil {
		return HookDefinition{}, fmt.Errorf("invalid hook file %s: %w", path, err)
	}
	return hook, nil
}

func validateHook(hook HookDefinition) error {
	if hook.Version != 1 {
		return fmt.Errorf("version must be 1")
	}
	if strings.TrimSpace(hook.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if len(hook.Triggers) == 0 {
		return fmt.Errorf("triggers is required")
	}
	for _, trigger := range hook.Triggers {
		if trigger != HookTriggerSessionStarted && trigger != HookTriggerRepoAdded && trigger != HookTriggerProviderCodexSelected {
			return fmt.Errorf("unsupported trigger %q", trigger)
		}
	}
	if len(hook.Execute) == 0 {
		return fmt.Errorf("execute is required")
	}
	for i := range hook.Execute {
		if strings.TrimSpace(hook.Execute[i]) == "" {
			return fmt.Errorf("execute[%d] cannot be empty", i)
		}
	}
	return nil
}

func parseHookYAML(raw string) (HookDefinition, error) {
	lines := strings.Split(raw, "\n")
	hook := HookDefinition{Env: map[string]string{}}
	var section string

	for idx, line := range lines {
		lineNo := idx + 1
		line = strings.TrimRight(line, "\r")
		if strings.TrimSpace(line) == "" {
			continue
		}
		if strings.Contains(line, "\t") {
			return HookDefinition{}, fmt.Errorf("line %d: tabs are not supported", lineNo)
		}

		indent := countLeadingSpaces(line)
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			continue
		}

		if indent == 0 {
			section = ""
			key, value, hasValue, err := parseTopLevelLine(trimmed)
			if err != nil {
				return HookDefinition{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
			switch key {
			case "version":
				n, err := strconv.Atoi(unquoteScalar(value))
				if err != nil {
					return HookDefinition{}, fmt.Errorf("line %d: invalid version", lineNo)
				}
				hook.Version = n
			case "id":
				if !hasValue {
					return HookDefinition{}, fmt.Errorf("line %d: id requires a value", lineNo)
				}
				hook.ID = unquoteScalar(value)
			case "description":
				if !hasValue {
					return HookDefinition{}, fmt.Errorf("line %d: description requires a value", lineNo)
				}
				hook.Description = unquoteScalar(value)
			case "timeout":
				if !hasValue {
					return HookDefinition{}, fmt.Errorf("line %d: timeout requires a value", lineNo)
				}
				d, err := time.ParseDuration(unquoteScalar(value))
				if err != nil {
					return HookDefinition{}, fmt.Errorf("line %d: invalid timeout", lineNo)
				}
				hook.Timeout = d
			case "triggers", "execute", "env":
				if hasValue && strings.TrimSpace(value) != "" {
					return HookDefinition{}, fmt.Errorf("line %d: %s must be a block", lineNo, key)
				}
				section = key
			default:
				return HookDefinition{}, fmt.Errorf("line %d: unknown key %q", lineNo, key)
			}
			continue
		}

		switch section {
		case "triggers":
			item, err := parseListItem(trimmed)
			if err != nil {
				return HookDefinition{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
			hook.Triggers = append(hook.Triggers, HookTrigger(unquoteScalar(item)))
		case "execute":
			item, err := parseListItem(trimmed)
			if err != nil {
				return HookDefinition{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
			hook.Execute = append(hook.Execute, unquoteScalar(item))
		case "env":
			k, v, err := parseMapItem(trimmed)
			if err != nil {
				return HookDefinition{}, fmt.Errorf("line %d: %w", lineNo, err)
			}
			hook.Env[k] = unquoteScalar(v)
		default:
			return HookDefinition{}, fmt.Errorf("line %d: unexpected indentation", lineNo)
		}
	}

	if len(hook.Env) == 0 {
		hook.Env = nil
	}
	if hook.Timeout <= 0 {
		hook.Timeout = defaultHookTimeout
	}
	return hook, nil
}

func parseTopLevelLine(line string) (key, value string, hasValue bool, err error) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", "", false, fmt.Errorf("expected key: value")
	}
	key = strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", false, fmt.Errorf("empty key")
	}
	value = strings.TrimSpace(parts[1])
	return key, value, value != "", nil
}

func parseListItem(line string) (string, error) {
	if !strings.HasPrefix(line, "- ") {
		return "", fmt.Errorf("expected list item")
	}
	return strings.TrimSpace(line[2:]), nil
}

func parseMapItem(line string) (string, string, error) {
	parts := strings.SplitN(line, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("expected map item key: value")
	}
	key := strings.TrimSpace(parts[0])
	if key == "" {
		return "", "", fmt.Errorf("empty map key")
	}
	return key, strings.TrimSpace(parts[1]), nil
}

func countLeadingSpaces(line string) int {
	n := 0
	for i := 0; i < len(line); i++ {
		if line[i] != ' ' {
			break
		}
		n++
	}
	return n
}

func unquoteScalar(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '"' && s[len(s)-1] == '"') || (s[0] == '\'' && s[len(s)-1] == '\'') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
