// Package keymutex provides one mutex per key, created on demand and removed when no
// goroutine holds or waits for it, so memory stays proportional to active keys.
package keymutex

import "sync"

type KeyMutex struct {
	mu    sync.Mutex
	locks map[string]*entry
}

type entry struct {
	mu   sync.Mutex
	refs int // holders + waiters; the entry is deleted when it drops to zero
}

func New() *KeyMutex {
	return &KeyMutex{locks: make(map[string]*entry)}
}

// Lock blocks until the key is free and returns the function that releases it.
func (k *KeyMutex) Lock(key string) (unlock func()) {
	k.mu.Lock()
	e, ok := k.locks[key]
	if !ok {
		e = &entry{}
		k.locks[key] = e
	}
	e.refs++
	k.mu.Unlock()

	e.mu.Lock()

	var once sync.Once
	return func() {
		once.Do(func() {
			e.mu.Unlock()
			k.mu.Lock()
			e.refs--
			if e.refs == 0 {
				delete(k.locks, key)
			}
			k.mu.Unlock()
		})
	}
}
