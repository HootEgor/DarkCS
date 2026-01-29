package services

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// CreateRating creates a service rating in Zoho CRM.
func (s *ZohoService) CreateRating(rating entity.ServiceRating) error {
	if time.Now().After(s.tokenExpiresIn.Add(time.Minute * time.Duration(-5))) {
		err := s.refreshTokenCall()
		if err != nil {
			return err
		}
	}

	return s.createRating(rating)
}

func (s *ZohoService) createRating(rating entity.ServiceRating) error {
	// Prepare payload
	payload := map[string]interface{}{
		"data": []entity.ServiceRating{rating},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	fullURL, err := buildURL(s.crmUrl, s.scope, s.apiVersion, "Raiting")
	if err != nil {
		return err
	}

	req, err := http.NewRequest(
		http.MethodPost,
		fullURL,
		bytes.NewBuffer(body),
	)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.refreshToken)
	req.Header.Set("Content-Type", "application/json")

	log := s.log.With(
		slog.String("url", fullURL),
		slog.String("method", req.Method),
		slog.String("payload", string(body)),
	)
	t := time.Now()
	defer func() {
		log = log.With(slog.Duration("duration", time.Since(t)))
		if err != nil {
			log.Error("create rating", sl.Err(err))
		} else {
			log.Debug("create rating")
		}
	}()

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	s.log.With(
		slog.String("response", string(bodyBytes)),
	).Debug("create rating response")

	var apiResp entity.ZohoAPIResponse
	if err := json.Unmarshal(bodyBytes, &apiResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if len(apiResp.Data) == 0 {
		return fmt.Errorf("empty response data")
	}

	item := apiResp.Data[0]

	if item.Status != "success" {
		return fmt.Errorf("rating not created: [%s] %s", item.Code, item.Message)
	}

	return nil
}
