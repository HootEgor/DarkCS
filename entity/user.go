package entity

import (
	"fmt"
	"github.com/google/uuid"
	"strings"
	"time"
)

type User struct {
	UUID              string          `json:"uuid" bson:"uuid"`
	Name              string          `json:"name" bson:"name" validate:"omitempty"`
	Email             string          `json:"email" bson:"email" validate:"omitempty,email"`
	Phone             string          `json:"phone" bson:"phone" validate:"omitempty"`
	Address           string          `json:"address" bson:"address" validate:"omitempty"`
	TelegramId        int64           `json:"telegram_id" bson:"telegram_id" validate:"omitempty"`
	TelegramUsername  string          `json:"telegram_username" bson:"telegram_username" validate:"omitempty"`
	InstagramId       string          `json:"instagram_id" bson:"instagram_id" validate:"omitempty"`
	InstagramUsername string          `json:"instagram_username" bson:"instagram_username" validate:"omitempty"`
	SmartSenderId     string          `json:"smart_sender_id" bson:"smart_sender_id" validate:"omitempty"`
	ZohoId            string          `json:"zoho_id" bson:"zoho_id" validate:"omitempty"`
	Role              string          `json:"role" bson:"role" validate:"omitempty"`
	Blocked           bool            `json:"blocked" bson:"blocked" validate:"omitempty"`
	LastSeen          time.Time       `json:"last_seen" bson:"lastSeen"`
	PromoExpire       time.Time       `json:"promo_expire" bson:"promoExpire" validate:"omitempty"`
	Conversation      []DialogMessage `json:"conversation" bson:"conversation" validate:"omitempty"`
}

type DialogMessage struct {
	Question string    `json:"question" bson:"question"`
	Answer   string    `json:"answer" bson:"answer"`
	Time     time.Time `json:"time" bson:"time"`
}

type UserInfo struct {
	Name     string `json:"name" bson:"name"`
	Email    string `json:"email" bson:"email"`
	Phone    string `json:"phone" bson:"phone"`
	Address  string `json:"address" bson:"address"`
	Discount int    `json:"discount" bson:"discount"`
}

type OneCUser struct {
	Discount int `json:"discount" bson:"discount"`
}

const (
	GuestRole   = "guest"
	UserRole    = "user"
	ManagerRole = "manager"
	AdminRole   = "admin"
)

type TelegramImportItem struct {
	SmartSenderID    int64  `json:"smartsender_id"`
	TelegramID       int64  `json:"telegram_id"`
	Phone            string `json:"phone"`
	TelegramUsername string `json:"telegram_username"`
}

func NewUser(email, phone string, telegramId int64) *User {

	phoneDigits := ""
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			phoneDigits += string(ch)
		}
	}
	phone = fmt.Sprintf("+%s", phoneDigits)

	return &User{
		UUID:       uuid.NewString(),
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

func (u *User) IsManager() bool {
	return u.Role == ManagerRole || u.Role == AdminRole
}

func (u *User) SameUser(other *User) bool {
	if other == nil {
		return false
	}

	if u.TelegramId != 0 && other.TelegramId != 0 {
		return u.TelegramId == other.TelegramId
	}

	if u.InstagramId != "" && other.InstagramId != "" {
		return u.InstagramId == other.InstagramId
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
		return []string{OverseerAss, ConsultantAss, OrderManagerAss}
	case UserRole:
		return []string{OverseerAss, ConsultantAss, OrderManagerAss}
	case ManagerRole:
		return []string{OverseerAss, ConsultantAss, OrderManagerAss}
	case AdminRole:
		return []string{OverseerAss, ConsultantAss, OrderManagerAss}
	}

	return []string{OverseerAss, ConsultantAss, OrderManagerAss}
}

func (u *User) GetInfo() *UserInfo {
	return &UserInfo{
		Name:    u.Name,
		Email:   u.Email,
		Phone:   u.Phone,
		Address: u.Address,
	}
}

func (u *UserInfo) ToContact() *Contact {
	firstName := u.Name
	lastName := "-"
	if parts := strings.SplitN(u.Name, " ", 2); len(parts) >= 2 {
		firstName = parts[0]
		lastName = parts[1]

		if lastName == "" {
			lastName = "-"
		}
	}
	return &Contact{
		FirstName: firstName,
		LastName:  lastName,
		Email:     u.Email,
		Field2:    u.Address,
		Phone:     u.Phone,
	}
}

func (u *User) ToContact() *Contact {
	firstName := u.Name
	lastName := "-"
	if parts := strings.SplitN(u.Name, " ", 2); len(parts) >= 2 {
		firstName = parts[0]
		lastName = parts[1]

		if lastName == "" {
			lastName = "-"
		}
	}
	return &Contact{
		FirstName: firstName,
		LastName:  lastName,
		Email:     u.Email,
		Field2:    u.Address,
		Phone:     u.Phone,
	}
}

func (u *User) HasPromo() bool {
	if u.PromoExpire.IsZero() {
		return false
	}
	return time.Now().Before(u.PromoExpire)
}
