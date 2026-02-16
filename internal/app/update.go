package app

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles input/events and returns updated model state.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch {
		case key.Matches(msg, m.keys.Quit):
			m.shutdownProviders(nil)
			return m, tea.Quit
		case key.Matches(msg, m.keys.Submit):
			m = m.processEnter()
			return m, nil
		case key.Matches(msg, m.keys.Backspace):
			if len(m.Input) > 0 {
				m.Input = m.Input[:len(m.Input)-1]
			}
		default:
			if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
				m.Input += msg.String()
			}
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Ready = true
	case providerEventMsg:
		m.handleProviderEvent(msg.event)
		if m.providerEvents != nil {
			return m, waitProviderEvent(m.providerEvents)
		}
	}

	return m, nil
}
