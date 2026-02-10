package bot

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"strconv"
	"time"

	"DarkCS/bot/chat"
	tgmessenger "DarkCS/bot/chat/telegram"
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
func (b *UserBot) handleStart(bot *tgbotapi.Bot, ctx *ext.Context) error {
	if b.chatEngine == nil {
		b.log.Warn("chat engine not initialized")
		return nil
	}

	userID := strconv.FormatInt(ctx.EffectiveUser.Id, 10)
	chatID := strconv.FormatInt(ctx.EffectiveChat.Id, 10)
	messenger := b.newMessenger()

	err := b.chatEngine.StartWorkflow(context.Background(), messenger, "telegram", userID, chatID, "onboarding")
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

	// Answer callback to remove loading indicator
	ctx.CallbackQuery.Answer(bot, nil)

	err := b.chatEngine.HandleCallback(context.Background(), messenger, "telegram", userID, chatID, data)
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

	err := b.chatEngine.HandleMessage(context.Background(), messenger, "telegram", userID, chatID, text)
	if err != nil {
		b.log.Error("message error",
			slog.String("user_id", userID),
			sl.Err(err),
		)
	}
	return err
}
