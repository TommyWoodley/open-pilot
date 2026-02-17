package chat

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/thwoodle/open-pilot/internal/config"
	"github.com/thwoodle/open-pilot/internal/core/command"
	corehooks "github.com/thwoodle/open-pilot/internal/core/hooks"
	"github.com/thwoodle/open-pilot/internal/core/session"
	"github.com/thwoodle/open-pilot/internal/domain"
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

type itemRef struct {
	SessionID string
	ItemID    string
}

type commandRunState struct {
	startedAt  time.Time
	command    string
	lastOutput string
	status     string
}

type Engine struct {
	Store         *session.Store
	Manager       ProviderManager
	Hooks         corehooks.Service
	Config        config.Config
	ProviderState string
	StatusText    string
	pending       map[string]pendingRef
	itemRefs      map[string]pendingRef
	commandRuns   map[itemRef]commandRunState
	unknownSeen   map[string]struct{}
	nowFn         func() time.Time
}

func NewEngine(store *session.Store, manager ProviderManager, cfg config.Config) *Engine {
	return &Engine{
		Store:         store,
		Manager:       manager,
		Hooks:         corehooks.NewService(cfg.BuiltinHooks, cfg.BuiltinHooksLoadError),
		Config:        cfg,
		ProviderState: "disconnected",
		StatusText:    "No agent connected",
		pending:       make(map[string]pendingRef),
		itemRefs:      make(map[string]pendingRef),
		commandRuns:   make(map[itemRef]commandRunState),
		unknownSeen:   make(map[string]struct{}),
		nowFn:         time.Now,
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
	if s.HooksBlocked {
		reason := strings.TrimSpace(s.HooksBlockReason)
		if reason == "" {
			reason = "startup hooks failed"
		}
		e.AddSystemMessage("Prompts blocked for this session until hooks pass. Run /hooks run after fixes. Last error: " + reason)
		e.StatusText = "Hooks blocked"
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
		e.runHooks(s, config.HookTriggerSessionStarted, "")
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
		repo := e.Store.ActiveRepo()
		repoPath := ""
		if repo != nil {
			repoPath = repo.Path
		}
		if s := e.Store.ActiveSession(); s != nil {
			e.runHooks(s, config.HookTriggerRepoAdded, repoPath)
		}
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
	case command.KindHooksRun:
		s := e.Store.ActiveSession()
		if s == nil {
			e.AddSystemMessage("Create/select a session first: /session new <name>")
			return
		}
		e.runHooks(s, config.HookTriggerSessionStarted, "")
	default:
		e.AddSystemMessage("Unknown command")
	}
}

func (e *Engine) runHooks(s *domain.Session, trigger config.HookTrigger, repoPath string) {
	if s == nil {
		return
	}
	if e.Hooks == nil {
		s.HooksBlocked = false
		s.HooksBlockReason = ""
		s.LastHookRunAt = e.nowFn()
		return
	}
	e.StatusText = "Running hooks..."
	result := e.Hooks.Run(context.Background(), trigger, s.ID, repoPath)
	s.LastHookRunAt = e.nowFn()
	e.AddSystemMessage(fmt.Sprintf("Running hooks (%d)...", result.HooksMatched))
	for _, hookResult := range result.PerHookResults {
		e.AddSystemMessage("Hook start: " + hookResult.HookID)
		if hookResult.Passed {
			e.AddSystemMessage("Hook passed: " + hookResult.HookID)
			continue
		}
		if hookResult.HookID == result.FailedHookID && result.FailedCommandIndex > 0 {
			e.AddSystemMessage(fmt.Sprintf("Hook failed: %s (command %d, %s)", hookResult.HookID, result.FailedCommandIndex, hookResult.Reason))
		} else {
			e.AddSystemMessage("Hook failed: " + hookResult.HookID + " (" + hookResult.Reason + ")")
		}
	}
	if result.Passed {
		s.HooksBlocked = false
		s.HooksBlockReason = ""
		e.StatusText = "Hooks passed"
		return
	}
	s.HooksBlocked = true
	reason := strings.TrimSpace(result.Reason)
	if reason == "" {
		reason = "unknown hook failure"
	}
	if result.FailedHookID != "" {
		reason = fmt.Sprintf("%s command %d %s", result.FailedHookID, result.FailedCommandIndex, reason)
	}
	s.HooksBlockReason = reason
	e.AddSystemMessage("Prompts blocked for this session until hooks pass. Run /hooks run after fixes.")
	e.StatusText = "Hooks blocked"
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
			content := "[agent-thought] " + text
			if strings.TrimSpace(ev.ItemID) != "" {
				e.upsertItemMessage(s.ID, ev.ItemID, content)
			} else {
				e.Store.AddAssistantMessage(s.ID, content)
			}
		}
	case providers.EventAgentMessage:
		text := strings.TrimSpace(ev.Text)
		if text == "" {
			return
		}
		e.clearPendingPlaceholder(ev.RequestID, s.ID)
		if strings.TrimSpace(ev.ItemID) != "" {
			e.upsertItemMessage(s.ID, ev.ItemID, text)
		} else {
			e.Store.AddAssistantMessage(s.ID, text)
		}
	case providers.EventCommandExecution:
		e.handleCommandExecutionEvent(s.ID, ev)
	case providers.EventTurnUsage:
		if ev.RequestID != "" {
			delete(e.pending, ev.RequestID)
		}
		e.ProviderState = "ready"
		e.StatusText = "Response complete"
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
	status := strings.ToLower(strings.TrimSpace(ev.CommandStatus))
	now := e.nowFn()
	ref := itemRef{SessionID: sessionID, ItemID: strings.TrimSpace(ev.ItemID)}

	state := commandRunState{
		startedAt:  now,
		command:    strings.TrimSpace(ev.Command),
		lastOutput: strings.TrimSpace(ev.CommandOutput),
		status:     status,
	}
	if ref.ItemID != "" {
		if existing, ok := e.commandRuns[ref]; ok {
			state = existing
		}
		if state.startedAt.IsZero() {
			state.startedAt = now
		}
		if cmd := strings.TrimSpace(ev.Command); cmd != "" {
			state.command = cmd
		}
		if out := strings.TrimSpace(ev.CommandOutput); out != "" {
			state.lastOutput = out
		}
		if status != "" {
			state.status = status
		}
		e.commandRuns[ref] = state
	}
	if strings.TrimSpace(state.command) == "" {
		state.command = "(unknown command)"
	}

	displayCommand := shortenCommand(state.command, 120)
	if status == "in_progress" {
		content := renderCommandRunning(displayCommand)
		if ref.ItemID != "" {
			e.upsertItemMessage(sessionID, ref.ItemID, content)
			return
		}
		e.Store.AddAssistantMessage(sessionID, content)
		return
	}

	duration := formatDuration(state.startedAt, now)
	failed := status == "failed"
	if !failed && ev.CommandExitCode != nil && *ev.CommandExitCode != 0 {
		failed = true
	}

	summary := renderCommandSummary(displayCommand, duration, classifyCommand(state.command) == "explored", failed, ev.CommandExitCode)
	var content string
	if failed {
		teaser := extractErrorTeaser(state.lastOutput)
		content = renderCommandFailed(summary, teaser)
	} else {
		content = renderCommandCompleted(summary)
	}
	if ref.ItemID != "" {
		delete(e.commandRuns, ref)
		e.upsertItemMessage(sessionID, ref.ItemID, content)
		return
	}
	e.Store.AddAssistantMessage(sessionID, content)
}

func (e *Engine) upsertItemMessage(sessionID, itemID, content string) {
	key := itemRefKey(sessionID, itemID)
	if ref, ok := e.itemRefs[key]; ok {
		if e.Store.ReplaceMessageAt(ref.SessionID, ref.Index, content) {
			return
		}
		delete(e.itemRefs, key)
	}
	idx := e.Store.AppendAssistantMessage(sessionID, content)
	if idx >= 0 {
		e.itemRefs[key] = pendingRef{SessionID: sessionID, Index: idx}
	}
}

func itemRefKey(sessionID, itemID string) string {
	return sessionID + "|" + itemID
}

func renderCommandRunning(command string) string {
	return fmt.Sprintf("Running `%s` ...", escapeInlineCode(command))
}

func renderCommandCompleted(summary string) string {
	return summary
}

func renderCommandFailed(summary, teaser string) string {
	if strings.TrimSpace(teaser) == "" {
		return summary
	}
	return summary + "\nError: " + teaser
}

func renderCommandSummary(command, duration string, explored, failed bool, exitCode *int) string {
	var line string
	if explored {
		line = fmt.Sprintf("Explored for %s", duration)
	} else {
		line = fmt.Sprintf("Ran `%s` for %s", escapeInlineCode(command), duration)
	}
	if failed {
		line += fmt.Sprintf(" (failed, exit=%s)", formatExitCode(exitCode))
	}
	return line
}

func shortenCommand(command string, maxChars int) string {
	normalized := normalizeShellWrappedCommand(command)
	normalized = strings.Join(strings.Fields(strings.TrimSpace(normalized)), " ")
	if normalized == "" {
		return "(unknown command)"
	}
	return truncateRunes(normalized, maxChars)
}

func normalizeShellWrappedCommand(command string) string {
	trimmed := strings.TrimSpace(command)
	prefixes := []string{"/bin/bash -lc ", "bash -lc ", "sh -lc ", "/bin/sh -lc "}
	for _, p := range prefixes {
		if strings.HasPrefix(trimmed, p) {
			inner := strings.TrimSpace(strings.TrimPrefix(trimmed, p))
			if len(inner) >= 2 {
				if (inner[0] == '\'' && inner[len(inner)-1] == '\'') || (inner[0] == '"' && inner[len(inner)-1] == '"') {
					return inner[1 : len(inner)-1]
				}
			}
			return inner
		}
	}
	return trimmed
}

func escapeInlineCode(s string) string {
	return strings.ReplaceAll(s, "`", "'")
}

func formatDuration(startedAt, finishedAt time.Time) string {
	if startedAt.IsZero() || finishedAt.Before(startedAt) {
		return "0s"
	}
	d := finishedAt.Sub(startedAt)
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	if d < time.Minute {
		sec := float64(d.Milliseconds()) / 1000.0
		return fmt.Sprintf("%.1fs", sec)
	}
	return d.Round(time.Second).String()
}

func classifyCommand(command string) string {
	normalized := strings.ToLower(normalizeShellWrappedCommand(command))
	segments := splitCommandSegments(normalized)
	if len(segments) == 0 {
		return "ran"
	}
	isExploreSegment := func(segment string) bool {
		fields := strings.Fields(strings.TrimSpace(segment))
		if len(fields) == 0 {
			return true
		}
		switch fields[0] {
		case "ls", "find", "rg", "fd", "tree", "pwd", "cat", "head", "tail", "du", "wc", "stat", "which":
			return true
		case "sed":
			return true
		default:
			return false
		}
	}
	for _, segment := range segments {
		if !isExploreSegment(segment) {
			return "ran"
		}
	}
	return "explored"
}

func splitCommandSegments(command string) []string {
	replaced := strings.NewReplacer("&&", ";", "||", ";", "|", ";", "\n", ";").Replace(command)
	parts := strings.Split(replaced, ";")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func extractErrorTeaser(output string) string {
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		return truncateRunes(strings.Join(strings.Fields(line), " "), 180)
	}
	return ""
}

func (e *Engine) clearPendingPlaceholder(requestID, sessionID string) {
	if strings.TrimSpace(requestID) == "" {
		return
	}
	ref, ok := e.pending[requestID]
	if !ok {
		return
	}
	delete(e.pending, requestID)
	if ref.SessionID != sessionID {
		return
	}
	if !e.Store.DeleteMessageAt(ref.SessionID, ref.Index) {
		return
	}
	for reqID, pendingRef := range e.pending {
		if pendingRef.SessionID == ref.SessionID && pendingRef.Index > ref.Index {
			pendingRef.Index--
			e.pending[reqID] = pendingRef
		}
	}
	for key, itemRef := range e.itemRefs {
		if itemRef.SessionID == ref.SessionID && itemRef.Index > ref.Index {
			itemRef.Index--
			e.itemRefs[key] = itemRef
		}
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
