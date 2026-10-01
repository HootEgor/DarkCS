package bot

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"DarkCS/entity"
	"DarkCS/internal/lib/sl"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/businessconnection"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
)

// Telegram Business relay.
//
// Telegram Premium accounts can connect this bot as their chatbot (Settings → Telegram
// Business → Chatbots). The bot then receives every message of the account's private
// chats, both from clients and from the owner. These chats are human-only: messages are
// stored for the CRM as platform "telegram_business" with the owner id as channel, and
// managers reply from the CRM as the premium account. Nothing here talks to the client.

// allowedUpdates is passed to getUpdates explicitly: Telegram remembers the last list, so
// relying on the default could silently drop business updates after any narrower setting.
var allowedUpdates = []string{
	"message",
	"callback_query",
	"business_connection",
	"business_message",
	"edited_business_message",
	"deleted_business_messages",
}

// businessListener stores relayed business chats; implemented by core.Core.
type businessListener interface {
	SaveAndBroadcastChatMessage(msg entity.ChatMessage)
	UploadAndSaveChatFile(base entity.ChatMessage, reader io.Reader, filename, mimeType string, size int64) error
	SaveBusinessConnection(conn entity.BusinessConnection) error
	GetBusinessConnection(connectionID string) (*entity.BusinessConnection, error)
	SaveBusinessContact(contact entity.BusinessContact) error
	EditBusinessMessage(userID, channel string, tgMessageID int64, text string, editedAt time.Time) error
	DeleteBusinessMessages(userID, channel string, tgMessageIDs []int64, deletedAt time.Time) error
}

// updateHandler is a minimal ext.Handler for update types gotgbot has no handler for
// (deleted business messages) or that must not overlap other handlers.
type updateHandler struct {
	name     string
	check    func(ctx *ext.Context) bool
	response handlers.Response
}

func (h updateHandler) CheckUpdate(_ *tgbotapi.Bot, ctx *ext.Context) bool { return h.check(ctx) }
func (h updateHandler) HandleUpdate(b *tgbotapi.Bot, ctx *ext.Context) error {
	return h.response(b, ctx)
}
func (h updateHandler) Name() string { return h.name }

// SetBusinessListener enables the Telegram Business relay.
func (b *UserBot) SetBusinessListener(l businessListener) {
	b.business = l
}

func (b *UserBot) addBusinessHandlers(dispatcher *ext.Dispatcher) {
	if b.business == nil {
		return
	}
	dispatcher.AddHandler(handlers.BusinessConnection{Filter: businessconnection.All, Response: b.handleBusinessConnection})
	dispatcher.AddHandler(handlers.NewMessage(message.Business, b.handleBusinessMessage).SetAllowBusiness(true))
	dispatcher.AddHandler(updateHandler{
		name:     "business_edited",
		check:    func(ctx *ext.Context) bool { return ctx.EditedBusinessMessage != nil },
		response: b.handleBusinessEdit,
	})
	dispatcher.AddHandler(updateHandler{
		name:     "business_deleted",
		check:    func(ctx *ext.Context) bool { return ctx.DeletedBusinessMessages != nil },
		response: b.handleBusinessDelete,
	})
}

// handleBusinessEdit mirrors a message edited by the client or the owner. Edits of
// media messages change the caption. Only the text is mirrored, not replaced media.
func (b *UserBot) handleBusinessEdit(_ *tgbotapi.Bot, ctx *ext.Context) error {
	msg := ctx.EditedBusinessMessage
	conn, err := b.businessConnection(msg.BusinessConnectionId)
	if err != nil {
		b.log.Error("unknown business connection", slog.String("connection_id", msg.BusinessConnectionId), sl.Err(err))
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
	channel := strconv.FormatInt(conn.OwnerUserID, 10)
	if err := b.business.EditBusinessMessage(userID, channel, msg.MessageId, text, editedAt); err != nil {
		b.log.Error("failed to mirror business message edit",
			slog.String("user_id", userID),
			slog.Int64("message_id", msg.MessageId),
			sl.Err(err),
		)
		return err
	}
	return nil
}

// handleBusinessDelete mirrors messages deleted in a business chat (by either side).
func (b *UserBot) handleBusinessDelete(_ *tgbotapi.Bot, ctx *ext.Context) error {
	del := ctx.DeletedBusinessMessages
	conn, err := b.businessConnection(del.BusinessConnectionId)
	if err != nil {
		b.log.Error("unknown business connection", slog.String("connection_id", del.BusinessConnectionId), sl.Err(err))
		return err
	}

	userID := strconv.FormatInt(del.Chat.Id, 10)
	channel := strconv.FormatInt(conn.OwnerUserID, 10)
	if err := b.business.DeleteBusinessMessages(userID, channel, del.MessageIds, time.Now()); err != nil {
		b.log.Error("failed to mirror business message deletion",
			slog.String("user_id", userID),
			slog.Int("count", len(del.MessageIds)),
			sl.Err(err),
		)
		return err
	}
	return nil
}

// handleBusinessConnection records a premium account connecting, changing rights or
// disconnecting the bot.
func (b *UserBot) handleBusinessConnection(_ *tgbotapi.Bot, ctx *ext.Context) error {
	conn := toBusinessConnection(ctx.BusinessConnection)
	if err := b.business.SaveBusinessConnection(conn); err != nil {
		b.log.Error("failed to save business connection", slog.Int64("owner_id", conn.OwnerUserID), sl.Err(err))
		return err
	}
	b.log.Info("business connection updated",
		slog.Int64("owner_id", conn.OwnerUserID),
		slog.String("owner", conn.OwnerName),
		slog.Bool("enabled", conn.IsEnabled),
		slog.Bool("can_reply", conn.CanReply),
	)
	return nil
}

func toBusinessConnection(bc *tgbotapi.BusinessConnection) entity.BusinessConnection {
	return entity.BusinessConnection{
		OwnerUserID:   bc.User.Id,
		ConnectionID:  bc.Id,
		OwnerName:     strings.TrimSpace(bc.User.FirstName + " " + bc.User.LastName),
		OwnerUsername: bc.User.Username,
		IsEnabled:     bc.IsEnabled,
		CanReply:      bc.Rights != nil && bc.Rights.CanReply,
	}
}

// businessConnection returns the stored connection, asking Telegram when the bot missed
// the business_connection update (e.g. connected while the bot was down).
func (b *UserBot) businessConnection(connectionID string) (*entity.BusinessConnection, error) {
	conn, err := b.business.GetBusinessConnection(connectionID)
	if err != nil || conn != nil {
		return conn, err
	}
	bc, err := b.api.GetBusinessConnection(connectionID, nil)
	if err != nil {
		return nil, fmt.Errorf("get business connection: %w", err)
	}
	c := toBusinessConnection(bc)
	if err := b.business.SaveBusinessConnection(c); err != nil {
		b.log.Error("failed to save business connection", slog.Int64("owner_id", c.OwnerUserID), sl.Err(err))
	}
	return &c, nil
}

// classifyBusinessMessage decides how a business chat message is stored. Messages from
// the owner (typed in the Telegram app) are manager messages; messages our bot sent on
// the owner's behalf come back as updates too and are skipped, as the CRM already saved them.
func classifyBusinessMessage(fromID, ownerID, senderBotID, ourBotID int64) (direction, sender string, skip bool) {
	if senderBotID != 0 && senderBotID == ourBotID {
		return "", "", true
	}
	if fromID == ownerID {
		return "outgoing", "manager", false
	}
	return "incoming", "user", false
}

// handleBusinessMessage relays one message of a premium account chat to the CRM.
func (b *UserBot) handleBusinessMessage(_ *tgbotapi.Bot, ctx *ext.Context) error {
	msg := ctx.BusinessMessage
	if msg == nil || msg.BusinessConnectionId == "" {
		return nil
	}

	conn, err := b.businessConnection(msg.BusinessConnectionId)
	if err != nil {
		b.log.Error("unknown business connection", slog.String("connection_id", msg.BusinessConnectionId), sl.Err(err))
		return err
	}

	var fromID, senderBotID int64
	if msg.From != nil {
		fromID = msg.From.Id
	}
	if msg.SenderBusinessBot != nil {
		senderBotID = msg.SenderBusinessBot.Id
	}
	direction, sender, skip := classifyBusinessMessage(fromID, conn.OwnerUserID, senderBotID, b.api.Id)
	if skip {
		return nil
	}

	// In a private chat the chat is the client, whoever wrote the message.
	clientID := strconv.FormatInt(msg.Chat.Id, 10)
	if err := b.business.SaveBusinessContact(entity.BusinessContact{
		UserID:   clientID,
		Name:     strings.TrimSpace(msg.Chat.FirstName + " " + msg.Chat.LastName),
		Username: msg.Chat.Username,
	}); err != nil {
		b.log.Warn("failed to save business contact", slog.String("user_id", clientID), sl.Err(err))
	}

	base := entity.ChatMessage{
		Platform:    entity.PlatformTelegramBusiness,
		UserID:      clientID,
		Channel:     strconv.FormatInt(conn.OwnerUserID, 10),
		ChatID:      clientID,
		Direction:   direction,
		Sender:      sender,
		TgMessageID: msg.MessageId,
		CreatedAt:   time.Unix(msg.Date, 0),
	}

	if fileID, filename := messageMedia(msg); fileID != "" {
		base.Text = msg.Caption
		return b.saveBusinessFile(base, fileID, filename)
	}

	base.Text = msg.Text
	if base.Text == "" {
		base.Text = businessPlaceholder(msg)
	}
	b.business.SaveAndBroadcastChatMessage(base)
	return nil
}

// saveBusinessFile stores business chat media. On failure the message is still saved as
// a placeholder so managers know something was sent; the client is never answered.
func (b *UserBot) saveBusinessFile(base entity.ChatMessage, fileID, filename string) error {
	body, mimeType, size, err := b.downloadFile(fileID)
	if err == nil {
		err = b.business.UploadAndSaveChatFile(base, body, filename, mimeType, size)
		body.Close()
	}
	if err == nil {
		return nil
	}

	b.log.Error("failed to store business chat file",
		slog.String("user_id", base.UserID),
		slog.String("channel", base.Channel),
		sl.Err(err),
	)
	note := "[Файл не збережено: " + filename + "]"
	if errors.Is(err, entity.ErrFileTooLarge) {
		note = fmt.Sprintf("[Файл більше %d MB не збережено: %s]", entity.MaxFileSize>>20, filename)
	}
	if base.Text != "" {
		note += "\n" + base.Text
	}
	base.Text = note
	b.business.SaveAndBroadcastChatMessage(base)
	return nil
}

// businessPlaceholder describes messages without text or downloadable media.
func businessPlaceholder(msg *tgbotapi.Message) string {
	switch {
	case msg.Sticker != nil:
		return strings.TrimSpace("[Стікер] " + msg.Sticker.Emoji)
	case msg.Contact != nil:
		return strings.TrimSpace("[Контакт] " + msg.Contact.FirstName + " " + msg.Contact.PhoneNumber)
	case msg.Location != nil:
		return fmt.Sprintf("[Локація] %.6f, %.6f", msg.Location.Latitude, msg.Location.Longitude)
	}
	return "[Непідтримуваний тип повідомлення]"
}
