package product

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

func (r *Service) GetUserDiscount(phone string) (int, error) {
	url := fmt.Sprintf("%s/%s/%s", r.BaseURL, "client", phone)

	// Create request
	req, err := http.NewRequest(http.MethodGet, url, bytes.NewBuffer(nil))
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	// Add headers
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", r.getBase64Auth()))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	// Handle response
	if resp.StatusCode != http.StatusOK {
		r.Log.With(
			slog.Any("response", resp),
		).Error("invalid response code")
		return 0, fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %v", err)
	}

	response, err := ParseGetUserResponse(body)
	if err != nil {
		return 0, fmt.Errorf("failed to parse response: %v", err)
	}

	r.Log.With(
		slog.Any("prods", response),
	).Debug("get user discount")

	return response, nil
}
