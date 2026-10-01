// Package tgexport parses a single-chat "Machine-readable JSON" export from Telegram
// Desktop (result.json). The Bot API cannot read chat history, so managers import the
// history of Telegram Business chats that predates the bot connection from such exports.
//
// Only text is imported; media becomes a placeholder like "[Фото]" because exports are
// usually made without files (and the JSON alone does not carry them).
package tgexport

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// Validation errors. Their text is shown to the CRM manager as is.
var (
	ErrInvalidJSON     = errors.New("file is not a Telegram Desktop JSON export (result.json)")
	ErrNotPersonalChat = errors.New("export is not a personal chat; export a single private chat")
	ErrWrongChat       = errors.New("export belongs to a different chat")
	ErrWrongAccount    = errors.New("export was made from a different Telegram account; log in as the premium account owner")
)

// Message is one imported chat message.
type Message struct {
	ID        int64
	Time      time.Time
	FromOwner bool // written by the business account owner (shown as a manager message)
	Text      string
}

type export struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	ID       int64           `json:"id"`
	Messages []exportMessage `json:"messages"`
}

type exportMessage struct {
	ID           int64           `json:"id"`
	Type         string          `json:"type"`
	Date         string          `json:"date"`
	DateUnix     string          `json:"date_unixtime"`
	FromID       string          `json:"from_id"`
	Text         json.RawMessage `json:"text"`
	TextEntities []struct {
		Text string `json:"text"`
	} `json:"text_entities"`
	Photo        string          `json:"photo"`
	File         string          `json:"file"`
	FileName     string          `json:"file_name"`
	MediaType    string          `json:"media_type"`
	StickerEmoji string          `json:"sticker_emoji"`
	Contact      json.RawMessage `json:"contact_information"`
	Location     json.RawMessage `json:"location_information"`
	Poll         json.RawMessage `json:"poll"`
}

// Parse reads an export and returns its messages oldest first. The export must be the
// private chat with clientID as seen by the account ownerID: any other chat or account
// is rejected so history never lands in the wrong CRM chat.
func Parse(r io.Reader, clientID, ownerID int64) ([]Message, error) {
	var exp export
	if err := json.NewDecoder(r).Decode(&exp); err != nil {
		return nil, ErrInvalidJSON
	}
	if exp.Type == "" {
		return nil, ErrInvalidJSON
	}
	if exp.Type != "personal_chat" {
		return nil, ErrNotPersonalChat
	}
	if exp.ID != clientID {
		return nil, ErrWrongChat
	}

	clientFrom := "user" + strconv.FormatInt(clientID, 10)
	ownerFrom := "user" + strconv.FormatInt(ownerID, 10)

	out := make([]Message, 0, len(exp.Messages))
	for _, m := range exp.Messages {
		if m.Type != "message" {
			continue // service entries: calls, pins, history clears...
		}
		switch m.FromID {
		case clientFrom, ownerFrom:
		default:
			return nil, ErrWrongAccount
		}

		t, err := messageTime(m)
		if err != nil {
			return nil, fmt.Errorf("message %d: %w", m.ID, err)
		}

		text := joinNonEmpty(mediaPlaceholder(m), messageText(m))
		if text == "" {
			continue
		}
		out = append(out, Message{ID: m.ID, Time: t, FromOwner: m.FromID == ownerFrom, Text: text})
	}
	return out, nil
}

// messageTime prefers date_unixtime; older exports only have "date" in the exporting
// computer's local time.
func messageTime(m exportMessage) (time.Time, error) {
	if m.DateUnix != "" {
		sec, err := strconv.ParseInt(m.DateUnix, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("bad date_unixtime %q", m.DateUnix)
		}
		return time.Unix(sec, 0), nil
	}
	t, err := time.ParseInLocation("2006-01-02T15:04:05", m.Date, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("bad date %q", m.Date)
	}
	return t, nil
}

// messageText flattens the message text. "text" is either a string or an array of
// strings and entity objects; text_entities is the normalized form of the same.
func messageText(m exportMessage) string {
	if len(m.TextEntities) > 0 {
		var b strings.Builder
		for _, e := range m.TextEntities {
			b.WriteString(e.Text)
		}
		return strings.TrimSpace(b.String())
	}
	if len(m.Text) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(m.Text, &s) == nil {
		return strings.TrimSpace(s)
	}
	var parts []json.RawMessage
	if json.Unmarshal(m.Text, &parts) != nil {
		return ""
	}
	var b strings.Builder
	for _, p := range parts {
		var str string
		if json.Unmarshal(p, &str) == nil {
			b.WriteString(str)
			continue
		}
		var ent struct {
			Text string `json:"text"`
		}
		if json.Unmarshal(p, &ent) == nil {
			b.WriteString(ent.Text)
		}
	}
	return strings.TrimSpace(b.String())
}

// mediaPlaceholder describes attached media, or returns "" for plain text.
func mediaPlaceholder(m exportMessage) string {
	switch {
	case m.Photo != "":
		return "[Фото]"
	case m.MediaType == "voice_message":
		return "[Голосове повідомлення]"
	case m.MediaType == "video_message":
		return "[Відеоповідомлення]"
	case m.MediaType == "video_file":
		return "[Відео]"
	case m.MediaType == "animation":
		return "[GIF]"
	case m.MediaType == "sticker":
		return strings.TrimSpace("[Стікер] " + m.StickerEmoji)
	case m.MediaType == "audio_file":
		return withName("[Аудіо", m.FileName)
	case m.File != "" || m.FileName != "":
		return withName("[Файл", m.FileName)
	case len(m.Contact) > 0:
		return "[Контакт]"
	case len(m.Location) > 0:
		return "[Локація]"
	case len(m.Poll) > 0:
		return "[Опитування]"
	}
	return ""
}

func withName(prefix, name string) string {
	if name == "" {
		return prefix + "]"
	}
	return prefix + ": " + name + "]"
}

func joinNonEmpty(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "\n" + b
}
