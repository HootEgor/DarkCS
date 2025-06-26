package auth

import "DarkCS/entity"

func (s *Service) GetBasket(userUUID string) (*entity.Basket, error) {
	return s.repository.GetBasket(userUUID)
}

func (s *Service) AddToBasket(userUUID string, products []entity.OrderProduct) (*entity.Basket, error) {
	basket, err := s.repository.GetBasket(userUUID)
	if err != nil {
		// If basket doesn't exist, create a new one
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
