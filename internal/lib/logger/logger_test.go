package logger

import (
	"log/slog"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestParseLevel(t *testing.T) {
	tests := []struct {
		in   string
		want slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"WARN", slog.LevelWarn},
		{" error ", slog.LevelError},
		{"", slog.LevelInfo},        // default
		{"verbose", slog.LevelInfo}, // unknown → default
	}
	for _, tt := range tests {
		if got := ParseLevel(tt.in, slog.LevelInfo); got != tt.want {
			t.Errorf("ParseLevel(%q) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestTruncateKeepsRunesValid(t *testing.T) {
	s := strings.Repeat("ї", 3000) // 2 bytes per rune
	got := truncate(s, 3901)
	if !utf8.ValidString(got) {
		t.Fatal("truncated inside a rune")
	}
	if len(got) > 3901+len("…") {
		t.Fatalf("too long: %d bytes", len(got))
	}
}
