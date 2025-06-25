package user

import "DarkCS/entity"

type Core interface {
	BlockUser(email, phone string, telegramId int64, block bool) error
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
}
