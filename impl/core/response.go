package core

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"fmt"
	"log/slog"
)

const (
	errorResponse = "От халепа... Щось пішло не так, будь ласка, спробуйте повторити запит :("
)

func (c *Core) ComposeResponse(msg entity.HttpUserMsg) (interface{}, error) {
	if msg.SmartSenderId != "" {
		go func(msg entity.HttpUserMsg) {

			answer, err := c.processRequest(msg)
			if err != nil {
				c.log.With(
					sl.Err(err),
				).Error("compose smart response")
				answer.Text = errorResponse
			}

			err = c.smartService.EditLatestInputMessage(msg.SmartSenderId, answer.Text)
			if err != nil {
				c.log.With(
					sl.Err(err),
				).Error("send smart msg")
			}
		}(msg)

		return nil, nil
	}

	return c.processRequest(msg)
}

func (c *Core) processRequest(msg entity.HttpUserMsg) (*entity.AiAnswer, error) {
	if c.ass == nil {
		return nil, fmt.Errorf("assistant not initialized")
	}

	user, err := c.authService.GetUser(msg.Email, msg.Phone, msg.TelegramId)
	if err != nil {
		return nil, err
	}

	if user.SmartSenderId == "" && msg.SmartSenderId != "" {
		err = c.authService.SetSmartSenderId(msg.Email, msg.Phone, msg.TelegramId, msg.SmartSenderId)
		if err != nil {
			return nil, err
		}
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
		c.log.With(
			slog.String("text", userMsg),
			slog.Any("user", user),
		).Debug("audio to text")
	}

	answer, err := c.ass.ComposeResponse(user, systemMsg, userMsg)
	if err != nil {
		return nil, err
	}

	//message := entity.Message{
	//	User:     user,
	//	Question: msg.Message,
	//	Answer:   answer,
	//	Time:     time.Now(),
	//}

	//sErr := c.repo.SaveMessage(message)
	//if sErr != nil {
	//	c.log.With(
	//		slog.Any("msg", message),
	//		sl.Err(sErr),
	//	).Error("save message")
	//}

	if msg.WithHtmlLinks {
		if len(answer.Products) > 0 {
			answer.Text += "\n"
			for _, p := range answer.Products {
				answer.Text += fmt.Sprintf("\n<a href=\"%s\">%s</a> - %.2f грн.", p.Url, p.Name, p.Price)
			}
		}
	}

	c.log.With(
		slog.String("text", answer.Text),
		slog.Any("user", user),
	).Debug("response")

	return &answer, err
}
