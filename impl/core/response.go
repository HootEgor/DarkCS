package core

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"fmt"
	"log/slog"
	"time"
)

func (c *Core) ComposeResponse(msg entity.HttpUserMsg) (interface{}, error) {
	if c.ass == nil {
		return nil, fmt.Errorf("assistant not initialized")
	}

	user, err := c.authService.GetUser(msg.Email, msg.Phone, msg.TelegramId)
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

	userMsg := msg.Message

	if msg.VoiceMsgBase64 != "" {
		userMsg, err = c.ass.GetAudioText(msg.VoiceMsgBase64)
		if err != nil {
			return nil, err
		}
	}

	answer, err := c.ass.ComposeResponse(user, systemMsg, userMsg)
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

	c.log.With(
		slog.String("text", answer.Text),
		slog.Any("user", user),
	).Debug("response")

	return answer, err
}
