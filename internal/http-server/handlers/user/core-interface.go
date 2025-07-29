package user

import "DarkCS/entity"

type Core interface {
	BlockUser(email, phone string, telegramId int64, block bool) error
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
	CreateUser(name, email, phone string, telegramId int64) (string, string, error)
	UserHasPromoAccess(phone string) (bool, error)
	UserActivatePromoCode(phone, code string) (bool, error)
	ClosePromoForUser(phone string) error
}
