package autocomplete

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/thwoodle/open-pilot/internal/core/command"
)

type Engine struct {
	completionPrefix  string
	completionOptions []string
	completionIndex   int
}

type Options struct {
	SessionIDs []string
	RepoIDs    []string
}

func (e *Engine) Reset() {
	e.completionPrefix = ""
	e.completionOptions = nil
	e.completionIndex = 0
}

func (e *Engine) Apply(input string, opt Options) string {
	raw := strings.TrimLeft(input, " \t")
	if !strings.HasPrefix(raw, "/") {
		return input
	}

	tokens, trailing := splitTokens(raw)
	if len(tokens) == 0 {
		return input
	}

	if trailing {
		tokens = append(tokens, "")
	}
	idx := len(tokens) - 1
	context := tokens[:idx]
	current := tokens[idx]

	options := tokenOptionsForContext(context, current, opt)
	if len(options) == 0 {
		return input
	}

	contextKey := strings.Join(context, "\x00") + "\x00" + current
	if e.completionPrefix != contextKey || !equalStrings(e.completionOptions, options) {
		e.completionPrefix = contextKey
		e.completionOptions = options
		start := firstMatchingIndex(options, current)
		if start < 0 {
			start = 0
		}
		e.completionIndex = start
	}

	if len(e.completionOptions) == 0 {
		return input
	}
	if e.completionIndex >= len(e.completionOptions) {
		e.completionIndex = 0
	}

	chosen := e.completionOptions[e.completionIndex]
	e.completionIndex = (e.completionIndex + 1) % len(e.completionOptions)

	tokens[idx] = chosen
	return strings.Join(tokens, " ") + " "
}

func (e *Engine) Suggestions(input string, opt Options) []string {
	raw := strings.TrimLeft(input, " \t")
	if !strings.HasPrefix(raw, "/") {
		return nil
	}

	candidates := append([]string{}, command.BaseSuggestions()...)
	for _, id := range opt.SessionIDs {
		candidates = append(candidates, "/session use "+id)
	}
	for _, repoID := range opt.RepoIDs {
		candidates = append(candidates, "/session repo use "+repoID)
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
	return command.SortAndDedupe(filtered)
}

func tokenOptionsForContext(context []string, current string, opt Options) []string {
	switch len(context) {
	case 0:
		return command.RootSuggestions()
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
			ids := append([]string{}, opt.SessionIDs...)
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
			ids := append([]string{}, opt.RepoIDs...)
			sort.Strings(ids)
			return ids
		}
	}
	return nil
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
