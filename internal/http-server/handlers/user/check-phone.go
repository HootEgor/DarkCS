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

type CheckRequest struct {
	Phone string `json:"phone"`
}

func CheckPhone(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mod := sl.Module("http.handlers.user")

		logger := log.With(
			mod,
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		if handler == nil {
			logger.Error("check phone not available")
			render.JSON(w, r, response.Error("check phone not available"))
			return
		}

		var req CheckRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Error("failed to decode request body", sl.Err(err))
			render.JSON(w, r, response.Error("Invalid request body"))
			return
		}

		code, err := handler.CheckUserPhone(req.Phone)
		if err != nil {
			logger.Error("check phone", sl.Err(err))
			render.JSON(w, r, response.Error(fmt.Sprintf("Check failed: %v", err)))
			return
		}
		logger.With(
			slog.String("phone", req.Phone),
			slog.String("code", code),
		).Debug("check phone successfully")

		render.JSON(w, r, response.Ok(code))
	}
}
