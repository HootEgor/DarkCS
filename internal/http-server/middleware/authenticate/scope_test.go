package authenticate

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/api/cont"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func serveWithScope(t *testing.T, mw func(http.Handler) http.Handler, user *entity.UserAuth) int {
	t.Helper()
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	if user != nil {
		req = req.WithContext(cont.PutUser(req.Context(), user))
	}
	rec := httptest.NewRecorder()
	mw(ok).ServeHTTP(rec, req)
	return rec.Code
}

func TestRequireScope(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	adminOnly := RequireScope(log, "admin")
	crmOnly := RequireScope(log, "crm", entity.ScopeCRM)

	tests := []struct {
		name string
		mw   func(http.Handler) http.Handler
		user *entity.UserAuth
		want int
	}{
		{"admin passes admin route", adminOnly, &entity.UserAuth{Username: "a", Scope: entity.ScopeAdmin}, 200},
		{"admin passes crm route", crmOnly, &entity.UserAuth{Username: "a", Scope: entity.ScopeAdmin}, 200},
		{"legacy key keeps access", adminOnly, &entity.UserAuth{Username: "old", Scope: entity.ScopeLegacy}, 200},
		{"crm key on crm route", crmOnly, &entity.UserAuth{Username: "c", Scope: entity.ScopeCRM}, 200},
		{"integration key denied admin route", adminOnly, &entity.UserAuth{Username: "i", Scope: entity.ScopeIntegration}, 403},
		{"mcp key denied crm route", crmOnly, &entity.UserAuth{Username: "m", Scope: entity.ScopeMCP}, 403},
		{"no authenticated key", adminOnly, nil, 401},
	}
	for _, tt := range tests {
		if got := serveWithScope(t, tt.mw, tt.user); got != tt.want {
			t.Errorf("%s: status %d, want %d", tt.name, got, tt.want)
		}
	}
}
