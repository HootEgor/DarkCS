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

type Response struct {
	Response  string   `json:"response"`
	Codes     []string `json:"codes"`
	ShowCodes bool     `json:"show_codes"`
}

func (o *Overseer) ask(user *entity.User, userMsg, assId string) (string, []entity.ProductInfo, error) {
	defer o.locker.Unlock(user.UUID)
	thread, err := o.getOrCreateThread(user.UUID)
	if err != nil {
		return "", nil, err
	}

	// Send the user message to the assistant
	_, err = o.client.CreateMessage(context.Background(), thread.ID, openai.MessageRequest{
		Role:    string(openai.ThreadMessageRoleUser),
		Content: userMsg,
	})
	if err != nil {
		return "", nil, fmt.Errorf("error creating message: %v", err)
	}

	completed := o.handleRun(user, thread.ID, assId)
	if !completed {
		return "", nil, fmt.Errorf("max retries reached, unable to complete run")
	}

	// Fetch the messages once the run is complete
	msgs, err := o.client.ListMessage(context.Background(), thread.ID, nil, nil, nil, nil, nil)
	if err != nil {
		return "", nil, fmt.Errorf("error listing messages: %v", err)
	}

	if len(msgs.Messages) == 0 {
		return "", nil, fmt.Errorf("no messages found")
	}

	responseText := msgs.Messages[0].Content[0].Text.Value

	var response Response
	err = json.Unmarshal([]byte(responseText), &response)
	if err != nil {
		o.log.With(
			slog.String("userUUID", user.UUID),
			slog.String("response", responseText),
			sl.Err(err),
		).Error("unmarshalling response")
		return responseText, nil, nil
	}

	if !response.ShowCodes {
		return response.Response, nil, nil
	}

	productsInfo, err := o.productService.GetProductInfo(response.Codes)
	if err != nil {
		o.log.With(
			slog.String("userUUID", user.UUID),
			sl.Err(err),
		).Error("ask")
		return response.Response, nil, nil
	}

	return response.Response, productsInfo, nil
}
