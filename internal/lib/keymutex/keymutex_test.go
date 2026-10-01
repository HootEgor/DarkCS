package keymutex

import (
	"sync"
	"sync/atomic"
	"testing"
)

// Run with -race: holders of the same key must never overlap.
func TestSameKeySerialized(t *testing.T) {
	k := New()
	var inside, maxInside atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock := k.Lock("user")
			n := inside.Add(1)
			for {
				m := maxInside.Load()
				if n <= m || maxInside.CompareAndSwap(m, n) {
					break
				}
			}
			inside.Add(-1)
			unlock()
		}()
	}
	wg.Wait()
	if maxInside.Load() != 1 {
		t.Fatalf("%d goroutines held the same key at once", maxInside.Load())
	}
}

func TestEntriesRemoved(t *testing.T) {
	k := New()
	unlock := k.Lock("a")
	unlock()
	unlock() // second call is a no-op
	if len(k.locks) != 0 {
		t.Fatalf("expected no entries, got %d", len(k.locks))
	}
}

func TestDifferentKeysIndependent(t *testing.T) {
	k := New()
	unlockA := k.Lock("a")
	done := make(chan struct{})
	go func() {
		k.Lock("b")()
		close(done)
	}()
	<-done // would deadlock if keys shared a lock
	unlockA()
}
