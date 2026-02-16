package app

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Quit         key.Binding
	Submit       key.Binding
	Backspace    key.Binding
	Complete     key.Binding
	ScrollUp     key.Binding
	ScrollDown   key.Binding
	PageUp       key.Binding
	PageDown     key.Binding
	ScrollTop    key.Binding
	ScrollBottom key.Binding
}

func defaultKeyMap() keyMap {
	return keyMap{
		Quit: key.NewBinding(
			key.WithKeys("q", "ctrl+c"),
			key.WithHelp("q", "quit"),
		),
		Submit: key.NewBinding(
			key.WithKeys("enter"),
		),
		Backspace: key.NewBinding(
			key.WithKeys("backspace", "ctrl+h"),
		),
		Complete: key.NewBinding(
			key.WithKeys("tab"),
			key.WithHelp("tab", "autocomplete"),
		),
		ScrollUp: key.NewBinding(
			key.WithKeys("up"),
		),
		ScrollDown: key.NewBinding(
			key.WithKeys("down"),
		),
		PageUp: key.NewBinding(
			key.WithKeys("pgup"),
		),
		PageDown: key.NewBinding(
			key.WithKeys("pgdown"),
		),
		ScrollTop: key.NewBinding(
			key.WithKeys("home"),
		),
		ScrollBottom: key.NewBinding(
			key.WithKeys("end"),
		),
	}
}
