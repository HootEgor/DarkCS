package core

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"DarkCS/bot/chat"
	"DarkCS/entity"
	"DarkCS/internal/lib/fileurl"
	"DarkCS/internal/lib/sl"
)

// Telegram group chats: the user bot relays groups (each forum topic as its own chat) to
// the CRM and managers answer as the bot. Like business chats there is no AI involved.
// Anyone can add the bot to a group, so groups need CRM approval before anything is
// stored; a rejected group is left.

// GroupLeaver makes the bot leave a Telegram group; implemented by bot.UserBot.
type GroupLeaver interface {
	LeaveGroup(chatID int64) error
}

// RegisterGroup records the bot joining (or its rights changing in) a group and returns
// the stored group with its current status.
func (c *Core) RegisterGroup(g entity.TelegramGroup) (*entity.TelegramGroup, error) {
	now := time.Now()
	g.Status = entity.GroupStatusPending
	g.CreatedAt, g.UpdatedAt = now, now
	stored, err := c.repo.RegisterTelegramGroup(g)
	if err != nil {
		return nil, err
	}
	c.broadcastGroup(stored)
	return stored, nil
}

// SetGroupLeft records the bot being removed from a group.
func (c *Core) SetGroupLeft(chatID int64) error {
	if err := c.repo.MarkTelegramGroupLeft(chatID, time.Now()); err != nil {
		return err
	}
	c.broadcastGroupByID(chatID)
	return nil
}

// GetTelegramGroup returns a group by chat id, or nil when the bot never joined it.
func (c *Core) GetTelegramGroup(chatID int64) (*entity.TelegramGroup, error) {
	return c.repo.GetTelegramGroup(chatID)
}

// SetGroupTitle stores a group's new title.
func (c *Core) SetGroupTitle(chatID int64, title string) error {
	return c.repo.SetTelegramGroupTitle(chatID, title)
}

// SaveGroupTopic stores a forum topic name (empty keeps the stored one) and/or its closed flag.
func (c *Core) SaveGroupTopic(chatID, threadID int64, name string, closed *bool) error {
	return c.repo.UpsertGroupTopic(chatID, threadID, name, closed)
}

// NameGroupTopicIfUnknown stores a topic name learned from its creation message (attached
// to ordinary topic messages) unless a name is already known.
func (c *Core) NameGroupTopicIfUnknown(chatID, threadID int64, name string) error {
	return c.repo.NameGroupTopicIfUnknown(chatID, threadID, name)
}

// MigrateGroup moves a group upgraded to a supergroup to its new chat id.
func (c *Core) MigrateGroup(oldChatID, newChatID int64) error {
	if err := c.repo.MigrateTelegramGroup(oldChatID, newChatID); err != nil {
		return err
	}
	c.log.Info("telegram group migrated to supergroup",
		slog.Int64("old_chat_id", oldChatID),
		slog.Int64("new_chat_id", newChatID),
	)
	c.broadcastGroupByID(newChatID)
	return nil
}

// EditGroupMessage mirrors a message edited in a group and notifies CRM clients.
func (c *Core) EditGroupMessage(userID, channel string, tgMessageID int64, text string, editedAt time.Time) error {
	return c.editMirroredMessage(entity.PlatformTelegramGroup, userID, channel, tgMessageID, text, editedAt)
}

// ListTelegramGroups returns groups with the given status (all when empty).
func (c *Core) ListTelegramGroups(status string) ([]entity.TelegramGroup, error) {
	return c.repo.ListTelegramGroups(status)
}

// ListGroupTopics returns the known topics of a group.
func (c *Core) ListGroupTopics(chatID int64) ([]entity.TelegramGroupTopic, error) {
	return c.repo.ListGroupTopics(chatID)
}

// RenameGroupTopic sets a topic name from the CRM, for topics created before the bot
// joined (the Bot API cannot list them, so their names are otherwise unknown).
func (c *Core) RenameGroupTopic(chatID, threadID int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("topic name is required")
	}
	g, err := c.repo.GetTelegramGroup(chatID)
	if err != nil {
		return err
	}
	if g == nil {
		return entity.ErrGroupNotFound
	}
	return c.repo.UpsertGroupTopic(chatID, threadID, name, nil)
}

// ApproveGroup starts relaying a group to the CRM.
func (c *Core) ApproveGroup(chatID int64, username string) (*entity.TelegramGroup, error) {
	return c.reviewGroup(chatID, entity.GroupStatusApproved, username)
}

// RejectGroup stops relaying a group and makes the bot leave it. The status stays
// rejected, so if someone adds the bot again it leaves at once.
func (c *Core) RejectGroup(chatID int64, username string) (*entity.TelegramGroup, error) {
	g, err := c.reviewGroup(chatID, entity.GroupStatusRejected, username)
	if err != nil {
		return nil, err
	}
	if c.groupLeaver != nil {
		if err := c.groupLeaver.LeaveGroup(chatID); err != nil {
			// The group stays rejected: nothing is relayed even if the bot is still a member.
			c.log.Warn("failed to leave rejected group", slog.Int64("chat_id", chatID), sl.Err(err))
		}
	}
	return g, nil
}

func (c *Core) reviewGroup(chatID int64, status, username string) (*entity.TelegramGroup, error) {
	g, err := c.repo.GetTelegramGroup(chatID)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, entity.ErrGroupNotFound
	}
	if err = c.repo.SetTelegramGroupStatus(chatID, status, username, time.Now()); err != nil {
		return nil, err
	}
	c.log.Info("telegram group reviewed",
		slog.Int64("chat_id", chatID),
		slog.String("title", g.Title),
		slog.String("status", status),
		slog.String("by", username),
	)
	g, err = c.repo.GetTelegramGroup(chatID)
	if err != nil {
		return nil, err
	}
	c.broadcastGroup(g)
	return g, nil
}

// groupCrmMessenger returns the messenger a manager reply in a group chat goes through:
// the bot itself, posting into the chat's topic. The group must be approved and postable.
func (c *Core) groupCrmMessenger(userID, channel string) (chat.Messenger, error) {
	if c.groupMessenger == nil {
		return nil, fmt.Errorf("no messenger for platform: %s", entity.PlatformTelegramGroup)
	}
	chatID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil || chatID == 0 {
		return nil, fmt.Errorf("invalid group chat id %q", userID)
	}
	var threadID int64
	if channel != "" {
		if threadID, err = strconv.ParseInt(channel, 10, 64); err != nil || threadID <= 0 {
			return nil, fmt.Errorf("invalid group topic %q", channel)
		}
	}
	g, err := c.repo.GetTelegramGroup(chatID)
	if err != nil {
		return nil, fmt.Errorf("get telegram group: %w", err)
	}
	if g == nil || g.Status != entity.GroupStatusApproved || !g.CanPost {
		return nil, entity.ErrGroupUnavailable
	}
	return c.groupMessenger(threadID), nil
}

// groupNames returns display names for group chats: group titles by chat id and topic
// names by entity.ChatKey(telegram_group, chat id, thread id).
func (c *Core) groupNames() (titles, topics map[string]string) {
	titles, topics = make(map[string]string), make(map[string]string)
	if c.repo == nil {
		return titles, topics
	}
	groups, err := c.repo.ListTelegramGroups("")
	if err != nil {
		c.log.Error("failed to list telegram groups", sl.Err(err))
	}
	for _, g := range groups {
		titles[strconv.FormatInt(g.ChatID, 10)] = g.Title
	}
	list, err := c.repo.ListGroupTopics(0)
	if err != nil {
		c.log.Error("failed to list group topics", sl.Err(err))
	}
	for _, t := range list {
		key := entity.ChatKey(entity.PlatformTelegramGroup, strconv.FormatInt(t.ChatID, 10), strconv.FormatInt(t.ThreadID, 10))
		topics[key] = t.Name
	}
	return titles, topics
}

// groupTopicName names a topic chat, falling back for topics whose name the bot never saw.
func groupTopicName(known, channel string) string {
	switch {
	case channel == "":
		return ""
	case known != "":
		return known
	case channel == strconv.FormatInt(entity.GeneralTopicID, 10):
		return "General"
	}
	return "Тема #" + channel
}

// groupTitle returns the stored title of a group chat (user_id = chat id).
func (c *Core) groupTitle(userID string) string {
	if c.repo == nil {
		return ""
	}
	chatID, err := strconv.ParseInt(userID, 10, 64)
	if err != nil {
		return ""
	}
	g, err := c.repo.GetTelegramGroup(chatID)
	if err != nil || g == nil {
		return ""
	}
	return g.Title
}

func (c *Core) broadcastGroupByID(chatID int64) {
	if c.wsHub == nil {
		return
	}
	g, err := c.repo.GetTelegramGroup(chatID)
	if err != nil {
		c.log.Error("failed to load telegram group", slog.Int64("chat_id", chatID), sl.Err(err))
		return
	}
	c.broadcastGroup(g)
}

// broadcastGroup tells CRM clients a group was added, reviewed or left (e.g. to badge
// pending groups).
func (c *Core) broadcastGroup(g *entity.TelegramGroup) {
	if c.wsHub != nil && g != nil {
		c.wsHub.BroadcastGroupUpdated(*g)
	}
}

// editMirroredMessage mirrors a message edited in Telegram (business or group chat) and
// notifies CRM clients. Edits of messages the CRM never stored are ignored.
func (c *Core) editMirroredMessage(platform, userID, channel string, tgMessageID int64, text string, editedAt time.Time) error {
	msg, err := c.repo.EditChatMessageText(platform, userID, channel, tgMessageID, text, editedAt)
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
