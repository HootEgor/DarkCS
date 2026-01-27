package school

import (
	"DarkCS/internal/lib/api/response"
	"DarkCS/internal/lib/sl"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
)

type AddRequest struct {
	Names string `json:"names"`
}

func AddSchools(log *slog.Logger, handler Core) http.HandlerFunc {
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

		var req AddRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Error("failed to decode request body", sl.Err(err))
			render.JSON(w, r, response.Error("Invalid request body"))
			return
		}

		var names []string
		for _, name := range strings.Split(req.Names, ",") {
			trimmed := strings.TrimSpace(name)
			if trimmed != "" {
				names = append(names, trimmed)
			}
		}

		if len(names) == 0 {
			render.JSON(w, r, response.Error("no school names provided"))
			return
		}

		schools, err := handler.AddSchools(names)
		if err != nil {
			logger.Error("failed to add schools", sl.Err(err))
			render.JSON(w, r, response.Error(fmt.Sprintf("Failed to add schools: %v", err)))
			return
		}

		logger.Debug("schools added", slog.Int("count", len(schools)))
		render.JSON(w, r, response.Ok(schools))
	}
}
