package bot

import (
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"DarkCS/entity"
	"DarkCS/internal/lib/sl"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
)

// Telegram group relay.
//
// The bot can be added to groups, usually forums where every topic is a separate thread.
// Group chats are human-only like business chats: every message is stored for the CRM
// as platform "telegram_group" (user_id = group chat id, channel = topic id) and managers
// reply from the CRM as the bot, into the same topic. Anyone can add a bot to a group, so
// a new group waits as pending until a CRM manager approves it; nothing is stored before.
// Every group message is "incoming" (staff writing in the group is not told apart); the
// author's name and @username are kept on each message.
//
// The bot only sees all group messages when it is a group admin or its privacy mode is
// off. The Bot API reports no deletions in groups, so those are not mirrored.
//
// The Bot API cannot list forum topics. Names come from topic created/edited service
// messages and, for topics created before the bot joined, from the topic's creation
// message that Telegram attaches as reply_to_message to ordinary messages in the topic.

// groupListener stores relayed group chats and their state; implemented by core.Core.
type groupListener interface {
	chatStore
	RegisterGroup(g entity.TelegramGroup) (*entity.TelegramGroup, error)
	GetTelegramGroup(chatID int64) (*entity.TelegramGroup, error)
	SetGroupLeft(chatID int64) error
	SetGroupTitle(chatID int64, title string) error
	SaveGroupTopic(chatID, threadID int64, name string, closed *bool) error
	NameGroupTopicIfUnknown(chatID, threadID int64, name string) error
	MigrateGroup(oldChatID, newChatID int64) error
	EditGroupMessage(userID, channel string, tgMessageID int64, text string, editedAt time.Time) error
}

// SetGroupListener enables the Telegram group relay.
func (b *UserBot) SetGroupListener(l groupListener) {
	b.groups = l
}

// LeaveGroup makes the bot leave a group (used when a group is rejected in the CRM).
func (b *UserBot) LeaveGroup(chatID int64) error {
	_, err := b.api.LeaveChat(chatID, nil)
	return err
}

func isGroupChat(chat *tgbotapi.Chat) bool {
	return chat != nil && (chat.Type == "group" || chat.Type == "supergroup")
}

// addGroupHandlers must be registered before the private-chat handlers: the dispatcher
// runs only the first matching handler, so group traffic is consumed here and never
// reaches the ChatEngine (onboarding/AI), even with the relay disabled.
func (b *UserBot) addGroupHandlers(dispatcher *ext.Dispatcher) {
	dispatcher.AddHandler(updateHandler{
		name:     "group_my_chat_member",
		check:    func(ctx *ext.Context) bool { return ctx.MyChatMember != nil && isGroupChat(&ctx.MyChatMember.Chat) },
		response: b.handleGroupMembership,
	})
	dispatcher.AddHandler(updateHandler{
		name:     "group_message",
		check:    func(ctx *ext.Context) bool { return ctx.Message != nil && isGroupChat(&ctx.Message.Chat) },
		response: b.handleGroupMessage,
	})
	dispatcher.AddHandler(updateHandler{
		name:     "group_edited",
		check:    func(ctx *ext.Context) bool { return ctx.EditedMessage != nil && isGroupChat(&ctx.EditedMessage.Chat) },
		response: b.handleGroupEdit,
	})
	// Inline buttons are never sent to groups; drop any callback from one.
	dispatcher.AddHandler(updateHandler{
		name: "group_callback",
		check: func(ctx *ext.Context) bool {
			return ctx.CallbackQuery != nil && ctx.CallbackQuery.Message != nil && isGroupChat(ptr(ctx.CallbackQuery.Message.GetChat()))
		},
		response: func(*tgbotapi.Bot, *ext.Context) error { return nil },
	})
}

func ptr[T any](v T) *T { return &v }

// handleGroupMembership tracks the bot being added to, restricted in or removed from a group.
func (b *UserBot) handleGroupMembership(_ *tgbotapi.Bot, ctx *ext.Context) error {
	if b.groups == nil {
		return nil
	}
	upd := ctx.MyChatMember
	member := upd.NewChatMember.MergeChatMember()

	if !isActiveMember(member) {
		if err := b.groups.SetGroupLeft(upd.Chat.Id); err != nil {
			b.log.Error("failed to mark group left", slog.Int64("chat_id", upd.Chat.Id), sl.Err(err))
			return err
		}
		b.log.Info("bot removed from group", slog.Int64("chat_id", upd.Chat.Id), slog.String("title", upd.Chat.Title))
		return nil
	}

	group, err := b.registerGroup(upd.Chat, &upd.From, member)
	if err != nil {
		return err
	}
	b.log.Info("bot added to group",
		slog.Int64("chat_id", group.ChatID),
		slog.String("title", group.Title),
		slog.String("status", group.Status),
		slog.Bool("can_post", group.CanPost),
		slog.Int64("added_by", upd.From.Id),
	)
	return nil
}

// isActiveMember reports whether the bot is (still) in the group.
func isActiveMember(m tgbotapi.MergedChatMember) bool {
	switch m.Status {
	case "creator", "administrator", "member":
		return true
	case "restricted":
		return m.IsMember
	}
	return false
}

// canPost reports whether the bot may send messages in the group. Chat-wide permissions
// are not checked; a failed send surfaces as an error in the CRM.
func canPost(m tgbotapi.MergedChatMember) bool {
	if m.Status == "restricted" {
		return m.CanSendMessages
	}
	return isActiveMember(m)
}

// registerGroup records the group (pending when new) and leaves it again when it was
// rejected before.
func (b *UserBot) registerGroup(chat tgbotapi.Chat, addedBy *tgbotapi.User, member tgbotapi.MergedChatMember) (*entity.TelegramGroup, error) {
	g := entity.TelegramGroup{
		ChatID:   chat.Id,
		Title:    chat.Title,
		Username: chat.Username,
		IsForum:  chat.IsForum,
		CanPost:  canPost(member),
	}
	if addedBy != nil {
		g.AddedByID = addedBy.Id
		g.AddedByName = userDisplayName(addedBy)
	}
	group, err := b.groups.RegisterGroup(g)
	if err != nil {
		b.log.Error("failed to register group", slog.Int64("chat_id", chat.Id), sl.Err(err))
		return nil, err
	}
	if group == nil {
		return nil, fmt.Errorf("group %d not stored", chat.Id)
	}
	if group.Status == entity.GroupStatusRejected {
		if err := b.LeaveGroup(chat.Id); err != nil {
			b.log.Warn("failed to leave rejected group", slog.Int64("chat_id", chat.Id), sl.Err(err))
		}
	}
	return group, nil
}

// group returns the stored group, registering it when the bot missed the my_chat_member
// update (e.g. added while the bot was down or before the relay existed).
func (b *UserBot) group(chat tgbotapi.Chat) (*entity.TelegramGroup, error) {
	group, err := b.groups.GetTelegramGroup(chat.Id)
	if err != nil || group != nil {
		return group, err
	}
	cm, err := b.api.GetChatMember(chat.Id, b.api.Id, nil)
	if err != nil {
		return nil, fmt.Errorf("get bot membership: %w", err)
	}
	return b.registerGroup(chat, nil, cm.MergeChatMember())
}

// groupChannel returns the CRM channel of a group message: the forum topic id, the
// General topic in forums, or "" for groups without topics. In non-forum supergroups
// message_thread_id marks reply threads, not topics, so it is ignored there.
func groupChannel(isForum, isTopicMessage bool, threadID int64) string {
	switch {
	case isTopicMessage && threadID > 0:
		return strconv.FormatInt(threadID, 10)
	case isForum:
		return strconv.FormatInt(entity.GeneralTopicID, 10)
	}
	return ""
}

// handleGroupMessage relays one group message to the CRM and keeps group/topic state.
func (b *UserBot) handleGroupMessage(_ *tgbotapi.Bot, ctx *ext.Context) error {
	if b.groups == nil {
		return nil
	}
	msg := ctx.Message

	// Group → supergroup upgrade: the old group gets migrate_to, the new one migrate_from.
	// Handle both before anything registers the new id as a separate group.
	if msg.MigrateToChatId != 0 || msg.MigrateFromChatId != 0 {
		oldID, newID := msg.Chat.Id, msg.MigrateToChatId
		if msg.MigrateFromChatId != 0 {
			oldID, newID = msg.MigrateFromChatId, msg.Chat.Id
		}
		if err := b.groups.MigrateGroup(oldID, newID); err != nil {
			b.log.Error("failed to migrate group", slog.Int64("old_chat_id", oldID), slog.Int64("new_chat_id", newID), sl.Err(err))
			return err
		}
		return nil
	}

	group, err := b.group(msg.Chat)
	if err != nil {
		b.log.Error("failed to load group", slog.Int64("chat_id", msg.Chat.Id), sl.Err(err))
		return err
	}

	// Topic names and the title are kept even while pending, so they are known on approval.
	b.learnTopicName(msg)
	if b.saveGroupServiceMessage(msg) {
		return nil
	}
	if group.Status != entity.GroupStatusApproved {
		return nil
	}
	if msg.From != nil && msg.From.Id == b.api.Id {
		return nil // our own CRM replies are already stored by the CRM
	}

	chatID := strconv.FormatInt(msg.Chat.Id, 10)
	base := entity.ChatMessage{
		Platform:    entity.PlatformTelegramGroup,
		UserID:      chatID,
		Channel:     groupChannel(msg.Chat.IsForum, msg.IsTopicMessage, msg.MessageThreadId),
		ChatID:      chatID,
		Direction:   "incoming",
		Sender:      "user",
		TgMessageID: msg.MessageId,
		CreatedAt:   time.Unix(msg.Date, 0),
	}
	base.AuthorID, base.AuthorName, base.AuthorUsername = messageAuthor(msg)

	if fileID, filename := messageMedia(msg); fileID != "" {
		base.Text = msg.Caption
		return b.saveRelayedFile(b.groups, base, fileID, filename)
	}
	base.Text = msg.Text
	if base.Text == "" {
		base.Text = relayPlaceholder(msg)
	}
	b.groups.SaveAndBroadcastChatMessage(base)
	return nil
}

// learnTopicName stores the name of a topic created before the bot joined: an ordinary
// (non-reply) topic message carries the topic's creation message as reply_to_message.
// It is the original name, so it is only stored when no name is known; a later rename
// (forum_topic_edited) still overwrites it. Each topic is written once per process.
func (b *UserBot) learnTopicName(msg *tgbotapi.Message) {
	name, ok := topicNameFromReply(msg)
	if !ok {
		return
	}
	key := strconv.FormatInt(msg.Chat.Id, 10) + ":" + strconv.FormatInt(msg.MessageThreadId, 10)
	if _, seen := b.namedTopics.LoadOrStore(key, struct{}{}); seen {
		return
	}
	if err := b.groups.NameGroupTopicIfUnknown(msg.Chat.Id, msg.MessageThreadId, name); err != nil {
		b.namedTopics.Delete(key) // retry on the next message
		b.log.Error("failed to save group topic name", slog.Int64("chat_id", msg.Chat.Id), sl.Err(err))
	}
}

// topicNameFromReply returns the topic name from the creation message attached to a topic message.
func topicNameFromReply(msg *tgbotapi.Message) (string, bool) {
	reply := msg.ReplyToMessage
	if !msg.IsTopicMessage || msg.MessageThreadId <= 0 || reply == nil || reply.ForumTopicCreated == nil {
		return "", false
	}
	if reply.MessageId != msg.MessageThreadId || reply.ForumTopicCreated.Name == "" {
		return "", false
	}
	return reply.ForumTopicCreated.Name, true
}

// saveGroupServiceMessage applies group service messages (topic and title changes) and
// reports whether msg was one; service messages are never stored as chat messages.
func (b *UserBot) saveGroupServiceMessage(msg *tgbotapi.Message) bool {
	chatID, threadID := msg.Chat.Id, msg.MessageThreadId
	var err error
	switch {
	case msg.ForumTopicCreated != nil:
		err = b.groups.SaveGroupTopic(chatID, threadID, msg.ForumTopicCreated.Name, ptr(false))
	case msg.ForumTopicEdited != nil:
		if msg.ForumTopicEdited.Name != "" {
			err = b.groups.SaveGroupTopic(chatID, threadID, msg.ForumTopicEdited.Name, nil)
		}
	case msg.ForumTopicClosed != nil:
		err = b.groups.SaveGroupTopic(chatID, threadID, "", ptr(true))
	case msg.ForumTopicReopened != nil:
		err = b.groups.SaveGroupTopic(chatID, threadID, "", ptr(false))
	case msg.NewChatTitle != "":
		err = b.groups.SetGroupTitle(chatID, msg.NewChatTitle)
	case len(msg.NewChatMembers) > 0, msg.LeftChatMember != nil, len(msg.NewChatPhoto) > 0,
		msg.DeleteChatPhoto, msg.GroupChatCreated, msg.SupergroupChatCreated, msg.PinnedMessage != nil,
		msg.MessageAutoDeleteTimerChanged != nil, msg.GeneralForumTopicHidden != nil,
		msg.GeneralForumTopicUnhidden != nil, msg.VideoChatScheduled != nil, msg.VideoChatStarted != nil,
		msg.VideoChatEnded != nil, msg.VideoChatParticipantsInvited != nil, msg.ChatBackgroundSet != nil,
		msg.BoostAdded != nil, msg.WriteAccessAllowed != nil, msg.ProximityAlertTriggered != nil:
	default:
		return false
	}
	if err != nil {
		b.log.Error("failed to apply group service message", slog.Int64("chat_id", chatID), sl.Err(err))
	}
	return true
}

// handleGroupEdit mirrors an edited group message (text or caption) of an approved group.
func (b *UserBot) handleGroupEdit(_ *tgbotapi.Bot, ctx *ext.Context) error {
	if b.groups == nil {
		return nil
	}
	msg := ctx.EditedMessage
	group, err := b.groups.GetTelegramGroup(msg.Chat.Id)
	if err != nil || group == nil || group.Status != entity.GroupStatusApproved {
		return err
	}

	text := msg.Text
	if text == "" {
		text = msg.Caption
	}
	editedAt := time.Now()
	if msg.EditDate != 0 {
		editedAt = time.Unix(msg.EditDate, 0)
	}

	userID := strconv.FormatInt(msg.Chat.Id, 10)
	channel := groupChannel(msg.Chat.IsForum, msg.IsTopicMessage, msg.MessageThreadId)
	if err := b.groups.EditGroupMessage(userID, channel, msg.MessageId, text, editedAt); err != nil {
		b.log.Error("failed to mirror group message edit",
			slog.String("user_id", userID),
			slog.Int64("message_id", msg.MessageId),
			sl.Err(err),
		)
		return err
	}
	return nil
}

// messageAuthor returns who wrote a group message. Anonymous admins and channels post
// as a chat (sender_chat), whose title then names the author.
func messageAuthor(msg *tgbotapi.Message) (id int64, name, username string) {
	if msg.SenderChat != nil {
		return msg.SenderChat.Id, msg.SenderChat.Title, msg.SenderChat.Username
	}
	if msg.From != nil {
		return msg.From.Id, userDisplayName(msg.From), msg.From.Username
	}
	return 0, "", ""
}

func userDisplayName(u *tgbotapi.User) string {
	return strings.TrimSpace(u.FirstName + " " + u.LastName)
}
