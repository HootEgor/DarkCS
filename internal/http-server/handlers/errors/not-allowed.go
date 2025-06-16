package errors

import (
	"DarkCS/internal/lib/api/response"
	"github.com/go-chi/render"
	"log/slog"
	"net/http"
)

func NotAllowed(_ *slog.Logger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		//mod := sl.Module("http.handlers.errors")

		render.Status(r, 405)
		render.JSON(w, r, response.Error("Method not allowed"))
	}
}
