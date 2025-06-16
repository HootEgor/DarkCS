package product

import (
	"DarkCS/entity"
	"DarkCS/internal/config"
	"DarkCS/internal/lib/sl"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
)

type Service struct {
	Login    string
	Password string
	BaseURL  string
	Log      *slog.Logger
}

func NewProductService(conf *config.Config, logger *slog.Logger) *Service {
	return &Service{
		Login:    conf.ProdService.Login,
		Password: conf.ProdService.Password,
		BaseURL:  conf.ProdService.BaseURL,
		Log:      logger.With(sl.Module("prod service")),
	}
}

func (r *Service) getBase64Auth() string {
	return base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", r.Login, r.Password)))
}

func (r *Service) ProductSearch(article string) (interface{}, error) {
	prods, err := r.GetProductInfo([]string{article})
	r.Log.With(
		slog.Any("prod", prods),
	).Debug("search product")
	return prods, err
}

func (r *Service) GetProductInfo(articles []string) ([]entity.Product, error) {
	url := fmt.Sprintf("%s", r.BaseURL)

	requestBody, err := json.Marshal(articles)
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

	response, err := ParseResponse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("response indicated failure: %s", response.Message)
	}

	r.Log.With(
		slog.Any("prods", response),
	).Debug("get product info")

	return response.Products, nil
}

func (r *Service) GetAvailableProducts() ([]entity.Product, error) {
	url := fmt.Sprintf("%s", r.BaseURL)

	// Create request
	req, err := http.NewRequest("GET", url, bytes.NewBuffer(nil))
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

	response, err := ParseResponse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse response: %v", err)
	}

	if !response.Success {
		return nil, fmt.Errorf("response indicated failure: %s", response.Message)
	}

	r.Log.With(
		slog.Int("size", len(response.Products)),
	).Debug("all products")

	return response.Products, nil
}
