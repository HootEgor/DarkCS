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

		name, zohoId, err := handler.CreateUser(req.Name, req.Email, req.Phone, req.TelegramId)
		if err != nil {
			log.Error("Failed to create user", slog.Any("error", err))
			http.Error(w, "Failed to create user", http.StatusInternalServerError)
			return
		}

		if zohoId == "" {
			http.Error(w, "User not created", http.StatusNotFound)
			return
		}

		var response struct {
			Name   string `json:"name"`
			ZohoId string `json:"zoho_id"`
		}

		response.Name = name
		response.ZohoId = zohoId

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}
