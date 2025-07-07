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
	case "update_user_email":
		return o.handleUpdateUserEmail(user, args)
	case "update_user_address":
		return o.handleUpdateUserAddress(user, args)
	case "update_user_name":
		return o.handleUpdateUserName(user, args)
	case "get_user_info":
		return o.handleGetUserInfo(user)
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

type updateUserEmailResp struct {
	Email string `json:"email"`
}

type updateUserAddressResp struct {
	Address string `json:"address"`
}

type updateUserNameResp struct {
	Name string `json:"name"`
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
	user.Phone = resp.Phone

	err = o.authService.UpdateUser(user)
	if err != nil {
		return "", err
	}

	return "Phone updated successfully", nil
}

func (o *Overseer) handleUpdateUserEmail(user *entity.User, args string) (string, error) {

	var resp *updateUserEmailResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		return "", err
	}
	user.Email = resp.Email

	err = o.authService.UpdateUser(user)
	if err != nil {
		return "", err
	}

	return "Email updated successfully", nil
}

func (o *Overseer) handleUpdateUserAddress(user *entity.User, args string) (string, error) {

	var resp *updateUserAddressResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		return "", err
	}
	user.Address = resp.Address

	err = o.authService.UpdateUser(user)
	if err != nil {
		return "", err
	}

	return "Email updated successfully", nil
}

func (o *Overseer) handleUpdateUserName(user *entity.User, args string) (string, error) {

	var resp *updateUserNameResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		return "", err
	}
	user.Name = resp.Name

	err = o.authService.UpdateUser(user)
	if err != nil {
		return "", err
	}

	return "Name updated successfully", nil
}

func (o *Overseer) handleGetUserInfo(user *entity.User) (interface{}, error) {
	return user.GetInfo(), nil
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
