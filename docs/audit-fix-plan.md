# Audit Fix Plan

Implementation plan for every finding from the October 2026 project audit. Work is split into
phases ordered by risk: each phase is independently shippable and leaves the system working.
Task IDs (e.g. `P1.3`) are meant to be used in commit messages.

**Constraints**
- `bot/tgbot.go` must not be modified (CLAUDE.md). Findings in that file are handled from the
  outside or listed under [Decisions needed](#decisions-needed).
- The project is built and run by the owner only — every task ends with a "verify" note describing
  what to check manually after building.

---

## Decisions needed

These block specific tasks; everything else can start immediately.

| # | Question | Blocks | Default if no answer |
|---|---|---|---|
| D1 | Allow a minimal change to `bot/tgbot.go` (mutex around `adminLevels`)? | P1.6 | Work around via snapshot in logger (partial fix) |
| D2 | Which API keys exist today and who uses them (website, SmartSender, CRM frontend, OpenAI/MCP)? | P2.1 | Treat all existing keys as `integration` scope, issue new `admin` key |
| D3 | Is the `/user/phone` OTP consumed by a trusted backend or by a browser? | P2.5 | Keep returning code but behind `admin` scope |
| D4 | For Instagram/WhatsApp onboarding: is linking to an existing account by typed phone an accepted business risk, or require manager confirmation? | P2.6 | Create a separate user instead of auto-linking |
| D5 | Is serving CRM files from the API origin required, or can they be `attachment`-only? | P2.9 | `inline` only for image/video/audio |

---

## Phase 1 — Data loss and crashes (ship ASAP)

### P1.1 Phone normalization on every lookup path
**Problem:** raw phone (no `+`) misses the DB lookup → auto-register → upsert overwrites the
existing user with a blank guest. Triggered by `/user/*` endpoints and by the CRM chat list for
WhatsApp chats (`impl/core/crm.go:99` passes raw `wa_id`).

**Changes**
- `entity/user.go`: extract `NormalizePhone(s string) string` (digits only, `+` prefix, `""` for
  empty input — today `NewUser("", "", id)` produces `"+"`, which must stop). Reuse it in
  `NewUser` and replace `chat.NormalizePhone` with a call to it (or make one delegate to the other).
- `internal/service/auth/auth-service.go`: normalize `phone` at the top of `GetUser`,
  `UserExists`, `ActivatePromoCode`, `BlockUser`, and any other method taking a phone.
- `internal/database/user.go`, `GetUser`: normalize defensively as well.
- `impl/core/crm.go:99`: use `GetUser("", entity.NormalizePhone(userID), 0)`.

**Verify:** call `POST /user/promo {"phone":"380..."}` for an existing `+380...` user → returns the
existing user, document in Mongo unchanged.

### P1.2 Field-level updates instead of whole-document `$set`
**Problem:** `UpsertUser` does `$set: <whole struct>` with an `$or` filter. Every caller holding a
stale copy overwrites concurrent changes (tool edits during AI turn, admin block, promo expiry).

**Changes**
- `internal/database/user.go`
  - `CreateUser(user)` — `InsertOne`; on duplicate key return the existing doc (relies on P3.2 unique indexes).
  - `UpdateUserFields(uuid string, fields bson.M)` — `UpdateOne({uuid}, {$set: fields})`.
  - `PushConversation(uuid string, msg entity.Message, keep int)` — `$push` with `$each` + `$slice: -keep`.
  - `ClearConversation(uuid)`.
  - Keep `UpsertUser` temporarily, but filter by `uuid` only; remove after all callers migrate.
- `internal/service/auth/auth-service.go`: migrate callers
  - `UpdateConversation` → `PushConversation`
  - `BlockUser` → `UpdateUserFields{blocked, role?}` (see P2.4 for role handling)
  - `SetSmartSenderId`, `ActivatePromoCode`, phone/email/name/address updates → `UpdateUserFields`
  - `RegisterUser` → `CreateUser`
- `internal/database/qr-stat.go` `FollowQr`: `$setOnInsert: {registered: false, ...}`, `$set` only
  the scan fields.
- Remove the auto-register fallback from read-only paths (`GetUser` used by `/user`, `/user/promo`,
  `/qr/stat`, `/user/close`, `/user/reset_conv`, `/user/block`) — return `user not found` instead.
  Only onboarding and `/user/create` register.

**Verify:** start an AI turn that calls `update_user_address`; after reply, the new address is in
Mongo. Block a user during an in-flight AI request; block persists.

### P1.3 Remove `log.Fatal` from request path
- `ai/gpt/audio.go:45`: return the error. Add a size check (`len(base64Str) > 25MB*4/3` → error)
  before decoding.
- Grep the repo for `log.Fatal`/`os.Exit`/`panic(` outside `main()` and replace with returned errors.

**Verify:** `POST /api/v1/response {"voice_msg_base64":"!!"}` → 4xx, process still alive.

### P1.4 Lock the API-key cache
- `impl/core/core.go`: replace `keys map[string]string` with a small type
  `keyCache{mu sync.RWMutex; m map[string]cachedKey}` where `cachedKey{username, scope, expires}`.
- `impl/core/http-auth.go`: read under `RLock`, write under `Lock`; entries expire after 5 min so
  revoked keys stop working without restart. Compare config key with `subtle.ConstantTimeCompare`.
- `impl/core/api.go:245`: same cache.

### P1.5 Make `auth.Service` user cache safe — or remove it
**Recommendation:** remove it. After P3.1 (shared Mongo client) and P3.2 (indexes) a lookup is a
sub-millisecond indexed query, and the cache is the source of stale data.
- Delete `s.users`, `updateUser`, and all `for _, user := range s.users` loops; call the repository directly.
- If profiling later shows a need, reintroduce as `map[uuid]entry` + `RWMutex` + TTL, invalidated on every write.

### P1.6 `adminLevels` concurrent map access (`bot/tgbot.go:116,179`)
- **If D1 = yes:** add `sync.RWMutex` to `TgBot` around `adminLevels` reads/writes. Minimal diff.
- **If D1 = no:** P4.4 (async log sender) moves all `SendMessageWithLevel` calls onto one goroutine;
  the `/level` command still runs on the dispatcher goroutine, so the race remains but is narrower.
  Document as known issue.

### P1.7 Panic recovery in background goroutines
- New helper `internal/lib/safego/safego.go`: `Go(log *slog.Logger, name string, fn func())` that
  wraps `fn` with `defer recover()` + error log with stack (`debug.Stack()`).
- Use it for: `bot/insta/instabot.go:158` (token refresher) and `:235` (`processPayload`),
  `bot/whatsapp/whatsappbot.go:179`, `bot/chat/mainmenu/video_step.go` goroutines,
  `impl/core/core.go` schedulers, `impl/core/response.go:16` (SmartSender), zoho-functions flusher,
  ws hub `Run`.

### P1.8 Instagram token persistence
- `main.go:245-250`: load `db.GetInstagramToken()` first; use config token only if Mongo is empty
  (then save it). Never overwrite a stored token with the config value on startup.
- `bot/insta/instabot.go` refresh loop: on failure retry with backoff (1h, 2h, 4h … cap 24h) instead
  of waiting 30 days. Log at Error so it reaches Telegram.

### P1.9 Nil / bounds panics
- `ai/gpt/cmd-handler.go`: all handlers unmarshal into value structs (`var req T`, not `*T`);
  `handleValidateOrder` / `handleCreateOrder` treat nil basket as empty → return "basket is empty".
- `internal/service/zoho/zoho-service.go:185`: guard `len(multiErr.Errors) > 0`.
- `internal/service/zoho/order-products.go`: return empty struct, not `nil, nil`.
- `internal/service/zoho/create-order.go:91`: check `len(phone) >= 4` before `phone[:4]`.
- `internal/http-server/middleware/authenticate/authenticate.go:61`: `strings.CutPrefix(h, "Bearer ")`.
- `bot/gdrive_auth.go:74`: checked type assertion.
- `impl/core/core.go:252` `Init`: skip index creation when `c.repo == nil`
  (or make Mongo mandatory — see P3.1).

---

## Phase 2 — Security and access control

### P2.1 Scoped API keys
**Scopes:** `admin`, `integration` (website/SmartSender), `crm` (CRM frontend), `mcp` (OpenAI only).

- `entity`: `UserAuth{Username, Scope}`; api-keys documents get a `scope` field
  (migration: existing keys → `integration`, config key → `admin`; see D2).
- New middleware `authenticate.RequireScope(scopes ...string)`.
- `internal/http-server/api/api.go` route mapping:

| Routes | Scopes |
|---|---|
| `/products/info`, `/response`, `/smart/send`, `/qr/follow`, `/user` (GET), `/user/create`, `/user/phone`, `/user/promo`, `/user/activate`, `/school/list` | `integration`, `admin` |
| `/user/block`, `/user/close`, `/user/reset_conv`, `/user/import-telegram`, `/assistant/*`, `/promo/*`, `/key/new`, `/school/add`, `/school/status`, `/qr/stat`, `/zoho/order_products` | `admin` |
| `/crm/*`, `/crm/ws` | `crm`, `admin` |
| `/mcp` | `mcp` only |

### P2.2 API key generation
- `internal/database/mongo.go:126-152`: never return an existing key; generate 32 bytes from
  `crypto/rand`, hex/base64url-encode; store `sha256(key)` instead of plaintext (lookup by hash).
- `/key/new` takes `{name, scope}`; `admin` only (P2.1).
- Provide a one-off migration that hashes existing keys in place.
- Log key prefix only via an ID, not the first 5 chars of the secret.

### P2.3 MCP hardening
- `main.go:123`: create a dedicated `mcp`-scope key instead of `GenerateApiKey("openai")`; rotate
  the old one.
- `ai/gpt/response-ask.go:143`: MCP server URL from config (`openai.mcp_url`), not hard-coded
  `backup.darkbyrior.com`.
- User binding: instead of a bare `X-User-UUID`, `Ask` sends `X-User-Token` = HMAC(uuid | assistant
  | expiry, mcp_secret). `mcp/handler.go` verifies it and derives uuid + assistant from it. Reject
  missing/invalid tokens (no `"default-user"` fallback).
- `ai/gpt/cmd-handler.go` `HandleCommand`: reject any tool not in `ToolsDescription(assistant)`.
  Remove the unadvertised `update_user_phone/email/name` cases (or advertise them deliberately).
- `mcp/handler.go`: `MaxBytesReader` (1 MB); drop request/response body debug logging.

### P2.4 `/user/block` role handling
- `auth-service.go:238`: only change role if a non-empty role is supplied and it's in the allow-list
  `{guest, user, manager}`. Granting `admin` only via config/DB, never via API.

### P2.5 Phone OTP
- Per D3: generate code, store `sha256(code)` + expiry (5 min) + attempts in Mongo; add
  `POST /user/phone/verify {phone, code}`; stop returning the code in `/user/phone`.
- `/user/create` and `/user/import-telegram`: refuse to overwrite a non-empty `telegram_id`
  with a different value (return 409).

### P2.6 Onboarding phone linking
- `bot/chat/onboarding/steps.go` `RequestPhoneStep`:
  - Telegram: accept only `input.Phone` from contact share; verify `contact.UserId == sender`
    (requires passing it through `UserInput`). Typed text → re-prompt with contact button.
  - Instagram/WhatsApp (per D4): if the phone belongs to an existing user with a different platform
    ID, do not link; create a new user or flag for manager confirmation.
- `ConfirmDataStep`: never overwrite a non-empty `InstagramId`/`TelegramId`.

### P2.7 Separate file-signing secret
- `internal/config/config.go`: `listen.file_signing_secret` (required, no default).
- `main.go:74`: use it; refuse to start if empty.

### P2.8 Webhook hardening
- `bot/insta/instabot.go`, `bot/whatsapp/whatsappbot.go`:
  - Refuse to start the bot if `enabled` and `app_secret` or `verify_token` is empty.
  - `http.MaxBytesReader(w, r.Body, 1<<20)` before `io.ReadAll`.
  - Verify-token compare with `subtle.ConstantTimeCompare`; respond `text/plain` + `nosniff`.
  - On bad signature log only length + IP, not the body.
- Deduplicate deliveries: in-memory TTL set (10 min) of `message.mid` / WA `messages[].id`;
  skip duplicates before `processPayload`.

### P2.9 File serving
- `internal/http-server/handlers/crm/file.go`: `X-Content-Type-Options: nosniff`,
  `Content-Security-Policy: sandbox`; `inline` only for `image/*`, `video/*`, `audio/*` (D5),
  otherwise `attachment`; filename via `mime.FormatMediaType("attachment", {"filename": name})`.
- `crm/send-file.go`: wrap body in `MaxBytesReader` (N files × max size), cap file count (e.g. 10),
  detect MIME with `http.DetectContentType` instead of trusting the client header.

### P2.10 Public URL from config
- `config.go`: `listen.public_url` (required when IG/WA enabled).
- Delete `detectPublicURL` middleware (`api.go:215-237`).

### P2.11 WebSocket auth
- `POST /crm/ws-ticket` (scope `crm`) returns a one-time ticket (random 32 bytes, 30 s TTL, in-memory).
- `internal/ws/client.go`: accept `?ticket=` instead of `?token=`; `CheckOrigin` against
  `listen.allowed_origins` config list.

### P2.12 Order ownership check
- `bot/chat/mainmenu/steps.go:450,543`: accept `products:<id>` only if `<id>` is in the IDs stored
  in state for this user (`current_order_id`, `completed_order_ids`).

### P2.13 Secrets and PII out of logs
- Instagram Graph calls: token via `Authorization: Bearer` header, not query.
- Telegram file download (`userbot.go:345`) and zoho-functions `zapikey` (`service.go:89`):
  on error log `errors.Unwrap(urlErr)` only. Add helper `sl.HTTPErr(err)` that strips URLs from
  `*url.Error`.
- Remove: Zoho `TokenResponse` debug log (`zoho-service.go:83`), order payload in error attrs
  (`zoho-service.go:233`, `create-rating.go:57`), full message text in `smart-sender/send.go:75`,
  WA body debug log (`whatsappbot.go:156`).
- Add `sl.Phone(p)` masking helper (`+380*****4567`) and use it everywhere a phone is logged.

### P2.14 Error leakage and CORS
- Handlers listed in the audit (`compose-response.go:48`, `block-user.go:47`, `check-phone.go:43`,
  `reset_conversation.go:33`, `get-active.go:31`, `key/generate.go:43`, `school/*`, `qr-stat/*`,
  `mcp/handler.go:104,110`): return generic messages, log the detailed error.
- CORS: allow-list from config instead of `*`.

---

## Phase 3 — Persistence

### P3.1 Single Mongo client
- `internal/database/mongo.go`: `NewMongoClient` connects once, `Ping`s with 10 s timeout (fail
  startup on error), stores `*mongo.Client`; options `MaxPoolSize(50)`, `ServerSelectionTimeout(5s)`,
  `Timeout(10s)`.
- Delete `connect()` / `disconnect()`; mechanically replace the pattern in all 52 repository methods
  with `m.client.Database(m.database).Collection(...)`.
- Every method takes `ctx context.Context` as first arg (or at minimum uses
  `context.WithTimeout(context.Background(), 5*time.Second)` internally — prefer the former, done
  in the same pass).
- `MongoDB.Close(ctx)` called from graceful shutdown (P4.1).
- Decide: make Mongo mandatory (remove `mongo.enabled`) — almost every feature depends on it.

### P3.2 Indexes
New `EnsureIndexes(ctx)` (replaces `EnsureChatMessageIndexes`), called at startup:

| Collection | Index | Options |
|---|---|---|
| users | `uuid` | unique |
| users | `phone`, `email`, `telegram_id`, `instagram_id`, `smart_sender_id` | unique + partial (`$exists`/non-empty) |
| chat_states | `(platform, user_id)` | unique |
| api-keys | `key_hash` | unique |
| api-keys | `username` | — |
| promo-codes | `code` | unique |
| baskets | `userUUID` | unique |
| qr-stat | `smart_sender_id`; `(platform, user_id)` | — |
| schools | `code` | unique |
| chat-messages | `created_at` | — (for `GetActiveChats`) |
| otp (new, P2.5) | `expires_at` | TTL |

**Before creating unique indexes:** run a one-off script to find and merge duplicates (users by phone
especially — P1.1 may already have created some). Write it as `cmd/dedupe-users/main.go`, dry-run by default.

### P3.3 Query fixes
- `chat-message.go` `GetActiveChats`: `$match` recent window (e.g. 90 days) before `$sort`, add
  `$limit`; `CountUnreadPerChat`: replace `$push` of all timestamps with `$sum`+`$cond`.
- Check `cursor.Err()` after every `Next` loop; return decode/delete errors instead of swallowing.
- `assistant.go:48`: `errors.Is(err, mongo.ErrNoDocuments)`.
- `SetVectorStore`: don't report error after upsert created a stub.

### P3.4 Promo atomicity
- `auth/promo.go`: return `user not found` instead of nil; activate code + grant access in one
  Mongo transaction (requires replica set) or compensate by un-activating the code on grant failure.

---

## Phase 4 — Lifecycle, timeouts, logging

### P4.1 Graceful shutdown
- `main.go`: `ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`.
- Pass `ctx` to: schedulers (`core.Init`), ws hub, bots (`Start(ctx)`), Instagram refresher,
  zoho-functions flusher.
- On `ctx.Done()`: `srv.Shutdown(30s)` → stop bots → flush zoho-functions buffer → flush log
  queue (P4.4) → `db.Close()` → close log file.
- systemd: set `TimeoutStopSec=45` in the unit file.

### P4.2 HTTP server and client timeouts
- `api.go:196`: `ReadHeaderTimeout: 10s`, `ReadTimeout: 60s`, `WriteTimeout: 120s`, `IdleTimeout: 120s`.
- Remove `timeout.Timeout(5)` (it's ineffective and would cut AI requests if it worked); instead
  pass `r.Context()` through `Core` methods into OpenAI/Zoho/Mongo calls.
- Shared clients, one per service, injected at construction:
  - OpenAI Responses: `http.Client{Timeout: 120s}`
  - Zoho, product service, zoho-functions, SmartSender: `Timeout: 15s`
  - IG/WA Graph API: `30s`; media download: no total timeout, `ResponseHeaderTimeout: 30s` + ctx deadline
  - Google Drive download: separate client without `Timeout` (P5.2)
- Check `resp.StatusCode` everywhere a response is decoded; wrap bodies in `io.LimitReader`.

### P4.3 Zoho token management
- `internal/service/zoho/token.go`: `type tokenSource struct{ mu sync.Mutex; token string; exp time.Time }`
  with `Token(ctx)` — refresh under lock only if `time.Until(exp) < 60s`; handle `ExpiresIn == 0`
  (default 1h).
- All Zoho methods call `doWithAuth(req)`: on 401 / `INVALID_TOKEN` force-refresh once and retry.
- Named returns `(err error)` in all methods with deferred logging so failures aren't logged as success.

### P4.4 Async Telegram log handler
- `internal/lib/logger/tghandler.go`: `Handle` pushes records into a buffered channel (cap 500,
  drop-oldest on full with a counter); single sender goroutine batches records (max 1 msg/s per
  admin, concatenating up to 4000 chars).
- Honor `conf.Telegram.MinLogLevel` (default `warn`) instead of hard-coded Debug in `main.go:53`.
- Shared state (channel, mutex) must live in a pointer shared across `WithAttrs`/`WithGroup` clones.
- `Flush(ctx)` for shutdown.
- Log file: mode `0640`, closed on shutdown.

### P4.5 Zoho-functions buffer
- `zoho-functions/buffer.go`: on send failure re-queue items (cap 1000, drop oldest); `Stop(ctx)`
  flushes synchronously; called from P4.1.

### P4.6 Per-user serialization
- New `internal/lib/keymutex`: `Lock(key) func()` (map of refcounted mutexes, entries removed when unused).
- `bot/chat/engine.go`: lock `platform:userID` around load → step → save in `HandleMessage`,
  `HandleCallback`, `HandleContact`.
- `impl/core/response.go` `processRequest`: lock `user.UUID` for the whole AI turn.
- Delete the unused `LockThreads`.

---

## Phase 5 — AI and functional bugs

### P5.1 OpenAI cost and reliability
- Message cap: reject `> 4 KB` text in `/response` and bot AI step; request body `MaxBytesReader(1MB)`
  (voice: 25 MB).
- Per-user rate limit (`golang.org/x/time/rate`, map by UUID with TTL cleanup): e.g. 10 msgs/min.
- Overseer routing call: send only the last 3 Q/A pairs.
- `response-ask.go`: check `status == "completed"`, surface `incomplete_details`; fix deferred
  `recover` with named returns so panics become errors.
- `determineAssistant`: if result not in `user.GetAssistants()`, fall back to Consultant; drop
  `Calculator` from the enum if unused.
- Return errors from `handleClearBasket`, unknown tool names, product lookup (`response-ask.go:277`).
- SmartSender path (`impl/core/response.go:16`): bounded worker pool (e.g. 8 workers, queue 200).

### P5.2 Order tool validation
- `add_to_basket`: `1 <= quantity <= 100`.
- `create_order`: reject empty basket; re-run `ValidateOrder` inside; idempotency — reject if an
  identical order (same basket hash) was created by this user in the last 10 minutes.

### P5.3 Vector store refresh
- `overseer.go:658-731`: poll new store until `file_counts.completed == total` (timeout 10 min);
  switch assistants only if all `SetVectorStore` calls succeed; delete old stores only after that.
  Paginate `ListVectorStores`. Match old product files on `filepath.Base` prefix `products-`.

### P5.4 Delete dead Assistants API code
- Remove `handleRun`, `getOrCreateThread`, `summarizeMessages`, `o.threads`, `LockThreads`,
  `entity.Assistant.Id` usages.
- Update CLAUDE.md "Thread management with automatic summarization" → describe Responses API +
  `user.Conversation` (20 pairs).

### P5.5 Bots
- **WhatsApp user resolution** (`mainmenu/steps.go:218`): add `whatsapp` case →
  `GetUser("", NormalizePhone(state.UserID), 0)`.
- **Telegram video timeout:** pass `RequestOpts{Timeout: 10 * time.Minute}` on `SendVideo`/
  `SendDocument` in `bot/chat/telegram/messenger.go`; Drive download client without total
  timeout (`internal/gdrive/gdrive.go:169`).
- **HTML escaping:** `SendText` escapes text by default (`html.EscapeString`); add
  `SendHTML` for the few call sites that build markup (ТТН links). On 400 "can't parse entities"
  retry without parse mode. Stop discarding send errors (`_ =`) — log them.
- **Message length:** split in messenger `SendText`: Telegram 4096, Instagram 1000, WhatsApp 4096
  runes, splitting on newline boundaries.
- **Stuck users:** in `engine.go`, if workflow/step not found → delete state, restart onboarding;
  auto-steps' `HandleInput` re-runs `Enter`; text keywords `start`/`старт`/`меню` reset on IG/WA.
- **Stable callback IDs:** `vid_sel:<driveFileID>`, `school_sel:<objectID hex>` (both < 64 bytes).
- **Protected video fallback:** add `protected` to `SendFile`; make the public Drive link fallback
  opt-in via config.
- **Video step timeout:** on Drive listing error return `NextStep: StepMainMenu` with a message.
- **Expired buttons:** reply "Меню застаріло" + re-enter current step instead of silent ignore.
- **gdrive:** paginate `Files.List`, filter `mimeType contains 'video/'`, serve stale cache on error,
  `singleflight` around refresh. `cmd/gdrive-auth`: listen on `127.0.0.1`, random `state`
  verified on callback, non-blocking channel send.
- `mainmenu/workflow.go:117` `SetStepActive`: guard `activeSteps` with `RWMutex`.
- IG/WA attachment downloads: check `resp.StatusCode`, reject empty `mediaInfo.URL`.

### P5.6 Money and product service
- `zoho/create-order.go:126-137`: `math.Round(v*100)/100`; guard `Quantity > 0` before dividing.
- `product/user.go`, `product/response.go`: return errors on non-200 / parse failure;
  `url.PathEscape` the phone; `order.go:64` check `response.Success`.

---

## Phase 6 — CI/CD and tests

### P6.1 Deploy workflow (`.github/workflows/deploy.yml`)
- Pin `appleboy/scp-action` and `appleboy/ssh-action` to full commit SHAs.
- `go-version-file: go.mod` instead of `'1.24'`.
- Replace the `sed` block with `envsubst < darkcs-conf.yml > out.yml` (all values via `env:`).
- Credentials file: `printf '%s' "$GDRIVE_CREDS" > gdrive-credentials.json` with `env:`, not
  inline `${{ }}` interpolation.
- Add `go vet ./...` and `go test ./...` steps before deploy.
- After restart: `systemctl is-active --quiet darkcs.service` + curl a new `/healthz` endpoint
  (P6.2); fail the job if not healthy.
- Remove the `darkcs-conf.yml` entry from `.gitignore` (the file is tracked intentionally as a
  placeholder template, so the ignore entry is misleading).

### P6.2 Health endpoint
- `GET /healthz` (no auth): Mongo ping + uptime; returns 503 if Mongo unreachable.

### P6.3 Tests (minimum safety net, written alongside the phases)
| Area | Test |
|---|---|
| `entity.NormalizePhone` | table test incl. empty, `+`, spaces, dashes |
| `auth.Service` | GetUser with unnormalized phone does not create/overwrite (mock repo) |
| `authenticate` middleware | missing/malformed header, scope enforcement |
| `fileurl` | sign/verify, expired, tampered |
| webhook signature | valid/invalid/empty-secret |
| `chat.ChatEngine` | unknown step resets state; concurrent messages serialized |
| `cmd-handler` | null args, nil basket, disallowed tool rejected |
| `zoho` money rounding | rounding + zero quantity |
| `keymutex`, log handler queue | race tests (`go test -race`) |
| Mongo repository | integration tests behind build tag `integration` using a test DB |

---

## New configuration fields

```yaml
listen:
  public_url: ${PUBLIC_URL}
  file_signing_secret: ${FILE_SIGNING_SECRET}
  allowed_origins: [https://crm.example.com]
openai:
  mcp_url: ${MCP_URL}
  mcp_secret: ${MCP_SECRET}
telegram:
  min_log_level: warn
limits:
  max_message_bytes: 4096
  user_rate_per_minute: 10
```

All must be added to `darkcs-conf.yml`, the deploy workflow, and GitHub secrets/vars before
deploying the phase that needs them.

---

## Rollout order and estimates

| Phase | Effort | Notes |
|---|---|---|
| P1 | 1.5–2 days | Ship immediately; P1.2 is the largest item. Run `cmd/dedupe-users` (P3.2) dry-run right after to assess damage already done. |
| P3.1 + P3.2 | 1.5 days | Mechanical but touches all repository files; do before P4 since contexts flow through. |
| P2 | 3 days | Coordinate key migration (D2) with integration owners; issue new keys before enforcing scopes. |
| P4 | 2 days | |
| P5 | 3 days | Bot items are independent and can be split into separate PRs. |
| P6 | 1 day | P6.1 can be done at any time; tests are written per phase. |

**Total:** ~12 working days for one developer.

Suggested PR split: one PR per task group (P1.1+P1.2, P1.3–P1.9, P3.1, P3.2, P2.1–P2.3, …) so
each can be reviewed and rolled back independently.
