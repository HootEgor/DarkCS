package entity

import (
	"DarkCS/internal/lib/validate"
	"net/http"
)

// API key scopes. A scope limits which route groups a key may call (see
// authenticate.RequireScope). ScopeLegacy is the empty scope of keys created before
// scopes existed: they keep full access so existing integrations don't break, and their
// use outside the integration routes is logged so they can be narrowed later.
const (
	ScopeLegacy      = ""
	ScopeAdmin       = "admin"
	ScopeIntegration = "integration"
	ScopeCRM         = "crm"
	ScopeMCP         = "mcp"
)

// ValidScope reports whether s can be assigned to a new key.
func ValidScope(s string) bool {
	switch s {
	case ScopeLegacy, ScopeAdmin, ScopeIntegration, ScopeCRM, ScopeMCP:
		return true
	}
	return false
}

type UserAuth struct {
	Username string `json:"username" bson:"username" validate:"required"`
	Scope    string `json:"scope" bson:"scope"`
	Name     string `json:"name" bson:"name" validate:"omitempty"`
	Email    string `json:"email" bson:"email" validate:"omitempty"`
	Token    string `json:"token" bson:"token" validate:"required,min=1"`
}

func (u *UserAuth) Bind(_ *http.Request) error {
	return validate.Struct(u)
}
