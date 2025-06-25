package gpt

import (
	"DarkCS/entity"
	"encoding/json"
	"log/slog"
)

func (o *Overseer) handleCommand(userId, name, args string) (interface{}, error) {
	o.log.With(
		slog.String("command", name),
		slog.String("args", args),
	).Debug("handling command")
	switch name {
	case "get_products_info":
		return o.handleGetProductInfo(args)
	case "update_user_phone":
		return o.handleUpdateUserPhone(userId, args)
	case "create_order":
		return o.handleCreateOrder(userId, args)
	default:
		return "", nil
	}
}

type getProductInfoResp struct {
	Codes []string `json:"codes"`
}

type updateUserPhoneResp struct {
	Phone string `json:"phone"`
}

type createOrderResp struct {
	Codes []string `json:"codes"`
}

func (o *Overseer) handleGetProductInfo(args string) ([]entity.ProductInfo, error) {
	var resp *getProductInfoResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		return nil, err
	}

	productsInfo, err := o.productService.GetProductInfo(resp.Codes)
	if err != nil {
		return nil, err
	}

	return productsInfo, nil
}

func (o *Overseer) handleUpdateUserPhone(userId, args string) (string, error) {

	var resp *updateUserPhoneResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		return "", err
	}
	phone := resp.Phone

	email, _, telegramId, err := entity.GetUserDataFromId(userId)
	if err != nil {
		return "", err
	}
	err = o.authService.UpdateUserPhone(email, phone, telegramId)
	if err != nil {
		return "", err
	}

	return "Phone updated successfully", nil
}

func (o *Overseer) handleCreateOrder(userId, args string) (interface{}, error) {
	var resp *createOrderResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		return nil, err
	}

	_, phone, _, err := entity.GetUserDataFromId(userId)
	if err != nil {
		return "", err
	}

	productsInfo, err := o.productService.GetProductInfo(resp.Codes)
	if err != nil {
		return nil, err
	}

	msg := struct {
		Products []entity.ProductInfo `json:"products"`
		Msg      string               `json:"msg"`
		Phone    string               `json:"phone,omitempty"`
	}{}

	msg.Products = productsInfo
	msg.Msg = "Order created successfully"
	msg.Phone = phone

	return msg, nil
}
