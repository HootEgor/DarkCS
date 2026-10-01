package sl

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func TestRedactURL(t *testing.T) {
	tests := []struct{ in, mustNotContain string }{
		{"https://graph.instagram.com/v24.0/me/messages?access_token=SECRET123", "SECRET123"},
		{"https://api.telegram.org/file/bot123456:ABCDEF/photos/file_1.jpg", "ABCDEF"},
		{"https://www.zohoapis.eu/crm/v7/functions/x/actions/execute?auth_type=apikey&zapikey=KEY", "KEY"},
	}
	for _, tt := range tests {
		got := RedactURL(tt.in)
		if strings.Contains(got, tt.mustNotContain) {
			t.Errorf("RedactURL(%q) = %q still contains the secret", tt.in, got)
		}
	}
}

func TestRedactURLError(t *testing.T) {
	err := &url.Error{Op: "Get", URL: "https://x.test/a?access_token=SECRET", Err: errors.New("timeout")}
	got := RedactURLError(err).Error()
	if strings.Contains(got, "SECRET") || !strings.Contains(got, "timeout") {
		t.Fatalf("RedactURLError = %q", got)
	}

	plain := errors.New("plain")
	if RedactURLError(plain) != plain {
		t.Fatal("non-url errors must be returned unchanged")
	}
}

func TestPhoneMasking(t *testing.T) {
	if got := Phone("p", "+380501234567").Value.String(); got != "+380*****4567" {
		t.Errorf("Phone = %q", got)
	}
	if got := Phone("p", "123").Value.String(); got != "***" {
		t.Errorf("short phone = %q", got)
	}
}
