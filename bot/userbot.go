package bot

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"DarkCS/bot/chat"
	tgmessenger "DarkCS/bot/chat/telegram"
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
)

// UserBot is the Telegram bot for general users using the unified ChatEngine.
type UserBot struct {
	log         *slog.Logger
	api         *tgbotapi.Bot
	botUsername string
	chatEngine  *chat.ChatEngine
}

// NewUserBot creates a new user bot instance.
func NewUserBot(botName, apiKey string, log *slog.Logger) (*UserBot, error) {
	bot := &UserBot{
		log:         log.With(sl.Module("userbot")),
		botUsername: botName,
	}

	api, err := tgbotapi.NewBot(apiKey, nil)
	if err != nil {
		return nil, fmt.Errorf("creating api instance: %v", err)
	}
	bot.api = api

	return bot, nil
}

// SetChatEngine sets the unified chat engine for the bot.
func (b *UserBot) SetChatEngine(engine *chat.ChatEngine) {
	b.chatEngine = engine
}

// GetAPI returns the underlying Telegram bot API for creating messengers.
func (b *UserBot) GetAPI() *tgbotapi.Bot {
	return b.api
}

// Start begins polling for updates and handling them.
func (b *UserBot) Start() error {
	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		Error: func(bot *tgbotapi.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Println("an error occurred while handling update:", err.Error())
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})
	updater := ext.NewUpdater(dispatcher, nil)

	dispatcher.AddHandler(handlers.NewCommand("start", b.handleStart))
	dispatcher.AddHandler(handlers.NewCallback(func(cq *tgbotapi.CallbackQuery) bool { return true }, b.handleCallback))
	dispatcher.AddHandler(handlers.NewMessage(message.Contact, b.handleContact))
	dispatcher.AddHandler(handlers.NewMessage(message.Text, b.handleMessage))

	err := updater.StartPolling(b.api, &ext.PollingOpts{
		DropPendingUpdates: true,
		GetUpdatesOpts: &tgbotapi.GetUpdatesOpts{
			Timeout: 9,
			RequestOpts: &tgbotapi.RequestOpts{
				Timeout: time.Second * 10,
			},
		},
	})
	if err != nil {
		return fmt.Errorf("failed to start polling: %w", err)
	}

	b.log.Info("user bot started", slog.String("username", b.botUsername))

	updater.Idle()

	return nil
}

func (b *UserBot) newMessenger() *tgmessenger.Messenger {
	return tgmessenger.NewMessenger(b.api)
}

// handleStart handles the /start command — always starts onboarding.
// If the message contains a deep link payload (e.g. /start ZGw6Mjg5MjM0),
// it is decoded from base64 as "type:id" and passed into the workflow state.
func (b *UserBot) handleStart(bot *tgbotapi.Bot, ctx *ext.Context) error {
	if b.chatEngine == nil {
		b.log.Warn("chat engine not initialized")
		return nil
	}

	userID := strconv.FormatInt(ctx.EffectiveUser.Id, 10)
	chatID := strconv.FormatInt(ctx.EffectiveChat.Id, 10)
	messenger := b.newMessenger()

	var initialData map[string]any
	if parts := strings.Fields(ctx.EffectiveMessage.Text); len(parts) > 1 {
		decoded, err := base64.StdEncoding.DecodeString(parts[1])
		if err == nil {
			if kv := strings.SplitN(string(decoded), ":", 2); len(kv) == 2 {
				initialData = map[string]any{
					"deep_link_type": kv[0],
					"deep_link_id":   kv[1],
				}
				b.log.Info("deep link parsed",
					slog.String("user_id", userID),
					slog.String("type", kv[0]),
					slog.String("id", kv[1]),
				)
			}
		}
	}

	err := b.chatEngine.StartWorkflowWithData(context.Background(), messenger, "telegram", userID, chatID, "onboarding", initialData)
	if err != nil {
		b.log.Error("failed to start onboarding",
			slog.String("user_id", userID),
			sl.Err(err),
		)
		return err
	}

	return nil
}

// handleCallback handles inline keyboard callbacks.
func (b *UserBot) handleCallback(bot *tgbotapi.Bot, ctx *ext.Context) error {
	if b.chatEngine == nil {
		return nil
	}

	userID := strconv.FormatInt(ctx.EffectiveUser.Id, 10)
	chatID := strconv.FormatInt(ctx.EffectiveChat.Id, 10)
	data := ctx.CallbackQuery.Data
	messenger := b.newMessenger()

	// Extract message ID for inline message editing
	var messageID string
	if msg := ctx.CallbackQuery.Message; msg != nil {
		messageID = strconv.FormatInt(msg.GetMessageId(), 10)
	}

	// Answer callback to remove loading indicator
	ctx.CallbackQuery.Answer(bot, nil)

	err := b.chatEngine.HandleCallback(context.Background(), messenger, "telegram", userID, chatID, data, messageID)
	if err != nil {
		b.log.Error("callback error",
			slog.String("user_id", userID),
			slog.String("data", data),
			sl.Err(err),
		)
	}
	return err
}

// handleContact handles contact sharing.
func (b *UserBot) handleContact(bot *tgbotapi.Bot, ctx *ext.Context) error {
	if b.chatEngine == nil {
		return nil
	}

	userID := strconv.FormatInt(ctx.EffectiveUser.Id, 10)
	chatID := strconv.FormatInt(ctx.EffectiveChat.Id, 10)
	messenger := b.newMessenger()

	contact := ctx.EffectiveMessage.Contact
	if contact == nil {
		return nil
	}

	phone := contact.PhoneNumber
	err := b.chatEngine.HandleContact(context.Background(), messenger, "telegram", userID, chatID, phone)
	if err != nil {
		b.log.Error("contact error",
			slog.String("user_id", userID),
			sl.Err(err),
		)
	}
	return err
}

// handleMessage handles text messages.
func (b *UserBot) handleMessage(bot *tgbotapi.Bot, ctx *ext.Context) error {
	if b.chatEngine == nil {
		return nil
	}

	userID := strconv.FormatInt(ctx.EffectiveUser.Id, 10)
	chatID := strconv.FormatInt(ctx.EffectiveChat.Id, 10)
	text := ctx.EffectiveMessage.Text
	messenger := b.newMessenger()

	// Save incoming message for CRM
	if listener := b.chatEngine.GetMessageListener(); listener != nil {
		listener.SaveAndBroadcastChatMessage(entity.ChatMessage{
			Platform:  "telegram",
			UserID:    userID,
			ChatID:    chatID,
			Direction: "incoming",
			Sender:    "user",
			Text:      text,
			CreatedAt: time.Now(),
		})

		// Save Telegram @username
		if username := ctx.EffectiveUser.Username; username != "" {
			listener.UpdateUserPlatformInfo("telegram", userID, "@"+username)
		}
	}

	err := b.chatEngine.HandleMessage(context.Background(), messenger, "telegram", userID, chatID, text)
	if err != nil {
		b.log.Error("message error",
			slog.String("user_id", userID),
			sl.Err(err),
		)
	}
	return err
}
