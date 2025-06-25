package gpt

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"context"
	"encoding/json"
	"fmt"
	"github.com/sashabaranov/go-openai"
	"log/slog"
)

type CalculatorResponse struct {
	Response string   `json:"response"`
	Codes    []string `json:"codes"`
}

func (o *Overseer) askCalculator(userId, userMsg string) (string, error) {
	defer o.locker.Unlock(userId)
	thread, err := o.getOrCreateThread(userId)
	if err != nil {
		return "", err
	}

	// Send the user message to the assistant
	_, err = o.client.CreateMessage(context.Background(), thread.ID, openai.MessageRequest{
		Role:    string(openai.ThreadMessageRoleUser),
		Content: userMsg,
	})
	if err != nil {
		return "", fmt.Errorf("error creating message: %v", err)
	}

	completed := o.handleRun(userId, thread.ID, o.assistants[entity.CalculatorAss])
	if !completed {
		return "", fmt.Errorf("max retries reached, unable to complete run")
	}

	// Fetch the messages once the run is complete
	msgs, err := o.client.ListMessage(context.Background(), thread.ID, nil, nil, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("error listing messages: %v", err)
	}

	if len(msgs.Messages) == 0 {
		return "", fmt.Errorf("no messages found")
	}

	responseText := msgs.Messages[0].Content[0].Text.Value

	var response CalculatorResponse
	err = json.Unmarshal([]byte(responseText), &response)
	if err != nil {
		o.log.With(
			slog.String("user", userId),
			slog.String("response", responseText),
			sl.Err(err),
		).Error("unmarshalling response")
		return responseText, nil
	}

	o.log.With(
		slog.String("user", userId),
		slog.Any("response", response),
	).Debug("chat response")

	return response.Response, nil
}
