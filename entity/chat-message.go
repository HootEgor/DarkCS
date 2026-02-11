package entity

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// ChatMessage represents a single message in a CRM chat conversation.
type ChatMessage struct {
	ID        primitive.ObjectID `json:"id" bson:"_id,omitempty"`
	Platform  string             `json:"platform" bson:"platform"`
	UserID    string             `json:"user_id" bson:"user_id"`
	ChatID    string             `json:"chat_id" bson:"chat_id"`
	Direction string             `json:"direction" bson:"direction"` // "incoming" | "outgoing"
	Sender    string             `json:"sender" bson:"sender"`       // "user" | "manager" | "bot"
	Text      string             `json:"text" bson:"text"`
	CreatedAt time.Time          `json:"created_at" bson:"created_at"`
}

// ChatReadReceipt tracks the last time a CRM user marked a chat as read.
type ChatReadReceipt struct {
	Username string    `json:"username" bson:"username"`
	Platform string    `json:"platform" bson:"platform"`
	UserID   string    `json:"user_id" bson:"user_id"`
	ReadAt   time.Time `json:"read_at" bson:"read_at"`
}

// ChatSummary represents a chat summary for the CRM chat list.
type ChatSummary struct {
	Platform      string    `json:"platform" bson:"platform"`
	UserID        string    `json:"user_id" bson:"user_id"`
	UserName      string    `json:"user_name" bson:"user_name"`
	MessengerName string    `json:"messenger_name" bson:"messenger_name"`
	LastMessage   string    `json:"last_message" bson:"last_message"`
	LastTime      time.Time `json:"last_time" bson:"last_time"`
	Unread        int       `json:"unread" bson:"unread"`
}
