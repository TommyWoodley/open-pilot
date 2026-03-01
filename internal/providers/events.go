package providers

import "errors"

const (
	EventReady            = "ready"
	EventChunk            = "chunk"
	EventFinal            = "final"
	EventError            = "error"
	EventStatus           = "status"
	EventExited           = "exited"
	EventUnknown          = "unknown"
	EventReasoning        = "reasoning"
	EventCommandExecution = "command_execution"
	EventAgentMessage     = "agent_message"
	EventToolCall         = "tool_call"
	EventTurnUsage        = "turn.usage"
)

// Event is a normalized provider event.
type Event struct {
	Type                   string
	SessionID              string
	Provider               string
	RepoPath               string
	RequestID              string
	Text                   string
	Message                string
	RawType                string
	RawJSON                string
	DebugNote              string
	ItemType               string
	ItemID                 string
	Command                string
	CommandStatus          string
	CommandExitCode        *int
	CommandOutput          string
	ProviderThreadID       string
	UsageInputTokens       int
	UsageCachedInputTokens int
	UsageOutputTokens      int
	Err                    error
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
	if isKnownEventType(raw.Type) {
		return Event{Type: raw.Type, RequestID: raw.ID, Text: raw.Text, Message: raw.Message}, nil
	}
	return Event{
		Type:      EventUnknown,
		RequestID: raw.ID,
		Text:      raw.Text,
		Message:   raw.Message,
		RawType:   raw.Type,
		RawJSON:   string(line),
		DebugNote: "wrapper event type not recognized",
	}, nil
}

func isKnownEventType(eventType string) bool {
	switch eventType {
	case EventReady, EventChunk, EventFinal, EventError, EventStatus, EventExited, EventReasoning, EventCommandExecution, EventAgentMessage, EventToolCall, EventTurnUsage:
		return true
	default:
		return false
	}
}
