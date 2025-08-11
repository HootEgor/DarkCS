package smart

type Core interface {
	SendMessage(userId, text string) error
}
