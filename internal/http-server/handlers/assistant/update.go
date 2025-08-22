package assistant

import (
	"DarkCS/internal/lib/api/response"
	"DarkCS/internal/lib/sl"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

type UpdateRequest struct {
	Name           string      `json:"name"`
	Id             string      `json:"id"`
	Active         bool        `json:"active"`
	Model          string      `json:"model"`
	Prompt         string      `json:"prompt"`
	VectorStoreId  string      `json:"vector_store_id"`
	ResponseFormat interface{} `json:"response_format"`
	AllowedTools   []string    `json:"allowed_tools"`
}

func Update(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mod := sl.Module("http.handlers.assistant")

		logger := log.With(
			mod,
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		if handler == nil {
			logger.Error("assistant service not available")
			render.JSON(w, r, response.Error("Assistant service not available"))
			return
		}

		var req UpdateRequest
		if err := render.DecodeJSON(r.Body, &req); err != nil {
			logger.Error("failed to decode request body", sl.Err(err))
			render.JSON(w, r, response.Error("Invalid request body"))
			return
		}

		if req.Id == "" && req.Name == "" {
			logger.Error("invalid request: id and name cannot be empty")
			render.JSON(w, r, response.Error("Id and Name cannot be empty"))
			return
		}

		err := handler.UpdateAssistant(req.Name, req.Id, req.Active, req.Model, req.Prompt, req.VectorStoreId, req.ResponseFormat, req.AllowedTools)
		if err != nil {
			logger.Error("update assistant", sl.Err(err))
			render.JSON(w, r, response.Error("Update failed"))
			return
		}

		logger.With(
			slog.String("id", req.Id),
			slog.String("name", req.Name),
			slog.Bool("active", req.Active),
		).Debug("assistant updated successfully")

		render.JSON(w, r, response.Ok("Assistant updated successfully"))
	}
}
