package assistant

import (
	"DarkCS/internal/lib/api/response"
	"DarkCS/internal/lib/sl"
	"fmt"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

func GetAllAssistants(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mod := sl.Module("http.handlers.assistant")

		logger := log.With(
			mod,
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		if handler == nil {
			logger.Error("assistant service not available")
			render.JSON(w, r, response.Error("assistant service not available"))
			return
		}

		assistants, err := handler.GetAllAssistants()
		if err != nil {
			logger.With(sl.Err(err)).Error("get all assistants")
			render.JSON(w, r, response.Error(fmt.Sprintf("Attach failed: %v", err)))
			return
		}
		logger.Debug("get all assistants successfully")

		render.JSON(w, r, response.Ok(assistants))
	}
}
