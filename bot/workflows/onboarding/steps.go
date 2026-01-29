package onboarding

import (
	"DarkCS/entity"
	"context"
	"fmt"
	"regexp"
	"strings"

	"DarkCS/bot/workflow"
	"DarkCS/bot/workflow/ui"

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

// HelloStep - Welcome message
type HelloStep struct {
	BaseStep
}

func NewHelloStep() *HelloStep {
	return &HelloStep{BaseStep: BaseStep{id: StepHello}}
}

func (s *HelloStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	msg := "<b>Привіт! 🖤</b>\nДля швидкої перевірки в системі, будь ласка, надайте свій номер телефону у міжнародному форматі, починаючи з <b>+380...</b> 📱"
	if state.DeepLink.IsSchoolDeepLink() {
		msg = "Привіт! Вітаємо із завершення курсу 🖤\nВпевнені, що твої знання та наш матеріал стануть кроком до ще більших можливостей!\n\nНадайте свій номер телефону у міжнародному форматі, починаючи з <b>+380...</b> 📱"
	}
	_, err := b.SendMessage(state.ChatID, msg, &tgbotapi.SendMessageOpts{
		ParseMode: "HTML",
	})
	if err != nil {
		return workflow.StepResult{Error: err}
	}

	// Immediately transition to request phone step
	return workflow.StepResult{NextStep: StepRequestPhone}
}

// RequestPhoneStep - Request phone number
type RequestPhoneStep struct {
	BaseStep
}

func NewRequestPhoneStep() *RequestPhoneStep {
	return &RequestPhoneStep{BaseStep: BaseStep{id: StepRequestPhone}}
}

func (s *RequestPhoneStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	keyboard := ui.ContactRequestKeyboard("📱 Поділитися номером телефону")
	_, err := b.SendMessage(state.ChatID, "Натисніть кнопку нижче, щоб поділитися номером телефону:", &tgbotapi.SendMessageOpts{
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return workflow.StepResult{Error: err}
	}
	return workflow.StepResult{} // Wait for user input
}

func (s *RequestPhoneStep) HandleContact(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
	contact := c.EffectiveMessage.Contact
	if contact == nil {
		return workflow.StepResult{}
	}

	phone := normalizePhone(contact.PhoneNumber)
	return workflow.StepResult{
		NextStep:    StepValidatePhone,
		UpdateState: map[string]any{KeyPhone: phone},
	}
}

func (s *RequestPhoneStep) HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
	// Check if user sent phone number as text
	text := strings.TrimSpace(c.EffectiveMessage.Text)
	if isValidPhone(text) {
		phone := normalizePhone(text)
		return workflow.StepResult{
			NextStep:    StepValidatePhone,
			UpdateState: map[string]any{KeyPhone: phone},
		}
	}

	// Remind user to share contact
	b.SendMessage(state.ChatID, "Будь ласка, використовуйте кнопку нижче для надсилання номера телефону.", nil)
	return workflow.StepResult{}
}

// ValidatePhoneStep - Validate phone format
type ValidatePhoneStep struct {
	BaseStep
}

func NewValidatePhoneStep() *ValidatePhoneStep {
	return &ValidatePhoneStep{BaseStep: BaseStep{id: StepValidatePhone}}
}

func (s *ValidatePhoneStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	phone := state.GetString(KeyPhone)
	if !isValidPhone(phone) {
		b.SendMessage(state.ChatID, "❌ Невірний формат номера телефону. Спробуйте ще раз.", &tgbotapi.SendMessageOpts{
			ReplyMarkup: ui.RemoveKeyboard(),
		})
		return workflow.StepResult{NextStep: StepRequestPhone}
	}

	// Phone is valid, remove keyboard and auto-transition to check user
	b.SendMessage(state.ChatID, fmt.Sprintf("✅ Номер телефону: %s", phone), &tgbotapi.SendMessageOpts{
		ReplyMarkup: ui.RemoveKeyboard(),
	})
	return workflow.StepResult{NextStep: StepCheckUser}
}

// CheckUserStep - Check if user exists
type CheckUserStep struct {
	BaseStep
	authService AuthService
	zohoService ZohoService
}

func NewCheckUserStep(authService AuthService, zohoService ZohoService) *CheckUserStep {
	return &CheckUserStep{
		BaseStep:    BaseStep{id: StepCheckUser},
		authService: authService,
		zohoService: zohoService,
	}
}

func (s *CheckUserStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	phone := state.GetString(KeyPhone)
	user, _ := s.authService.UserExists("", phone, state.UserID)

	if user != nil && user.Name != "" {
		// User exists with name, check if we need to update Zoho ID
		if user.ZohoId == "" && s.zohoService != nil {
			zohoId, err := s.zohoService.CreateContact(user)
			if err == nil && zohoId != "" {
				user.ZohoId = zohoId
				s.authService.UpdateUser(user)
			}
		}

		// Save info and skip name entry
		return workflow.StepResult{
			NextStep: StepProcessDeepCode,
			UpdateState: map[string]any{
				KeyUserExists: true,
				KeyUserUUID:   user.UUID,
				KeyName:       user.Name,
			},
		}
	}

	// New user or no name, need to request name
	return workflow.StepResult{
		NextStep:    StepRequestName,
		UpdateState: map[string]any{KeyUserExists: false},
	}
}

// RequestNameStep - Request user's name
type RequestNameStep struct {
	BaseStep
}

func NewRequestNameStep() *RequestNameStep {
	return &RequestNameStep{BaseStep: BaseStep{id: StepRequestName}}
}

func (s *RequestNameStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	msg := "Будь ласка, залиште ваші <b>ім’я та прізвище</b> для знайомства 😎"
	if state.DeepLink.IsSchoolDeepLink() {
		msg = "Залишай свої контакти та тримай знижку на перше замовлення! 😎"
	}
	_, err := b.SendMessage(state.ChatID, msg, &tgbotapi.SendMessageOpts{
		ParseMode: "HTML",
	})
	if err != nil {
		return workflow.StepResult{Error: err}
	}
	return workflow.StepResult{} // Wait for user input
}

func (s *RequestNameStep) HandleMessage(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) workflow.StepResult {
	name := strings.TrimSpace(c.EffectiveMessage.Text)
	if name == "" || len(name) < 2 {
		b.SendMessage(state.ChatID, "Будь ласка, введіть коректне ім'я (мінімум 2 символи).", nil)
		return workflow.StepResult{}
	}

	return workflow.StepResult{
		NextStep:    StepConfirmData,
		UpdateState: map[string]any{KeyName: name},
	}
}

// ConfirmDataStep - Show summary and confirm
type ConfirmDataStep struct {
	BaseStep
	authService AuthService
	zohoService ZohoService
}

func NewConfirmDataStep(authService AuthService, zohoService ZohoService) *ConfirmDataStep {
	return &ConfirmDataStep{
		BaseStep:    BaseStep{id: StepConfirmData},
		authService: authService,
		zohoService: zohoService,
	}
}

func (s *ConfirmDataStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	name := state.GetString(KeyName)
	phone := state.GetString(KeyPhone)

	msg := fmt.Sprintf("📋 Перевірте дані:\n\n👤 Ім'я: %s\n📱 Телефон: %s\n\nВсе вірно?", name, phone)

	keyboard := ui.YesNoKeyboard("✅ Так", "❌ Ні, змінити")
	_, err := b.SendMessage(state.ChatID, msg, &tgbotapi.SendMessageOpts{
		ReplyMarkup: keyboard,
	})
	if err != nil {
		return workflow.StepResult{Error: err}
	}
	return workflow.StepResult{} // Wait for user input
}

func (s *ConfirmDataStep) HandleCallback(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState, data string) workflow.StepResult {
	cb := workflow.ParseCallback(data)
	if cb == nil {
		return workflow.StepResult{}
	}

	// Answer callback
	c.CallbackQuery.Answer(b, nil)

	if cb.IsNo() {
		return workflow.StepResult{NextStep: StepRequestName}
	}

	if cb.IsYes() {
		// Save user data
		name := state.GetString(KeyName)
		phone := state.GetString(KeyPhone)

		user, err := s.authService.RegisterUser(name, "", phone, state.UserID)
		if err != nil {
			b.SendMessage(state.ChatID, "Виникла помилка при збереженні даних. Спробуйте пізніше.", nil)
			return workflow.StepResult{Error: err}
		}

		// Check if name or phone changed, or if ZohoId is missing
		needsZohoUpdate := user.ZohoId == "" || user.Name != name || user.Phone != phone

		// Update name if user already existed
		if user.Name != name {
			user.Name = name
		}

		// Create or update Zoho contact if needed
		if needsZohoUpdate && s.zohoService != nil {
			zohoId, zohoErr := s.zohoService.CreateContact(user)
			if zohoErr == nil && zohoId != "" {
				user.ZohoId = zohoId
			}
		}

		// Save user with updated ZohoId
		s.authService.UpdateUser(user)

		state.Set(KeyUserUUID, user.UUID)

		b.SendMessage(state.ChatID, "✅ Дані збережено!", nil)
		return workflow.StepResult{NextStep: StepProcessDeepCode}
	}

	return workflow.StepResult{}
}

// ProcessDeepCodeStep - Check deep link and route accordingly
type ProcessDeepCodeStep struct {
	BaseStep
}

func NewProcessDeepCodeStep() *ProcessDeepCodeStep {
	return &ProcessDeepCodeStep{BaseStep: BaseStep{id: StepProcessDeepCode}}
}

func (s *ProcessDeepCodeStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	// Auto-transition based on deep link
	if state.DeepLink == nil || state.DeepLink.IsEmpty() {
		return workflow.StepResult{NextStep: StepMainMenu}
	}

	if state.DeepLink.IsSchoolDeepLink() {
		return workflow.StepResult{NextStep: StepSelectSchool}
	}

	return workflow.StepResult{NextStep: StepMainMenu}
}

// SelectSchoolStep - Show school selection
type SelectSchoolStep struct {
	BaseStep
	schoolRepo SchoolRepository
}

func NewSelectSchoolStep(schoolRepo SchoolRepository) *SelectSchoolStep {
	return &SelectSchoolStep{
		BaseStep:   BaseStep{id: StepSelectSchool},
		schoolRepo: schoolRepo,
	}
}

func (s *SelectSchoolStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	schools, err := s.schoolRepo.GetAllActiveSchools(ctx)
	if err != nil {
		b.SendMessage(state.ChatID, "Виникла помилка при завантаженні списку шкіл.", nil)
		return workflow.StepResult{Error: err}
	}

	if len(schools) == 0 {
		b.SendMessage(state.ChatID, "Наразі немає доступних шкіл.", nil)
		return workflow.StepResult{NextStep: StepMainMenu}
	}

	// Initialize pagination
	state.InitPagination(len(schools), ui.DefaultItemsPerPage)

	if err := s.sendSchoolList(ctx, b, state, schools); err != nil {
		return workflow.StepResult{Error: err}
	}
	return workflow.StepResult{} // Wait for user selection
}

func (s *SelectSchoolStep) sendSchoolList(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState, schools []entity.School) error {
	// Get schools from repository
	schoolList, err := s.schoolRepo.GetAllActiveSchools(ctx)
	if err != nil {
		return err
	}

	start, end := state.GetPageItems()
	pageSchools := schoolList[start:end]

	items := make([]ui.SelectableItem, len(pageSchools))
	for i, school := range pageSchools {
		items[i] = ui.SelectableItem{
			ID:   school.Name,
			Text: school.Name,
		}
	}

	keyboard := ui.PaginatedList(items, state.Pagination.CurrentPage, state.Pagination.TotalPages)

	_, err = b.SendMessage(state.ChatID, "<b>Розкажи, будь ласка, з якої школи ти дізнався/дізналася про нас 🖤</b>\nОберіть школу:", &tgbotapi.SendMessageOpts{
		ParseMode:   "HTML",
		ReplyMarkup: keyboard,
	})
	return err
}

func (s *SelectSchoolStep) HandleCallback(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState, data string) workflow.StepResult {
	cb := workflow.ParseCallback(data)
	if cb == nil {
		return workflow.StepResult{}
	}

	// Answer callback
	c.CallbackQuery.Answer(b, nil)

	if cb.IsNoop() {
		return workflow.StepResult{}
	}

	if cb.IsPage() {
		page := cb.PageNumber()
		if page > 0 {
			state.Pagination.CurrentPage = page
			s.updateSchoolList(ctx, b, c, state)
		}
		return workflow.StepResult{}
	}

	if cb.IsSelect() {
		schoolID := cb.SelectedID()
		school, err := s.schoolRepo.GetSchoolByID(ctx, schoolID)
		if err != nil || school == nil {
			b.SendMessage(state.ChatID, "Школу не знайдено.", nil)
			return workflow.StepResult{}
		}

		// Save school selection
		b.SendMessage(state.ChatID, "Отримай -15% на перше замовлення з промо-кодом <b>DARKSCHOOL</b> 🖤\nСкористайся протягом 14 днів на сайті 👉 riornails.com\n\nP.S. Твоя особиста знижка -7% вже активна, і з часом може стати ще більшою ✨",
			&tgbotapi.SendMessageOpts{
				ParseMode: "HTML",
			})

		return workflow.StepResult{
			NextStep: StepMainMenu,
			UpdateState: map[string]any{
				KeySchoolName: school.Name,
			},
		}
	}

	return workflow.StepResult{}
}

func (s *SelectSchoolStep) updateSchoolList(ctx context.Context, b *tgbotapi.Bot, c *ext.Context, state *workflow.UserState) {
	schools, err := s.schoolRepo.GetAllActiveSchools(ctx)
	if err != nil {
		return
	}

	start, end := state.GetPageItems()
	if start >= len(schools) {
		return
	}
	if end > len(schools) {
		end = len(schools)
	}
	pageSchools := schools[start:end]

	items := make([]ui.SelectableItem, len(pageSchools))
	for i, school := range pageSchools {
		items[i] = ui.SelectableItem{
			ID:   school.Name,
			Text: school.Name,
		}
	}

	keyboard := ui.PaginatedList(items, state.Pagination.CurrentPage, state.Pagination.TotalPages)

	c.EffectiveMessage.EditReplyMarkup(b, &tgbotapi.EditMessageReplyMarkupOpts{
		ReplyMarkup: keyboard,
	})
}

// MainMenuStep - Show main menu and complete workflow, then chain to mainmenu workflow
type MainMenuStep struct {
	BaseStep
}

func NewMainMenuStep() *MainMenuStep {
	return &MainMenuStep{BaseStep: BaseStep{id: StepMainMenu}}
}

func (s *MainMenuStep) Enter(ctx context.Context, b *tgbotapi.Bot, state *workflow.UserState) workflow.StepResult {
	name := state.GetString(KeyName)
	msg := fmt.Sprintf("%s, цей чат-бот для того, щоб зробити нашу взаємодію ще зручнішою!", name)

	_, err := b.SendMessage(state.ChatID, msg, nil)
	if err != nil {
		return workflow.StepResult{Error: err}
	}
	return workflow.StepResult{
		Complete: true,
		UpdateState: map[string]any{
			"next_workflow": "mainmenu",
		},
	}
}

// Helper functions

func normalizePhone(phone string) string {
	// Remove all non-digit characters
	digits := ""
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			digits += string(ch)
		}
	}

	// Add + prefix
	if len(digits) > 0 && digits[0] != '+' {
		digits = "+" + digits
	}

	return digits
}

func isValidPhone(phone string) bool {
	// Remove non-digits for validation
	digits := ""
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			digits += string(ch)
		}
	}

	// Phone should have at least 10 digits
	if len(digits) < 10 {
		return false
	}

	// Basic phone pattern
	pattern := regexp.MustCompile(`^\+?[0-9]{10,15}$`)
	return pattern.MatchString("+" + digits)
}
