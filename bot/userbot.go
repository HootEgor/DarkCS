package bot

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"DarkCS/bot/chat"
	chatmainmenu "DarkCS/bot/chat/mainmenu"
	tgmessenger "DarkCS/bot/chat/telegram"
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers"
	"github.com/PaulSonOfLars/gotgbot/v2/ext/handlers/filters/message"
)

// userBotAuthService is the minimal auth interface used by the user bot for admin checks.
type userBotAuthService interface {
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
}

// UserBot is the Telegram bot for general users using the unified ChatEngine.
// It also relays chats of connected Telegram Business accounts to the CRM
// (see userbot_business.go); those never reach the ChatEngine.
type UserBot struct {
	log         *slog.Logger
	api         *tgbotapi.Bot
	botUsername string
	chatEngine  *chat.ChatEngine
	authService userBotAuthService
	business    businessListener            // nil = Telegram Business updates are ignored
	updater     atomic.Pointer[ext.Updater] // set by Start, used by Stop
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

// SetAuthService sets the auth service used to verify admin status in commands.
func (b *UserBot) SetAuthService(svc userBotAuthService) {
	b.authService = svc
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
	b.updater.Store(updater)

	dispatcher.AddHandler(handlers.NewCommand("start", b.handleStart))
	dispatcher.AddHandler(handlers.NewCommand("reset", b.handleReset))
	dispatcher.AddHandler(handlers.NewCallback(func(cq *tgbotapi.CallbackQuery) bool { return true }, b.handleCallback))
	dispatcher.AddHandler(handlers.NewMessage(message.Contact, b.handleContact))
	dispatcher.AddHandler(handlers.NewMessage(message.Photo, b.handleMedia))
	dispatcher.AddHandler(handlers.NewMessage(message.Document, b.handleMedia))
	dispatcher.AddHandler(handlers.NewMessage(message.Audio, b.handleMedia))
	dispatcher.AddHandler(handlers.NewMessage(message.Video, b.handleMedia))
	dispatcher.AddHandler(handlers.NewMessage(message.Voice, b.handleMedia))
	dispatcher.AddHandler(handlers.NewMessage(message.Text, b.handleMessage))
	// The handlers above only match ordinary messages (AllowBusiness is off by default),
	// so premium account chats never start onboarding or reach the AI.
	b.addBusinessHandlers(dispatcher)

	err := updater.StartPolling(b.api, &ext.PollingOpts{
		DropPendingUpdates: true,
		GetUpdatesOpts: &tgbotapi.GetUpdatesOpts{
			Timeout:        9,
			AllowedUpdates: allowedUpdates,
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

// Stop ends polling and waits for in-flight update handlers; Start then returns.
func (b *UserBot) Stop() {
	if u := b.updater.Load(); u != nil {
		if err := u.Stop(); err != nil {
			b.log.Error("stopping user bot", sl.Err(err))
		}
	}
}

func (b *UserBot) newMessenger() *tgmessenger.Messenger {
	return tgmessenger.NewMessenger(b.api)
}

// handleReset is an admin-only command that resets all users currently in the
// ai_consultant or make_order steps back to the main_menu step.
func (b *UserBot) handleReset(bot *tgbotapi.Bot, ctx *ext.Context) error {
	if b.authService == nil || b.chatEngine == nil {
		return nil
	}

	user, err := b.authService.GetUser("", "", ctx.EffectiveUser.Id)
	if err != nil || user == nil || !user.IsAdmin() {
		return nil
	}

	steps := []chat.StepID{chatmainmenu.StepAIConsultant, chatmainmenu.StepMakeOrder, chatmainmenu.StepSelectVideo}
	count, err := b.chatEngine.ResetUsersAtSteps(context.Background(), chatmainmenu.WorkflowID, steps, chatmainmenu.StepMainMenu)
	if err != nil {
		b.log.Error("handleReset: failed to reset users", sl.Err(err))
		_, _ = ctx.EffectiveMessage.Reply(bot, "Помилка при скиданні кроків.", nil)
		return err
	}

	reply := fmt.Sprintf("Скинуто %d користувачів на головне меню.", count)
	_, _ = ctx.EffectiveMessage.Reply(bot, reply, nil)
	return nil
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
	// A user can share any contact card; only their own contact proves the phone is theirs.
	verified := contact.UserId != 0 && contact.UserId == ctx.EffectiveUser.Id
	err := b.chatEngine.HandleContact(context.Background(), messenger, "telegram", userID, chatID, phone, verified)
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

// handleMedia handles photo, document, audio, video, and voice messages.
func (b *UserBot) handleMedia(bot *tgbotapi.Bot, ctx *ext.Context) error {
	if b.chatEngine == nil {
		return nil
	}

	listener := b.chatEngine.GetMessageListener()
	if listener == nil {
		return nil
	}

	userID := strconv.FormatInt(ctx.EffectiveUser.Id, 10)
	msg := ctx.EffectiveMessage

	fileID, filename := messageMedia(msg)
	if fileID == "" {
		return nil
	}
	caption := msg.Caption

	body, mimeType, size, err := b.downloadFile(fileID)
	if err != nil {
		b.log.Error("failed to download file from Telegram",
			slog.String("user_id", userID),
			slog.String("file_id", fileID),
			sl.Err(err),
		)
		return err
	}
	defer body.Close()

	// Upload to GridFS and save message
	if err := listener.UploadAndSaveFile("telegram", userID, body, filename, mimeType, size, caption); err != nil {
		b.log.Error("failed to upload and save file",
			slog.String("user_id", userID),
			sl.Err(err),
		)
		if errors.Is(err, entity.ErrFileTooLarge) {
			chatID := ctx.EffectiveChat.Id
			limitMB := entity.MaxFileSize >> 20
			text := fmt.Sprintf("Файл занадто великий. Максимально дозволений розмір - %d MB.", limitMB)
			_, _ = bot.SendMessage(chatID, text, nil)
		}
		return err
	}

	// Save Telegram @username
	if username := ctx.EffectiveUser.Username; username != "" {
		listener.UpdateUserPlatformInfo("telegram", userID, "@"+username)
	}

	// Route caption text to ChatEngine if present
	if caption != "" {
		chatIDStr := strconv.FormatInt(ctx.EffectiveChat.Id, 10)
		messenger := b.newMessenger()
		_ = b.chatEngine.HandleMessage(context.Background(), messenger, "telegram", userID, chatIDStr, caption)
	}

	return nil
}

// messageMedia returns the file id and a filename of the message's media, or "" when
// the message carries no downloadable media.
func messageMedia(msg *tgbotapi.Message) (fileID, filename string) {
	withDefault := func(name, def string) string {
		if name == "" {
			return def
		}
		return name
	}
	switch {
	case len(msg.Photo) > 0:
		// Use largest photo size
		return msg.Photo[len(msg.Photo)-1].FileId, "photo.jpg"
	case msg.Document != nil:
		return msg.Document.FileId, msg.Document.FileName
	case msg.Audio != nil:
		return msg.Audio.FileId, withDefault(msg.Audio.FileName, "audio.mp3")
	case msg.Video != nil:
		return msg.Video.FileId, withDefault(msg.Video.FileName, "video.mp4")
	case msg.Voice != nil:
		return msg.Voice.FileId, "voice.ogg"
	case msg.VideoNote != nil:
		return msg.VideoNote.FileId, "video_note.mp4"
	case msg.Animation != nil:
		return msg.Animation.FileId, withDefault(msg.Animation.FileName, "animation.mp4")
	}
	return "", ""
}

// downloadFile streams a Telegram file; the caller must close the body.
func (b *UserBot) downloadFile(fileID string) (body io.ReadCloser, mimeType string, size int64, err error) {
	file, err := b.api.GetFile(fileID, nil)
	if err != nil {
		return nil, "", 0, fmt.Errorf("get file: %w", err)
	}

	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", b.api.Token, file.FilePath)
	resp, err := telegramFileClient.Get(fileURL)
	if err != nil {
		return nil, "", 0, fmt.Errorf("download file: %w", sl.RedactURLError(err))
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", 0, fmt.Errorf("download file: status %d", resp.StatusCode)
	}
	return resp.Body, mimeFromPath(file.FilePath), file.FileSize, nil
}

// mimeFromPath detects the MIME type from the Telegram file path extension.
func mimeFromPath(filePath string) string {
	mimeType := "application/octet-stream"
	fileExt := strings.ToLower(path.Ext(filePath))
	switch fileExt {
	case ".jpg", ".jpeg":
		mimeType = "image/jpeg"
	case ".png":
		mimeType = "image/png"
	case ".gif":
		mimeType = "image/gif"
	case ".webp":
		mimeType = "image/webp"
	case ".mp4":
		mimeType = "video/mp4"
	case ".mp3":
		mimeType = "audio/mpeg"
	case ".ogg", ".oga":
		mimeType = "audio/ogg"
	case ".pdf":
		mimeType = "application/pdf"
	case ".webm":
		mimeType = "video/webm"
	}
	return mimeType
}

// telegramFileClient downloads user media from Telegram with a bound on the transfer.
var telegramFileClient = &http.Client{Timeout: 2 * time.Minute}
