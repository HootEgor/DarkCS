package zoho

import (
	"DarkCS/internal/lib/api/response"
	"encoding/json"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

type GetRequest struct {
	OrderZohoID   string `json:"order_zoho_id"`
	SmartSenderID string `json:"smart_sender_id"`
}

func GetOrderProducts(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req GetRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		err := handler.GetOrderProducts(req.OrderZohoID, req.SmartSenderID)
		if err != nil {
			log.Error("Failed to get order", slog.Any("error", err))
			http.Error(w, "Failed to get order", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		render.JSON(w, r, response.Ok("Order products retrieved successfully"))
	}
}
