package providers

import "context"

// SessionHandle identifies a running provider subprocess.
type SessionHandle string

// StartRequest is the input for starting a provider process.
type StartRequest struct {
	SessionID string
	Provider  string
	RepoPath  string
}

// PromptRequest is one user prompt bound to a target repository.
type PromptRequest struct {
	ID        string
	SessionID string
	Text      string
	RepoPath  string
}

// Adapter wraps provider process lifecycle and IO.
type Adapter interface {
	ProviderID() string
	Start(ctx context.Context, req StartRequest) (SessionHandle, error)
	Stop(ctx context.Context, handle SessionHandle) error
	Send(ctx context.Context, handle SessionHandle, prompt PromptRequest) error
	Events(handle SessionHandle) <-chan Event
}
