package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type codexCLIAdapter struct {
	binary string

	mu      sync.Mutex
	handles map[SessionHandle]*codexHandle
	logMu   sync.Mutex
	logPath string
}

type codexHandle struct {
	sessionID string
	repoPath  string
	events    chan Event
	mu        sync.Mutex
	closed    bool
	codexID   string
}

type codexJSONEvent struct {
	Type     string `json:"type"`
	ThreadID string `json:"thread_id"`
	Message  string `json:"message"`
	Delta    string `json:"delta"`
	Text     string `json:"text"`
	Error    struct {
		Message string `json:"message"`
	} `json:"error"`
}

type codexRunResult struct {
	ThreadID       string
	LastMessage    string
	FailureMessage string
}

func newCodexCLIAdapter(binary string) Adapter {
	logPath := os.Getenv("OPEN_PILOT_CODEX_DEBUG_LOG")
	if strings.TrimSpace(logPath) == "" {
		logPath = filepath.Join(os.TempDir(), "open-pilot-codex-debug.log")
	}
	return &codexCLIAdapter{
		binary:  binary,
		handles: make(map[SessionHandle]*codexHandle),
		logPath: logPath,
	}
}

func (a *codexCLIAdapter) ProviderID() string {
	return "codex"
}

func (a *codexCLIAdapter) Start(_ context.Context, req StartRequest) (SessionHandle, error) {
	handle := SessionHandle(newID("codex"))
	h := &codexHandle{
		sessionID: req.SessionID,
		repoPath:  req.RepoPath,
		events:    make(chan Event, 64),
	}

	a.mu.Lock()
	a.handles[handle] = h
	a.mu.Unlock()

	h.events <- Event{
		Type:      EventReady,
		SessionID: req.SessionID,
		Provider:  "codex",
		RepoPath:  req.RepoPath,
		Message:   "codex adapter ready",
	}
	return handle, nil
}

func (a *codexCLIAdapter) Stop(_ context.Context, handle SessionHandle) error {
	a.mu.Lock()
	h := a.handles[handle]
	delete(a.handles, handle)
	a.mu.Unlock()
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if !h.closed {
		h.closed = true
		close(h.events)
	}
	return nil
}

func (a *codexCLIAdapter) Send(ctx context.Context, handle SessionHandle, prompt PromptRequest) error {
	a.mu.Lock()
	h := a.handles[handle]
	a.mu.Unlock()
	if h == nil {
		return errors.New("codex session handle not found")
	}

	go func() {
		result, err := a.runCodexPrompt(ctx, h, prompt, func(chunk string) {
			h.safeEmit(Event{
				Type:      EventChunk,
				SessionID: h.sessionID,
				Provider:  "codex",
				RepoPath:  prompt.RepoPath,
				RequestID: prompt.ID,
				Text:      chunk,
			})
		})
		if result.ThreadID != "" {
			h.mu.Lock()
			h.codexID = result.ThreadID
			h.mu.Unlock()
		}

		if err != nil {
			msg := result.FailureMessage
			if msg == "" {
				msg = "codex exec failed"
			}
			h.safeEmit(Event{
				Type:      EventError,
				SessionID: h.sessionID,
				Provider:  "codex",
				RepoPath:  prompt.RepoPath,
				RequestID: prompt.ID,
				Message:   msg,
				Err:       err,
			})
			return
		}

		clean := strings.TrimSpace(result.LastMessage)
		if clean == "" {
			h.safeEmit(Event{
				Type:      EventError,
				SessionID: h.sessionID,
				Provider:  "codex",
				RepoPath:  prompt.RepoPath,
				RequestID: prompt.ID,
				Message:   "codex returned no assistant message",
			})
			return
		}

		h.safeEmit(Event{
			Type:      EventFinal,
			SessionID: h.sessionID,
			Provider:  "codex",
			RepoPath:  prompt.RepoPath,
			RequestID: prompt.ID,
			Text:      clean,
		})
	}()
	return nil
}

func (a *codexCLIAdapter) Events(handle SessionHandle) <-chan Event {
	a.mu.Lock()
	defer a.mu.Unlock()
	if h := a.handles[handle]; h != nil {
		return h.events
	}
	ch := make(chan Event)
	close(ch)
	return ch
}

func (h *codexHandle) safeEmit(ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.events <- ev
}

func (a *codexCLIAdapter) runCodexPrompt(ctx context.Context, h *codexHandle, prompt PromptRequest, onChunk func(string)) (codexRunResult, error) {
	result := codexRunResult{}

	h.mu.Lock()
	existingID := h.codexID
	h.mu.Unlock()

	outputPath := ""
	if existingID == "" {
		outFile, err := os.CreateTemp("", "open-pilot-codex-last-*.txt")
		if err != nil {
			return result, fmt.Errorf("create temp output file: %w", err)
		}
		outputPath = outFile.Name()
		if closeErr := outFile.Close(); closeErr != nil {
			_ = os.Remove(outputPath)
			return result, fmt.Errorf("close temp output file: %w", closeErr)
		}
		defer os.Remove(outputPath)
	}

	args := codexArgs(existingID, outputPath, prompt.Text)
	a.logf("run", "session=%s request=%s repo=%s args=%q", h.sessionID, prompt.ID, prompt.RepoPath, strings.Join(args, " "))
	cmd := exec.CommandContext(ctx, a.binary, args...)
	cmd.Dir = prompt.RepoPath

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return result, fmt.Errorf("codex stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return result, fmt.Errorf("codex stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return result, fmt.Errorf("start codex: %w", err)
	}

	var mu sync.Mutex
	lastErrorMsg := ""
	turnFailedMsg := ""
	lastStderrMsg := ""
	stderrLines := make([]string, 0, 8)
	var streamedText strings.Builder
	lastAgentMessage := ""

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		s := bufio.NewScanner(stdout)
		for s.Scan() {
			a.logf("stdout", "%s", s.Text())
			ev, raw, ok := parseCodexJSONLine(s.Bytes())
			if !ok {
				continue
			}
			mu.Lock()
			switch ev.Type {
			case "thread.started":
				if ev.ThreadID != "" {
					result.ThreadID = strings.TrimSpace(ev.ThreadID)
				}
			case "error":
				if msg := normalizeCodexErrorMessage(ev.Message); msg != "" {
					lastErrorMsg = msg
				}
			case "turn.failed":
				if msg := normalizeCodexErrorMessage(ev.Error.Message); msg != "" {
					turnFailedMsg = msg
				}
			}
			if msg := extractCompletedAgentMessage(raw); msg != "" {
				lastAgentMessage = msg
			}
			if chunk := extractCodexPreviewChunk(ev, raw); chunk != "" {
				a.logf("chunk", "event_type=%s len=%d", ev.Type, len(chunk))
				streamedText.WriteString(chunk)
				if onChunk != nil {
					onChunk(chunk)
				}
			}
			mu.Unlock()
		}
	}()
	go func() {
		defer wg.Done()
		s := bufio.NewScanner(stderr)
		for s.Scan() {
			rawLine := strings.TrimSpace(s.Text())
			a.logf("stderr", "%s", rawLine)
			if rawLine == "" {
				continue
			}
			mu.Lock()
			stderrLines = append(stderrLines, rawLine)
			mu.Unlock()

			msg := normalizeCodexStderrLine(s.Text())
			if msg == "" {
				continue
			}
			mu.Lock()
			lastStderrMsg = msg
			mu.Unlock()
		}
	}()

	waitErr := cmd.Wait()
	if waitErr != nil {
		a.logf("exit", "request=%s err=%v", prompt.ID, waitErr)
	} else {
		a.logf("exit", "request=%s ok", prompt.ID)
	}
	wg.Wait()

	readErr := error(nil)
	if outputPath != "" {
		content, err := os.ReadFile(outputPath)
		readErr = err
		if err == nil {
			result.LastMessage = string(content)
		}
	} else {
		result.LastMessage = streamedText.String()
		if strings.TrimSpace(lastAgentMessage) != "" {
			result.LastMessage = lastAgentMessage
		}
	}

	if waitErr != nil {
		mu.Lock()
		result.FailureMessage = summarizeCodexFailure(turnFailedMsg, lastErrorMsg, lastStderrMsg)
		if len(stderrLines) > 0 {
			details := strings.Join(stderrLines, "\n")
			if result.FailureMessage == "" {
				result.FailureMessage = details
			} else if !strings.Contains(details, result.FailureMessage) {
				result.FailureMessage = result.FailureMessage + "\n" + details
			}
		}
		mu.Unlock()
		if result.FailureMessage == "" {
			result.FailureMessage = "codex exec failed"
		}
		a.logf("failure", "request=%s msg=%q", prompt.ID, result.FailureMessage)
		return result, waitErr
	}

	if readErr != nil {
		result.FailureMessage = "failed to read codex output"
		return result, readErr
	}

	if strings.TrimSpace(result.LastMessage) == "" {
		result.FailureMessage = "codex returned no assistant message"
		a.logf("failure", "request=%s msg=%q", prompt.ID, result.FailureMessage)
		return result, errors.New(result.FailureMessage)
	}

	a.logf("final", "request=%s final_len=%d streamed_len=%d", prompt.ID, len(strings.TrimSpace(result.LastMessage)), streamedText.Len())
	return result, nil
}

func codexArgs(existingID, outputPath, prompt string) []string {
	if existingID != "" {
		return []string{"exec", "resume", "--json", "--skip-git-repo-check", "--", existingID, prompt}
	}
	return []string{"exec", "--json", "--skip-git-repo-check", "--output-last-message", outputPath, "--", prompt}
}

func parseCodexJSONLine(line []byte) (codexJSONEvent, map[string]any, bool) {
	trimmed := strings.TrimSpace(string(line))
	if trimmed == "" || !strings.HasPrefix(trimmed, "{") {
		return codexJSONEvent{}, nil, false
	}

	var ev codexJSONEvent
	if err := json.Unmarshal([]byte(trimmed), &ev); err != nil {
		return codexJSONEvent{}, nil, false
	}
	if ev.Type == "" {
		return codexJSONEvent{}, nil, false
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(trimmed), &raw); err != nil {
		raw = nil
	}
	return ev, raw, true
}

func summarizeCodexFailure(turnFailedMsg, lastErrorMsg, lastStderrMsg string) string {
	if turnFailedMsg != "" {
		return turnFailedMsg
	}
	if lastErrorMsg != "" {
		return lastErrorMsg
	}
	if lastStderrMsg != "" {
		return lastStderrMsg
	}
	return ""
}

func normalizeCodexErrorMessage(msg string) string {
	msg = strings.TrimSpace(msg)
	if msg == "" {
		return ""
	}
	if strings.HasPrefix(msg, "Reconnecting...") {
		return ""
	}
	if strings.Contains(strings.ToLower(msg), "failed to shutdown rollout recorder") {
		return ""
	}
	return msg
}

func normalizeCodexStderrLine(line string) string {
	msg := strings.TrimSpace(line)
	if msg == "" {
		return ""
	}
	lower := strings.ToLower(msg)
	if strings.Contains(lower, "failed to record rollout items") {
		return ""
	}
	if strings.Contains(lower, "failed to flush rollout recorder") {
		return ""
	}
	if strings.Contains(lower, "failed to shutdown rollout recorder") {
		return ""
	}
	if strings.Contains(lower, "failed to create shell snapshot") {
		return ""
	}
	if strings.HasPrefix(lower, "warning: proceeding, even though we could not update path") {
		return ""
	}
	if strings.Contains(lower, "warn codex_core::") || strings.Contains(lower, "error codex_core::") {
		return ""
	}
	return msg
}

func extractCodexPreviewChunk(ev codexJSONEvent, raw map[string]any) string {
	if msg := extractCompletedAgentMessage(raw); msg != "" {
		return msg + "\n\n"
	}
	if !isPreviewEventType(ev.Type) {
		return ""
	}
	if ev.Delta != "" {
		return ev.Delta
	}
	if ev.Text != "" {
		return ev.Text
	}
	if s := findFirstString(raw, "delta", "text"); s != "" {
		return s
	}
	return ""
}

func extractCompletedAgentMessage(raw map[string]any) string {
	if raw == nil {
		return ""
	}
	t, _ := raw["type"].(string)
	if t != "item.completed" {
		return ""
	}
	item, _ := raw["item"].(map[string]any)
	if item == nil {
		return ""
	}
	itemType, _ := item["type"].(string)
	if itemType != "agent_message" {
		return ""
	}
	text, _ := item["text"].(string)
	return strings.TrimSpace(text)
}

func isPreviewEventType(eventType string) bool {
	t := strings.ToLower(strings.TrimSpace(eventType))
	if t == "" {
		return false
	}
	if strings.Contains(t, "error") || strings.Contains(t, "failed") {
		return false
	}
	if t == "thread.started" || t == "turn.started" || t == "turn.completed" {
		return false
	}
	if strings.Contains(t, "delta") ||
		strings.Contains(t, "chunk") ||
		strings.Contains(t, "token") ||
		strings.Contains(t, "output_text") ||
		strings.Contains(t, "message") {
		return true
	}
	return false
}

func findFirstString(v any, keys ...string) string {
	switch x := v.(type) {
	case map[string]any:
		for _, key := range keys {
			if value, ok := x[key]; ok {
				if s, ok := value.(string); ok && s != "" {
					return s
				}
			}
		}
		for _, value := range x {
			if s := findFirstString(value, keys...); s != "" {
				return s
			}
		}
	case []any:
		for _, item := range x {
			if s := findFirstString(item, keys...); s != "" {
				return s
			}
		}
	}
	return ""
}

func (a *codexCLIAdapter) logf(kind string, format string, args ...any) {
	if strings.TrimSpace(a.logPath) == "" {
		return
	}
	line := fmt.Sprintf(format, args...)
	stamp := time.Now().Format(time.RFC3339Nano)
	entry := fmt.Sprintf("%s [%s] %s\n", stamp, kind, line)

	a.logMu.Lock()
	defer a.logMu.Unlock()

	f, err := os.OpenFile(a.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.WriteString(entry)
}
