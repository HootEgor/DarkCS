package product

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	neturl "net/url"
)

func (r *Service) GetUserDiscount(phone string) (int, error) {
	url := fmt.Sprintf("%s/%s/%s", r.BaseURL, "client", neturl.PathEscape(phone))

	// Create request
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, fmt.Errorf("failed to create request: %v", err)
	}

	// Add headers
	req.Header.Set("Authorization", fmt.Sprintf("Basic %s", r.getBase64Auth()))
	req.Header.Set("Content-Type", "application/json")

	// Send request
	client := &http.Client{Timeout: productHTTPTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return 0, fmt.Errorf("failed to send request: %v", err)
	}
	defer func(Body io.ReadCloser) {
		_ = Body.Close()
	}(resp.Body)

	// Handle response
	// Errors are returned rather than reported as a 0% discount, so callers can tell
	// "no discount" from "service unavailable".
	if resp.StatusCode != http.StatusOK {
		r.Log.Error("get user discount invalid response code", slog.Int("status", resp.StatusCode))
		return 0, fmt.Errorf("get user discount: status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, fmt.Errorf("failed to read response body: %v", err)
	}

	response, err := r.ParseGetUserResponse(body)
	if err != nil {
		return 0, fmt.Errorf("failed to parse response: %v", err)
	}

	r.Log.With(
		slog.Any("prods", response),
	).Debug("get user discount")

	return response, nil
}
