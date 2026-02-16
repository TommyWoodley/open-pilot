package ui

import "github.com/charmbracelet/lipgloss"

var (
	HeaderStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("205")).
			Padding(0, 1)

	BodyStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(1, 2)

	InputStyle = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(lipgloss.Color("238")).
			Padding(0, 1)

	SuggestionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245")).
			Padding(0, 1)

	FooterStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241")).
			Padding(0, 1)

	TranscriptUserPrefixStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("81"))

	TranscriptAgentPrefixStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("119"))

	TranscriptSystemPrefixStyle = lipgloss.NewStyle().
					Bold(true).
					Foreground(lipgloss.Color("214"))

	MarkdownHeadingStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("220"))

	MarkdownListStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("252"))

	InlineCodeStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("212")).
			Background(lipgloss.Color("236"))

	CodeBlockStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("239")).
			Foreground(lipgloss.Color("252")).
			Padding(0, 1)

	CodeBlockLangStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("150"))
)
