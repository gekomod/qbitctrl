package websocket

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Simple WebSocket implementation using Server-Sent Events (SSE)
// SSE works in all browsers without any JS library — no WS handshake needed.

type Event struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

type Hub struct {
	mu      sync.RWMutex
	clients map[chan []byte]struct{}
}

func NewHub() *Hub {
	return &Hub{clients: make(map[chan []byte]struct{})}
}

func (h *Hub) register(ch chan []byte) {
	h.mu.Lock()
	h.clients[ch] = struct{}{}
	h.mu.Unlock()
}

func (h *Hub) unregister(ch chan []byte) {
	h.mu.Lock()
	delete(h.clients, ch)
	h.mu.Unlock()
	close(ch)
}

// Broadcast sends an event to all connected SSE clients
func (h *Hub) Broadcast(eventType string, data interface{}) {
	ev := Event{Type: eventType, Data: data}
	b, err := json.Marshal(ev)
	if err != nil {
		return
	}

	h.mu.RLock()
	for ch := range h.clients {
		select {
		case ch <- b:
		default:
			// Client too slow — skip this message
		}
	}
	h.mu.RUnlock()
}

// ClientCount returns number of connected SSE clients
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}

// ServeSSE handles Server-Sent Events connections
// JS: const es = new EventSource('/api/events');
//
//	es.onmessage = e => { const ev = JSON.parse(e.data); ... }
func (h *Hub) ServeSSE(w http.ResponseWriter, r *http.Request) {
	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	ch := make(chan []byte, 32)
	h.register(ch)
	defer h.unregister(ch)

	log.Printf("[SSE] Client connected (%d total)", h.ClientCount())

	// Send initial ping
	fmt.Fprintf(w, "data: {\"type\":\"connected\"}\n\n")
	flusher.Flush()

	// Keepalive ticker (every 15s to prevent proxy timeouts)
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()

		case <-ticker.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()

		case <-r.Context().Done():
			log.Printf("[SSE] Client disconnected (%d remaining)", h.ClientCount()-1)
			return
		}
	}
}
