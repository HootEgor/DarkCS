package product

import (
	"DarkCS/entity"
	"encoding/json"
)

type Response struct {
	Success  bool             `json:"success"`
	Products []entity.Product `json:"data"`
	Message  string           `json:"message"`
}

func ParseResponse(body []byte) (*Response, error) {
	var response Response
	err := json.Unmarshal(body, &response)
	if err != nil {
		return nil, err
	}
	return &response, nil
}
