package bot

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"time"

	"DarkCS/bot/workflow"
	"DarkCS/bot/workflows/onboarding"
	"DarkCS/internal/lib/sl"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
)

// UserBot is the Telegram bot for general users with workflow support.
type UserBot struct {
	log            *slog.Logger
	api            *tgbotapi.Bot
	botUsername    string
	workflowEngine workflow.Engine
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

// SetWorkflowEngine sets the workflow engine for the bot.
func (b *UserBot) SetWorkflowEngine(engine workflow.Engine) {
	b.workflowEngine = engine
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

	// Workflow handlers
	dispatcher.AddHandler(handlers.NewCommand("start", b.handleStart))
	dispatcher.AddHandler(handlers.NewCallback(b.workflowCallbackFilter, b.handleCallback))
	dispatcher.AddHandler(handlers.NewMessage(message.Contact, b.handleContact))
	dispatcher.AddHandler(handlers.NewMessage(message.Text, b.handleMessage))

	// Start receiving updates
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

	// Idle, to keep updates coming in
	updater.Idle()

	return nil
}

// workflowCallbackFilter filters callbacks that belong to workflows.
func (b *UserBot) workflowCallbackFilter(cq *tgbotapi.CallbackQuery) bool {
	return workflow.IsWorkflowCallback(cq.Data)
}

// handleStart handles the /start command and initiates workflows.
func (b *UserBot) handleStart(bot *tgbotapi.Bot, ctx *ext.Context) error {
	if b.workflowEngine == nil {
		b.log.Warn("workflow engine not initialized")
		return nil
	}

	userID := ctx.EffectiveUser.Id
	chatID := ctx.EffectiveChat.Id

	// Parse deep link from start command
	startParam := workflow.ExtractStartParam(ctx.EffectiveMessage.Text)
	var deepLink *workflow.DeepLinkData
	if startParam != "" {
		deepLink = workflow.ParseDeepLink(startParam)
		b.log.Debug("parsed deep link",
			slog.Int64("user_id", userID),
			slog.String("type", deepLink.Type),
			slog.String("code", deepLink.Code),
		)
	}

	// Start onboarding workflow
	err := b.workflowEngine.StartWorkflow(context.Background(), bot, userID, chatID, onboarding.WorkflowID, deepLink)
	if err != nil {
		b.log.Error("failed to start workflow",
			slog.Int64("user_id", userID),
			sl.Err(err),
		)
		return err
	}

	return nil
}

// handleCallback handles inline keyboard callbacks for workflows.
func (b *UserBot) handleCallback(bot *tgbotapi.Bot, ctx *ext.Context) error {
	if b.workflowEngine == nil {
		return nil
	}

	data := ctx.CallbackQuery.Data
	err := b.workflowEngine.HandleCallback(context.Background(), bot, ctx, data)
	if err != nil {
		b.log.Error("workflow callback error",
			slog.Int64("user_id", ctx.EffectiveUser.Id),
			slog.String("data", data),
			sl.Err(err),
		)
	}
	return err
}

// handleContact handles contact sharing for workflows.
func (b *UserBot) handleContact(bot *tgbotapi.Bot, ctx *ext.Context) error {
	if b.workflowEngine == nil {
		return nil
	}

	userID := ctx.EffectiveUser.Id

	// Check if user has active workflow
	hasWorkflow, err := b.workflowEngine.HasActiveWorkflow(context.Background(), userID)
	if err != nil {
		b.log.Error("check active workflow error", sl.Err(err))
		return err
	}

	if !hasWorkflow {
		return nil
	}

	err = b.workflowEngine.HandleContact(context.Background(), bot, ctx)
	if err != nil {
		b.log.Error("workflow contact error",
			slog.Int64("user_id", userID),
			sl.Err(err),
		)
	}
	return err
}

// handleMessage handles text messages for workflows.
func (b *UserBot) handleMessage(bot *tgbotapi.Bot, ctx *ext.Context) error {
	if b.workflowEngine == nil {
		return nil
	}

	userID := ctx.EffectiveUser.Id

	// Check if user has active workflow
	hasWorkflow, err := b.workflowEngine.HasActiveWorkflow(context.Background(), userID)
	if err != nil {
		b.log.Error("check active workflow error", sl.Err(err))
		return err
	}

	if !hasWorkflow {
		return nil
	}

	err = b.workflowEngine.HandleMessage(context.Background(), bot, ctx)
	if err != nil {
		b.log.Error("workflow message error",
			slog.Int64("user_id", userID),
			sl.Err(err),
		)
	}
	return err
}
