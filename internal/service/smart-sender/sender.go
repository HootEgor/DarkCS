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

func (s *Service) EditLatestInputMessage(userId, newText string) error {
	defer func() {
		if r := recover(); r != nil {
			s.log.With(slog.Any("panic", r)).Error("edit smart msg")
		}
	}()

	// 1. Fetch latest messages
	url := fmt.Sprintf("%s/contacts/%s/messages?page=1&limitation=10", s.baseUrl, userId)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		s.log.With(sl.Err(err)).Error("create GET request")
		return err
	}

	req.Header.Set("Authorization", "Bearer "+s.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		s.log.With(sl.Err(err)).Error("send GET HTTP")
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		s.log.With(sl.Err(err)).Error("non-2xx response on GET")
		return fmt.Errorf("failed to fetch messages: status %d", resp.StatusCode)
	}

	var data struct {
		Collection []struct {
			ID      int64 `json:"id"`
			Content struct {
				Type string `json:"type"`
			} `json:"content"`
			Gate struct {
				ID int64 `json:"id"`
			} `json:"gate"`
		} `json:"collection"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		s.log.With(sl.Err(err)).Error("decode response")
		return err
	}

	var messageID, gateID int64
	found := false
	for _, msg := range data.Collection {
		if msg.Content.Type == "input" {
			messageID = msg.ID
			gateID = msg.Gate.ID
			found = true
			break
		}
	}

	if !found {
		s.log.Warn("no input-type message found")
		return fmt.Errorf("no input-type message found for user %s", userId)
	}

	// 2. Edit the message
	editURL := fmt.Sprintf("%s/gates/%d/messages", s.baseUrl, gateID)

	editBody := map[string]interface{}{
		"text":      newText,
		"messageId": messageID,
	}
	bodyBytes, err := json.Marshal(editBody)
	if err != nil {
		s.log.With(sl.Err(err)).Error("marshal edit body")
		return err
	}

	editReq, err := http.NewRequest("PUT", editURL, bytes.NewReader(bodyBytes))
	if err != nil {
		s.log.With(sl.Err(err)).Error("create PUT request")
		return err
	}

	editReq.Header.Set("Content-Type", "application/json")
	editReq.Header.Set("Authorization", "Bearer "+s.apiKey)

	editResp, err := client.Do(editReq)
	if err != nil {
		s.log.With(sl.Err(err)).Error("send PUT HTTP")
		return err
	}
	defer editResp.Body.Close()

	if editResp.StatusCode < 200 || editResp.StatusCode >= 300 {
		s.log.With(sl.Err(err)).Error("non-2xx on PUT")
		return fmt.Errorf("failed to edit message: status %d", editResp.StatusCode)
	}

	s.log.With(
		slog.String("user", userId),
		slog.Int64("messageId", messageID),
	).Info("message edited successfully")
	return s.trigger(userId, "assistant_response")
}

func (s *Service) trigger(userId, name string) error {
	fireURL := fmt.Sprintf("%s/contacts/%s/fire", s.baseUrl, userId)

	fireBody := map[string]interface{}{
		"name": name,
	}
	fireBytes, err := json.Marshal(fireBody)
	if err != nil {
		s.log.With(sl.Err(err)).Error("marshal fire body")
		return err
	}

	fireReq, err := http.NewRequest("POST", fireURL, bytes.NewReader(fireBytes))
	if err != nil {
		s.log.With(sl.Err(err)).Error("create POST request for fire")
		return err
	}

	fireReq.Header.Set("Content-Type", "application/json")
	fireReq.Header.Set("Authorization", "Bearer "+s.apiKey)

	client := &http.Client{Timeout: 10 * time.Second}
	fireResp, err := client.Do(fireReq)
	if err != nil {
		s.log.With(sl.Err(err)).Error("send POST /fire")
		return err
	}
	defer fireResp.Body.Close()

	if fireResp.StatusCode < 200 || fireResp.StatusCode >= 300 {
		s.log.With(sl.Err(err)).Error("non-2xx on POST /fire")
		return fmt.Errorf("failed to fire assistant_response event: status %d", fireResp.StatusCode)
	}

	s.log.With(slog.String("user", userId)).Info("assistant_response event fired")

	return nil
}
