package core

import (
	"DarkCS/entity"
	"crypto/subtle"
	"fmt"
)

// ValidateToken validates a token and returns the username (used by WebSocket auth).
func (c *Core) ValidateToken(token string) (string, error) {
	user, err := c.AuthenticateByToken(token)
	if err != nil {
		return "", err
	}
	return user.Username, nil
}

func (c *Core) AuthenticateByToken(token string) (*entity.UserAuth, error) {
	if token == "" {
		return nil, fmt.Errorf("token not provided")
	}

	if userName, ok := c.keys.get(token); ok {
		return &entity.UserAuth{Username: userName}, nil
	}

	var userName string
	var err error = fmt.Errorf("repository is not set")
	if c.repo != nil {
		userName, err = c.repo.CheckApiKey(token)
	}
	if err == nil {
		c.log.With("username", userName).Debug("user authenticated from database")
		c.keys.set(token, userName)
		return &entity.UserAuth{Username: userName}, nil
	}

	if c.authKey != "" && subtle.ConstantTimeCompare([]byte(c.authKey), []byte(token)) == 1 {
		userName = "internal"
		c.log.With("username", userName).Debug("user authenticated from config")
		c.keys.set(token, userName)
		return &entity.UserAuth{Username: userName}, nil
	}

	return nil, fmt.Errorf("invalid token")
}
