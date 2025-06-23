package entity

type HttpUserMsg struct {
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	TelegramId int64  `json:"telegram_id"`
	Message    string `json:"message"`
}
