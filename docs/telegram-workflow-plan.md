# Telegram Bot Workflow System Implementation Plan

## Overview

Implement a flexible workflow engine for the Telegram bot that handles multi-step user interactions with state persistence, inline buttons, and paginated lists. The first workflow will be an onboarding flow triggered by deep links.

## Deep Link Format

Telegram deep links: `t.me/botname?start=CODE` where CODE is base64-encoded.

Example: `https://t.me/darkbyriorbot?start=ZGw6Mjg5MjM0`
- `ZGw6Mjg5MjM0` decodes to `dl:289234`
- Format: `type:id` (base64 encoded)

Supported types:
- `school:123` → triggers school selection, ID used to filter/preselect
- `manager:456` → (future) triggers manager workflow
- empty/unknown → skips to main menu

### DeepLink Struct
```go
type DeepLink struct {
    Type string // e.g., "school", "manager", "dl"
    ID   string // optional ID
}
```

### Parsing Logic
```go
func ParseDeepLink(code string) (*DeepLink, error) {
    decoded, err := base64.StdEncoding.DecodeString(code)
    if err != nil {
        return nil, err
    }
    parts := strings.SplitN(string(decoded), ":", 2)
    dl := &DeepLink{Type: parts[0]}
    if len(parts) > 1 {
        dl.ID = parts[1]
    }
    return dl, nil
}
```

## File Structure

```
bot/
├── tgbot.go                      # KEEP AS-IS - logging/admin notifications only
├── userbot.go                    # NEW - user-facing bot with workflows
├── workflow/
│   ├── interfaces.go             # NEW - Step, Workflow, Engine interfaces
│   ├── state.go                  # NEW - UserState struct
│   ├── engine.go                 # NEW - WorkflowEngine implementation
│   ├── storage.go                # NEW - MongoDB state storage
│   ├── callbacks.go              # NEW - Callback data parser/builder
│   ├── deeplink.go               # NEW - Base64 deep link parser
│   └── ui/
│       ├── keyboard.go           # NEW - Inline keyboard builders
│       └── pagination.go         # NEW - Paginated list (5 items, 3-button nav)
└── workflows/
    └── onboarding/
        ├── workflow.go           # NEW - Onboarding workflow definition
        └── steps.go              # NEW - Step implementations
entity/
└── school.go                     # NEW - School entity
internal/database/
├── workflow_state.go             # NEW - MongoDB operations for workflow_states collection
└── school.go                     # NEW - MongoDB operations for schools collection
main.go                           # MODIFY - wire workflow dependencies
```

## New Entity: School

```go
// entity/school.go
type School struct {
    ID        string    `json:"id" bson:"_id"`
    Name      string    `json:"name" bson:"name"`
    Active    bool      `json:"active" bson:"active"`
    CreatedAt time.Time `json:"created_at" bson:"created_at"`
}
```

New MongoDB collection: `schools`

## Core Interfaces

### Step Interface
```go
type Step interface {
    ID() StepID
    Enter(ctx context.Context, b *tgbotapi.Bot, state *UserState) error
    HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *UserState) StepResult
    HandleCallback(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *UserState, data string) StepResult
    HandleContact(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *UserState) StepResult
}
```

### StepResult
```go
type StepResult struct {
    NextStep    StepID
    UpdateState map[string]any
    Complete    bool
    Error       error
}
```

### UserState
```go
type UserState struct {
    UserID      int64
    ChatID      int64
    WorkflowID  WorkflowID
    CurrentStep StepID
    DeepLink    *DeepLink          // Parsed deep link {Type, ID}
    Data        map[string]any     // Workflow-specific collected data
    Pagination  *PaginationState
    UpdatedAt   time.Time
}
```

## UI Components

### Yes/No Buttons
```go
func YesNoKeyboard(yesText, noText string) tgbotapi.InlineKeyboardMarkup
```

### Paginated List (5 items per page)
```
[Item 1]
[Item 2]
[Item 3]
[Item 4]
[Item 5]
[◀️ Back] [Page 1/3] [Next ▶️]
```

### Contact Request (phone sharing)
```go
func ContactRequestKeyboard(buttonText string) tgbotapi.ReplyKeyboardMarkup
```

## Callback Data Format

Pattern: `wf:action:value`
- `wf:yes` / `wf:no` - Yes/No buttons
- `wf:page:3` - Pagination
- `wf:select:school_123` - Item selection
- `wf:confirm` / `wf:cancel`

## Onboarding Workflow Steps

| Step | ID | Action                                 | Next Step                                                                   |
|------|------|----------------------------------------|-----------------------------------------------------------------------------|
| 1 | `hello` | Send welcome message                   | `request_phone`                                                             |
| 2 | `request_phone` | Show contact share button              | `validate_phone`                                                            |
| 3 | `validate_phone` | Validate phone format                  | Invalid → `request_phone`, Valid → `check_user`                             |
| 4 | `check_user` | Query `authService.UserExists()`       | New → `request_name`, Existing → `process_deep_code`                        |
| 5 | `request_name` | Ask for name                           | `confirm_data`                                                              |
| 6 | `confirm_data` | Show Yes/No buttons with data summary  | No → `request_name`, Yes → save & `process_deep_code`                       |
| 7 | `process_deep_code` | Check DeepLink.Type                    | nil/unknown → `main_menu`, "school" → `select_school`, "manager" → (future) |
| 8 | `select_school` | Show paginated school list             | On select → save to DB user selection → `main_menu`                         |
| 9 | `main_menu` | Show main menu, mark workflow complete | -                                                                           |

## Workflow State Diagram

```
                    /start=CODE
                         │
                         ▼
                    ┌─────────┐
                    │  hello  │
                    └────┬────┘
                         │
                         ▼
                 ┌───────────────┐
                 │ request_phone │◄────────┐
                 └───────┬───────┘         │
                         │                 │
                         ▼                 │
                 ┌───────────────┐         │
                 │validate_phone │─────────┘
                 └───────┬───────┘  invalid
                         │ valid
                         ▼
                  ┌────────────┐
                  │ check_user │
                  └──────┬─────┘
            ┌────────────┴────────────┐
            │ new                     │ existing
            ▼                         │
    ┌──────────────┐                  │
    │ request_name │◄─────┐           │
    └──────┬───────┘      │           │
           │              │ no        │
           ▼              │           │
    ┌──────────────┐      │           │
    │ confirm_data │──────┘           │
    └──────┬───────┘                  │
           │ yes                      │
           ▼                          │
    ┌─────────────────┐◄──────────────┘
    │process_deep_code│
    └────────┬────────┘
             │
     ┌───────┼───────────────┐
     │       │               │
     │school │manager(future)│ empty/other
     ▼       ▼               ▼
┌──────────┐ ┌──────┐  ┌───────────┐
│select    │ │ ...  │  │ main_menu │
│school    │ └──────┘  └───────────┘
└────┬─────┘                 ▲
     └───────────────────────┘
```

## New UserBot (bot/userbot.go)

Separate from existing `tgbot.go` (which handles logging/admin notifications only).

```go
// bot/userbot.go
type UserBot struct {
    log            *slog.Logger
    api            *tgbotapi.Bot
    botUsername    string
    workflowEngine workflow.WorkflowEngine
    authService    core.AuthService
}

func NewUserBot(botName, apiKey string, log *slog.Logger,
    storage workflow.StateStorage, authService core.AuthService) (*UserBot, error)

func (u *UserBot) Start() error {
    dispatcher := ext.NewDispatcher(...)

    // Workflow handlers
    dispatcher.AddHandler(handlers.NewCommand("start", u.handleStart))
    dispatcher.AddHandler(handlers.NewCallback(callbackFilter, u.handleCallback))
    dispatcher.AddHandler(handlers.NewMessage(message.Contact, u.handleContact))
    dispatcher.AddHandler(handlers.NewMessage(message.Text, u.handleMessage))

    // Start polling...
}
```

### handleStart implementation:
```go
func (u *UserBot) handleStart(b *tgbotapi.Bot, ctx *ext.Context) error {
    // Parse: "/start ZGw6Mjg5MjM0" -> extract "ZGw6Mjg5MjM0"
    parts := strings.Fields(ctx.EffectiveMessage.Text)
    var deepLink *workflow.DeepLink
    if len(parts) > 1 {
        deepLink, _ = workflow.ParseDeepLink(parts[1])
    }

    return u.workflowEngine.StartWorkflow(
        ctx.EffectiveUser.Id,
        ctx.EffectiveChat.Id,
        onboarding.WorkflowID,
        map[string]any{"deep_link": deepLink},
    )
}
```

### Config addition:
```yaml
telegram:
  # existing logging bot config...

user_bot:
  enabled: true
  api_key: "USER_BOT_TOKEN"
  bot_name: "darkbyriorbot"
```

## Implementation Order

1. **Core workflow engine** (`workflow/interfaces.go`, `state.go`, `engine.go`)
2. **Storage layer** (`workflow/storage.go`, `internal/database/workflow_state.go`)
3. **School entity & DB** (`entity/school.go`, `internal/database/school.go`)
4. **UI components** (`workflow/ui/keyboard.go`, `pagination.go`)
5. **Callback & deep link handlers** (`workflow/callbacks.go`, `deeplink.go`)
6. **Onboarding workflow** (`workflows/onboarding/workflow.go`, `steps.go`)
7. **New UserBot** (`bot/userbot.go`) - separate from existing tgbot.go
8. **Config & main.go** - add user_bot config, wire up in main.go

## Key Dependencies

- Reuse existing `authService.RegisterUser()`, `UserExists()`, `GetUser()` from `internal/service/auth/auth-service.go`
- Follow MongoDB patterns from `internal/database/mongo.go`
- Use gotgbot/v2 patterns already in `bot/tgbot.go`

## Verification

1. Start bot with `/start` - should trigger onboarding without deep link
2. Start bot with `/start c2Nob29sOjEyMw==` (base64 of `school:123`) - should trigger onboarding with school selection
3. Test phone validation loop (invalid → retry)
4. Test existing user detection (skip name entry)
5. Test pagination on school list (7+ items)
6. Restart bot mid-workflow - state should persist from MongoDB

### Generate test deep links:
```go
// school:123 -> c2Nob29sOjEyMw==
base64.StdEncoding.EncodeToString([]byte("school:123"))
```
