package entity

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Chat platforms. PlatformTelegramBusiness is a private chat of a connected Telegram
// Business (Premium) account and PlatformTelegramGroup a Telegram group (or one forum
// topic of it) the bot was added to: the bot only relays both to the CRM, people reply.
const (
	PlatformTelegram         = "telegram"
	PlatformTelegramBusiness = "telegram_business"
	PlatformTelegramGroup    = "telegram_group"
	PlatformInstagram        = "instagram"
	PlatformWhatsApp         = "whatsapp"
)

// ChatMessage represents a single message in a CRM chat conversation.
//
// A chat is identified by (platform, user_id, channel). Channel is empty for the bot,
// Instagram and WhatsApp chats and is never stored then (legacy documents have no field);
// for telegram_business it is the business account owner's Telegram user id, so one
// client writing to several premium accounts gets a separate chat per account.
// For telegram_group, user_id is the group chat id and channel the forum topic id
// ("1" = General topic, empty for a group without topics), so every topic is its own chat.
type ChatMessage struct {
	ID          primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Platform    string             `json:"platform" bson:"platform"`
	UserID      string             `json:"user_id" bson:"user_id"`
	Channel     string             `json:"channel,omitempty" bson:"channel,omitempty"`
	ChatID      string             `json:"chat_id" bson:"chat_id"`
	Direction   string             `json:"direction" bson:"direction"` // "incoming" | "outgoing"
	Sender      string             `json:"sender" bson:"sender"`       // "user" | "manager" | "bot"
	Text        string             `json:"text" bson:"text"`
	Attachments []Attachment       `json:"attachments,omitempty" bson:"attachments,omitempty"`
	CreatedAt   time.Time          `json:"created_at" bson:"created_at"`
	// TgMessageID is the Telegram message id (telegram_business and telegram_group); it
	// matches later edits and deduplicates history imported from a Telegram Desktop export.
	TgMessageID int64 `json:"tg_message_id,omitempty" bson:"tg_message_id,omitempty"`
	// AuthorID, AuthorName and AuthorUsername identify the member who wrote a group chat
	// message (telegram_group only; a group has many writers, so user_id is not a person).
	// The CRM chooses whether to show the display name or the @username.
	AuthorID       int64  `json:"author_id,omitempty" bson:"author_id,omitempty"`
	AuthorName     string `json:"author_name,omitempty" bson:"author_name,omitempty"`
	AuthorUsername string `json:"author_username,omitempty" bson:"author_username,omitempty"`
	// Imported marks messages loaded from a chat history export rather than received live.
	Imported bool `json:"imported,omitempty" bson:"imported,omitempty"`
	// EditedAt, OriginalText and DeletedAt mirror edits and deletions made in Telegram
	// Business chats. Deleted messages are kept (soft delete) so the CRM keeps a record;
	// OriginalText is the text before the first edit.
	EditedAt      *time.Time `json:"edited_at,omitempty" bson:"edited_at,omitempty"`
	OriginalText  string     `json:"original_text,omitempty" bson:"original_text,omitempty"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty" bson:"deleted_at,omitempty"`
	UserName      string     `json:"user_name,omitempty" bson:"-"`
	MessengerName string     `json:"messenger_name,omitempty" bson:"-"`
	ContactKey    string     `json:"contact_key,omitempty" bson:"-"`
}

// ChatKey returns the unique "platform:user_id[:channel]" key of a chat. Legacy chats
// (empty channel) keep the old two-part key.
func ChatKey(platform, userID, channel string) string {
	if channel == "" {
		return platform + ":" + userID
	}
	return platform + ":" + userID + ":" + channel
}

// ChatReadReceipt tracks the last time a CRM user marked a chat as read.
type ChatReadReceipt struct {
	Username string    `json:"username" bson:"username"`
	Platform string    `json:"platform" bson:"platform"`
	UserID   string    `json:"user_id" bson:"user_id"`
	Channel  string    `json:"channel,omitempty" bson:"channel,omitempty"`
	ReadAt   time.Time `json:"read_at" bson:"read_at"`
}

// ChatSummary represents a chat summary for the CRM chat list.
//
// ContactKey groups the chats of one person: the user UUID when a registered user is
// linked, "tg:<telegram id>" for unlinked Telegram bot/business chats (they share the id),
// otherwise "platform:user_id". The CRM shows a channel picker when a key has several chats.
type ChatSummary struct {
	Platform      string    `json:"platform" bson:"platform"`
	UserID        string    `json:"user_id" bson:"user_id"`
	Channel       string    `json:"channel,omitempty" bson:"channel,omitempty"`
	ChannelName   string    `json:"channel_name,omitempty" bson:"-"`
	ContactKey    string    `json:"contact_key" bson:"-"`
	UserName      string    `json:"user_name" bson:"user_name"`
	MessengerName string    `json:"messenger_name" bson:"messenger_name"`
	LastMessage   string    `json:"last_message" bson:"last_message"`
	LastTime      time.Time `json:"last_time" bson:"last_time"`
	Unread        int       `json:"unread" bson:"unread"`
}
