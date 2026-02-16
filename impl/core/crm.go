package core

import (
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"DarkCS/bot/chat"
	"DarkCS/entity"
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
// Attachment URLs are populated at read-time so clients can download files.
func (c *Core) GetChatMessages(platform, userID string, limit, offset int) ([]entity.ChatMessage, error) {
	messages, err := c.repo.GetChatMessages(platform, userID, limit, offset)
	if err != nil {
		return nil, err
	}

	for i := range messages {
		for j := range messages[i].Attachments {
			messages[i].Attachments[j].URL = "/api/v1/crm/files/" + messages[i].Attachments[j].FileID.Hex()
		}
	}

	return messages, nil
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

// UploadAndSaveFile uploads a file to GridFS, saves a ChatMessage with the attachment, and broadcasts via WebSocket.
// Called by platform bots when receiving media from users.
func (c *Core) UploadAndSaveFile(platform, userID string, reader io.Reader, filename, mimeType string, size int64, caption string) error {
	if size > entity.MaxFileSize {
		return fmt.Errorf("file %q (%d bytes) exceeds the %d MB limit", filename, size, entity.MaxFileSize>>20)
	}

	// Wrap reader with a size-limited reader to enforce the limit even when size is unknown or incorrect
	reader = io.LimitReader(reader, entity.MaxFileSize+1)

	meta := entity.FileMetadata{
		MIMEType: mimeType,
		Platform: platform,
		UserID:   userID,
		Uploader: "user",
	}

	fileID, storedSize, err := c.repo.UploadFile(filename, reader, meta)
	if err != nil {
		return fmt.Errorf("upload file: %w", err)
	}

	if storedSize > entity.MaxFileSize {
		return fmt.Errorf("file %q (%d bytes) exceeds the %d MB limit", filename, storedSize, entity.MaxFileSize>>20)
	}

	if size == 0 {
		size = storedSize
	}

	att := entity.Attachment{
		FileID:   fileID,
		Filename: filename,
		MIMEType: mimeType,
		Size:     size,
		URL:      "/api/v1/crm/files/" + fileID.Hex(),
	}

	msg := entity.ChatMessage{
		Platform:    platform,
		UserID:      userID,
		ChatID:      userID,
		Direction:   "incoming",
		Sender:      "user",
		Text:        caption,
		Attachments: []entity.Attachment{att},
		CreatedAt:   time.Now(),
	}

	c.SaveAndBroadcastChatMessage(msg)
	return nil
}

// DownloadFile retrieves a file from GridFS by its ID.
// Returns the filename, MIME type, and a ReadCloser the caller must close.
func (c *Core) DownloadFile(fileID primitive.ObjectID) (string, string, io.ReadCloser, error) {
	filename, meta, reader, err := c.repo.DownloadFile(fileID)
	if err != nil {
		return "", "", nil, err
	}
	return filename, meta.MIMEType, reader, nil
}

// SendCrmFiles sends files from a manager to a user via their platform messenger.
// It downloads each file from GridFS, sends it via the platform, then saves a single ChatMessage.
func (c *Core) SendCrmFiles(platform, userID, caption string, attachments []entity.Attachment) error {
	messenger, ok := c.messengers[platform]
	if !ok {
		return fmt.Errorf("no messenger for platform: %s", platform)
	}

	// Send caption only with the first file
	fileCaption := caption
	for _, att := range attachments {
		_, meta, reader, err := c.repo.DownloadFile(att.FileID)
		if err != nil {
			return fmt.Errorf("download file %s: %w", att.FileID.Hex(), err)
		}

		sendErr := messenger.SendFile(userID, chat.FileMessage{
			Reader:   reader,
			Filename: att.Filename,
			MIMEType: meta.MIMEType,
			Caption:  fileCaption,
		})
		reader.Close()

		if sendErr != nil {
			return fmt.Errorf("send file to %s/%s: %w", platform, userID, sendErr)
		}

		fileCaption = ""
	}

	// Populate URLs for WebSocket broadcast
	for i := range attachments {
		attachments[i].URL = "/api/v1/crm/files/" + attachments[i].FileID.Hex()
	}

	msg := entity.ChatMessage{
		Platform:    platform,
		UserID:      userID,
		ChatID:      userID,
		Direction:   "outgoing",
		Sender:      "manager",
		Text:        caption,
		Attachments: attachments,
		CreatedAt:   time.Now(),
	}

	if err := c.repo.SaveChatMessage(msg); err != nil {
		c.log.Error("failed to save outgoing file message",
			slog.String("platform", platform),
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
	}

	if c.wsHub != nil {
		user := c.lookupUserByPlatform(msg.Platform, msg.UserID)
		if user != nil {
			msg.UserName = user.Name
		}
		c.wsHub.BroadcastMessage(msg)
	}

	return nil
}

// SaveAndBroadcastChatMessage saves a chat message and broadcasts it via WebSocket.
// If the user has a Zoho contact ID, the message is also buffered for Zoho Functions.
func (c *Core) SaveAndBroadcastChatMessage(msg entity.ChatMessage) {
	if err := c.repo.SaveChatMessage(msg); err != nil {
		c.log.Error("failed to save chat message",
			slog.String("platform", msg.Platform),
			slog.String("user_id", msg.UserID),
			slog.String("error", err.Error()),
		)
	}

	var user *entity.User
	if c.wsHub != nil || c.zohoFn != nil {
		user = c.lookupUserByPlatform(msg.Platform, msg.UserID)
	}

	if c.wsHub != nil {
		if user != nil {
			msg.UserName = user.Name
		}
		c.wsHub.BroadcastMessage(msg)
	}

	if c.zohoFn != nil && user != nil && user.ZohoId != "" {
		c.zohoFn.BufferMessage(user.ZohoId, entity.ZohoMessageItem{
			MessageID: fmt.Sprintf("%d", time.Now().UnixMilli()), // now unique!
			ChatID:    msg.ChatID,
			Content:   msg.Text,
			Sender:    msg.Sender,
		})
	}
}
