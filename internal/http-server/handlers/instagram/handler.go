package instagram

import (
	"log/slog"
	"net/http"

	"DarkCS/bot/insta"
	"DarkCS/internal/lib/sl"
)

// WebhookVerify handles GET requests for webhook verification
func WebhookVerify(log *slog.Logger, bot *insta.InstaBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.With(sl.Module("instagram.webhook")).Debug("webhook verification request")
		bot.HandleWebhookVerification(w, r)
	}
}

// WebhookHandler handles POST requests for incoming messages
func WebhookHandler(log *slog.Logger, bot *insta.InstaBot) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.With(sl.Module("instagram.webhook")).Debug("webhook message received")
		bot.HandleWebhook(w, r)
	}
}
