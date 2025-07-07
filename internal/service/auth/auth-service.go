package auth

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"fmt"
	"github.com/google/uuid"
	"log/slog"
)

type Repository interface {
	UpsertUser(user entity.User) error
	GetUser(email, phone string, telegramId int64) (*entity.User, error)

	UpsertBasket(basket *entity.Basket) (*entity.Basket, error)
	GetBasket(userUUID string) (*entity.Basket, error)
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

func (s *Service) RegisterUser(email, phone string, telegramId int64) (*entity.User, error) {
	user, _ := s.repository.GetUser(email, phone, telegramId)

	if user == nil {
		user = entity.NewUser(email, phone, telegramId)
		err := s.repository.UpsertUser(*user)
		if err != nil {
			return nil, err
		}
		s.users = append(s.users, *user)
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

	user, err = s.RegisterUser(email, phone, telegramId)

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

func (s *Service) BlockUser(email, phone string, telegramId int64, block bool) error {
	user, err := s.GetUser(email, phone, telegramId)
	if err != nil {
		return err
	}

	user.Blocked = block

	err = s.repository.UpsertUser(*user)
	if err != nil {
		return err
	}

	s.updateUser(*user)

	return nil
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
