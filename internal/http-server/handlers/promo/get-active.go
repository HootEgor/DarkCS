package promo

import (
	"DarkCS/internal/lib/api/response"
	"DarkCS/internal/lib/sl"
	"fmt"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

func GetActivePromoCodes(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mod := sl.Module("http.handlers.promo")

		logger := log.With(
			mod,
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		if handler == nil {
			logger.Error("promo service not available")
			http.Error(w, "Promo service not available", http.StatusServiceUnavailable)
			return
		}

		codes, err := handler.GetActivePromoCodes()
		if err != nil {
			logger.Error("failed to fetch active promo codes", sl.Err(err))
			http.Error(w, fmt.Sprintf("Failed to fetch codes: %v", err), http.StatusInternalServerError)
			return
		}

		render.JSON(w, r, response.Ok(codes))
	}
}
