package product

import (
	"DarkCS/entity"
	"encoding/json"
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

func ParseOrderValidateResponse(body []byte) ([]entity.OrderProduct, error) {
	var response []entity.OrderProduct
	err := json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}
	return response, nil
}
