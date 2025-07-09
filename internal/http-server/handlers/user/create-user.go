package user

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type CreateRequest struct {
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	TelegramId int64  `json:"telegram_id"`
	Name       string `json:"name"`
}

func CreateUser(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req CreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		user, err := handler.CreateUser(req.Name, req.Email, req.Phone, req.TelegramId)
		if err != nil {
			log.Error("Failed to create user", slog.Any("error", err))
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}

		if user == nil {
			http.Error(w, "User not created", http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(user)
	}
}
