package auth

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
)

type Repository interface {
	UpsertUser(user entity.User) error
	GetUser(email, phone string, telegramId int64) (*entity.User, error)
	GetUserByUUID(uuid string) (*entity.User, error)

	UpsertBasket(basket *entity.Basket) (*entity.Basket, error)
	GetBasket(userUUID string) (*entity.Basket, error)

	GetPromoCode(code string) (*entity.PromoCode, error)
	ActivatePromoCode(code string) error
	SavePromoCodes(codes []string) error
	GetAllPromoCodes() ([]entity.PromoCode, error)
}

type Service struct {
	repository Repository
	users      []entity.User
	log        *slog.Logger
}

func NewAuthService(logger *slog.Logger) *Service {
	return &Service{
		repository: nil,
		users:      make([]entity.User, 0),
		log:        logger.With(sl.Module("auth-service")),
	}
}

func (s *Service) SetRepository(repository Repository) {
	s.repository = repository
}

func (s *Service) updateUser(user entity.User) {
	for i, u := range s.users {
		if user.SameUser(&u) {
			s.users[i] = user
		}
	}
}

func (s *Service) RegisterUser(name, email, phone string, telegramId int64) (*entity.User, error) {
	user, _ := s.repository.GetUser(email, phone, telegramId)

	if user == nil {
		user = entity.NewUser(email, phone, telegramId)
		user.Name = name
		err := s.repository.UpsertUser(*user)
		if err != nil {
			return nil, err
		}
		s.users = append(s.users, *user)
	} else {
		// Update telegram ID if provided and missing/different
		if telegramId != 0 && user.TelegramId != telegramId {
			user.TelegramId = telegramId
			err := s.repository.UpsertUser(*user)
			if err != nil {
				return nil, err
			}
			s.updateUser(*user)
		}
	}

	return user, nil
}

func (s *Service) UpdateUser(user *entity.User) error {
	if user == nil {
		return fmt.Errorf("user is nil")
	}

	err := s.repository.UpsertUser(*user)
	if err != nil {
		return err
	}

	s.updateUser(*user)

	return nil
}

func (s *Service) GetUser(email, phone string, telegramId int64) (*entity.User, error) {
	filterUser := entity.NewUser(email, phone, telegramId)
	for _, user := range s.users {
		if user.SameUser(filterUser) {
			return &user, nil
		}
	}
	user, err := s.repository.GetUser(email, phone, telegramId)
	if err != nil {
		return nil, err
	}
	if user != nil {
		if user.UUID == "" {
			user.UUID = uuid.NewString()
			err = s.repository.UpsertUser(*user)
			if err != nil {
				s.log.Error("upserting user", sl.Err(err))
				return nil, err
			}
		}
		s.users = append(s.users, *user)
		return user, nil
	}

	user, err = s.RegisterUser("", email, phone, telegramId)

	return user, err
}

func (s *Service) GetUserByUUID(uuid string) (*entity.User, error) {
	for _, user := range s.users {
		if user.UUID == uuid {
			return &user, nil
		}
	}
	user, err := s.repository.GetUserByUUID(uuid)
	if err != nil {
		return nil, err
	}
	if user != nil {
		s.users = append(s.users, *user)
		return user, nil
	}
	return nil, fmt.Errorf("user not found")
}

func (s *Service) UserExists(email, phone string, telegramId int64) (*entity.User, error) {
	filterUser := entity.NewUser(email, phone, telegramId)
	for _, user := range s.users {
		if user.SameUser(filterUser) {
			return &user, nil
		}
	}
	user, err := s.repository.GetUser(email, phone, telegramId)
	if err != nil {
		return nil, err
	}
	if user != nil {
		if user.UUID == "" {
			user.UUID = uuid.NewString()
			err = s.repository.UpsertUser(*user)
			if err != nil {
				s.log.Error("upserting user", sl.Err(err))
				return nil, err
			}
		}
		s.users = append(s.users, *user)
		return user, nil
	}

	return user, err
}

func (s *Service) IsUserGuest(email, phone string, telegramId int64) bool {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil {
		s.log.Error("getting user", sl.Err(err))
		return true
	}

	return user.IsGuest()
}

func (s *Service) IsUserAdmin(email, phone string, telegramId int64) bool {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil {
		s.log.Error("getting user", sl.Err(err))
		return false
	}

	return user.IsAdmin()
}

func (s *Service) IsUserManager(email, phone string, telegramId int64) bool {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil {
		s.log.Error("getting user", sl.Err(err))
		return false
	}

	return user.IsManager()
}

func (s *Service) BlockUser(email, phone string, telegramId int64, block bool, role string) error {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil {
		return err
	}

	user.Blocked = block
	user.Role = role

	err = s.repository.UpsertUser(*user)
	if err != nil {
		return err
	}

	s.updateUser(*user)

	return nil
}

func (s *Service) SetSmartSenderId(email, phone string, telegramId int64, smartSenderId string) error {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil {
		return err
	}

	user.SmartSenderId = smartSenderId

	err = s.repository.UpsertUser(*user)
	if err != nil {
		return err
	}

	s.updateUser(*user)

	return nil
}

func (s *Service) UpdateConversation(user entity.User, message entity.DialogMessage) error {
	const contextLimit = 400000
	const safeMargin = int(float64(contextLimit) * 0.7)
	const maxMessages = 20

	// Append the new message
	user.Conversation = append(user.Conversation, message)

	// Trim oldest messages if exceeding safe margin
	data, err := json.Marshal(user.Conversation)
	if err != nil {
		return fmt.Errorf("failed to marshal conversation: %w", err)
	}
	for len(data) > safeMargin && len(user.Conversation) > 1 {
		user.Conversation = user.Conversation[1:]
		data, _ = json.Marshal(user.Conversation)
	}

	// Ensure max message count
	if len(user.Conversation) > maxMessages {
		user.Conversation = user.Conversation[len(user.Conversation)-maxMessages:]
	}

	return s.UpdateUser(&user)
}

func (s *Service) ClearConversation(user *entity.User) error {
	if user == nil {
		return fmt.Errorf("user is nil")
	}

	user.Conversation = make([]entity.DialogMessage, 0)

	return s.UpdateUser(user)
}

//func (a *Service) getAssistantsBySection(section string) []entity.AssistantData {
//
//	var assistants []entity.AssistantData
//	for _, ass := range a.assistants {
//		if strings.HasPrefix(ass.Name, a.devPrefix) {
//			continue
//		}
//		if ass.Section == section {
//			assistants = append(assistants, ass)
//		}
//	}
//
//	return assistants
//}

//func (a *Service) GetAssistantsForUser(chatId int64) ([]entity.AssistantData, error) {
//
//	if a.IsUserAdmin(chatId) {
//		return a.assistants, nil
//	}
//
//	user, err := a.GetUser(chatId)
//	if err != nil {
//		return nil, err
//	}
//
//	assistants := make([]entity.AssistantData, 0)
//	for _, section := range user.Sections {
//		sectionAss := a.getAssistantsBySection(section)
//		assistants = append(assistants, sectionAss...)
//	}
//
//	return assistants, nil
//}
