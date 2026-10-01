package chat

import (
	"DarkCS/entity"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// NormalizePhone strips non-digit characters and prepends "+".
// Delegates to entity.NormalizePhone so bots and the user store agree on one format.
func NormalizePhone(phone string) string {
	return entity.NormalizePhone(phone)
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

// FormatNumberedInlineGrid creates a numbered text list from a multi-row inline grid.
// Rows are flattened into a single numbered list for text-only platforms.
func FormatNumberedInlineGrid(text string, rows [][]InlineButton) string {
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

// MatchNumberToInlineGrid converts a number string to the corresponding inline button data
// from a multi-row grid.
func MatchNumberToInlineGrid(text string, rows [][]InlineButton) string {
	text = strings.TrimSpace(text)
	num, err := strconv.Atoi(text)
	if err != nil || num < 1 {
		return ""
	}

	idx := 1
	for _, row := range rows {
		for _, btn := range row {
			if idx == num {
				return btn.Data
			}
			idx++
		}
	}
	return ""
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

// Per-platform message length limits in characters (runes). Longer messages are
// rejected by the platform APIs, so the user would get nothing.
const (
	TelegramTextLimit  = 4096
	InstagramTextLimit = 1000
	WhatsAppTextLimit  = 4096
)

// SplitText splits text into chunks of at most limit runes, preferring to break at a
// newline, then at a space, and only mid-word when a single line is longer than limit.
func SplitText(text string, limit int) []string {
	runes := []rune(text)
	if limit <= 0 || len(runes) <= limit {
		return []string{text}
	}

	var chunks []string
	for len(runes) > limit {
		cut := lastIndexRune(runes[:limit], '\n')
		if cut <= 0 {
			cut = lastIndexRune(runes[:limit], ' ')
		}
		if cut <= 0 {
			cut = limit
		}
		chunk := strings.TrimRight(string(runes[:cut]), " \n")
		if chunk != "" {
			chunks = append(chunks, chunk)
		}
		runes = []rune(strings.TrimLeft(string(runes[cut:]), " \n"))
	}
	if len(runes) > 0 {
		chunks = append(chunks, string(runes))
	}
	return chunks
}

func lastIndexRune(runes []rune, r rune) int {
	for i := len(runes) - 1; i >= 0; i-- {
		if runes[i] == r {
			return i
		}
	}
	return -1
}
