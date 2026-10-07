package core

import "testing"

func TestGroupTopicName(t *testing.T) {
	tests := []struct{ known, channel, want string }{
		{"Замовлення", "42", "Замовлення"},
		{"", "42", "Тема #42"},
		{"", "1", "General"},
		{"", "", ""},
	}
	for _, tt := range tests {
		if got := groupTopicName(tt.known, tt.channel); got != tt.want {
			t.Fatalf("groupTopicName(%q, %q) = %q, want %q", tt.known, tt.channel, got, tt.want)
		}
	}
}
