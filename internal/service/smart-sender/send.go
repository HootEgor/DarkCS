package smart_sender

import (
	"DarkCS/internal/lib/sl"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type sendRequest struct {
	Type      string `json:"type"`
	Content   string `json:"content"`
	Watermark int64  `json:"watermark"`
}

func (s *Service) SendMessage(userId, text string) error {
	defer func() {
		if r := recover(); r != nil {
			s.log.With(slog.Any("panic", r)).Error("send smart msg")
		}
	}()

	url := fmt.Sprintf("%s/contacts/%s/send", s.baseUrl, userId)

	body := sendRequest{
		Type:      "text",
		Content:   text,
		Watermark: time.Now().UnixMilli(),
	}
	bodyBytes, err := json.Marshal(body)
	if err != nil {
		s.log.With(sl.Err(err)).Error("marshal send body")
		return err
	}

	sendReq, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		s.log.With(sl.Err(err)).Error("create POST request")
		return err
	}

	if sendReq == nil {
		return fmt.Errorf("nil request to smart sender")
	}

	sendReq.Header.Set("Content-Type", "application/json")
	sendReq.Header.Set("Authorization", "Bearer "+s.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	if client == nil {
		return fmt.Errorf("nil HTTP client")
	}
	sendResp, err := client.Do(sendReq)
	if err != nil {
		s.log.With(sl.Err(err)).Error("send POST HTTP")
		return err
	}

	if sendResp == nil {
		return fmt.Errorf("nil response from smart sender")
	}

	if sendResp.Body != nil {
		defer sendResp.Body.Close()
	}

	if sendResp.StatusCode < 200 || sendResp.StatusCode >= 300 {
		s.log.With(sl.Err(err)).Error("non-2xx on POST")
		return fmt.Errorf("failed to send message: status %d", sendResp.StatusCode)
	}

	s.log.With(
		slog.String("user", userId),
		slog.String("text", text),
	).Info("message send successfully")

	return nil
}
