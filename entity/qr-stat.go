package entity

import (
	"time"
	"unicode"
)

// QrStat records a single QR-code funnel event.
// SmartSenderId is kept for backward compatibility with legacy records;
// new records use Platform + UserID as the primary key since SmartSender is deprecated.
type QrStat struct {
	SmartSenderId string    `json:"smart_sender_id" bson:"smart_sender_id"`
	Platform      string    `json:"platform"        bson:"platform"`
	UserID        string    `json:"user_id"         bson:"user_id"`
	SchoolName    string    `json:"school_name"     bson:"school_name"`
	Date          time.Time `json:"date"            bson:"date"`
	FollowQr      bool      `json:"follow_qr"       bson:"follow_qr"`
	Registered    bool      `json:"registered"      bson:"registered"`
}

// GetMonthName returns the Ukrainian month name for the given date, capitalized.
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
