package app

import (
	"sort"
	"strings"
)

func (m *Model) applyAutocomplete() {
	raw := strings.TrimLeft(m.Input, " \t")
	if !strings.HasPrefix(raw, "/") {
		return
	}

	tokens, trailing := splitTokens(raw)
	if len(tokens) == 0 {
		return
	}

	if trailing {
		tokens = append(tokens, "")
	}
	idx := len(tokens) - 1
	context := tokens[:idx]
	current := tokens[idx]

	options := m.tokenOptionsForContext(context)
	if len(options) == 0 {
		return
	}

	contextKey := strings.Join(context, "\x00")
	if m.completionPrefix != contextKey || !equalStrings(m.completionOptions, options) {
		m.completionPrefix = contextKey
		m.completionOptions = options
		start := firstMatchingIndex(options, current)
		if start < 0 {
			return
		}
		m.completionIndex = start
	}

	if len(m.completionOptions) == 0 {
		return
	}
	if m.completionIndex >= len(m.completionOptions) {
		m.completionIndex = 0
	}

	chosen := m.completionOptions[m.completionIndex]
	m.completionIndex = (m.completionIndex + 1) % len(m.completionOptions)

	tokens[idx] = chosen
	m.Input = strings.Join(tokens, " ") + " "
}

func (m *Model) resetAutocomplete() {
	m.completionPrefix = ""
	m.completionOptions = nil
	m.completionIndex = 0
}

func (m *Model) tokenOptionsForContext(context []string) []string {
	switch len(context) {
	case 0:
		return []string{"/help", "/provider", "/session"}
	case 1:
		switch context[0] {
		case "/provider":
			return []string{"status", "use"}
		case "/session":
			return []string{"add-repo", "list", "new", "repo", "repos", "use"}
		}
	case 2:
		if context[0] == "/provider" && context[1] == "use" {
			return []string{"codex", "cursor"}
		}
		if context[0] == "/session" && context[1] == "use" {
			ids := make([]string, 0, len(m.SessionOrder))
			for _, id := range m.SessionOrder {
				ids = append(ids, id)
			}
			sort.Strings(ids)
			return ids
		}
		if context[0] == "/session" && context[1] == "repo" {
			return []string{"use"}
		}
	case 3:
		if context[0] == "/session" && context[1] == "repo" && context[2] == "use" {
			s := m.activeSession()
			if s == nil {
				return nil
			}
			ids := make([]string, 0, len(s.Repos))
			for _, repo := range s.Repos {
				ids = append(ids, repo.ID)
			}
			sort.Strings(ids)
			return ids
		}
	}

	return nil
}

func (m *Model) commandSuggestions(input string) []string {
	raw := strings.TrimLeft(input, " \t")
	if !strings.HasPrefix(raw, "/") {
		return nil
	}

	candidates := []string{
		"/help",
		"/provider status",
		"/provider use codex",
		"/provider use cursor",
		"/session new <name>",
		"/session list",
		"/session use <session-id>",
		"/session add-repo <abs-path> [label]",
		"/session repos",
		"/session repo use <repo-id>",
	}

	for _, id := range m.SessionOrder {
		candidates = append(candidates, "/session use "+id)
	}
	if s := m.activeSession(); s != nil {
		for _, repo := range s.Repos {
			candidates = append(candidates, "/session repo use "+repo.ID)
		}
	}

	filtered := make([]string, 0, len(candidates))
	for _, c := range candidates {
		if raw == "/" || strings.HasPrefix(c, raw) {
			filtered = append(filtered, c)
		}
	}

	sort.Strings(filtered)
	return dedupeStrings(filtered)
}

func splitTokens(input string) ([]string, bool) {
	trimLeft := strings.TrimLeft(input, " \t")
	trailing := strings.HasSuffix(trimLeft, " ")
	parts := strings.Fields(trimLeft)
	return parts, trailing
}

func firstMatchingIndex(options []string, prefix string) int {
	if prefix == "" {
		return 0
	}
	for i, opt := range options {
		if strings.HasPrefix(opt, prefix) {
			return i
		}
	}
	return -1
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return values
	}
	out := []string{values[0]}
	for i := 1; i < len(values); i++ {
		if values[i] != values[i-1] {
			out = append(out, values[i])
		}
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
