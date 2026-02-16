package domain

import "time"

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)

// Message represents one transcript entry.
type Message struct {
	ID         string
	Role       string
	Content    string
	Timestamp  time.Time
	ProviderID string
	RepoID     string
	Streaming  bool
}
