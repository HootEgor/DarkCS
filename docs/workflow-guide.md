# Workflow Engine Guide

How to create new steps and workflows for the Telegram bot.

## Directory Structure

```
bot/
  workflow/           # Engine core (don't modify unless extending the engine)
    interfaces.go     # Step, Workflow, Engine, StateStorage interfaces
    engine.go         # WorkflowEngine implementation
    state.go          # UserState, pagination, deep links
    callbacks.go      # Callback data parsing (wf:action:value)
    storage.go        # MongoDB state storage adapter
    ui/
      keyboard.go     # Keyboard builders (YesNo, Contact, Selection, etc.)
      pagination.go   # Paginated list UI
  workflows/
    onboarding/       # Example workflow
      workflow.go     # Workflow definition, constants, service interfaces
      steps.go        # All step implementations
  userbot.go          # Bot entry point, handler registration
```

## Creating a New Step

### 1. Define the step ID

Add a constant in your workflow's `workflow.go`:

```go
const (
    StepGreeting workflow.StepID = "greeting"
)
```

### 2. Create the step struct

Embed `BaseStep` for default no-op handlers. Override only the methods you need.

```go
type GreetingStep struct {
    BaseStep
}

func NewGreetingStep() *GreetingStep {
    return &GreetingStep{BaseStep: BaseStep{id: StepGreeting}}
}
```

### 3. Implement `Enter()`

`Enter()` is called when the workflow transitions into this step. It returns a `StepResult`.

**Interactive step** (waits for user input):

```go
func (s *GreetingStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
    _, err := b.SendMessage(state.ChatID, "What is your name?", nil)
    if err != nil {
        return workflow.StepResult{Error: err}
    }
    return workflow.StepResult{} // empty result = wait for user input
}
```

**Auto-transition step** (processes data and moves on immediately):

```go
func (s *ValidateStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
    value := state.GetString("some_key")
    if value == "" {
        return workflow.StepResult{NextStep: StepCollectData} // go back
    }
    return workflow.StepResult{NextStep: StepNextStep} // proceed
}
```

The engine loops through auto-transitions automatically. If `Enter()` returns a `NextStep`, the engine transitions without waiting for user input.

### 4. Implement event handlers

Override only the handlers relevant to your step. `BaseStep` provides no-op defaults for all three.

**HandleMessage** — user sends a text message:

```go
func (s *GreetingStep) HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
    name := strings.TrimSpace(c.EffectiveMessage.Text)
    if name == "" {
        b.SendMessage(state.ChatID, "Please enter a valid name.", nil)
        return workflow.StepResult{} // stay on this step
    }
    return workflow.StepResult{
        NextStep:    StepNextStep,
        UpdateState: map[string]any{"name": name},
    }
}
```

**HandleCallback** — user presses an inline keyboard button:

```go
func (s *ChoiceStep) HandleCallback(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState, data string) workflow.StepResult {
    cb := workflow.ParseCallback(data)
    if cb == nil {
        return workflow.StepResult{}
    }
    c.CallbackQuery.Answer(b, nil) // always answer callbacks

    if cb.IsYes() {
        return workflow.StepResult{NextStep: StepConfirmed}
    }
    if cb.IsNo() {
        return workflow.StepResult{NextStep: StepCancelled}
    }
    return workflow.StepResult{}
}
```

**HandleContact** — user shares a phone contact:

```go
func (s *PhoneStep) HandleContact(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
    contact := c.EffectiveMessage.Contact
    if contact == nil {
        return workflow.StepResult{}
    }
    return workflow.StepResult{
        NextStep:    StepValidate,
        UpdateState: map[string]any{"phone": contact.PhoneNumber},
    }
}
```

### 5. Register the step

In your workflow's `registerSteps()`:

```go
func (w *MyWorkflow) registerSteps() {
    w.steps[StepGreeting] = NewGreetingStep()
    // ...
}
```

## StepResult Reference

```go
type StepResult struct {
    NextStep    StepID         // step to transition to (empty = stay)
    UpdateState map[string]any // key-value pairs to merge into state
    Complete    bool           // true = workflow finished, state deleted
    Error       error          // non-nil = abort with error
}
```

| Return value | Behavior |
|---|---|
| `StepResult{}` | Stay on current step, wait for user input |
| `StepResult{NextStep: StepX}` | Transition to step X |
| `StepResult{NextStep: StepX, UpdateState: map[string]any{"key": val}}` | Save data, then transition |
| `StepResult{Complete: true}` | End workflow, delete state from DB |
| `StepResult{Error: err}` | Log error and abort |

## Creating a New Workflow

### 1. Create a package

```
bot/workflows/myworkflow/
  workflow.go   # workflow definition
  steps.go      # step implementations
```

### 2. Define workflow.go

```go
package myworkflow

import (
    "DarkCS/bot/workflow"
    "log/slog"
)

const (
    WorkflowID workflow.WorkflowID = "my_workflow"
)

// Step IDs
const (
    StepFirst  workflow.StepID = "first"
    StepSecond workflow.StepID = "second"
    StepDone   workflow.StepID = "done"
)

// State data keys
const (
    KeySomeValue = "some_value"
)

// Define service interfaces your steps need
type SomeService interface {
    DoSomething(ctx context.Context) error
}

type MyWorkflow struct {
    steps   map[workflow.StepID]workflow.Step
    service SomeService
    log     *slog.Logger
}

func NewMyWorkflow(service SomeService, log *slog.Logger) *MyWorkflow {
    w := &MyWorkflow{
        steps:   make(map[workflow.StepID]workflow.Step),
        service: service,
        log:     log,
    }
    w.registerSteps()
    return w
}

func (w *MyWorkflow) ID() workflow.WorkflowID {
    return WorkflowID
}

func (w *MyWorkflow) InitialStep() workflow.StepID {
    return StepFirst
}

func (w *MyWorkflow) GetStep(id workflow.StepID) (workflow.Step, bool) {
    step, ok := w.steps[id]
    return step, ok
}

func (w *MyWorkflow) Steps() []workflow.Step {
    steps := make([]workflow.Step, 0, len(w.steps))
    for _, step := range w.steps {
        steps = append(steps, step)
    }
    return steps
}

func (w *MyWorkflow) registerSteps() {
    w.steps[StepFirst] = NewFirstStep()
    w.steps[StepSecond] = NewSecondStep(w.service)
    w.steps[StepDone] = NewDoneStep()
}
```

### 3. Implement steps.go

Follow the step creation guide above. Use `BaseStep` embedding.

### 4. Register the workflow with the engine

In your bot initialization code (where the engine is created):

```go
myWf := myworkflow.NewMyWorkflow(someService, log)
workflowEngine.RegisterWorkflow(myWf)
```

### 5. Start the workflow

From a bot handler:

```go
err := workflowEngine.StartWorkflow(ctx, bot, userID, chatID, myworkflow.WorkflowID, deepLink)
```

## Step Types

### Interactive steps (wait for user input)

- Send a message or keyboard in `Enter()`
- Return `workflow.StepResult{}` from `Enter()`
- Handle user response in `HandleMessage`, `HandleCallback`, or `HandleContact`

### Processing steps (auto-transition)

- Perform logic in `Enter()` (API calls, validation, routing)
- Return `workflow.StepResult{NextStep: ...}` from `Enter()`
- No need to implement event handlers — `BaseStep` defaults are sufficient

### Terminal steps (end the workflow)

- Send a final message in `Enter()`
- Return `workflow.StepResult{Complete: true}` from `Enter()`

## UserState API

Access collected data inside any step via `state`:

```go
state.GetString("key")   // returns "" if missing
state.GetInt("key")      // returns 0 if missing
state.GetBool("key")     // returns false if missing
state.Set("key", value)  // set directly (prefer UpdateState in StepResult)
```

Prefer returning `UpdateState` in `StepResult` over calling `state.Set()` directly — the engine merges and persists it automatically.

State also provides:
- `state.ChatID` / `state.UserID` — Telegram IDs
- `state.DeepLink` — parsed deep link data (`*DeepLinkData`)
- `state.Pagination` — pagination state for paginated lists

## UI Helpers

Available in `bot/workflow/ui`:

```go
ui.YesNoKeyboard("Yes", "No")                          // inline Yes/No
ui.ConfirmCancelKeyboard("Confirm", "Cancel")           // inline Confirm/Cancel
ui.ContactRequestKeyboard("Share phone")                // reply keyboard for contact
ui.RemoveKeyboard()                                     // remove reply keyboard
ui.SingleButtonKeyboard("Click me", "wf:action:value")  // single inline button
ui.SelectionKeyboard(items)                             // list of selectable items
ui.PaginatedList(items, currentPage, totalPages)        // paginated item list
ui.MainMenuKeyboard(rows)                               // grid of menu buttons
```

## Callback Data Format

All workflow callbacks use the prefix `wf:`:

```
wf:yes           — Yes button
wf:no            — No button
wf:confirm       — Confirm button
wf:cancel        — Cancel button
wf:select:<id>   — Item selection
wf:page:<num>    — Pagination
wf:menu:<id>     — Menu item
wf:noop          — No operation (e.g. page indicator)
```

Parse with `workflow.ParseCallback(data)`, build with `workflow.BuildCallback(action, value)`.

## Auto-Transition Chain

The engine supports chaining multiple auto-transitions in a single user action. For example, after a user shares their phone:

```
[user shares contact]
  → RequestPhoneStep.HandleContact() returns NextStep: ValidatePhone
  → ValidatePhoneStep.Enter() validates, returns NextStep: CheckUser
  → CheckUserStep.Enter() checks DB, returns NextStep: RequestName
  → RequestNameStep.Enter() sends "What is your name?", returns StepResult{}
  [stops here, waits for input]
```

The engine loops up to 20 transitions to prevent infinite loops. If a step's `Enter()` returns an empty `NextStep`, the chain stops and the engine waits for user input.
