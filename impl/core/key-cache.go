package core

import (
	"sync"
	"time"
)

// keyCacheTTL bounds how long a key deleted from the database keeps working.
const keyCacheTTL = 5 * time.Minute

// keyCache maps API tokens to usernames. It is read on every authenticated request and
// written on first sight of a token, so it must be locked: an unsynchronized map write
// here is a fatal runtime error that kills the process.
type keyCache struct {
	mu sync.RWMutex
	m  map[string]cachedKey
}

type cachedKey struct {
	username string
	expires  time.Time
}

func newKeyCache() *keyCache {
	return &keyCache{m: make(map[string]cachedKey)}
}

func (c *keyCache) get(token string) (string, bool) {
	c.mu.RLock()
	k, ok := c.m[token]
	c.mu.RUnlock()
	if !ok || time.Now().After(k.expires) {
		return "", false
	}
	return k.username, true
}

func (c *keyCache) set(token, username string) {
	now := time.Now()
	c.mu.Lock()
	defer c.mu.Unlock()
	// Drop expired entries opportunistically so rejected or rotated keys don't accumulate.
	for t, k := range c.m {
		if now.After(k.expires) {
			delete(c.m, t)
		}
	}
	c.m[token] = cachedKey{username: username, expires: now.Add(keyCacheTTL)}
}
