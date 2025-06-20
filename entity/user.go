package entity

import "time"

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
