package chat

import (
	"io"
	"time"

	"DarkCS/entity"
)

// FileMessage holds the data needed to send a file to a user.
type FileMessage struct {
	Reader   io.Reader
	Filename string
	MIMEType string
	Caption  string
	URL      string // Public download URL for platforms that require a link (Instagram, WhatsApp).
}

// Messenger is the platform UI adapter interface.
// Each platform (Telegram, Instagram, WhatsApp) implements this to handle
// platform-specific message delivery.
type Messenger interface {
	SendText(chatID, text string) error
	SendFile(chatID string, file FileMessage) error
	// SendVideo sends a training video.
	// r is the Drive download stream; pass nil when cachedFileID is set (Telegram fast-path).
	// cachedFileID is a Telegram file_id from a previous upload; ignored on other platforms.
	// publicURL is used by Instagram/WhatsApp, which require a public URL instead of a stream.
	// protected enables Telegram's protect_content flag (no forwarding/saving); ignored elsewhere.
	// Returns the Telegram file_id after an upload so the caller can cache it; empty on other platforms.
	SendVideo(chatID string, r io.Reader, cachedFileID, publicURL, filename string, protected bool) (returnedFileID string, err error)
	SendMenu(chatID, text string, rows [][]MenuButton) error
	SendInlineOptions(chatID, text string, buttons []InlineButton) error
	SendInlineGrid(chatID, text string, rows [][]InlineButton) error
	EditInlineGrid(chatID, messageID, text string, rows [][]InlineButton) error
	SendContactRequest(chatID, text, buttonText string) error
	SendTyping(chatID string) error
	// SendUploadAction signals to the platform that the bot is uploading a file.
	// On Telegram this sends the "upload_video" chat action; other platforms ignore it.
	// It must be called repeatedly (every ~4 s) to keep the indicator alive.
	SendUploadAction(chatID string) error
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

func (m *loggingMessenger) SendFile(chatID string, file FileMessage) error {
	if err := m.inner.SendFile(chatID, file); err != nil {
		return err
	}
	text := file.Caption
	if text == "" {
		text = "[File: " + file.Filename + "]"
	}
	m.saveOutgoing(text)
	return nil
}

func (m *loggingMessenger) SendVideo(chatID string, r io.Reader, cachedFileID, publicURL, filename string, protected bool) (string, error) {
	returnedID, err := m.inner.SendVideo(chatID, r, cachedFileID, publicURL, filename, protected)
	if err != nil {
		return "", err
	}
	m.saveOutgoing("[Video: " + filename + "]")
	return returnedID, nil
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

func (m *loggingMessenger) EditInlineGrid(chatID, messageID, text string, rows [][]InlineButton) error {
	return m.inner.EditInlineGrid(chatID, messageID, text, rows)
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

func (m *loggingMessenger) SendUploadAction(chatID string) error {
	return m.inner.SendUploadAction(chatID)
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
	MessageID    string // ID of the message that triggered the callback (for editing)
}
