package smart

import (
	"DarkCS/internal/lib/api/response"
	"encoding/json"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

type SendRequest struct {
	SmartSenderID string `json:"smart_sender_id"`
	Text          string `json:"text"`
}

func SendMsg(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req SendRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		err := handler.SendMessage(req.SmartSenderID, req.Text)
		if err != nil {
			log.Error("Failed to send message", slog.Any("error", err))
			http.Error(w, "Failed to send message", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		render.JSON(w, r, response.Ok("message sent successfully"))
	}
}
