package providers

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/thwoodle/open-pilot/internal/config"
)

// Manager coordinates provider adapters and emits a unified event stream.
type Manager interface {
	SetProviderConfig(providerID string, cfg config.ProviderConfig)
	SendPrompt(ctx context.Context, providerID, sessionID, repoPath, requestID, prompt string) error
	Events() <-chan Event
	StopAll(ctx context.Context) error
}

type service struct {
	mu sync.Mutex

	providerCfg map[string]config.ProviderConfig
	adapters    map[string]Adapter
	handles     map[string]SessionHandle

	events chan Event
}

func NewManager(cfg config.Config) Manager {
	providerCfg := make(map[string]config.ProviderConfig, len(cfg.Providers))
	for k, v := range cfg.Providers {
		providerCfg[k] = v
	}

	return &service{
		providerCfg: providerCfg,
		adapters:    make(map[string]Adapter),
		handles:     make(map[string]SessionHandle),
		events:      make(chan Event, 256),
	}
}

func (s *service) SetProviderConfig(providerID string, cfg config.ProviderConfig) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.providerCfg[providerID] = cfg
	delete(s.adapters, providerID)
}

func (s *service) SendPrompt(ctx context.Context, providerID, sessionID, repoPath, requestID, prompt string) error {
	if providerID == "" || sessionID == "" || repoPath == "" {
		return errors.New("provider, session, and repo path are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	s.mu.Lock()
	adapter, cfg, err := s.adapterForLocked(providerID)
	if err != nil {
		s.mu.Unlock()
		return err
	}
	key := handleKey(providerID, sessionID, repoPath)
	handle, ok := s.handles[key]
	var eventStream <-chan Event
	if !ok {
		handle, err = adapter.Start(ctx, StartRequest{SessionID: sessionID, Provider: providerID, RepoPath: repoPath})
		if err != nil {
			s.mu.Unlock()
			return err
		}
		eventStream = adapter.Events(handle)
		timeout := cfg.StartupTimeout
		if timeout <= 0 {
			timeout = 10 * time.Second
		}
		if err := waitReady(ctx, timeout, eventStream); err != nil {
			_ = adapter.Stop(ctx, handle)
			s.mu.Unlock()
			return err
		}
		s.handles[key] = handle
		s.events <- Event{Type: EventReady, SessionID: sessionID, Provider: providerID, RepoPath: repoPath, Message: "provider ready"}
		go s.forwardEvents(eventStream, key)
	}
	s.mu.Unlock()

	return adapter.Send(ctx, handle, PromptRequest{ID: requestID, SessionID: sessionID, Text: prompt, RepoPath: repoPath})
}

func (s *service) Events() <-chan Event {
	return s.events
}

func (s *service) StopAll(ctx context.Context) error {
	s.mu.Lock()
	entries := make([]struct {
		adapter Adapter
		handle  SessionHandle
	}, 0, len(s.handles))
	for key, handle := range s.handles {
		providerID, _, _, err := parseHandleKey(key)
		if err != nil {
			continue
		}
		adapter := s.adapters[providerID]
		if adapter != nil {
			entries = append(entries, struct {
				adapter Adapter
				handle  SessionHandle
			}{adapter: adapter, handle: handle})
		}
	}
	s.mu.Unlock()

	var firstErr error
	for _, e := range entries {
		if err := e.adapter.Stop(ctx, e.handle); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *service) adapterForLocked(providerID string) (Adapter, config.ProviderConfig, error) {
	cfg, ok := s.providerCfg[providerID]
	if !ok {
		return nil, config.ProviderConfig{}, fmt.Errorf("provider %q is not configured", providerID)
	}
	if adapter := s.adapters[providerID]; adapter != nil {
		return adapter, cfg, nil
	}
	cfg.ID = providerID
	var adapter Adapter
	if providerID == "codex" {
		binary := cfg.Command
		if binary == "" || binary == "open-pilot-codex-wrapper" {
			binary = "codex"
		}
		adapter = newCodexCLIAdapter(binary)
	} else {
		adapter = newProcessAdapter(cfg)
	}
	s.adapters[providerID] = adapter
	return adapter, cfg, nil
}

func (s *service) forwardEvents(ch <-chan Event, key string) {
	for ev := range ch {
		s.events <- ev
	}

	s.mu.Lock()
	delete(s.handles, key)
	s.mu.Unlock()
}

func handleKey(providerID, sessionID, repoPath string) string {
	return providerID + "\x00" + sessionID + "\x00" + repoPath
}

func parseHandleKey(key string) (providerID, sessionID, repoPath string, err error) {
	parts := split3(key)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid handle key")
	}
	return parts[0], parts[1], parts[2], nil
}

func split3(s string) []string {
	out := make([]string, 0, 3)
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\x00' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func waitReady(ctx context.Context, timeout time.Duration, events <-chan Event) error {
	t := time.NewTimer(timeout)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-t.C:
			return errors.New("provider startup timeout")
		case ev, ok := <-events:
			if !ok {
				return errors.New("provider closed before ready")
			}
			if ev.Type == EventReady {
				return nil
			}
		}
	}
}
