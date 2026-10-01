package entity

import (
	"errors"
	"time"
)

// Errors whose text is safe to show to the CRM manager.
var (
	// ErrChannelUnavailable: the business account disconnected the bot or did not grant
	// it the right to reply, so a CRM message cannot be delivered.
	ErrChannelUnavailable = errors.New("premium account is disconnected or the bot has no right to reply")
	// ErrInvalidHistoryImport wraps a rejected chat history export.
	ErrInvalidHistoryImport = errors.New("invalid history export")
)

// BusinessConnection is a Telegram Business (Premium) account that connected our user
// bot as its chatbot. It is keyed by OwnerUserID because the connection id changes when
// the owner reconnects the bot, while the owner id is the stable chat channel.
type BusinessConnection struct {
	OwnerUserID   int64     `json:"owner_user_id" bson:"owner_user_id"`
	ConnectionID  string    `json:"-" bson:"connection_id"` // lets the bot write as the account; not exposed to the CRM
	OwnerName     string    `json:"name" bson:"owner_name"`
	OwnerUsername string    `json:"username,omitempty" bson:"owner_username,omitempty"`
	IsEnabled     bool      `json:"is_enabled" bson:"is_enabled"`
	CanReply      bool      `json:"can_reply" bson:"can_reply"`
	UpdatedAt     time.Time `json:"updated_at" bson:"updated_at"`
}

// BusinessContact caches the Telegram profile of a client who wrote to a business
// account, so the CRM can name chats of people who never registered via the bot.
type BusinessContact struct {
	UserID    string    `json:"user_id" bson:"user_id"`
	Name      string    `json:"name" bson:"name"`
	Username  string    `json:"username,omitempty" bson:"username,omitempty"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}

// ChatHistoryImport records that older history of a chat was imported from a Telegram
// Desktop export, so the CRM stops offering the import.
type ChatHistoryImport struct {
	Platform   string    `json:"platform" bson:"platform"`
	UserID     string    `json:"user_id" bson:"user_id"`
	Channel    string    `json:"channel" bson:"channel"`
	ImportedAt time.Time `json:"imported_at" bson:"imported_at"`
	ImportedBy string    `json:"imported_by" bson:"imported_by"`
	Count      int       `json:"count" bson:"count"`
	OldestAt   time.Time `json:"oldest_at" bson:"oldest_at"`
	NewestAt   time.Time `json:"newest_at" bson:"newest_at"`
}

// ChatHistoryStatus tells the CRM whether older history of a chat can or has been
// imported and where (which accounts) the manager should export it from.
type ChatHistoryStatus struct {
	Importable     bool       `json:"importable"`
	Imported       bool       `json:"imported"`
	ImportedAt     *time.Time `json:"imported_at,omitempty"`
	ImportedCount  int        `json:"imported_count,omitempty"`
	HistoryBefore  *time.Time `json:"history_before,omitempty"`
	OwnerName      string     `json:"owner_name,omitempty"`
	OwnerUsername  string     `json:"owner_username,omitempty"`
	ClientName     string     `json:"client_name,omitempty"`
	ClientUsername string     `json:"client_username,omitempty"`
}

// ChatHistoryImportResult is returned after importing a history export.
type ChatHistoryImportResult struct {
	Imported int        `json:"imported"`
	Skipped  int        `json:"skipped"`
	OldestAt *time.Time `json:"oldest_at,omitempty"`
	NewestAt *time.Time `json:"newest_at,omitempty"`
}
