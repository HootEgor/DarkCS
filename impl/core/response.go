package core

import (
	"DarkCS/entity"
	"fmt"
	"strconv"
)

func (c *Core) ComposeResponse(msg entity.UserMsg) (interface{}, error) {
	if c.ass == nil {
		return nil, fmt.Errorf("assistant not initialized")
	}

	id, _ := strconv.Atoi(msg.UserId)
	user, err := c.authService.GetUser("", "", int64(id))
	if err != nil {
		return nil, err
	}

	assistants := user.GetAssistants()
	systemMsg := "Available assistants: "
	for _, a := range assistants {
		systemMsg = fmt.Sprintf("%s %s,", systemMsg, a)
	}

	return c.ass.ComposeResponse(systemMsg, systemMsg, msg.Message)
}
