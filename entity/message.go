package entity

import (
	"time"
)

// Message User represents a user in the system.
type Message struct {
	User     *User     `json:"user" bson:"user"`
	Question string    `json:"question" bson:"question"`
	Answer   AiAnswer  `json:"answer" bson:"answer"`
	Time     time.Time `json:"time" bson:"time"`
}
