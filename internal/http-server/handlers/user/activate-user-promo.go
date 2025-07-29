package user

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type ActivatePromoRequest struct {
	Phone string `json:"phone"`
	Code  string `json:"code"`
}

func ActivateUserPromo(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req ActivatePromoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		access, err := handler.UserActivatePromoCode(req.Phone, req.Code)
		if err != nil {
			log.Error("Failed to get user", slog.Any("error", err))
			http.Error(w, "Failed to get user", http.StatusInternalServerError)
			return
		}

		var response struct {
			Access bool `json:"access"`
		}

		response.Access = access

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}
