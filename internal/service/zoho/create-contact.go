package services

import (
	"DarkCS/entity"
	"time"
)

func (s *ZohoService) CreateContact(user *entity.User) (string, error) {
	if time.Now().After(s.tokenExpiresIn.Add(time.Minute * time.Duration(-5))) {
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
