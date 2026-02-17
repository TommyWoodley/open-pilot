package app

import (
	"strings"
	"testing"

	"github.com/thwoodle/open-pilot/internal/config"
	"github.com/thwoodle/open-pilot/internal/core/session"
)

type fakeAppPersister struct {
	snap session.Snapshot
}

func (f *fakeAppPersister) Load() (session.Snapshot, error) { return f.snap, nil }
func (f *fakeAppPersister) Save(session.Snapshot) error     { return nil }

func TestModelStartsWithLoadedSessionsButNoActive(t *testing.T) {
	t.Parallel()
	p := &fakeAppPersister{snap: session.Snapshot{Sessions: []session.SessionSnapshot{{ID: "session-1", Name: "demo", CreatedAt: 100}}, NextID: 2}}
	m := NewModel(nil, config.Default(), p)
	if m.activeSession() != nil {
		t.Fatalf("expected no active session at startup")
	}
	list := m.listSessionsText()
	if !strings.Contains(list, "demo") {
		t.Fatalf("expected loaded session in list, got %q", list)
	}
}

func TestModelCanUseLoadedSessionAfterStartup(t *testing.T) {
	t.Parallel()
	p := &fakeAppPersister{snap: session.Snapshot{Sessions: []session.SessionSnapshot{{ID: "session-1", Name: "demo", CreatedAt: 100}}, NextID: 2}}
	m := NewModel(nil, config.Default(), p)
	if !m.useSession("demo") {
		t.Fatalf("expected use session to succeed")
	}
	if m.activeSession() == nil || m.activeSession().Name != "demo" {
		t.Fatalf("expected active loaded session")
	}
}
