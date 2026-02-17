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
		case key.Matches(msg, m.keys.ScrollUp):
			m.TranscriptScroll++
			m.AutoFollowTranscript = false
			m.clampTranscriptScroll()
			return m, nil
		case key.Matches(msg, m.keys.ScrollDown):
			m.TranscriptScroll--
			if m.TranscriptScroll <= 0 {
				m.TranscriptScroll = 0
				m.AutoFollowTranscript = true
			}
			return m, nil
		case key.Matches(msg, m.keys.PageUp):
			m.TranscriptScroll += m.transcriptVisibleLines()
			m.AutoFollowTranscript = false
			m.clampTranscriptScroll()
			return m, nil
		case key.Matches(msg, m.keys.PageDown):
			m.TranscriptScroll -= m.transcriptVisibleLines()
			if m.TranscriptScroll <= 0 {
				m.TranscriptScroll = 0
				m.AutoFollowTranscript = true
			}
			return m, nil
		case key.Matches(msg, m.keys.ScrollTop):
			m.TranscriptScroll = m.maxTranscriptScroll()
			m.AutoFollowTranscript = m.TranscriptScroll == 0
			return m, nil
		case key.Matches(msg, m.keys.ScrollBottom):
			m.TranscriptScroll = 0
			m.AutoFollowTranscript = true
			return m, nil
		case key.Matches(msg, m.keys.Submit):
			m.resetAutocomplete()
			m = m.processEnter()
			return m, nil
		case key.Matches(msg, m.keys.Complete):
			m.applyAutocomplete()
			return m, nil
		case key.Matches(msg, m.keys.Backspace):
			if len(m.Input) > 0 {
				m.Input = m.Input[:len(m.Input)-1]
			}
			m.resetAutocomplete()
		default:
			if msg.Type == tea.KeyRunes || msg.Type == tea.KeySpace {
				m.Input += msg.String()
				m.resetAutocomplete()
			}
		}
	case tea.WindowSizeMsg:
		m.Width = msg.Width
		m.Height = msg.Height
		m.Ready = true
	case tea.MouseMsg:
		switch {
		case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelUp:
			m.TranscriptScroll++
			m.AutoFollowTranscript = false
			m.clampTranscriptScroll()
			return m, nil
		case msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonWheelDown:
			m.TranscriptScroll--
			if m.TranscriptScroll <= 0 {
				m.TranscriptScroll = 0
				m.AutoFollowTranscript = true
			}
			return m, nil
		}
	case providerEventMsg:
		m.handleProviderEvent(msg.event)
		if m.providerEvents != nil {
			return m, waitProviderEvent(m.providerEvents)
		}
	case hookEventMsg:
		m.handleHookEvent(msg.event)
		if m.hookEvents != nil {
			return m, waitHookEvent(m.hookEvents)
		}
	case generationTickMsg:
		if m.ProviderState == "busy" {
			m.GeneratingTick = (m.GeneratingTick + 1) % 3
		}
		return m, generationTickCmd()
	}

	return m, nil
}
