package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveBuiltinAssetsRootFromFindsGoModByWalkingUp(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/test\n")

	start := filepath.Join(root, "a", "b", "c")
	mkdirAll(t, start)

	got := resolveBuiltinAssetsRootFrom(start)
	if got != root {
		t.Fatalf("expected root %q, got %q", root, got)
	}
}

func TestResolveBuiltinAssetsRootFromFallsBackToStartWhenGoModMissing(t *testing.T) {
	start := filepath.Join(t.TempDir(), "nested")
	mkdirAll(t, start)

	got := resolveBuiltinAssetsRootFrom(start)
	if got != start {
		t.Fatalf("expected fallback start %q, got %q", start, got)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	mkdirAll(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
}

func mkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}
