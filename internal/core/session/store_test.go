package session

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/thwoodle/open-pilot/internal/domain"
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
	if !s.UseSession("demo") {
		t.Fatalf("expected use session by name to succeed")
	}
}

func TestDeleteSessionByNameClearsActive(t *testing.T) {
	s := NewStore()
	created := s.CreateSession("demo")
	if created == nil {
		t.Fatalf("expected session create")
	}
	if !s.DeleteSession("demo") {
		t.Fatalf("expected delete by name to succeed")
	}
	if s.ActiveSessionID != "" {
		t.Fatalf("expected active session to clear after delete, got %q", s.ActiveSessionID)
	}
	if len(s.SessionOrder) != 0 {
		t.Fatalf("expected no sessions after delete")
	}
}

func TestHasSessionNameIsCaseInsensitive(t *testing.T) {
	s := NewStore()
	s.CreateSession("Demo Session")
	if !s.HasSessionName("demo session") {
		t.Fatalf("expected case-insensitive session name match")
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

type fakePersister struct {
	loadSnapshot Snapshot
	loadErr      error
	saveErr      error
	saveCalls    int
}

func (f *fakePersister) Load() (Snapshot, error) {
	return f.loadSnapshot, f.loadErr
}

func (f *fakePersister) Save(s Snapshot) error {
	f.saveCalls++
	_ = s
	return f.saveErr
}

func TestStoreMutationsTriggerSave(t *testing.T) {
	fp := &fakePersister{}
	s := NewStoreWithPersister(fp)
	s.CreateSession("demo")
	if fp.saveCalls == 0 {
		t.Fatalf("expected save call after mutation")
	}
}

func TestLoadFromPersisterLeavesNoActiveSession(t *testing.T) {
	fp := &fakePersister{
		loadSnapshot: Snapshot{
			NextID: 20,
			Sessions: []SessionSnapshot{
				{
					ID:         "session-1",
					Name:       "one",
					ProviderID: "codex",
					CreatedAt:  100,
					Repos:      []domain.RepoRef{{ID: "repo-1", Path: "/tmp", Label: "tmp"}},
				},
			},
		},
	}
	s := NewStoreWithPersister(fp)
	if len(s.SessionOrder) != 1 {
		t.Fatalf("expected loaded session order")
	}
	if s.ActiveSessionID != "" {
		t.Fatalf("expected no active session on load, got %q", s.ActiveSessionID)
	}
	if got := s.NextID("msg"); got != "msg-20" {
		t.Fatalf("expected next id continuity, got %q", got)
	}
}

func TestLoadFromPersisterNormalizesStreamingMessages(t *testing.T) {
	fp := &fakePersister{
		loadSnapshot: Snapshot{
			Sessions: []SessionSnapshot{
				{
					ID:        "session-1",
					Name:      "one",
					CreatedAt: 100,
					Messages: []MessageSnapshot{{
						ID:        "msg-1",
						Role:      domain.RoleAssistant,
						Content:   "partial",
						Timestamp: 100,
						Streaming: true,
					}},
				},
			},
		},
	}
	s := NewStoreWithPersister(fp)
	msg := s.Sessions["session-1"].Messages[0]
	if msg.Streaming {
		t.Fatalf("expected loaded streaming message to be normalized to false")
	}
}

func TestSnapshotRoundTripPreservesAutoReviewLoopOption(t *testing.T) {
	s := NewStore()
	created := s.CreateSession("demo")
	if created == nil {
		t.Fatalf("expected session create")
	}
	created.AutoReviewLoopEnabled = true

	snap := s.snapshot()
	if len(snap.Sessions) != 1 {
		t.Fatalf("expected one snapshot session, got %d", len(snap.Sessions))
	}
	if !snap.Sessions[0].AutoReviewLoopEnabled {
		t.Fatalf("expected snapshot to preserve auto-review loop option")
	}

	restored := NewStore()
	restored.applySnapshot(snap)
	session := restored.Sessions[created.ID]
	if session == nil {
		t.Fatalf("expected restored session")
	}
	if !session.AutoReviewLoopEnabled {
		t.Fatalf("expected restored session to preserve auto-review loop option")
	}
}

func TestSaveErrorSetsWarningButKeepsState(t *testing.T) {
	fp := &fakePersister{saveErr: errors.New("disk full")}
	s := NewStoreWithPersister(fp)
	created := s.CreateSession("demo")
	if created == nil {
		t.Fatalf("expected session create despite save failure")
	}
	warn := s.TakePersistenceWarning()
	if warn == "" {
		t.Fatalf("expected persistence warning after save failure")
	}
}
