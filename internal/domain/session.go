package domain

import "time"

// RepoRef identifies one repository attached to a session.
type RepoRef struct {
	ID    string
	Path  string
	Label string
}

// Session tracks provider context and message history.
type Session struct {
	ID           string
	Name         string
	ProviderID   string
	Repos        []RepoRef
	ActiveRepoID string
	Messages     []Message
	CreatedAt    time.Time
}
