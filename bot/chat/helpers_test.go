package chat

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSplitTextShort(t *testing.T) {
	if got := SplitText("hello", 10); len(got) != 1 || got[0] != "hello" {
		t.Fatalf("got %q", got)
	}
}

func TestSplitTextRespectsLimitAndKeepsContent(t *testing.T) {
	text := strings.Repeat("Привіт світ\n", 300) // multi-byte runes
	chunks := SplitText(text, 100)
	if len(chunks) < 2 {
		t.Fatalf("expected several chunks, got %d", len(chunks))
	}
	for _, c := range chunks {
		if n := utf8.RuneCountInString(c); n > 100 {
			t.Fatalf("chunk of %d runes exceeds limit", n)
		}
		if !utf8.ValidString(c) {
			t.Fatal("chunk split inside a rune")
		}
	}
	joined := strings.Join(chunks, "\n")
	if strings.Count(joined, "Привіт світ") != 300 {
		t.Fatal("content lost or duplicated")
	}
}

func TestSplitTextLongWord(t *testing.T) {
	chunks := SplitText(strings.Repeat("x", 250), 100)
	if len(chunks) != 3 {
		t.Fatalf("got %d chunks", len(chunks))
	}
}
