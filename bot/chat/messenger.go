package chat

// Messenger is the platform UI adapter interface.
// Each platform (Telegram, Instagram, WhatsApp) implements this to handle
// platform-specific message delivery.
type Messenger interface {
	SendText(chatID, text string) error
	SendMenu(chatID, text string, rows [][]MenuButton) error
	SendInlineOptions(chatID, text string, buttons []InlineButton) error
	SendContactRequest(chatID, text, buttonText string) error
	SendTyping(chatID string) error
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
