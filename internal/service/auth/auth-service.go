// Package auth manages customer records: lookup, registration, promo access, baskets and
// the AI conversation history stored on the user document.
//
// There is deliberately no in-memory user cache: it was shared across goroutines without
// locking and served stale copies that were then written back over newer data. All user
// writes are field-level (UpdateUserFields, PushConversation) for the same reason.
package auth

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"fmt"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"
)

type Repository interface {
	CreateUser(user *entity.User) error
	UpdateUserFields(uuid string, fields map[string]any) error
	SetUserUUID(id primitive.ObjectID, uuid string) error
	PushConversation(uuid string, message entity.DialogMessage, keep int) error
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
	GetUserByUUID(uuid string) (*entity.User, error)
	GetUserByInstagramId(instagramId string) (*entity.User, error)
	GetUserBySmartSenderId(smartSenderId string) (*entity.User, error)

	UpsertBasket(basket *entity.Basket) (*entity.Basket, error)
	GetBasket(userUUID string) (*entity.Basket, error)

	GetPromoCode(code string) (*entity.PromoCode, error)
	ActivatePromoCode(code string) error
	DeactivatePromoCode(code string) error
	SavePromoCodes(codes []string) error
	GetAllPromoCodes() ([]entity.PromoCode, error)
}

type Service struct {
	repository Repository
	log        *slog.Logger
}

func NewAuthService(logger *slog.Logger) *Service {
	return &Service{
		repository: nil,
		log:        logger.With(sl.Module("auth-service")),
	}
}

func (s *Service) SetRepository(repository Repository) {
	s.repository = repository
}

// ensureUUID backfills a UUID on legacy documents stored without one.
func (s *Service) ensureUUID(user *entity.User) error {
	if user == nil || user.UUID != "" {
		return nil
	}
	if user.ID.IsZero() {
		return fmt.Errorf("user has neither uuid nor _id")
	}
	id := uuid.NewString()
	if err := s.repository.SetUserUUID(user.ID, id); err != nil {
		s.log.Error("assigning user uuid", sl.Err(err))
		return err
	}
	user.UUID = id
	return nil
}

// RegisterUser returns the existing user matching any of the identifiers, linking the
// Telegram ID when it differs, or creates a new guest user.
func (s *Service) RegisterUser(name, email, phone string, telegramId int64) (*entity.User, error) {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil {
		return nil, err
	}

	if user == nil {
		user = entity.NewUser(email, phone, telegramId)
		user.Name = name
		if err = s.repository.CreateUser(user); err != nil {
			// A concurrent registration won the race; the unique index rejected ours.
			if mongo.IsDuplicateKeyError(err) {
				if existing, getErr := s.GetUser(email, phone, telegramId); getErr == nil && existing != nil {
					return existing, nil
				}
			}
			return nil, err
		}
		return user, nil
	}

	if telegramId != 0 && user.TelegramId != telegramId {
		if err = s.UpdateUserFields(user, map[string]any{entity.UserFieldTelegramId: telegramId}); err != nil {
			return nil, err
		}
	}

	return user, nil
}

// UpdateUserFields persists only the given fields (keys are entity.UserField* constants)
// and mirrors them onto the in-memory user so callers keep a consistent copy.
// Phone values are normalized before saving.
func (s *Service) UpdateUserFields(user *entity.User, fields map[string]any) error {
	if user == nil {
		return fmt.Errorf("user is nil")
	}
	if len(fields) == 0 {
		return nil
	}
	if err := s.ensureUUID(user); err != nil {
		return err
	}
	if p, ok := fields[entity.UserFieldPhone].(string); ok {
		fields[entity.UserFieldPhone] = entity.NormalizePhone(p)
	}
	if err := s.repository.UpdateUserFields(user.UUID, fields); err != nil {
		return err
	}
	applyFields(user, fields)
	return nil
}

// applyFields copies updated values onto the struct; unknown keys are ignored.
func applyFields(u *entity.User, fields map[string]any) {
	for k, v := range fields {
		switch k {
		case entity.UserFieldName:
			u.Name, _ = v.(string)
		case entity.UserFieldEmail:
			u.Email, _ = v.(string)
		case entity.UserFieldPhone:
			u.Phone, _ = v.(string)
		case entity.UserFieldAddress:
			u.Address, _ = v.(string)
		case entity.UserFieldTelegramId:
			u.TelegramId, _ = v.(int64)
		case entity.UserFieldTelegramUsername:
			u.TelegramUsername, _ = v.(string)
		case entity.UserFieldInstagramId:
			u.InstagramId, _ = v.(string)
		case entity.UserFieldInstagramUser:
			u.InstagramUsername, _ = v.(string)
		case entity.UserFieldSmartSenderId:
			u.SmartSenderId, _ = v.(string)
		case entity.UserFieldZohoId:
			u.ZohoId, _ = v.(string)
		case entity.UserFieldRole:
			u.Role, _ = v.(string)
		case entity.UserFieldBlocked:
			u.Blocked, _ = v.(bool)
		case entity.UserFieldPromoExpire:
			u.PromoExpire, _ = v.(time.Time)
		case entity.UserFieldConversation:
			u.Conversation, _ = v.([]entity.DialogMessage)
		}
	}
}

// GetUser looks a user up by any of the identifiers. Returns (nil, nil) when not found;
// read paths must never register users (see GetOrCreateUser).
func (s *Service) GetUser(email, phone string, telegramId int64) (*entity.User, error) {
	phone = entity.NormalizePhone(phone)
	if email == "" && phone == "" && telegramId == 0 {
		return nil, nil
	}
	user, err := s.repository.GetUser(email, phone, telegramId)
	if err != nil {
		return nil, err
	}
	if err = s.ensureUUID(user); err != nil {
		return nil, err
	}
	return user, nil
}

// GetOrCreateUser is GetUser that registers a guest when nothing matches. Only for entry
// points that legitimately introduce new customers (AI requests from website/SmartSender).
func (s *Service) GetOrCreateUser(email, phone string, telegramId int64) (*entity.User, error) {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil || user != nil {
		return user, err
	}
	if email == "" && entity.NormalizePhone(phone) == "" && telegramId == 0 {
		return nil, fmt.Errorf("no user identifier provided")
	}
	return s.RegisterUser("", email, phone, telegramId)
}

func (s *Service) GetUserByUUID(uuid string) (*entity.User, error) {
	user, err := s.repository.GetUserByUUID(uuid)
	if err != nil {
		return nil, err
	}
	if user == nil {
		return nil, fmt.Errorf("user not found")
	}
	return user, nil
}

func (s *Service) GetUserByInstagramId(instagramId string) (*entity.User, error) {
	if instagramId == "" {
		return nil, nil
	}
	user, err := s.repository.GetUserByInstagramId(instagramId)
	if err != nil {
		return nil, err
	}
	if err = s.ensureUUID(user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *Service) GetUserBySmartSenderId(smartSenderId string) (*entity.User, error) {
	if smartSenderId == "" {
		return nil, nil
	}
	user, err := s.repository.GetUserBySmartSenderId(smartSenderId)
	if err != nil {
		return nil, err
	}
	if err = s.ensureUUID(user); err != nil {
		return nil, err
	}
	return user, nil
}

// UserExists is kept for existing callers; it is identical to GetUser.
func (s *Service) UserExists(email, phone string, telegramId int64) (*entity.User, error) {
	return s.GetUser(email, phone, telegramId)
}

func (s *Service) IsUserGuest(email, phone string, telegramId int64) bool {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil || user == nil {
		return true
	}
	return user.IsGuest()
}

func (s *Service) IsUserAdmin(email, phone string, telegramId int64) bool {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil || user == nil {
		return false
	}
	return user.IsAdmin()
}

func (s *Service) IsUserManager(email, phone string, telegramId int64) bool {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil || user == nil {
		return false
	}
	return user.IsManager()
}

// BlockUser sets the blocked flag. The role changes only when one is supplied, so
// blocking without a role no longer wipes the user's role.
func (s *Service) BlockUser(email, phone string, telegramId int64, block bool, role string) error {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	fields := map[string]any{entity.UserFieldBlocked: block}
	if role != "" {
		// Reject unknown roles: GetAssistants and role checks treat them as no role.
		role = strings.ToLower(strings.TrimSpace(role))
		switch role {
		case entity.GuestRole, entity.UserRole, entity.ManagerRole, entity.AdminRole:
		default:
			return fmt.Errorf("unknown role %q", role)
		}
		fields[entity.UserFieldRole] = role
	}
	return s.UpdateUserFields(user, fields)
}

func (s *Service) SetSmartSenderId(email, phone string, telegramId int64, smartSenderId string) error {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}
	return s.UpdateUserFields(user, map[string]any{entity.UserFieldSmartSenderId: smartSenderId})
}

const (
	// maxConversationMessages is how many Q/A pairs are kept as AI context.
	maxConversationMessages = 20
	// maxDialogFieldBytes caps each question/answer so 20 pairs stay under ~280 KB,
	// the budget the previous whole-history trimming enforced.
	maxDialogFieldBytes = 7000
)

// UpdateConversation appends a Q/A pair. Only the conversation field is written, so the
// user snapshot held during a long AI turn cannot revert changes made in the meantime.
func (s *Service) UpdateConversation(user entity.User, message entity.DialogMessage) error {
	if err := s.ensureUUID(&user); err != nil {
		return err
	}
	message.Question = truncateUTF8(message.Question, maxDialogFieldBytes)
	message.Answer = truncateUTF8(message.Answer, maxDialogFieldBytes)
	return s.repository.PushConversation(user.UUID, message, maxConversationMessages)
}

func (s *Service) ClearConversation(user *entity.User) error {
	return s.UpdateUserFields(user, map[string]any{entity.UserFieldConversation: []entity.DialogMessage{}})
}

// truncateUTF8 cuts s to at most n bytes without splitting a multi-byte rune.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}
