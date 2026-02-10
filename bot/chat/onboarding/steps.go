package onboarding

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"DarkCS/bot/chat"
)

// HelloStep — Welcome message, then auto-transition to request phone.
type HelloStep struct{}

func (s *HelloStep) ID() chat.StepID { return StepHello }

func (s *HelloStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	if err := m.SendText(state.ChatID, "Привіт! 🖤\nДля швидкої перевірки в системі, будь ласка, надайте свій номер телефону у міжнародному форматі, починаючи з +380... 📱"); err != nil {
		return chat.StepResult{Error: err}
	}

	// WhatsApp: offer choice — use WhatsApp phone or enter manually
	if state.Platform == "whatsapp" {
		waPhone := chat.NormalizePhone(state.UserID)
		if chat.IsValidPhone(waPhone) {
			state.Set("wa_phone", waPhone)
			return chat.StepResult{NextStep: StepChoosePhone}
		}
	}

	return chat.StepResult{NextStep: StepRequestPhone}
}

func (s *HelloStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	return chat.StepResult{}
}

// ChoosePhoneStep — WhatsApp only: let user pick their WA phone or enter another.
type ChoosePhoneStep struct{}

func (s *ChoosePhoneStep) ID() chat.StepID { return StepChoosePhone }

func (s *ChoosePhoneStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	waPhone := state.GetString("wa_phone")
	buttons := []chat.InlineButton{
		{Text: fmt.Sprintf("📱 Використати %s", waPhone), Data: "use_wa_phone"},
		{Text: "✏️ Ввести інший номер", Data: "enter_manual"},
	}
	_ = m.SendInlineOptions(state.ChatID, "Бажаєте використати номер телефону з WhatsApp або ввести інший?", buttons)
	return chat.StepResult{}
}

func (s *ChoosePhoneStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	data := input.CallbackData
	if data == "" {
		waPhone := state.GetString("wa_phone")
		buttons := []chat.InlineButton{
			{Text: fmt.Sprintf("📱 Використати %s", waPhone), Data: "use_wa_phone"},
			{Text: "✏️ Ввести інший номер", Data: "enter_manual"},
		}
		data = chat.MatchNumberToInline(input.Text, buttons)
	}

	switch data {
	case "use_wa_phone":
		waPhone := state.GetString("wa_phone")
		_ = m.SendText(state.ChatID, fmt.Sprintf("✅ Номер телефону: %s", waPhone))
		return chat.StepResult{
			NextStep:    StepCheckUser,
			UpdateState: map[string]any{KeyPhone: waPhone},
		}
	case "enter_manual":
		return chat.StepResult{NextStep: StepRequestPhone}
	}

	return chat.StepResult{}
}

// RequestPhoneStep — Wait for user to type a phone number.
type RequestPhoneStep struct{}

func (s *RequestPhoneStep) ID() chat.StepID { return StepRequestPhone }

func (s *RequestPhoneStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	if state.Platform == "telegram" {
		_ = m.SendContactRequest(state.ChatID, "Натисніть кнопку нижче, щоб поділитися номером телефону:", "📱 Поділитися номером телефону")
		return chat.StepResult{}
	}
	_ = m.SendText(state.ChatID, "Введіть номер телефону (наприклад +380XXXXXXXXX):")
	return chat.StepResult{} // Wait for input
}

func (s *RequestPhoneStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	text := strings.TrimSpace(input.Text)

	// Check if phone was shared via contact
	if input.Phone != "" {
		text = input.Phone
	}

	if !chat.IsValidPhone(text) {
		_ = m.SendText(state.ChatID, "❌ Невірний формат номера телефону. Спробуйте ще раз (наприклад +380XXXXXXXXX):")
		return chat.StepResult{}
	}

	phone := chat.NormalizePhone(text)
	_ = m.SendText(state.ChatID, fmt.Sprintf("✅ Номер телефону: %s", phone))

	return chat.StepResult{
		NextStep:    StepCheckUser,
		UpdateState: map[string]any{KeyPhone: phone},
	}
}

// CheckUserStep — Check if user exists by phone (auto-transition step).
type CheckUserStep struct {
	authService AuthService
	zohoService ZohoService
}

func (s *CheckUserStep) ID() chat.StepID { return StepCheckUser }

func (s *CheckUserStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	phone := state.GetString(KeyPhone)

	// Parse telegramId when platform is telegram
	var telegramId int64
	if state.Platform == "telegram" {
		telegramId, _ = strconv.ParseInt(state.UserID, 10, 64)
	}

	user, _ := s.authService.UserExists("", phone, telegramId)

	if user != nil && user.Name != "" {
		needsUpdate := false

		// Link Instagram ID if missing
		if state.Platform == "instagram" && user.InstagramId == "" {
			user.InstagramId = state.UserID
			needsUpdate = true
		}

		// Link Telegram ID if missing
		if state.Platform == "telegram" && user.TelegramId == 0 && telegramId != 0 {
			user.TelegramId = telegramId
			needsUpdate = true
		}

		// Ensure Zoho contact exists
		if user.ZohoId == "" && s.zohoService != nil {
			zohoId, err := s.zohoService.CreateContact(user)
			if err == nil && zohoId != "" {
				user.ZohoId = zohoId
				needsUpdate = true
			}
		}

		if needsUpdate {
			_ = s.authService.UpdateUser(user)
		}

		return chat.StepResult{
			NextStep: StepDone,
			UpdateState: map[string]any{
				KeyUserExists: true,
				KeyUserUUID:   user.UUID,
				KeyName:       user.Name,
			},
		}
	}

	// New user — need name
	return chat.StepResult{
		NextStep:    StepRequestName,
		UpdateState: map[string]any{KeyUserExists: false},
	}
}

func (s *CheckUserStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	return chat.StepResult{}
}

// RequestNameStep — Ask for the user's name.
type RequestNameStep struct{}

func (s *RequestNameStep) ID() chat.StepID { return StepRequestName }

func (s *RequestNameStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	_ = m.SendText(state.ChatID, "Будь ласка, залиште ваші ім'я та прізвище для знайомства 😎")
	return chat.StepResult{}
}

func (s *RequestNameStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	name := strings.TrimSpace(input.Text)
	if name == "" || len(name) < 2 {
		_ = m.SendText(state.ChatID, "Будь ласка, введіть коректне ім'я (мінімум 2 символи).")
		return chat.StepResult{}
	}

	return chat.StepResult{
		NextStep:    StepConfirmData,
		UpdateState: map[string]any{KeyName: name},
	}
}

// ConfirmDataStep — Show summary and ask to confirm.
type ConfirmDataStep struct {
	authService AuthService
	zohoService ZohoService
}

func (s *ConfirmDataStep) ID() chat.StepID { return StepConfirmData }

func (s *ConfirmDataStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	name := state.GetString(KeyName)
	phone := state.GetString(KeyPhone)

	msg := fmt.Sprintf("📋 Перевірте дані:\n\n👤 Ім'я: %s\n📱 Телефон: %s\n\nВсе вірно?", name, phone)

	buttons := []chat.InlineButton{
		{Text: "✅ Так", Data: "confirm_yes"},
		{Text: "❌ Ні, змінити", Data: "confirm_no"},
	}
	_ = m.SendInlineOptions(state.ChatID, msg, buttons)
	return chat.StepResult{}
}

func (s *ConfirmDataStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	data := input.CallbackData
	if data == "" {
		// Try matching numbered input
		buttons := []chat.InlineButton{
			{Text: "✅ Так", Data: "confirm_yes"},
			{Text: "❌ Ні, змінити", Data: "confirm_no"},
		}
		data = chat.MatchNumberToInline(input.Text, buttons)
	}

	switch data {
	case "confirm_no":
		return chat.StepResult{NextStep: StepRequestName}

	case "confirm_yes":
		name := state.GetString(KeyName)
		phone := state.GetString(KeyPhone)

		// Parse telegramId when platform is telegram
		var telegramId int64
		if state.Platform == "telegram" {
			telegramId, _ = strconv.ParseInt(state.UserID, 10, 64)
		}

		user, err := s.authService.RegisterUser(name, "", phone, telegramId)
		if err != nil {
			_ = m.SendText(state.ChatID, "Виникла помилка при збереженні даних. Спробуйте пізніше.")
			return chat.StepResult{Error: err}
		}

		// Link platform ID
		if state.Platform == "instagram" {
			user.InstagramId = state.UserID
		}
		if state.Platform == "telegram" && user.TelegramId == 0 && telegramId != 0 {
			user.TelegramId = telegramId
		}

		// Create or update Zoho contact
		if user.ZohoId == "" && s.zohoService != nil {
			zohoId, zohoErr := s.zohoService.CreateContact(user)
			if zohoErr == nil && zohoId != "" {
				user.ZohoId = zohoId
			}
		}

		// Update name if needed
		if user.Name != name {
			user.Name = name
		}

		_ = s.authService.UpdateUser(user)

		state.Set(KeyUserUUID, user.UUID)
		_ = m.SendText(state.ChatID, "✅ Дані збережено!")

		return chat.StepResult{NextStep: StepDone}
	}

	return chat.StepResult{}
}

// DoneStep — Finish onboarding, chain to mainmenu.
type DoneStep struct{}

func (s *DoneStep) ID() chat.StepID { return StepDone }

func (s *DoneStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	name := state.GetString(KeyName)
	_ = m.SendText(state.ChatID, fmt.Sprintf("%s, цей чат-бот для того, щоб зробити нашу взаємодію ще зручнішою!", name))

	return chat.StepResult{
		Complete: true,
		UpdateState: map[string]any{
			"next_workflow": "mainmenu",
		},
	}
}

func (s *DoneStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	return chat.StepResult{}
}
