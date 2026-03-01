package app

import (
	"fmt"

	"github.com/thwoodle/open-pilot/internal/ui"
)

func (m Model) renderStatus() string {
	session := "none"
	provider := "none"
	repo := "none"
	stateText := m.ProviderState
	if s := m.activeSession(); s != nil {
		if s.Name != "" {
			session = s.Name
		} else {
			session = s.ID
		}
		if s.ProviderID != "" {
			provider = s.ProviderID
		}
		if r := m.activeRepo(); r != nil {
			repo = r.Label
		}
		stateText = m.sessionState(s.ID)
	}
	text := fmt.Sprintf("session=%s provider=%s repo=%s state=%s | F1-F12 switch session | ↑/↓ scroll PgUp/PgDn Home/End", session, provider, repo, stateText)
	return ui.FooterStyle.Render(text)
}
