package auth

import (
	"DarkCS/entity"
	"fmt"
	"io"
	"log/slog"
	"testing"

	"go.mongodb.org/mongo-driver/bson/primitive"
)

// fakeRepo is an in-memory Repository mirroring the Mongo semantics the service relies
// on: GetUser matches any given identifier, writes are field-level by UUID.
type fakeRepo struct {
	users  []*entity.User
	promos map[string]*entity.PromoCode
}

func (f *fakeRepo) CreateUser(u *entity.User) error {
	c := *u
	f.users = append(f.users, &c)
	return nil
}

func (f *fakeRepo) byUUID(uuid string) *entity.User {
	for _, u := range f.users {
		if u.UUID == uuid {
			return u
		}
	}
	return nil
}

func (f *fakeRepo) UpdateUserFields(uuid string, fields map[string]any) error {
	u := f.byUUID(uuid)
	if u == nil {
		return fmt.Errorf("user %s not found", uuid)
	}
	applyFields(u, fields)
	return nil
}

func (f *fakeRepo) SetUserUUID(id primitive.ObjectID, uuid string) error { return nil }

func (f *fakeRepo) PushConversation(uuid string, m entity.DialogMessage, keep int) error {
	u := f.byUUID(uuid)
	if u == nil {
		return fmt.Errorf("user %s not found", uuid)
	}
	u.Conversation = append(u.Conversation, m)
	if len(u.Conversation) > keep {
		u.Conversation = u.Conversation[len(u.Conversation)-keep:]
	}
	return nil
}

func (f *fakeRepo) GetUser(email, phone string, telegramId int64) (*entity.User, error) {
	phone = entity.NormalizePhone(phone)
	for _, u := range f.users {
		if (email != "" && u.Email == email) || (phone != "" && u.Phone == phone) ||
			(telegramId != 0 && u.TelegramId == telegramId) {
			c := *u
			return &c, nil
		}
	}
	return nil, nil
}

func (f *fakeRepo) GetUserByUUID(uuid string) (*entity.User, error) {
	if u := f.byUUID(uuid); u != nil {
		c := *u
		return &c, nil
	}
	return nil, nil
}

func (f *fakeRepo) GetUserByInstagramId(string) (*entity.User, error)   { return nil, nil }
func (f *fakeRepo) GetUserBySmartSenderId(string) (*entity.User, error) { return nil, nil }
func (f *fakeRepo) UpsertBasket(b *entity.Basket) (*entity.Basket, error) {
	return b, nil
}
func (f *fakeRepo) GetBasket(string) (*entity.Basket, error) { return nil, nil }
func (f *fakeRepo) GetPromoCode(code string) (*entity.PromoCode, error) {
	return f.promos[code], nil
}
func (f *fakeRepo) ActivatePromoCode(code string) error {
	f.promos[code].Activated = true
	return nil
}
func (f *fakeRepo) DeactivatePromoCode(code string) error {
	f.promos[code].Activated = false
	return nil
}
func (f *fakeRepo) SavePromoCodes([]string) error                 { return nil }
func (f *fakeRepo) GetAllPromoCodes() ([]entity.PromoCode, error) { return nil, nil }

func newTestService(users ...*entity.User) (*Service, *fakeRepo) {
	repo := &fakeRepo{users: users, promos: map[string]*entity.PromoCode{}}
	s := NewAuthService(slog.New(slog.NewTextHandler(io.Discard, nil)))
	s.SetRepository(repo)
	return s, repo
}

func existingUser() *entity.User {
	return &entity.User{
		UUID: "uuid-1", Name: "Olena", Phone: "+380501234567", Role: entity.UserRole,
		ZohoId: "zoho-1", TelegramId: 111,
	}
}

// Regression: a phone without "+" used to miss the lookup, auto-register a blank guest
// and overwrite the real user with it.
func TestGetUserUnnormalizedPhoneFindsExisting(t *testing.T) {
	s, repo := newTestService(existingUser())

	u, err := s.GetUser("", "380501234567", 0)
	if err != nil || u == nil || u.UUID != "uuid-1" {
		t.Fatalf("got %+v, %v", u, err)
	}
	if len(repo.users) != 1 {
		t.Fatalf("lookup created users: %d", len(repo.users))
	}
	if got := repo.users[0]; got.Name != "Olena" || got.ZohoId != "zoho-1" || got.Role != entity.UserRole {
		t.Fatalf("existing user modified: %+v", got)
	}
}

func TestGetUserNeverRegisters(t *testing.T) {
	s, repo := newTestService()
	u, err := s.GetUser("", "+380000000000", 0)
	if err != nil || u != nil || len(repo.users) != 0 {
		t.Fatalf("read path registered a user: %+v %v", u, err)
	}
}

func TestGetOrCreateUser(t *testing.T) {
	s, repo := newTestService(existingUser())
	if u, _ := s.GetOrCreateUser("", "380501234567", 0); u.UUID != "uuid-1" || len(repo.users) != 1 {
		t.Fatal("existing user must be returned, not duplicated")
	}
	if u, _ := s.GetOrCreateUser("", "+380999999999", 0); u == nil || len(repo.users) != 2 {
		t.Fatal("unknown user must be created")
	}
	if _, err := s.GetOrCreateUser("", "", 0); err == nil {
		t.Fatal("no identifier must be an error")
	}
}

// Regression: the conversation write used to save the whole (stale) user, reverting
// fields changed meanwhile.
func TestUpdateConversationKeepsOtherFields(t *testing.T) {
	s, repo := newTestService(existingUser())
	stale := *repo.users[0]

	if err := s.UpdateUserFields(repo.users[0], map[string]any{entity.UserFieldBlocked: true}); err != nil {
		t.Fatal(err)
	}
	if err := s.UpdateConversation(stale, entity.DialogMessage{Question: "q", Answer: "a"}); err != nil {
		t.Fatal(err)
	}
	if !repo.users[0].Blocked {
		t.Fatal("conversation update reverted the block")
	}
	if len(repo.users[0].Conversation) != 1 {
		t.Fatal("conversation not appended")
	}
}

func TestBlockUser(t *testing.T) {
	s, repo := newTestService(existingUser())

	if err := s.BlockUser("", "+380501234567", 0, true, ""); err != nil {
		t.Fatal(err)
	}
	if !repo.users[0].Blocked || repo.users[0].Role != entity.UserRole {
		t.Fatalf("blocking without a role must keep the role: %+v", repo.users[0])
	}
	if err := s.BlockUser("", "+380501234567", 0, false, "superuser"); err == nil {
		t.Fatal("unknown role must be rejected")
	}
	if err := s.BlockUser("", "+380000000000", 0, true, ""); err == nil {
		t.Fatal("unknown user must be an error, not created")
	}
}

func TestActivatePromoCode(t *testing.T) {
	s, repo := newTestService(existingUser())
	repo.promos["CODE1"] = &entity.PromoCode{Code: "CODE1"}

	if err := s.ActivatePromoCode("380501234567", "CODE1"); err != nil {
		t.Fatal(err)
	}
	if !repo.promos["CODE1"].Activated || !repo.users[0].HasPromo() {
		t.Fatal("promo not granted")
	}
	if err := s.ActivatePromoCode("380501234567", "CODE1"); err == nil {
		t.Fatal("reusing a code must fail")
	}
	if err := s.ActivatePromoCode("+380000000000", "CODE2"); err == nil {
		t.Fatal("unknown user must be an error")
	}
}

func TestTruncateUTF8(t *testing.T) {
	if got := truncateUTF8("Привіт", 5); got != "Пр" { // 2 bytes per rune
		t.Fatalf("got %q", got)
	}
}
