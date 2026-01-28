package refresh

import (
	"context"
	"sync"
	"time"

	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-crud/gql/generator"
)

// GeneratorFunc executes a GraphQL generation cycle.
type GeneratorFunc func(context.Context, generator.Options) (generator.Result, error)

// ListenerRegistrar wires a schema registry listener.
type ListenerRegistrar func(crud.SchemaListener)

// Options configure registry-driven refreshes.
type Options struct {
	Generator        GeneratorFunc
	GeneratorOptions generator.Options
	Debounce         time.Duration
	OnResult         func(generator.Result)
	OnError          func(error)
	RegisterListener ListenerRegistrar
}

// Refresher listens for schema registry updates and regenerates GraphQL output.
type Refresher struct {
	ctx  context.Context
	opts Options

	mu      sync.Mutex
	timer   *time.Timer
	running bool
	pending bool
}

// RegisterSchemaRefresh subscribes to schema registry updates and triggers
// generator runs when schemas change (e.g. content type publish).
func RegisterSchemaRefresh(ctx context.Context, opts Options) *Refresher {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Generator == nil {
		opts.Generator = generator.Generate
	}
	register := opts.RegisterListener
	if register == nil {
		register = crud.RegisterSchemaListener
	}

	refresher := &Refresher{ctx: ctx, opts: opts}
	register(func(entry crud.SchemaEntry) {
		refresher.Trigger()
	})
	return refresher
}

// Trigger schedules a refresh immediately (honoring debounce settings).
func (r *Refresher) Trigger() {
	if r == nil {
		return
	}
	if r.ctx.Err() != nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.opts.Debounce > 0 {
		if r.timer == nil {
			r.timer = time.AfterFunc(r.opts.Debounce, r.flush)
		} else {
			r.timer.Reset(r.opts.Debounce)
		}
		return
	}

	if r.running {
		r.pending = true
		return
	}

	r.running = true
	go r.run()
}

func (r *Refresher) flush() {
	if r.ctx.Err() != nil {
		return
	}

	r.mu.Lock()
	if r.running {
		r.pending = true
		r.mu.Unlock()
		return
	}
	r.running = true
	r.mu.Unlock()

	r.run()
}

func (r *Refresher) run() {
	if r.ctx.Err() != nil {
		r.finish(false)
		return
	}

	result, err := r.opts.Generator(r.ctx, r.opts.GeneratorOptions)
	if err != nil {
		if r.opts.OnError != nil {
			r.opts.OnError(err)
		}
	} else if r.opts.OnResult != nil {
		r.opts.OnResult(result)
	}

	r.finish(true)
}

func (r *Refresher) finish(allowPending bool) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if allowPending && r.pending && r.ctx.Err() == nil {
		r.pending = false
		go r.run()
		return
	}

	r.running = false
}
