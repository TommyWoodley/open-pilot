package chat

import (
	"context"
	"fmt"
	"log"
	"regexp"
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
	SendPrompt(ctx context.Context, providerID, sessionID, repoPath, requestID, prompt string, opts providers.SendOptions) error
	Events() <-chan providers.Event
	StopAll(ctx context.Context) error
}

type pendingRef struct {
	SessionID       string
	Index           int
	ProviderID      string
	RepoPath        string
	Prompt          string
	ResumeAttempted bool
	ReplayRetried   bool
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

type exploreSequenceState struct {
	requestID     string
	groupItemID   string
	started       map[itemRef]struct{}
	completed     map[itemRef]struct{}
	completedRuns int
	totalDuration time.Duration
}

const hooksRunningReason = "running"
const replayResumeWarning = "Resumed via replay; prior chat was resent to restore context."

type HookEventType string

const (
	HookEventProgress HookEventType = "progress"
	HookEventDone     HookEventType = "done"
)

type HookEvent struct {
	Type      HookEventType
	SessionID string
	Update    corehooks.ProgressUpdate
	Result    corehooks.RunResult
}

type AutoReviewEvent struct {
	SessionID string
	Cycle     int
	BaseRef   string
	Progress  string
	Result    autoReviewResult
	Err       error
}

type hookProgressState struct {
	progressID string
	hookIDs    []string
	statuses   map[string]string
}

type Engine struct {
	Store               *session.Store
	Manager             ProviderManager
	Hooks               corehooks.Service
	Config              config.Config
	ProviderState       string
	StatusText          string
	pending             map[string]pendingRef
	itemRefs            map[string]pendingRef
	commandRuns         map[itemRef]commandRunState
	exploreSeq          map[string]*exploreSequenceState
	nextExploreID       int
	hookProgress        map[string]hookProgressState
	hookEvents          chan HookEvent
	autoReviewEvents    chan AutoReviewEvent
	asyncHookRuns       bool
	unknownSeen         map[string]struct{}
	nowFn               func() time.Time
	logf                func(format string, args ...any)
	autoReviewBySession map[string]*autoReviewState
	autoReviewRunner    autoReviewRunner
	autoReviewMaxCycles int
	replayWarned        map[string]bool
}

func NewEngine(store *session.Store, manager ProviderManager, cfg config.Config) *Engine {
	return &Engine{
		Store:               store,
		Manager:             manager,
		Hooks:               corehooks.NewService(cfg.BuiltinHooks, cfg.BuiltinHooksLoadError, cfg.BuiltinSkillsDir),
		Config:              cfg,
		ProviderState:       "disconnected",
		StatusText:          "No agent connected",
		pending:             make(map[string]pendingRef),
		itemRefs:            make(map[string]pendingRef),
		commandRuns:         make(map[itemRef]commandRunState),
		exploreSeq:          make(map[string]*exploreSequenceState),
		nextExploreID:       1,
		hookProgress:        make(map[string]hookProgressState),
		unknownSeen:         make(map[string]struct{}),
		nowFn:               time.Now,
		logf:                log.Printf,
		autoReviewBySession: make(map[string]*autoReviewState),
		autoReviewRunner:    newCLIAutoReviewRunner(),
		autoReviewMaxCycles: 5,
		replayWarned:        make(map[string]bool),
	}
}

func (e *Engine) EnableAsyncHooks() {
	if e.hookEvents == nil {
		e.hookEvents = make(chan HookEvent, 256)
	}
	e.asyncHookRuns = true
}

func (e *Engine) EnableAsyncAutoReview() {
	if e.autoReviewEvents == nil {
		e.autoReviewEvents = make(chan AutoReviewEvent, 256)
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
		if reason == hooksRunningReason {
			e.AddSystemMessage("Hooks are still running for this session. Please wait for completion.")
			e.StatusText = "Running hooks..."
			return
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
	resumeThreadID := strings.TrimSpace(s.CodexThreadID)
	e.pending[requestID] = pendingRef{
		SessionID:       s.ID,
		Index:           idx,
		ProviderID:      s.ProviderID,
		RepoPath:        repo.Path,
		Prompt:          input,
		ResumeAttempted: s.ProviderID == "codex" && resumeThreadID != "",
	}
	e.ProviderState = "busy"
	e.StatusText = "Sending prompt..."

	err := e.Manager.SendPrompt(context.Background(), s.ProviderID, s.ID, repo.Path, requestID, input, providers.SendOptions{
		ProviderThreadID: resumeThreadID,
	})
	if err != nil {
		if e.retryPendingWithReplay(s, requestID) {
			return
		}
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
		if s.ProviderID == "codex" && !s.HooksBlocked {
			e.runHooks(s, config.HookTriggerProviderCodexSelected, "")
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
			if repo := e.Store.ActiveRepo(); repo != nil {
				e.runHooks(active, config.HookTriggerRepoSelected, repo.Path)
			}
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
			e.runHooks(s, config.HookTriggerRepoSelected, repoPath)
		}
	case command.KindSessionRepos:
		e.AddSystemMessage(e.Store.ListReposText())
	case command.KindSessionRepoUse:
		if err := e.Store.SetActiveRepo(cmd.RepoID); err != nil {
			e.AddSystemMessage(err.Error())
			return
		}
		e.AddSystemMessage("Active repo set")
		if s := e.Store.ActiveSession(); s != nil {
			if repo := e.Store.ActiveRepo(); repo != nil {
				e.runHooks(s, config.HookTriggerRepoSelected, repo.Path)
			}
		}
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
		if cmd.ProviderID == "codex" {
			e.runHooks(s, config.HookTriggerProviderCodexSelected, "")
		}
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
	case command.KindReview:
		e.startManualAutoReview()
	default:
		e.AddSystemMessage("Unknown command")
	}
}

func (e *Engine) startManualAutoReview() {
	s := e.Store.ActiveSession()
	if s == nil {
		e.AddSystemMessage("Create/select a session first: /session new <name>")
		return
	}
	if s.ProviderID != "codex" {
		e.AddSystemMessage("Automatic review requires provider codex: /provider use codex")
		return
	}
	if strings.TrimSpace(e.repoPathForSession(s)) == "" {
		e.AddSystemMessage("Add/select a repo first: /session add-repo <abs-path>")
		return
	}
	e.handleAutoReviewCompletionTag(s, true)
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
	if e.asyncHookRuns {
		e.runHooksAsync(s, trigger, repoPath)
		return
	}
	e.runHooksSync(s, trigger, repoPath)
}

func (e *Engine) runHooksSync(s *domain.Session, trigger config.HookTrigger, repoPath string) {
	hookDefs := e.Config.BuiltinHooks.HooksFor(trigger)
	hookIDs := make([]string, 0, len(hookDefs))
	for _, hook := range hookDefs {
		hookIDs = append(hookIDs, hook.ID)
	}
	if len(hookIDs) == 0 && strings.TrimSpace(e.Config.BuiltinHooksLoadError) == "" {
		s.HooksBlocked = false
		s.HooksBlockReason = ""
		s.LastHookRunAt = e.nowFn()
		e.StatusText = "Hooks passed"
		return
	}
	progressID := e.Store.NextID("hooks-progress")
	state := hookProgressState{
		progressID: progressID,
		hookIDs:    hookIDs,
		statuses:   make(map[string]string, len(hookIDs)),
	}
	e.StatusText = "Running hooks..."
	e.renderHookProgress(s.ID, state, 0, "")
	onUpdate := func(update corehooks.ProgressUpdate) {
		if strings.TrimSpace(update.HookID) == "" {
			return
		}
		switch update.Status {
		case "running":
			state.statuses[update.HookID] = "running"
			e.renderHookProgress(s.ID, state, update.Completed, update.HookID)
		case "passed":
			state.statuses[update.HookID] = "passed"
			e.renderHookProgress(s.ID, state, update.Completed, "")
		default:
			state.statuses[update.HookID] = "failed (" + update.Status + ")"
			e.renderHookProgress(s.ID, state, update.Completed, "")
		}
	}
	result := e.Hooks.Run(context.Background(), trigger, s.ID, s.Name, repoPath, onUpdate)
	s.LastHookRunAt = e.nowFn()
	if !result.Passed && result.FailedHookID != "" && result.FailedCommandIndex > 0 {
		state.statuses[result.FailedHookID] = fmt.Sprintf("failed (command %d, %s)", result.FailedCommandIndex, result.Reason)
		e.renderHookProgress(s.ID, state, len(result.PerHookResults), "")
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

func (e *Engine) runHooksAsync(s *domain.Session, trigger config.HookTrigger, repoPath string) {
	hookDefs := e.Config.BuiltinHooks.HooksFor(trigger)
	hookIDs := make([]string, 0, len(hookDefs))
	for _, hook := range hookDefs {
		hookIDs = append(hookIDs, hook.ID)
	}
	if len(hookIDs) == 0 {
		if strings.TrimSpace(e.Config.BuiltinHooksLoadError) == "" {
			s.HooksBlocked = false
			s.HooksBlockReason = ""
			s.LastHookRunAt = e.nowFn()
			e.StatusText = "Hooks passed"
		} else {
			s.HooksBlocked = true
			s.HooksBlockReason = e.Config.BuiltinHooksLoadError
			e.StatusText = "Hooks blocked"
			e.AddSystemMessage("Prompts blocked for this session until hooks pass. Run /hooks run after fixes.")
		}
		return
	}

	state := hookProgressState{
		progressID: e.Store.NextID("hooks-progress"),
		hookIDs:    hookIDs,
		statuses:   make(map[string]string, len(hookIDs)),
	}
	e.hookProgress[s.ID] = state
	s.HooksBlocked = true
	s.HooksBlockReason = hooksRunningReason
	e.StatusText = "Running hooks..."
	e.renderHookProgress(s.ID, state, 0, "")

	go func(sessionID, sessionName string) {
		onUpdate := func(update corehooks.ProgressUpdate) {
			e.emitHookEvent(HookEvent{
				Type:      HookEventProgress,
				SessionID: sessionID,
				Update:    update,
			})
		}
		result := e.Hooks.Run(context.Background(), trigger, sessionID, sessionName, repoPath, onUpdate)
		e.emitHookEvent(HookEvent{
			Type:      HookEventDone,
			SessionID: sessionID,
			Result:    result,
		})
	}(s.ID, s.Name)
}

func (e *Engine) renderHookProgress(sessionID string, state hookProgressState, completed int, runningID string) {
	lines := []string{fmt.Sprintf("[[pilot-divider:Hooks %d/%d]]", completed, len(state.hookIDs))}
	for _, id := range state.hookIDs {
		status := state.statuses[id]
		if status == "" {
			status = "pending"
		}
		if id == runningID && status == "pending" {
			status = "running"
		}
		lines = append(lines, fmt.Sprintf("%s: %s", id, status))
	}
	lines = append(lines, "[[pilot-divider:]]")
	e.upsertItemMessageWithRole(sessionID, "", state.progressID, strings.Join(lines, "\n"), domain.RoleSystem)
}

func (e *Engine) emitHookEvent(ev HookEvent) {
	if e.hookEvents == nil {
		return
	}
	select {
	case e.hookEvents <- ev:
	default:
	}
}

func (e *Engine) HandleHookEvent(ev HookEvent) {
	s := e.Store.Sessions[ev.SessionID]
	if s == nil {
		return
	}
	state, ok := e.hookProgress[ev.SessionID]
	if !ok {
		return
	}
	switch ev.Type {
	case HookEventProgress:
		update := ev.Update
		if strings.TrimSpace(update.HookID) == "" {
			return
		}
		switch update.Status {
		case "running":
			state.statuses[update.HookID] = "running"
			e.renderHookProgress(ev.SessionID, state, update.Completed, update.HookID)
		case "passed":
			state.statuses[update.HookID] = "passed"
			e.renderHookProgress(ev.SessionID, state, update.Completed, "")
		default:
			state.statuses[update.HookID] = "failed (" + update.Status + ")"
			e.renderHookProgress(ev.SessionID, state, update.Completed, "")
		}
		e.hookProgress[ev.SessionID] = state
		e.StatusText = "Running hooks..."
	case HookEventDone:
		result := ev.Result
		if !result.Passed && result.FailedHookID != "" && result.FailedCommandIndex > 0 {
			state.statuses[result.FailedHookID] = fmt.Sprintf("failed (command %d, %s)", result.FailedCommandIndex, result.Reason)
		}
		e.renderHookProgress(ev.SessionID, state, len(result.PerHookResults), "")
		s.LastHookRunAt = e.nowFn()
		delete(e.hookProgress, ev.SessionID)
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
}

func (e *Engine) HandleProviderEvent(ev providers.Event) {
	s := e.Store.Sessions[ev.SessionID]
	if s == nil {
		e.StatusText = ev.Message
		return
	}
	if strings.TrimSpace(ev.ProviderThreadID) != "" && (ev.Provider == "codex" || s.ProviderID == "codex") {
		s.CodexThreadID = strings.TrimSpace(ev.ProviderThreadID)
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
		e.runDevelopmentWorkCompleteHooksForContent(s, ev.Text)
		e.ProviderState = "ready"
		e.StatusText = "Response complete"
	case providers.EventError:
		if e.retryWithReplayIfNeeded(s, ev) {
			return
		}
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
				e.upsertItemMessage(s.ID, ev.RequestID, ev.ItemID, content)
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
			e.upsertItemMessage(s.ID, ev.RequestID, ev.ItemID, text)
		} else {
			e.Store.AddAssistantMessage(s.ID, text)
		}
		e.runDevelopmentWorkCompleteHooksForContent(s, ev.Text)
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

func (e *Engine) runDevelopmentWorkCompleteHooksForContent(s *domain.Session, content string) {
	if s == nil {
		return
	}
	count := countDevelopmentWorkCompleteTags(content)
	if count <= 0 {
		return
	}
	repoPath := e.repoPathForSession(s)
	for i := 0; i < count; i++ {
		e.runHooks(s, config.HookTriggerDevelopmentWorkComplete, repoPath)
	}
	e.handleAutoReviewCompletionTag(s, false)
}

func (e *Engine) handleAutoReviewCompletionTag(s *domain.Session, allowWhenDisabled bool) {
	if s == nil {
		return
	}
	if s.ProviderID != "codex" {
		return
	}
	if !allowWhenDisabled && !s.AutoReviewLoopEnabled {
		return
	}
	repoPath := e.repoPathForSession(s)
	if strings.TrimSpace(repoPath) == "" {
		return
	}
	if e.autoReviewRunner == nil {
		return
	}
	state := e.ensureAutoReviewState(s.ID)
	if state.running {
		return
	}
	if !state.active {
		state.runID++
		state.active = true
		state.cycle = 0
		state.running = false
		state.waitingForCompletion = false
		e.runAutoReviewCycle(s, state)
		return
	}
	if state.waitingForCompletion {
		state.waitingForCompletion = false
		e.runAutoReviewCycle(s, state)
	}
}

func (e *Engine) ensureAutoReviewState(sessionID string) *autoReviewState {
	if st, ok := e.autoReviewBySession[sessionID]; ok && st != nil {
		return st
	}
	st := &autoReviewState{}
	e.autoReviewBySession[sessionID] = st
	return st
}

func (e *Engine) runAutoReviewCycle(s *domain.Session, st *autoReviewState) {
	if s == nil || st == nil {
		return
	}
	if st.cycle >= e.autoReviewMaxCycles {
		e.emitAutoReviewState(s.ID, st.cycle, e.autoReviewMaxCycles, "Max cycles reached (5)", "Manual follow-up required.")
		e.finishAutoReview(st)
		return
	}
	st.cycle++
	st.running = true
	st.progressLines = nil
	e.emitAutoReviewState(s.ID, st.cycle, e.autoReviewMaxCycles, "Starting", "")
	e.emitAutoReviewState(s.ID, st.cycle, e.autoReviewMaxCycles, "Computing base", "")
	repoPath := e.repoPathForSession(s)
	if strings.TrimSpace(repoPath) == "" {
		st.running = false
		e.failAutoReview(s.ID, st, "active repo is required")
		return
	}
	e.emitAutoReviewStateWithItem(s.ID, st.cycle, e.autoReviewMaxCycles, "Running codex review", "", autoReviewProgressItemID(st.runID, st.cycle))
	cycle := st.cycle
	progress := func(line string) {
		line = strings.TrimSpace(line)
		if line == "" {
			return
		}
		if e.autoReviewEvents == nil {
			e.appendAutoReviewProgressLine(s.ID, st, cycle, line)
			return
		}
		e.emitAutoReviewEvent(AutoReviewEvent{
			SessionID: s.ID,
			Cycle:     cycle,
			Progress:  line,
		})
	}
	if e.autoReviewEvents == nil {
		baseRef, reviewResult, err := e.executeAutoReview(repoPath, progress)
		e.applyAutoReviewCycleResult(s, st, cycle, baseRef, reviewResult, err)
		return
	}
	go func(sessionID string, cycleNum int, path string) {
		baseRef, reviewResult, err := e.executeAutoReview(path, progress)
		e.emitAutoReviewCycleResultEvent(AutoReviewEvent{
			SessionID: sessionID,
			Cycle:     cycleNum,
			BaseRef:   baseRef,
			Result:    reviewResult,
			Err:       err,
		})
	}(s.ID, cycle, repoPath)
}

func (e *Engine) finishAutoReview(st *autoReviewState) {
	if st == nil {
		return
	}
	st.active = false
	st.running = false
	st.waitingForCompletion = false
}

func (e *Engine) failAutoReview(sessionID string, st *autoReviewState, reason string) {
	if st != nil {
		st.running = false
	}
	e.emitAutoReviewState(sessionID, st.cycle, e.autoReviewMaxCycles, "Error", summarizeAutoReviewDetail(reason))
	e.finishAutoReview(st)
}

func (e *Engine) emitAutoReviewState(sessionID string, cycle, maxCycles int, state, detail string) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(state) == "" {
		return
	}
	e.Store.AddSessionSystemMessage(sessionID, renderAutoReviewStateMessage(cycle, maxCycles, state, detail))
	e.StatusText = "Automatic review: " + state
}

func (e *Engine) emitAutoReviewStateWithItem(sessionID string, cycle, maxCycles int, state, detail, itemID string) {
	if strings.TrimSpace(sessionID) == "" || strings.TrimSpace(state) == "" {
		return
	}
	if strings.TrimSpace(itemID) == "" {
		e.emitAutoReviewState(sessionID, cycle, maxCycles, state, detail)
		return
	}
	e.upsertItemMessageWithRole(sessionID, "", itemID, renderAutoReviewStateMessage(cycle, maxCycles, state, detail), domain.RoleSystem)
	e.StatusText = "Automatic review: " + state
}

func renderAutoReviewStateMessage(cycle, maxCycles int, state, detail string) string {
	lines := []string{
		"[[pilot-divider:Automatic Review]]",
		fmt.Sprintf("Cycle: %d/%d", cycle, maxCycles),
		"State: " + state,
	}
	if strings.TrimSpace(detail) != "" {
		lines = append(lines, detail)
	}
	lines = append(lines, "[[pilot-divider:]]")
	return strings.Join(lines, "\n")
}

func autoReviewProgressItemID(runID, cycle int) string {
	return fmt.Sprintf("auto-review-progress-%d-%d", runID, cycle)
}

func (e *Engine) appendAutoReviewProgressLine(sessionID string, st *autoReviewState, cycle int, line string) {
	if st == nil {
		return
	}
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return
	}
	st.progressLines = append(st.progressLines, trimmed)
	e.emitAutoReviewStateWithItem(
		sessionID,
		cycle,
		e.autoReviewMaxCycles,
		"Running codex review",
		strings.Join(st.progressLines, "\n"),
		autoReviewProgressItemID(st.runID, cycle),
	)
}

func (e *Engine) repoPathForSession(s *domain.Session) string {
	if s == nil || strings.TrimSpace(s.ActiveRepoID) == "" {
		return ""
	}
	for i := range s.Repos {
		if s.Repos[i].ID == s.ActiveRepoID {
			return s.Repos[i].Path
		}
	}
	return ""
}

func (e *Engine) executeAutoReview(repoPath string, onProgress func(string)) (string, autoReviewResult, error) {
	baseSHA, baseRef, err := e.autoReviewRunner.ResolveBase(repoPath)
	if err != nil {
		return "", autoReviewResult{}, err
	}
	reviewResult, err := e.autoReviewRunner.Review(repoPath, baseSHA, onProgress)
	if err != nil {
		return "", autoReviewResult{}, err
	}
	return baseRef, reviewResult, nil
}

func (e *Engine) applyAutoReviewCycleResult(s *domain.Session, st *autoReviewState, cycle int, baseRef string, reviewResult autoReviewResult, err error) {
	if s == nil || st == nil {
		return
	}
	st.running = false
	if cycle != st.cycle {
		return
	}
	if err != nil {
		e.failAutoReview(s.ID, st, err.Error())
		return
	}
	if reviewResult.Approved {
		e.emitAutoReviewState(s.ID, st.cycle, e.autoReviewMaxCycles, "Review approved", summarizeAutoReviewDetail(reviewResult.Summary))
		e.finishAutoReview(st)
		return
	}
	e.emitAutoReviewState(s.ID, st.cycle, e.autoReviewMaxCycles, "Review requires changes", summarizeAutoReviewDetail(reviewResult.Summary))
	if st.cycle >= e.autoReviewMaxCycles {
		e.emitAutoReviewState(s.ID, st.cycle, e.autoReviewMaxCycles, "Max cycles reached (5)", "Manual follow-up required.")
		e.finishAutoReview(st)
		return
	}
	if e.Manager == nil {
		e.failAutoReview(s.ID, st, "provider manager is unavailable")
		return
	}
	repoPath := e.repoPathForSession(s)
	if strings.TrimSpace(repoPath) == "" {
		e.failAutoReview(s.ID, st, "active repo is required")
		return
	}
	prompt := buildAutoReviewPrompt(baseRef, reviewResult.Summary)
	e.emitAutoReviewState(s.ID, st.cycle, e.autoReviewMaxCycles, "Resuming agent to address feedback", "")
	reqID := e.Store.NextID("req")
	if err := e.Manager.SendPrompt(context.Background(), s.ProviderID, s.ID, repoPath, reqID, prompt, providers.SendOptions{}); err != nil {
		e.failAutoReview(s.ID, st, "failed to resume agent: "+err.Error())
		return
	}
	st.waitingForCompletion = true
	e.emitAutoReviewState(s.ID, st.cycle, e.autoReviewMaxCycles, "Waiting for DEVELOPMENT_WORK_COMPLETE", "")
}

func (e *Engine) retryWithReplayIfNeeded(s *domain.Session, ev providers.Event) bool {
	if s == nil || strings.TrimSpace(ev.RequestID) == "" {
		return false
	}
	return e.retryPendingWithReplay(s, ev.RequestID)
}

func (e *Engine) retryPendingWithReplay(s *domain.Session, requestID string) bool {
	if s == nil || strings.TrimSpace(requestID) == "" {
		return false
	}
	ref, ok := e.pending[requestID]
	if !ok || !ref.ResumeAttempted || ref.ReplayRetried || ref.ProviderID != "codex" {
		return false
	}
	replayPrompt := e.buildReplayPrompt(s)
	if strings.TrimSpace(replayPrompt) == "" {
		return false
	}
	ref.ReplayRetried = true
	ref.ResumeAttempted = false
	e.pending[requestID] = ref
	s.CodexThreadID = ""
	if !e.replayWarned[s.ID] {
		e.Store.AddSessionSystemMessage(s.ID, replayResumeWarning)
		e.replayWarned[s.ID] = true
	}
	if err := e.Manager.SendPrompt(context.Background(), ref.ProviderID, ref.SessionID, ref.RepoPath, requestID, replayPrompt, providers.SendOptions{DisableResume: true}); err != nil {
		return false
	}
	e.ProviderState = "busy"
	e.StatusText = "Resuming via replay..."
	return true
}

func (e *Engine) buildReplayPrompt(s *domain.Session) string {
	if s == nil {
		return ""
	}
	lines := make([]string, 0, len(s.Messages)+3)
	lines = append(lines, "Prior conversation transcript:")
	for _, m := range s.Messages {
		if m.Streaming {
			continue
		}
		role := strings.TrimSpace(m.Role)
		content := strings.TrimSpace(m.Content)
		if role == "" || content == "" {
			continue
		}
		lines = append(lines, role+": "+content)
	}
	lines = append(lines, "Continue from this context.")
	return strings.Join(lines, "\n")
}

func (e *Engine) emitAutoReviewEvent(ev AutoReviewEvent) {
	if e.autoReviewEvents == nil {
		return
	}
	select {
	case e.autoReviewEvents <- ev:
	default:
		if e.logf != nil {
			if ev.Err != nil {
				errMsg := strings.TrimSpace(ev.Err.Error())
				if errMsg == "" {
					errMsg = "(empty)"
				}
				const maxErrLen = 120
				if len(errMsg) > maxErrLen {
					errMsg = errMsg[:maxErrLen-3] + "..."
				}
				e.logf("dropping auto-review event: session=%s cycle=%d base=%s has_err=%t err=%s buffer=%d/%d", strings.TrimSpace(ev.SessionID), ev.Cycle, strings.TrimSpace(ev.BaseRef), true, errMsg, len(e.autoReviewEvents), cap(e.autoReviewEvents))
				return
			}
			e.logf("dropping auto-review event: session=%s cycle=%d base=%s has_err=%t buffer=%d/%d", strings.TrimSpace(ev.SessionID), ev.Cycle, strings.TrimSpace(ev.BaseRef), false, len(e.autoReviewEvents), cap(e.autoReviewEvents))
		}
	}
}

func (e *Engine) emitAutoReviewCycleResultEvent(ev AutoReviewEvent) {
	if e.autoReviewEvents == nil {
		return
	}
	ev.Progress = ""
	select {
	case e.autoReviewEvents <- ev:
		return
	default:
	}

	buffered := make([]AutoReviewEvent, 0, cap(e.autoReviewEvents))
	for {
		select {
		case existing := <-e.autoReviewEvents:
			buffered = append(buffered, existing)
		default:
			goto drained
		}
	}

drained:
	droppedProgress := false
	kept := buffered[:0]
	for _, existing := range buffered {
		if !droppedProgress && strings.TrimSpace(existing.Progress) != "" {
			droppedProgress = true
			continue
		}
		kept = append(kept, existing)
	}
	for _, existing := range kept {
		select {
		case e.autoReviewEvents <- existing:
		default:
			if e.logf != nil {
				e.logf("dropping auto-review event while restoring buffer: session=%s cycle=%d base=%s has_err=%t", strings.TrimSpace(existing.SessionID), existing.Cycle, strings.TrimSpace(existing.BaseRef), existing.Err != nil)
			}
		}
	}
	if !droppedProgress {
		if e.logf != nil {
			e.logf("dropping auto-review cycle result event: session=%s cycle=%d base=%s has_err=%t reason=no-progress-to-evict buffer=%d/%d", strings.TrimSpace(ev.SessionID), ev.Cycle, strings.TrimSpace(ev.BaseRef), ev.Err != nil, len(e.autoReviewEvents), cap(e.autoReviewEvents))
		}
		return
	}
	select {
	case e.autoReviewEvents <- ev:
	default:
		if e.logf != nil {
			e.logf("dropping auto-review cycle result event: session=%s cycle=%d base=%s has_err=%t reason=buffer-full-after-evict buffer=%d/%d", strings.TrimSpace(ev.SessionID), ev.Cycle, strings.TrimSpace(ev.BaseRef), ev.Err != nil, len(e.autoReviewEvents), cap(e.autoReviewEvents))
		}
	}
}

func (e *Engine) AutoReviewEvents() <-chan AutoReviewEvent {
	return e.autoReviewEvents
}

func (e *Engine) HandleAutoReviewEvent(ev AutoReviewEvent) {
	s := e.Store.Sessions[ev.SessionID]
	if s == nil {
		return
	}
	st := e.ensureAutoReviewState(ev.SessionID)
	if !st.active {
		return
	}
	if strings.TrimSpace(ev.Progress) != "" {
		if ev.Cycle == st.cycle {
			e.appendAutoReviewProgressLine(s.ID, st, st.cycle, ev.Progress)
		}
		return
	}
	e.applyAutoReviewCycleResult(s, st, ev.Cycle, ev.BaseRef, ev.Result, ev.Err)
}

var developmentWorkCompleteOccurrenceRegex = regexp.MustCompile(`\[(?:<)?DEVELOPMENT_WORK_COMPLETE(?:>)?\]|<DEVELOPMENT_WORK_COMPLETE>`)

func countDevelopmentWorkCompleteTags(content string) int {
	if strings.TrimSpace(content) == "" {
		return 0
	}
	return len(developmentWorkCompleteOccurrenceRegex.FindAllStringIndex(content, -1))
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

	explored := classifyCommand(state.command) == "explored"
	displayCommand := shortenCommand(state.command, 120)
	if status == "in_progress" {
		if explored && ref.ItemID != "" {
			seq := e.ensureExploreSequence(sessionID, ev.RequestID)
			seq.started[ref] = struct{}{}
			e.upsertItemMessage(sessionID, ev.RequestID, seq.groupItemID, renderExploreSequenceRunning(len(seq.started)))
			return
		}
		e.resetExploreSequence(sessionID)
		content := renderCommandRunning(displayCommand)
		if ref.ItemID != "" {
			e.upsertItemMessage(sessionID, ev.RequestID, ref.ItemID, content)
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

	if explored && !failed && ref.ItemID != "" {
		seq := e.ensureExploreSequence(sessionID, ev.RequestID)
		if _, seen := seq.completed[ref]; !seen {
			seq.completed[ref] = struct{}{}
			seq.completedRuns++
			if !state.startedAt.IsZero() && !now.Before(state.startedAt) {
				seq.totalDuration += now.Sub(state.startedAt)
			}
		}
		delete(e.commandRuns, ref)
		e.upsertItemMessage(sessionID, ev.RequestID, seq.groupItemID, renderExploreSequenceSummary(seq.completedRuns, seq.totalDuration))
		return
	}

	e.resetExploreSequence(sessionID)
	summary := renderCommandSummary(displayCommand, duration, explored, failed, ev.CommandExitCode)
	var content string
	if failed {
		teaser := extractErrorTeaser(state.lastOutput)
		content = renderCommandFailed(summary, teaser)
	} else {
		content = renderCommandCompleted(summary)
	}
	if ref.ItemID != "" {
		delete(e.commandRuns, ref)
		e.upsertItemMessage(sessionID, ev.RequestID, ref.ItemID, content)
		return
	}
	e.Store.AddAssistantMessage(sessionID, content)
}

func (e *Engine) upsertItemMessage(sessionID, requestID, itemID, content string) {
	e.upsertItemMessageWithRole(sessionID, requestID, itemID, content, domain.RoleAssistant)
}

func (e *Engine) upsertItemMessageWithRole(sessionID, requestID, itemID, content, role string) {
	key := itemRefKey(sessionID, requestID, itemID)
	if ref, ok := e.itemRefs[key]; ok {
		if e.Store.ReplaceMessageAt(ref.SessionID, ref.Index, content) {
			return
		}
		delete(e.itemRefs, key)
	}
	var idx int
	if role == domain.RoleSystem {
		idx = e.Store.AppendSystemMessage(sessionID, content)
	} else {
		idx = e.Store.AppendAssistantMessage(sessionID, content)
	}
	if idx >= 0 {
		e.itemRefs[key] = pendingRef{SessionID: sessionID, Index: idx}
	}
}

func itemRefKey(sessionID, requestID, itemID string) string {
	if strings.TrimSpace(requestID) == "" {
		return sessionID + "|" + itemID
	}
	return sessionID + "|" + requestID + "|" + itemID
}

func renderCommandRunning(command string) string {
	return fmt.Sprintf("Running `%s` ...", escapeInlineCode(command))
}

func renderExploreSequenceRunning(commands int) string {
	if commands <= 1 {
		return "Exploring ..."
	}
	return fmt.Sprintf("Exploring (%d commands) ...", commands)
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

func renderExploreSequenceSummary(commands int, duration time.Duration) string {
	if commands <= 1 {
		return fmt.Sprintf("Explored for %s", formatDurationFromDelta(duration))
	}
	return fmt.Sprintf("Explored %d commands for %s", commands, formatDurationFromDelta(duration))
}

func formatDurationFromDelta(d time.Duration) string {
	if d < 0 {
		return "0s"
	}
	return formatDuration(time.Unix(0, 0), time.Unix(0, 0).Add(d))
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

func (e *Engine) ensureExploreSequence(sessionID, requestID string) *exploreSequenceState {
	requestID = strings.TrimSpace(requestID)
	if seq := e.exploreSeq[sessionID]; seq != nil {
		if seq.requestID == requestID {
			return seq
		}
	}

	seq := &exploreSequenceState{
		requestID:   requestID,
		groupItemID: fmt.Sprintf("__explore_sequence_%d", e.nextExploreID),
		started:     make(map[itemRef]struct{}),
		completed:   make(map[itemRef]struct{}),
	}
	e.nextExploreID++
	e.exploreSeq[sessionID] = seq
	return seq
}

func (e *Engine) resetExploreSequence(sessionID string) {
	delete(e.exploreSeq, sessionID)
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

func (e *Engine) HookEvents() <-chan HookEvent {
	return e.hookEvents
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
