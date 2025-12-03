package activity

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEmitterAppliesDefaultChannel(t *testing.T) {
	capture := &CaptureHook{}
	emitter := NewEmitter(Hooks{capture}, Config{Enabled: true})

	err := emitter.Emit(context.Background(), Event{
		Verb:       "crud.sample.create",
		ObjectType: "sample",
		ObjectID:   "id",
	})
	require.NoError(t, err)
	require.Len(t, capture.Events, 1)
	require.Equal(t, "crud", capture.Events[0].Channel)
}

func TestEmitterRespectsExistingChannel(t *testing.T) {
	capture := &CaptureHook{}
	emitter := NewEmitter(Hooks{capture}, Config{Enabled: true, Channel: "custom"})

	err := emitter.Emit(context.Background(), Event{
		Verb:       "crud.sample.create",
		ObjectType: "sample",
		ObjectID:   "id",
		Channel:    "explicit",
	})
	require.NoError(t, err)
	require.Len(t, capture.Events, 1)
	require.Equal(t, "explicit", capture.Events[0].Channel)
}

func TestEmitterDisabledWhenNoHooks(t *testing.T) {
	emitter := NewEmitter(nil, Config{Enabled: true})
	require.False(t, emitter.Enabled())
	require.NoError(t, emitter.Emit(context.Background(), Event{
		Verb:       "crud.sample.create",
		ObjectType: "sample",
		ObjectID:   "id",
	}))
}
