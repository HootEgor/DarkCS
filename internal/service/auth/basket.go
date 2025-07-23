package auth

import (
	"DarkCS/entity"
	"fmt"
)

func (s *Service) GetBasket(userUUID string) (*entity.Basket, error) {
	return s.repository.GetBasket(userUUID)
}

func (s *Service) ClearBasket(userUUID string) error {

	basket := &entity.Basket{
		UserUUID: userUUID,
		Products: []entity.OrderProduct{},
	}

	_, err := s.repository.UpsertBasket(basket)
	return err
}

func (s *Service) UpdateBasket(userUUID string, products []entity.OrderProduct) (*entity.Basket, error) {
	basket, err := s.repository.GetBasket(userUUID)
	if err != nil {
		return nil, err
	}

	if basket == nil {
		basket = &entity.Basket{
			UserUUID: userUUID,
			Products: []entity.OrderProduct{},
		}
	}

	basket.Products = products

	return s.repository.UpsertBasket(basket)
}

func (s *Service) AddToBasket(userUUID string, products []entity.OrderProduct) (*entity.Basket, error) {
	basket, err := s.repository.GetBasket(userUUID)
	if err != nil {
		return nil, err
	}

	if basket == nil {
		basket = &entity.Basket{
			UserUUID: userUUID,
			Products: []entity.OrderProduct{},
		}
	}

	// Create a map to merge product quantities
	productMap := make(map[string]int)
	for _, p := range basket.Products {
		productMap[p.Code] = p.Quantity
	}

	// Add/update the incoming products
	for _, p := range products {
		productMap[p.Code] += p.Quantity
	}

	// Build new product list
	var newProducts []entity.OrderProduct
	for code, quantity := range productMap {
		if quantity > 0 {
			newProducts = append(newProducts, entity.OrderProduct{
				Code:     code,
				Quantity: quantity,
			})
		}
	}
	basket.Products = newProducts

	return s.repository.UpsertBasket(basket)
}

func (s *Service) RemoveFromBasket(userUUID string, products []entity.OrderProduct) (*entity.Basket, error) {
	basket, err := s.repository.GetBasket(userUUID)
	if err != nil {
		return nil, err
	}

	if basket == nil {
		return nil, fmt.Errorf("the basket for user does not exist")
	}

	// Create a map for quick lookup of products to remove
	removeMap := make(map[string]int)
	for _, p := range products {
		removeMap[p.Code] = p.Quantity
	}

	var newProducts []entity.OrderProduct
	for _, p := range basket.Products {
		if removeQty, exists := removeMap[p.Code]; exists {
			newQty := p.Quantity - removeQty
			if newQty > 0 {
				newProducts = append(newProducts, entity.OrderProduct{
					Code:     p.Code,
					Quantity: newQty,
				})
			}
			// else: product removed entirely
		} else {
			newProducts = append(newProducts, p)
		}
	}
	basket.Products = newProducts

	return s.repository.UpsertBasket(basket)
}
