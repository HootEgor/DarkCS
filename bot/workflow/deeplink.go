package workflow

import (
	"strings"
)

// Deep link type constants
const (
	DeepLinkTypeSchool = "school"
)

// ParseDeepLink parses a deep link code from a /start command.
// Format: "type_code" (e.g., "school_abc123")
// The code parameter is up to 64 base64url characters from t.me/botname?start=CODE
func ParseDeepLink(startParam string) *DeepLinkData {
	if startParam == "" {
		return nil
	}

	// Trim any whitespace
	startParam = strings.TrimSpace(startParam)

	// Split on first underscore
	parts := strings.SplitN(startParam, "_", 2)
	if len(parts) < 2 {
		// No underscore, treat entire param as type with empty code
		return &DeepLinkData{
			Type: startParam,
			Code: "",
		}
	}

	return &DeepLinkData{
		Type: parts[0],
		Code: parts[1],
	}
}

// ExtractStartParam extracts the parameter from a /start command message.
// Returns empty string if no parameter present.
func ExtractStartParam(messageText string) string {
	// Message format: "/start CODE" or just "/start"
	messageText = strings.TrimSpace(messageText)

	if !strings.HasPrefix(messageText, "/start") {
		return ""
	}

	// Remove "/start" prefix
	rest := strings.TrimPrefix(messageText, "/start")
	rest = strings.TrimSpace(rest)

	return rest
}

// IsSchoolDeepLink checks if the deep link is for a school.
func (d *DeepLinkData) IsSchoolDeepLink() bool {
	return d != nil && d.Type == DeepLinkTypeSchool
}

// HasCode checks if the deep link has a code.
func (d *DeepLinkData) HasCode() bool {
	return d != nil && d.Code != ""
}

// IsEmpty checks if the deep link data is empty or nil.
func (d *DeepLinkData) IsEmpty() bool {
	return d == nil || (d.Type == "" && d.Code == "")
}

// FullCode returns the full deep link code (type_code).
func (d *DeepLinkData) FullCode() string {
	if d == nil {
		return ""
	}
	if d.Code == "" {
		return d.Type
	}
	return d.Type + "_" + d.Code
}
