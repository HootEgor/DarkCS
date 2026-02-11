package ws

import (
	"encoding/json"
	"log/slog"
	"sync"

	"DarkCS/entity"
)

// ClientMessageHandler handles incoming WebSocket messages from CRM clients.
type ClientMessageHandler interface {
	HandleMarkRead(username, platform, userID string) error
}

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
	handler    ClientMessageHandler
	log        *slog.Logger
}

// NewHub creates a new Hub instance.
func NewHub(log *slog.Logger) *Hub {
	return &Hub{
		clients:    make(map[*Client]bool),
		broadcast:  make(chan *Event, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		log:        log,
	}
}

// SetHandler sets the handler for incoming client messages.
func (h *Hub) SetHandler(handler ClientMessageHandler) {
	h.handler = handler
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

// BroadcastReadReceipt sends a read_receipt event to all connected CRM clients.
func (h *Hub) BroadcastReadReceipt(username, platform, userID string) {
	h.broadcast <- &Event{
		Type: "read_receipt",
		Data: map[string]string{
			"username": username,
			"platform": platform,
			"user_id":  userID,
		},
	}
}

// clientEvent represents an incoming WebSocket message from a CRM client.
type clientEvent struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}

// HandleClientMessage parses and dispatches an incoming message from a client.
func (h *Hub) HandleClientMessage(username string, raw []byte) {
	if h.handler == nil {
		return
	}

	var event clientEvent
	if err := json.Unmarshal(raw, &event); err != nil {
		if h.log != nil {
			h.log.Warn("failed to parse client ws message", slog.String("error", err.Error()))
		}
		return
	}

	switch event.Type {
	case "mark_read":
		var data struct {
			Platform string `json:"platform"`
			UserID   string `json:"user_id"`
		}
		if err := json.Unmarshal(event.Data, &data); err != nil {
			if h.log != nil {
				h.log.Warn("failed to parse mark_read data", slog.String("error", err.Error()))
			}
			return
		}
		if data.Platform == "" || data.UserID == "" {
			return
		}
		if err := h.handler.HandleMarkRead(username, data.Platform, data.UserID); err != nil {
			if h.log != nil {
				h.log.Error("failed to handle mark_read",
					slog.String("username", username),
					slog.String("platform", data.Platform),
					slog.String("user_id", data.UserID),
					slog.String("error", err.Error()),
				)
			}
		}
	}
}
