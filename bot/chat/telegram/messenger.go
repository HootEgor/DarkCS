package telegram

import (
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

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
	// businessConnectionID, when set, sends every message on behalf of a connected
	// Telegram Business account instead of from the bot itself.
	businessConnectionID string
}

// NewMessenger creates a new Telegram Messenger.
func NewMessenger(api TelegramAPI) *Messenger {
	return &Messenger{api: api}
}

// NewBusinessMessenger creates a Messenger that writes as the Telegram Business account
// of the given connection (used by the CRM to reply in premium account chats).
func NewBusinessMessenger(api TelegramAPI, businessConnectionID string) *Messenger {
	return &Messenger{api: api, businessConnectionID: businessConnectionID}
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
		BusinessConnectionId: m.businessConnectionID,
		ProtectContent:       protected,
		RequestOpts:          uploadRequestOpts,
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
		BusinessConnectionId: m.businessConnectionID,
		Caption:              file.Caption,
		ProtectContent:       file.Protected,
		RequestOpts:          uploadRequestOpts,
	})
	return err
}

// SendText sends text with HTML parse mode, so callers may use markup such as the ТТН
// tracking link. Text is split at the 4096-character limit. Arbitrary text (AI answers,
// names) can contain "<" or "&" that Telegram rejects as invalid HTML, so on a parse
// error the chunk is resent as plain text instead of being lost.
func (m *Messenger) SendText(chatID, text string) error {
	_, err := m.SendTextReturningID(chatID, text)
	return err
}

// SendTextReturningID is SendText that also returns the Telegram id of the first sent
// message, so Telegram Business replies can be matched to later edits and deletions.
func (m *Messenger) SendTextReturningID(chatID, text string) (int64, error) {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return 0, err
	}
	var firstID int64
	for _, chunk := range chat.SplitText(text, chat.TelegramTextLimit) {
		var sent *tgbotapi.Message
		sent, err = m.api.SendMessage(id, chunk, m.messageOpts(&tgbotapi.SendMessageOpts{ParseMode: "HTML"}))
		if isParseError(err) {
			sent, err = m.api.SendMessage(id, chunk, m.messageOpts(nil))
		}
		if err != nil {
			return firstID, err
		}
		if firstID == 0 && sent != nil {
			firstID = sent.MessageId
		}
	}
	return firstID, nil
}

// messageOpts adds the business connection to SendMessage options (nil allowed).
func (m *Messenger) messageOpts(opts *tgbotapi.SendMessageOpts) *tgbotapi.SendMessageOpts {
	if m.businessConnectionID == "" {
		return opts
	}
	if opts == nil {
		opts = &tgbotapi.SendMessageOpts{}
	}
	opts.BusinessConnectionId = m.businessConnectionID
	return opts
}

// uploadRequestOpts lifts gotgbot's default 5 s request timeout for media uploads: it
// covers the whole multipart request, including streaming the file from Google Drive,
// so any video that takes longer than 5 s to transfer used to fail.
var uploadRequestOpts = &tgbotapi.RequestOpts{Timeout: 10 * time.Minute}

// isParseError reports whether Telegram rejected the message's HTML markup.
func isParseError(err error) bool {
	var tgErr *tgbotapi.TelegramError
	return errors.As(err, &tgErr) && strings.Contains(tgErr.Description, "can't parse entities")
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

	_, err = m.api.SendMessage(id, text, m.messageOpts(&tgbotapi.SendMessageOpts{
		ReplyMarkup: tgbotapi.ReplyKeyboardMarkup{
			Keyboard:       keyboard,
			ResizeKeyboard: true,
		},
	}))
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

	_, err = m.api.SendMessage(id, text, m.messageOpts(&tgbotapi.SendMessageOpts{
		ReplyMarkup: tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: [][]tgbotapi.InlineKeyboardButton{inlineButtons},
		},
	}))
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

	_, err = m.api.SendMessage(id, text, m.messageOpts(&tgbotapi.SendMessageOpts{
		ReplyMarkup: tgbotapi.InlineKeyboardMarkup{
			InlineKeyboard: keyboard,
		},
	}))
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
		BusinessConnectionId: m.businessConnectionID,
		ChatId:               chatInt,
		MessageId:            msgInt,
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

	_, err = m.api.SendMessage(id, text, m.messageOpts(&tgbotapi.SendMessageOpts{
		ReplyMarkup: tgbotapi.ReplyKeyboardMarkup{
			Keyboard: [][]tgbotapi.KeyboardButton{
				{{Text: buttonText, RequestContact: true}},
			},
			ResizeKeyboard:  true,
			OneTimeKeyboard: true,
		},
	}))
	return err
}

func (m *Messenger) SendTyping(chatID string) error {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}
	_, err = m.api.SendChatAction(id, "typing", m.chatActionOpts())
	return err
}

func (m *Messenger) SendUploadAction(chatID string) error {
	id, err := strconv.ParseInt(chatID, 10, 64)
	if err != nil {
		return err
	}
	_, err = m.api.SendChatAction(id, "upload_video", m.chatActionOpts())
	return err
}

// chatActionOpts returns nil for the bot itself, keeping the existing call shape.
func (m *Messenger) chatActionOpts() *tgbotapi.SendChatActionOpts {
	if m.businessConnectionID == "" {
		return nil
	}
	return &tgbotapi.SendChatActionOpts{BusinessConnectionId: m.businessConnectionID}
}
