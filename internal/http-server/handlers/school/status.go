package school

import (
	"DarkCS/internal/lib/api/response"
	"DarkCS/internal/lib/sl"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
)

type SetStatusRequest struct {
	ID     string `json:"id"`
	Active bool   `json:"active"`
}

func SetStatus(log *slog.Logger, handler Core) http.HandlerFunc {
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

		var req SetStatusRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Error("failed to decode request body", sl.Err(err))
			render.JSON(w, r, response.Error("Invalid request body"))
			return
		}

		if req.ID == "" {
			render.JSON(w, r, response.Error("school id is required"))
			return
		}

		err := handler.SetSchoolActive(req.ID, req.Active)
		if err != nil {
			logger.Error("failed to set school status", sl.Err(err))
			render.JSON(w, r, response.Error(fmt.Sprintf("Failed to set school status: %v", err)))
			return
		}

		logger.Debug("school status updated", slog.String("id", req.ID), slog.Bool("active", req.Active))
		render.JSON(w, r, response.Ok("School status updated successfully"))
	}
}
