package app

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/thwoodle/open-pilot/internal/config"
	"github.com/thwoodle/open-pilot/internal/domain"
	"github.com/thwoodle/open-pilot/internal/providers"
)

func buildScrollTestModel(messageCount int) Model {
	m := NewModel(nil, config.Default())
	s := m.createSession("demo")
	m.ActiveSessionID = s.ID
	for i := 1; i <= messageCount; i++ {
		s.Messages = append(s.Messages, domain.Message{
			ID:      fmt.Sprintf("msg-%d", i),
			Role:    domain.RoleAssistant,
			Content: fmt.Sprintf("line-%02d", i),
		})
	}
	m.Width = 120
	m.Height = 12
	return m
}

func TestTranscriptViewportShowsLatestByDefault(t *testing.T) {
	t.Parallel()

	m := buildScrollTestModel(8)
	rendered := m.renderTranscript()

	if !strings.Contains(rendered, "line-08") {
		t.Fatalf("expected latest line in transcript")
	}
	if strings.Contains(rendered, "line-01") {
		t.Fatalf("expected oldest line to be out of viewport by default")
	}
}

func TestTranscriptHomeAndEndNavigation(t *testing.T) {
	t.Parallel()

	m := buildScrollTestModel(8)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyHome})
	m = updated.(Model)
	if m.TranscriptScroll != m.maxTranscriptScroll() {
		t.Fatalf("expected home to jump to top, got scroll=%d max=%d", m.TranscriptScroll, m.maxTranscriptScroll())
	}
	if m.AutoFollowTranscript {
		t.Fatalf("expected auto-follow to be disabled at top")
	}
	if !strings.Contains(m.renderTranscript(), "line-01") {
		t.Fatalf("expected oldest lines visible after Home")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnd})
	m = updated.(Model)
	if m.TranscriptScroll != 0 || !m.AutoFollowTranscript {
		t.Fatalf("expected End to restore bottom follow mode")
	}
	if !strings.Contains(m.renderTranscript(), "line-08") {
		t.Fatalf("expected latest lines visible after End")
	}
}

func TestTranscriptPageNavigationAndClamp(t *testing.T) {
	t.Parallel()

	m := buildScrollTestModel(12)
	maxScroll := m.maxTranscriptScroll()

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyPgUp})
	m = updated.(Model)
	if m.TranscriptScroll == 0 {
		t.Fatalf("expected PageUp to move transcript up")
	}
	if m.TranscriptScroll > maxScroll {
		t.Fatalf("expected scroll clamp <= %d, got %d", maxScroll, m.TranscriptScroll)
	}
	if m.AutoFollowTranscript {
		t.Fatalf("expected auto-follow off after PageUp")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	m = updated.(Model)
	if m.TranscriptScroll < 0 {
		t.Fatalf("scroll cannot be negative")
	}
}

func TestTypingStillTargetsInputWhileScrolled(t *testing.T) {
	t.Parallel()

	m := buildScrollTestModel(8)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = updated.(Model)
	if m.TranscriptScroll == 0 {
		t.Fatalf("expected non-zero scroll after Up")
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("h")})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(Model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(Model)

	if m.Input != "h i" {
		t.Fatalf("expected typing to continue targeting input, got %q", m.Input)
	}
}

func TestAutoFollowBehaviorOnNewProviderMessage(t *testing.T) {
	t.Parallel()

	m := buildScrollTestModel(4)
	m.AutoFollowTranscript = true
	m.TranscriptScroll = 3
	m.handleProviderEvent(providers.Event{Type: providers.EventStatus, Message: "working"})
	if m.TranscriptScroll != 0 {
		t.Fatalf("expected auto-follow mode to stay pinned to bottom")
	}

	m.AutoFollowTranscript = false
	m.TranscriptScroll = 2
	m.handleProviderEvent(providers.Event{Type: providers.EventStatus, Message: "still working"})
	if m.TranscriptScroll != 2 {
		t.Fatalf("expected manual scroll position to remain stable, got %d", m.TranscriptScroll)
	}
}
