package app

import (
	"github.com/thwoodle/open-pilot/internal/domain"
	"github.com/thwoodle/open-pilot/internal/providers"

	tea "github.com/charmbracelet/bubbletea"
)

type providerEventMsg struct {
	event providers.Event
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

func (m *Model) handleProviderEvent(ev providers.Event) {
	s := m.activeSession()
	if s == nil {
		m.StatusText = ev.Message
		return
	}

	switch ev.Type {
	case providers.EventReady:
		m.ProviderState = "ready"
		if ev.Message != "" {
			m.StatusText = ev.Message
		} else {
			m.StatusText = "Provider ready"
		}
	case providers.EventChunk:
		idx, ok := m.pending[ev.RequestID]
		if !ok || idx < 0 || idx >= len(s.Messages) {
			return
		}
		msg := s.Messages[idx]
		msg.Content += ev.Text
		s.Messages[idx] = msg
	case providers.EventFinal:
		m.finalizeRequest(ev.RequestID, ev.Text)
		m.ProviderState = "ready"
		m.StatusText = "Response complete"
	case providers.EventError:
		m.ProviderState = "error"
		errText := ev.Message
		if errText == "" && ev.Err != nil {
			errText = ev.Err.Error()
		}
		s.Messages = append(s.Messages, domain.Message{
			ID:        m.nextMessageID("msg"),
			Role:      domain.RoleSystem,
			Content:   "Provider error: " + errText,
			Timestamp: now(),
		})
		m.StatusText = "Provider error"
	case providers.EventStatus:
		m.StatusText = ev.Message
	case providers.EventExited:
		m.ProviderState = "error"
		msg := ev.Message
		if ev.Err != nil {
			msg += ": " + ev.Err.Error()
		}
		s.Messages = append(s.Messages, domain.Message{
			ID:        m.nextMessageID("msg"),
			Role:      domain.RoleSystem,
			Content:   msg,
			Timestamp: now(),
		})
		m.StatusText = "Provider disconnected"
	}

	if m.AutoFollowTranscript {
		m.TranscriptScroll = 0
	}
}
