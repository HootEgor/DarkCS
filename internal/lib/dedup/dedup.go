// Package dedup remembers recently seen IDs. Meta re-delivers webhooks when it doesn't
// get a fast acknowledgement, and processing a message twice duplicates CRM entries and
// repeats side effects such as registration or ratings.
package dedup

import (
	"sync"
	"time"
)

// Set is a concurrency-safe set of IDs that forgets entries after ttl.
type Set struct {
	mu        sync.Mutex
	ttl       time.Duration
	seen      map[string]time.Time
	lastSweep time.Time
}

func New(ttl time.Duration) *Set {
	return &Set{ttl: ttl, seen: make(map[string]time.Time)}
}

// FirstSeen records id and reports whether it was not seen within ttl.
// An empty id is always treated as new.
func (s *Set) FirstSeen(id string) bool {
	if id == "" {
		return true
	}
	now := time.Now()

	s.mu.Lock()
	defer s.mu.Unlock()

	if now.Sub(s.lastSweep) > s.ttl {
		for k, t := range s.seen {
			if now.Sub(t) > s.ttl {
				delete(s.seen, k)
			}
		}
		s.lastSweep = now
	}

	if t, ok := s.seen[id]; ok && now.Sub(t) <= s.ttl {
		return false
	}
	s.seen[id] = now
	return true
}
