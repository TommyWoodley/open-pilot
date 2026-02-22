package app

import (
	coreautocomplete "github.com/thwoodle/open-pilot/internal/core/autocomplete"
	corechat "github.com/thwoodle/open-pilot/internal/core/chat"
	corecommand "github.com/thwoodle/open-pilot/internal/core/command"
	"github.com/thwoodle/open-pilot/internal/domain"
	"github.com/thwoodle/open-pilot/internal/providers"

	tea "github.com/charmbracelet/bubbletea"
)

// Command is a parsed slash command.
type Command = corecommand.Command

// ParseCommand parses slash-prefixed input.
func ParseCommand(input string) (Command, bool, error) {
	return corecommand.Parse(input)
}

func (m Model) processEnter() Model {
	input := m.Input
	m.Input = ""
	if input == "" {
		return m
	}

	m.chat.ProcessInput(input)
	m.ProviderState = m.chat.ProviderState
	m.StatusText = m.chat.StatusText
	m.ActiveSessionID = m.store.ActiveSessionID

	cmd, isCommand, err := ParseCommand(input)
	if isCommand && err == nil && cmd.Kind == corecommand.KindSessionNew {
		m.Input = "/session add-repo "
	}
	m.consumeStoreWarning()
	return m
}

func (m *Model) runCommand(cmd Command) {
	m.chat.RunCommand(cmd)
	m.ProviderState = m.chat.ProviderState
	m.StatusText = m.chat.StatusText
	m.ActiveSessionID = m.store.ActiveSessionID
	if cmd.Kind == corecommand.KindSessionNew {
		m.Input = "/session add-repo "
	}
	m.consumeStoreWarning()
}

func helpText() string {
	return corecommand.HelpText()
}

func (m *Model) handleProviderEvent(ev providers.Event) {
	if ev.SessionID == "" {
		ev.SessionID = m.store.ActiveSessionID
	}
	if idx, ok := m.pending[ev.RequestID]; ok {
		switch ev.Type {
		case providers.EventChunk:
			_ = m.store.AppendChunkAt(ev.SessionID, idx, ev.Text)
			return
		case providers.EventFinal:
			if !m.store.FinalizeAt(ev.SessionID, idx, ev.Text) {
				m.store.AddAssistantMessage(ev.SessionID, ev.Text)
			}
			delete(m.pending, ev.RequestID)
			m.chat.ProviderState = "ready"
			m.chat.StatusText = "Response complete"
			m.ProviderState = m.chat.ProviderState
			m.StatusText = m.chat.StatusText
			return
		}
	}
	m.chat.HandleProviderEvent(ev)
	m.ProviderState = m.chat.ProviderState
	m.StatusText = m.chat.StatusText
	m.ActiveSessionID = m.store.ActiveSessionID
	if m.AutoFollowTranscript {
		m.TranscriptScroll = 0
	}
	m.consumeStoreWarning()
}

type providerEventMsg struct {
	event providers.Event
}

type hookEventMsg struct {
	event corechat.HookEvent
}

type autoReviewEventMsg struct {
	event corechat.AutoReviewEvent
}

func waitProviderEvent(ch <-chan providers.Event) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return providerEventMsg{event: ev}
	}
}

func waitHookEvent(ch <-chan corechat.HookEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return hookEventMsg{event: ev}
	}
}

func waitAutoReviewEvent(ch <-chan corechat.AutoReviewEvent) tea.Cmd {
	return func() tea.Msg {
		ev, ok := <-ch
		if !ok {
			return nil
		}
		return autoReviewEventMsg{event: ev}
	}
}

func (m *Model) applyAutocomplete() {
	m.Input = m.autocomplete.Apply(m.Input, coreautocomplete.Options{
		SessionNames: m.store.SessionNames(),
		RepoIDs:      m.store.ActiveRepoIDs(),
	})
}

func (m *Model) resetAutocomplete() {
	m.autocomplete.Reset()
}

func (m *Model) commandSuggestions(input string) []string {
	return m.autocomplete.Suggestions(input, coreautocomplete.Options{
		SessionNames: m.store.SessionNames(),
		RepoIDs:      m.store.ActiveRepoIDs(),
	})
}

func (m *Model) addSystemMessage(text string) {
	m.chat.AddSystemMessage(text)
	m.StatusText = m.chat.StatusText
}

func (m *Model) createSession(name string) *domain.Session {
	s := m.store.CreateSession(name)
	m.ActiveSessionID = m.store.ActiveSessionID
	m.StatusText = "Created session " + s.ID
	return s
}

func (m *Model) useSession(id string) bool {
	ok := m.store.UseSession(id)
	if ok {
		m.ActiveSessionID = m.store.ActiveSessionID
		m.StatusText = "Using session " + id
	}
	return ok
}

func (m *Model) addRepoToActiveSession(path, label string) error {
	err := m.store.AddRepoToActiveSession(path, label)
	if err == nil {
		repo := m.store.ActiveRepo()
		if repo != nil {
			m.StatusText = "Added repo " + repo.Path
		}
	}
	return err
}

func (m *Model) setActiveRepo(repoID string) error {
	err := m.store.SetActiveRepo(repoID)
	if err == nil {
		repo := m.store.ActiveRepo()
		if repo != nil {
			m.StatusText = "Using repo " + repo.Label
		}
	}
	return err
}

func (m *Model) activeRepo() *domain.RepoRef {
	return m.store.ActiveRepo()
}

func (m *Model) activeSession() *domain.Session {
	if m.ActiveSessionID != "" && m.ActiveSessionID != m.store.ActiveSessionID {
		_ = m.store.UseSession(m.ActiveSessionID)
	}
	m.ActiveSessionID = m.store.ActiveSessionID
	return m.store.ActiveSession()
}

func (m *Model) appendUserMessage(providerID, repoID, text string) {
	m.store.AppendUserMessage(providerID, repoID, text)
}

func (m *Model) appendAssistantStreaming(providerID, repoID, requestID string) {
	idx := m.store.AppendAssistantStreaming(providerID, repoID)
	if requestID != "" && idx >= 0 {
		m.pending[requestID] = idx
	}
}

func (m *Model) listSessionsText() string {
	return m.store.ListSessionsText()
}

func (m *Model) listReposText() string {
	return m.store.ListReposText()
}

func (m *Model) consumeStoreWarning() {
	if warn := m.store.TakePersistenceWarning(); warn != "" {
		m.StatusText = warn
	}
}

func (m *Model) handleHookEvent(ev corechat.HookEvent) {
	m.chat.HandleHookEvent(ev)
	m.ProviderState = m.chat.ProviderState
	m.StatusText = m.chat.StatusText
	m.ActiveSessionID = m.store.ActiveSessionID
	if m.AutoFollowTranscript {
		m.TranscriptScroll = 0
	}
	m.consumeStoreWarning()
}

func (m *Model) handleAutoReviewEvent(ev corechat.AutoReviewEvent) {
	m.chat.HandleAutoReviewEvent(ev)
	m.ProviderState = m.chat.ProviderState
	m.StatusText = m.chat.StatusText
	m.ActiveSessionID = m.store.ActiveSessionID
	if m.AutoFollowTranscript {
		m.TranscriptScroll = 0
	}
	m.consumeStoreWarning()
}
