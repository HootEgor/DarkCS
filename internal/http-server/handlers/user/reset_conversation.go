package user

import (
	"DarkCS/internal/lib/api/response"
	"DarkCS/internal/lib/sl"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

func ResetConversation(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if handler == nil {
			log.Error("reset conversation not available")
			http.Error(w, "reset conversation not available", http.StatusServiceUnavailable)
			return
		}

		phone := r.URL.Query().Get("phone")
		if phone == "" {
			http.Error(w, "Missing phone parameter", http.StatusBadRequest)
			return
		}

		err := handler.ResetConversation(phone)
		if err != nil {
			log.Error("reset conversation", sl.Err(err))
			http.Error(w, "Reset failed: "+err.Error(), http.StatusInternalServerError)
			return
		}

		render.JSON(w, r, response.Ok("Conversation reset successfully"))
	}
}
