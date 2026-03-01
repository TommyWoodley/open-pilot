package app

import (
	"context"
	"time"

	"github.com/thwoodle/open-pilot/internal/config"
	coreautocomplete "github.com/thwoodle/open-pilot/internal/core/autocomplete"
	corechat "github.com/thwoodle/open-pilot/internal/core/chat"
	coresession "github.com/thwoodle/open-pilot/internal/core/session"
	"github.com/thwoodle/open-pilot/internal/providers"
	"github.com/thwoodle/open-pilot/internal/ui"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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

	providerEvents   <-chan providers.Event
	hookEvents       <-chan corechat.HookEvent
	autoReviewEvents <-chan corechat.AutoReviewEvent

	ProviderState  string
	StatusText     string
	GeneratingTick int

	keys keyMap

	TranscriptScroll     int
	AutoFollowTranscript bool

	SessionSetupActive            bool
	SessionSetupAutoReviewEnabled bool
}

// NewModel returns the initial application state.
func NewModel(manager providers.Manager, cfg config.Config, persister ...coresession.Persister) Model {
	store := coresession.NewStore()
	if len(persister) > 0 && persister[0] != nil {
		store = coresession.NewStoreWithPersister(persister[0])
	}
	chat := corechat.NewEngine(store, manager, cfg)
	chat.EnableAsyncHooks()
	chat.EnableAsyncAutoReview()
	m := Model{
		store:                store,
		chat:                 chat,
		providerEvents:       chat.ProviderEvents(),
		hookEvents:           chat.HookEvents(),
		autoReviewEvents:     chat.AutoReviewEvents(),
		ProviderState:        chat.ProviderState,
		StatusText:           chat.StatusText,
		GeneratingTick:       0,
		keys:                 defaultKeyMap(),
		TranscriptScroll:     0,
		AutoFollowTranscript: true,
		ActiveSessionID:      store.ActiveSessionID,
		pending:              make(map[string]int),
	}
	if cfg.SessionPersistenceWarning != "" {
		m.StatusText = cfg.SessionPersistenceWarning
	}
	if warn := store.TakePersistenceWarning(); warn != "" {
		m.StatusText = warn
	}
	return m
}

// Init performs startup work.
func (m Model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, 4)
	if m.providerEvents != nil {
		cmds = append(cmds, waitProviderEvent(m.providerEvents))
	}
	if m.hookEvents != nil {
		cmds = append(cmds, waitHookEvent(m.hookEvents))
	}
	if m.autoReviewEvents != nil {
		cmds = append(cmds, waitAutoReviewEvent(m.autoReviewEvents))
	}
	cmds = append(cmds, generationTickCmd())
	return tea.Batch(cmds...)
}

func (m *Model) shutdownProviders(ctx context.Context) {
	m.chat.StopAll(ctx)
}

func (m Model) transcriptVisibleLines() int {
	outer := m.transcriptOuterHeight()
	inner := outer - ui.BodyStyle.GetVerticalFrameSize()
	if inner < 1 {
		return 1
	}
	return inner
}

func (m Model) maxTranscriptScroll() int {
	total := len(m.displayTranscriptLines())
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

func (m Model) transcriptOuterHeight() int {
	suggestions := m.renderSuggestions()
	suggestionsHeight := 0
	separators := 5 // header/chat (2), chat/input (2), input/status (1)
	if suggestions != "" {
		suggestionsHeight = lipgloss.Height(suggestions)
		separators = 6 // + input/suggestions (1), suggestions/status (1)
	}

	headerHeight := lipgloss.Height(ui.HeaderStyle.Render("open-pilot"))
	inputHeight := lipgloss.Height(ui.InputStyle.Width(max(m.Width-2, 50)).Render("> " + m.Input))
	statusHeight := lipgloss.Height(m.renderStatus())

	outer := m.Height - (headerHeight + inputHeight + statusHeight + suggestionsHeight + separators)
	if outer < 1 {
		return 1
	}
	return outer
}

type generationTickMsg struct{}

func generationTickCmd() tea.Cmd {
	return tea.Tick(250*time.Millisecond, func(time.Time) tea.Msg {
		return generationTickMsg{}
	})
}

func (m Model) generationDots() string {
	switch m.GeneratingTick % 3 {
	case 0:
		return "."
	case 1:
		return ".."
	default:
		return "..."
	}
}
