package user

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type GetRequest struct {
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	TelegramId int64  `json:"telegram_id"`
}

func GetUser(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req GetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		user, err := handler.GetUser(req.Email, req.Phone, req.TelegramId)
		if err != nil {
			log.Error("Failed to get user", slog.Any("error", err))
			http.Error(w, "Failed to get user", http.StatusInternalServerError)
			return
		}

		if user == nil {
			http.Error(w, "User not found", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}
