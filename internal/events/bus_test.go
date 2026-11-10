package events

import "testing"

func TestBus(t *testing.T) {
	var b Bus
	ch, unsub := b.Subscribe()
	b.Publish(Event{Type: "task", Data: 1})
	if e := <-ch; e.Type != "task" {
		t.Errorf("event = %+v", e)
	}
	// A full subscriber does not block publishers.
	for i := 0; i < 100; i++ {
		b.Publish(Event{Type: "x"})
	}
	unsub()
	unsub() // idempotent
	b.Publish(Event{Type: "after"})
}
