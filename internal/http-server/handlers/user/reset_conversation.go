package user

import (
	"DarkCS/internal/lib/api/response"
	"DarkCS/internal/lib/sl"
	"encoding/json"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

type ResetRequest struct {
	Phone string `json:"phone"`
}

func ResetConversation(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if handler == nil {
			log.Error("reset conversation not available")
			http.Error(w, "reset conversation not available", http.StatusServiceUnavailable)
			return
		}

		var req ResetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		err := handler.ResetConversation(req.Phone)
		if err != nil {
			log.Error("reset conversation", sl.Err(err))
			http.Error(w, "Reset failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		render.JSON(w, r, response.Ok("Conversation reset successfully"))
	}
}
