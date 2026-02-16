package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/thwoodle/open-pilot/internal/config"
	"github.com/thwoodle/open-pilot/internal/domain"
	"github.com/thwoodle/open-pilot/internal/providers"

	tea "github.com/charmbracelet/bubbletea"
)

// Model is the Bubble Tea state container for the open-pilot TUI.
type Model struct {
	Width  int
	Height int
	Ready  bool

	Input string

	Sessions        map[string]*domain.Session
	SessionOrder    []string
	ActiveSessionID string

	ProviderState string
	StatusText    string

	manager        providers.Manager
	cfg            config.Config
	providerEvents <-chan providers.Event

	nextID  int
	pending map[string]int

	keys keyMap

	TranscriptScroll                int
	AutoFollowTranscript            bool
	lastRenderedTranscriptLineCount int

	completionPrefix  string
	completionOptions []string
	completionIndex   int
}

// NewModel returns the initial application state.
func NewModel(manager providers.Manager, cfg config.Config) Model {
	m := Model{
		StatusText:                      "No agent connected",
		ProviderState:                   "disconnected",
		Sessions:                        make(map[string]*domain.Session),
		SessionOrder:                    make([]string, 0),
		manager:                         manager,
		cfg:                             cfg,
		nextID:                          1,
		pending:                         make(map[string]int),
		keys:                            defaultKeyMap(),
		TranscriptScroll:                0,
		AutoFollowTranscript:            true,
		lastRenderedTranscriptLineCount: 0,
	}
	if manager != nil {
		m.providerEvents = manager.Events()
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

func (m *Model) activeSession() *domain.Session {
	if m.ActiveSessionID == "" {
		return nil
	}
	return m.Sessions[m.ActiveSessionID]
}

func (m *Model) nextMessageID(prefix string) string {
	id := prefix + "-" + strconv.Itoa(m.nextID)
	m.nextID++
	return id
}

func now() time.Time {
	return time.Now()
}

func normalizeRepoPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("repo path cannot be empty")
	}
	if !filepath.IsAbs(path) {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("failed to resolve working directory: %w", err)
		}
		path = filepath.Join(wd, path)
	}
	return filepath.Clean(path), nil
}

func (m *Model) shutdownProviders(ctx context.Context) {
	if m.manager == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = m.manager.StopAll(ctx)
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
