package auth

import (
	"DarkCS/entity"
	"DarkCS/internal/lib/sl"
	"fmt"
	"log/slog"
)

type Repository interface {
	UpsertUser(user entity.User) error
	GetUser(userId int64) (*entity.User, error)
	SetAssistantForUser(id int64, assistantId string) error
}

type Service struct {
	repository Repository
	users      []entity.User
	devPrefix  string
	log        *slog.Logger
}

func NewAuthService(logger *slog.Logger, devPrefix string) *Service {
	return &Service{
		repository: nil,
		users:      make([]entity.User, 0),
		devPrefix:  devPrefix,
		log:        logger.With(sl.Module("auth-service")),
	}
}

func (s *Service) SetRepository(repository Repository) {
	s.repository = repository
}

func (s *Service) updateUser(user entity.User) {
	for i, u := range s.users {
		if u.UID == user.UID {
			s.users[i] = user
		}
	}
}

func (s *Service) RegisterUser(chatId int64) error {
	user, _ := s.GetUser(chatId)

	if user == nil {
		user = entity.NewUser(chatId)
		err := s.repository.UpsertUser(*user)
		if err != nil {
			return err
		}
		s.users = append(s.users, *user)
	}

	return nil
}

func (s *Service) UpdateUser(chatId int64) error {
	user, err := s.repository.GetUser(chatId)
	if err != nil {
		return err
	}

	s.updateUser(*user)

	return nil
}

func (s *Service) GetUser(chatId int64) (*entity.User, error) {
	for _, user := range s.users {
		if user.TelegramId == chatId {
			return &user, nil
		}
	}
	user, err := s.repository.GetUser(chatId)
	if err != nil {
		return nil, err
	}
	if user != nil {
		s.users = append(s.users, *user)
		return user, nil
	}
	return nil, fmt.Errorf("user %d not found", chatId)
}

func (s *Service) IsUserGuest(chatId int64) bool {
	user, err := s.GetUser(chatId)
	if err != nil {
		s.log.Error("getting user", sl.Err(err))
		return true
	}

	return user.IsGuest()
}

func (s *Service) IsUserAdmin(chatId int64) bool {
	user, err := s.GetUser(chatId)
	if err != nil {
		s.log.Error("getting user", sl.Err(err))
		return false
	}

	return user.IsAdmin()
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
