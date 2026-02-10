# Backend Plan: CRM Chat with WebSocket

## Overview

Add CRM chat functionality: store user messages, provide REST API for chat history, WebSocket for real-time notifications, and allow managers to send messages to users on any platform.

---

## 1. Message Entity

### New file: `entity/chat-message.go`

```go
type ChatMessage struct {
    ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
    Platform  string             `json:"platform" bson:"platform"`
    UserID    string             `json:"user_id" bson:"user_id"`
    ChatID    string             `json:"chat_id" bson:"chat_id"`
    Direction string             `json:"direction" bson:"direction"`   // "incoming" | "outgoing"
    Sender    string             `json:"sender" bson:"sender"`         // "user" | "manager" | "bot"
    Text      string             `json:"text" bson:"text"`
    CreatedAt time.Time          `json:"created_at" bson:"created_at"`
}

type ChatSummary struct {
    Platform    string    `json:"platform" bson:"platform"`
    UserID      string    `json:"user_id" bson:"user_id"`
    UserName    string    `json:"user_name" bson:"user_name"`
    LastMessage string    `json:"last_message" bson:"last_message"`
    LastTime    time.Time `json:"last_time" bson:"last_time"`
    Unread      int       `json:"unread" bson:"unread"`
}
```

---

## 2. MongoDB Storage

### New collection constant in `internal/database/mongo.go`

```go
chatMessagesCollection = "chat-messages"
```

### New file: `internal/database/chat-message.go`

**Methods:**

```go
// SaveChatMessage inserts a message and trims to 100 per user.
func (m *MongoDB) SaveChatMessage(msg entity.ChatMessage) error

// GetChatMessages returns messages for a user, paginated (newest first).
func (m *MongoDB) GetChatMessages(platform, userID string, limit, offset int) ([]entity.ChatMessage, error)

// GetActiveChats returns chat summaries with last message info.
func (m *MongoDB) GetActiveChats() ([]entity.ChatSummary, error)

// CleanupChatMessages deletes messages older than 30 days, keeping at least 20 per user.
func (m *MongoDB) CleanupChatMessages() error
```

### SaveChatMessage logic

```
1. Insert message
2. Count messages for this (platform, user_id)
3. If count > 100:
   - Find the 100th newest message's created_at
   - Delete all older messages for this user
```

### CleanupChatMessages logic (daily job)

```
1. Aggregate: group by (platform, user_id), count messages
2. For each user with count > 20:
   - Find messages older than 30 days
   - Calculate how many can be deleted (count - 20 = max deletable)
   - Delete oldest messages up to that limit
```

### Indexes

```go
// Compound index for efficient queries
{platform: 1, user_id: 1, created_at: -1}
```

---

## 3. WebSocket Hub

### New file: `internal/ws/hub.go`

Standard hub pattern with gorilla/websocket.

```go
type Hub struct {
    clients    map[*Client]bool
    broadcast  chan *Event
    register   chan *Client
    unregister chan *Client
    mu         sync.RWMutex
}

type Client struct {
    hub      *Hub
    conn     *websocket.Conn
    send     chan []byte
    username string
}

type Event struct {
    Type string      `json:"type"`   // "new_message", "typing"
    Data interface{} `json:"data"`
}
```

**Hub methods:**
- `NewHub() *Hub`
- `Run()` — goroutine, listens on register/unregister/broadcast channels
- `BroadcastMessage(msg entity.ChatMessage)` — serialize event, send to all clients
- `BroadcastTyping(platform, userID string)` — notify managers user is typing

**Client lifecycle:**
1. HTTP upgrade → register client in hub
2. `readPump()` — read from WebSocket (ping/pong keepalive, detect disconnect)
3. `writePump()` — write from send channel to WebSocket
4. On disconnect → unregister from hub

### New file: `internal/ws/client.go`

Read/write pump implementations with ping/pong heartbeat (30s interval).

---

## 4. REST + WebSocket Endpoints

### New handler: `internal/http-server/handlers/crm/`

#### `internal/http-server/handlers/crm/handler.go`

```go
type Core interface {
    GetActiveChats() ([]entity.ChatSummary, error)
    GetChatMessages(platform, userID string, limit, offset int) ([]entity.ChatMessage, error)
    SendCrmMessage(platform, userID, text string) error
}
```

#### Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/crm/chats` | List active chats with last message |
| GET | `/api/v1/crm/chats/{platform}/{user_id}/messages?limit=50&offset=0` | Get message history |
| POST | `/api/v1/crm/chats/{platform}/{user_id}/send` | Manager sends message to user |
| GET | `/api/v1/crm/ws` | WebSocket upgrade (authenticated) |

#### WebSocket endpoint handler

```go
func WsHandler(hub *ws.Hub, auth authenticate.Authenticate) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // 1. Auth: read token from query param ?token=xxx
        // 2. Validate token via auth.AuthenticateByToken()
        // 3. Upgrade to WebSocket
        // 4. Create Client, register in hub
    }
}
```

#### Register in router (`internal/http-server/api/api.go`)

```go
// Inside authenticated /api/v1 group:
r.Route("/crm", func(r chi.Router) {
    r.Get("/chats", crmHandler.GetChats)
    r.Get("/chats/{platform}/{user_id}/messages", crmHandler.GetMessages)
    r.Post("/chats/{platform}/{user_id}/send", crmHandler.SendMessage)
})

// Outside auth middleware (WebSocket handles its own auth):
r.Get("/api/v1/crm/ws", ws.WsHandler(hub, handler))
```

---

## 5. Core Integration

### Extend `impl/core/core.go`

Add to Core struct:

```go
type Core struct {
    // ... existing fields ...
    wsHub      *ws.Hub
    messengers map[string]chat.Messenger  // platform → messenger
}
```

New setter:

```go
func (c *Core) SetWsHub(hub *ws.Hub)
func (c *Core) SetPlatformMessenger(platform string, m chat.Messenger)
```

New methods in `impl/core/crm.go`:

```go
// GetActiveChats returns chat list from MongoDB.
func (c *Core) GetActiveChats() ([]entity.ChatSummary, error)

// GetChatMessages returns paginated history from MongoDB.
func (c *Core) GetChatMessages(platform, userID string, limit, offset int) ([]entity.ChatMessage, error)

// SendCrmMessage sends a message from manager to user via their platform.
func (c *Core) SendCrmMessage(platform, userID, text string) error {
    // 1. Look up messenger for platform
    // 2. Get chatID (for Telegram: chatID == userID; for WhatsApp: phone; for Instagram: userID)
    // 3. messenger.SendText(chatID, text)
    // 4. Store as outgoing message with sender="manager"
    // 5. Broadcast to WebSocket (so other managers see it)
}
```

---

## 6. Capturing Incoming Messages

### Hook in platform bot handlers

The message capture happens BEFORE ChatEngine processes the message, in each platform's webhook handler.

**Telegram** (`bot/userbot.go` — NO, can't modify `tgbot.go`, but `userbot.go` is fine):

In the message handler, before calling `chatEngine.HandleMessage()`:

```go
// Save incoming message
msg := entity.ChatMessage{
    Platform:  "telegram",
    UserID:    strconv.FormatInt(update.Message.From.ID, 10),
    ChatID:    strconv.FormatInt(update.Message.Chat.ID, 10),
    Direction: "incoming",
    Sender:    "user",
    Text:      update.Message.Text,
    CreatedAt: time.Now(),
}
core.SaveChatMessage(msg)
core.BroadcastToWs(msg)
```

**Instagram** (`internal/http-server/handlers/webhook/instagram.go`):

Same pattern in the webhook handler, before `chatEngine.HandleMessage()`.

**WhatsApp** (`internal/http-server/handlers/webhook/whatsapp.go`):

Same pattern in the webhook handler, before `chatEngine.HandleMessage()`.

---

## 7. Cleanup Scheduler

### Extend `impl/core/init.go`

Add daily cleanup alongside existing 21:00 scheduler:

```go
// In the existing daily ticker or add a new one
go func() {
    for {
        // Run at 03:00 daily
        next := nextTime(3, 0)
        time.Sleep(time.Until(next))
        if err := c.repo.CleanupChatMessages(); err != nil {
            c.log.Error("chat message cleanup failed", slog.String("error", err.Error()))
        }
    }
}()
```

---

## 8. Wiring in `main.go`

```go
// After existing service setup:

// 1. Create WebSocket hub
wsHub := ws.NewHub()
go wsHub.Run()

// 2. Wire to Core
handler.SetWsHub(wsHub)

// 3. Register platform messengers on Core (after bots created)
handler.SetPlatformMessenger("telegram", telegramMessenger)
handler.SetPlatformMessenger("instagram", instagramMessenger)
handler.SetPlatformMessenger("whatsapp", whatsappMessenger)

// 4. Pass hub to API router
srv := api.New(lg, handler, wsHub)
```

---

## 9. Dependencies

```
go get github.com/gorilla/websocket
```

No other new dependencies needed.

---

## 10. File Summary

| Action | File |
|--------|------|
| **New** | `entity/chat-message.go` |
| **New** | `internal/database/chat-message.go` |
| **New** | `internal/ws/hub.go` |
| **New** | `internal/ws/client.go` |
| **New** | `internal/http-server/handlers/crm/handler.go` |
| **New** | `impl/core/crm.go` |
| **Modify** | `internal/database/mongo.go` — add collection constant |
| **Modify** | `internal/http-server/api/api.go` — add CRM routes |
| **Modify** | `impl/core/core.go` — add wsHub, messengers fields + setters |
| **Modify** | `impl/core/init.go` — add cleanup scheduler |
| **Modify** | `main.go` — wire hub, messengers |
| **Modify** | `bot/userbot.go` — save incoming Telegram messages |
| **Modify** | Instagram webhook handler — save incoming messages |
| **Modify** | WhatsApp webhook handler — save incoming messages |

---

## 11. Implementation Order

1. Entity + MongoDB storage (messages CRUD + cleanup)
2. WebSocket hub + client
3. REST endpoints (chats list, history, send)
4. Core integration (crm.go methods)
5. Capture incoming messages in platform handlers
6. Wire everything in main.go
7. Cleanup scheduler
