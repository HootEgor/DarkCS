// Package gpt provides functionality for handling AI-powered interactions and commands
// in the DarkCS system. This package includes command handling, user management,
// product operations, and order processing capabilities.
package gpt

import (
	"DarkCS/entity"
	"encoding/json"
	"log/slog"
)

// handleCommand processes user commands and routes them to appropriate handlers.
// It takes a user entity, command name, and arguments as input, then delegates
// the processing to specific handler functions based on the command name.
//
// Parameters:
//   - user: The user entity initiating the command
//   - name: The name of the command to execute
//   - args: JSON-formatted string containing command arguments
//
// Returns:
//   - interface{}: Command execution result, varies by command type
//   - error: Any error encountered during command processing
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
	case "clear_basket":
		return o.handleClearBasket(user)
	case "add_to_basket":
		return o.handleAddToBasket(user, args)
	case "remove_from_basket":
		return o.handleRemoveFromBasket(user, args)
	case "validate_order":
		return o.handleValidateOrder(user)
	case "create_order":
		return o.handleCreateOrder(user)
	default:
		return "", nil
	}
}

// getProductInfoResp represents the request structure for retrieving product information.
// It contains a list of product codes to look up.
type getProductInfoResp struct {
	Codes []string `json:"codes"`
}

// updateUserPhoneResp represents the request structure for updating a user's phone number.
type updateUserPhoneResp struct {
	Phone string `json:"phone"`
}

// updateUserEmailResp represents the request structure for updating a user's email address.
type updateUserEmailResp struct {
	Email string `json:"email"`
}

// updateUserAddressResp represents the request structure for updating a user's physical address.
type updateUserAddressResp struct {
	Address string `json:"address"`
}

// updateUserNameResp represents the request structure for updating a user's name.
type updateUserNameResp struct {
	Name string `json:"name"`
}

// orderResp represents the request structure for basket operations (add/remove products).
// It contains a list of products with their details.
type orderResp struct {
	Products []entity.OrderProduct `json:"products"`
}

// handleGetProductInfo retrieves detailed information about products based on their codes.
// It unmarshals the JSON arguments to extract product codes, then queries the product service.
//
// Parameters:
//   - args: JSON string containing product codes to look up
//
// Returns:
//   - []entity.ProductInfo: Array of product information objects
//   - error: Any error encountered during processing
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

// handleUpdateUserPhone updates a user's phone number.
// It unmarshals the JSON arguments to extract the new phone number,
// updates the user entity, and persists the changes.
//
// Parameters:
//   - user: User entity to update
//   - args: JSON string containing the new phone number
//
// Returns:
//   - string: Success message
//   - error: Any error encountered during processing
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

// handleUpdateUserEmail updates a user's email address.
// It unmarshals the JSON arguments to extract the new email address,
// updates the user entity, and persists the changes.
//
// Parameters:
//   - user: User entity to update
//   - args: JSON string containing the new email address
//
// Returns:
//   - string: Success message
//   - error: Any error encountered during processing
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

// handleUpdateUserAddress updates a user's physical address.
// It unmarshals the JSON arguments to extract the new address,
// updates the user entity, and persists the changes.
//
// Parameters:
//   - user: User entity to update
//   - args: JSON string containing the new address
//
// Returns:
//   - string: Success message
//   - error: Any error encountered during processing
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

	return "Address updated successfully", nil
}

// handleUpdateUserName updates a user's name.
// It unmarshals the JSON arguments to extract the new name,
// updates the user entity, and persists the changes.
//
// Parameters:
//   - user: User entity to update
//   - args: JSON string containing the new name
//
// Returns:
//   - string: Success message
//   - error: Any error encountered during processing
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

// handleGetUserInfo retrieves information about the user.
// It returns a simplified representation of the user entity.
//
// Parameters:
//   - user: The user entity to get information about
//
// Returns:
//   - interface{}: User information in a format suitable for external use
//   - error: Any error encountered during processing
func (o *Overseer) handleGetUserInfo(user *entity.User) (interface{}, error) {
	return user.GetInfo(), nil
}

// handleClearBasket removes all products from the user's shopping basket.
//
// Parameters:
//   - user: The user whose basket should be cleared
//
// Returns:
//   - interface{}: Result of the clear operation
//   - error: Any error encountered during processing
func (o *Overseer) handleClearBasket(user *entity.User) (interface{}, error) {
	return o.authService.ClearBasket(user.UUID), nil
}

// handleGetBasket retrieves the current contents of the user's shopping basket.
// If the basket is empty or doesn't exist, it returns nil.
//
// Parameters:
//   - user: The user whose basket should be retrieved
//
// Returns:
//   - interface{}: Basket contents formatted for the assistant
//   - error: Any error encountered during processing
func (o *Overseer) handleGetBasket(user *entity.User) (interface{}, error) {
	basket, err := o.authService.GetBasket(user.UUID)
	if err != nil {
		return nil, err
	}

	if basket == nil {
		return nil, nil
	}

	return entity.ProdForAssistant(basket.Products), nil
}

// handleRemoveFromBasket removes specified products from the user's shopping basket.
// It unmarshals the JSON arguments to extract the products to remove.
//
// Parameters:
//   - user: The user whose basket should be modified
//   - args: JSON string containing products to remove
//
// Returns:
//   - interface{}: Updated basket contents formatted for the assistant
//   - error: Any error encountered during processing
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

	if basket == nil {
		return nil, nil
	}

	return entity.ProdForAssistant(basket.Products), nil
}

// handleAddToBasket adds products to the user's shopping basket.
// It unmarshals the JSON arguments to extract the products to add,
// retrieves detailed product information, and updates the basket.
//
// Parameters:
//   - user: The user whose basket should be modified
//   - args: JSON string containing products to add
//
// Returns:
//   - interface{}: Updated basket contents formatted for the assistant
//   - error: Any error encountered during processing
func (o *Overseer) handleAddToBasket(user *entity.User, args string) (interface{}, error) {
	var resp *orderResp
	err := json.Unmarshal([]byte(args), &resp)
	if err != nil {
		return nil, err
	}

	// Extract product codes from the request
	codes := make([]string, 0, len(resp.Products))
	for _, product := range resp.Products {
		codes = append(codes, product.Code)
	}

	// Get detailed product information
	productsInfo, err := o.productService.GetProductInfo(codes)
	if err != nil {
		return nil, err
	}

	// Create complete product objects with all necessary information
	var products []entity.OrderProduct
	for _, product := range resp.Products {
		for _, info := range productsInfo {
			if product.Code == info.Code {
				products = append(products, entity.OrderProduct{
					Name:     info.Name,
					Code:     product.Code,
					Quantity: product.Quantity,
					Price:    info.Price,
				})
				break
			}
		}
	}

	// Add products to the basket
	basket, err := o.authService.AddToBasket(user.UUID, products)
	if err != nil {
		return nil, err
	}

	if basket == nil {
		return nil, nil
	}

	return entity.ProdForAssistant(basket.Products), nil
}

// canUserOrder checks if a user is allowed to create a new order.
// It verifies that the user doesn't have any active orders in progress.
//
// Parameters:
//   - user: The user to check
//
// Returns:
//   - bool: true if the user can create a new order, false otherwise
//   - error: Any error encountered during processing
func (o *Overseer) canUserOrder(user *entity.User) (bool, error) {
	// Get all orders for the user
	orders, err := o.zohoService.GetOrders(*user.GetInfo())
	if err != nil {
		return false, err
	}

	// If user has no orders, they can create a new one
	if orders == nil || len(orders) == 0 {
		return true, nil
	}

	// Check if any orders are in an active state
	for _, order := range orders {
		if order.Status == entity.OrderStatusNew || order.Status == entity.OrderStatusProcessing || order.Status == entity.OrderStatusInvoiced {
			return false, nil
		}
	}

	return true, nil
}

// handleValidateOrder validates the products in the user's basket.
// It checks product availability, updates prices, and verifies if the user can place an order.
//
// Parameters:
//   - user: The user whose basket should be validated
//
// Returns:
//   - interface{}: Validated basket contents or error message
//   - error: Any error encountered during processing
func (o *Overseer) handleValidateOrder(user *entity.User) (interface{}, error) {
	// Get the user's basket
	basket, err := o.authService.GetBasket(user.UUID)
	if err != nil {
		return nil, err
	}

	// Extract product codes from the basket
	codes := make([]string, 0, len(basket.Products))
	for _, product := range basket.Products {
		codes = append(codes, product.Code)
	}

	// Get detailed product information
	productsInfo, err := o.productService.GetProductInfo(codes)
	if err != nil {
		return nil, err
	}

	// Discount functionality is commented out but kept for future implementation
	//discount, err := o.productService.GetUserDiscount(user.Phone)
	//if err != nil {
	//	return nil, err
	//}

	// Create complete product objects with all necessary information
	var products []entity.OrderProduct
	for _, product := range basket.Products {
		for _, info := range productsInfo {
			if product.Code == info.Code {
				products = append(products, entity.OrderProduct{
					Name:     info.Name,
					Code:     product.Code,
					Quantity: product.Quantity,
					Price:    info.Price,
					//Discount: discount,
				})
				break
			}
		}
	}

	// Validate the order with the product service
	validProducts, err := o.productService.ValidateOrder(products, user.Phone)
	if err != nil {
		return nil, err
	}

	// Update the basket with validated products
	basket, err = o.authService.UpdateBasket(user.UUID, validProducts)
	if err != nil {
		return nil, err
	}

	// Check if the user can place a new order
	canOrder, err := o.canUserOrder(user)
	if err != nil {
		return nil, err
	}

	// If user has an active order, return a message explaining the situation
	if !canOrder {
		msg := struct {
			Message  string      `json:"message"`
			Products interface{} `json:"products"`
		}{}
		msg.Message = "Products are validated but, user have an active order, please wait until it is processed before creating a new one."
		msg.Products = entity.ProdForAssistant(basket.Products)
		return msg, nil
	}

	return entity.ProdForAssistant(basket.Products), nil
}

// handleCreateOrder creates a new order from the user's basket.
// It verifies that the user can place an order, then submits it to the order processing system.
//
// Parameters:
//   - user: The user creating the order
//
// Returns:
//   - interface{}: Success message or error message
//   - error: Any error encountered during processing
func (o *Overseer) handleCreateOrder(user *entity.User) (interface{}, error) {
	// Check if the user can place a new order
	canOrder, err := o.canUserOrder(user)
	if err != nil {
		return nil, err
	}

	// If user has an active order, return an error message
	if !canOrder {
		return "User have an active order, please wait until it is processed before creating a new one.", nil
	}

	// Get the user's basket
	basket, err := o.authService.GetBasket(user.UUID)
	if err != nil {
		return nil, err
	}

	// Create a new order from the basket contents
	order := entity.Order{
		User:     *user.GetInfo(),
		Products: basket.Products,
	}

	// Submit the order to the order processing system
	err = o.zohoService.CreateOrder(&order)
	if err != nil {
		return nil, err
	}

	// Return success message and clear the basket
	return "order created successfully", o.authService.ClearBasket(user.UUID)
}
