package user

import "DarkCS/entity"

type Core interface {
	BlockUser(email, phone string, telegramId int64, block bool, role string) error
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
	CreateUser(name, email, phone, smartSenderId string, telegramId int64) (string, string, error)
	UserHasPromoAccess(phone string) (bool, error)
	UserActivatePromoCode(phone, code string) (bool, error)
	ClosePromoForUser(phone string) error
	CheckUserPhone(phone string) (string, error)
	ResetConversation(phone string) error
	ImportTelegramUsers(items []entity.TelegramImportItem) (int, error)
}
