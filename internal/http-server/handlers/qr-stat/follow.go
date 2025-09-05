package qr_stat

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

type FollowRequest struct {
	SmartSenderId string `json:"smart_sender_id"`
}

func FollowQr(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mod := sl.Module("http.handlers.qrstat")

		logger := log.With(
			mod,
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		if handler == nil {
			logger.Error("qr service not available")
			render.JSON(w, r, response.Error("qr not available"))
			return
		}

		// Parse request body
		var req FollowRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Error("failed to decode request body", sl.Err(err))
			render.JSON(w, r, response.Error("Invalid request body"))
			return
		}

		err := handler.FollowQr(req.SmartSenderId)
		if err != nil {
			logger.Error("FollowQr", sl.Err(err))
			render.JSON(w, r, response.Error(fmt.Sprintf("FollowQr: %v", err)))
			return
		}
		logger.Debug("FollowQr")

		render.JSON(w, r, response.Ok("success"))
	}
}
