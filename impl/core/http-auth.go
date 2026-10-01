package core

import (
	"DarkCS/entity"
	"crypto/subtle"
	"fmt"
)

// ValidateToken validates a token for the CRM WebSocket and returns the username.
// Only CRM, admin and legacy keys may open the socket.
func (c *Core) ValidateToken(token string) (string, error) {
	user, err := c.AuthenticateByToken(token)
	if err != nil {
		return "", err
	}
	switch user.Scope {
	case entity.ScopeCRM, entity.ScopeAdmin, entity.ScopeLegacy:
		return user.Username, nil
	}
	return "", fmt.Errorf("key scope %q may not open the CRM socket", user.Scope)
}

// AuthenticateByToken resolves an API key to its username and scope. The config key
// ("internal") is the admin key.
func (c *Core) AuthenticateByToken(token string) (*entity.UserAuth, error) {
	if token == "" {
		return nil, fmt.Errorf("token not provided")
	}

	if k, ok := c.keys.get(token); ok {
		return &entity.UserAuth{Username: k.username, Scope: k.scope}, nil
	}

	var userName, scope string
	var err error = fmt.Errorf("repository is not set")
	if c.repo != nil {
		userName, scope, err = c.repo.CheckApiKey(token)
	}
	if err == nil {
		c.log.With("username", userName).Debug("user authenticated from database")
		c.keys.set(token, userName, scope)
		return &entity.UserAuth{Username: userName, Scope: scope}, nil
	}

	if c.authKey != "" && subtle.ConstantTimeCompare([]byte(c.authKey), []byte(token)) == 1 {
		userName = "internal"
		c.log.With("username", userName).Debug("user authenticated from config")
		c.keys.set(token, userName, entity.ScopeAdmin)
		return &entity.UserAuth{Username: userName, Scope: entity.ScopeAdmin}, nil
	}

	return nil, fmt.Errorf("invalid token")
}
