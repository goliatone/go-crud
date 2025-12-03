package activity

import (
	"context"
	"strings"
)

// Config controls activity emission defaults supplied by DI/config.
type Config struct {
	Enabled bool
	Channel string
}

// Emitter fans out events to hooks while applying defaults.
type Emitter struct {
	hooks   Hooks
	enabled bool
	channel string
}

// NewEmitter constructs an emitter from hooks and configuration.
func NewEmitter(hooks Hooks, cfg Config) *Emitter {
	channel := strings.TrimSpace(cfg.Channel)
	if channel == "" {
		channel = "crud"
	}
	normalizedHooks := cloneHooks(hooks)
	return &Emitter{
		hooks:   normalizedHooks,
		enabled: cfg.Enabled && len(normalizedHooks) > 0,
		channel: channel,
	}
}

// Enabled reports whether emissions should be attempted.
func (e *Emitter) Enabled() bool {
	return e != nil && e.enabled && len(e.hooks) > 0
}

// Emit forwards the event to all hooks, applying default channel when missing.
func (e *Emitter) Emit(ctx context.Context, event Event) error {
	if !e.Enabled() {
		return nil
	}
	if strings.TrimSpace(event.Channel) == "" && e.channel != "" {
		event.Channel = e.channel
	}
	return e.hooks.Notify(ctx, event)
}

func cloneHooks(hooks Hooks) Hooks {
	if len(hooks) == 0 {
		return nil
	}
	normalized := make([]ActivityHook, 0, len(hooks))
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		normalized = append(normalized, hook)
	}
	if len(normalized) == 0 {
		return nil
	}
	return Hooks(normalized)
}
