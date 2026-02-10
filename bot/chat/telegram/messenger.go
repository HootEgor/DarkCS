package telegram

import (
	"strconv"

	"DarkCS/bot/chat"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
)

// TelegramAPI defines the Telegram bot methods needed by the messenger.
// This avoids importing the concrete bot type and prevents circular imports.
type TelegramAPI interface {
	SendMessage(chatId int64, text string, opts *tgbotapi.SendMessageOpts) (*tgbotapi.Message, error)
	SendChatAction(chatId int64, action string, opts *tgbotapi.SendChatActionOpts) (bool, error)
}

// Messenger implements chat.Messenger for Telegram using native keyboards.
type Messenger struct {
	api TelegramAPI
}

// NewMessenger creates a new Telegram Messenger.
func NewMessenger(api TelegramAPI) *Messenger {
	return &Messenger{api: api}
}

func (m *Messenger) SendText(chatID, text string) error {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}
	_, err = m.api.SendMessage(id, text, &tgbotapi.SendMessageOpts{
		ParseMode: "HTML",
	})
	return err
}

func (m *Messenger) SendMenu(chatID, text string, rows [][]chat.MenuButton) error {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}

	keyboard := make([][]tgbotapi.KeyboardButton, len(rows))
	for i, row := range rows {
		keyboard[i] = make([]tgbotapi.KeyboardButton, len(row))
		for j, btn := range row {
			keyboard[i][j] = tgbotapi.KeyboardButton{Text: btn.Text}
		}
	}

	_, err = m.api.SendMessage(id, text, &tgbotapi.SendMessageOpts{
		ReplyMarkup: tgbotapi.ReplyKeyboardMarkup{
			Keyboard:       keyboard,
			ResizeKeyboard: true,
		},
	})
	return err
}

func (m *Messenger) SendInlineOptions(chatID, text string, buttons []chat.InlineButton) error {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}

	inlineButtons := make([]tgbotapi.InlineKeyboardButton, len(buttons))
	for i, btn := range buttons {
		inlineButtons[i] = tgbotapi.InlineKeyboardButton{
			Text:         btn.Text,
			CallbackData: btn.Data,
		}
	}

	_, err = m.api.SendMessage(id, text, &tgbotapi.SendMessageOpts{
		ReplyMarkup: tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{inlineButtons},
		},
	})
	return err
}

func (m *Messenger) SendContactRequest(chatID, text, buttonText string) error {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}

	_, err = m.api.SendMessage(id, text, &tgbotapi.SendMessageOpts{
		ReplyMarkup: tgbotapi.ReplyKeyboardMarkup{
			Keyboard: [][]tgbotapi.KeyboardButton{
				{{Text: buttonText, RequestContact: true}},
			},
			ResizeKeyboard:  true,
			OneTimeKeyboard: true,
		},
	})
	return err
}

func (m *Messenger) SendTyping(chatID string) error {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}
	_, err = m.api.SendChatAction(id, "typing", nil)
	return err
}
