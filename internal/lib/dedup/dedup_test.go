package dedup

import (
	"testing"
	"time"
)

func TestFirstSeen(t *testing.T) {
	s := New(time.Minute)
	if !s.FirstSeen("m1") {
		t.Fatal("first delivery must be new")
	}
	if s.FirstSeen("m1") {
		t.Fatal("redelivery must be detected")
	}
	if !s.FirstSeen("m2") {
		t.Fatal("other ids are independent")
	}
	if !s.FirstSeen("") || !s.FirstSeen("") {
		t.Fatal("empty id is never deduplicated")
	}
}

func TestFirstSeenExpires(t *testing.T) {
	s := New(10 * time.Millisecond)
	s.FirstSeen("m1")
	time.Sleep(20 * time.Millisecond)
	if !s.FirstSeen("m1") {
		t.Fatal("id must be forgotten after ttl")
	}
}
