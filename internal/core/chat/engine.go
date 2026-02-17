package chat

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/thwoodle/open-pilot/internal/config"
	"github.com/thwoodle/open-pilot/internal/core/command"
	"github.com/thwoodle/open-pilot/internal/core/session"
	"github.com/thwoodle/open-pilot/internal/providers"
)

type ProviderManager interface {
	SendPrompt(ctx context.Context, providerID, sessionID, repoPath, requestID, prompt string) error
	Events() <-chan providers.Event
	StopAll(ctx context.Context) error
}

type pendingRef struct {
	SessionID string
	Index     int
}

type Engine struct {
	Store         *session.Store
	Manager       ProviderManager
	Config        config.Config
	ProviderState string
	StatusText    string
	pending       map[string]pendingRef
	unknownSeen   map[string]struct{}
}

func NewEngine(store *session.Store, manager ProviderManager, cfg config.Config) *Engine {
	return &Engine{
		Store:         store,
		Manager:       manager,
		Config:        cfg,
		ProviderState: "disconnected",
		StatusText:    "No agent connected",
		pending:       make(map[string]pendingRef),
		unknownSeen:   make(map[string]struct{}),
	}
}

func (e *Engine) ProcessInput(input string) {
	trimmed := strings.TrimSpace(input)
	if trimmed == "" {
		return
	}
	cmd, isCommand, err := command.Parse(trimmed)
	if isCommand {
		if err != nil {
			e.AddSystemMessage(err.Error())
			return
		}
		e.RunCommand(cmd)
		return
	}
	e.SendPrompt(trimmed)
}

func (e *Engine) AddSystemMessage(text string) {
	if e.Store.ActiveSession() == nil {
		e.StatusText = text
		return
	}
	e.Store.AddSystemMessage(text)
	e.StatusText = text
}

func (e *Engine) SendPrompt(input string) {
	s := e.Store.ActiveSession()
	if s == nil {
		e.AddSystemMessage("Create/select a session first: /session new <name>")
		return
	}
	if s.ProviderID == "" {
		e.AddSystemMessage("Select provider first: /provider use <codex|cursor>")
		return
	}
	repo := e.Store.ActiveRepo()
	if repo == nil {
		e.AddSystemMessage("Add/select a repo first: /session add-repo <abs-path>")
		return
	}
	if e.Manager == nil {
		e.AddSystemMessage("Provider manager is unavailable")
		return
	}

	requestID := e.Store.NextID("req")
	e.Store.AppendUserMessage(s.ProviderID, repo.ID, input)
	idx := e.Store.AppendAssistantStreaming(s.ProviderID, repo.ID)
	e.pending[requestID] = pendingRef{SessionID: s.ID, Index: idx}
	e.ProviderState = "busy"
	e.StatusText = "Sending prompt..."

	err := e.Manager.SendPrompt(context.Background(), s.ProviderID, s.ID, repo.Path, requestID, input)
	if err != nil {
		e.ProviderState = "error"
		e.AddSystemMessage("Provider send failed: " + err.Error())
	}
}

func (e *Engine) RunCommand(cmd command.Command) {
	switch cmd.Kind {
	case command.KindHelp:
		e.AddSystemMessage(command.HelpText())
	case command.KindSessionNew:
		name := strings.TrimSpace(cmd.Session)
		if name == "" {
			e.AddSystemMessage("Session name is required: /session new <name>")
			return
		}
		if e.Store.HasSessionName(name) {
			e.AddSystemMessage("Session name already exists: " + name)
			return
		}
		s := e.Store.CreateSession(name)
		if s == nil {
			e.AddSystemMessage("Failed to create session")
			return
		}
		if pcfg, ok := e.Config.Providers["codex"]; ok && pcfg.Command != "" {
			s.ProviderID = "codex"
			e.ProviderState = "starting"
			e.AddSystemMessage("Session " + s.Name + " created. Provider set to codex. Enter repo path.")
		} else {
			e.AddSystemMessage("Session " + s.Name + " created. Codex provider config missing; set provider manually.")
		}
	case command.KindSessionList:
		e.AddSystemMessage(e.Store.ListSessionsText())
	case command.KindSessionUse:
		if !e.Store.UseSession(cmd.SessionID) {
			e.AddSystemMessage("Unknown session: " + cmd.SessionID)
			return
		}
		if active := e.Store.ActiveSession(); active != nil {
			e.AddSystemMessage("Using session " + active.Name)
		} else {
			e.AddSystemMessage("Using session " + cmd.SessionID)
		}
	case command.KindSessionDelete:
		if !e.Store.DeleteSession(cmd.SessionID) {
			e.AddSystemMessage("Unknown session: " + cmd.SessionID)
			return
		}
		e.AddSystemMessage("Deleted session " + cmd.SessionID)
	case command.KindSessionAddRepo:
		if err := e.Store.AddRepoToActiveSession(cmd.RepoPath, cmd.RepoLabel); err != nil {
			e.AddSystemMessage(err.Error())
			return
		}
		e.AddSystemMessage("Repo added")
	case command.KindSessionRepos:
		e.AddSystemMessage(e.Store.ListReposText())
	case command.KindSessionRepoUse:
		if err := e.Store.SetActiveRepo(cmd.RepoID); err != nil {
			e.AddSystemMessage(err.Error())
			return
		}
		e.AddSystemMessage("Active repo set")
	case command.KindProviderUse:
		if cmd.ProviderID != "codex" && cmd.ProviderID != "cursor" {
			e.AddSystemMessage("Unsupported provider: " + cmd.ProviderID)
			return
		}
		pcfg, ok := e.Config.Providers[cmd.ProviderID]
		if !ok || pcfg.Command == "" {
			e.AddSystemMessage("Provider config missing for " + cmd.ProviderID)
			return
		}
		s := e.Store.ActiveSession()
		if s == nil {
			e.AddSystemMessage("Create/select session first: /session new <name>")
			return
		}
		s.ProviderID = cmd.ProviderID
		e.ProviderState = "starting"
		e.StatusText = "Provider set to " + cmd.ProviderID
		e.AddSystemMessage("Using provider " + cmd.ProviderID)
	case command.KindProviderStatus:
		s := e.Store.ActiveSession()
		provider := "none"
		if s != nil {
			provider = s.ProviderID
		}
		e.AddSystemMessage("provider=" + provider + " state=" + e.ProviderState)
	default:
		e.AddSystemMessage("Unknown command")
	}
}

func (e *Engine) HandleProviderEvent(ev providers.Event) {
	s := e.Store.Sessions[ev.SessionID]
	if s == nil {
		e.StatusText = ev.Message
		return
	}

	switch ev.Type {
	case providers.EventReady:
		if len(e.pending) > 0 {
			// Initial provider-ready can arrive while a request is already in-flight.
			// Keep busy state so streaming UX continues animating.
			return
		}
		e.ProviderState = "ready"
		if ev.Message != "" {
			e.StatusText = ev.Message
		} else {
			e.StatusText = "Provider ready"
		}
	case providers.EventChunk:
		ref, ok := e.pending[ev.RequestID]
		if !ok {
			return
		}
		_ = e.Store.AppendChunkAt(ref.SessionID, ref.Index, ev.Text)
	case providers.EventFinal:
		ref, ok := e.pending[ev.RequestID]
		if ok {
			if !e.Store.FinalizeAt(ref.SessionID, ref.Index, ev.Text) {
				e.Store.AddAssistantMessage(ev.SessionID, ev.Text)
			}
			delete(e.pending, ev.RequestID)
		} else {
			e.Store.AddAssistantMessage(ev.SessionID, ev.Text)
		}
		e.ProviderState = "ready"
		e.StatusText = "Response complete"
	case providers.EventError:
		e.ProviderState = "error"
		errText := ev.Message
		if errText == "" && ev.Err != nil {
			errText = ev.Err.Error()
		}
		e.Store.AddSessionSystemMessage(s.ID, "Provider error: "+errText)
		e.StatusText = "Provider error"
	case providers.EventStatus:
		e.StatusText = ev.Message
	case providers.EventReasoning:
		text := conciseReasoningText(ev.Text)
		if text != "" {
			e.Store.AddSessionSystemMessage(s.ID, "[agent-thought] "+text)
		}
	case providers.EventCommandExecution:
		e.handleCommandExecutionEvent(s.ID, ev)
	case providers.EventTurnUsage:
		// Usage is logged but intentionally not shown in transcript/status.
	case providers.EventExited:
		e.ProviderState = "error"
		msg := ev.Message
		if ev.Err != nil {
			msg += ": " + ev.Err.Error()
		}
		e.Store.AddSessionSystemMessage(s.ID, msg)
		e.StatusText = "Provider disconnected"
	default:
		rawType := strings.TrimSpace(ev.RawType)
		if rawType == "" {
			rawType = strings.TrimSpace(ev.Type)
		}
		if rawType == "" {
			rawType = "unknown"
		}
		key := s.ID + "|" + ev.Provider + "|" + rawType
		if _, seen := e.unknownSeen[key]; !seen {
			e.unknownSeen[key] = struct{}{}
			e.Store.AddSessionSystemMessage(s.ID, "Unhandled provider event '"+rawType+"' (details logged).")
		}
		e.StatusText = "Unhandled provider event: " + rawType + " (logged)"
	}
}

func (e *Engine) handleCommandExecutionEvent(sessionID string, ev providers.Event) {
	cmd := strings.TrimSpace(ev.Command)
	if cmd == "" {
		cmd = "(unknown command)"
	}
	status := strings.ToLower(strings.TrimSpace(ev.CommandStatus))
	switch status {
	case "in_progress":
		e.Store.AddSessionSystemMessage(sessionID, "Running command: "+cmd)
		return
	case "failed":
		e.Store.AddSessionSystemMessage(sessionID, fmt.Sprintf("Command failed (exit=%s): %s", formatExitCode(ev.CommandExitCode), cmd))
	default:
		if ev.CommandExitCode != nil && *ev.CommandExitCode != 0 {
			e.Store.AddSessionSystemMessage(sessionID, fmt.Sprintf("Command failed (exit=%s): %s", formatExitCode(ev.CommandExitCode), cmd))
		} else {
			e.Store.AddSessionSystemMessage(sessionID, fmt.Sprintf("Command completed (exit=%s): %s", formatExitCode(ev.CommandExitCode), cmd))
		}
	}
	if out := summarizeCommandOutput(ev.CommandOutput, 8, 500); out != "" {
		e.Store.AddSessionSystemMessage(sessionID, "Command output:\n"+out)
	}
}

func conciseReasoningText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	replacer := strings.NewReplacer("**", "", "__", "", "*", "", "_", "", "`", "")
	text = replacer.Replace(text)
	text = strings.Join(strings.Fields(text), " ")
	return truncateRunes(text, 140)
}

func summarizeCommandOutput(output string, maxLines, maxChars int) string {
	out := strings.TrimSpace(output)
	if out == "" {
		return ""
	}
	lines := strings.Split(out, "\n")
	truncated := false
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
		truncated = true
	}
	out = strings.Join(lines, "\n")
	if maxChars > 0 && utf8.RuneCountInString(out) > maxChars {
		out = truncateRunes(out, maxChars)
		truncated = true
	}
	if truncated {
		return out + "\n... (truncated)"
	}
	return out
}

func truncateRunes(input string, limit int) string {
	if limit <= 0 {
		return ""
	}
	if utf8.RuneCountInString(input) <= limit {
		return input
	}
	runes := []rune(input)
	if limit == 1 {
		return "…"
	}
	return string(runes[:limit-1]) + "…"
}

func formatExitCode(code *int) string {
	if code == nil {
		return "?"
	}
	return fmt.Sprintf("%d", *code)
}

func (e *Engine) ProviderEvents() <-chan providers.Event {
	if e.Manager == nil {
		return nil
	}
	return e.Manager.Events()
}

func (e *Engine) StopAll(ctx context.Context) {
	if e.Manager == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	_ = e.Manager.StopAll(ctx)
}
