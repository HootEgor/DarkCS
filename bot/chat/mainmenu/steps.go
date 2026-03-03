package mainmenu

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"DarkCS/bot/chat"
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
)

const schoolsPerPage = 5

// SelectSchoolStep — Shows a paginated school selection when deep link type is "dl".
// Auto-skips to main menu if no deep link is present.
// On selection, the chosen school is saved to qr-stat for analytics.
type SelectSchoolStep struct {
	schoolRepo  SchoolRepository
	authService AuthService
	zohoService ZohoService
	qrStatRepo  QrStatRepository
}

func (s *SelectSchoolStep) ID() chat.StepID { return StepSelectSchool }

func (s *SelectSchoolStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	if state.GetString("deep_link_type") != "dl" {
		return chat.StepResult{NextStep: StepMainMenu}
	}

	schools, err := s.schoolRepo.GetAllActiveSchools(ctx)
	if err != nil || len(schools) == 0 {
		return chat.StepResult{NextStep: StepMainMenu}
	}

	rows := s.buildPage(schools, 0)
	_ = m.SendInlineGrid(state.ChatID, "Розкажи, будь ласка, з якої школи ти дізнався/дізналася про нас 🖤\n\nОберіть школу:", rows)
	return chat.StepResult{}
}

func (s *SelectSchoolStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	data := input.CallbackData

	// For text-only platforms: try to match number input
	if data == "" {
		schools, err := s.schoolRepo.GetAllActiveSchools(ctx)
		if err != nil || len(schools) == 0 {
			return chat.StepResult{NextStep: StepMainMenu}
		}
		page := state.GetInt("school_page")
		rows := s.buildPage(schools, page)
		data = chat.MatchNumberToInlineGrid(input.Text, rows)
	}

	if data == "" {
		return chat.StepResult{}
	}

	// Handle pagination — edit the existing message
	if strings.HasPrefix(data, "school_pg:") {
		pageStr := strings.TrimPrefix(data, "school_pg:")
		if pageStr == "noop" {
			return chat.StepResult{}
		}
		page, err := strconv.Atoi(pageStr)
		if err != nil {
			return chat.StepResult{}
		}

		schools, err := s.schoolRepo.GetAllActiveSchools(ctx)
		if err != nil || len(schools) == 0 {
			return chat.StepResult{NextStep: StepMainMenu}
		}

		rows := s.buildPage(schools, page)
		if input.MessageID != "" {
			_ = m.EditInlineGrid(state.ChatID, input.MessageID, "Оберіть школу:", rows)
		} else {
			_ = m.SendInlineGrid(state.ChatID, "Оберіть школу:", rows)
		}
		return chat.StepResult{
			UpdateState: map[string]any{"school_page": page},
		}
	}

	// Handle school selection
	if strings.HasPrefix(data, "school_sel:") {
		name := strings.TrimPrefix(data, "school_sel:")
		_ = m.SendText(state.ChatID, fmt.Sprintf(
			"Вітаємо, %s!\n\nОтримай -15%% на перше замовлення з промо-кодом *DARKSCHOOL* 🖤\nСкористайся протягом 14 днів на сайті 👉 riornails.com\n\nP.S. Твоя особиста знижка -7%% вже активна, і з часом може стати ще більшою ✨",
			name,
		))

		// Sync school to Zoho CRM
		if user, err := getUser(state, s.authService); err == nil && user != nil && user.ZohoId != "" {
			if err := s.zohoService.UpdateContactSchool(user.ZohoId, name); err != nil {
				slog.Warn("failed to update school in Zoho", slog.String("zoho_id", user.ZohoId), sl.Err(err))
			}
		}

		// Persist school choice to qr-stat analytics
		if err := s.qrStatRepo.SaveSchoolStat(state.Platform, state.UserID, name); err != nil {
			slog.Warn("failed to save school stat", slog.String("platform", state.Platform), slog.String("user_id", state.UserID), sl.Err(err))
		}

		return chat.StepResult{NextStep: StepMainMenu}
	}

	return chat.StepResult{}
}

// buildPage builds a page of inline buttons for school selection.
func (s *SelectSchoolStep) buildPage(schools []entity.School, page int) [][]chat.InlineButton {
	start := page * schoolsPerPage
	if start >= len(schools) {
		start = 0
		page = 0
	}
	end := start + schoolsPerPage
	if end > len(schools) {
		end = len(schools)
	}

	var rows [][]chat.InlineButton
	for _, school := range schools[start:end] {
		rows = append(rows, []chat.InlineButton{
			{Text: school.Name, Data: "school_sel:" + school.Name},
		})
	}

	// Navigation row: always [⬅️] [page/total] [➡️]
	totalPages := (len(schools) + schoolsPerPage - 1) / schoolsPerPage
	if totalPages > 1 {
		backData := "school_pg:noop"
		if page > 0 {
			backData = fmt.Sprintf("school_pg:%d", page-1)
		}
		fwdData := "school_pg:noop"
		if page < totalPages-1 {
			fwdData = fmt.Sprintf("school_pg:%d", page+1)
		}
		navRow := []chat.InlineButton{
			{Text: "⬅️", Data: backData},
			{Text: fmt.Sprintf("%d/%d", page+1, totalPages), Data: "school_pg:noop"},
			{Text: "➡️", Data: fwdData},
		}
		rows = append(rows, navRow)
	}

	return rows
}

// mainMenuButtonsForRole builds the main menu layout, appending the manager-only
// "School statistic" button when the caller has manager privileges.
func mainMenuButtonsForRole(isManager bool) [][]chat.MenuButton {
	buttons := [][]chat.MenuButton{
		{{Text: BtnMyOffice}, {Text: BtnServiceRate}},
		{{Text: BtnOrderStatus}},
		{{Text: BtnAIConsultant}, {Text: BtnMakeOrder}},
	}
	if isManager {
		buttons = append(buttons, []chat.MenuButton{{Text: BtnSchoolStat}})
	}
	return buttons
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

// PreMainMenuStep — Silent step; any input transitions to main menu.
type PreMainMenuStep struct{}

func (s *PreMainMenuStep) ID() chat.StepID { return StepPreMainMenu }

func (s *PreMainMenuStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	return chat.StepResult{}
}

func (s *PreMainMenuStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	return chat.StepResult{NextStep: StepMainMenu}
}

// MainMenuStep — Show main menu. Manager-role users additionally see the
// "School statistic" button. Non-managers cannot navigate to that step even
// if they somehow send the correct button text.
type MainMenuStep struct {
	authService AuthService
}

func (s *MainMenuStep) ID() chat.StepID { return StepMainMenu }

func (s *MainMenuStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	var isManager bool
	if user, err := getUser(state, s.authService); err == nil && user != nil {
		isManager = user.IsManager()
	}

	buttons := mainMenuButtonsForRole(isManager)
	if err := m.SendMenu(state.ChatID, "Натисніть на потрібний варіант, щоб перейти у бажаний розділ 👇", buttons); err != nil {
		return chat.StepResult{Error: err}
	}
	return chat.StepResult{}
}

func (s *MainMenuStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	text := strings.TrimSpace(input.Text)

	// Resolve role once for both exact-match and number-match paths.
	var isManager bool
	if user, err := getUser(state, s.authService); err == nil && user != nil {
		isManager = user.IsManager()
	}

	// Exact button text match.
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
	case BtnSchoolStat:
		if isManager {
			return chat.StepResult{NextStep: StepSchoolStat}
		}
		return chat.StepResult{}
	}

	// Number-based match (for text-only platforms like Instagram/WhatsApp).
	// Use the role-appropriate button set so numbers line up correctly.
	buttons := mainMenuButtonsForRole(isManager)
	switch chat.MatchNumberToOption(text, buttons) {
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
	case BtnSchoolStat:
		// Button only appears in the list for managers, so no extra check needed.
		return chat.StepResult{NextStep: StepSchoolStat}
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

	state.Set("current_order_id", activeOrder.ID)

	msg := formatOrderMessage(activeOrder, user.Name, state.Platform)
	buttons := []chat.InlineButton{
		{Text: "📋 Товари", Data: "products:" + activeOrder.ID},
	}
	_ = m.SendInlineOptions(state.ChatID, msg, buttons)
	return chat.StepResult{}
}

func (s *CurrentOrderStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	data := input.CallbackData
	if data == "" {
		orderID := state.GetString("current_order_id")
		if orderID != "" {
			buttons := []chat.InlineButton{
				{Text: "📋 Товари", Data: "products:" + orderID},
			}
			data = chat.MatchNumberToInline(input.Text, buttons)
		}
	}

	if strings.HasPrefix(data, "products:") {
		orderID := strings.TrimPrefix(data, "products:")
		products, err := s.zohoService.GetOrderProducts(orderID)
		if err != nil {
			_ = m.SendText(state.ChatID, "Не вдалося отримати товари.")
		} else {
			_ = m.SendText(state.ChatID, products)
		}
		return chat.StepResult{NextStep: StepMainMenu}
	}

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

	buttons := make([]chat.InlineButton, len(completedOrders))
	orderIDs := make([]string, len(completedOrders))
	for i, order := range completedOrders {
		buttons[i] = chat.InlineButton{
			Text: fmt.Sprintf("📋 Товари №%d", i+1),
			Data: "products:" + order.ID,
		}
		orderIDs[i] = order.ID
	}
	state.Set("completed_order_ids", strings.Join(orderIDs, ","))

	_ = m.SendInlineOptions(state.ChatID, "Оберіть замовлення для перегляду товарів:", buttons)
	return chat.StepResult{}
}

func (s *CompletedOrdersStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	data := input.CallbackData
	if data == "" {
		orderIDsStr := state.GetString("completed_order_ids")
		if orderIDsStr != "" {
			orderIDs := strings.Split(orderIDsStr, ",")
			buttons := make([]chat.InlineButton, len(orderIDs))
			for i, id := range orderIDs {
				buttons[i] = chat.InlineButton{
					Text: fmt.Sprintf("📋 Товари №%d", i+1),
					Data: "products:" + id,
				}
			}
			data = chat.MatchNumberToInline(input.Text, buttons)
		}
	}

	if strings.HasPrefix(data, "products:") {
		orderID := strings.TrimPrefix(data, "products:")
		products, err := s.zohoService.GetOrderProducts(orderID)
		if err != nil {
			_ = m.SendText(state.ChatID, "Не вдалося отримати товари.")
		} else {
			_ = m.SendText(state.ChatID, products)
		}
		return chat.StepResult{NextStep: StepMyOffice}
	}

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
	backMenu := [][]chat.MenuButton{{{Text: BtnBack}}}
	_ = m.SendMenu(state.ChatID, "Привіт! Я — консультант бренду DARK 🖤\nДопоможу з вибором товарів, проконсультую щодо продукції та оформлення замовлення.", backMenu)
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
	backMenu := [][]chat.MenuButton{{{Text: BtnBack}}}
	_ = m.SendMenu(state.ChatID, "Готові оформити замовлення!", backMenu)
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

// SchoolStatStep displays aggregated QR-funnel statistics and, on button press,
// the per-school user breakdown. Accessible only to manager-role users.
type SchoolStatStep struct {
	qrStatRepo QrStatRepository
}

func (s *SchoolStatStep) ID() chat.StepID { return StepSchoolStat }

func (s *SchoolStatStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	stats, err := s.qrStatRepo.GetAllQrStat()
	if err != nil {
		_ = m.SendText(state.ChatID, "Не вдалося отримати статистику.")
		return chat.StepResult{NextStep: StepMainMenu}
	}

	var followNum, regNum, schoolNum int
	for _, qr := range stats {
		if qr.FollowQr {
			followNum++
		}
		if qr.Registered {
			regNum++
		}
		if qr.SchoolName != "" {
			schoolNum++
		}
	}

	msg := fmt.Sprintf(
		"📊 Статистика шкіл\n\n🔗 Підписалися через QR: %d\n📝 Зареєстровані: %d\n🏫 Обрали школу: %d",
		followNum, regNum, schoolNum,
	)

	buttons := []chat.InlineButton{
		{Text: "📚 По школах", Data: "stat:schools"},
		{Text: "📅 По місяцях", Data: "stat:months"},
	}
	_ = m.SendInlineOptions(state.ChatID, msg, buttons)
	return chat.StepResult{}
}

func (s *SchoolStatStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	allButtons := []chat.InlineButton{
		{Text: "📚 По школах", Data: "stat:schools"},
		{Text: "📅 По місяцях", Data: "stat:months"},
	}

	data := input.CallbackData
	if data == "" {
		data = chat.MatchNumberToInline(input.Text, allButtons)
	}

	switch data {
	case "stat:schools":
		stats, err := s.qrStatRepo.GetAllQrStat()
		if err != nil {
			_ = m.SendText(state.ChatID, "Не вдалося отримати статистику.")
			return chat.StepResult{NextStep: StepMainMenu}
		}

		schoolCounts := make(map[string]int)
		for _, qr := range stats {
			if qr.SchoolName != "" {
				schoolCounts[qr.SchoolName]++
			}
		}

		if len(schoolCounts) == 0 {
			_ = m.SendText(state.ChatID, "Немає даних по школах.")
			return chat.StepResult{NextStep: StepMainMenu}
		}

		type entry struct {
			name  string
			count int
		}
		entries := make([]entry, 0, len(schoolCounts))
		for name, count := range schoolCounts {
			entries = append(entries, entry{name, count})
		}
		// Sort by count descending, then alphabetically for stable order.
		sort.Slice(entries, func(i, j int) bool {
			if entries[i].count != entries[j].count {
				return entries[i].count > entries[j].count
			}
			return entries[i].name < entries[j].name
		})

		msg := "📚 По школах:\n"
		for _, e := range entries {
			msg += fmt.Sprintf("\n%s — %d", e.name, e.count)
		}
		_ = m.SendText(state.ChatID, msg)

	case "stat:months":
		stats, err := s.qrStatRepo.GetAllQrStat()
		if err != nil {
			_ = m.SendText(state.ChatID, "Не вдалося отримати статистику.")
			return chat.StepResult{NextStep: StepMainMenu}
		}

		type monthKey struct {
			year  int
			month time.Month
		}
		type monthStat struct {
			subscribed int
			registered int
		}

		monthly := make(map[monthKey]*monthStat)
		for _, qr := range stats {
			key := monthKey{year: qr.Date.Year(), month: qr.Date.Month()}
			if monthly[key] == nil {
				monthly[key] = &monthStat{}
			}
			if qr.FollowQr {
				monthly[key].subscribed++
			}
			if qr.Registered {
				monthly[key].registered++
			}
		}

		if len(monthly) == 0 {
			_ = m.SendText(state.ChatID, "Немає даних по місяцях.")
			return chat.StepResult{NextStep: StepMainMenu}
		}

		keys := make([]monthKey, 0, len(monthly))
		for k := range monthly {
			keys = append(keys, k)
		}
		// Sort oldest first.
		sort.Slice(keys, func(i, j int) bool {
			if keys[i].year != keys[j].year {
				return keys[i].year < keys[j].year
			}
			return keys[i].month < keys[j].month
		})

		msg := "📊 Статистика по місяцях:\n"
		for _, k := range keys {
			st := monthly[k]
			label := entity.GetMonthName(time.Date(k.year, k.month, 1, 0, 0, 0, 0, time.UTC))
			msg += fmt.Sprintf("\n%s %d: 🔗 %d | 📝 %d", label, k.year, st.subscribed, st.registered)
		}
		_ = m.SendText(state.ChatID, msg)
	}

	return chat.StepResult{NextStep: StepMainMenu}
}
