package services

import (
	"DarkCS/entity"
)

func (s *ZohoService) CreateContact(user *entity.User) (string, error) {

	contact := user.ToContact()

	contactID, err := s.createContact(*contact)
	if err != nil {
		return "", err
	}

	return contactID, nil
}
