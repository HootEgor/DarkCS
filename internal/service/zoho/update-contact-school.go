package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// UpdateContactSchool updates the School field on a Zoho CRM contact.
func (s *ZohoService) UpdateContactSchool(zohoID, schoolName string) error {
	if time.Now().After(s.tokenExpiresIn.Add(time.Minute * time.Duration(-5))) {
		if err := s.refreshTokenCall(); err != nil {
			return err
		}
	}

	payload := map[string]interface{}{
		"data": []map[string]string{
			{"School": schoolName, "id": zohoID},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	fullURL, err := buildURL(s.crmUrl, s.scope, s.apiVersion, "Contacts", zohoID)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPut, fullURL, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.refreshToken)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read response body: %w", err)
	}

	s.log.With(
		slog.String("response", string(bodyBytes)),
	).Debug("update contact school response")

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("update contact school failed (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
