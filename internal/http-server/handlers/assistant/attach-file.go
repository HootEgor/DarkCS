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

func AttachFile(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mod := sl.Module("http.handlers.assistant")

		logger := log.With(
			mod,
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		if handler == nil {
			logger.Error("assistant service not available")
			render.JSON(w, r, response.Error("Attach file not available"))
			return
		}

		err := handler.AttachNewFile()
		if err != nil {
			logger.With(sl.Err(err)).Error("attach file")
			render.JSON(w, r, response.Error(fmt.Sprintf("Attach failed: %v", err)))
			return
		}
		logger.Debug("Attach file")

		render.JSON(w, r, response.Ok("File attached successfully"))
	}
}
