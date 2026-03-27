package telegram

import (
	"io"
	"strconv"

	"DarkCS/bot/chat"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
)

// TelegramAPI defines the Telegram bot methods needed by the messenger.
// This avoids importing the concrete bot type and prevents circular imports.
type TelegramAPI interface {
	SendMessage(chatId int64, text string, opts *tgbotapi.SendMessageOpts) (*tgbotapi.Message, error)
	SendDocument(chatId int64, document tgbotapi.InputFileOrString, opts *tgbotapi.SendDocumentOpts) (*tgbotapi.Message, error)
	SendVideo(chatId int64, video tgbotapi.InputFileOrString, opts *tgbotapi.SendVideoOpts) (*tgbotapi.Message, error)
	EditMessageText(text string, opts *tgbotapi.EditMessageTextOpts) (*tgbotapi.Message, bool, error)
	SendChatAction(chatId int64, action string, opts *tgbotapi.SendChatActionOpts) (bool, error)
	GetFile(fileId string, opts *tgbotapi.GetFileOpts) (*tgbotapi.File, error)
}

// Messenger implements chat.Messenger for Telegram using native keyboards.
type Messenger struct {
	api TelegramAPI
}

// NewMessenger creates a new Telegram Messenger.
func NewMessenger(api TelegramAPI) *Messenger {
	return &Messenger{api: api}
}

// SendVideo uploads a video to Telegram and optionally protects it from forwarding.
// If cachedFileID is non-empty, the previously uploaded file is resent without re-uploading.
// publicURL is ignored on Telegram; the stream r or the cached file_id is used instead.
func (m *Messenger) SendVideo(chatID string, r io.Reader, cachedFileID, publicURL, filename string, protected bool) (string, error) {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return "", err
	}

	var inputFile tgbotapi.InputFileOrString
	if cachedFileID != "" && r == nil {
		inputFile = tgbotapi.InputFileByID(cachedFileID)
	} else {
		inputFile = tgbotapi.InputFileByReader(filename, r)
	}

	msg, err := m.api.SendVideo(id, inputFile, &tgbotapi.SendVideoOpts{
		ProtectContent: protected,
	})
	if err != nil {
		return "", err
	}
	if msg.Video != nil {
		return msg.Video.FileId, nil
	}
	return "", nil
}

func (m *Messenger) SendFile(chatID string, file chat.FileMessage) error {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}
	doc := tgbotapi.InputFileByReader(file.Filename, file.Reader)
	_, err = m.api.SendDocument(id, doc, &tgbotapi.SendDocumentOpts{
		Caption: file.Caption,
	})
	return err
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

func (m *Messenger) SendInlineGrid(chatID, text string, rows [][]chat.InlineButton) error {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}

	keyboard := make([][]tgbotapi.InlineKeyboardButton, len(rows))
	for i, row := range rows {
		keyboard[i] = make([]tgbotapi.InlineKeyboardButton, len(row))
		for j, btn := range row {
			keyboard[i][j] = tgbotapi.InlineKeyboardButton{
				Text:         btn.Text,
				CallbackData: btn.Data,
			}
		}
	}

	_, err = m.api.SendMessage(id, text, &tgbotapi.SendMessageOpts{
		ReplyMarkup: tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	})
	return err
}

func (m *Messenger) EditInlineGrid(chatID, messageID, text string, rows [][]chat.InlineButton) error {
	chatInt, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}
	msgInt, err := strconv.ParseInt(messageID, 10, 64)
	if err != nil {
		return err
	}

	keyboard := make([][]tgbotapi.InlineKeyboardButton, len(rows))
	for i, row := range rows {
		keyboard[i] = make([]tgbotapi.InlineKeyboardButton, len(row))
		for j, btn := range row {
			keyboard[i][j] = tgbotapi.InlineKeyboardButton{
				Text:         btn.Text,
				CallbackData: btn.Data,
			}
		}
	}

	_, _, err = m.api.EditMessageText(text, &tgbotapi.EditMessageTextOpts{
		ChatId:    chatInt,
		MessageId: msgInt,
		ReplyMarkup: tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
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

func (m *Messenger) SendUploadAction(chatID string) error {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}
	_, err = m.api.SendChatAction(id, "upload_video", nil)
	return err
}
