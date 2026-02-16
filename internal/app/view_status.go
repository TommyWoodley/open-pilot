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
	}
	statusText := m.StatusText
	if m.ProviderState == "busy" {
		statusText += m.generationDots()
	}
	text := fmt.Sprintf("session=%s provider=%s repo=%s state=%s | %s | ↑/↓ scroll PgUp/PgDn Home/End", session, provider, repo, m.ProviderState, statusText)
	return ui.FooterStyle.Render(text)
}
