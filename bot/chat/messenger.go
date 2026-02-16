package chat

import (
	"DarkCS/entity"
	"time"
)

// Messenger is the platform UI adapter interface.
// Each platform (Telegram, Instagram, WhatsApp) implements this to handle
// platform-specific message delivery.
type Messenger interface {
	SendText(chatID, text string) error
	SendMenu(chatID, text string, rows [][]MenuButton) error
	SendInlineOptions(chatID, text string, buttons []InlineButton) error
	SendInlineGrid(chatID, text string, rows [][]InlineButton) error
	SendContactRequest(chatID, text, buttonText string) error
	SendTyping(chatID string) error
}

// loggingMessenger wraps a Messenger and saves outgoing bot messages to CRM.
type loggingMessenger struct {
	inner    Messenger
	listener MessageListener
	platform string
	userID   string
}

func newLoggingMessenger(inner Messenger, listener MessageListener, platform, userID string) Messenger {
	if listener == nil {
		return inner
	}
	// Avoid double-wrapping
	if _, ok := inner.(*loggingMessenger); ok {
		return inner
	}
	return &loggingMessenger{inner: inner, listener: listener, platform: platform, userID: userID}
}

func (m *loggingMessenger) saveOutgoing(text string) {
	m.listener.SaveAndBroadcastChatMessage(entity.ChatMessage{
		Platform:  m.platform,
		UserID:    m.userID,
		ChatID:    m.userID,
		Direction: "outgoing",
		Sender:    "bot",
		Text:      text,
		CreatedAt: time.Now(),
	})
}

func (m *loggingMessenger) SendText(chatID, text string) error {
	if err := m.inner.SendText(chatID, text); err != nil {
		return err
	}
	m.saveOutgoing(text)
	return nil
}

func (m *loggingMessenger) SendMenu(chatID, text string, rows [][]MenuButton) error {
	if err := m.inner.SendMenu(chatID, text, rows); err != nil {
		return err
	}
	m.saveOutgoing(text)
	return nil
}

func (m *loggingMessenger) SendInlineOptions(chatID, text string, buttons []InlineButton) error {
	if err := m.inner.SendInlineOptions(chatID, text, buttons); err != nil {
		return err
	}
	m.saveOutgoing(text)
	return nil
}

func (m *loggingMessenger) SendInlineGrid(chatID, text string, rows [][]InlineButton) error {
	if err := m.inner.SendInlineGrid(chatID, text, rows); err != nil {
		return err
	}
	m.saveOutgoing(text)
	return nil
}

func (m *loggingMessenger) SendContactRequest(chatID, text, buttonText string) error {
	if err := m.inner.SendContactRequest(chatID, text, buttonText); err != nil {
		return err
	}
	m.saveOutgoing(text)
	return nil
}

func (m *loggingMessenger) SendTyping(chatID string) error {
	return m.inner.SendTyping(chatID)
}

// MenuButton represents a button in a reply/menu keyboard.
type MenuButton struct {
	Text string
}

// InlineButton represents an inline button with callback data.
type InlineButton struct {
	Text string
	Data string
}

// UserInput represents a normalized event from any platform.
type UserInput struct {
	Text         string // Regular message text
	CallbackData string // Inline button press or matched number
	Phone        string // Contact share or typed phone
}
