package entity

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

type User struct {
	Name       string    `json:"name" bson:"name" validate:"omitempty"`
	Email      string    `json:"email" bson:"email" validate:"omitempty,email"`
	Phone      string    `json:"phone" bson:"phone" validate:"omitempty"`
	TelegramId int64     `json:"telegram_id" bson:"telegram_id" validate:"omitempty"`
	Role       string    `json:"role" bson:"role" validate:"omitempty"`
	Blocked    bool      `json:"blocked" bson:"blocked" validate:"omitempty"`
	LastSeen   time.Time `json:"last_seen" bson:"lastSeen"`
}

const (
	GuestRole   = "guest"
	UserRole    = "user"
	ManagerRole = "manager"
	AdminRole   = "admin"
)

const (
	OverseerAss   = "Overseer"
	ConsultantAss = "Consultant"
	CalculatorAss = "Calculator"
)

func NewUser(email, phone string, telegramId int64) *User {
	return &User{
		Email:      email,
		Phone:      phone,
		TelegramId: telegramId,
		Role:       GuestRole,
		Blocked:    false,
		LastSeen:   time.Now(),
	}
}

func (u *User) GetId() string {
	return fmt.Sprintf("%s:%s:%d", u.Email, u.Phone, u.TelegramId)
}

func GetUserDataFromId(id string) (string, string, int64, error) {
	var email, phone string
	var telegramId int64

	// Split the string by colons
	parts := strings.Split(id, ":")
	if len(parts) != 3 {
		return "", "", 0, fmt.Errorf("invalid user id format: expected 3 parts separated by colons")
	}

	email = parts[0]
	phone = parts[1]

	// Parse the telegram ID
	if parts[2] == "" {
		return "", "", 0, fmt.Errorf("telegram ID cannot be empty")
	}

	var err error
	telegramId, err = strconv.ParseInt(parts[2], 10, 64)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to parse telegram ID: %w", err)
	}

	return email, phone, telegramId, nil
}

func (u *User) IsGuest() bool {
	return u.Role == GuestRole
}

func (u *User) IsAdmin() bool {
	return u.Role == AdminRole
}

func (u *User) SameUser(other *User) bool {
	if other == nil {
		return false
	}

	if u.TelegramId != 0 && other.TelegramId != 0 {
		return u.TelegramId == other.TelegramId
	}

	if u.Email != "" && other.Email != "" {
		return u.Email == other.Email
	}

	if u.Phone != "" && other.Phone != "" {
		return u.Phone == other.Phone
	}

	return false
}

func (u *User) GetAssistants() []string {

	switch u.Role {
	case GuestRole:
		return []string{OverseerAss, ConsultantAss}
	case UserRole:
		return []string{OverseerAss, ConsultantAss}
	case ManagerRole:
		return []string{OverseerAss, ConsultantAss}
	case AdminRole:
		return []string{OverseerAss, ConsultantAss}
	}

	return []string{OverseerAss, ConsultantAss}
}
