package sl

import (
	"fmt"
	"log/slog"
	"strings"
)

func Err(err error) slog.Attr {
	return slog.Attr{
		Key:   "error",
		Value: slog.StringValue(err.Error()),
	}
}

// Secret returns a string with the first 5 characters of the input string
// used to hide sensitive information in logs
func Secret(key, value string) slog.Attr {
	r := "***"
	if len(value) > 5 {
		r = fmt.Sprintf("%s***", value[0:5])
	}
	if value == "" {
		r = "?"
	}
	return slog.Attr{
		Key:   key,
		Value: slog.StringValue(r),
	}
}

func Module(mod string) slog.Attr {
	return slog.Attr{
		Key:   "mod",
		Value: slog.StringValue(mod),
	}
}

// Phone logs a phone number with the middle digits masked (+380*****4567): enough to
// tell customers apart in logs, which are also forwarded to the admin Telegram chat.
func Phone(key, phone string) slog.Attr {
	r := []rune(phone)
	if len(r) <= 7 {
		return slog.String(key, "***")
	}
	masked := string(r[:4]) + strings.Repeat("*", len(r)-8) + string(r[len(r)-4:])
	return slog.String(key, masked)
}
