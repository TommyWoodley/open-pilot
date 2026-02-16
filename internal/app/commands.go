package app

import (
	"errors"
	"strings"
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

// ParseCommand parses slash-prefixed input.
func ParseCommand(input string) (Command, bool, error) {
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
		return Command{Kind: "help"}, true, nil
	case "/provider":
		if len(parts) == 2 && parts[1] == "status" {
			return Command{Kind: "provider.status"}, true, nil
		}
		if len(parts) == 3 && parts[1] == "use" {
			return Command{Kind: "provider.use", ProviderID: strings.ToLower(parts[2])}, true, nil
		}
		return Command{}, true, errors.New("usage: /provider use <codex|cursor> OR /provider status")
	case "/session":
		if len(parts) == 2 && parts[1] == "list" {
			return Command{Kind: "session.list"}, true, nil
		}
		if len(parts) >= 3 && parts[1] == "new" {
			return Command{Kind: "session.new", Session: strings.Join(parts[2:], " ")}, true, nil
		}
		if len(parts) == 3 && parts[1] == "use" {
			return Command{Kind: "session.use", SessionID: parts[2]}, true, nil
		}
		if len(parts) >= 3 && parts[1] == "add-repo" {
			label := ""
			if len(parts) > 3 {
				label = strings.Join(parts[3:], " ")
			}
			return Command{Kind: "session.add-repo", RepoPath: parts[2], RepoLabel: label}, true, nil
		}
		if len(parts) == 2 && parts[1] == "repos" {
			return Command{Kind: "session.repos"}, true, nil
		}
		if len(parts) == 4 && parts[1] == "repo" && parts[2] == "use" {
			return Command{Kind: "session.repo.use", RepoID: parts[3]}, true, nil
		}
		return Command{}, true, errors.New("usage: /session <new|list|use|add-repo|repos|repo use>")
	default:
		return Command{}, true, errors.New("unknown command; run /help")
	}
}
