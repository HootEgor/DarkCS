package services

import (
	"DarkCS/internal/lib/sl"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

type productsResponse struct {
	Data []struct {
		OrderedItems []struct {
			ProductName struct {
				Name string `json:"name"`
			} `json:"Product_Name"`
			ListPrice float64 `json:"List_Price"`
			Quantity  int     `json:"Quantity"`
			Total     float64 `json:"Total"`
			NetTotal  float64 `json:"Net_Total"`
			Discount  int     `json:"DiscountP"`
		} `json:"Ordered_Items"`
	} `json:"data"`
}

func (s *ZohoService) GetOrderProducts(orderId string) (string, error) {

	if time.Now().After(s.tokenExpiresIn.Add(time.Minute * time.Duration(-5))) {
		err := s.refreshTokenCall()
		if err != nil {
			return "", err
		}
	}

	products, err := s.getOrderProducts(orderId)
	if err != nil {
		return "", err
	}

	msg := "Товари:\n"
	for _, item := range products.Data {
		for _, product := range item.OrderedItems {
			msg += fmt.Sprintf(" - %s, Кількість: %d, Ціна: %.2f\nСума: %.2f, Знижка: %d%%\nФінальна сума: %.2f\n\n",
				product.ProductName.Name,
				product.Quantity,
				product.ListPrice,
				product.Total,
				product.Discount,
				product.NetTotal)
		}
	}

	return msg, nil
}

func (s *ZohoService) getOrderProducts(orderId string) (*productsResponse, error) {
	// Build URL
	path := fmt.Sprintf("Sales_Orders/%s", orderId)
	fullURL, err := buildURL(s.crmUrl, s.scope, "v4", path)
	if err != nil {
		return nil, fmt.Errorf("build url: %w", err)
	}

	fullURL = fullURL + "?fields=Ordered_Items"

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
	var result productsResponse

	if bodyBytes == nil || len(bodyBytes) == 0 {
		return nil, nil
	}

	if err := json.Unmarshal(bodyBytes, &result); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	return &result, nil
}
