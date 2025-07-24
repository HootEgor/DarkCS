package zoho

import (
	"encoding/json"
	"log/slog"
	"net/http"
)

type GetRequest struct {
	OrderZohoID string `json:"order_zoho_id"`
}

func GetOrderProducts(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req GetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		msg, err := handler.GetOrderProducts(req.OrderZohoID)
		if err != nil {
			log.Error("Failed to get order", slog.Any("error", err))
			http.Error(w, "Failed to get order", http.StatusInternalServerError)
			return
		}

		if msg == "" {
			http.Error(w, "Order not found", http.StatusNotFound)
			return
		}

		var response struct {
			Data string `json:"data"`
		}

		response.Data = msg

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(response)
	}
}
