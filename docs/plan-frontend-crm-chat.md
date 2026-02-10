# Frontend Plan: CRM Chat Interface

## Overview

Web-based CRM chat interface where managers can view conversations with users across all platforms (Telegram, Instagram, WhatsApp) and reply in real-time.

---

## 1. API Contract (from backend)

### REST Endpoints

```
GET  /api/v1/crm/chats
     → [{ platform, user_id, user_name, last_message, last_time, unread }]

GET  /api/v1/crm/chats/{platform}/{user_id}/messages?limit=50&offset=0
     → [{ id, platform, user_id, direction, sender, text, created_at }]

POST /api/v1/crm/chats/{platform}/{user_id}/send
     Body: { "text": "message" }
     → { "ok": true }
```

### WebSocket

```
WS   /api/v1/crm/ws?token={api_key}

Events (server → client):
{
    "type": "new_message",
    "data": { id, platform, user_id, direction, sender, text, created_at }
}

{
    "type": "typing",
    "data": { platform, user_id }
}
```

---

## 2. Page Layout

```
┌─────────────────────────────────────────────────────────┐
│  DARK CRM                                    [Manager]  │
├──────────────────┬──────────────────────────────────────┤
│                  │  Telegram · @username                 │
│  Chat List       │──────────────────────────────────────│
│                  │                                      │
│  [TG] User 1    │  [12:30] User:                       │
│  Last msg...     │  Привіт, хочу дізнатись про...      │
│                  │                                      │
│  [IG] User 2    │  [12:31] Bot:                        │
│  Last msg...     │  Вітаю! Я консультант...            │
│                  │                                      │
│  [WA] User 3    │  [12:35] Manager:                    │
│  Last msg...     │  Добрий день! Ось деталі...         │
│                  │                                      │
│                  │──────────────────────────────────────│
│                  │  [Message input]            [Send]   │
├──────────────────┴──────────────────────────────────────┤
```

---

## 3. Components

### 3.1 ChatList (left panel)

**Data:** `GET /api/v1/crm/chats` — fetch on mount, refresh on WebSocket `new_message` event.

**Each item shows:**
- Platform icon (TG / IG / WA)
- User name
- Last message text (truncated)
- Timestamp
- Unread badge (count)

**Behavior:**
- Click → select chat → load messages in right panel
- Active chat highlighted
- On `new_message` WS event: move chat to top, update last message, increment unread (if not active chat)
- Sort by `last_time` descending

### 3.2 ChatWindow (right panel)

**Data:** `GET /api/v1/crm/chats/{platform}/{user_id}/messages?limit=50` — fetch when chat selected.

**Message display:**
- Group by date (separator: "Сьогодні", "Вчора", date)
- Incoming (user) messages: left-aligned, colored
- Outgoing (manager) messages: right-aligned, different color
- Bot messages: left-aligned, italic or dimmed
- Each message: sender label, text, timestamp

**Pagination:**
- Load last 50 on open
- Scroll to top → load next 50 (offset=50)
- Prepend older messages

**Real-time updates:**
- On `new_message` WS event for active chat → append message, scroll to bottom
- On `typing` WS event → show typing indicator

### 3.3 MessageInput (bottom bar)

- Text input (multiline with Shift+Enter for newline)
- Enter to send
- Send button
- On send:
  1. `POST /api/v1/crm/chats/{platform}/{user_id}/send`
  2. Optimistically append message to chat (don't wait for WS echo)
  3. Clear input

---

## 4. WebSocket Connection

### Connection Manager

```
On app mount:
  1. Connect to ws://{host}/api/v1/crm/ws?token={api_key}
  2. On open → mark connected
  3. On message → parse JSON event → dispatch to handlers
  4. On close → reconnect with exponential backoff (1s, 2s, 4s, max 30s)
  5. On error → same reconnect logic
```

### Event Handling

```
on "new_message":
  - If message.platform + message.user_id matches active chat:
      → append to message list, scroll down, mark as read
  - Always:
      → update chat list (move to top, update last msg)
      → if not active chat → increment unread counter
      → show browser notification (if tab not focused)

on "typing":
  - If matches active chat → show typing indicator for 3s
```

### Heartbeat

- Send ping every 25s to keep connection alive
- If no pong within 10s → reconnect

---

## 5. State Management

Minimal state needed:

```
{
    chats: ChatSummary[]        // chat list
    activeChat: {               // currently selected
        platform: string
        userId: string
    } | null
    messages: ChatMessage[]     // messages for active chat
    wsConnected: boolean
    loading: boolean
}
```

**On chat switch:**
1. Set `activeChat`
2. Clear `messages`
3. Fetch messages from API
4. Reset unread count for this chat

---

## 6. Browser Notifications

When a `new_message` event arrives and:
- The browser tab is not focused (`document.hidden === true`)
- The message is from a user (`direction === "incoming"`)

Then:
- Request notification permission (once, on app load)
- Show browser notification: `"New message from {user_name} ({platform})"`
- On notification click → focus tab, switch to that chat

---

## 7. Platform Indicators

| Platform | Icon | Color |
|----------|------|-------|
| Telegram | Paper plane / TG logo | `#0088cc` |
| Instagram | IG logo | `#E4405F` |
| WhatsApp | WA logo | `#25D366` |

Display next to user name in chat list and in chat header.

---

## 8. Error Handling

| Scenario | Behavior |
|----------|----------|
| WS disconnect | Show "Reconnecting..." banner at top, auto-retry |
| Send fails | Show error toast, keep message in input |
| Messages fetch fails | Show "Failed to load" with retry button |
| Chat list fetch fails | Show error state with retry |

---

## 9. API Integration Summary

```
App mount:
  → GET /api/v1/crm/chats           (load chat list)
  → WS  /api/v1/crm/ws?token=...    (open WebSocket)

Select chat:
  → GET /api/v1/crm/chats/{p}/{uid}/messages?limit=50

Scroll up:
  → GET /api/v1/crm/chats/{p}/{uid}/messages?limit=50&offset=N

Send message:
  → POST /api/v1/crm/chats/{p}/{uid}/send  { "text": "..." }

WebSocket events update chat list and message view in real-time.
```

---

## 10. Implementation Order

1. **WebSocket connection** — connect, reconnect, event parsing
2. **Chat list** — fetch + display, sort by last message time
3. **Chat window** — fetch messages on select, display with direction styling
4. **Send message** — input + POST + optimistic append
5. **Real-time updates** — WS events update chat list and active chat
6. **Pagination** — scroll-to-top loading
7. **Browser notifications** — for incoming messages when tab unfocused
8. **Polish** — typing indicator, platform icons, error states
