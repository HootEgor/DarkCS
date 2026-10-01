package tgexport

import (
	"errors"
	"strings"
	"testing"
	"time"
)

const (
	client = int64(111)
	owner  = int64(999)
)

const fixture = `{
 "name": "Client",
 "type": "personal_chat",
 "id": 111,
 "messages": [
  {"id": 1, "type": "service", "date": "2024-01-01T10:00:00", "date_unixtime": "1704103200", "actor_id": "user999", "action": "phone_call", "text": ""},
  {"id": 2, "type": "message", "date_unixtime": "1704103300", "from_id": "user111", "text": "Hello", "text_entities": [{"type": "plain", "text": "Hello"}]},
  {"id": 3, "type": "message", "date_unixtime": "1704103400", "from_id": "user999", "text": ["Price is ", {"type": "bold", "text": "100"}]},
  {"id": 4, "type": "message", "date_unixtime": "1704103500", "from_id": "user111", "photo": "(File not included. Change data exporting settings to download.)", "text": "see this"},
  {"id": 5, "type": "message", "date_unixtime": "1704103600", "from_id": "user999", "file": "(File not included.)", "file_name": "invoice.pdf", "text": ""},
  {"id": 6, "type": "message", "date_unixtime": "1704103700", "from_id": "user111", "media_type": "voice_message", "file": "(File not included.)", "text": ""},
  {"id": 7, "type": "message", "date_unixtime": "1704103800", "from_id": "user111", "text": ""}
 ]
}`

func TestParse(t *testing.T) {
	msgs, err := Parse(strings.NewReader(fixture), client, owner)
	if err != nil {
		t.Fatal(err)
	}

	want := []Message{
		{ID: 2, Time: time.Unix(1704103300, 0), FromOwner: false, Text: "Hello"},
		{ID: 3, Time: time.Unix(1704103400, 0), FromOwner: true, Text: "Price is 100"},
		{ID: 4, Time: time.Unix(1704103500, 0), FromOwner: false, Text: "[Фото]\nsee this"},
		{ID: 5, Time: time.Unix(1704103600, 0), FromOwner: true, Text: "[Файл: invoice.pdf]"},
		{ID: 6, Time: time.Unix(1704103700, 0), FromOwner: false, Text: "[Голосове повідомлення]"},
	}
	if len(msgs) != len(want) {
		t.Fatalf("got %d messages, want %d: %+v", len(msgs), len(want), msgs)
	}
	for i := range want {
		if msgs[i].ID != want[i].ID || !msgs[i].Time.Equal(want[i].Time) ||
			msgs[i].FromOwner != want[i].FromOwner || msgs[i].Text != want[i].Text {
			t.Errorf("message %d = %+v, want %+v", i, msgs[i], want[i])
		}
	}
}

func TestParseRejects(t *testing.T) {
	tests := []struct {
		name string
		json string
		want error
	}{
		{"not json", `not json`, ErrInvalidJSON},
		{"no type", `{"foo": 1}`, ErrInvalidJSON},
		{"group", `{"type": "private_group", "id": 111, "messages": []}`, ErrNotPersonalChat},
		{"other chat", `{"type": "personal_chat", "id": 222, "messages": []}`, ErrWrongChat},
		{"other account", `{"type": "personal_chat", "id": 111, "messages": [
			{"id": 1, "type": "message", "date_unixtime": "1704103300", "from_id": "user555", "text": "hi"}]}`, ErrWrongAccount},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.json), client, owner)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestParseLegacyDate(t *testing.T) {
	js := `{"type": "personal_chat", "id": 111, "messages": [
		{"id": 1, "type": "message", "date": "2024-01-01T10:00:00", "from_id": "user111", "text": "hi"}]}`
	msgs, err := Parse(strings.NewReader(js), client, owner)
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2024, 1, 1, 10, 0, 0, 0, time.Local)
	if len(msgs) != 1 || !msgs[0].Time.Equal(want) {
		t.Fatalf("got %+v, want time %v", msgs, want)
	}
}
