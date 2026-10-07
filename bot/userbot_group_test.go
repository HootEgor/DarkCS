package bot

import (
	"testing"

	tgbotapi "github.com/PaulSonOfLars/gotgbot/v2"
)

func TestGroupChannel(t *testing.T) {
	tests := []struct {
		name           string
		isForum, topic bool
		threadID       int64
		want           string
	}{
		{"forum topic", true, true, 42, "42"},
		{"forum general topic", true, false, 0, "1"},
		{"plain group", false, false, 0, ""},
		{"reply thread in plain supergroup", false, false, 99, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := groupChannel(tt.isForum, tt.topic, tt.threadID); got != tt.want {
				t.Fatalf("groupChannel = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestMessageAuthor(t *testing.T) {
	member := &tgbotapi.Message{From: &tgbotapi.User{Id: 7, FirstName: "Олена", LastName: "К", Username: "olena"}}
	if id, name, username := messageAuthor(member); id != 7 || name != "Олена К" || username != "olena" {
		t.Fatalf("member author = (%d, %q, %q)", id, name, username)
	}

	// Anonymous admins come from GroupAnonymousBot with the group as sender_chat.
	anon := &tgbotapi.Message{
		From:       &tgbotapi.User{Id: 1087968824, FirstName: "Group"},
		SenderChat: &tgbotapi.Chat{Id: -100500, Title: "Dealers"},
	}
	if id, name, _ := messageAuthor(anon); id != -100500 || name != "Dealers" {
		t.Fatalf("anonymous author = (%d, %q)", id, name)
	}
}

func TestBotMembership(t *testing.T) {
	tests := []struct {
		name             string
		member           tgbotapi.MergedChatMember
		active, canWrite bool
	}{
		{"admin", tgbotapi.MergedChatMember{Status: "administrator"}, true, true},
		{"member", tgbotapi.MergedChatMember{Status: "member"}, true, true},
		{"muted", tgbotapi.MergedChatMember{Status: "restricted", IsMember: true}, true, false},
		{"restricted can write", tgbotapi.MergedChatMember{Status: "restricted", IsMember: true, CanSendMessages: true}, true, true},
		{"kicked", tgbotapi.MergedChatMember{Status: "kicked"}, false, false},
		{"left", tgbotapi.MergedChatMember{Status: "left"}, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isActiveMember(tt.member); got != tt.active {
				t.Fatalf("isActiveMember = %v, want %v", got, tt.active)
			}
			if got := canPost(tt.member); got != tt.canWrite {
				t.Fatalf("canPost = %v, want %v", got, tt.canWrite)
			}
		})
	}
}

func TestTopicNameFromReply(t *testing.T) {
	created := &tgbotapi.Message{MessageId: 42, ForumTopicCreated: &tgbotapi.ForumTopicCreated{Name: "Замовлення"}}
	tests := []struct {
		name     string
		msg      *tgbotapi.Message
		want     string
		wantSeen bool
	}{
		{"ordinary topic message", &tgbotapi.Message{IsTopicMessage: true, MessageThreadId: 42, ReplyToMessage: created}, "Замовлення", true},
		{"explicit reply", &tgbotapi.Message{IsTopicMessage: true, MessageThreadId: 42, ReplyToMessage: &tgbotapi.Message{MessageId: 50}}, "", false},
		{"other topic's creation", &tgbotapi.Message{IsTopicMessage: true, MessageThreadId: 7, ReplyToMessage: created}, "", false},
		{"not a topic message", &tgbotapi.Message{MessageThreadId: 42, ReplyToMessage: created}, "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := topicNameFromReply(tt.msg)
			if got != tt.want || ok != tt.wantSeen {
				t.Fatalf("topicNameFromReply = (%q, %v), want (%q, %v)", got, ok, tt.want, tt.wantSeen)
			}
		})
	}
}
