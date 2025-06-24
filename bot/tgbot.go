package bot

import (
	"DarkCS/internal/lib/sl"
	"fmt"
	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
	"github.com/PaulSonOfLars/gotgbot/v2/ext"
	"log"
	"log/slog"
	"strings"
	"time"
)

type TgBot struct {
	log         *slog.Logger
	api         *tgbotapi.Bot
	botUsername string
	adminId     int64
}

func NewTgBot(botName, apiKey string, adminId int64, log *slog.Logger) (*TgBot, error) {
	tgBot := &TgBot{
		log:         log.With(sl.Module("tgbot")),
		adminId:     adminId,
		botUsername: botName,
	}

	api, err := tgbotapi.NewBot(apiKey, nil)
	if err != nil {
		return nil, fmt.Errorf("creating api instance: %v", err)
	}
	tgBot.api = api

	return tgBot, nil
}

func (t *TgBot) Start() error {

	dispatcher := ext.NewDispatcher(&ext.DispatcherOpts{
		// If an error is returned by a handler, log it and continue going.
		Error: func(b *tgbotapi.Bot, ctx *ext.Context, err error) ext.DispatcherAction {
			log.Println("an error occurred while handling update:", err.Error())
			return ext.DispatcherActionNoop
		},
		MaxRoutines: ext.DefaultMaxRoutines,
	})
	updater := ext.NewUpdater(dispatcher, nil)

	// Start receiving updates.
	err := updater.StartPolling(t.api, &ext.PollingOpts{
		DropPendingUpdates: true,
		GetUpdatesOpts: &tgbotapi.GetUpdatesOpts{
			Timeout: 9,
			RequestOpts: &tgbotapi.RequestOpts{
				Timeout: time.Second * 10,
			},
		},
	})
	if err != nil {
		panic("failed to start polling: " + err.Error())
	}

	// Idle, to keep updates coming in, and avoid bot stopping.
	updater.Idle()

	// Set up an update configuration
	return nil
}

func (t *TgBot) SendMessage(msg string) {

	t.plainResponse(t.adminId, msg)
}

func (t *TgBot) plainResponse(chatId int64, text string) {

	// ChatGPT uses ** for bold text, so we need to replace it
	text = strings.ReplaceAll(text, "**", "*")
	text = strings.ReplaceAll(text, "![", "[")

	// Send the response back to the user
	sanitized := sanitize(text, false)

	if sanitized != "" {
		_, err := t.api.SendMessage(chatId, sanitized, &tgbotapi.SendMessageOpts{
			ParseMode: "MarkdownV2",
		})
		if err != nil {
			t.log.With(
				slog.Int64("id", chatId),
			).Warn("sending message", sl.Err(err))
			_, err = t.api.SendMessage(chatId, sanitized, &tgbotapi.SendMessageOpts{})
			if err != nil {
				t.log.With(
					slog.Int64("id", chatId),
				).Error("sending safe message", sl.Err(err))
			}
		}
	} else {
		t.log.With(
			slog.Int64("id", chatId),
		).Debug("empty message")
	}
}

func sanitize(input string, preserveLinks bool) string {
	// Define a list of reserved characters that need to be escaped
	reservedChars := "\\`_{}#+-.!|()[]"
	if preserveLinks {
		reservedChars = "\\`_{}#+-.!|"
	}

	// Loop through each character in the input string
	sanitized := ""
	for _, char := range input {
		// Check if the character is reserved
		if strings.ContainsRune(reservedChars, char) {
			// Escape the character with a backslash
			sanitized += "\\" + string(char)
		} else {
			// Add the character to the sanitized string
			sanitized += string(char)
		}
	}

	return sanitized
}
