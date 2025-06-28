package gpt

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"encoding/json"
	"log/slog"
)

func (o *Overseer) handleCommand(user *entity.User, name, args string) (interface{}, error) {
	o.log.With(
		slog.String("command", name),
		slog.String("args", args),
	).Debug("handling command")
	switch name {
	case "get_products_info":
		return o.handleGetProductInfo(args)
	case "update_user_phone":
		return o.handleUpdateUserPhone(user, args)
	case "get_basket":
		return o.handleGetBasket(user)
	case "add_to_basket":
		return o.handleAddToBasket(user, args)
	case "remove_from_basket":
		return o.handleRemoveFromBasket(user, args)
	case "create_order":
		return o.handleCreateOrder(user)
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

type orderResp struct {
	Products []entity.OrderProduct `json:"products"`
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

func (o *Overseer) handleUpdateUserPhone(user *entity.User, args string) (string, error) {

	var resp *updateUserPhoneResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		return "", err
	}
	phone := resp.Phone

	err = o.authService.UpdateUserPhone(user.Email, phone, user.TelegramId)
	if err != nil {
		return "", err
	}

	return "Phone updated successfully", nil
}

func (o *Overseer) handleGetBasket(user *entity.User) (interface{}, error) {
	basket, err := o.authService.GetBasket(user.UUID)
	if err != nil {
		return nil, err
	}

	return basket, nil
}

func (o *Overseer) handleRemoveFromBasket(user *entity.User, args string) (interface{}, error) {
	var resp *orderResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		return nil, err
	}

	basket, err := o.authService.RemoveFromBasket(user.UUID, resp.Products)
	if err != nil {
		return nil, err
	}

	return basket, nil
}

func (o *Overseer) handleAddToBasket(user *entity.User, args string) (interface{}, error) {
	var resp *orderResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		return nil, err
	}

	basket, err := o.authService.AddToBasket(user.UUID, resp.Products)
	if err != nil {
		return nil, err
	}

	return basket, nil
}

func (o *Overseer) handleCreateOrder(user *entity.User) (interface{}, error) {

	basket, err := o.authService.GetBasket(user.UUID)
	if err != nil {
		return nil, err
	}

	msg := struct {
		Basket entity.Basket `json:"basket"`
		Msg    string        `json:"msg"`
		Phone  string        `json:"phone,omitempty"`
	}{}

	msg.Basket = *basket
	msg.Msg = "Order created successfully"
	msg.Phone = user.Phone

	err = o.authService.ClearBasket(user.UUID)
	if err != nil {
		o.log.With(
			slog.String("user", user.UUID),
			sl.Err(err),
		).Error("clear basket after order creation")
	}

	return msg, nil
}
