package resolvers

import (
	"context"
	"errors"
	"log"
	"strings"
	"sync"

	"github.com/goliatone/go-crud"
	"github.com/goliatone/go-crud/examples/relationships-gql"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/dataloader"
	"github.com/goliatone/go-crud/examples/relationships-gql/graph/model"
	repository "github.com/goliatone/go-repository-bun"
)

// Custom resolver stubs. Safe to edit.
// Resolver satisfies gqlgen bindings and can hold your dependencies.
type Resolver struct {
	ScopeGuard         ScopeGuardFunc
	ContextFactory     ContextFactory
	Loaders            *dataloader.Loader
	Events             EventBus
	AuthorSvc          crud.Service[model.Author]
	AuthorProfileSvc   crud.Service[model.AuthorProfile]
	BookSvc            crud.Service[model.Book]
	ChapterSvc         crud.Service[model.Chapter]
	HeadquartersSvc    crud.Service[model.Headquarters]
	PublishingHouseSvc crud.Service[model.PublishingHouse]
	TagSvc             crud.Service[model.Tag]
}

// NewResolver wires CRUD services backed by the shared repositories.
func NewResolver(repos relationships.Repositories) *Resolver {
	svc := newServices(repos)
	return &Resolver{
		ContextFactory:     NewCRUDContext,
		Events:             NewEventBus(),
		AuthorSvc:          svc.author,
		AuthorProfileSvc:   svc.authorProfile,
		BookSvc:            svc.book,
		ChapterSvc:         svc.chapter,
		HeadquartersSvc:    svc.headquarters,
		PublishingHouseSvc: svc.publishingHouse,
		TagSvc:             svc.tag,
	}
}

// Hook stubs (safe to edit); wire your auth/scope/preload/wrapping/error logic here.
func (r *Resolver) AuthGuard(ctx context.Context, entity, action string) error {
	return nil
}

func (r *Resolver) ScopeHook(ctx context.Context, entity, action string) error {
	return nil
}

func (r *Resolver) PreloadHook(ctx context.Context, entity, action string, criteria []repository.SelectCriteria) []repository.SelectCriteria {
	return criteria
}

func (r *Resolver) WrapService(ctx context.Context, entity, action string, svc crud.Service[any]) crud.Service[any] {
	return svc
}

func (r *Resolver) HandleError(ctx context.Context, entity, action string, err error) error {
	return err
}

// Event bus contract for subscription publish/subscribe.
type EventBus interface {
	Publish(ctx context.Context, topic string, payload any) error
	Subscribe(ctx context.Context, topic string) (<-chan EventMessage, error)
}

// EventMessage carries subscription payloads and errors from the bus.
type EventMessage struct {
	Topic   string
	Payload any
	Err     error
}

// InMemoryEventBus is a lightweight subscription bus for the demo server.
type InMemoryEventBus struct {
	mu     sync.RWMutex
	subs   map[string]map[chan EventMessage]struct{}
	closed bool
}

// NewEventBus creates an in-memory broadcast bus.
func NewEventBus() *InMemoryEventBus {
	return &InMemoryEventBus{
		subs: make(map[string]map[chan EventMessage]struct{}),
	}
}

// Publish fan-outs payloads to topic subscribers (best effort).
func (b *InMemoryEventBus) Publish(ctx context.Context, topic string, payload any) error {
	log.Printf("[events] publish topic=%s payload_type=%T", topic, payload)
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.closed {
		return errors.New("event bus closed")
	}
	targets := b.subs[topic]
	for ch := range targets {
		select {
		case ch <- EventMessage{Topic: topic, Payload: payload}:
		default:
		}
	}
	return nil
}

// Subscribe registers a subscriber for a topic and removes it on ctx cancellation.
func (b *InMemoryEventBus) Subscribe(ctx context.Context, topic string) (<-chan EventMessage, error) {
	if strings.TrimSpace(topic) == "" {
		return nil, errors.New("topic is required")
	}
	log.Printf("[events] subscribe topic=%s", topic)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errors.New("event bus closed")
	}
	ch := make(chan EventMessage, 1)
	if b.subs[topic] == nil {
		b.subs[topic] = make(map[chan EventMessage]struct{})
	}
	b.subs[topic][ch] = struct{}{}

	go func() {
		<-ctx.Done()
		b.mu.Lock()
		defer b.mu.Unlock()
		if subs := b.subs[topic]; subs != nil {
			delete(subs, ch)
			if len(subs) == 0 {
				delete(b.subs, topic)
			}
		}
		close(ch)
		log.Printf("[events] unsubscribe topic=%s", topic)
	}()

	return ch, nil
}

// Close drains all subscribers and prevents new registrations.
func (b *InMemoryEventBus) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return
	}
	b.closed = true
	for topic, subs := range b.subs {
		for ch := range subs {
			close(ch)
		}
		delete(b.subs, topic)
	}
}
