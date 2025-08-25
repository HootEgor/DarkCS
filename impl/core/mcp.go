package core

import (
	"encoding/json"
	"fmt"
)

func (c *Core) Ping() string {
	return "core pong"
}

func (c *Core) HandleCommand(userUID, name string, args json.RawMessage) (interface{}, error) {
	user, err := c.authService.GetUserByUUID(userUID)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}

	return c.ass.HandleCommand(user, name, args)
}
