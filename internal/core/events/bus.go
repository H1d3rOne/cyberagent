package events

import (
	"sync"

	"github.com/H1d3rOne/cyberagent/internal/core/contracts"
)

// Bus publishes events to a persistent store and in-memory subscribers.
type Bus struct {
	store *Store
	mu    sync.RWMutex
	subs  map[chan contracts.Event]struct{}
}

// NewBus creates an event bus backed by store.
func NewBus(store *Store) *Bus {
	return &Bus{store: store, subs: map[chan contracts.Event]struct{}{}}
}

// Publish appends and broadcasts an event.
func (b *Bus) Publish(event contracts.Event) error {
	if b.store != nil {
		if err := b.store.Append(event); err != nil {
			return err
		}
	}

	b.mu.RLock()
	defer b.mu.RUnlock()
	for ch := range b.subs {
		select {
		case ch <- event:
		default:
		}
	}
	return nil
}

// Subscribe subscribes to future events. Call the returned cancel function to unsubscribe.
func (b *Bus) Subscribe(buffer int) (<-chan contracts.Event, func()) {
	if buffer < 0 {
		buffer = 0
	}
	ch := make(chan contracts.Event, buffer)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()

	cancel := func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
	return ch, cancel
}
