package config

import "time"

// ProviderConfig controls a provider wrapper command.
type ProviderConfig struct {
	ID             string
	Command        string
	Args           []string
	Env            map[string]string
	StartupTimeout time.Duration
}

// Config is the application runtime config.
type Config struct {
	Providers                 map[string]ProviderConfig
	SessionPersistenceEnabled bool
	SessionDBPath             string
	SessionPersistenceWarning string
	BuiltinHooksDir           string
	BuiltinHooks              HookCatalog
	BuiltinHooksLoadError     string
}
