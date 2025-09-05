package entity

import (
	"time"
	"unicode"
)

type QrStat struct {
	SmartSenderId string    `json:"smart_sender_id" bson:"smart_sender_id"`
	Date          time.Time `json:"date" bson:"date"`
	FollowQr      bool      `json:"follow_qr" bson:"follow_qr"`
	Registered    bool      `json:"registered" bson:"registered"`
}

func GetMonthName(date time.Time) string {
	months := []string{
		"січень",   // January
		"лютий",    // February
		"березень", // March
		"квітень",  // April
		"травень",  // May
		"червень",  // June
		"липень",   // July
		"серпень",  // August
		"вересень", // September
		"жовтень",  // October
		"листопад", // November
		"грудень",  // December
	}

	month := months[date.Month()-1]
	// Capitalize first letter (works with Cyrillic too)
	runes := []rune(month)
	runes[0] = unicode.ToUpper(runes[0])

	return string(runes)
}
