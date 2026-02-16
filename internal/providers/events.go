package providers

import "errors"

const (
	EventReady  = "ready"
	EventChunk  = "chunk"
	EventFinal  = "final"
	EventError  = "error"
	EventStatus = "status"
	EventExited = "exited"
)

// Event is a normalized provider event.
type Event struct {
	Type      string
	SessionID string
	Provider  string
	RepoPath  string
	RequestID string
	Text      string
	Message   string
	Err       error
}

func parseWrapperEvent(line []byte) (Event, error) {
	var raw struct {
		Type    string `json:"type"`
		ID      string `json:"id"`
		Text    string `json:"text"`
		Message string `json:"message"`
	}
	if err := unmarshalJSON(line, &raw); err != nil {
		return Event{}, err
	}
	if raw.Type == "" {
		return Event{}, errors.New("missing event type")
	}
	return Event{Type: raw.Type, RequestID: raw.ID, Text: raw.Text, Message: raw.Message}, nil
}
