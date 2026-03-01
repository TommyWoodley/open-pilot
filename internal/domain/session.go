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
	ID                    string
	Name                  string
	ProviderID            string
	CodexThreadID         string
	AutoReviewLoopEnabled bool
	Repos                 []RepoRef
	ActiveRepoID          string
	HooksBlocked          bool
	HooksBlockReason      string
	LastHookRunAt         time.Time
	Messages              []Message
	CreatedAt             time.Time
}
