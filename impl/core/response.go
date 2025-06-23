package core

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"fmt"
	"log/slog"
	"strconv"
	"time"
)

func (c *Core) ComposeResponse(msg entity.HttpUserMsg) (interface{}, error) {
	if c.ass == nil {
		return nil, fmt.Errorf("assistant not initialized")
	}

	id, _ := strconv.Atoi(msg.TelegramId)
	user, err := c.authService.GetUser(msg.Email, msg.Phone, int64(id))
	if err != nil {
		return nil, err
	}

	if user.Blocked {
		return nil, fmt.Errorf("user is blocked")
	}

	assistants := user.GetAssistants()
	systemMsg := "Available assistants: "
	for _, a := range assistants {
		systemMsg = fmt.Sprintf("%s %s,", systemMsg, a)
	}

	answer, err := c.ass.ComposeResponse(user.GetId(), systemMsg, msg.Message)
	if err != nil {
		return nil, err
	}

	message := entity.Message{
		User:     user,
		Question: msg.Message,
		Answer:   answer,
		Time:     time.Now(),
	}

	sErr := c.repo.SaveMessage(message)
	if sErr != nil {
		c.log.With(
			slog.Any("msg", message),
			sl.Err(sErr),
		).Error("save message")
	}

	return answer.Text, err
}
