package product

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

type InfoRequest struct {
	Articles []string `json:"articles"`
}

func ProductsInfo(log *slog.Logger, handler Core) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		mod := sl.Module("http.handlers.product")

		logger := log.With(
			mod,
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)

		if handler == nil {
			logger.Error("product service not available")
			render.JSON(w, r, response.Error("Product search not available"))
			return
		}

		// Parse request body
		var req InfoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			logger.Error("failed to decode request body", sl.Err(err))
			render.JSON(w, r, response.Error("Invalid request body"))
			return
		}

		// Check if articles are provided
		if len(req.Articles) == 0 {
			logger.Error("no articles provided")
			render.JSON(w, r, response.Error("No articles provided"))
			return
		}

		logger = logger.With(slog.Any("articles", req.Articles))

		products, err := handler.ProductsInfo(req.Articles)
		if err != nil {
			logger.Error("product search", sl.Err(err))
			render.JSON(w, r, response.Error(fmt.Sprintf("Search failed: %v", err)))
			return
		}
		logger.Debug("product search")

		render.JSON(w, r, response.Ok(products))
	}
}
