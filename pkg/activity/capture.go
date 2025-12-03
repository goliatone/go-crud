package activity

import (
	"context"
	"sync"
)

// CaptureHook records events for assertions in tests.
type CaptureHook struct {
	Events []Event
	Err    error
	mu     sync.Mutex
}

// Notify records the event and returns any configured error.
func (h *CaptureHook) Notify(_ context.Context, event Event) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.Events = append(h.Events, NormalizeEvent(event))
	return h.Err
}
