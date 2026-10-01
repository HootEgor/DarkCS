package product

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"encoding/json"
	"fmt"
)

type InfoResponse struct {
	Success  bool                 `json:"success"`
	Products []entity.ProductInfo `json:"data"`
	Message  string               `json:"message"`
}

type Response struct {
	Success  bool             `json:"success"`
	Products []entity.Product `json:"data"`
	Message  string           `json:"message"`
}

type OrderValidateResponse struct {
	Success  bool                  `json:"success"`
	Products []entity.OrderProduct `json:"data"`
	Message  string                `json:"message"`
}

type GetUserResponse struct {
	Success bool            `json:"success"`
	User    entity.OneCUser `json:"data"`
	Message string          `json:"message"`
}

func ParseInfoResponse(body []byte) (*InfoResponse, error) {
	var response InfoResponse
	err := json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

func ParseResponse(body []byte) (*Response, error) {
	var response Response
	err := json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}

// ParseOrderValidateResponse returns the validated products. A response flagged
// unsuccessful is an error only when it carries no products, so partial results the
// service returns today keep working.
func ParseOrderValidateResponse(body []byte) ([]entity.OrderProduct, error) {
	var response OrderValidateResponse
	err := json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}
	if !response.Success && len(response.Products) == 0 {
		return nil, fmt.Errorf("order validation failed: %s", response.Message)
	}
	return response.Products, nil
}

func (r *Service) ParseGetUserResponse(body []byte) (int, error) {
	var response GetUserResponse
	err := json.Unmarshal(body, &response)
	if err != nil {
		r.Log.With(
			sl.Err(err),
		).Error("parse get user response")
		return 0, err
	}
	if !response.Success {
		return 0, fmt.Errorf("get user discount: %s", response.Message)
	}
	return response.User.Discount, nil
}
