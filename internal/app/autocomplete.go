package app

import (
	"os"
	"path/filepath"
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

	options := m.tokenOptionsForContext(context, current)
	if len(options) == 0 {
		return
	}

	contextKey := strings.Join(context, "\x00") + "\x00" + current
	if m.completionPrefix != contextKey || !equalStrings(m.completionOptions, options) {
		m.completionPrefix = contextKey
		m.completionOptions = options
		start := firstMatchingIndex(options, current)
		if start < 0 {
			start = 0
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

func (m *Model) tokenOptionsForContext(context []string, current string) []string {
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
		if context[0] == "/session" && context[1] == "add-repo" {
			return pathCompletionOptions(current)
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
		"/session add-repo [path] [label]",
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

	if strings.HasPrefix(raw, "/session add-repo ") {
		pathPrefix := strings.TrimPrefix(raw, "/session add-repo ")
		for _, p := range limitStrings(pathCompletionOptions(pathPrefix), 15) {
			candidates = append(candidates, "/session add-repo "+p)
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

func pathCompletionOptions(current string) []string {
	wd, err := os.Getwd()
	if err != nil {
		return nil
	}

	absMode := strings.HasPrefix(current, string(os.PathSeparator))
	trailing := strings.HasSuffix(current, string(os.PathSeparator))

	if current == "" {
		return readDirMatches(wd, "", "", false)
	}

	dirPart := current
	prefix := ""
	if !trailing {
		dirPart = filepath.Dir(current)
		prefix = filepath.Base(current)
	}

	searchDir := dirPart
	outputBase := dirPart
	if absMode {
		if searchDir == "" {
			searchDir = string(os.PathSeparator)
			outputBase = string(os.PathSeparator)
		}
		searchDir = filepath.Clean(searchDir)
		if outputBase == "." {
			outputBase = string(os.PathSeparator)
		}
		if !strings.HasSuffix(outputBase, string(os.PathSeparator)) {
			outputBase += string(os.PathSeparator)
		}
		return readDirMatches(searchDir, outputBase, prefix, true)
	}

	if dirPart == "." || dirPart == "" {
		searchDir = wd
		if dirPart == "." {
			outputBase = "./"
		} else {
			outputBase = ""
		}
	} else {
		searchDir = filepath.Join(wd, dirPart)
		outputBase = dirPart
		if !strings.HasSuffix(outputBase, string(os.PathSeparator)) {
			outputBase += string(os.PathSeparator)
		}
	}

	return readDirMatches(searchDir, outputBase, prefix, false)
}

func readDirMatches(searchDir, outputBase, prefix string, absMode bool) []string {
	entries, err := os.ReadDir(searchDir)
	if err != nil {
		return nil
	}

	matches := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		candidate := outputBase + name
		if absMode && strings.HasPrefix(candidate, "//") {
			candidate = string(os.PathSeparator) + strings.TrimPrefix(candidate, "//")
		}
		if entry.IsDir() {
			candidate += string(os.PathSeparator)
		}
		matches = append(matches, candidate)
	}
	sort.Strings(matches)
	return matches
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

func limitStrings(values []string, limit int) []string {
	if limit <= 0 || len(values) <= limit {
		return values
	}
	return values[:limit]
}
