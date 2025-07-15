package services

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

func (s *ZohoService) GetOrders(userInfo entity.UserInfo) ([]entity.OrderStatus, error) {

	if time.Now().After(s.tokenExpiresIn.Add(time.Minute * time.Duration(-5))) {
		err := s.refreshTokenCall()
		if err != nil {
			return nil, err
		}
	}

	contact := userInfo.ToContact()

	contactID, err := s.createContact(*contact)
	if err != nil {
		return nil, err
	}
	zohoOrders, err := s.getOrders(contactID)
	if err != nil {
		return nil, err
	}

	return zohoOrders, nil
}

func (s *ZohoService) getOrders(contactID string) ([]entity.OrderStatus, error) {
	// Build URL
	fullURL, err := buildURL(
		s.crmUrl,
		s.scope,
		"v7",
		fmt.Sprintf("Contacts/%s/SalesOrders?fields=Status", contactID),
	)
	if err != nil {
		return nil, fmt.Errorf("build url: %w", err)
	}

	// Create request
	req, err := http.NewRequest(http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+s.refreshToken)
	req.Header.Set("Content-Type", "application/json")

	log := s.log.With(
		slog.String("url", fullURL),
		slog.String("method", req.Method),
	)

	t := time.Now()
	defer func() {
		log = log.With(slog.Duration("duration", time.Since(t)))
		if err != nil {
			log.Error("get order", sl.Err(err))
		} else {
			log.Debug("get order")
		}
	}()

	// Execute request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response body: %w", err)
	}

	log.With(slog.String("response", string(bodyBytes))).Debug("get order response")

	// Parse response
	var result struct {
		Data []struct {
			Status string `json:"Status"`
		} `json:"data"`
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	// Map to []entity.OrderStatus
	var orders []entity.OrderStatus
	for _, d := range result.Data {
		orders = append(orders, entity.OrderStatus{
			Status: d.Status,
		})
	}

	return orders, nil
}
