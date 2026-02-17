package providers

import (
	"encoding/json"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultProviderDebugLogPath = "/tmp/open-pilot-provider-events.log"
	providerDebugLogMaxSize     = 10 * 1024 * 1024
)

var providerDebugLogMu sync.Mutex

type providerDiagnosticEntry struct {
	Timestamp      string `json:"timestamp"`
	Provider       string `json:"provider,omitempty"`
	SessionID      string `json:"session_id,omitempty"`
	RequestID      string `json:"request_id,omitempty"`
	RawType        string `json:"raw_type,omitempty"`
	NormalizedType string `json:"normalized_type,omitempty"`
	Reason         string `json:"reason,omitempty"`
	RawJSON        string `json:"raw_json,omitempty"`
}

func logProviderDiagnostic(provider, sessionID, requestID, rawType, normalizedType, reason, rawJSON string) {
	entry := providerDiagnosticEntry{
		Timestamp:      time.Now().Format(time.RFC3339Nano),
		Provider:       strings.TrimSpace(provider),
		SessionID:      strings.TrimSpace(sessionID),
		RequestID:      strings.TrimSpace(requestID),
		RawType:        strings.TrimSpace(rawType),
		NormalizedType: strings.TrimSpace(normalizedType),
		Reason:         strings.TrimSpace(reason),
		RawJSON:        strings.TrimSpace(rawJSON),
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}

	path := strings.TrimSpace(os.Getenv("OPEN_PILOT_PROVIDER_DEBUG_LOG"))
	if path == "" {
		path = defaultProviderDebugLogPath
	}

	providerDebugLogMu.Lock()
	defer providerDebugLogMu.Unlock()

	if st, err := os.Stat(path); err == nil && st.Size() >= providerDebugLogMaxSize {
		backup := path + ".1"
		_ = os.Remove(backup)
		_ = os.Rename(path, backup)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = f.Write(append(data, '\n'))
}
