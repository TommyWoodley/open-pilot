package providers

import (
	"context"
	"testing"
	"time"
)

func TestWaitReady(t *testing.T) {
	t.Parallel()

	ch := make(chan Event, 1)
	ch <- Event{Type: EventReady}

	if err := waitReady(context.Background(), 50*time.Millisecond, ch); err != nil {
		t.Fatalf("expected ready, got err: %v", err)
	}
}

func TestWaitReadyTimeout(t *testing.T) {
	t.Parallel()

	ch := make(chan Event)
	err := waitReady(context.Background(), 20*time.Millisecond, ch)
	if err == nil {
		t.Fatalf("expected timeout error")
	}
}
