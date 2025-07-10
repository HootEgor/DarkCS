package product

import (
	"DarkCS/entity"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

func (r *Service) ValidateOrder(products []entity.OrderProduct) ([]entity.OrderProduct, error) {
	url := fmt.Sprintf("%s/%s/1", r.BaseURL, "order")

	requestBody, err := json.Marshal(products)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	// Create request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %v", err)
	}

	// Add headers
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", r.getBase64Auth()))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %v", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	// Handle response
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed with status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}

	response, err := ParseOrderValidateResponse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	r.Log.With(
		slog.Any("prods", response),
	).Debug("validate order")

	return response, nil
}
