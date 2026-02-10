package mainmenu

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"DarkCS/bot/chat"
	"DarkCS/entity"
)

// mainMenuButtons defines the main menu layout.
var mainMenuButtons = [][]chat.MenuButton{
	{{Text: BtnMyOffice}, {Text: BtnServiceRate}},
	{{Text: BtnOrderStatus}},
	{{Text: BtnAIConsultant}, {Text: BtnMakeOrder}},
}

// myOfficeButtons defines the "my office" sub-menu layout.
var myOfficeButtons = [][]chat.MenuButton{
	{{Text: BtnCurrentOrder}, {Text: BtnCompletedOrders}},
	{{Text: BtnBack}},
}

// getUser resolves a user from state depending on platform.
func getUser(state *chat.ChatState, authService AuthService) (*entity.User, error) {
	if state.Platform == "instagram" {
		user, err := authService.GetUserByInstagramId(state.UserID)
		if err == nil && user != nil {
			return user, nil
		}
	}
	if state.Platform == "telegram" {
		telegramId, err := strconv.ParseInt(state.UserID, 10, 64)
		if err == nil && telegramId != 0 {
			user, err := authService.GetUser("", "", telegramId)
			if err == nil && user != nil {
				return user, nil
			}
		}
	}
	// Fallback: try by phone stored in state
	phone := state.GetString("phone")
	if phone != "" {
		return authService.GetUser("", phone, 0)
	}
	return nil, fmt.Errorf("user not found")
}

// MainMenuStep — Show main menu as numbered text list.
type MainMenuStep struct{}

func (s *MainMenuStep) ID() chat.StepID { return StepMainMenu }

func (s *MainMenuStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	err := m.SendMenu(state.ChatID, "Натисніть на потрібний варіант, щоб перейти у бажаний розділ 👇", mainMenuButtons)
	if err != nil {
		return chat.StepResult{Error: err}
	}
	return chat.StepResult{}
}

func (s *MainMenuStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	text := strings.TrimSpace(input.Text)

	// Try to match by exact button text first
	switch text {
	case BtnMyOffice:
		return chat.StepResult{NextStep: StepMyOffice}
	case BtnServiceRate:
		return chat.StepResult{NextStep: StepServiceRate}
	case BtnOrderStatus:
		return chat.StepResult{NextStep: StepCurrentOrder}
	case BtnAIConsultant:
		return chat.StepResult{NextStep: StepAIConsultant}
	case BtnMakeOrder:
		return chat.StepResult{NextStep: StepMakeOrder}
	}

	// Try matching by number
	matched := chat.MatchNumberToOption(text, mainMenuButtons)
	switch matched {
	case BtnMyOffice:
		return chat.StepResult{NextStep: StepMyOffice}
	case BtnServiceRate:
		return chat.StepResult{NextStep: StepServiceRate}
	case BtnOrderStatus:
		return chat.StepResult{NextStep: StepCurrentOrder}
	case BtnAIConsultant:
		return chat.StepResult{NextStep: StepAIConsultant}
	case BtnMakeOrder:
		return chat.StepResult{NextStep: StepMakeOrder}
	}

	return chat.StepResult{}
}

// MyOfficeStep — Sub-menu with order options.
type MyOfficeStep struct{}

func (s *MyOfficeStep) ID() chat.StepID { return StepMyOffice }

func (s *MyOfficeStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	err := m.SendMenu(state.ChatID, "Що саме цікавить?", myOfficeButtons)
	if err != nil {
		return chat.StepResult{Error: err}
	}
	return chat.StepResult{}
}

func (s *MyOfficeStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	text := strings.TrimSpace(input.Text)

	switch text {
	case BtnCurrentOrder:
		return chat.StepResult{NextStep: StepCurrentOrder}
	case BtnCompletedOrders:
		return chat.StepResult{NextStep: StepCompletedOrders}
	case BtnBack:
		return chat.StepResult{NextStep: StepMainMenu}
	}

	matched := chat.MatchNumberToOption(text, myOfficeButtons)
	switch matched {
	case BtnCurrentOrder:
		return chat.StepResult{NextStep: StepCurrentOrder}
	case BtnCompletedOrders:
		return chat.StepResult{NextStep: StepCompletedOrders}
	case BtnBack:
		return chat.StepResult{NextStep: StepMainMenu}
	}

	return chat.StepResult{}
}

// CurrentOrderStep — Show active orders.
type CurrentOrderStep struct {
	authService AuthService
	zohoService ZohoService
}

func (s *CurrentOrderStep) ID() chat.StepID { return StepCurrentOrder }

func (s *CurrentOrderStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	user, err := getUser(state, s.authService)
	if err != nil || user == nil {
		_ = m.SendText(state.ChatID, "Не вдалося отримати інформацію про користувача.")
		return chat.StepResult{NextStep: StepMainMenu}
	}

	var orders []entity.OrderDetail
	if user.ZohoId != "" {
		orders, err = s.zohoService.GetOrdersDetailedByZohoId(user.ZohoId)
	} else {
		orders, err = s.zohoService.GetOrdersDetailed(*user.GetInfo())
	}
	if err != nil {
		_ = m.SendText(state.ChatID, "Не вдалося отримати інформацію про замовлення.")
		return chat.StepResult{NextStep: StepMainMenu}
	}

	var activeOrder *entity.OrderDetail
	for _, order := range orders {
		if order.IsActive() {
			activeOrder = &order
			break
		}
	}

	if activeOrder == nil {
		_ = m.SendText(state.ChatID, "У вас немає активних замовлень.")
		return chat.StepResult{NextStep: StepMainMenu}
	}

	msg := formatOrderMessage(activeOrder, user.Name, state.Platform)
	_ = m.SendText(state.ChatID, msg)
	return chat.StepResult{NextStep: StepMainMenu}
}

func (s *CurrentOrderStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	return chat.StepResult{NextStep: StepMainMenu}
}

// CompletedOrdersStep — Show last 3 completed orders.
type CompletedOrdersStep struct {
	authService AuthService
	zohoService ZohoService
}

func (s *CompletedOrdersStep) ID() chat.StepID { return StepCompletedOrders }

func (s *CompletedOrdersStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	user, err := getUser(state, s.authService)
	if err != nil || user == nil {
		_ = m.SendText(state.ChatID, "Не вдалося отримати інформацію про користувача.")
		return chat.StepResult{NextStep: StepMyOffice}
	}

	var orders []entity.OrderDetail
	if user.ZohoId != "" {
		orders, err = s.zohoService.GetOrdersDetailedByZohoId(user.ZohoId)
	} else {
		orders, err = s.zohoService.GetOrdersDetailed(*user.GetInfo())
	}
	if err != nil {
		_ = m.SendText(state.ChatID, "Не вдалося отримати інформацію про замовлення.")
		return chat.StepResult{NextStep: StepMyOffice}
	}

	var completedOrders []entity.OrderDetail
	for _, order := range orders {
		if !order.IsActive() {
			completedOrders = append(completedOrders, order)
		}
	}

	if len(completedOrders) == 0 {
		_ = m.SendText(state.ChatID, "У вас немає виконаних замовлень.")
		return chat.StepResult{NextStep: StepMyOffice}
	}

	if len(completedOrders) > 3 {
		completedOrders = completedOrders[:3]
	}

	for i, order := range completedOrders {
		msg := formatOrderMessageNumbered(&order, user.Name, i+1, state.Platform)
		_ = m.SendText(state.ChatID, msg)
	}

	return chat.StepResult{NextStep: StepMyOffice}
}

func (s *CompletedOrdersStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	return chat.StepResult{NextStep: StepMyOffice}
}

// ServiceRateStep — Rating 1-5.
type ServiceRateStep struct {
	authService AuthService
	zohoService ZohoService
}

func (s *ServiceRateStep) ID() chat.StepID { return StepServiceRate }

func (s *ServiceRateStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	user, err := getUser(state, s.authService)
	if err != nil || user == nil {
		_ = m.SendText(state.ChatID, "Не вдалося отримати інформацію про користувача.")
		return chat.StepResult{NextStep: StepMainMenu}
	}

	var orders []entity.OrderDetail
	if user.ZohoId != "" {
		orders, err = s.zohoService.GetOrdersDetailedByZohoId(user.ZohoId)
	} else {
		orders, err = s.zohoService.GetOrdersDetailed(*user.GetInfo())
	}
	if err != nil || len(orders) == 0 {
		_ = m.SendText(state.ChatID, "У вас немає замовлень для оцінки.")
		return chat.StepResult{NextStep: StepMainMenu}
	}

	latestOrder := orders[0]
	state.Set("rating_order_number", latestOrder.ID)

	buttons := []chat.InlineButton{
		{Text: "1", Data: "rate:1"},
		{Text: "2", Data: "rate:2"},
		{Text: "3", Data: "rate:3"},
		{Text: "4", Data: "rate:4"},
		{Text: "5", Data: "rate:5"},
	}

	msg := "Як вам сервіс? 🙌\nЗалиште, будь ласка, оцінку — це допоможе нам ставати кращими.\n\nВаш відгук важливий для нас!"
	_ = m.SendInlineOptions(state.ChatID, msg, buttons)
	return chat.StepResult{}
}

func (s *ServiceRateStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	data := input.CallbackData
	if data == "" {
		// Try matching numbered text input
		buttons := []chat.InlineButton{
			{Text: "1", Data: "rate:1"},
			{Text: "2", Data: "rate:2"},
			{Text: "3", Data: "rate:3"},
			{Text: "4", Data: "rate:4"},
			{Text: "5", Data: "rate:5"},
		}
		data = chat.MatchNumberToInline(input.Text, buttons)
	}

	if !strings.HasPrefix(data, "rate:") {
		if strings.TrimSpace(input.Text) == BtnBack {
			return chat.StepResult{NextStep: StepMainMenu}
		}
		return chat.StepResult{}
	}

	var rating int
	fmt.Sscanf(data, "rate:%d", &rating)
	if rating < 1 || rating > 5 {
		return chat.StepResult{}
	}

	user, err := getUser(state, s.authService)
	if err != nil || user == nil {
		_ = m.SendText(state.ChatID, "Не вдалося отримати інформацію про користувача.")
		return chat.StepResult{NextStep: StepMainMenu}
	}

	contactID := user.ZohoId
	if contactID == "" {
		contactID, err = s.zohoService.CreateContact(user)
		if err != nil {
			_ = m.SendText(state.ChatID, "Не вдалося зберегти оцінку.")
			return chat.StepResult{NextStep: StepMainMenu}
		}
	}

	orderNumber := state.GetString("rating_order_number")
	serviceRating := entity.ServiceRating{
		OrderNumber:   orderNumber,
		ContactID:     contactID,
		ServiceRating: rating,
	}

	err = s.zohoService.CreateRating(serviceRating)
	if err != nil {
		_ = m.SendText(state.ChatID, "Не вдалося зберегти оцінку. Спробуйте пізніше.")
		return chat.StepResult{NextStep: StepMainMenu}
	}

	_ = m.SendText(state.ChatID, "Ваша оцінка успішно створена! 🎉\n\nДякуємо за ваш відгук!")
	return chat.StepResult{NextStep: StepMainMenu}
}

// AIConsultantStep — AI mode.
type AIConsultantStep struct {
	authService AuthService
	aiService   AIService
}

func (s *AIConsultantStep) ID() chat.StepID { return StepAIConsultant }

func (s *AIConsultantStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	_ = m.SendText(state.ChatID, "Привіт! Я — консультант бренду DARK 🖤\nДопоможу з вибором товарів, проконсультую щодо продукції та оформлення замовлення.\n\nНапишіть \"назад\" щоб повернутися в меню.")
	return chat.StepResult{}
}

func (s *AIConsultantStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	text := strings.TrimSpace(input.Text)

	if text == BtnBack || strings.EqualFold(text, "назад") {
		return chat.StepResult{NextStep: StepMainMenu}
	}

	user, err := getUser(state, s.authService)
	if err != nil || user == nil {
		_ = m.SendText(state.ChatID, "Не вдалося отримати інформацію про користувача.")
		return chat.StepResult{}
	}

	_ = m.SendTyping(state.ChatID)

	response, err := s.aiService.ProcessUserRequest(user, text)
	if err != nil {
		_ = m.SendText(state.ChatID, "Виникла помилка при обробці запиту. Спробуйте ще раз.")
		return chat.StepResult{}
	}

	_ = m.SendText(state.ChatID, response.Text)
	return chat.StepResult{}
}

// MakeOrderStep — AI mode for making orders.
type MakeOrderStep struct {
	authService AuthService
	aiService   AIService
}

func (s *MakeOrderStep) ID() chat.StepID { return StepMakeOrder }

func (s *MakeOrderStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	_ = m.SendText(state.ChatID, "Готові оформити замовлення!\n\nНапишіть \"назад\" щоб повернутися в меню.")
	return chat.StepResult{}
}

func (s *MakeOrderStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	text := strings.TrimSpace(input.Text)

	if text == BtnBack || strings.EqualFold(text, "назад") {
		return chat.StepResult{NextStep: StepMainMenu}
	}

	user, err := getUser(state, s.authService)
	if err != nil || user == nil {
		_ = m.SendText(state.ChatID, "Не вдалося отримати інформацію про користувача.")
		return chat.StepResult{}
	}

	_ = m.SendTyping(state.ChatID)

	response, err := s.aiService.ProcessUserRequest(user, text)
	if err != nil {
		_ = m.SendText(state.ChatID, "Виникла помилка при обробці запиту. Спробуйте ще раз.")
		return chat.StepResult{}
	}

	_ = m.SendText(state.ChatID, response.Text)
	return chat.StepResult{}
}

// formatOrderMessage formats an order for display.
// Telegram gets HTML links; other platforms get a plain URL on its own line.
func formatOrderMessage(order *entity.OrderDetail, customerName, platform string) string {
	msg := fmt.Sprintf("Замовник: %s\nСтатус: %s", customerName, order.Status)
	if order.Subject != "" {
		msg += fmt.Sprintf("\nНомер замовлення: %s", order.Subject)
	}
	if order.TTN != "" {
		msg += formatTTN(order.TTN, platform)
	}
	return msg
}

// formatOrderMessageNumbered formats an order with a number prefix.
func formatOrderMessageNumbered(order *entity.OrderDetail, customerName string, orderNum int, platform string) string {
	msg := fmt.Sprintf("Замовлення №%d\n\nЗамовник: %s\nСтатус: %s", orderNum, customerName, order.Status)
	if order.Subject != "" {
		msg += fmt.Sprintf("\nНомер замовлення: %s", order.Subject)
	}
	if order.TTN != "" {
		msg += formatTTN(order.TTN, platform)
	}
	return msg
}

func formatTTN(ttn, platform string) string {
	if platform == "telegram" {
		return fmt.Sprintf("\nТТН: <a href=\"https://novaposhta.ua/tracking/%s\">%s</a>", ttn, ttn)
	}
	return fmt.Sprintf("\nТТН: %s\nhttps://novaposhta.ua/tracking/%s", ttn, ttn)
}
