package instagram

import (
	"DarkCS/bot/chat"
)

// MessageSender can send a text message or media to a recipient.
type MessageSender interface {
	SendMessage(recipientID, text string) error
	SendMediaMessage(recipientID, mediaURL, mediaType string) error
}

// Messenger implements chat.Messenger for Instagram.
type Messenger struct {
	sender MessageSender
}

// NewMessenger creates a new Instagram Messenger.
func NewMessenger(sender MessageSender) *Messenger {
	return &Messenger{sender: sender}
}

func (m *Messenger) SendFile(chatID string, file chat.FileMessage) error {
	// Instagram doesn't support file upload via reader — it requires a publicly accessible URL.
	// The CRM file download endpoint URL is passed as the caption fallback.
	if file.MIMEType != "" {
		mediaType := "file"
		if len(file.MIMEType) > 6 && file.MIMEType[:6] == "image/" {
			mediaType = "image"
		}
		// Caption is sent as a separate text message if present
		if file.Caption != "" {
			_ = m.sender.SendMessage(chatID, file.Caption)
		}
		return m.sender.SendMediaMessage(chatID, file.Filename, mediaType)
	}
	// Fallback: send filename as text
	text := "[File: " + file.Filename + "]"
	if file.Caption != "" {
		text = file.Caption + "\n" + text
	}
	return m.sender.SendMessage(chatID, text)
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
