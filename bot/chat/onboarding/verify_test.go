package onboarding

import (
	"DarkCS/bot/chat"
	"DarkCS/entity"
	"testing"
)

func TestNeedsVerification(t *testing.T) {
	user := &entity.User{Phone: "+380501234567", InstagramId: "ig-owner", TelegramId: 111}

	tests := []struct {
		name     string
		platform string
		userID   string
		verified bool
		want     bool
	}{
		{"instagram stranger typed the phone", "instagram", "ig-attacker", false, true},
		{"instagram already linked", "instagram", "ig-owner", false, false},
		{"telegram stranger", "telegram", "222", false, true},
		{"telegram same account", "telegram", "111", false, false},
		{"whatsapp own number", "whatsapp", "380501234567", false, false},
		{"whatsapp other number", "whatsapp", "380999999999", false, true},
		{"verified by code or contact", "instagram", "ig-attacker", true, false},
	}
	for _, tt := range tests {
		state := chat.NewChatState(tt.platform, tt.userID, tt.userID, WorkflowID, StepCheckUser)
		if tt.verified {
			state.Set(keyPhoneVerified, true)
		}
		if got := needsVerification(state, user); got != tt.want {
			t.Errorf("%s: needsVerification = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestRandomCode(t *testing.T) {
	code, err := randomCode(verifyCodeDigits)
	if err != nil || len(code) != verifyCodeDigits {
		t.Fatalf("code %q, err %v", code, err)
	}
	if hashCode(code) == code || hashCode(code) != hashCode(code) {
		t.Fatal("hash must be deterministic and not the plain code")
	}
}
