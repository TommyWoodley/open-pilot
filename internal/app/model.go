package app

import (
	"context"

	"github.com/thwoodle/open-pilot/internal/config"
	coreautocomplete "github.com/thwoodle/open-pilot/internal/core/autocomplete"
	corechat "github.com/thwoodle/open-pilot/internal/core/chat"
	coresession "github.com/thwoodle/open-pilot/internal/core/session"
	"github.com/thwoodle/open-pilot/internal/providers"

	tea "github.com/charmbracelet/bubbletea"
)

// Model is the Bubble Tea state container for the open-pilot TUI.
type Model struct {
	Width  int
	Height int
	Ready  bool

	Input string

	store        *coresession.Store
	chat         *corechat.Engine
	autocomplete coreautocomplete.Engine

	// Compatibility fields retained for legacy tests/callers.
	ActiveSessionID string
	pending         map[string]int

	providerEvents <-chan providers.Event

	ProviderState string
	StatusText    string

	keys keyMap

	TranscriptScroll     int
	AutoFollowTranscript bool
}

// NewModel returns the initial application state.
func NewModel(manager providers.Manager, cfg config.Config) Model {
	store := coresession.NewStore()
	chat := corechat.NewEngine(store, manager, cfg)
	m := Model{
		store:                store,
		chat:                 chat,
		providerEvents:       chat.ProviderEvents(),
		ProviderState:        chat.ProviderState,
		StatusText:           chat.StatusText,
		keys:                 defaultKeyMap(),
		TranscriptScroll:     0,
		AutoFollowTranscript: true,
		ActiveSessionID:      store.ActiveSessionID,
		pending:              make(map[string]int),
	}
	return m
}

// Init performs startup work.
func (m Model) Init() tea.Cmd {
	if m.providerEvents == nil {
		return nil
	}
	return waitProviderEvent(m.providerEvents)
}

func (m *Model) shutdownProviders(ctx context.Context) {
	m.chat.StopAll(ctx)
}

func (m Model) transcriptVisibleLines() int {
	return max(m.Height-8, 6)
}

func (m Model) maxTranscriptScroll() int {
	total := len(m.buildTranscriptLines())
	visible := m.transcriptVisibleLines()
	if total <= visible {
		return 0
	}
	return total - visible
}

func (m *Model) clampTranscriptScroll() {
	if m.TranscriptScroll < 0 {
		m.TranscriptScroll = 0
	}
	maxScroll := m.maxTranscriptScroll()
	if m.TranscriptScroll > maxScroll {
		m.TranscriptScroll = maxScroll
	}
	if m.TranscriptScroll == 0 {
		m.AutoFollowTranscript = true
	}
}
