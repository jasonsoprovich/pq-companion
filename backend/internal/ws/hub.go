// Package ws implements the WebSocket hub for real-time event broadcasting.
package ws

import (
	"encoding/json"
	"log/slog"
	"sync"
)

// Event is the envelope sent to all connected clients.
type Event struct {
	Type string `json:"type"`
	Data any    `json:"data"`
}

// Hub maintains the set of active clients and broadcasts messages to them.
type Hub struct {
	mu         sync.RWMutex
	clients    map[*Client]struct{}
	broadcast  chan []byte
	register   chan *Client
	unregister chan *Client
}

// NewHub creates an initialised Hub. Call Run in a goroutine to start it.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]struct{}),
		broadcast:  make(chan []byte, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run processes register/unregister/broadcast events. Blocks until done.
// Call as: go hub.Run().
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c] = struct{}{}
			h.mu.Unlock()
			slog.Debug("ws client connected", "addr", c.remoteAddr)

		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c]; ok {
				delete(h.clients, c)
				close(c.send)
			}
			h.mu.Unlock()
			slog.Debug("ws client disconnected", "addr", c.remoteAddr)

		case msg := <-h.broadcast:
			h.mu.RLock()
			for c := range h.clients {
				select {
				case c.send <- msg:
				default:
					// Slow client — drop and remove.
					h.mu.RUnlock()
					h.mu.Lock()
					if _, ok := h.clients[c]; ok {
						delete(h.clients, c)
						close(c.send)
					}
					h.mu.Unlock()
					h.mu.RLock()
				}
			}
			h.mu.RUnlock()
		}
	}
}

// Broadcast encodes event as JSON and queues it for all connected clients.
func (h *Hub) Broadcast(event Event) {
	b, err := json.Marshal(event)
	if err != nil {
		slog.Error("ws marshal event", "err", err)
		return
	}
	select {
	case h.broadcast <- b:
	default:
		slog.Warn("ws broadcast channel full, dropping event", "type", event.Type)
	}
}

// ClientCount returns the number of currently connected clients.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
