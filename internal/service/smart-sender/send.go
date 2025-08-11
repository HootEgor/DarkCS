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
		s.log.With(sl.Err(err)).Error("marshal edit body")
		return err
	}

	editReq, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		s.log.With(sl.Err(err)).Error("create POST request")
		return err
	}

	editReq.Header.Set("Content-Type", "application/json")
	editReq.Header.Set("Authorization", "Bearer "+s.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	editResp, err := client.Do(editReq)
	if err != nil {
		s.log.With(sl.Err(err)).Error("send POST HTTP")
		return err
	}
	defer editResp.Body.Close()

	if editResp.StatusCode < 200 || editResp.StatusCode >= 300 {
		s.log.With(sl.Err(err)).Error("non-2xx on POST")
		return fmt.Errorf("failed to edit message: status %d", editResp.StatusCode)
	}

	s.log.With(
		slog.String("user", userId),
		slog.String("text", text),
	).Info("message send successfully")

	return nil
}
