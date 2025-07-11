package services

import (
	"DarkCS/entity"
	"fmt"
	"strconv"
	"time"
)

func (s *ZohoService) CreateOrder(order *entity.Order) error {

	if time.Now().After(s.tokenExpiresIn.Add(time.Minute * time.Duration(-5))) {
		err := s.refreshTokenCall()
		if err != nil {
			return err
		}
	}

	contact := order.User.ToContact()

	contactID, err := s.createContact(*contact)
	if err != nil {
		return err
	}

	if hasEmptyName(order.Products) {
		return fmt.Errorf("order still has product(s) without name")
	}

	if hasEmptyZohoID(order.Products) {
		return fmt.Errorf("order still has product(s) without Zoho ID")
	}

	zohoOrder := s.buildZohoOrder(order, contactID)

	err = s.createOrder(zohoOrder)

	return err
}

func hasEmptyZohoID(products []entity.OrderProduct) bool {
	for _, p := range products {
		if p.ZohoId == "" {
			return true
		}
	}
	return false
}

func hasEmptyName(products []entity.OrderProduct) bool {
	for _, p := range products {
		if p.Name == "" {
			return true
		}
	}
	return false
}

func (s *ZohoService) buildZohoOrder(order *entity.Order, contactID string) entity.ZohoOrder {
	orderedItems := convertToOrderedItems(order.Products)

	return entity.ZohoOrder{
		ContactName:        entity.ContactName{ID: contactID},
		OrderedItems:       orderedItems,
		Discount:           0,
		Description:        "order from bot",
		CustomerNo:         "",
		ShippingState:      "",
		Tax:                0,
		BillingCountry:     "",
		Carrier:            "",
		Status:             entity.OrderStatusNew,
		SalesCommission:    0,
		DueDate:            time.Now().Format("2006-01-02"),
		BillingStreet:      order.User.Address,
		Adjustment:         0,
		TermsAndConditions: "Standard terms apply.",
		BillingCode:        "",
		ProductDetails:     nil,
		Location:           "Україна",
		OrderSource:        "AI Bot",
		Subject:            fmt.Sprintf("AI Bot - %s", order.User.Name),
	}
}

func convertToOrderedItems(details []entity.OrderProduct) []entity.OrderedItem {
	var orderedItems []entity.OrderedItem

	for _, d := range details {
		if !d.Available {
			continue
		}
		price, _ := strconv.ParseFloat(d.Price, 64)
		item := entity.OrderedItem{
			Product: entity.ZohoProduct{
				ID: d.ZohoId,
				//Name: d.Name,
			},
			Quantity:  d.Quantity,
			Discount:  0,
			DiscountP: 0,
			ListPrice: roundToTwoDecimalPlaces(price),
			Total:     roundToTwoDecimalPlaces(price * float64(d.Quantity)),
		}
		orderedItems = append(orderedItems, item)
	}

	return orderedItems
}

func roundToTwoDecimalPlaces(value float64) float64 {
	return float64(int(value*100)) / 100.0
}
