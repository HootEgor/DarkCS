package user

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

type BlockRequest struct {
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	TelegramId int64  `json:"telegram_id"`
	Block      bool   `json:"block"`
}

func BlockUser(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mod := sl.Module("http.handlers.user")

		logger := log.With(
			mod,
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		if handler == nil {
			logger.Error("block user not available")
			render.JSON(w, r, response.Error("Block user not available"))
			return
		}

		var req BlockRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Error("failed to decode request body", sl.Err(err))
			render.JSON(w, r, response.Error("Invalid request body"))
			return
		}

		err := handler.BlockUser(req.Email, req.Phone, req.TelegramId, req.Block)
		if err != nil {
			logger.Error("block user", sl.Err(err))
			render.JSON(w, r, response.Error(fmt.Sprintf("Block failed: %v", err)))
			return
		}
		logger.Debug("block user")

		render.JSON(w, r, response.Ok(fmt.Sprintf("User block: %t", req.Block)))
	}
}
