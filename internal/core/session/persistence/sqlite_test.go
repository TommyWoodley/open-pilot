package persistence

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thwoodle/open-pilot/internal/core/session"
	"github.com/thwoodle/open-pilot/internal/domain"
)

func TestSQLitePersisterSaveLoadRoundTrip(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sessions.db")
	p, err := NewSQLitePersister(path)
	if err != nil {
		t.Fatalf("new persister: %v", err)
	}

	in := session.Snapshot{
		NextID: 42,
		Sessions: []session.SessionSnapshot{{
			ID:           "session-1",
			Name:         "demo",
			ProviderID:   "codex",
			ActiveRepoID: "repo-1",
			CreatedAt:    100,
			Repos: []domain.RepoRef{{
				ID:    "repo-1",
				Path:  "/tmp/repo",
				Label: "repo",
			}},
			Messages: []session.MessageSnapshot{{
				ID:        "msg-1",
				Role:      domain.RoleAssistant,
				Content:   "hello",
				Timestamp: 123,
				Streaming: true,
			}},
		}},
	}
	if err := p.Save(in); err != nil {
		t.Fatalf("save: %v", err)
	}

	out, err := p.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if out.NextID != 42 {
		t.Fatalf("expected nextID 42, got %d", out.NextID)
	}
	if len(out.Sessions) != 1 || len(out.Sessions[0].Repos) != 1 || len(out.Sessions[0].Messages) != 1 {
		t.Fatalf("expected loaded graph, got %#v", out)
	}
	if out.Sessions[0].Messages[0].Streaming {
		t.Fatalf("expected streaming normalized false on load")
	}
}

func TestSQLitePersisterSaveLoadRoundTripPreservesCodexThreadID(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sessions.db")
	p, err := NewSQLitePersister(path)
	if err != nil {
		t.Fatalf("new persister: %v", err)
	}

	in := session.Snapshot{
		NextID: 1,
		Sessions: []session.SessionSnapshot{{
			ID:            "session-1",
			Name:          "demo",
			ProviderID:    "codex",
			CodexThreadID: "thread-abc",
			CreatedAt:     100,
		}},
	}
	if err := p.Save(in); err != nil {
		t.Fatalf("save: %v", err)
	}

	out, err := p.Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(out.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(out.Sessions))
	}
	if out.Sessions[0].CodexThreadID != "thread-abc" {
		t.Fatalf("expected codex thread id to round trip, got %q", out.Sessions[0].CodexThreadID)
	}
}

func TestSQLitePersisterLoadEmpty(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "sessions.db")
	p, err := NewSQLitePersister(path)
	if err != nil {
		t.Fatalf("new persister: %v", err)
	}
	out, err := p.Load()
	if err != nil {
		t.Fatalf("load empty: %v", err)
	}
	if len(out.Sessions) != 0 {
		t.Fatalf("expected empty snapshot")
	}
}

func TestSQLitePersisterRecoversCorruptFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "sessions.db")
	if err := os.WriteFile(path, []byte("not-a-sqlite-db"), 0o644); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	p, err := NewSQLitePersister(path)
	if err != nil {
		t.Fatalf("expected recovery, got err: %v", err)
	}
	if _, err := p.Load(); err != nil {
		t.Fatalf("load after recovery: %v", err)
	}
}
