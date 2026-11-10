// Package events is a tiny in-process publish/subscribe bus feeding the
// /api/v1/events server-sent-events stream.
package events

import "sync"

// Event is one message to the UI.
type Event struct {
	Type string // SSE event name, e.g. "task"
	Data any    // JSON-encoded as the SSE data line
}

// Bus fans events out to subscribers. Slow subscribers lose events rather
// than block publishers.
type Bus struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// Subscribe returns a channel of events and a function to unsubscribe.
func (b *Bus) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 32)
	b.mu.Lock()
	if b.subs == nil {
		b.subs = map[chan Event]struct{}{}
	}
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch, func() {
		b.mu.Lock()
		if _, ok := b.subs[ch]; ok {
			delete(b.subs, ch)
			close(ch)
		}
		b.mu.Unlock()
	}
}

// Publish sends e to every subscriber without blocking.
func (b *Bus) Publish(e Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs {
		select {
		case ch <- e:
		default:
		}
	}
}
