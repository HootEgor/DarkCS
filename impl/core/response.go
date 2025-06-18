package core

import (
	"DarkCS/entity"
	"fmt"
)

func (c *Core) ComposeResponse(msg entity.UserMsg) (interface{}, error) {
	if c.ass == nil {
		return nil, fmt.Errorf("assistant not initialized")
	}

	systemMsg := ""
	//id, _ := strconv.Atoi(msg.UserId)
	//user, err := c.authService.GetUser(int64(id))
	//if err != nil {
	//	return nil, err
	//}
	//
	//if user == nil {
	//	systemMsg = "User not found"
	//}

	return c.ass.ComposeResponse(systemMsg, systemMsg, msg.Message)
}
