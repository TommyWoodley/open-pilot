package config

import "time"

// Default returns in-memory defaults for v1.
func Default() Config {
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
		BuiltinHooksDir:           "/Users/thwoodle/Desktop/open-pilot/hooks/builtin",
		BuiltinSkillsDir:          "/Users/thwoodle/Desktop/open-pilot/skills/builtin",
	}
}
