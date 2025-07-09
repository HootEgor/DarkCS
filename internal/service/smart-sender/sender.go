package smart_sender

import (
	"DarkCS/internal/config"
	"DarkCS/internal/lib/sl"
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

type Service struct {
	apiKey  string
	baseUrl string
	log     *slog.Logger
}

func NewSmartSenderService(conf *config.Config, logger *slog.Logger) *Service {
	return &Service{
		apiKey:  conf.SmartSender.ApiKey,
		baseUrl: conf.SmartSender.BaseUrl,
		log:     logger.With(sl.Module("smart sender service")),
	}
}

type sendRequest struct {
	Type      string `json:"type"`
	Content   string `json:"content"`
	Watermark int64  `json:"watermark"`
}

func (s *Service) SendMessage(userId, text string) error {

	defer func() {
		if r := recover(); r != nil {
			s.log.With(
				slog.Any("panic", r),
			).Error("send smart msg")
		}
	}()

	url := fmt.Sprintf("%s/contacts/%s/send", s.baseUrl, userId)

	reqBody := sendRequest{
		Type:      "text",
		Content:   text,
		Watermark: time.Now().Unix(),
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		s.log.With(
			sl.Err(err),
		).Error("marshal request body")
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(bodyBytes))
	if err != nil {
		s.log.With(
			sl.Err(err),
		).Error("create request")
		return err
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.log.With(
			sl.Err(err),
		).Error("send HTTP")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.log.With(
			sl.Err(err),
		).Error("non-2xx response")
		return fmt.Errorf("smart sender responded with %d", resp.StatusCode)
	}

	s.log.With(
		slog.String("user", userId),
	).Info("message sent")
	return nil
}
