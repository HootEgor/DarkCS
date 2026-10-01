package ratelimit

import (
	"testing"
	"time"
)

func TestLimit(t *testing.T) {
	l := New(2, time.Minute)
	if !l.Allow("u") || !l.Allow("u") {
		t.Fatal("first two events must pass")
	}
	if l.Allow("u") {
		t.Fatal("third event in the window must be rejected")
	}
	if !l.Allow("other") {
		t.Fatal("keys are independent")
	}
}

func TestWindowSlides(t *testing.T) {
	l := New(1, 10*time.Millisecond)
	l.Allow("u")
	time.Sleep(20 * time.Millisecond)
	if !l.Allow("u") {
		t.Fatal("event must pass after the window")
	}
}

func TestDisabled(t *testing.T) {
	var nilLimiter *Limiter
	if !nilLimiter.Allow("u") || !New(0, time.Minute).Allow("u") {
		t.Fatal("nil or zero limiter must allow everything")
	}
}
