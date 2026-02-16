# File Handling Implementation Plan

## Context

The entire DarkCS chat system is currently **text-only** — no file/media support anywhere. Users on Telegram, Instagram, and WhatsApp cannot send photos/documents to the CRM, and managers cannot send files back. This plan adds bidirectional file transfer with MongoDB GridFS storage and multi-file per message support.

## Decisions

- **Storage**: MongoDB GridFS (files stored as binaries alongside metadata in MongoDB)
- **File types**: All (images, documents, audio, video, archives)
- **Direction**: Both — users → managers AND managers → users
- **Multi-file**: Multiple attachments per single ChatMessage

---

## Phase 1: Entity & Storage Foundation

### 1.1 New file: `entity/attachment.go`
Define:
```go
type Attachment struct {
    FileID   primitive.ObjectID `json:"fileId" bson:"file_id"`
    Filename string             `json:"filename" bson:"filename"`
    MIMEType string             `json:"mimeType" bson:"mime_type"`
    Size     int64              `json:"size" bson:"size"`
    URL      string             `json:"url,omitempty" bson:"-"` // computed at read-time
}

type FileMetadata struct {
    MIMEType string `bson:"mime_type"`
    Platform string `bson:"platform"`
    UserID   string `bson:"user_id"`
    Uploader string `bson:"uploader"` // "user" | "manager"
}
```

### 1.2 Modify: `entity/chat-message.go`
Add field to `ChatMessage`:
```go
Attachments []Attachment `json:"attachments,omitempty" bson:"attachments,omitempty"`
```
`omitempty` ensures backward-compatibility — existing text-only messages unchanged.

### 1.3 New file: `internal/database/gridfs.go`
GridFS upload/download on `MongoDB` struct, following existing connect/disconnect pattern:

- `UploadFile(filename string, reader io.Reader, meta entity.FileMetadata) (primitive.ObjectID, int64, error)` — returns file ID and size
- `DownloadFile(fileID primitive.ObjectID) (filename string, meta entity.FileMetadata, reader io.ReadCloser, err error)` — wraps download stream + mongo client in a custom `io.ReadCloser` that disconnects on Close()

### 1.4 Modify: `impl/core/core.go` — `Repository` interface
Add:
```go
UploadFile(filename string, reader io.Reader, meta entity.FileMetadata) (primitive.ObjectID, int64, error)
DownloadFile(fileID primitive.ObjectID) (string, entity.FileMetadata, io.ReadCloser, error)
```

---

## Phase 2: File Download HTTP Endpoint

### 2.1 New file: `internal/http-server/handlers/crm/file.go`
- `DownloadFile()` handler: `GET /api/v1/crm/files/{file_id}`
- Parses `file_id` as ObjectID, calls Core, streams response with `Content-Type` and `Content-Disposition` headers

### 2.2 Modify: `internal/http-server/handlers/crm/handler.go` — `Core` interface
Add:
```go
DownloadFile(fileID primitive.ObjectID) (filename, mimeType string, reader io.ReadCloser, err error)
UploadFile(filename string, reader io.Reader, meta entity.FileMetadata) (primitive.ObjectID, int64, error)
SendCrmFiles(platform, userID, caption string, attachments []entity.Attachment) error
```

### 2.3 Modify: `impl/core/crm.go`
- `DownloadFile()` — delegates to `c.repo.DownloadFile()`
- `GetChatMessages()` — populate `attachment.URL = "/api/v1/crm/files/" + fileID.Hex()` for each attachment before returning

### 2.4 Modify: `internal/http-server/api/api.go`
Register inside the authenticated `/crm` route group:
```go
r.Get("/files/{file_id}", crm.DownloadFile(log, handler))
```

---

## Phase 3: Messenger Interface — SendFile

### 3.1 Modify: `bot/chat/messenger.go`
Add type and interface method:
```go
type FileMessage struct {
    Reader   io.Reader
    Filename string
    MIMEType string
    Caption  string
}

// Add to Messenger interface:
SendFile(chatID string, file FileMessage) error
```

Update `loggingMessenger`:
```go
func (m *loggingMessenger) SendFile(chatID string, file FileMessage) error {
    if err := m.inner.SendFile(chatID, file); err != nil {
        return err
    }
    text := file.Caption
    if text == "" {
        text = "[File: " + file.Filename + "]"
    }
    m.saveOutgoing(text)
    return nil
}
```

### 3.2 Modify: `bot/chat/listener.go` — `MessageListener` interface
Add:
```go
UploadAndSaveFile(platform, userID string, reader io.Reader, filename, mimeType string, size int64, caption string) error
```
This method uploads to GridFS → creates ChatMessage with Attachment → saves → broadcasts via WebSocket. Keeps bots decoupled from storage.

### 3.3 Implement in `impl/core/crm.go`
```go
func (c *Core) UploadAndSaveFile(platform, userID string, reader io.Reader, filename, mimeType string, size int64, caption string) error
```
1. Upload to GridFS via `c.repo.UploadFile()`
2. Build `ChatMessage{Attachments: [...], Text: caption, Direction: "incoming", Sender: "user"}`
3. Save message, broadcast via WebSocket

---

## Phase 4: Platform Implementations

### 4.1 Telegram — Send Files
**Modify: `bot/chat/telegram/messenger.go`**
- Extend `TelegramAPI` interface with `SendDocument(chatId int64, ...)`
- Implement `SendFile()` — uses `bot.SendDocument()` for universal file sending (works for all types)

### 4.2 Telegram — Receive Files
**Modify: `bot/userbot.go`**
- Add handlers for `message.Photo`, `message.Document`, `message.Audio`, `message.Video`, `message.Voice`
- New `handleMedia()` method:
  1. Extract file_id from message (photo uses largest size)
  2. Call `bot.GetFile(fileID)` to get file path
  3. Download from Telegram API
  4. Call `listener.UploadAndSaveFile()` with the file data
  5. Route to ChatEngine with caption text if present

### 4.3 Instagram — Send Files
**Modify: `bot/chat/instagram/messenger.go`**
- Extend `MessageSender` interface with `SendMediaMessage(recipientID, mediaURL, mediaType string) error`
- Implement `SendFile()` — uses Instagram Graph API attachment payload with a publicly-accessible URL
- The media URL points to the file download endpoint

**Modify: `bot/insta/instabot.go`**
- Implement `SendMediaMessage()` on `InstaBot`

### 4.4 Instagram — Receive Files
**Modify: `bot/insta/instabot.go`**
- Expand webhook message struct to include `Attachments` field
- In `processPayload()`, after text handling, check for attachments
- Download each attachment URL from Facebook CDN
- Call `listener.UploadAndSaveFile()`

### 4.5 WhatsApp — Send Files
**Modify: `bot/chat/whatsapp/messenger.go`**
- Extend `MessageSender` interface with `SendMediaMessage(recipientPhone, mediaType, mediaURL, caption, filename string) error`
- Implement `SendFile()` — uses WhatsApp Cloud API media message with `link` field

**Modify: `bot/whatsapp/whatsappbot.go`**
- Implement `SendMediaMessage()` on `WhatsAppBot`

### 4.6 WhatsApp — Receive Files
**Modify: `bot/whatsapp/whatsappbot.go`**
- Expand webhook message struct to include `Image`, `Document`, `Audio`, `Video`, `Sticker`, `Voice` fields
- In `processPayload()`, for media types:
  1. Call `GET https://graph.facebook.com/v21.0/{media_id}` to get download URL
  2. Download file with auth header
  3. Call `listener.UploadAndSaveFile()`

---

## Phase 5: CRM Manager File Upload & Send

### 5.1 New file: `internal/http-server/handlers/crm/send-file.go`
- `SendFile()` handler: `POST /api/v1/crm/chats/{platform}/{user_id}/send-file`
- `Content-Type: multipart/form-data`
- Fields: `files[]` (multiple files), `caption` (optional text)
- Handler:
  1. `r.ParseMultipartForm(32 << 20)` — 32MB limit
  2. For each file: upload to GridFS, collect `Attachment` structs
  3. Call `handler.SendCrmFiles(platform, userID, caption, attachments)`

### 5.2 Implement in `impl/core/crm.go`
```go
func (c *Core) SendCrmFiles(platform, userID, caption string, attachments []entity.Attachment) error
```
1. For each attachment: download from GridFS, send via `messenger.SendFile()`
2. Save single `ChatMessage{Attachments: all, Sender: "manager", Direction: "outgoing"}`
3. Broadcast via WebSocket

### 5.3 Register route in `api.go`
```go
r.Post("/chats/{platform}/{user_id}/send-file", crm.SendFile(log, handler))
```

---

## Phase 6: WebSocket Adjustments

### 6.1 Modify: `internal/ws/client.go`
- Increase `maxMessageSize` from `2048` to `16384` (16KB) — file metadata (not binaries) in multi-attachment messages may exceed 2KB
- Increase `WriteBufferSize` in upgrader from `1024` to `4096`

### 6.2 No other WebSocket changes needed
`BroadcastMessage()` already serializes `entity.ChatMessage` to JSON. The new `Attachments` field with `json:"attachments,omitempty"` will automatically be included. CRM frontend uses `attachment.url` to fetch files.

---

## Files Summary

| Action | File |
|--------|------|
| **New** | `entity/attachment.go` |
| **New** | `internal/database/gridfs.go` |
| **New** | `internal/http-server/handlers/crm/file.go` |
| **New** | `internal/http-server/handlers/crm/send-file.go` |
| Modify | `entity/chat-message.go` |
| Modify | `impl/core/core.go` (Repository interface) |
| Modify | `impl/core/crm.go` (UploadAndSaveFile, SendCrmFiles, DownloadFile, GetChatMessages) |
| Modify | `bot/chat/messenger.go` (FileMessage, SendFile, loggingMessenger) |
| Modify | `bot/chat/listener.go` (UploadAndSaveFile on MessageListener) |
| Modify | `bot/chat/telegram/messenger.go` (SendFile, TelegramAPI) |
| Modify | `bot/chat/instagram/messenger.go` (SendFile, MessageSender) |
| Modify | `bot/chat/whatsapp/messenger.go` (SendFile, MessageSender) |
| Modify | `bot/userbot.go` (media handlers) |
| Modify | `bot/insta/instabot.go` (receive attachments, SendMediaMessage) |
| Modify | `bot/whatsapp/whatsappbot.go` (receive media, SendMediaMessage) |
| Modify | `internal/http-server/handlers/crm/handler.go` (Core interface) |
| Modify | `internal/http-server/api/api.go` (new routes) |
| Modify | `internal/ws/client.go` (maxMessageSize) |

## Implementation Order

1. Entity types (attachment.go, chat-message.go update)
2. GridFS storage layer (gridfs.go)
3. Repository interface update (core.go)
4. MessageListener interface update (listener.go)
5. Core implementation (crm.go — UploadAndSaveFile, DownloadFile, SendCrmFiles, GetChatMessages)
6. Messenger interface (messenger.go — FileMessage, SendFile, loggingMessenger)
7. Platform send implementations (telegram, instagram, whatsapp messengers)
8. CRM HTTP endpoints (file.go, send-file.go, handler.go, api.go)
9. Platform receive implementations (userbot.go, instabot.go, whatsappbot.go)
10. WebSocket adjustments (client.go)

## Verification

- Send a photo from Telegram → check it appears in CRM dashboard with download link
- Send a document from WhatsApp → verify GridFS storage and WebSocket broadcast
- Upload a file from CRM dashboard → verify it arrives on the user's platform
- Send multiple files in one manager message → verify all arrive and are stored as one ChatMessage
- Check backward compatibility — existing text-only messages still work unchanged
