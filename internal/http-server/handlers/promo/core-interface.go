package promo

import "DarkCS/entity"

type Core interface {
	GeneratePromoCodes(number int) error
	GetActivePromoCodes() ([]entity.PromoCode, error)
}
