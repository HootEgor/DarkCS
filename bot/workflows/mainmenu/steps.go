package mainmenu

import (
	"context"
	"fmt"
	"strings"

	"DarkCS/bot/workflow"
	"DarkCS/bot/workflow/ui"
	"DarkCS/entity"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// BaseStep provides common functionality for all steps.
type BaseStep struct {
	id workflow.StepID
}

func (s *BaseStep) ID() workflow.StepID {
	return s.id
}

func (s *BaseStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	return workflow.StepResult{}
}

func (s *BaseStep) HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
	return workflow.StepResult{}
}

func (s *BaseStep) HandleCallback(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState, data string) workflow.StepResult {
	return workflow.StepResult{}
}

func (s *BaseStep) HandleContact(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
	return workflow.StepResult{}
}

// MainMenuStep - Show main menu with reply keyboard
type MainMenuStep struct {
	BaseStep
}

func NewMainMenuStep() *MainMenuStep {
	return &MainMenuStep{BaseStep: BaseStep{id: StepMainMenu}}
}

func (s *MainMenuStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	keyboard := ui.ReplyKeyboard([][]string{
		{BtnMyOffice, BtnServiceRate},
		{BtnOrderStatus},
		{BtnAIConsultant, BtnMakeOrder},
	})

	_, err := b.SendMessage(state.ChatID, "Натисніть на потрібний варіант, щоб перейти у бажаний розділ 👇", &tgbotapi.SendMessageOpts{
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return workflow.StepResult{Error: err}
	}
	return workflow.StepResult{}
}

func (s *MainMenuStep) HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
	text := strings.TrimSpace(c.EffectiveMessage.Text)

	switch text {
	case BtnMyOffice:
		return workflow.StepResult{NextStep: StepMyOffice}
	case BtnServiceRate:
		return workflow.StepResult{NextStep: StepServiceRate}
	case BtnOrderStatus:
		return workflow.StepResult{NextStep: StepCurrentOrder}
	case BtnAIConsultant:
		return workflow.StepResult{NextStep: StepAIConsultant}
	case BtnMakeOrder:
		return workflow.StepResult{NextStep: StepMakeOrder}
	}

	return workflow.StepResult{}
}

// MyOfficeStep - Sub-menu with order options
type MyOfficeStep struct {
	BaseStep
}

func NewMyOfficeStep() *MyOfficeStep {
	return &MyOfficeStep{BaseStep: BaseStep{id: StepMyOffice}}
}

func (s *MyOfficeStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	keyboard := ui.ReplyKeyboard([][]string{
		{BtnCurrentOrder, BtnCompletedOrders},
		{BtnBack},
	})

	_, err := b.SendMessage(state.ChatID, "Що саме цікавить?", &tgbotapi.SendMessageOpts{
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return workflow.StepResult{Error: err}
	}
	return workflow.StepResult{}
}

func (s *MyOfficeStep) HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
	text := strings.TrimSpace(c.EffectiveMessage.Text)

	switch text {
	case BtnCurrentOrder:
		return workflow.StepResult{NextStep: StepCurrentOrder}
	case BtnCompletedOrders:
		return workflow.StepResult{NextStep: StepCompletedOrders}
	case BtnBack:
		return workflow.StepResult{NextStep: StepMainMenu}
	}

	return workflow.StepResult{}
}

// CurrentOrderStep - Show active orders
type CurrentOrderStep struct {
	BaseStep
	authService AuthService
	zohoService ZohoService
}

func NewCurrentOrderStep(authService AuthService, zohoService ZohoService) *CurrentOrderStep {
	return &CurrentOrderStep{
		BaseStep:    BaseStep{id: StepCurrentOrder},
		authService: authService,
		zohoService: zohoService,
	}
}

func (s *CurrentOrderStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	// Get user
	user, err := s.authService.GetUser("", "", state.UserID)
	if err != nil || user == nil {
		b.SendMessage(state.ChatID, "Не вдалося отримати інформацію про користувача.", nil)
		return workflow.StepResult{NextStep: StepMainMenu}
	}

	//log user info
	return workflow.StepResult{Error: fmt.Errorf("user info: %v", user)}

	// Get orders using stored ZohoId if available
	var orders []entity.OrderDetail
	if user.ZohoId != "" {
		orders, err = s.zohoService.GetOrdersDetailedByZohoId(user.ZohoId)
	} else {
		orders, err = s.zohoService.GetOrdersDetailed(*user.GetInfo())
	}
	if err != nil {
		b.SendMessage(state.ChatID, "Не вдалося отримати інформацію про замовлення.", nil)
		return workflow.StepResult{NextStep: StepMainMenu}
	}

	// Find first active order
	var activeOrder *entity.OrderDetail
	for _, order := range orders {
		if order.IsActive() {
			activeOrder = &order
			break
		}
	}

	if activeOrder == nil {
		b.SendMessage(state.ChatID, "У вас немає активних замовлень.", nil)
		return workflow.StepResult{NextStep: StepMainMenu}
	}

	// Format order message
	msg := formatOrderMessage(activeOrder, user.Name)

	keyboard := ui.OrderProductsButton(activeOrder.ID, "Товари")
	b.SendMessage(state.ChatID, msg, &tgbotapi.SendMessageOpts{
		ParseMode:   "HTML",
		ReplyMarkup: keyboard,
	})

	// Stay in this step to handle callback
	return workflow.StepResult{}
}

func (s *CurrentOrderStep) HandleCallback(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState, data string) workflow.StepResult {
	cb := workflow.ParseCallback(data)
	if cb == nil {
		return workflow.StepResult{}
	}

	c.CallbackQuery.Answer(b, nil)

	if cb.IsProducts() {
		orderID := cb.ProductsOrderID()
		products, err := s.zohoService.GetOrderProducts(orderID)
		if err != nil {
			b.SendMessage(state.ChatID, "Не вдалося отримати товари замовлення.", nil)
			return workflow.StepResult{NextStep: StepMainMenu}
		}

		b.SendMessage(state.ChatID, products, nil)
		return workflow.StepResult{NextStep: StepMainMenu}
	}

	return workflow.StepResult{NextStep: StepMainMenu}
}

func (s *CurrentOrderStep) HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
	text := strings.TrimSpace(c.EffectiveMessage.Text)
	if text == BtnBack {
		return workflow.StepResult{NextStep: StepMainMenu}
	}
	return workflow.StepResult{NextStep: StepMainMenu}
}

// CompletedOrdersStep - Show last 3 completed orders
type CompletedOrdersStep struct {
	BaseStep
	authService AuthService
	zohoService ZohoService
}

func NewCompletedOrdersStep(authService AuthService, zohoService ZohoService) *CompletedOrdersStep {
	return &CompletedOrdersStep{
		BaseStep:    BaseStep{id: StepCompletedOrders},
		authService: authService,
		zohoService: zohoService,
	}
}

func (s *CompletedOrdersStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	// Get user
	user, err := s.authService.GetUser("", "", state.UserID)
	if err != nil || user == nil {
		b.SendMessage(state.ChatID, "Не вдалося отримати інформацію про користувача.", nil)
		return workflow.StepResult{NextStep: StepMyOffice}
	}

	// Get orders using stored ZohoId if available
	var orders []entity.OrderDetail
	if user.ZohoId != "" {
		orders, err = s.zohoService.GetOrdersDetailedByZohoId(user.ZohoId)
	} else {
		orders, err = s.zohoService.GetOrdersDetailed(*user.GetInfo())
	}
	if err != nil {
		b.SendMessage(state.ChatID, "Не вдалося отримати інформацію про замовлення.", nil)
		return workflow.StepResult{NextStep: StepMyOffice}
	}

	// Filter completed orders (not active)
	var completedOrders []entity.OrderDetail
	for _, order := range orders {
		if !order.IsActive() {
			completedOrders = append(completedOrders, order)
		}
	}

	if len(completedOrders) == 0 {
		b.SendMessage(state.ChatID, "У вас немає виконаних замовлень.", nil)
		return workflow.StepResult{NextStep: StepMyOffice}
	}

	// Limit to last 3
	if len(completedOrders) > 3 {
		completedOrders = completedOrders[:3]
	}

	// Send each completed order with formatted message
	for i, order := range completedOrders {
		msg := formatOrderMessageNumbered(&order, user.Name, i+1)

		keyboard := ui.OrderProductsButton(order.ID, "Товари")
		b.SendMessage(state.ChatID, msg, &tgbotapi.SendMessageOpts{
			ParseMode:   "HTML",
			ReplyMarkup: keyboard,
		})
	}

	// Stay in this step to handle callbacks
	return workflow.StepResult{}
}

func (s *CompletedOrdersStep) HandleCallback(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState, data string) workflow.StepResult {
	cb := workflow.ParseCallback(data)
	if cb == nil {
		return workflow.StepResult{}
	}

	c.CallbackQuery.Answer(b, nil)

	if cb.IsProducts() {
		orderID := cb.ProductsOrderID()
		products, err := s.zohoService.GetOrderProducts(orderID)
		if err != nil {
			b.SendMessage(state.ChatID, "Не вдалося отримати товари замовлення.", nil)
			return workflow.StepResult{NextStep: StepMyOffice}
		}

		b.SendMessage(state.ChatID, products, nil)
		return workflow.StepResult{NextStep: StepMyOffice}
	}

	return workflow.StepResult{NextStep: StepMyOffice}
}

func (s *CompletedOrdersStep) HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
	text := strings.TrimSpace(c.EffectiveMessage.Text)
	if text == BtnBack {
		return workflow.StepResult{NextStep: StepMyOffice}
	}
	return workflow.StepResult{NextStep: StepMyOffice}
}

// ServiceRateStep - Rating 1-5 inline keyboard
type ServiceRateStep struct {
	BaseStep
	authService AuthService
	zohoService ZohoService
}

func NewServiceRateStep(authService AuthService, zohoService ZohoService) *ServiceRateStep {
	return &ServiceRateStep{
		BaseStep:    BaseStep{id: StepServiceRate},
		authService: authService,
		zohoService: zohoService,
	}
}

func (s *ServiceRateStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	// Get user
	user, err := s.authService.GetUser("", "", state.UserID)
	if err != nil || user == nil {
		b.SendMessage(state.ChatID, "Не вдалося отримати інформацію про користувача.", nil)
		return workflow.StepResult{NextStep: StepMainMenu}
	}

	// Get orders using stored ZohoId if available
	var orders []entity.OrderDetail
	if user.ZohoId != "" {
		orders, err = s.zohoService.GetOrdersDetailedByZohoId(user.ZohoId)
	} else {
		orders, err = s.zohoService.GetOrdersDetailed(*user.GetInfo())
	}
	if err != nil || len(orders) == 0 {
		b.SendMessage(state.ChatID, "У вас немає замовлень для оцінки.", nil)
		return workflow.StepResult{NextStep: StepMainMenu}
	}

	// Find latest order (first in list)
	latestOrder := orders[0]

	// Store order info for rating
	state.Set("rating_order_number", latestOrder.ID)

	keyboard := ui.RatingKeyboard()
	msg := "Як вам сервіс? 🙌\nЗалиште, будь ласка, оцінку — це допоможе нам ставати кращими.\n\n<i> Ваш відгук важливий для нас!</i>"

	_, err = b.SendMessage(state.ChatID, msg, &tgbotapi.SendMessageOpts{
		ParseMode:   "HTML",
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return workflow.StepResult{Error: err}
	}
	return workflow.StepResult{}
}

func (s *ServiceRateStep) HandleCallback(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState, data string) workflow.StepResult {
	cb := workflow.ParseCallback(data)
	if cb == nil {
		return workflow.StepResult{}
	}

	c.CallbackQuery.Answer(b, nil)

	if cb.IsRate() {
		rating := cb.RatingValue()
		if rating < 1 || rating > 5 {
			return workflow.StepResult{}
		}

		// Get user to get contact ID
		user, err := s.authService.GetUser("", "", state.UserID)
		if err != nil || user == nil {
			b.SendMessage(state.ChatID, "Не вдалося отримати інформацію про користувача.", nil)
			return workflow.StepResult{NextStep: StepMainMenu}
		}

		// Use stored ZohoId if available, otherwise create contact
		contactID := user.ZohoId
		if contactID == "" {
			contactID, err = s.zohoService.CreateContact(user)
			if err != nil {
				b.SendMessage(state.ChatID, "Не вдалося зберегти оцінку.", nil)
				return workflow.StepResult{NextStep: StepMainMenu}
			}
		}

		orderNumber := state.GetString("rating_order_number")

		// Create rating
		serviceRating := entity.ServiceRating{
			OrderNumber:   orderNumber,
			ContactID:     contactID,
			ServiceRating: rating,
		}

		err = s.zohoService.CreateRating(serviceRating)
		if err != nil {
			b.SendMessage(state.ChatID, "Не вдалося зберегти оцінку. Спробуйте пізніше.", nil)
			return workflow.StepResult{NextStep: StepMainMenu}
		}

		b.SendMessage(state.ChatID, "<b>Ваша оцінка успішно створена! 🎉</b>\n\nДякуємо за ваш відгук!", &tgbotapi.SendMessageOpts{
			ParseMode: "HTML",
		})
		return workflow.StepResult{NextStep: StepMainMenu}
	}

	return workflow.StepResult{}
}

func (s *ServiceRateStep) HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
	text := strings.TrimSpace(c.EffectiveMessage.Text)
	if text == BtnBack {
		return workflow.StepResult{NextStep: StepMainMenu}
	}
	return workflow.StepResult{}
}

// AIConsultantStep - AI mode with back button
type AIConsultantStep struct {
	BaseStep
	authService AuthService
	aiService   AIService
}

func NewAIConsultantStep(authService AuthService, aiService AIService) *AIConsultantStep {
	return &AIConsultantStep{
		BaseStep:    BaseStep{id: StepAIConsultant},
		authService: authService,
		aiService:   aiService,
	}
}

func (s *AIConsultantStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	keyboard := ui.BackButtonKeyboard(BtnBack)

	_, err := b.SendMessage(state.ChatID, "Привіт! Я — консультант бренду DARK 🖤\nДопоможу з вибором товарів, проконсультую щодо продукції та оформлення замовлення.", &tgbotapi.SendMessageOpts{
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return workflow.StepResult{Error: err}
	}
	return workflow.StepResult{}
}

func (s *AIConsultantStep) HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
	text := strings.TrimSpace(c.EffectiveMessage.Text)

	if text == BtnBack {
		return workflow.StepResult{NextStep: StepMainMenu}
	}

	// Get user
	user, err := s.authService.GetUser("", "", state.UserID)
	if err != nil || user == nil {
		b.SendMessage(state.ChatID, "Не вдалося отримати інформацію про користувача.", nil)
		return workflow.StepResult{}
	}

	// Send typing action
	b.SendChatAction(state.ChatID, "typing", nil)

	// Get AI response
	response, err := s.aiService.ProcessUserRequest(user, text)
	if err != nil {
		b.SendMessage(state.ChatID, "Виникла помилка при обробці запиту. Спробуйте ще раз.", nil)
		return workflow.StepResult{}
	}

	b.SendMessage(state.ChatID, response.Text, nil)
	return workflow.StepResult{}
}

// MakeOrderStep - AI mode for making orders
type MakeOrderStep struct {
	BaseStep
	authService AuthService
	aiService   AIService
}

func NewMakeOrderStep(authService AuthService, aiService AIService) *MakeOrderStep {
	return &MakeOrderStep{
		BaseStep:    BaseStep{id: StepMakeOrder},
		authService: authService,
		aiService:   aiService,
	}
}

func (s *MakeOrderStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	keyboard := ui.BackButtonKeyboard(BtnBack)

	_, err := b.SendMessage(state.ChatID, "Готові оформити замовлення!", &tgbotapi.SendMessageOpts{
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return workflow.StepResult{Error: err}
	}
	return workflow.StepResult{}
}

func (s *MakeOrderStep) HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
	text := strings.TrimSpace(c.EffectiveMessage.Text)

	if text == BtnBack {
		return workflow.StepResult{NextStep: StepMainMenu}
	}

	// Get user
	user, err := s.authService.GetUser("", "", state.UserID)
	if err != nil || user == nil {
		b.SendMessage(state.ChatID, "Не вдалося отримати інформацію про користувача.", nil)
		return workflow.StepResult{}
	}

	// Send typing action
	b.SendChatAction(state.ChatID, "typing", nil)

	// Get AI response - route to OrderManager
	response, err := s.aiService.ProcessUserRequest(user, text)
	if err != nil {
		b.SendMessage(state.ChatID, "Виникла помилка при обробці запиту. Спробуйте ще раз.", nil)
		return workflow.StepResult{}
	}

	b.SendMessage(state.ChatID, response.Text, nil)
	return workflow.StepResult{}
}

// formatOrderMessage formats an order for display without number prefix (HTML format).
func formatOrderMessage(order *entity.OrderDetail, customerName string) string {
	msg := fmt.Sprintf("<b>Замовник:</b> %s\n<b>Статус:</b> %s", customerName, order.Status)

	if order.Subject != "" {
		msg += fmt.Sprintf("\n<b>Номер замовлення:</b> %s", order.Subject)
	}

	if order.TTN != "" {
		msg += fmt.Sprintf("\n<b>ТТН:</b> <a href=\"https://novaposhta.ua/tracking/%s\">%s</a>", order.TTN, order.TTN)
	}

	return msg
}

// formatOrderMessageNumbered formats an order for display with order number prefix (HTML format).
func formatOrderMessageNumbered(order *entity.OrderDetail, customerName string, orderNum int) string {
	msg := fmt.Sprintf("<b>Замовлення №%d</b>\n\n<b>Замовник:</b> %s\n<b>Статус:</b> %s", orderNum, customerName, order.Status)

	if order.Subject != "" {
		msg += fmt.Sprintf("\n<b>Номер замовлення:</b> %s", order.Subject)
	}

	if order.TTN != "" {
		msg += fmt.Sprintf("\n<b>ТТН:</b> <a href=\"https://novaposhta.ua/tracking/%s\">%s</a>", order.TTN, order.TTN)
	}

	return msg
}
