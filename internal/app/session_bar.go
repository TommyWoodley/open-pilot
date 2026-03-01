package app

import (
	"fmt"
	"strings"

	"github.com/thwoodle/open-pilot/internal/domain"
	"github.com/thwoodle/open-pilot/internal/ui"
)

const maxFunctionKeySessions = 12

func (m Model) renderSessionBar() string {
	if len(m.store.SessionOrder) == 0 {
		return ui.FooterStyle.Render("sessions: none")
	}

	parts := make([]string, 0, min(len(m.store.SessionOrder), maxFunctionKeySessions))
	for i := 0; i < len(m.store.SessionOrder) && i < maxFunctionKeySessions; i++ {
		sessionID := m.store.SessionOrder[i]
		s := m.store.Sessions[sessionID]
		if s == nil {
			continue
		}
		keyLabel := fmt.Sprintf("F%d", i+1)
		if sessionID == m.store.ActiveSessionID {
			keyLabel += "*"
		}
		parts = append(parts, fmt.Sprintf("%s:%s:%s", keyLabel, sessionDisplayName(s), m.sessionState(sessionID)))
	}

	return ui.FooterStyle.Render("sessions " + strings.Join(parts, " | "))
}

func (m Model) sessionState(sessionID string) string {
	s := m.store.Sessions[sessionID]
	if s == nil {
		return "ready"
	}
	if s.HooksBlocked && strings.EqualFold(strings.TrimSpace(s.HooksBlockReason), "running") {
		return "in-progress"
	}
	for _, msg := range s.Messages {
		if msg.Streaming {
			return "in-progress"
		}
	}
	return "ready"
}

func (m Model) sessionIDForFunctionKey(index int) (string, bool) {
	if index < 0 || index >= maxFunctionKeySessions || index >= len(m.store.SessionOrder) {
		return "", false
	}
	return m.store.SessionOrder[index], true
}

func sessionDisplayName(s *domain.Session) string {
	if s == nil {
		return ""
	}
	name := strings.TrimSpace(s.Name)
	if name != "" {
		return name
	}
	return s.ID
}

func functionKeyIndex(key string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "f1":
		return 0, true
	case "f2":
		return 1, true
	case "f3":
		return 2, true
	case "f4":
		return 3, true
	case "f5":
		return 4, true
	case "f6":
		return 5, true
	case "f7":
		return 6, true
	case "f8":
		return 7, true
	case "f9":
		return 8, true
	case "f10":
		return 9, true
	case "f11":
		return 10, true
	case "f12":
		return 11, true
	default:
		return 0, false
	}
}
