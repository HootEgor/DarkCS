package user

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type ClosePromoRequest struct {
	Phone string `json:"phone"`
}

func CloseUserPromo(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ClosePromoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		err := handler.ClosePromoForUser(req.Phone)
		if err != nil {
			log.Error("Failed to get user", slog.Any("error", err))
			http.Error(w, "Failed to get user", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode("Promo closed successfully")
	}
}
