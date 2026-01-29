# Main Menu Workflow Implementation Plan

## Overview

Implement a Main Menu workflow for the Telegram bot that provides:
- "My office" sub-menu with order viewing
- "Service rate" functionality (1-5 rating to Zoho)
- "Order status" shortcut
- "AI consultant" and "Make order" AI integration modes

**Entry Point:** After onboarding completes (replaces current `StepMainMenu`)

---

## File Structure

### New Files

```
bot/workflows/mainmenu/
    workflow.go     # Workflow struct, constants, interfaces
    steps.go        # All step implementations
```

### Files to Modify

| File | Changes |
|------|---------|
| `bot/workflow/ui/keyboard.go` | Add reply keyboard helpers |
| `bot/workflow/callbacks.go` | Add `rate`, `products`, `back` actions |
| `impl/core/core.go` | Add `GetOrdersDetailed()`, `CreateRating()` to ZohoService |
| `internal/service/zoho/zoho-service.go` | Implement new methods |
| `entity/zoho-order.go` | Add `OrderDetail`, `ServiceRating` structs |
| `bot/workflows/onboarding/steps.go` | Modify `MainMenuStep` to start mainmenu workflow |
| `bot/userbot.go` | Register mainmenu workflow, handle workflow chaining |

---

## Step Definitions

```
StepMainMenu         # Main menu with reply keyboard
    |
    +-- StepMyOffice         # Sub-menu: current/completed orders
    |       +-- StepCurrentOrder     # Show active orders
    |       +-- StepCompletedOrders  # Show last 3 completed
    |
    +-- StepServiceRate      # Rating 1-5 inline keyboard -> Zoho API
    |
    +-- StepAIConsultant     # AI mode with back button
    |
    +-- StepMakeOrder        # AI mode (routes to OrderManager)
```

**"Order status"** is a shortcut to `StepCurrentOrder`.

---

## Keyboard Layouts

### Main Menu (Reply Keyboard)

```
Row 1: ["Мій кабінет", "Оцінка сервісу"]
Row 2: ["Статус замовлення"]
Row 3: ["AI консультант", "Зробити замовлення"]
```

### My Office Sub-Menu (Reply Keyboard)

```
Row 1: ["Поточне замовлення"]
Row 2: ["Виконані замовлення"]
Row 3: ["Назад"]
```

### AI Mode (Reply Keyboard)

```
Row 1: ["Назад"]
```

### Rating (Inline Keyboard)

```
[1] [2] [3] [4] [5]
```

### Order Products (Inline Keyboard)

```
[Товари]  (callback: wf:products:{orderID})
```

---

## Service Interfaces

### Extended ZohoService (`impl/core/core.go:77`)

```go
type ZohoService interface {
    // Existing
    CreateOrder(order *entity.Order) error
    GetOrders(userInfo entity.UserInfo) ([]entity.OrderStatus, error)
    GetOrderProducts(orderId string) (string, error)
    CreateContact(user *entity.User) (string, error)

    // New
    GetOrdersDetailed(userInfo entity.UserInfo) ([]entity.OrderDetail, error)
    CreateRating(rating entity.ServiceRating) error
}
```

### New Entities (`entity/zoho-order.go`)

```go
type OrderDetail struct {
    ID          string `json:"id"`
    OrderNumber string `json:"SO_Number"`
    Subject     string `json:"Subject"`
    Status      string `json:"Status"`
    ClientName  string `json:"Contact_Name"`
}

type ServiceRating struct {
    OrderNumber   string `json:"Sales_Orders_raiting"`
    ContactID     string `json:"Contact_raiting"`
    ServiceRating int    `json:"Servise_rating"`
}
```

### Mainmenu Workflow Interfaces (`bot/workflows/mainmenu/workflow.go`)

```go
type AuthService interface {
    GetUser(email, phone string, telegramId int64) (*entity.User, error)
}

type ZohoService interface {
    GetOrdersDetailed(userInfo entity.UserInfo) ([]entity.OrderDetail, error)
    GetOrderProducts(orderId string) (string, error)
    CreateRating(rating entity.ServiceRating) error
    CreateContact(user *entity.User) (string, error)
}

type AIService interface {
    ComposeResponse(user *entity.User, systemMsg, userMsg string) (entity.AiAnswer, error)
}
```

---

## Key Implementation Details

### 1. Onboarding -> MainMenu Transition

Modify `bot/workflows/onboarding/steps.go:435-444`:

```go
func (s *MainMenuStep) Enter(...) workflow.StepResult {
    // Send welcome message
    // Return Complete: true with UpdateState: {"next_workflow": "mainmenu"}
}
```

In `bot/userbot.go`, after workflow completes, check for `next_workflow` and start mainmenu workflow.

### 2. User Data for Zoho Queries

Use `AuthService.GetUser("", "", state.UserID)` to get user profile, then `user.GetInfo()` for Zoho API calls.

### 3. AI Integration

Direct integration via `AIService.ComposeResponse()`:
- Wrap existing `Core.ass.ComposeResponse()`
- Pass user's message as-is (preserve language)
- System message indicates Telegram bot mode

### 4. Rating API Call

```
POST https://www.zohoapis.eu/crm/v7/Raiting
Authorization: Bearer {token}
Body: {
    "data": [{
        "Sales_Orders_raiting": "{order_number}",
        "Contact_raiting": "{user_zoho_id}",
        "Servise_rating": {1-5}
    }]
}
```

### 5. Active Order Statuses

Filter for: `"Нове"`, `"Оброблення замовлення"`, `"Рахунок виставлено"`

---

## Implementation Sequence

### Phase 1: Infrastructure
1. Add callback actions to `bot/workflow/callbacks.go`
2. Add reply keyboard helpers to `bot/workflow/ui/keyboard.go`
3. Add `OrderDetail`, `ServiceRating` to `entity/zoho-order.go`

### Phase 2: ZohoService
4. Add interface methods to `impl/core/core.go`
5. Implement `GetOrdersDetailed()` in `internal/service/zoho/get-order.go`
6. Implement `CreateRating()` in `internal/service/zoho/zoho-service.go`

### Phase 3: Workflow
7. Create `bot/workflows/mainmenu/workflow.go`
8. Create `bot/workflows/mainmenu/steps.go` with all steps

### Phase 4: Integration
9. Modify onboarding `MainMenuStep` to trigger mainmenu workflow
10. Register mainmenu workflow in `bot/userbot.go`
11. Wire up services in main.go

---

## Verification

1. **Manual testing flow:**
   - Start bot with `/start`
   - Complete onboarding (phone, name)
   - Verify main menu appears
   - Test each button navigation
   - Test "Назад" returns to correct menu

2. **Order functionality:**
   - Test with user that has Zoho orders
   - Verify "Products" button shows order items
   - Test empty state (no orders)

3. **Rating:**
   - Select rating 1-5
   - Verify Zoho API call succeeds (check Zoho CRM)

4. **AI modes:**
   - Send message in AI consultant mode
   - Verify AI responds
   - Test "Назад" exits to main menu

---

## Critical Files Reference

- `bot/workflows/onboarding/steps.go:17-40` - BaseStep pattern to copy
- `bot/workflows/onboarding/steps.go:427-444` - Current MainMenuStep to modify
- `bot/workflow/ui/keyboard.go:32-42` - Reply keyboard pattern
- `impl/core/core.go:77-87` - ZohoService interface
- `internal/service/zoho/get-order.go` - Zoho API call patterns
