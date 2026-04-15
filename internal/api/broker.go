package api

import (
	"fmt"
	"net/http"
	"sync"
)

// Broker manages Server-Sent Event connections and broadcasts refresh
// notifications to all connected clients whenever the data changes.
type Broker struct {
	mu      sync.Mutex
	clients map[chan struct{}]struct{}
}

func NewBroker() *Broker {
	return &Broker{clients: make(map[chan struct{}]struct{})}
}

// Notify sends a refresh signal to every connected SSE client.
func (b *Broker) Notify() {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.clients {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func (b *Broker) add(ch chan struct{}) {
	b.mu.Lock()
	b.clients[ch] = struct{}{}
	b.mu.Unlock()
}

func (b *Broker) remove(ch chan struct{}) {
	b.mu.Lock()
	delete(b.clients, ch)
	b.mu.Unlock()
}

// Subscribe / Unsubscribe allow callers (e.g. tests) to register a channel
// directly without going through ServeHTTP.
func (b *Broker) Subscribe(ch chan struct{})   { b.add(ch) }
func (b *Broker) Unsubscribe(ch chan struct{}) { b.remove(ch) }

// sseEvent formats a proper SSE data line.
// The "data" field prefix is what causes EventSource.onmessage to fire.
func sseEvent(payload string) string {
	return fmt.Sprintf("%s: %s\n\n", "data", payload)
}

// ServeHTTP implements GET /api/events (SSE).
func (b *Broker) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	fl, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	// SSE comment line — signals the stream is open but does not trigger onmessage.
	fmt.Fprintf(w, ": connected\n\n")
	fl.Flush()

	ch := make(chan struct{}, 1)
	b.add(ch)
	defer b.remove(ch)

	for {
		select {
		case <-ch:
			fmt.Fprint(w, sseEvent("refresh"))
			fl.Flush()
		case <-r.Context().Done():
			return
		}
	}
}
