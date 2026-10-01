package core

import (
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"time"

	"DarkCS/entity"
	"DarkCS/internal/lib/fileurl"
	"DarkCS/internal/lib/sl"
	"DarkCS/internal/lib/tgexport"
)

// Telegram Business (premium account) chats: the user bot relays them to the CRM and
// managers answer as the premium account. There is no AI or workflow involved. History
// from before the bot connection can be imported from a Telegram Desktop JSON export.

// SaveBusinessConnection stores the state of a business connection reported by Telegram.
func (c *Core) SaveBusinessConnection(conn entity.BusinessConnection) error {
	conn.UpdatedAt = time.Now()
	return c.repo.UpsertBusinessConnection(conn)
}

// GetBusinessConnection returns a stored business connection by Telegram connection id, or nil.
func (c *Core) GetBusinessConnection(connectionID string) (*entity.BusinessConnection, error) {
	return c.repo.GetBusinessConnectionByID(connectionID)
}

// SaveBusinessContact caches the Telegram profile of a business chat client.
func (c *Core) SaveBusinessContact(contact entity.BusinessContact) error {
	contact.UpdatedAt = time.Now()
	return c.repo.UpsertBusinessContact(contact)
}

// ListBusinessAccounts returns every connected (or formerly connected) premium account.
func (c *Core) ListBusinessAccounts() ([]entity.BusinessConnection, error) {
	return c.repo.ListBusinessConnections()
}

// EditBusinessMessage mirrors a message edited in a Telegram Business chat and notifies
// CRM clients. Edits of messages the CRM never stored are ignored.
func (c *Core) EditBusinessMessage(userID, channel string, tgMessageID int64, text string, editedAt time.Time) error {
	msg, err := c.repo.EditChatMessageText(entity.PlatformTelegramBusiness, userID, channel, tgMessageID, text, editedAt)
	if err != nil || msg == nil {
		return err
	}
	if c.wsHub != nil {
		for i := range msg.Attachments {
			msg.Attachments[i].URL = fileurl.SignURL(msg.Attachments[i].FileID.Hex(), c.signingSecret, 15*time.Minute)
		}
		c.enrichMessageUser(msg)
		c.wsHub.BroadcastMessageEdited(*msg)
	}
	return nil
}

// DeleteBusinessMessages mirrors messages deleted in a Telegram Business chat. They are
// only marked deleted so the CRM keeps the record.
func (c *Core) DeleteBusinessMessages(userID, channel string, tgMessageIDs []int64, deletedAt time.Time) error {
	marked, err := c.repo.MarkChatMessagesDeleted(entity.PlatformTelegramBusiness, userID, channel, tgMessageIDs, deletedAt)
	if err != nil || len(marked) == 0 {
		return err
	}
	if c.wsHub != nil {
		ids := make([]string, len(marked))
		tgIDs := make([]int64, len(marked))
		for i, m := range marked {
			ids[i] = m.ID.Hex()
			tgIDs[i] = m.TgMessageID
		}
		c.wsHub.BroadcastMessagesDeleted(entity.PlatformTelegramBusiness, userID, channel, ids, tgIDs, deletedAt)
	}
	return nil
}

// businessChannelNames maps a channel (owner id) to the premium account's display name.
func (c *Core) businessChannelNames() map[string]string {
	names := make(map[string]string)
	if c.repo == nil {
		return names
	}
	conns, err := c.repo.ListBusinessConnections()
	if err != nil {
		c.log.Error("failed to list business connections", sl.Err(err))
		return names
	}
	for _, conn := range conns {
		name := conn.OwnerName
		if name == "" && conn.OwnerUsername != "" {
			name = "@" + conn.OwnerUsername
		}
		names[strconv.FormatInt(conn.OwnerUserID, 10)] = name
	}
	return names
}

// parseBusinessChat validates the ids of a Telegram Business chat.
func parseBusinessChat(platform, userID, channel string) (clientID, ownerID int64, err error) {
	if platform != entity.PlatformTelegramBusiness {
		return 0, 0, fmt.Errorf("%w: history import is only available for %s chats", entity.ErrInvalidHistoryImport, entity.PlatformTelegramBusiness)
	}
	clientID, err = strconv.ParseInt(userID, 10, 64)
	if err != nil || clientID == 0 {
		return 0, 0, fmt.Errorf("%w: invalid user_id", entity.ErrInvalidHistoryImport)
	}
	ownerID, err = strconv.ParseInt(channel, 10, 64)
	if err != nil || ownerID == 0 {
		return 0, 0, fmt.Errorf("%w: invalid channel", entity.ErrInvalidHistoryImport)
	}
	return clientID, ownerID, nil
}

// GetHistoryStatus tells the CRM whether older history of a chat can be imported, whether
// it already was, and which accounts the manager has to export it from.
func (c *Core) GetHistoryStatus(platform, userID, channel string) (*entity.ChatHistoryStatus, error) {
	status := &entity.ChatHistoryStatus{}
	if platform != entity.PlatformTelegramBusiness {
		return status, nil
	}
	_, ownerID, err := parseBusinessChat(platform, userID, channel)
	if err != nil {
		return nil, err
	}
	status.Importable = true

	rec, err := c.repo.GetHistoryImport(platform, userID, channel)
	if err != nil {
		return nil, err
	}
	if rec != nil {
		status.Imported = true
		status.ImportedAt = &rec.ImportedAt
		status.ImportedCount = rec.Count
	}

	if status.HistoryBefore, err = c.repo.GetEarliestLiveMessageTime(platform, userID, channel); err != nil {
		return nil, err
	}

	conn, err := c.repo.GetBusinessConnectionByOwner(ownerID)
	if err != nil {
		return nil, err
	}
	if conn != nil {
		status.OwnerName = conn.OwnerName
		status.OwnerUsername = conn.OwnerUsername
	}

	status.ClientName, status.ClientUsername = c.chatDisplayNames(platform, userID, c.lookupUserByPlatform(platform, userID))
	return status, nil
}

// ImportChatHistory imports a Telegram Desktop export of a business chat. Only messages
// older than the first live (bot-received) message are taken, so history never overlaps
// what the bot already stored; re-imports skip messages already imported.
func (c *Core) ImportChatHistory(username, platform, userID, channel string, r io.Reader) (*entity.ChatHistoryImportResult, error) {
	clientID, ownerID, err := parseBusinessChat(platform, userID, channel)
	if err != nil {
		return nil, err
	}

	parsed, err := tgexport.Parse(r, clientID, ownerID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", entity.ErrInvalidHistoryImport, err)
	}

	cutoff, err := c.repo.GetEarliestLiveMessageTime(platform, userID, channel)
	if err != nil {
		return nil, err
	}

	result := &entity.ChatHistoryImportResult{}
	msgs := make([]entity.ChatMessage, 0, len(parsed))
	for _, p := range parsed {
		if cutoff != nil && !p.Time.Before(*cutoff) {
			continue
		}
		msg := entity.ChatMessage{
			Platform:    platform,
			UserID:      userID,
			Channel:     channel,
			ChatID:      userID,
			Direction:   "incoming",
			Sender:      "user",
			Text:        p.Text,
			CreatedAt:   p.Time,
			TgMessageID: p.ID,
			Imported:    true,
		}
		if p.FromOwner {
			msg.Direction = "outgoing"
			msg.Sender = "manager"
		}
		msgs = append(msgs, msg)

		t := p.Time
		if result.OldestAt == nil || t.Before(*result.OldestAt) {
			result.OldestAt = &t
		}
		if result.NewestAt == nil || t.After(*result.NewestAt) {
			result.NewestAt = &t
		}
	}

	inserted, err := c.repo.InsertImportedMessages(msgs)
	if err != nil {
		return nil, err
	}
	result.Imported = inserted
	result.Skipped = len(parsed) - inserted

	rec := entity.ChatHistoryImport{
		Platform:   platform,
		UserID:     userID,
		Channel:    channel,
		ImportedAt: time.Now(),
		ImportedBy: username,
		Count:      inserted,
	}
	if result.OldestAt != nil {
		rec.OldestAt, rec.NewestAt = *result.OldestAt, *result.NewestAt
	}
	if err = c.repo.SaveHistoryImport(rec); err != nil {
		c.log.Error("failed to record history import", slog.String("user_id", userID), sl.Err(err))
	}

	c.log.Info("chat history imported",
		slog.String("platform", platform),
		slog.String("user_id", userID),
		slog.String("channel", channel),
		slog.String("by", username),
		slog.Int("imported", inserted),
		slog.Int("skipped", result.Skipped),
	)

	if c.wsHub != nil && inserted > 0 {
		c.wsHub.BroadcastHistoryImported(platform, userID, channel, inserted)
	}
	return result, nil
}
