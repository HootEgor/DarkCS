package user

type Core interface {
	BlockUser(email, phone string, telegramId int64, block bool) error
}
