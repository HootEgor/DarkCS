package chat

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// normalizePhone strips non-digit characters and prepends "+".
func NormalizePhone(phone string) string {
	digits := ""
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			digits += string(ch)
		}
	}
	if len(digits) > 0 {
		digits = "+" + digits
	}
	return digits
}

// IsValidPhone checks if the input looks like a valid phone number (10-15 digits).
func IsValidPhone(phone string) bool {
	digits := ""
	for _, ch := range phone {
		if ch >= '0' && ch <= '9' {
			digits += string(ch)
		}
	}
	if len(digits) < 10 {
		return false
	}
	pattern := regexp.MustCompile(`^\+?[0-9]{10,15}$`)
	return pattern.MatchString("+" + digits)
}

// MatchNumberToOption converts a number string ("1", "2", ...) to the
// corresponding menu button text. Returns empty string if no match.
func MatchNumberToOption(text string, buttons [][]MenuButton) string {
	text = strings.TrimSpace(text)
	num, err := strconv.Atoi(text)
	if err != nil || num < 1 {
		return ""
	}

	idx := 1
	for _, row := range buttons {
		for _, btn := range row {
			if idx == num {
				return btn.Text
			}
			idx++
		}
	}
	return ""
}

// FormatNumberedMenu creates a numbered text menu from button rows.
// Example output: "1. Option A\n2. Option B\n\nОберіть опцію:"
func FormatNumberedMenu(text string, rows [][]MenuButton) string {
	var sb strings.Builder
	sb.WriteString(text)
	sb.WriteString("\n\n")

	idx := 1
	for _, row := range rows {
		for _, btn := range row {
			sb.WriteString(fmt.Sprintf("%d. %s\n", idx, btn.Text))
			idx++
		}
	}
	sb.WriteString("\nОберіть опцію:")
	return sb.String()
}

// FormatNumberedInline creates a numbered text list from inline buttons.
func FormatNumberedInline(text string, buttons []InlineButton) string {
	var sb strings.Builder
	sb.WriteString(text)
	sb.WriteString("\n\n")

	for i, btn := range buttons {
		sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, btn.Text))
	}
	sb.WriteString("\nОберіть опцію:")
	return sb.String()
}

// MatchNumberToInline converts a number string to the corresponding inline button data.
func MatchNumberToInline(text string, buttons []InlineButton) string {
	text = strings.TrimSpace(text)
	num, err := strconv.Atoi(text)
	if err != nil || num < 1 || num > len(buttons) {
		return ""
	}
	return buttons[num-1].Data
}
