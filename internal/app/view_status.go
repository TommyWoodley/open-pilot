package app

import (
	"fmt"

	"github.com/thwoodle/open-pilot/internal/ui"
)

func (m Model) renderStatus() string {
	session := "none"
	provider := "none"
	repo := "none"
	if s := m.activeSession(); s != nil {
		session = s.ID
		provider = s.ProviderID
		if r := m.activeRepo(); r != nil {
			repo = r.Label
		}
	}
	text := fmt.Sprintf("session=%s provider=%s repo=%s state=%s | %s", session, provider, repo, m.ProviderState, m.StatusText)
	return ui.FooterStyle.Render(text)
}
