package entity

import (
	"DarkCS/internal/lib/validate"
	"net/http"
)

// MailMessage represents a message to be sent via email.
type MailMessage struct {
	Sender  *UserAuth `json:"sender,omitempty" bson:"sender"`
	To      string    `json:"to" validate:"required,email"`
	Message string    `json:"message" validate:"omitempty"`
}

func (m *MailMessage) Bind(_ *http.Request) error {
	return validate.Struct(m)
}
