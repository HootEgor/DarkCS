package entity

import (
	"errors"
	"time"
)

// ErrGroupUnavailable: the Telegram group is not approved, the bot left it or may not
// post there, so a CRM message cannot be delivered. Safe to show to the manager.
var ErrGroupUnavailable = errors.New("telegram group is not approved, the bot was removed or cannot post there")

// ErrGroupNotFound is returned for approve/reject/rename of a group the bot never joined.
var ErrGroupNotFound = errors.New("telegram group not found")

// Telegram group statuses. Anyone can add the bot to a group, so a new group starts as
// pending and nothing is relayed until a CRM manager approves it; rejecting makes the bot leave.
const (
	GroupStatusPending  = "pending"
	GroupStatusApproved = "approved"
	GroupStatusRejected = "rejected"
	GroupStatusLeft     = "left" // the bot was removed (or left after a reject)
)

// GeneralTopicID is the thread id of a forum's General topic. Messages there carry no
// message_thread_id and Telegram rejects sends with thread id 1, so it is only used as the
// CRM channel.
const GeneralTopicID int64 = 1

// TelegramGroup is a Telegram group or supergroup the user bot was added to. It is keyed
// by ChatID; a group upgraded to a supergroup gets a new id and is migrated.
type TelegramGroup struct {
	ChatID      int64      `json:"chat_id" bson:"chat_id"`
	Title       string     `json:"title" bson:"title"`
	Username    string     `json:"username,omitempty" bson:"username,omitempty"`
	IsForum     bool       `json:"is_forum" bson:"is_forum"`
	Status      string     `json:"status" bson:"status"`
	CanPost     bool       `json:"can_post" bson:"can_post"`
	AddedByID   int64      `json:"added_by_id,omitempty" bson:"added_by_id,omitempty"`
	AddedByName string     `json:"added_by_name,omitempty" bson:"added_by_name,omitempty"`
	ReviewedBy  string     `json:"reviewed_by,omitempty" bson:"reviewed_by,omitempty"`
	ReviewedAt  *time.Time `json:"reviewed_at,omitempty" bson:"reviewed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at" bson:"updated_at"`
}

// TelegramGroupTopic caches a forum topic name. The Bot API cannot list topics, so names
// come from topic created/edited service messages or a manual rename in the CRM.
type TelegramGroupTopic struct {
	ChatID    int64     `json:"chat_id" bson:"chat_id"`
	ThreadID  int64     `json:"thread_id" bson:"thread_id"`
	Name      string    `json:"name" bson:"name"`
	Closed    bool      `json:"closed" bson:"closed"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}
