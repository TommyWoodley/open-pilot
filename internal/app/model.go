package app

import tea "github.com/charmbracelet/bubbletea"

// Model is the Bubble Tea state container for the open-pilot TUI.
type Model struct {
	Width      int
	Height     int
	Ready      bool
	StatusText string
	FocusIndex int
}

// NewModel returns the initial application state.
func NewModel() Model {
	return Model{
		StatusText: "No agent connected",
	}
}

// Init performs startup work. Bootstrap has no startup command.
func (m Model) Init() tea.Cmd {
	return nil
}
