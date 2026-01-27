package school

import (
	"DarkCS/internal/lib/api/response"
	"DarkCS/internal/lib/sl"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
)

func ListSchools(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mod := sl.Module("http.handlers.school")

		logger := log.With(
			mod,
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		if handler == nil {
			logger.Error("school service not available")
			render.JSON(w, r, response.Error("school service not available"))
			return
		}

		status := r.URL.Query().Get("status")
		if status == "" {
			status = "all"
		}

		schools, err := handler.GetSchools(status)
		if err != nil {
			logger.Error("failed to list schools", sl.Err(err))
			render.JSON(w, r, response.Error(fmt.Sprintf("Failed to list schools: %v", err)))
			return
		}

		logger.Debug("schools listed", slog.Int("count", len(schools)))
		render.JSON(w, r, response.Ok(schools))
	}
}
