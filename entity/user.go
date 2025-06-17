package entity

import (
	"fmt"
	"github.com/google/uuid"
)

type User struct {
	UID        string `json:"uid" bson:"uid" validate:"omitempty"`
	Name       string `json:"name" bson:"name" validate:"omitempty"`
	Email      string `json:"email" bson:"email" validate:"omitempty,email"`
	Password   string `json:"password" bson:"password" validate:"required,min=8"`
	Role       string `json:"role" bson:"role" validate:"omitempty"`
	TelegramId int64  `json:"telegram_id" bson:"telegram_id" validate:"omitempty"`
}

const (
	GuestRole   = "guest"
	UserRole    = "user"
	ManagerRole = "manager"
	AdminRole   = "admin"
)

func NewUser(chatId int64) *User {
	uid := uuid.New()
	return &User{
		UID:        uid.String(),
		TelegramId: chatId,
	}
}

func (u *User) IsGuest() bool {
	return u.Role == GuestRole
}

func (u *User) IsAdmin() bool {
	return u.Role == AdminRole
}
