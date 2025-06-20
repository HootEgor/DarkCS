package entity

import (
	"time"
)

type Message struct {
	User     *User     `json:"user" bson:"user"`
	Question string    `json:"question" bson:"question"`
	Answer   AiAnswer  `json:"answer" bson:"answer"`
	Time     time.Time `json:"time" bson:"time"`
}
