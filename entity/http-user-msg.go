package entity

type HttpUserMsg struct {
	Email          string `json:"email"`
	Phone          string `json:"phone"`
	TelegramId     int64  `json:"telegram_id"`
	SmartSenderId  string `json:"smart_sender_id"`
	WithHtmlLinks  bool   `json:"with_html_links"`
	Message        string `json:"message"`
	VoiceMsgBase64 string `json:"voice_msg_base64"`
}
