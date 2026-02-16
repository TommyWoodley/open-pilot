package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSessionCreateUse(t *testing.T) {
	s := NewStore()
	created := s.CreateSession("demo")
	if created == nil || s.ActiveSession() == nil {
		t.Fatalf("expected active session")
	}
	if !s.UseSession(created.ID) {
		t.Fatalf("expected use session to succeed")
	}
}

func TestAddRepoToActiveSession(t *testing.T) {
	s := NewStore()
	s.CreateSession("demo")
	tmp := t.TempDir()
	repo := filepath.Join(tmp, "repo")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := s.AddRepoToActiveSession(repo, ""); err != nil {
		t.Fatalf("add repo: %v", err)
	}
	if s.ActiveRepo() == nil {
		t.Fatalf("expected active repo")
	}
}

func TestAddRepoAcceptsRelativePath(t *testing.T) {
	s := NewStore()
	s.CreateSession("demo")

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

	if err := s.AddRepoToActiveSession("repo", ""); err != nil {
		t.Fatalf("add repo failed: %v", err)
	}
	repo := s.ActiveRepo()
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
	s := NewStore()
	s.CreateSession("demo")

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

	if err := s.AddRepoToActiveSession("", ""); err != nil {
		t.Fatalf("add repo failed: %v", err)
	}

	repo := s.ActiveRepo()
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

func TestMessageLifecycle(t *testing.T) {
	s := NewStore()
	s.CreateSession("demo")
	idx := s.AppendAssistantStreaming("codex", "repo-1")
	if idx < 0 {
		t.Fatalf("expected streaming index")
	}
	if !s.AppendChunkAt(s.ActiveSessionID, idx, "hi") {
		t.Fatalf("expected chunk append")
	}
	if !s.FinalizeAt(s.ActiveSessionID, idx, "done") {
		t.Fatalf("expected finalize")
	}
}
