package app

import "github.com/charmbracelet/bubbles/key"

type keyMap struct {
	Quit      key.Binding
	Submit    key.Binding
	Backspace key.Binding
	Complete  key.Binding
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
	}
}
