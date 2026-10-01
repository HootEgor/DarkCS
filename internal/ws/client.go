package ws

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = 30 * time.Second
	maxMessageSize = 16384
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 4096,
	CheckOrigin:     checkOrigin,
}

// allowedOrigins is set once at startup by SetAllowedOrigins; empty allows any origin.
var allowedOrigins []string

// SetAllowedOrigins restricts which browser origins may open the CRM socket. Call before
// the server starts. Requests without an Origin header (non-browser clients) are allowed.
func SetAllowedOrigins(origins []string) {
	allowedOrigins = nil
	for _, o := range origins {
		if o = strings.TrimRight(strings.TrimSpace(o), "/"); o != "" {
			allowedOrigins = append(allowedOrigins, o)
		}
	}
}

func checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if len(allowedOrigins) == 0 || origin == "" {
		return true
	}
	for _, o := range allowedOrigins {
		if strings.EqualFold(o, origin) {
			return true
		}
	}
	return false
}

// Client represents a single WebSocket connection from a CRM manager.
type Client struct {
	hub      *Hub
	conn     *websocket.Conn
	send     chan []byte
	username string
}

// readPump pumps messages from the WebSocket connection to the hub.
// It handles ping/pong keepalive and detects disconnects.
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, msg, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
		c.hub.HandleClientMessage(c.username, msg)
	}
}

// writePump pumps messages from the hub to the WebSocket connection.
func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}

			w, err := c.conn.NextWriter(websocket.TextMessage)
			if err != nil {
				return
			}
			w.Write(message)

			if err := w.Close(); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}

// Authenticator resolves socket credentials to a CRM username.
type Authenticator interface {
	ValidateToken(token string) (string, error)
	RedeemWsTicket(ticket string) (string, error)
}

// ServeWs handles WebSocket upgrade requests for CRM clients.
func ServeWs(hub *Hub, auth Authenticator, log *slog.Logger, w http.ResponseWriter, r *http.Request) {
	// Auth: prefer a one-time ticket (POST /api/v1/crm/ws-ticket); the API key in
	// ?token= still works for existing clients but ends up in proxy and access logs.
	var username string
	var err error
	if ticket := r.URL.Query().Get("ticket"); ticket != "" {
		username, err = auth.RedeemWsTicket(ticket)
	} else if token := r.URL.Query().Get("token"); token != "" {
		username, err = auth.ValidateToken(token)
	} else {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if err != nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Error("websocket upgrade failed", slog.String("error", err.Error()))
		return
	}

	client := &Client{
		hub:      hub,
		conn:     conn,
		send:     make(chan []byte, 256),
		username: username,
	}

	hub.register <- client

	go client.writePump()
	go client.readPump()
}
