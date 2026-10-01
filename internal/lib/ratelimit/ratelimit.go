// Package ratelimit is a small per-key sliding-window limiter (N events per window).
package ratelimit

import (
	"sync"
	"time"
)

type Limiter struct {
	mu     sync.Mutex
	limit  int
	window time.Duration
	events map[string][]time.Time
	sweep  time.Time
}

// New allows limit events per window per key; limit <= 0 disables limiting.
func New(limit int, window time.Duration) *Limiter {
	return &Limiter{limit: limit, window: window, events: make(map[string][]time.Time)}
}

// Allow records an event for key and reports whether it is within the limit.
func (l *Limiter) Allow(key string) bool {
	if l == nil || l.limit <= 0 {
		return true
	}
	now := time.Now()
	cutoff := now.Add(-l.window)

	l.mu.Lock()
	defer l.mu.Unlock()

	// Forget idle keys now and then so memory tracks active users only.
	if now.Sub(l.sweep) > l.window {
		for k, ev := range l.events {
			if len(ev) == 0 || ev[len(ev)-1].Before(cutoff) {
				delete(l.events, k)
			}
		}
		l.sweep = now
	}

	ev := l.events[key]
	i := 0
	for i < len(ev) && ev[i].Before(cutoff) {
		i++
	}
	ev = ev[i:]
	if len(ev) >= l.limit {
		l.events[key] = ev
		return false
	}
	l.events[key] = append(ev, now)
	return true
}
