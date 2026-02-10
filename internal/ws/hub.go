package ws

import (
	"encoding/json"
	"sync"

	"DarkCS/entity"
)

// Event represents a WebSocket event sent to CRM clients.
type Event struct {
	Type string      `json:"type"` // "new_message", "typing"
	Data interface{} `json:"data"`
}

// Hub maintains the set of active WebSocket clients and broadcasts events.
type Hub struct {
	clients    map[*Client]bool
	broadcast  chan *Event
	register   chan *Client
	unregister chan *Client
	mu         sync.RWMutex
}

// NewHub creates a new Hub instance.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan *Event, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run starts the hub's event loop. Should be called in a goroutine.
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			h.clients[client] = true
			h.mu.Unlock()

		case client := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[client]; ok {
				delete(h.clients, client)
				close(client.send)
			}
			h.mu.Unlock()

		case event := <-h.broadcast:
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			h.mu.RLock()
			for client := range h.clients {
				select {
				case client.send <- data:
				default:
					close(client.send)
					delete(h.clients, client)
				}
			}
			h.mu.RUnlock()
		}
	}
}

// BroadcastMessage sends a new_message event to all connected CRM clients.
func (h *Hub) BroadcastMessage(msg entity.ChatMessage) {
	h.broadcast <- &Event{
		Type: "new_message",
		Data: msg,
	}
}

// BroadcastTyping sends a typing event to all connected CRM clients.
func (h *Hub) BroadcastTyping(platform, userID string) {
	h.broadcast <- &Event{
		Type: "typing",
		Data: map[string]string{
			"platform": platform,
			"user_id":  userID,
		},
	}
}
