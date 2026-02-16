package providers

import (
	"testing"

	"github.com/thwoodle/open-pilot/internal/config"
)

func TestManagerUsesBuiltInCodexAdapter(t *testing.T) {
	t.Parallel()

	cfg := config.Default()
	cfg.Providers["codex"] = config.ProviderConfig{
		ID:      "codex",
		Command: "open-pilot-codex-wrapper",
	}

	svc := NewManager(cfg).(*service)
	adapter, _, err := svc.adapterForLocked("codex")
	if err != nil {
		t.Fatalf("adapterForLocked returned error: %v", err)
	}

	codexAdapter, ok := adapter.(*codexCLIAdapter)
	if !ok {
		t.Fatalf("expected *codexCLIAdapter, got %T", adapter)
	}
	if codexAdapter.binary != "codex" {
		t.Fatalf("expected codex binary fallback, got %q", codexAdapter.binary)
	}
}
