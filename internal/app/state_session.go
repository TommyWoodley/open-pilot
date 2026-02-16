package app

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/thwoodle/open-pilot/internal/domain"
)

func (m *Model) addSystemMessage(text string) {
	s := m.activeSession()
	if s == nil {
		m.StatusText = text
		return
	}
	s.Messages = append(s.Messages, domain.Message{
		ID:        m.nextMessageID("msg"),
		Role:      domain.RoleSystem,
		Content:   text,
		Timestamp: now(),
	})
	m.StatusText = text
}

func (m *Model) createSession(name string) *domain.Session {
	id := m.nextMessageID("session")
	s := &domain.Session{
		ID:        id,
		Name:      strings.TrimSpace(name),
		CreatedAt: now(),
		Messages:  make([]domain.Message, 0),
		Repos:     make([]domain.RepoRef, 0),
	}
	m.Sessions[id] = s
	m.SessionOrder = append(m.SessionOrder, id)
	m.ActiveSessionID = id
	m.StatusText = "Created session " + id
	return s
}

func (m *Model) useSession(id string) bool {
	if _, ok := m.Sessions[id]; !ok {
		return false
	}
	m.ActiveSessionID = id
	m.StatusText = "Using session " + id
	return true
}

func (m *Model) addRepoToActiveSession(path, label string) error {
	s := m.activeSession()
	if s == nil {
		return fmt.Errorf("no active session")
	}
	if strings.TrimSpace(path) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to resolve working directory: %w", err)
		}
		path = wd
	}
	normalized, err := normalizeRepoPath(path)
	if err != nil {
		return err
	}
	id := m.nextMessageID("repo")
	if label == "" {
		label = filepath.Base(normalized)
	}
	s.Repos = append(s.Repos, domain.RepoRef{ID: id, Path: normalized, Label: label})
	if s.ActiveRepoID == "" {
		s.ActiveRepoID = id
	}
	m.StatusText = "Added repo " + normalized
	return nil
}

func (m *Model) setActiveRepo(repoID string) error {
	s := m.activeSession()
	if s == nil {
		return fmt.Errorf("no active session")
	}
	for _, repo := range s.Repos {
		if repo.ID == repoID {
			s.ActiveRepoID = repoID
			m.StatusText = "Using repo " + repo.Label
			return nil
		}
	}
	return fmt.Errorf("repo not found: %s", repoID)
}

func (m *Model) activeRepo() *domain.RepoRef {
	s := m.activeSession()
	if s == nil || s.ActiveRepoID == "" {
		return nil
	}
	for i := range s.Repos {
		if s.Repos[i].ID == s.ActiveRepoID {
			return &s.Repos[i]
		}
	}
	return nil
}

func (m *Model) appendUserMessage(providerID, repoID, text string) {
	s := m.activeSession()
	if s == nil {
		return
	}
	s.Messages = append(s.Messages, domain.Message{
		ID:         m.nextMessageID("msg"),
		Role:       domain.RoleUser,
		Content:    text,
		Timestamp:  now(),
		ProviderID: providerID,
		RepoID:     repoID,
	})
}

func (m *Model) appendAssistantStreaming(providerID, repoID, requestID string) {
	s := m.activeSession()
	if s == nil {
		return
	}
	s.Messages = append(s.Messages, domain.Message{
		ID:         m.nextMessageID("msg"),
		Role:       domain.RoleAssistant,
		Content:    "",
		Timestamp:  now(),
		ProviderID: providerID,
		RepoID:     repoID,
		Streaming:  true,
	})
	m.pending[requestID] = len(s.Messages) - 1
}

func (m *Model) listSessionsText() string {
	if len(m.SessionOrder) == 0 {
		return "No sessions"
	}
	lines := make([]string, 0, len(m.SessionOrder))
	for _, id := range m.SessionOrder {
		s := m.Sessions[id]
		if s == nil {
			continue
		}
		line := id + " " + s.Name
		if id == m.ActiveSessionID {
			line += " (active)"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (m *Model) listReposText() string {
	s := m.activeSession()
	if s == nil {
		return "No active session"
	}
	if len(s.Repos) == 0 {
		return "No repos in session"
	}
	lines := make([]string, 0, len(s.Repos))
	for _, repo := range s.Repos {
		line := repo.ID + " " + repo.Label + " -> " + repo.Path
		if repo.ID == s.ActiveRepoID {
			line += " (active)"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}
