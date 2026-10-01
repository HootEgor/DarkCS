package services

import (
	"DarkCS/entity"
)

func (s *ZohoService) CreateContact(user *entity.User) (string, error) {
	if s.tokenExpiring() {
		err := s.refreshTokenCall()
		if err != nil {
			return "", err
		}
	}

	contact := user.ToContact()

	contactID, err := s.createContact(*contact)
	if err != nil {
		return "", err
	}

	return contactID, nil
}
