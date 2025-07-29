package user

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type GetPromoRequest struct {
	Phone string `json:"phone"`
}

func GetUserPromoAccess(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req GetPromoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		has, err := handler.UserHasPromoAccess(req.Phone)
		if err != nil {
			log.Error("Failed to get user", slog.Any("error", err))
			http.Error(w, "Failed to get user", http.StatusInternalServerError)
			return
		}

		var response struct {
			Access bool `json:"access"`
		}

		response.Access = has

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}
