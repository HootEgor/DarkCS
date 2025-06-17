package gpt

import (
	"DarkCS/internal/lib/sl"
	"context"
	"encoding/json"
	"fmt"
	"github.com/sashabaranov/go-openai"
	"log/slog"
)

func (o *Overseer) askLogger(userId, msg string) (string, error) {
	var thread openai.Thread
	var err error

	threadId := o.threads[userId]
	if threadId != "" {
		thread, err = o.client.RetrieveThread(context.Background(), threadId)
		if err != nil {
			o.log.With(slog.String("thread", threadId)).Error("retrieving thread", sl.Err(err))
		}
	} else {
		err = fmt.Errorf("threadId is empty")
	}

	if err != nil {
		thread, err = o.client.CreateThread(context.Background(), openai.ThreadRequest{})
		if err != nil {
			return "", fmt.Errorf("error creating thread: %v", err)
		}
		o.threads[userId] = thread.ID
		o.log.With(slog.String("thread", thread.ID)).Info("created new thread")
	}

	// Send the user message to the assistant
	_, err = o.client.CreateMessage(context.Background(), thread.ID, openai.MessageRequest{
		Role:    string(openai.ThreadMessageRoleUser),
		Content: msg,
	})
	if err != nil {
		return "", fmt.Errorf("error creating message: %v", err)
	}

	completed := o.handleRun(thread.ID, o.loggerID)
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

	var response OverseerResponse
	err = json.Unmarshal([]byte(responseText), &response)
	if err != nil {
		o.log.With(
			slog.String("user", userId),
			slog.Int("text_length", len(responseText)),
			slog.String("response", responseText),
		).Debug("chat response")
		o.log.Error("error unmarshalling response", sl.Err(err))
		return responseText, nil
	}

	return response.Assistant, nil
}
