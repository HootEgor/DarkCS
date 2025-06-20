package response

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/api/response"
	"DarkCS/internal/lib/sl"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

func ComposeResponse(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mod := sl.Module("http.handlers.response")

		logger := log.With(
			mod,
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		if handler == nil {
			logger.Error("compose response not available")
			render.JSON(w, r, response.Error("Compose response not available"))
			return
		}

		var req entity.HttpUserMsg
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Error("failed to decode request body", sl.Err(err))
			render.JSON(w, r, response.Error("Invalid request body"))
			return
		}

		if req.Message == "" {
			logger.Error("no message provided")
			render.JSON(w, r, response.Error("No message provided"))
			return
		}

		logger = logger.With(slog.Any("message", req.Message))

		resp, err := handler.ComposeResponse(req)
		if err != nil {
			logger.Error("compose response", sl.Err(err))
			render.JSON(w, r, response.Error(fmt.Sprintf("Compose failed: %v", err)))
			return
		}
		logger.Debug("compose response")

		render.JSON(w, r, response.Ok(resp))
	}
}
