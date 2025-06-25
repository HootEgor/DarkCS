package gpt

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"encoding/json"
	"fmt"
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
	default:
		return "", nil
	}
}

type getProductInfoResp struct {
	Codes []string `json:"codes"`
}

type UpdateUserPhoneResp struct {
	Phone string `json:"phone"`
}

func (o *Overseer) handleGetProductInfo(args string) ([]entity.ProductInfo, error) {
	var resp *getProductInfoResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		o.log.With(
			slog.String("args", args),
			sl.Err(err),
		).Error("unmarshalling response")
		return nil, err
	}

	productsInfo, err := o.productService.GetProductInfo(resp.Codes)
	if err != nil {
		o.log.With(
			slog.Any("codes", resp.Codes),
			sl.Err(err),
		).Error("getting product info")
		return nil, err
	}

	return productsInfo, nil
}

func (o *Overseer) handleUpdateUserPhone(userId, args string) (string, error) {
	var resp *UpdateUserPhoneResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		o.log.With(
			slog.String("args", args),
			sl.Err(err),
		).Error("unmarshalling response")
		return fmt.Sprintf("error unmarshalling response: %v", err), nil
	}
	phone := resp.Phone

	email, _, telegramId, err := entity.GetUserDataFromId(userId)
	if err != nil {
		o.log.With(
			slog.String("user", userId),
			slog.String("phone", phone),
			sl.Err(err),
		).Error("parsing user data")
		return fmt.Sprintf("Error parsing user data: %v", err), nil
	}
	err = o.authService.UpdateUserPhone(email, phone, telegramId)
	if err != nil {
		o.log.With(
			slog.String("user", userId),
			slog.String("phone", phone),
			sl.Err(err),
		).Error("updating user phone")
		return fmt.Sprintf("Error updating phone: %v", err), nil
	}

	return "Phone updated successfully", nil
}
