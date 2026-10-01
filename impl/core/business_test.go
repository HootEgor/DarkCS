package core

import (
	"testing"

	"DarkCS/entity"
)

func TestContactKey(t *testing.T) {
	registered := &entity.User{UUID: "u-1"}
	tests := []struct {
		name     string
		platform string
		userID   string
		user     *entity.User
		want     string
	}{
		{"registered user groups by uuid", entity.PlatformInstagram, "ig1", registered, "u-1"},
		{"unregistered bot chat", entity.PlatformTelegram, "42", nil, "tg:42"},
		{"unregistered business chat matches bot chat", entity.PlatformTelegramBusiness, "42", nil, "tg:42"},
		{"unregistered whatsapp", entity.PlatformWhatsApp, "380", nil, "whatsapp:380"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := contactKey(tt.platform, tt.userID, tt.user); got != tt.want {
				t.Fatalf("contactKey = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChatKey(t *testing.T) {
	if got := entity.ChatKey("telegram", "1", ""); got != "telegram:1" {
		t.Fatalf("legacy key = %q", got)
	}
	if got := entity.ChatKey("telegram_business", "1", "9"); got != "telegram_business:1:9" {
		t.Fatalf("channel key = %q", got)
	}
}
