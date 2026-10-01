package onboarding

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log/slog"
	"math/big"
	"strconv"
	"strings"
	"time"

	"DarkCS/bot/chat"
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
)

// Phone ownership verification.
//
// A typed phone number proves nothing, so linking this chat to an existing customer
// found by a typed phone would let anyone take over that customer's account (orders,
// tracking numbers, ratings). Before linking, the phone must be verified:
//   - a Telegram contact share of the sender's own contact, or the WhatsApp number the
//     user is writing from, is verified by the platform;
//   - otherwise a one-time code is sent to the existing customer's Telegram and the user
//     must type it back here;
//   - if the customer has no Telegram, linking is refused and a manager has to help.

const (
	keyPhoneVerified  = "phone_verified"
	keyVerifyCodeHash = "verify_code_hash"
	keyVerifyExpires  = "verify_expires"
	keyVerifyAttempts = "verify_attempts"

	verifyCodeTTL      = 10 * time.Minute
	verifyMaxAttempts  = 3
	verifyCodeDigits   = 4
	msgVerifyNoChannel = "Цей номер телефону вже зареєстрований. Щоб підтвердити, що він ваш, " +
		"зверніться, будь ласка, до менеджера — він допоможе."
)

// CodeSender delivers a verification code to a customer's Telegram chat.
type CodeSender interface {
	SendTelegramText(telegramID int64, text string) error
}

// needsVerification reports whether linking this chat to user requires proof that the
// phone belongs to the sender.
func needsVerification(state *chat.ChatState, user *entity.User) bool {
	if state.GetBool(keyPhoneVerified) {
		return false
	}
	switch state.Platform {
	case "instagram":
		return user.InstagramId != state.UserID
	case "telegram":
		id, _ := strconv.ParseInt(state.UserID, 10, 64)
		return id == 0 || user.TelegramId != id
	case "whatsapp":
		return entity.NormalizePhone(state.UserID) != user.Phone
	}
	return true
}

// startVerification sends a code to the customer's Telegram and moves to StepVerifyCode,
// or refuses linking when there is no channel to send it through.
func startVerification(m chat.Messenger, state *chat.ChatState, user *entity.User, sender CodeSender, log *slog.Logger) chat.StepResult {
	log = log.With(slog.String("platform", state.Platform), slog.String("user_id", state.UserID),
		slog.String("customer_uuid", user.UUID))

	if user.TelegramId == 0 || sender == nil {
		log.Warn("onboarding: phone belongs to an existing customer without Telegram; link refused, manager needed")
		_ = m.SendText(state.ChatID, msgVerifyNoChannel)
		return chat.StepResult{NextStep: StepRequestPhone}
	}

	code, err := randomCode(verifyCodeDigits)
	if err != nil {
		log.Error("onboarding: generate verification code", sl.Err(err))
		_ = m.SendText(state.ChatID, "Виникла помилка. Спробуйте пізніше.")
		return chat.StepResult{NextStep: StepRequestPhone}
	}

	text := fmt.Sprintf("Код підтвердження: %s\n\nХтось вказав ваш номер телефону в чаті DarkCS (%s). "+
		"Якщо це ви — введіть код там. Якщо ні — просто проігноруйте це повідомлення.", code, platformName(state.Platform))
	if err = sender.SendTelegramText(user.TelegramId, text); err != nil {
		log.Error("onboarding: send verification code", sl.Err(err))
		_ = m.SendText(state.ChatID, msgVerifyNoChannel)
		return chat.StepResult{NextStep: StepRequestPhone}
	}
	log.Info("onboarding: verification code sent to customer's telegram")

	return chat.StepResult{
		NextStep: StepVerifyCode,
		UpdateState: map[string]any{
			keyVerifyCodeHash: hashCode(code),
			keyVerifyExpires:  time.Now().Add(verifyCodeTTL).Unix(),
			keyVerifyAttempts: 0,
		},
	}
}

// VerifyCodeStep — waits for the code sent to the existing customer's Telegram.
type VerifyCodeStep struct{}

func (s *VerifyCodeStep) ID() chat.StepID { return StepVerifyCode }

func (s *VerifyCodeStep) Enter(ctx context.Context, m chat.Messenger, state *chat.ChatState) chat.StepResult {
	_ = m.SendText(state.ChatID, "Цей номер уже зареєстрований. Ми надіслали код підтвердження "+
		"в Telegram-чат власника номера. Введіть код, щоб продовжити, або 0 — щоб ввести інший номер:")
	return chat.StepResult{}
}

func (s *VerifyCodeStep) HandleInput(ctx context.Context, m chat.Messenger, state *chat.ChatState, input chat.UserInput) chat.StepResult {
	text := strings.TrimSpace(input.Text)
	if text == "0" {
		return chat.StepResult{NextStep: StepRequestPhone, UpdateState: clearVerification()}
	}

	if time.Now().Unix() > int64(state.GetInt(keyVerifyExpires)) {
		_ = m.SendText(state.ChatID, "Час дії коду минув. Введіть номер телефону ще раз:")
		return chat.StepResult{NextStep: StepRequestPhone, UpdateState: clearVerification()}
	}

	expected := state.GetString(keyVerifyCodeHash)
	if expected != "" && subtle.ConstantTimeCompare([]byte(hashCode(text)), []byte(expected)) == 1 {
		update := clearVerification()
		update[keyPhoneVerified] = true
		return chat.StepResult{NextStep: StepCheckUser, UpdateState: update}
	}

	attempts := state.GetInt(keyVerifyAttempts) + 1
	if attempts >= verifyMaxAttempts {
		_ = m.SendText(state.ChatID, "Забагато невдалих спроб. Введіть номер телефону ще раз:")
		return chat.StepResult{NextStep: StepRequestPhone, UpdateState: clearVerification()}
	}
	_ = m.SendText(state.ChatID, fmt.Sprintf("❌ Невірний код. Залишилось спроб: %d", verifyMaxAttempts-attempts))
	return chat.StepResult{UpdateState: map[string]any{keyVerifyAttempts: attempts}}
}

func clearVerification() map[string]any {
	return map[string]any{
		keyVerifyCodeHash: "",
		keyVerifyExpires:  0,
		keyVerifyAttempts: 0,
	}
}

// hashCode keeps the plain code out of the stored chat state.
func hashCode(code string) string {
	sum := sha256.Sum256([]byte(code))
	return hex.EncodeToString(sum[:])
}

func randomCode(digits int) (string, error) {
	var b strings.Builder
	for i := 0; i < digits; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", err
		}
		b.WriteByte(byte('0' + n.Int64()))
	}
	return b.String(), nil
}

func platformName(platform string) string {
	switch platform {
	case "instagram":
		return "Instagram"
	case "whatsapp":
		return "WhatsApp"
	case "telegram":
		return "Telegram"
	}
	return platform
}
