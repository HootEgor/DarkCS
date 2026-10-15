package core

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"

	"DarkCS/bot/chat"
	"DarkCS/entity"
	"DarkCS/internal/lib/fileurl"
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
				receiptMap[entity.ChatKey(r.Platform, r.UserID, r.Channel)] = r.ReadAt
			}
		}
	}

	// Count unread messages per chat
	unreadMap, err := c.repo.CountUnreadPerChat(receiptMap)
	if err != nil {
		c.log.Error("failed to count unread messages", slog.String("error", err.Error()))
	}

	channelNames := c.businessChannelNames()
	groupTitles, topicNames := c.groupNames()

	for i := range summaries {
		s := &summaries[i]
		key := entity.ChatKey(s.Platform, s.UserID, s.Channel)
		if unreadMap != nil {
			s.Unread = unreadMap[key]
		}

		if s.Platform == entity.PlatformTelegramGroup {
			// Names come from the preloaded maps instead of a lookup per chat.
			s.UserName = groupTitles[s.UserID]
			s.ContactKey = contactKey(s.Platform, s.UserID, nil)
			s.ChannelName = groupTopicName(topicNames[key], s.Channel)
			continue
		}

		user := c.lookupUserByPlatform(s.Platform, s.UserID)
		s.UserName, s.MessengerName = c.chatDisplayNames(s.Platform, s.UserID, user)
		s.ContactKey = contactKey(s.Platform, s.UserID, user)
		if s.Channel != "" {
			s.ChannelName = channelNames[s.Channel]
		}
	}

	return summaries, nil
}

// HandleMarkRead persists a read receipt and broadcasts it via WebSocket.
func (c *Core) HandleMarkRead(username, platform, userID, channel string) error {
	if err := c.repo.UpsertReadReceipt(username, platform, userID, channel, time.Now()); err != nil {
		return err
	}

	if c.wsHub != nil {
		c.wsHub.BroadcastReadReceipt(username, platform, userID, channel)
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
	case entity.PlatformTelegram, entity.PlatformTelegramBusiness:
		// Business chats are keyed by the client's Telegram id, the same as bot chats.
		telegramId, _ := strconv.ParseInt(userID, 10, 64)
		if telegramId != 0 {
			user, err = c.authService.GetUser("", "", telegramId)
		}
	case "instagram":
		user, err = c.authService.GetUserByInstagramId(userID)
	case "whatsapp":
		// wa_id is digits without "+"; GetUser normalizes it to the stored "+digits" form.
		user, err = c.authService.GetUser("", userID, 0)
	}

	if err != nil || user == nil {
		return nil
	}
	return user
}

// enrichMessageUser populates transient UserName, MessengerName and ContactKey fields
// on a ChatMessage from the user record for WebSocket broadcasts.
func (c *Core) enrichMessageUser(msg *entity.ChatMessage) {
	c.applyMessageUser(msg, c.lookupUserByPlatform(msg.Platform, msg.UserID))
}

// applyMessageUser fills the transient display fields of msg from an already looked-up user (may be nil).
func (c *Core) applyMessageUser(msg *entity.ChatMessage, user *entity.User) {
	msg.UserName, msg.MessengerName = c.chatDisplayNames(msg.Platform, msg.UserID, user)
	msg.ContactKey = contactKey(msg.Platform, msg.UserID, user)
}

// chatDisplayNames returns the person's name and messenger handle for the CRM. Telegram
// Business clients often never registered, so their cached Telegram profile is the fallback.
func (c *Core) chatDisplayNames(platform, userID string, user *entity.User) (name, messengerName string) {
	if platform == entity.PlatformTelegramGroup {
		// A group chat is not a person: it is named after the group (authors are per message).
		return c.groupTitle(userID), ""
	}
	if user != nil {
		name = user.Name
		switch platform {
		case entity.PlatformTelegram, entity.PlatformTelegramBusiness:
			messengerName = user.TelegramUsername
		case entity.PlatformInstagram:
			messengerName = user.InstagramUsername
		case entity.PlatformWhatsApp:
			messengerName = user.Phone
		}
	}
	if platform == entity.PlatformTelegramBusiness && (name == "" || messengerName == "") && c.repo != nil {
		if contact, err := c.repo.GetBusinessContact(userID); err == nil && contact != nil {
			if name == "" {
				name = contact.Name
			}
			if messengerName == "" && contact.Username != "" {
				messengerName = "@" + contact.Username
			}
		}
	}
	return name, messengerName
}

// contactKey groups the chats of one person across platforms and channels (see
// entity.ChatSummary). Bot and business chats share the Telegram id, so they group even
// when the client never registered.
func contactKey(platform, userID string, user *entity.User) string {
	if user != nil && user.UUID != "" {
		return user.UUID
	}
	if platform == entity.PlatformTelegram || platform == entity.PlatformTelegramBusiness {
		return "tg:" + userID
	}
	// platform:user_id also covers groups: all topics of one group share it.
	return platform + ":" + userID
}

// GetChatMessages returns paginated message history from MongoDB.
// Attachment URLs are populated at read-time so clients can download files.
func (c *Core) GetChatMessages(platform, userID, channel string, limit, offset int) ([]entity.ChatMessage, error) {
	messages, err := c.repo.GetChatMessages(platform, userID, channel, limit, offset)
	if err != nil {
		return nil, err
	}
	c.signAttachments(messages)
	return messages, nil
}

// ListMessagesSince returns messages changed after the (since, afterID) cursor across
// chats (see repository ListMessagesSince), with signed attachment URLs and the chat's
// person in human-readable form (user_name, messenger_name, user_phone; the group title
// for groups): the consumer keys contacts by platform + user_id, but people match and
// read them by name, @username and phone. contact_key stays empty.
func (c *Core) ListMessagesSince(ctx context.Context, since time.Time, afterID primitive.ObjectID, platforms []string, limit int) ([]entity.ChatMessage, error) {
	messages, err := c.repo.ListMessagesSince(ctx, since, afterID, platforms, limit)
	if err != nil {
		return nil, err
	}
	c.signAttachments(messages)
	c.labelMessagePeople(messages)
	return messages, nil
}

// labelMessagePeople fills the display fields of a page of messages with one user
// lookup per chat person (a page holds many messages of few chats) and one group
// listing for the whole page.
func (c *Core) labelMessagePeople(messages []entity.ChatMessage) {
	type person struct{ name, handle, phone string }
	people := make(map[string]person)
	var titles map[string]string
	for i := range messages {
		m := &messages[i]
		if m.Platform == entity.PlatformTelegramGroup {
			if titles == nil {
				titles, _ = c.groupNames()
			}
			m.UserName = titles[m.UserID]
			continue
		}
		key := m.Platform + ":" + m.UserID
		p, ok := people[key]
		if !ok {
			user := c.lookupUserByPlatform(m.Platform, m.UserID)
			p.name, p.handle = c.chatDisplayNames(m.Platform, m.UserID, user)
			if user != nil {
				p.phone = user.Phone
			}
			people[key] = p
		}
		m.UserName, m.MessengerName, m.UserPhone = p.name, p.handle, p.phone
	}
}

// signAttachments fills short-lived download URLs on every attachment of messages.
func (c *Core) signAttachments(messages []entity.ChatMessage) {
	for i := range messages {
		for j := range messages[i].Attachments {
			messages[i].Attachments[j].URL = fileurl.SignURL(messages[i].Attachments[j].FileID.Hex(), c.signingSecret, 15*time.Minute)
		}
	}
}

// crmMessenger returns the messenger a manager reply goes through. Telegram Business
// chats are answered as the premium account of the channel, which must still be
// connected with the reply right; group chats by the bot in the channel's topic.
func (c *Core) crmMessenger(platform, userID, channel string) (chat.Messenger, error) {
	if platform == entity.PlatformTelegramGroup {
		return c.groupCrmMessenger(userID, channel)
	}
	if platform != entity.PlatformTelegramBusiness {
		messenger, ok := c.messengers[platform]
		if !ok {
			return nil, fmt.Errorf("no messenger for platform: %s", platform)
		}
		return messenger, nil
	}

	if c.businessMessenger == nil {
		return nil, fmt.Errorf("no messenger for platform: %s", platform)
	}
	ownerID, err := strconv.ParseInt(channel, 10, 64)
	if err != nil || ownerID == 0 {
		return nil, fmt.Errorf("invalid business channel %q", channel)
	}
	conn, err := c.repo.GetBusinessConnectionByOwner(ownerID)
	if err != nil {
		return nil, fmt.Errorf("get business connection: %w", err)
	}
	if conn == nil || !conn.IsEnabled || !conn.CanReply {
		return nil, entity.ErrChannelUnavailable
	}
	return c.businessMessenger(conn.ConnectionID), nil
}

// textIDSender is implemented by messengers that report the platform id of a sent text
// (the Telegram messenger).
type textIDSender interface {
	SendTextReturningID(chatID, text string) (int64, error)
}

// SendCrmMessage sends a message from a manager to a user via their platform. operatorID
// is the CRM operator behind the reply (may be empty) and is stored on the message.
func (c *Core) SendCrmMessage(platform, userID, channel, text, operatorID string) error {
	messenger, err := c.crmMessenger(platform, userID, channel)
	if err != nil {
		return err
	}

	// For all platforms, chatID == userID. Business and group replies keep their Telegram
	// id so later edits/deletions can be mirrored and history imports dedupe against them.
	var tgMessageID int64
	keepsTgID := platform == entity.PlatformTelegramBusiness || platform == entity.PlatformTelegramGroup
	if s, ok := messenger.(textIDSender); ok && keepsTgID {
		tgMessageID, err = s.SendTextReturningID(userID, text)
	} else {
		err = messenger.SendText(userID, text)
	}
	if err != nil {
		return fmt.Errorf("send message to %s/%s: %w", platform, userID, err)
	}

	// Store as outgoing message with sender="manager"
	now := time.Now()
	msg := entity.ChatMessage{
		Platform:    platform,
		UserID:      userID,
		Channel:     channel,
		ChatID:      userID,
		Direction:   "outgoing",
		Sender:      "manager",
		Text:        text,
		CreatedAt:   now,
		UpdatedAt:   now,
		TgMessageID: tgMessageID,
		OperatorID:  operatorID,
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
		c.enrichMessageUser(&msg)
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

	var field string
	switch platform {
	case entity.PlatformTelegram, entity.PlatformTelegramBusiness:
		if user.TelegramUsername == messengerName {
			return
		}
		field = entity.UserFieldTelegramUsername
	case "instagram":
		if user.InstagramUsername == messengerName {
			return
		}
		field = entity.UserFieldInstagramUser
	default:
		return
	}

	if err := c.authService.UpdateUserFields(user, map[string]any{field: messengerName}); err != nil {
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
	return c.UploadAndSaveChatFile(entity.ChatMessage{
		Platform:  platform,
		UserID:    userID,
		ChatID:    userID,
		Direction: "incoming",
		Sender:    "user",
		Text:      caption,
	}, reader, filename, mimeType, size)
}

// UploadAndSaveChatFile uploads a file to GridFS and saves base (with the attachment
// added) as a chat message. base carries the chat key, direction, sender and caption, so
// Telegram Business can store files from either side and in its own channel.
func (c *Core) UploadAndSaveChatFile(base entity.ChatMessage, reader io.Reader, filename, mimeType string, size int64) error {
	if size > entity.MaxFileSize {
		return entity.FileTooLargeError(filename, size)
	}

	// Wrap reader with a size-limited reader to enforce the limit even when size is unknown or incorrect
	reader = io.LimitReader(reader, entity.MaxFileSize+1)

	uploader := "user"
	if base.Sender == "manager" {
		uploader = "manager"
	}
	meta := entity.FileMetadata{
		MIMEType: mimeType,
		Platform: base.Platform,
		UserID:   base.UserID,
		Uploader: uploader,
	}

	fileID, storedSize, err := c.repo.UploadFile(filename, reader, meta)
	if err != nil {
		return fmt.Errorf("upload file: %w", err)
	}

	if storedSize > entity.MaxFileSize {
		return entity.FileTooLargeError(filename, storedSize)
	}

	if size == 0 {
		size = storedSize
	}

	att := entity.Attachment{
		FileID:   fileID,
		Filename: filename,
		MIMEType: mimeType,
		Size:     size,
		URL:      fileurl.SignURL(fileID.Hex(), c.signingSecret, 15*time.Minute),
	}

	msg := base
	msg.Attachments = []entity.Attachment{att}
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}

	c.SaveAndBroadcastChatMessage(msg)
	return nil
}

// UploadFile stores a file in GridFS and returns the resulting object ID and stored size.
func (c *Core) UploadFile(filename string, reader io.Reader, meta entity.FileMetadata) (primitive.ObjectID, int64, error) {
	return c.repo.UploadFile(filename, reader, meta)
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
// It downloads each file from GridFS, sends it via the platform, then saves a single
// ChatMessage carrying operatorID (the CRM operator behind the send; may be empty).
func (c *Core) SendCrmFiles(platform, userID, channel, caption, operatorID string, attachments []entity.Attachment) error {
	messenger, err := c.crmMessenger(platform, userID, channel)
	if err != nil {
		return err
	}

	// Send caption only with the first file
	fileCaption := caption
	for _, att := range attachments {
		_, meta, reader, err := c.repo.DownloadFile(att.FileID)
		if err != nil {
			return fmt.Errorf("download file %s: %w", att.FileID.Hex(), err)
		}

		// Build a public file URL for platforms that send links instead of streaming bytes.
		fileURL := ""
		if c.publicURL != "" {
			fileURL = c.publicURL + "/api/v1" + fileurl.SignURL(att.FileID.Hex(), c.signingSecret, 15*time.Minute)
		}

		sendErr := messenger.SendFile(userID, chat.FileMessage{
			Reader:   reader,
			Filename: att.Filename,
			MIMEType: meta.MIMEType,
			Caption:  fileCaption,
			URL:      fileURL,
		})
		reader.Close()

		if sendErr != nil {
			return fmt.Errorf("send file to %s/%s: %w", platform, userID, sendErr)
		}

		fileCaption = ""
	}

	// Populate URLs for WebSocket broadcast
	for i := range attachments {
		attachments[i].URL = fileurl.SignURL(attachments[i].FileID.Hex(), c.signingSecret, 15*time.Minute)
	}

	now := time.Now()
	msg := entity.ChatMessage{
		Platform:    platform,
		UserID:      userID,
		Channel:     channel,
		ChatID:      userID,
		Direction:   "outgoing",
		Sender:      "manager",
		Text:        caption,
		Attachments: attachments,
		CreatedAt:   now,
		UpdatedAt:   now,
		OperatorID:  operatorID,
	}

	if err := c.repo.SaveChatMessage(msg); err != nil {
		c.log.Error("failed to save outgoing file message",
			slog.String("platform", platform),
			slog.String("user_id", userID),
			slog.String("error", err.Error()),
		)
	}

	if c.wsHub != nil {
		c.enrichMessageUser(&msg)
		c.wsHub.BroadcastMessage(msg)
	}

	return nil
}

// SaveAndBroadcastChatMessage saves a chat message and broadcasts it via WebSocket.
// If the user has a Zoho contact ID, the message is also buffered for Zoho Functions.
func (c *Core) SaveAndBroadcastChatMessage(msg entity.ChatMessage) {
	// The repository stamps updated_at too; doing it here as well keeps the broadcast
	// copy identical to what was stored.
	if msg.CreatedAt.IsZero() {
		msg.CreatedAt = time.Now()
	}
	if msg.UpdatedAt.IsZero() {
		msg.UpdatedAt = msg.CreatedAt
	}
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
		c.applyMessageUser(&msg, user)
		c.wsHub.BroadcastMessage(msg)
	}

	if c.zohoFn != nil && user != nil && user.ZohoId != "" {
		c.zohoFn.BufferMessage(user.ZohoId, entity.ZohoMessageItem{
			MessageID: fmt.Sprintf("%d", time.Now().UnixMilli()),
			ChatID:    msg.ChatID,
			Content:   msg.Text,
			Sender:    msg.Sender,
		})
	}
}
