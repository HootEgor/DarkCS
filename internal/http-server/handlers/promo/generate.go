package promo

import (
	"DarkCS/internal/lib/api/response"
	"DarkCS/internal/lib/sl"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

type GenerateRequest struct {
	Quantity int `json:"quantity"`
}

func GeneratePromoCodes(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mod := sl.Module("http.handlers.promo")

		logger := log.With(
			mod,
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		if handler == nil {
			logger.Error("promo service not available")
			render.JSON(w, r, response.Error("promo search not available"))
			return
		}

		// Parse request body
		var req GenerateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Error("failed to decode request body", sl.Err(err))
			render.JSON(w, r, response.Error("Invalid request body"))
			return
		}

		logger = logger.With(slog.Any("quantity", req.Quantity))

		err := handler.GeneratePromoCodes(req.Quantity)
		if err != nil {
			logger.Error("product search", sl.Err(err))
			render.JSON(w, r, response.Error(fmt.Sprintf("Search failed: %v", err)))
			return
		}
		logger.Debug("product search")

		render.JSON(w, r, response.Ok("Promo codes generated successfully"))
	}
}
