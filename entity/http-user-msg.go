package entity

type HttpUserMsg struct {
	Email      string `json:"email"`
	Phone      string `json:"phone"`
	TelegramId string `json:"telegram_id"`
	Message    string `json:"message"`
}
