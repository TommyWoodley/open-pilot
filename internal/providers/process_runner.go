package providers

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"

	"github.com/thwoodle/open-pilot/internal/config"
)

type processAdapter struct {
	providerID string
	cfg        config.ProviderConfig

	mu      sync.Mutex
	handles map[SessionHandle]*processHandle
}

type processHandle struct {
	sessionID string
	repoPath  string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	events    chan Event
	waitDone  chan struct{}
}

func newProcessAdapter(cfg config.ProviderConfig) Adapter {
	return &processAdapter{
		providerID: cfg.ID,
		cfg:        cfg,
		handles:    make(map[SessionHandle]*processHandle),
	}
}

func (p *processAdapter) ProviderID() string {
	return p.providerID
}

func (p *processAdapter) Start(ctx context.Context, req StartRequest) (SessionHandle, error) {
	if req.RepoPath == "" {
		return "", errors.New("repo path is required")
	}
	if p.cfg.Command == "" {
		return "", fmt.Errorf("provider %s command is not configured", p.providerID)
	}
	if _, err := exec.LookPath(p.cfg.Command); err != nil {
		return "", fmt.Errorf("provider %s command %q not found in PATH", p.providerID, p.cfg.Command)
	}

	cmd := exec.CommandContext(ctx, p.cfg.Command, p.cfg.Args...)
	cmd.Dir = req.RepoPath
	cmd.Env = os.Environ()
	for k, v := range p.cfg.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return "", fmt.Errorf("stderr pipe: %w", err)
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return "", fmt.Errorf("stdin pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start provider process: %w", err)
	}

	h := SessionHandle(newID("handle"))
	proc := &processHandle{
		sessionID: req.SessionID,
		repoPath:  req.RepoPath,
		cmd:       cmd,
		stdin:     stdin,
		events:    make(chan Event, 128),
		waitDone:  make(chan struct{}),
	}

	p.mu.Lock()
	p.handles[h] = proc
	p.mu.Unlock()

	go p.readStdout(h, proc, stdout)
	go p.readStderr(h, proc, stderr)
	go p.waitProcess(h, proc)

	return h, nil
}

func (p *processAdapter) Stop(_ context.Context, handle SessionHandle) error {
	p.mu.Lock()
	proc := p.handles[handle]
	p.mu.Unlock()
	if proc == nil {
		return nil
	}

	_ = p.sendControl(proc, map[string]any{"type": "shutdown"})
	if proc.cmd.Process != nil {
		_ = proc.cmd.Process.Kill()
	}
	<-proc.waitDone
	return nil
}

func (p *processAdapter) Send(_ context.Context, handle SessionHandle, prompt PromptRequest) error {
	p.mu.Lock()
	proc := p.handles[handle]
	p.mu.Unlock()
	if proc == nil {
		return errors.New("provider handle not found")
	}

	msg := map[string]any{
		"type":       "prompt",
		"id":         prompt.ID,
		"text":       prompt.Text,
		"repo_path":  prompt.RepoPath,
		"session_id": prompt.SessionID,
	}
	return p.sendControl(proc, msg)
}

func (p *processAdapter) Events(handle SessionHandle) <-chan Event {
	p.mu.Lock()
	defer p.mu.Unlock()
	if proc := p.handles[handle]; proc != nil {
		return proc.events
	}
	ch := make(chan Event)
	close(ch)
	return ch
}

func (p *processAdapter) sendControl(proc *processHandle, payload map[string]any) error {
	enc := json.NewEncoder(proc.stdin)
	if err := enc.Encode(payload); err != nil {
		return fmt.Errorf("write provider control message: %w", err)
	}
	return nil
}

func (p *processAdapter) readStdout(_ SessionHandle, proc *processHandle, r io.Reader) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		line := s.Text()
		e, err := parseWrapperEvent([]byte(line))
		if err != nil {
			logProviderDiagnostic(p.providerID, proc.sessionID, "", "", EventError, "invalid wrapper JSON event", line)
			proc.events <- Event{Type: EventError, SessionID: proc.sessionID, Provider: p.providerID, RepoPath: proc.repoPath, Message: "invalid provider JSON event", Err: err}
			continue
		}
		e.SessionID = proc.sessionID
		e.Provider = p.providerID
		e.RepoPath = proc.repoPath
		if e.Type == EventUnknown {
			logProviderDiagnostic(p.providerID, proc.sessionID, e.RequestID, e.RawType, e.Type, e.DebugNote, e.RawJSON)
		}
		proc.events <- e
	}
	if err := s.Err(); err != nil {
		proc.events <- Event{Type: EventError, SessionID: proc.sessionID, Provider: p.providerID, RepoPath: proc.repoPath, Message: "provider stdout read error", Err: err}
	}
}

func (p *processAdapter) readStderr(_ SessionHandle, proc *processHandle, r io.Reader) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		proc.events <- Event{Type: EventStatus, SessionID: proc.sessionID, Provider: p.providerID, RepoPath: proc.repoPath, Message: s.Text()}
	}
}

func (p *processAdapter) waitProcess(handle SessionHandle, proc *processHandle) {
	err := proc.cmd.Wait()
	if err != nil {
		proc.events <- Event{Type: EventExited, SessionID: proc.sessionID, Provider: p.providerID, RepoPath: proc.repoPath, Message: "provider process exited", Err: err}
	} else {
		proc.events <- Event{Type: EventExited, SessionID: proc.sessionID, Provider: p.providerID, RepoPath: proc.repoPath, Message: "provider process exited"}
	}

	p.mu.Lock()
	delete(p.handles, handle)
	p.mu.Unlock()
	close(proc.events)
	close(proc.waitDone)
}

func unmarshalJSON(data []byte, target any) error {
	return json.Unmarshal(data, target)
}
