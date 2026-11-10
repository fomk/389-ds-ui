package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/fomk/389-ds-ui/internal/events"
)

// eventsHandler streams the event bus as server-sent events (text/event-stream).
func eventsHandler(bus *events.Bus) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok || bus == nil {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Accel-Buffering", "no")
		ch, unsubscribe := bus.Subscribe()
		defer unsubscribe()
		_, _ = fmt.Fprint(w, "retry: 3000\n\n")
		flusher.Flush()
		ping := time.NewTicker(15 * time.Second)
		defer ping.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case <-ping.C:
				_, _ = fmt.Fprint(w, "event: ping\ndata: {}\n\n")
			case e, ok := <-ch:
				if !ok {
					return
				}
				b, err := json.Marshal(e.Data)
				if err != nil {
					continue
				}
				_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, b)
			}
			flusher.Flush()
		}
	}
}
