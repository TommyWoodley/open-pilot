package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thwoodle/open-pilot/internal/config"
)

func TestSessionLifecycleCommands(t *testing.T) {
	t.Parallel()

	m := NewModel(nil, config.Default())

	m.runCommand(Command{Kind: "session.new", Session: "demo"})
	s := m.activeSession()
	if s == nil {
		t.Fatalf("expected active session")
	}

	if err := m.addRepoToActiveSession("/tmp", "tmp-repo"); err != nil {
		t.Fatalf("add repo failed: %v", err)
	}

	repo := m.activeRepo()
	if repo == nil {
		t.Fatalf("expected active repo")
	}

	m.runCommand(Command{Kind: "provider.use", ProviderID: "codex"})
	if s.ProviderID != "codex" {
		t.Fatalf("expected provider codex, got %q", s.ProviderID)
	}
}

func TestAddRepoAcceptsRelativePath(t *testing.T) {
	m := NewModel(nil, config.Default())
	m.runCommand(Command{Kind: "session.new", Session: "demo"})

	tmp := t.TempDir()
	relRepo := filepath.Join(tmp, "repo")
	if err := os.Mkdir(relRepo, 0o755); err != nil {
		t.Fatalf("mkdir failed: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	if err := m.addRepoToActiveSession("repo", ""); err != nil {
		t.Fatalf("add repo failed: %v", err)
	}
	repo := m.activeRepo()
	if repo == nil {
		t.Fatalf("expected active repo")
	}
	wantPath, err := filepath.EvalSymlinks(relRepo)
	if err != nil {
		t.Fatalf("eval symlinks want failed: %v", err)
	}
	gotPath, err := filepath.EvalSymlinks(repo.Path)
	if err != nil {
		t.Fatalf("eval symlinks got failed: %v", err)
	}
	if gotPath != wantPath {
		t.Fatalf("expected repo path %q, got %q", wantPath, gotPath)
	}
}

func TestAddRepoEmptyPathUsesCWD(t *testing.T) {
	m := NewModel(nil, config.Default())
	m.runCommand(Command{Kind: "session.new", Session: "demo"})

	tmp := t.TempDir()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir failed: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	if err := m.addRepoToActiveSession("", ""); err != nil {
		t.Fatalf("add repo failed: %v", err)
	}

	repo := m.activeRepo()
	if repo == nil {
		t.Fatalf("expected active repo")
	}

	wantPath, err := filepath.EvalSymlinks(tmp)
	if err != nil {
		t.Fatalf("eval symlinks want failed: %v", err)
	}
	gotPath, err := filepath.EvalSymlinks(repo.Path)
	if err != nil {
		t.Fatalf("eval symlinks got failed: %v", err)
	}
	if gotPath != wantPath {
		t.Fatalf("expected repo path %q, got %q", wantPath, gotPath)
	}
}
