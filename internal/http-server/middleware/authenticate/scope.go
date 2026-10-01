package authenticate

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/api/cont"
	"DarkCS/internal/lib/api/response"
	"log/slog"
	"net/http"
	"slices"
	"sync"

	"github.com/go-chi/render"
)

// RequireScope limits a route group to keys holding one of the given scopes. Admin keys
// pass everywhere. Legacy keys (no scope, created before scopes existed) also pass so
// existing integrations keep working; when a legacy key uses a group that is not open
// to integration keys, it is logged once per key and group, which shows which keys
// need a wider scope before legacy access is removed.
//
// Must run after New, which puts the authenticated key into the request context.
func RequireScope(log *slog.Logger, group string, scopes ...string) func(next http.Handler) http.Handler {
	log = log.With(slog.String("module", "middleware.scope"), slog.String("group", group))
	auditLegacy := !slices.Contains(scopes, entity.ScopeIntegration)
	var reported sync.Map // username -> struct{}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			user := cont.GetUser(r.Context())

			switch {
			case user.Username == "":
				render.Status(r, http.StatusUnauthorized)
				render.JSON(w, r, response.Error("Unauthorized"))
				return
			case user.Scope == entity.ScopeAdmin || slices.Contains(scopes, user.Scope):
			case user.Scope == entity.ScopeLegacy:
				if auditLegacy {
					if _, seen := reported.LoadOrStore(user.Username, struct{}{}); !seen {
						log.Info("legacy api key used outside integration scope",
							slog.String("key", user.Username), slog.String("path", r.URL.Path))
					}
				}
			default:
				log.Warn("api key scope denied",
					slog.String("key", user.Username), slog.String("scope", user.Scope), slog.String("path", r.URL.Path))
				render.Status(r, http.StatusForbidden)
				render.JSON(w, r, response.Error("Forbidden: key scope does not allow this endpoint"))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
