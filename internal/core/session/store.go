package session

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/thwoodle/open-pilot/internal/domain"
)

// Snapshot is a persistence-safe representation of the store state.
type Snapshot struct {
	Sessions []SessionSnapshot
	NextID   int
}

type SessionSnapshot struct {
	ID                    string
	Name                  string
	ProviderID            string
	CodexThreadID         string
	AutoReviewLoopEnabled bool
	ActiveRepoID          string
	CreatedAt             int64
	Repos                 []domain.RepoRef
	Messages              []MessageSnapshot
}

type MessageSnapshot struct {
	ID         string
	Role       string
	Content    string
	Timestamp  int64
	ProviderID string
	RepoID     string
	Streaming  bool
}

// Persister persists and restores session snapshots.
type Persister interface {
	Load() (Snapshot, error)
	Save(Snapshot) error
}

// Store keeps in-memory session state.
type Store struct {
	Sessions        map[string]*domain.Session
	SessionOrder    []string
	ActiveSessionID string

	nextID int
	nowFn  func() time.Time

	persister      Persister
	persistenceErr string
	errorLatched   bool
}

func NewStore() *Store {
	return newStore(nil)
}

func NewStoreWithPersister(p Persister) *Store {
	return newStore(p)
}

func newStore(p Persister) *Store {
	s := &Store{
		Sessions:     make(map[string]*domain.Session),
		SessionOrder: make([]string, 0),
		nextID:       1,
		nowFn:        time.Now,
		persister:    p,
	}
	if p != nil {
		if err := s.loadFromPersister(); err != nil {
			s.persistenceErr = "Session persistence disabled: " + err.Error()
			s.errorLatched = true
			s.persister = nil
		}
	}
	return s
}

func (s *Store) TakePersistenceWarning() string {
	msg := s.persistenceErr
	s.persistenceErr = ""
	return msg
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
	id := uuid.NewString()
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
	s.saveIfEnabled()
	return ss
}

func (s *Store) UseSession(id string) bool {
	match, ok := s.resolveSessionSelector(id)
	if !ok {
		return false
	}
	s.ActiveSessionID = match
	s.saveIfEnabled()
	return true
}

func (s *Store) DeleteSession(selector string) bool {
	match, ok := s.resolveSessionSelector(selector)
	if !ok {
		return false
	}
	delete(s.Sessions, match)
	filtered := make([]string, 0, len(s.SessionOrder))
	for _, sid := range s.SessionOrder {
		if sid != match {
			filtered = append(filtered, sid)
		}
	}
	s.SessionOrder = filtered
	if s.ActiveSessionID == match {
		s.ActiveSessionID = ""
	}
	s.saveIfEnabled()
	return true
}

func (s *Store) HasSessionName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, sess := range s.Sessions {
		if sess == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(sess.Name), name) {
			return true
		}
	}
	return false
}

func (s *Store) resolveSessionSelector(selector string) (string, bool) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return "", false
	}
	if _, ok := s.Sessions[selector]; ok {
		return selector, true
	}
	match := ""
	for sid, sess := range s.Sessions {
		if sess == nil {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(sess.Name), selector) {
			if match != "" {
				return "", false
			}
			match = sid
		}
	}
	if match == "" {
		return "", false
	}
	return match, true
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
	s.saveIfEnabled()
	return nil
}

func (s *Store) SetAutoReviewLoopEnabledForActiveSession(enabled bool) error {
	active := s.ActiveSession()
	if active == nil {
		return fmt.Errorf("no active session")
	}
	active.AutoReviewLoopEnabled = enabled
	s.saveIfEnabled()
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
			s.saveIfEnabled()
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
	s.saveIfEnabled()
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
	s.saveIfEnabled()
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
	s.saveIfEnabled()
	return len(active.Messages) - 1
}

func (s *Store) AddAssistantMessage(sessionID, text string) {
	_ = s.AppendAssistantMessage(sessionID, text)
}

func (s *Store) AppendSystemMessage(sessionID, text string) int {
	ss := s.Sessions[sessionID]
	if ss == nil {
		return -1
	}
	ss.Messages = append(ss.Messages, domain.Message{
		ID:        s.NextID("msg"),
		Role:      domain.RoleSystem,
		Content:   text,
		Timestamp: s.Now(),
	})
	s.saveIfEnabled()
	return len(ss.Messages) - 1
}

func (s *Store) AppendAssistantMessage(sessionID, text string) int {
	ss := s.Sessions[sessionID]
	if ss == nil {
		return -1
	}
	ss.Messages = append(ss.Messages, domain.Message{ID: s.NextID("msg"), Role: domain.RoleAssistant, Content: text, Timestamp: s.Now()})
	s.saveIfEnabled()
	return len(ss.Messages) - 1
}

func (s *Store) ReplaceMessageAt(sessionID string, index int, text string) bool {
	ss := s.Sessions[sessionID]
	if ss == nil || index < 0 || index >= len(ss.Messages) {
		return false
	}
	msg := ss.Messages[index]
	msg.Content = text
	msg.Streaming = false
	ss.Messages[index] = msg
	s.saveIfEnabled()
	return true
}

func (s *Store) DeleteMessageAt(sessionID string, index int) bool {
	ss := s.Sessions[sessionID]
	if ss == nil || index < 0 || index >= len(ss.Messages) {
		return false
	}
	ss.Messages = append(ss.Messages[:index], ss.Messages[index+1:]...)
	s.saveIfEnabled()
	return true
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
	s.saveIfEnabled()
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
	s.saveIfEnabled()
	return true
}

func (s *Store) AddSessionSystemMessage(sessionID, text string) {
	ss := s.Sessions[sessionID]
	if ss == nil {
		return
	}
	ss.Messages = append(ss.Messages, domain.Message{ID: s.NextID("msg"), Role: domain.RoleSystem, Content: text, Timestamp: s.Now()})
	s.saveIfEnabled()
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
		name := strings.TrimSpace(ss.Name)
		if name == "" {
			name = ss.ID
		}
		line := name
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
	ids = append(ids, s.SessionOrder...)
	return ids
}

func (s *Store) SessionNames() []string {
	names := make([]string, 0, len(s.SessionOrder))
	for _, id := range s.SessionOrder {
		sess := s.Sessions[id]
		if sess == nil {
			continue
		}
		name := strings.TrimSpace(sess.Name)
		if name == "" {
			name = sess.ID
		}
		names = append(names, name)
	}
	return names
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

func (s *Store) saveIfEnabled() {
	if s.persister == nil {
		return
	}
	if err := s.persister.Save(s.snapshot()); err != nil {
		if !s.errorLatched {
			s.persistenceErr = "Session persistence warning: " + err.Error()
			s.errorLatched = true
		}
		return
	}
	s.errorLatched = false
}

func (s *Store) loadFromPersister() error {
	snap, err := s.persister.Load()
	if err != nil {
		return err
	}
	s.applySnapshot(snap)
	return nil
}

func (s *Store) snapshot() Snapshot {
	snap := Snapshot{Sessions: make([]SessionSnapshot, 0, len(s.SessionOrder)), NextID: s.nextID}
	for _, sid := range s.SessionOrder {
		ss := s.Sessions[sid]
		if ss == nil {
			continue
		}
		item := SessionSnapshot{
			ID:                    ss.ID,
			Name:                  ss.Name,
			ProviderID:            ss.ProviderID,
			CodexThreadID:         ss.CodexThreadID,
			AutoReviewLoopEnabled: ss.AutoReviewLoopEnabled,
			ActiveRepoID:          ss.ActiveRepoID,
			CreatedAt:             ss.CreatedAt.Unix(),
			Repos:                 append([]domain.RepoRef{}, ss.Repos...),
			Messages:              make([]MessageSnapshot, 0, len(ss.Messages)),
		}
		for _, m := range ss.Messages {
			item.Messages = append(item.Messages, MessageSnapshot{
				ID:         m.ID,
				Role:       m.Role,
				Content:    m.Content,
				Timestamp:  m.Timestamp.Unix(),
				ProviderID: m.ProviderID,
				RepoID:     m.RepoID,
				Streaming:  m.Streaming,
			})
		}
		snap.Sessions = append(snap.Sessions, item)
	}
	return snap
}

func (s *Store) applySnapshot(snap Snapshot) {
	s.Sessions = make(map[string]*domain.Session, len(snap.Sessions))
	s.SessionOrder = make([]string, 0, len(snap.Sessions))
	s.ActiveSessionID = ""
	maxID := 0

	for _, ss := range snap.Sessions {
		sessionItem := &domain.Session{
			ID:                    ss.ID,
			Name:                  ss.Name,
			ProviderID:            ss.ProviderID,
			CodexThreadID:         ss.CodexThreadID,
			AutoReviewLoopEnabled: ss.AutoReviewLoopEnabled,
			ActiveRepoID:          ss.ActiveRepoID,
			CreatedAt:             time.Unix(ss.CreatedAt, 0),
			Repos:                 append([]domain.RepoRef{}, ss.Repos...),
			Messages:              make([]domain.Message, 0, len(ss.Messages)),
		}
		for _, msg := range ss.Messages {
			sessionItem.Messages = append(sessionItem.Messages, domain.Message{
				ID:         msg.ID,
				Role:       msg.Role,
				Content:    msg.Content,
				Timestamp:  time.Unix(msg.Timestamp, 0),
				ProviderID: msg.ProviderID,
				RepoID:     msg.RepoID,
				Streaming:  false,
			})
		}
		s.Sessions[sessionItem.ID] = sessionItem
		s.SessionOrder = append(s.SessionOrder, sessionItem.ID)
		maxID = maxInt(maxID, maxNumericID(sessionItem.ID))
		for _, r := range sessionItem.Repos {
			maxID = maxInt(maxID, maxNumericID(r.ID))
		}
		for _, m := range sessionItem.Messages {
			maxID = maxInt(maxID, maxNumericID(m.ID))
		}
	}

	if snap.NextID > 0 {
		s.nextID = snap.NextID
	} else {
		s.nextID = maxID + 1
		if s.nextID <= 0 {
			s.nextID = 1
		}
	}
}

func maxNumericID(id string) int {
	parts := strings.Split(id, "-")
	if len(parts) == 0 {
		return 0
	}
	n, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return 0
	}
	return n
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
