package entity

import "testing"

func TestNormalizePhone(t *testing.T) {
	tests := []struct{ in, want string }{
		{"+380501234567", "+380501234567"},
		{"380501234567", "+380501234567"},
		{"+38 (050) 123-45-67", "+380501234567"},
		{"", ""},
		{"+", ""}, // must not become "+", which matched every phoneless user
		{"abc", ""},
	}
	for _, tt := range tests {
		if got := NormalizePhone(tt.in); got != tt.want {
			t.Errorf("NormalizePhone(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestNewUserNormalizesPhone(t *testing.T) {
	u := NewUser("", "380501234567", 0)
	if u.Phone != "+380501234567" {
		t.Fatalf("phone = %q", u.Phone)
	}
	if u.UUID == "" || u.Role != GuestRole {
		t.Fatalf("unexpected user %+v", u)
	}
	if NewUser("a@b.c", "", 0).Phone != "" {
		t.Fatal("empty phone must stay empty")
	}
}
