// Package health serves the unauthenticated liveness/readiness probe used by the deploy
// pipeline after a restart.
package health

import (
	"context"
	"net/http"
	"time"

	"github.com/go-chi/render"
)

// Core reports whether the service's dependencies are reachable.
type Core interface {
	Health(ctx context.Context) error
}

// pingTimeout keeps the probe fast even when MongoDB hangs.
const pingTimeout = 3 * time.Second

// Handler returns 200 {"status":"ok"} when MongoDB answers, 503 otherwise. It exposes
// nothing beyond up/down and uptime, so it is safe without authentication.
func Handler(core Core, started time.Time) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), pingTimeout)
		defer cancel()

		status, code := "ok", http.StatusOK
		if err := core.Health(ctx); err != nil {
			status, code = "unavailable", http.StatusServiceUnavailable
		}
		render.Status(r, code)
		render.JSON(w, r, map[string]any{
			"status":         status,
			"uptime_seconds": int(time.Since(started).Seconds()),
		})
	}
}
