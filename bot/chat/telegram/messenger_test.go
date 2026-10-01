package telegram

import (
	"strings"
	"testing"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
)

// fakeAPI records the options of the last calls.
type fakeAPI struct {
	TelegramAPI
	msgOpts    *tgbotapi.SendMessageOpts
	actionOpts *tgbotapi.SendChatActionOpts
	sent       int64
}

func (f *fakeAPI) SendMessage(_ int64, _ string, opts *tgbotapi.SendMessageOpts) (*tgbotapi.Message, error) {
	f.msgOpts = opts
	f.sent++
	return &tgbotapi.Message{MessageId: 100 + f.sent}, nil
}

func TestSendTextReturningIDReturnsFirstChunk(t *testing.T) {
	api := &fakeAPI{}
	m := NewBusinessMessenger(api, "conn-1")

	long := strings.Repeat("a", 5000) // split into two Telegram messages
	id, err := m.SendTextReturningID("42", long)
	if err != nil {
		t.Fatal(err)
	}
	if api.sent != 2 || id != 101 {
		t.Fatalf("sent %d messages, id %d; want 2 messages, id 101", api.sent, id)
	}
}

func (f *fakeAPI) SendChatAction(_ int64, _ string, opts *tgbotapi.SendChatActionOpts) (bool, error) {
	f.actionOpts = opts
	return true, nil
}

func TestBusinessMessengerSetsConnection(t *testing.T) {
	api := &fakeAPI{}
	m := NewBusinessMessenger(api, "conn-1")

	if err := m.SendText("42", "hi"); err != nil {
		t.Fatal(err)
	}
	if api.msgOpts == nil || api.msgOpts.BusinessConnectionId != "conn-1" || api.msgOpts.ParseMode != "HTML" {
		t.Fatalf("SendMessage opts = %+v", api.msgOpts)
	}

	if err := m.SendTyping("42"); err != nil {
		t.Fatal(err)
	}
	if api.actionOpts == nil || api.actionOpts.BusinessConnectionId != "conn-1" {
		t.Fatalf("SendChatAction opts = %+v", api.actionOpts)
	}
}

func TestBotMessengerHasNoConnection(t *testing.T) {
	api := &fakeAPI{}
	m := NewMessenger(api)

	if err := m.SendText("42", "hi"); err != nil {
		t.Fatal(err)
	}
	if api.msgOpts == nil || api.msgOpts.BusinessConnectionId != "" {
		t.Fatalf("SendMessage opts = %+v", api.msgOpts)
	}
	if err := m.SendTyping("42"); err != nil {
		t.Fatal(err)
	}
	if api.actionOpts != nil {
		t.Fatalf("bot typing must keep nil opts, got %+v", api.actionOpts)
	}
}
