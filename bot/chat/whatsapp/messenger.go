package whatsapp

import (
	"DarkCS/bot/chat"
)

// MessageSender can send a text message or media to a recipient.
type MessageSender interface {
	SendMessage(recipientPhone, text string) error
	SendMediaMessage(recipientPhone, mediaType, mediaURL, caption, filename string) error
}

// Messenger implements chat.Messenger for WhatsApp.
type Messenger struct {
	sender MessageSender
}

// NewMessenger creates a new WhatsApp Messenger.
func NewMessenger(sender MessageSender) *Messenger {
	return &Messenger{sender: sender}
}

func (m *Messenger) SendFile(chatID string, file chat.FileMessage) error {
	mediaType := "document"
	if len(file.MIMEType) > 6 && file.MIMEType[:6] == "image/" {
		mediaType = "image"
	} else if len(file.MIMEType) > 6 && file.MIMEType[:6] == "video/" {
		mediaType = "video"
	} else if len(file.MIMEType) > 6 && file.MIMEType[:6] == "audio/" {
		mediaType = "audio"
	}
	return m.sender.SendMediaMessage(chatID, mediaType, "", file.Caption, file.Filename)
}

func (m *Messenger) SendText(chatID, text string) error {
	return m.sender.SendMessage(chatID, text)
}

func (m *Messenger) SendMenu(chatID, text string, rows [][]chat.MenuButton) error {
	formatted := chat.FormatNumberedMenu(text, rows)
	return m.sender.SendMessage(chatID, formatted)
}

func (m *Messenger) SendInlineOptions(chatID, text string, buttons []chat.InlineButton) error {
	formatted := chat.FormatNumberedInline(text, buttons)
	return m.sender.SendMessage(chatID, formatted)
}

func (m *Messenger) SendInlineGrid(chatID, text string, rows [][]chat.InlineButton) error {
	formatted := chat.FormatNumberedInlineGrid(text, rows)
	return m.sender.SendMessage(chatID, formatted)
}

func (m *Messenger) EditInlineGrid(chatID, messageID, text string, rows [][]chat.InlineButton) error {
	return m.SendInlineGrid(chatID, text, rows)
}

func (m *Messenger) SendContactRequest(chatID, text, buttonText string) error {
	return m.sender.SendMessage(chatID, text)
}

func (m *Messenger) SendTyping(chatID string) error {
	return nil
}
