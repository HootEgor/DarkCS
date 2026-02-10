package core

import (
	"DarkCS/entity"
	"fmt"
	"log/slog"
	"time"
)

// GetActiveChats returns the list of active chats from MongoDB.
func (c *Core) GetActiveChats() ([]entity.ChatSummary, error) {
	return c.repo.GetActiveChats()
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
		c.wsHub.BroadcastMessage(msg)
	}
}
