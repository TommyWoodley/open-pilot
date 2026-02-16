package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/thwoodle/open-pilot/internal/domain"
)

// Store keeps in-memory session state.
type Store struct {
	Sessions        map[string]*domain.Session
	SessionOrder    []string
	ActiveSessionID string

	nextID int
	nowFn  func() time.Time
}

func NewStore() *Store {
	return &Store{
		Sessions:     make(map[string]*domain.Session),
		SessionOrder: make([]string, 0),
		nextID:       1,
		nowFn:        time.Now,
	}
}

func (s *Store) ActiveSession() *domain.Session {
	if s.ActiveSessionID == "" {
		return nil
	}
	return s.Sessions[s.ActiveSessionID]
}

func (s *Store) NextID(prefix string) string {
	id := prefix + "-" + strconv.Itoa(s.nextID)
	s.nextID++
	return id
}

func (s *Store) Now() time.Time {
	return s.nowFn()
}

func (s *Store) CreateSession(name string) *domain.Session {
	id := s.NextID("session")
	ss := &domain.Session{
		ID:        id,
		Name:      strings.TrimSpace(name),
		CreatedAt: s.Now(),
		Messages:  make([]domain.Message, 0),
		Repos:     make([]domain.RepoRef, 0),
	}
	s.Sessions[id] = ss
	s.SessionOrder = append(s.SessionOrder, id)
	s.ActiveSessionID = id
	return ss
}

func (s *Store) UseSession(id string) bool {
	if _, ok := s.Sessions[id]; !ok {
		return false
	}
	s.ActiveSessionID = id
	return true
}

func NormalizeRepoPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("repo path cannot be empty")
	}
	if !filepath.IsAbs(path) {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to resolve working directory: %w", err)
		}
		path = filepath.Join(wd, path)
	}
	return filepath.Clean(path), nil
}

func (s *Store) AddRepoToActiveSession(path, label string) error {
	active := s.ActiveSession()
	if active == nil {
		return fmt.Errorf("no active session")
	}
	if strings.TrimSpace(path) == "" {
		wd, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to resolve working directory: %w", err)
		}
		path = wd
	}
	normalized, err := NormalizeRepoPath(path)
	if err != nil {
		return err
	}
	id := s.NextID("repo")
	if label == "" {
		label = filepath.Base(normalized)
	}
	active.Repos = append(active.Repos, domain.RepoRef{ID: id, Path: normalized, Label: label})
	if active.ActiveRepoID == "" {
		active.ActiveRepoID = id
	}
	return nil
}

func (s *Store) SetActiveRepo(repoID string) error {
	active := s.ActiveSession()
	if active == nil {
		return fmt.Errorf("no active session")
	}
	for _, repo := range active.Repos {
		if repo.ID == repoID {
			active.ActiveRepoID = repoID
			return nil
		}
	}
	return fmt.Errorf("repo not found: %s", repoID)
}

func (s *Store) ActiveRepo() *domain.RepoRef {
	active := s.ActiveSession()
	if active == nil || active.ActiveRepoID == "" {
		return nil
	}
	for i := range active.Repos {
		if active.Repos[i].ID == active.ActiveRepoID {
			return &active.Repos[i]
		}
	}
	return nil
}

func (s *Store) AddSystemMessage(text string) {
	active := s.ActiveSession()
	if active == nil {
		return
	}
	active.Messages = append(active.Messages, domain.Message{
		ID:        s.NextID("msg"),
		Role:      domain.RoleSystem,
		Content:   text,
		Timestamp: s.Now(),
	})
}

func (s *Store) AppendUserMessage(providerID, repoID, text string) {
	active := s.ActiveSession()
	if active == nil {
		return
	}
	active.Messages = append(active.Messages, domain.Message{
		ID:         s.NextID("msg"),
		Role:       domain.RoleUser,
		Content:    text,
		Timestamp:  s.Now(),
		ProviderID: providerID,
		RepoID:     repoID,
	})
}

func (s *Store) AppendAssistantStreaming(providerID, repoID string) int {
	active := s.ActiveSession()
	if active == nil {
		return -1
	}
	active.Messages = append(active.Messages, domain.Message{
		ID:         s.NextID("msg"),
		Role:       domain.RoleAssistant,
		Content:    "",
		Timestamp:  s.Now(),
		ProviderID: providerID,
		RepoID:     repoID,
		Streaming:  true,
	})
	return len(active.Messages) - 1
}

func (s *Store) AddAssistantMessage(sessionID, text string) {
	ss := s.Sessions[sessionID]
	if ss == nil {
		return
	}
	ss.Messages = append(ss.Messages, domain.Message{ID: s.NextID("msg"), Role: domain.RoleAssistant, Content: text, Timestamp: s.Now()})
}

func (s *Store) FinalizeAt(sessionID string, index int, text string) bool {
	ss := s.Sessions[sessionID]
	if ss == nil || index < 0 || index >= len(ss.Messages) {
		return false
	}
	msg := ss.Messages[index]
	if strings.TrimSpace(text) != "" {
		msg.Content = text
	}
	msg.Streaming = false
	ss.Messages[index] = msg
	return true
}

func (s *Store) AppendChunkAt(sessionID string, index int, chunk string) bool {
	ss := s.Sessions[sessionID]
	if ss == nil || index < 0 || index >= len(ss.Messages) {
		return false
	}
	msg := ss.Messages[index]
	msg.Content += chunk
	ss.Messages[index] = msg
	return true
}

func (s *Store) AddSessionSystemMessage(sessionID, text string) {
	ss := s.Sessions[sessionID]
	if ss == nil {
		return
	}
	ss.Messages = append(ss.Messages, domain.Message{ID: s.NextID("msg"), Role: domain.RoleSystem, Content: text, Timestamp: s.Now()})
}

func (s *Store) ListSessionsText() string {
	if len(s.SessionOrder) == 0 {
		return "No sessions"
	}
	lines := make([]string, 0, len(s.SessionOrder))
	for _, id := range s.SessionOrder {
		ss := s.Sessions[id]
		if ss == nil {
			continue
		}
		line := id + " " + ss.Name
		if id == s.ActiveSessionID {
			line += " (active)"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (s *Store) ListReposText() string {
	active := s.ActiveSession()
	if active == nil {
		return "No active session"
	}
	if len(active.Repos) == 0 {
		return "No repos in session"
	}
	lines := make([]string, 0, len(active.Repos))
	for _, repo := range active.Repos {
		line := repo.ID + " " + repo.Label + " -> " + repo.Path
		if repo.ID == active.ActiveRepoID {
			line += " (active)"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func (s *Store) SessionIDs() []string {
	ids := make([]string, 0, len(s.SessionOrder))
	for _, id := range s.SessionOrder {
		ids = append(ids, id)
	}
	return ids
}

func (s *Store) ActiveRepoIDs() []string {
	active := s.ActiveSession()
	if active == nil {
		return nil
	}
	ids := make([]string, 0, len(active.Repos))
	for _, repo := range active.Repos {
		ids = append(ids, repo.ID)
	}
	return ids
}
