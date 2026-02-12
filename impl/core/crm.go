package core

import (
	"DarkCS/entity"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

// GetActiveChats returns the list of active chats from MongoDB, enriched with user names
// and per-user unread counts based on read receipts.
func (c *Core) GetActiveChats(username string) ([]entity.ChatSummary, error) {
	summaries, err := c.repo.GetActiveChats()
	if err != nil {
		return nil, err
	}

	// Build read receipts map for this CRM user
	receiptMap := make(map[string]time.Time)
	if username != "" {
		receipts, err := c.repo.GetReadReceipts(username)
		if err != nil {
			c.log.Error("failed to get read receipts",
				slog.String("username", username),
				slog.String("error", err.Error()),
			)
		} else {
			for _, r := range receipts {
				receiptMap[r.Platform+":"+r.UserID] = r.ReadAt
			}
		}
	}

	// Count unread messages per chat
	unreadMap, err := c.repo.CountUnreadPerChat(receiptMap)
	if err != nil {
		c.log.Error("failed to count unread messages", slog.String("error", err.Error()))
	}

	for i := range summaries {
		key := summaries[i].Platform + ":" + summaries[i].UserID
		if unreadMap != nil {
			summaries[i].Unread = unreadMap[key]
		}

		user := c.lookupUserByPlatform(summaries[i].Platform, summaries[i].UserID)
		if user != nil {
			summaries[i].UserName = user.Name
			switch summaries[i].Platform {
			case "telegram":
				summaries[i].MessengerName = user.TelegramUsername
			case "instagram":
				summaries[i].MessengerName = user.InstagramUsername
			}
		}
	}

	return summaries, nil
}

// HandleMarkRead persists a read receipt and broadcasts it via WebSocket.
func (c *Core) HandleMarkRead(username, platform, userID string) error {
	if err := c.repo.UpsertReadReceipt(username, platform, userID, time.Now()); err != nil {
		return err
	}

	if c.wsHub != nil {
		c.wsHub.BroadcastReadReceipt(username, platform, userID)
	}

	return nil
}

// lookupUserByPlatform finds a user by their platform-specific ID.
func (c *Core) lookupUserByPlatform(platform, userID string) *entity.User {
	if c.authService == nil {
		return nil
	}

	var user *entity.User
	var err error

	switch platform {
	case "telegram":
		telegramId, _ := strconv.ParseInt(userID, 10, 64)
		if telegramId != 0 {
			user, err = c.authService.GetUser("", "", telegramId)
		}
	case "instagram":
		user, err = c.authService.GetUserByInstagramId(userID)
	case "whatsapp":
		user, err = c.authService.GetUser("", userID, 0)
	}

	if err != nil || user == nil {
		return nil
	}
	return user
}

// GetChatMessages returns paginated message history from MongoDB.
func (c *Core) GetChatMessages(platform, userID string, limit, offset int) ([]entity.ChatMessage, error) {
	return c.repo.GetChatMessages(platform, userID, limit, offset)
}

// SendCrmMessage sends a message from a manager to a user via their platform.
func (c *Core) SendCrmMessage(platform, userID, text string) error {
	messenger, ok := c.messengers[platform]
	if !ok {
		return fmt.Errorf("no messenger for platform: %s", platform)
	}

	// For all platforms, chatID == userID
	if err := messenger.SendText(userID, text); err != nil {
		return fmt.Errorf("send message to %s/%s: %w", platform, userID, err)
	}

	// Store as outgoing message with sender="manager"
	msg := entity.ChatMessage{
		Platform:  platform,
		UserID:    userID,
		ChatID:    userID,
		Direction: "outgoing",
		Sender:    "manager",
		Text:      text,
		CreatedAt: time.Now(),
	}

	if err := c.repo.SaveChatMessage(msg); err != nil {
		c.log.Error("failed to save outgoing CRM message",
			slog.String("platform", platform),
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
	}

	// Broadcast to WebSocket so other managers see it
	if c.wsHub != nil {
		c.wsHub.BroadcastMessage(msg)
	}

	return nil
}

// UpdateUserPlatformInfo saves a platform-specific username for the user.
func (c *Core) UpdateUserPlatformInfo(platform, userID, messengerName string) {
	if c.authService == nil || messengerName == "" {
		return
	}

	user := c.lookupUserByPlatform(platform, userID)
	if user == nil {
		return
	}

	switch platform {
	case "telegram":
		if user.TelegramUsername == messengerName {
			return
		}
		user.TelegramUsername = messengerName
	case "instagram":
		if user.InstagramUsername == messengerName {
			return
		}
		user.InstagramUsername = messengerName
	default:
		return
	}

	if err := c.authService.UpdateUser(user); err != nil {
		c.log.Error("failed to update platform username",
			slog.String("platform", platform),
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
	}
}

// SaveAndBroadcastChatMessage saves a chat message and broadcasts it via WebSocket.
func (c *Core) SaveAndBroadcastChatMessage(msg entity.ChatMessage) {
	if err := c.repo.SaveChatMessage(msg); err != nil {
		c.log.Error("failed to save chat message",
			slog.String("platform", msg.Platform),
			slog.String("user_id", msg.UserID),
			slog.String("error", err.Error()),
		)
	}

	if c.wsHub != nil {
		if user := c.lookupUserByPlatform(msg.Platform, msg.UserID); user != nil {
			msg.UserName = user.Name
		}
		c.wsHub.BroadcastMessage(msg)
	}
}
