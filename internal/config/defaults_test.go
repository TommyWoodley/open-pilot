package config

import (
	"errors"
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

func TestChooseBuiltinAssetsRootUsesCallerRootWhenItHasGoMod(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module example.com/caller\n")

	got := chooseBuiltinAssetsRoot(root, func() (string, error) {
		return "", errors.New("getwd should not be called")
	})
	if got != root {
		t.Fatalf("expected caller root %q, got %q", root, got)
	}
}

func TestChooseBuiltinAssetsRootFallsBackToWorkingDirWhenCallerRootMissingGoMod(t *testing.T) {
	callerRoot := filepath.Join(t.TempDir(), "missing")
	wdRoot := t.TempDir()
	writeFile(t, filepath.Join(wdRoot, "go.mod"), "module example.com/wd\n")

	got := chooseBuiltinAssetsRoot(callerRoot, func() (string, error) {
		return wdRoot, nil
	})
	if got != wdRoot {
		t.Fatalf("expected wd root %q, got %q", wdRoot, got)
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
