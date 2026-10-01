package sl

import (
	"errors"
	"net/url"
	"regexp"
)

// telegramBotToken matches the bot token embedded in Telegram file/API paths.
var telegramBotToken = regexp.MustCompile(`/bot[^/]+`)

// RedactURLError hides credentials in the URL of a *url.Error. net/http includes the
// full request URL in transport errors, so a token sent as a query parameter
// (access_token=, zapikey=) or in a Telegram path (/file/bot<token>/) would otherwise end
// up in log files and in the admin Telegram chat. Other errors are returned unchanged.
// Call it on the error straight from http.Client before wrapping it.
func RedactURLError(err error) error {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return err
	}
	return &url.Error{Op: ue.Op, URL: RedactURL(ue.URL), Err: ue.Err}
}

// RedactURL removes the query string and Telegram bot tokens from a URL.
func RedactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "<unparseable url>"
	}
	if u.RawQuery != "" {
		u.RawQuery = "redacted"
	}
	u.User = nil
	u.Path = telegramBotToken.ReplaceAllString(u.Path, "/bot<redacted>")
	u.RawPath = ""
	return u.String()
}

// Patterns for credentials that libraries embed in error messages: a Telegram bot token
// ("<bot id>:<35 chars>", e.g. in gotgbot's "Post https://api.telegram.org/bot<token>/..."
// errors) and secret query parameters.
var (
	botTokenInText    = regexp.MustCompile(`\d{6,}:[A-Za-z0-9_-]{30,}`)
	secretQueryInText = regexp.MustCompile(`(?i)\b(access_token|zapikey|api_key|apikey|token|sig|client_secret|refresh_token)=[^&\s"']+`)
)

// Redact removes Telegram bot tokens and secret query parameters from free text. It is
// applied to every logged string (see logger.SetupLogger) as a safety net, because
// third-party errors include request URLs the call sites don't control.
func Redact(s string) string {
	s = botTokenInText.ReplaceAllString(s, "<redacted-token>")
	return secretQueryInText.ReplaceAllString(s, "$1=<redacted>")
}
