package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// Default returns in-memory defaults for v1.
func Default() Config {
	root := resolveBuiltinAssetsRoot()

	return Config{
		Providers: map[string]ProviderConfig{
			"codex": {
				ID:             "codex",
				Command:        "codex",
				Args:           nil,
				Env:            map[string]string{},
				StartupTimeout: 10 * time.Second,
			},
			"cursor": {
				ID:             "cursor",
				Command:        "open-pilot-cursor-wrapper",
				Args:           nil,
				Env:            map[string]string{},
				StartupTimeout: 10 * time.Second,
			},
		},
		SessionPersistenceEnabled: true,
		SessionDBPath:             "",
		BuiltinHooksDir:           filepath.Join(root, "hooks", "builtin"),
		BuiltinSkillsDir:          filepath.Join(root, "skills", "builtin"),
	}
}

func resolveBuiltinAssetsRoot() string {
	callerRoot := ""
	if _, file, _, ok := runtime.Caller(0); ok {
		callerRoot = resolveBuiltinAssetsRootFrom(filepath.Dir(file))
	}
	return chooseBuiltinAssetsRoot(callerRoot, os.Getwd)
}

func chooseBuiltinAssetsRoot(callerRoot string, getwd func() (string, error)) string {
	if hasGoModFile(callerRoot) {
		return filepath.Clean(callerRoot)
	}

	wd, err := getwd()
	if err != nil {
		return "."
	}
	return resolveBuiltinAssetsRootFrom(wd)
}

func hasGoModFile(root string) bool {
	root = strings.TrimSpace(root)
	if root == "" {
		return false
	}
	info, err := os.Stat(filepath.Join(filepath.Clean(root), "go.mod"))
	return err == nil && !info.IsDir()
}

func resolveBuiltinAssetsRootFrom(start string) string {
	start = strings.TrimSpace(start)
	if start == "" {
		return "."
	}

	cur := filepath.Clean(start)
	for {
		if info, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil && !info.IsDir() {
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return filepath.Clean(start)
		}
		cur = parent
	}
}
