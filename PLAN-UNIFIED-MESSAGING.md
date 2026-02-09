# Plan: Unified Chat Engine for All Platforms

## Context

Instagram and WhatsApp bots are standalone echo bots. Telegram has a full workflow engine (onboarding, main menu, AI, orders, ratings) tightly coupled to the Telegram API (`*tgbotapi.Bot`, `*ext.Context`). The goal: ONE business logic layer, each platform is just a UI adapter. Same features everywhere. Cross-platform user linking by phone.

## Architecture

```
                    ┌─────────────────┐
                    │   ChatEngine    │  ← Platform-agnostic state machine
                    │  (bot/chat/)    │     (same code for all platforms)
                    └───────┬─────────┘
                            │
            ┌───────────────┼───────────────┐
            ▼               ▼               ▼
    ┌──────────────┐ ┌────────────┐ ┌──────────────┐
    │  Telegram    │ │ Instagram  │ │  WhatsApp    │
    │  Messenger   │ │ Messenger  │ │  Messenger   │
    │ (keyboards)  │ │ (text menu)│ │ (text menu)  │
    └──────────────┘ └────────────┘ └──────────────┘
```

All business logic (onboarding, main menu, AI, orders, ratings) lives in ChatEngine steps.
Each platform only implements the `Messenger` interface (how to send text/menus).

## Phase 1 (this PR): Unified engine + Instagram/WhatsApp

Telegram keeps its existing workflow engine — it works and we don't break it.
Instagram and WhatsApp get full features via the new unified ChatEngine.

## Phase 2 (follow-up PR): Migrate Telegram

Create `TelegramMessenger` adapter, switch UserBot to ChatEngine, remove old `bot/workflow/` + `bot/workflows/`.

---

## Key Interfaces

### Messenger — platform UI adapter (`bot/chat/messenger.go`)
```go
type Messenger interface {
    SendText(chatID, text string) error
    SendMenu(chatID, text string, rows [][]MenuButton) error
    SendInlineOptions(chatID, text string, buttons []InlineButton) error
    SendTyping(chatID string) error
}
type MenuButton struct { Text string }
type InlineButton struct { Text, Data string }
```
Telegram: `SendMenu` → reply keyboard. Instagram/WhatsApp: `SendMenu` → numbered text list.

### UserInput — normalized event from any platform
```go
type UserInput struct {
    Text         string  // Regular message text
    CallbackData string  // Inline button press or matched number
    Phone        string  // Contact share or typed phone
}
```

### Step — business logic unit (`bot/chat/interfaces.go`)
```go
type Step interface {
    ID() StepID
    Enter(ctx context.Context, m Messenger, state *ChatState) StepResult
    HandleInput(ctx context.Context, m Messenger, state *ChatState, input UserInput) StepResult
}
```
Same as current `bot/workflow/interfaces.go` Step, but `*tgbotapi.Bot` → `Messenger`, `*ext.Context` → `UserInput`.

### ChatState — platform-agnostic state (`bot/chat/state.go`)
```go
type ChatState struct {
    Platform    string         `bson:"platform"`
    UserID      string         `bson:"user_id"`       // Platform sender ID (string, not int64)
    ChatID      string         `bson:"chat_id"`
    WorkflowID  string         `bson:"workflow_id"`
    CurrentStep StepID         `bson:"current_step"`
    Data        map[string]any `bson:"data"`
    UpdatedAt   time.Time      `bson:"updated_at"`
}
```

### ChatEngine — orchestrator (`bot/chat/engine.go`)
```go
func (e *ChatEngine) HandleMessage(ctx, m Messenger, platform, userID, chatID, text string) error
func (e *ChatEngine) HandleCallback(ctx, m Messenger, platform, userID, chatID, data string) error
```
Same orchestration logic as `bot/workflow/engine.go` processResult() — step transitions, state saves, auto-transitions, workflow chaining. But platform-agnostic.

---

## Implementation Steps

### Step 1: Add `InstagramId` to User entity

**Modify: `entity/user.go`**
- Add `InstagramId string` field with json/bson tags
- Add Instagram matching to `SameUser()`

### Step 2: Extend database + auth for Instagram lookup

**Modify: `internal/database/user.go`**
- Add `GetUserByInstagramId(id string)` method
- In `UpsertUser`: add `instagram_id` to `$or` filter

**Modify: `internal/service/auth/auth-service.go`**
- Add `GetUserByInstagramId` to `Repository` interface + `Service` implementation

**Modify: `impl/core/core.go`**
- Add `GetUserByInstagramId` to `AuthService` interface

### Step 3: Create `bot/chat/` core package

**New: `bot/chat/messenger.go`** — Messenger interface, MenuButton, InlineButton, UserInput

**New: `bot/chat/interfaces.go`** — Step, Workflow, StepResult, ChatStateStorage interfaces

**New: `bot/chat/state.go`** — ChatState struct with Set/Get helpers (port from `bot/workflow/state.go`)

**New: `bot/chat/engine.go`** — ChatEngine struct
- `HandleMessage` / `HandleCallback` — entry points
- `processResult` — step transitions (port logic from `bot/workflow/engine.go:161-257`)
- `StartWorkflow` — begin a workflow
- State storage by `{platform, user_id}` composite key

**New: `bot/chat/helpers.go`** — normalizePhone, isValidPhone, number-to-menu-option matching

### Step 4: Implement workflows using Messenger interface

**New: `bot/chat/onboarding/`** — Onboarding workflow (port from `bot/workflows/onboarding/`)
- Steps: Hello → RequestPhone → CheckUser → RequestName → ConfirmData → done (chain to mainmenu)
- `RequestPhoneStep.Enter`: WhatsApp auto-skips (phone from sender ID), Instagram prompts text input
- `ConfirmDataStep`: cross-platform linking — if user exists by phone, link InstagramId
- Services needed: AuthService, ZohoService

**New: `bot/chat/mainmenu/`** — Main menu workflow (port from `bot/workflows/mainmenu/`)
- Steps: MainMenu, MyOffice, CurrentOrder, CompletedOrders, ServiceRate, AIConsultant, MakeOrder
- `MainMenuStep.Enter`: calls `m.SendMenu()` with same options as Telegram
- `MainMenuStep.HandleInput`: match by button text OR by number ("1"-"5")
- `AIConsultantStep.HandleInput`: calls `aiService.ProcessUserRequest(user, text)`
- `ServiceRateStep`: uses `m.SendInlineOptions()` for 1-5 rating
- `OrderSteps`: query Zoho, format as text, send
- Services needed: AuthService, AIService, ZohoService

### Step 5: Create platform Messenger implementations

**New: `bot/chat/instagram/messenger.go`**
- Wraps `InstaBot.SendMessage` for text delivery
- `SendMenu`: numbered text ("1. 📦Особистий кабінет\n2. ⭐Оцінка сервісу\n...\nОберіть опцію:")
- `SendInlineOptions`: same numbered text pattern

**New: `bot/chat/whatsapp/messenger.go`**
- Wraps `WhatsAppBot.SendMessage` for text delivery
- Same text-menu pattern as Instagram

### Step 6: Create ChatState MongoDB storage

**New: `internal/database/chat_state.go`**
- Collection: `chat_states`
- CRUD by `{platform, user_id}` composite key
- Same connect/disconnect pattern as existing DB code

### Step 7: Integrate bots with ChatEngine

**Modify: `bot/insta/instabot.go`**
- Add `chatEngine` field + setter
- In `processPayload`: create InstagramMessenger, call `chatEngine.HandleMessage(ctx, messenger, "instagram", senderID, senderID, text)`
- Keep echo fallback when engine is nil

**Modify: `bot/whatsapp/whatsappbot.go`**
- Same: add `chatEngine` field, create WhatsAppMessenger, delegate to engine

### Step 8: Wire in main.go

**Modify: `main.go`**
- Create ChatEngine with MongoDB storage
- Register onboarding + mainmenu workflows (with authService, zohoService, handler as AIService)
- Inject ChatEngine into Instagram + WhatsApp bots

---

## Files Summary

**New files (~12):**
- `bot/chat/messenger.go` — Messenger interface + types
- `bot/chat/interfaces.go` — Step, Workflow, ChatStateStorage
- `bot/chat/state.go` — ChatState struct
- `bot/chat/engine.go` — ChatEngine orchestrator
- `bot/chat/helpers.go` — phone normalization, menu matching
- `bot/chat/onboarding/workflow.go` — Onboarding workflow + steps
- `bot/chat/mainmenu/workflow.go` — Main menu workflow + steps
- `bot/chat/instagram/messenger.go` — Instagram Messenger
- `bot/chat/whatsapp/messenger.go` — WhatsApp Messenger
- `internal/database/chat_state.go` — MongoDB state storage

**Modified files (7):**
- `entity/user.go` — add InstagramId
- `internal/database/user.go` — add GetUserByInstagramId, update UpsertUser
- `internal/service/auth/auth-service.go` — add GetUserByInstagramId
- `impl/core/core.go` — add GetUserByInstagramId to interface
- `bot/insta/instabot.go` — delegate to ChatEngine
- `bot/whatsapp/whatsappbot.go` — delegate to ChatEngine
- `main.go` — create ChatEngine, wire dependencies

## Verification

1. Instagram: new user → onboarding (name → phone) → main menu as numbered text → "4" → AI mode → get response
2. WhatsApp: new user → onboarding (name only, phone auto-detected) → main menu → AI mode
3. Instagram: reply "3" from menu → order status from Zoho
4. Cross-platform: register on Instagram with same phone as Telegram user → linked, shared conversation
5. Telegram: continues working unchanged via old workflow engine
