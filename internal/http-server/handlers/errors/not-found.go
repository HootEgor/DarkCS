package errors

import (
	"DarkCS/internal/lib/api/response"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

func NotFound(_ *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		//mod := sl.Module("http.handlers.errors")

		render.Status(r, 404)
		render.JSON(w, r, response.Error("Requested resource not found"))
	}
}
