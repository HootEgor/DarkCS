# Messenger File Handling Plan

This document describes a concrete, repo-aware plan to implement file handling for messengers (Telegram, Instagram, WhatsApp) in the DarkCS project: to retrieve files from messengers, save them in conversation history, and deliver them through the websocket hub; and the reverse path — receive files from websocket or HTTP clients and send them to messenger platforms.

Status: saved as `docs/messenger-file-handling-plan.md`.

---

## High-level overview

Goals:
- When a messenger sends media, download it, store the blob + metadata, create a chat message referencing it, and notify connected websocket clients.
- When a web/websocket client uploads a file, store it, create a chat message, and allow sending that file to messenger platforms.
- Support both small inline websocket streaming (binary frames) and robust HTTP multipart upload + signed URLs for larger files.

Recommended defaults:
- Storage: Filesystem backend for initial implementation (easy local dev), with an S3 adapter for production later.
- Chunking: HTTP multipart + signed URL for files > 1 MB; allow websocket binary chunking for files <= 1 MB.
- Malware scanning: optional and asynchronous (disabled by default in config).

---

## Checklist (what I'll save here)
- Architecture and data flows (incoming/outgoing)
- Concrete files to add/modify (paths & brief description)
- DB metadata schema and indexes
- Storage options and recommendation
- Websocket message contract (JSON & binary patterns)
- HTTP endpoints and handler behaviors
- Storage API function signatures and expected errors
- Messenger integration pseudocode for Telegram/Instagram/WhatsApp
- Websocket hub/client changes and backpressure handling
- Security recommendations
- Testing plan and edge cases
- Incremental milestones for implementation

---

## 1) Architecture and data flows

A. Messenger -> Server -> Websocket clients (incoming flow)
- Bot receives update with a messenger file reference.
- Bot adapter (e.g., `bot/telegram`) downloads the file stream from the messenger API.
- Server calls storage API: `SaveFile(ctx, io.Reader, FileMeta)`.
- Storage writes blob to backend and creates a `files` metadata record in DB.
- Create a `ChatMessage` referencing `file_id` and persist it.
- Hub broadcasts a `file_meta` event to chat clients; clients fetch via signed URL or receive inline chunks.

B. Websocket client -> Server -> Messenger (outgoing flow)
- Client uploads via `POST /api/v1/files` (recommended) or via websocket small-file chunks.
- Server stores the file and returns `file_id`.
- Client sends websocket `file_action` to request sending the stored file to a messenger conversation.
- Server enqueues a background worker to send the file to the platform using `storage.Open()` stream.
- Update message status and broadcast updates to chat.

---

## 2) Files to add or modify

New files to add (primary):
- `entity/file.go` — File metadata (`FileRecord`) and attachment reference (`FileAttachment`).
- `internal/storage/backend.go` — `StorageBackend` interface definitions.
- `internal/storage/fs_backend.go` — Filesystem backend implementation.
- `internal/storage/s3_backend.go` — Optional S3 adapter (presigned URL support).
- `internal/database/file_store.go` — DB-level file metadata layer, Save/Get/Delete wrappers.
- `internal/http-server/handlers/file/upload.go` — `POST /api/v1/files` handler.
- `internal/http-server/handlers/file/download.go` — `GET /api/v1/files/{id}` with Range support.
- `internal/http-server/handlers/file/signedurl.go` — return pre-signed download URLs.
- `internal/ws/file_event.go` — websocket event types for `file_meta`, `file_chunk`, `file_upload_progress`.
- `internal/worker/send_to_messenger.go` — background worker for sending stored files to platforms.

Files to modify (wire-up):
- `entity/chat-message.go` — add `Attachment` or `FileID` reference.
- `internal/database/chat_message.go` — persist/restore attachment reference.
- `internal/ws/hub.go` — add `BroadcastFileMeta`, file transfer tracking structures.
- websocket client file (e.g., `ws/client.go`) — handle new events and optional chunking.
- messenger adapters: `bot/telegram/*`, `bot/instagram/*`, `whatsapp/*` — add file download and send helpers.
- `internal/config/config.go` — add storage config and file-related settings.
- `main.go` — init storage backend and register handlers + worker.

Tests to add:
- `internal/database/file_store_test.go`
- `internal/http-server/handlers/file/upload_test.go`
- `internal/ws/hub_file_test.go`
- `bot/telegram/messenger_file_test.go` (httptest simulating messenger file server)

---

## 3) Database schema (files collection)

Collection: `files`
Fields (recommended):
- `id` (ObjectID/UUID) — primary
- `owner_id` (user who uploaded)
- `chat_id` (conversation id)
- `platform` (telegram|instagram|whatsapp|web)
- `platform_ref` (messenger file id)
- `storage_type` (fs|s3|gridfs)
- `storage_path` (path or object key)
- `filename` (original name)
- `content_type` (mime)
- `size_bytes` (int64)
- `image_meta` {width,height,thumb_ref}
- `status` (stored|processing|available|error)
- `checksum` (optional sha256)
- `created_at`, `updated_at`

Indexes:
- `chat_id`, `owner_id`
- `platform + platform_ref`
- optional text index on `filename`

---

## 4) Storage options

- Filesystem (default dev): simple base path, layout like `/data/files/YYYY/MM/<id>.<ext>`.
- S3 (recommended for production): presigned URLs, lifecycle policies.
- GridFS (Mongo): single DB storage if preferred.

Recommendation: Start with filesystem backend; build S3 adapter after core functionality is stable.

---

## 5) Websocket message contract

Top-level envelope: `{ "type": "<event_type>", "data": {...} }`

Key event types
- `file_meta` (server->client): metadata + `url` (signed) or `inline` flag.
  - `id, chat_id, sender_id, filename, content_type, size, thumb_url, url, storage_type, inline, created_at`

- `file_chunk` (bi-directional): used with a preceding JSON header, then binary frames. Header contains `{file_temp_id, seq, offset, total_seqs}`.

- `file_upload_progress` (server->client): `{client_temp_id, uploaded_bytes, percent, status}`

- `file_action` (client->server): request to send file to messenger: `{action:"send_to_platform", file_id, platform, target}`

Chunking strategy
- Inline websocket chunking for files <= 1 MB (configurable). Use binary frames with pre-header JSON.
- For files > 1 MB, require HTTP multipart upload to `POST /api/v1/files` (fast, robust), server returns `file_id` and possibly signed URL.

Progress updates
- Emit periodic `file_upload_progress` events (e.g., every 5% or every N bytes) and final status.

---

## 6) HTTP endpoints

Mount point: `/api/v1/files`

- POST `/api/v1/files`
  - Auth required, multipart/form-data `file`
  - Params: `chat_id` (optional), `platform` (optional)
  - Enforce `MaxFileSize` (config)
  - Response: `{file_id, metadata, signed_url?}`

- GET `/api/v1/files/{id}`
  - Auth + authorization checks; supports Range
  - Streams file or redirects to presigned URL

- GET `/api/v1/files/{id}/signed-url`
  - Returns `{url, expires_at}`

- DELETE `/api/v1/files/{id}`
  - Owner/admin only

Handler behavior
- Use `authenticate` middleware from `internal/http-server/middleware`.
- Size checks with `io.LimitedReader`.
- Content sniffing of first 512 bytes using `http.DetectContentType`.

---

## 7) Storage API (signatures)

Proposed types:

- `type FileMeta struct { Filename, ContentType string; Size int64; OwnerID, ChatID, Platform, PlatformRef string; Extra map[string]string }`
- `type FileRecord struct { ID, StorageType, StoragePath, Filename, ContentType string; Size int64; CreatedAt time.Time; Checksum, Status string }`

Errors:
- `ErrNotFound`, `ErrTooLarge`, `ErrInvalidType`, `ErrStoreFailure`

StorageBackend interface (example):
- `Save(ctx context.Context, r io.Reader, meta FileMeta) (FileRecord, error)`
- `Open(ctx context.Context, id string) (io.ReadCloser, FileRecord, error)`
- `Stat(ctx context.Context, id string) (FileRecord, error)`
- `Delete(ctx context.Context, id string) error`
- `SignedURL(ctx context.Context, id string, ttl time.Duration) (string, time.Time, error)` // optional

Behavior notes:
- Save enforces max size and writes to a temp location first, computes checksum optionally, and writes DB metadata atomically.
- Open supports streaming for Range requests and for sending to messenger APIs.

---

## 8) Messenger integrations (pseudocode)

Telegram incoming pseudocode (in `bot/telegram`):
- fileID := update.Message.Document.FileID
- filePath := telegramGetFilePath(fileID)
- resp := httpClient.Get("https://api.telegram.org/file/bot<token>/" + filePath)
- fileRec, err := storage.Save(ctx, resp.Body, FileMeta{Platform:"telegram", PlatformRef:fileID, ChatID: chatID, Filename: name})
- create ChatMessage referencing fileRec.ID and broadcast via hub

Telegram outgoing:
- r, fileRec := storage.Open(ctx, fileID)
- send multipart request `sendDocument` streaming file from `r`

WhatsApp/Instagram: follow similar pattern: fetch media by platform API and Save; to send, upload per platform API.

---

## 9) Websocket hub & client changes

Hub changes:
- Add `BroadcastFileMeta(chatID string, fileRec entity.FileRecord)`
- Track in-progress transfers: `map[string]*FileTransfer`
- Add channels for file chunk/ack events

Client changes:
- `readPump` handles `file_upload_start`, `file_chunk` headers and reads binary frames to pass to storage.Save (for small files)
- `writePump` sends JSON events and binary frames; supports backpressure detection
- Require ack-based flow for chunked transfers to avoid buffer growth

Backpressure policy:
- If client `send` channel is full, drop non-critical events or mark client slow; prefer presigned URL downloads for large files

---

## 10) Security recommendations

- Enforce max upload size at entrypoints
- Content-type sniffing and filename sanitization
- Authenticate and authorize per-chat access
- Signed URLs for downloads (short TTL)
- Optional asynchronous malware scanning (ClamAV) and quarantine for infected files
- Audit logs for uploads/downloads

---

## 11) Testing plan

Unit tests:
- Storage backend roundtrips (Save/Open/Delete)
- Handler tests for upload/download and Range
- Hub tests to ensure `file_meta` broadcast

Integration tests:
- Simulate messenger file server with `httptest.Server` to test adapter download → storage → chat message → hub broadcast

Smoke test (manual):
1. Start server with FS backend
2. Simulate telegram webhook delivering a small image (httptest) and confirm storage and broadcast
3. Upload file via `POST /api/v1/files`, request `file_action` to send to messenger (mock), confirm worker uses stored file

---

## 12) Edge cases

- Partial/interrupted uploads: use temp staging and TTL to cleanup; support resumable uploads if required later
- Very large files: reject > max or route to S3/GridFS; stream from messenger directly to backing storage
- Duplicate files: compute checksum and optionally dedupe
- MIME mismatch: sniff content and fix content_type; optionally reject mismatched types
- Slow/disconnected WS clients: favor signed-URL/HTTP transfers for large payloads

---

## Implementation milestones (incremental PR plan)

1. Add types & interfaces: `entity/file.go`, `internal/storage/backend.go`, config entries
2. Implement FS backend + `internal/database/file_store.go` with unit tests
3. Implement HTTP handlers: upload/download/signed-url, router wiring, auth checks
4. Add message attachment reference and `ws.BroadcastFileMeta`; have clients receive `file_meta`
5. Implement `bot/telegram` inbound flow and tests (httptest)
6. Implement background worker for outgoing messenger sends and `file_action` handler
7. Optional: S3 backend, presigned URL optimizations, malware scanning, resumable websocket chunking

---

## Decisions requested / recommended

Please confirm preferred options (or I’ll proceed with these defaults):
- Store: `filesystem` (default) with `s3` adapter added later
- Chunking strategy: `HTTP-first` (multipart + signed URL for >1 MB), websocket binary chunking only for <=1 MB
- Malware scanning: `optional async` (disabled by default)

---

## Next steps I can implement for you

Pick one and I will implement it in code and tests (I will create files & run local checks as required by repo conventions):
- A: Implement milestone #1 & #2 (models + FS backend + DB wrapper + tests)
- B: Implement milestone #3 (HTTP endpoints and handlers)
- C: Implement milestone #5 (Telegram inbound adapter connecting to storage) + tests
- D: Create skeletons only (types and router wiring) for review


---

End of plan file.

