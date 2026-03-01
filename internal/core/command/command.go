package command

import (
	"errors"
	"sort"
	"strings"
)

const (
	KindHelp           = "help"
	KindHooksRun       = "hooks.run"
	KindReview         = "review"
	KindProviderStatus = "provider.status"
	KindProviderUse    = "provider.use"
	KindSessionList    = "session.list"
	KindSessionNew     = "session.new"
	KindSessionUse     = "session.use"
	KindSessionDelete  = "session.delete"
	KindSessionAddRepo = "session.add-repo"
	KindSessionRepos   = "session.repos"
	KindSessionRepoUse = "session.repo.use"
)

// Command is a parsed slash command.
type Command struct {
	Kind       string
	SessionID  string
	Session    string
	RepoPath   string
	RepoLabel  string
	RepoID     string
	ProviderID string
}

// Parse parses slash-prefixed input.
func Parse(input string) (Command, bool, error) {
	trimmed := strings.TrimSpace(input)
	if !strings.HasPrefix(trimmed, "/") {
		return Command{}, false, nil
	}
	parts := strings.Fields(trimmed)
	if len(parts) == 0 {
		return Command{}, true, errors.New("empty command")
	}

	switch parts[0] {
	case "/help":
		return Command{Kind: KindHelp}, true, nil
	case "/review":
		if len(parts) == 1 {
			return Command{Kind: KindReview}, true, nil
		}
		return Command{}, true, errors.New("usage: /review")
	case "/hooks":
		if len(parts) == 2 && parts[1] == "run" {
			return Command{Kind: KindHooksRun}, true, nil
		}
		return Command{}, true, errors.New("usage: /hooks run")
	case "/provider":
		if len(parts) == 2 && parts[1] == "status" {
			return Command{Kind: KindProviderStatus}, true, nil
		}
		if len(parts) == 3 && parts[1] == "use" {
			return Command{Kind: KindProviderUse, ProviderID: strings.ToLower(parts[2])}, true, nil
		}
		return Command{}, true, errors.New("usage: /provider use <codex|cursor> OR /provider status")
	case "/session":
		if len(parts) == 2 && parts[1] == "list" {
			return Command{Kind: KindSessionList}, true, nil
		}
		if len(parts) >= 3 && parts[1] == "new" {
			return Command{Kind: KindSessionNew, Session: strings.Join(parts[2:], " ")}, true, nil
		}
		if len(parts) >= 3 && parts[1] == "use" {
			return Command{Kind: KindSessionUse, SessionID: strings.Join(parts[2:], " ")}, true, nil
		}
		if len(parts) >= 3 && (parts[1] == "delete" || parts[1] == "remove" || parts[1] == "destroy") {
			return Command{Kind: KindSessionDelete, SessionID: strings.Join(parts[2:], " ")}, true, nil
		}
		if len(parts) == 2 && parts[1] == "add-repo" {
			return Command{Kind: KindSessionAddRepo}, true, nil
		}
		if len(parts) >= 3 && parts[1] == "add-repo" {
			label := ""
			if len(parts) > 3 {
				label = strings.Join(parts[3:], " ")
			}
			return Command{Kind: KindSessionAddRepo, RepoPath: parts[2], RepoLabel: label}, true, nil
		}
		if len(parts) == 2 && parts[1] == "repos" {
			return Command{Kind: KindSessionRepos}, true, nil
		}
		if len(parts) == 4 && parts[1] == "repo" && parts[2] == "use" {
			return Command{Kind: KindSessionRepoUse, RepoID: parts[3]}, true, nil
		}
		return Command{}, true, errors.New("usage: /session <new|list|use|delete|add-repo <path>|repos|repo use>")
	default:
		return Command{}, true, errors.New("unknown command; run /help")
	}
}

func HelpText() string {
	return strings.Join([]string{
		"Commands:",
		"/session new <name> (auto-sets provider=codex and prompts repo step)",
		"/session list",
		"/session use <name>",
		"/session delete <name> (aliases: remove, destroy)",
		"/session add-repo [path] [label] (empty path => current working directory)",
		"/session repos",
		"/session repo use <repo-id>",
		"/provider use <codex|cursor>",
		"/provider status",
		"/review",
		"/hooks run",
		"/help",
		"Navigation: F1-F12 switch sessions, Up/Down/PgUp/PgDn/Home/End scroll transcript",
	}, "\n")
}

func RootSuggestions() []string {
	return []string{"/help", "/hooks", "/provider", "/review", "/session"}
}

func BaseSuggestions() []string {
	return []string{
		"/help",
		"/review",
		"/hooks run",
		"/provider status",
		"/provider use codex",
		"/provider use cursor",
		"/session new <name>",
		"/session list",
		"/session use <session-name>",
		"/session delete <session-name>",
		"/session add-repo [path] [label]",
		"/session repos",
		"/session repo use <repo-id>",
	}
}

func SortAndDedupe(values []string) []string {
	sort.Strings(values)
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
