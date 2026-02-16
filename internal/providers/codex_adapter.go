package providers

import (
	"context"
	"errors"
	"os/exec"
	"regexp"
	"strings"
	"sync"
)

type codexCLIAdapter struct {
	binary string

	mu      sync.Mutex
	handles map[SessionHandle]*codexHandle
}

type codexHandle struct {
	sessionID string
	repoPath  string
	events    chan Event
	mu        sync.Mutex
	closed    bool
	codexID   string
}

var codexSessionIDPattern = regexp.MustCompile(`(?im)session id:\s*([a-z0-9-]+)`)

func newCodexCLIAdapter(binary string) Adapter {
	return &codexCLIAdapter{
		binary:  binary,
		handles: make(map[SessionHandle]*codexHandle),
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
		args := []string{"exec"}
		h.mu.Lock()
		existingCodexID := h.codexID
		h.mu.Unlock()
		if existingCodexID != "" {
			args = append(args, "resume", existingCodexID, prompt.Text)
		} else {
			args = append(args, prompt.Text)
		}

		cmd := exec.CommandContext(ctx, a.binary, args...)
		cmd.Dir = prompt.RepoPath
		out, err := cmd.CombinedOutput()
		text := strings.TrimSpace(string(out))
		if parsedCodexID := extractCodexSessionID(text); parsedCodexID != "" {
			h.mu.Lock()
			h.codexID = parsedCodexID
			h.mu.Unlock()
		}

		if err != nil {
			h.safeEmit(Event{
				Type:      EventError,
				SessionID: h.sessionID,
				Provider:  "codex",
				RepoPath:  prompt.RepoPath,
				RequestID: prompt.ID,
				Message:   "codex exec failed: " + text,
				Err:       err,
			})
			return
		}

		h.safeEmit(Event{
			Type:      EventFinal,
			SessionID: h.sessionID,
			Provider:  "codex",
			RepoPath:  prompt.RepoPath,
			RequestID: prompt.ID,
			Text:      text,
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

func extractCodexSessionID(output string) string {
	m := codexSessionIDPattern.FindStringSubmatch(output)
	if len(m) < 2 {
		return ""
	}
	return strings.TrimSpace(m[1])
}
