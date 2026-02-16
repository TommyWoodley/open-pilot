package app

import (
	"context"
	"strings"

	"github.com/thwoodle/open-pilot/internal/domain"
)

func (m Model) processEnter() Model {
	input := strings.TrimSpace(m.Input)
	m.Input = ""
	if input == "" {
		return m
	}

	cmd, isCommand, err := ParseCommand(input)
	if isCommand {
		if err != nil {
			m.addSystemMessage(err.Error())
			return m
		}
		m.runCommand(cmd)
		return m
	}

	s := m.activeSession()
	if s == nil {
		m.addSystemMessage("Create/select a session first: /session new <name>")
		return m
	}
	if s.ProviderID == "" {
		m.addSystemMessage("Select provider first: /provider use <codex|cursor>")
		return m
	}
	repo := m.activeRepo()
	if repo == nil {
		m.addSystemMessage("Add/select a repo first: /session add-repo <abs-path>")
		return m
	}
	if m.manager == nil {
		m.addSystemMessage("Provider manager is unavailable")
		return m
	}

	requestID := m.nextMessageID("req")
	m.appendUserMessage(s.ProviderID, repo.ID, input)
	m.appendAssistantStreaming(s.ProviderID, repo.ID, requestID)
	m.ProviderState = "busy"
	m.StatusText = "Sending prompt..."

	err = m.manager.SendPrompt(context.Background(), s.ProviderID, s.ID, repo.Path, requestID, input)
	if err != nil {
		m.ProviderState = "error"
		m.addSystemMessage("Provider send failed: " + err.Error())
	}

	return m
}

func (m *Model) runCommand(cmd Command) {
	switch cmd.Kind {
	case "help":
		m.addSystemMessage(helpText())
	case "session.new":
		s := m.createSession(cmd.Session)
		m.addSystemMessage("Session " + s.ID + " created")
	case "session.list":
		m.addSystemMessage(m.listSessionsText())
	case "session.use":
		if !m.useSession(cmd.SessionID) {
			m.addSystemMessage("Unknown session: " + cmd.SessionID)
			return
		}
		m.addSystemMessage("Using session " + cmd.SessionID)
	case "session.add-repo":
		if err := m.addRepoToActiveSession(cmd.RepoPath, cmd.RepoLabel); err != nil {
			m.addSystemMessage(err.Error())
			return
		}
		m.addSystemMessage("Repo added")
	case "session.repos":
		m.addSystemMessage(m.listReposText())
	case "session.repo.use":
		if err := m.setActiveRepo(cmd.RepoID); err != nil {
			m.addSystemMessage(err.Error())
			return
		}
		m.addSystemMessage("Active repo set")
	case "provider.use":
		if cmd.ProviderID != "codex" && cmd.ProviderID != "cursor" {
			m.addSystemMessage("Unsupported provider: " + cmd.ProviderID)
			return
		}
		pcfg, ok := m.cfg.Providers[cmd.ProviderID]
		if !ok || pcfg.Command == "" {
			m.addSystemMessage("Provider config missing for " + cmd.ProviderID)
			return
		}
		s := m.activeSession()
		if s == nil {
			m.addSystemMessage("Create/select session first: /session new <name>")
			return
		}
		s.ProviderID = cmd.ProviderID
		m.ProviderState = "starting"
		m.StatusText = "Provider set to " + cmd.ProviderID
		m.addSystemMessage("Using provider " + cmd.ProviderID)
	case "provider.status":
		s := m.activeSession()
		provider := "none"
		if s != nil {
			provider = s.ProviderID
		}
		m.addSystemMessage("provider=" + provider + " state=" + m.ProviderState)
	default:
		m.addSystemMessage("Unknown command")
	}
}

func helpText() string {
	return strings.Join([]string{
		"Commands:",
		"/session new <name>",
		"/session list",
		"/session use <id>",
		"/session add-repo <path> [label]",
		"/session repos",
		"/session repo use <repo-id>",
		"/provider use <codex|cursor>",
		"/provider status",
		"/help",
	}, "\n")
}

func (m *Model) finalizeRequest(requestID, text string) {
	s := m.activeSession()
	if s == nil {
		return
	}
	idx, ok := m.pending[requestID]
	if !ok || idx < 0 || idx >= len(s.Messages) {
		s.Messages = append(s.Messages, domain.Message{ID: m.nextMessageID("msg"), Role: domain.RoleAssistant, Content: text, Timestamp: now()})
		return
	}
	msg := s.Messages[idx]
	if strings.TrimSpace(text) != "" {
		msg.Content = text
	}
	msg.Streaming = false
	s.Messages[idx] = msg
	delete(m.pending, requestID)
}
