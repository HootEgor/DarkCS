package zoho_functions

import (
	"DarkCS/entity"
	"context"
	"sync"
	"time"
)

const (
	flushInterval = 2 * time.Minute
	// maxBufferedPerContact bounds memory when Zoho is down for a long time; the oldest
	// messages are dropped first.
	maxBufferedPerContact = 500
)

// messageBuffer accumulates ZohoMessageItems per contact ID for batched flushing.
// Failed flushes are re-queued so a Zoho outage doesn't lose CRM chat history, and Stop
// performs a final flush so a restart doesn't either.
type messageBuffer struct {
	mu   sync.Mutex
	data map[string][]entity.ZohoMessageItem

	stop chan struct{}
	done chan struct{}
}

func newMessageBuffer() *messageBuffer {
	return &messageBuffer{
		data: make(map[string][]entity.ZohoMessageItem),
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}
}

// Add appends a message item to the buffer for a given contact.
func (b *messageBuffer) Add(contactID string, item entity.ZohoMessageItem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[contactID] = capItems(append(b.data[contactID], item))
}

// requeue puts failed items back in front of anything buffered since the flush started.
func (b *messageBuffer) requeue(contactID string, items []entity.ZohoMessageItem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[contactID] = capItems(append(append([]entity.ZohoMessageItem(nil), items...), b.data[contactID]...))
}

func capItems(items []entity.ZohoMessageItem) []entity.ZohoMessageItem {
	if len(items) > maxBufferedPerContact {
		return items[len(items)-maxBufferedPerContact:]
	}
	return items
}

// Start launches a goroutine that flushes the buffer every flushInterval until Stop.
// flushFn returns an error to have the items re-queued for the next flush.
func (b *messageBuffer) Start(flushFn func(contactID string, items []entity.ZohoMessageItem) error) {
	go func() {
		defer close(b.done)
		ticker := time.NewTicker(flushInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				b.flush(flushFn)
			case <-b.stop:
				b.flush(flushFn)
				return
			}
		}
	}()
}

func (b *messageBuffer) flush(flushFn func(contactID string, items []entity.ZohoMessageItem) error) {
	b.mu.Lock()
	snapshot := b.data
	b.data = make(map[string][]entity.ZohoMessageItem)
	b.mu.Unlock()

	for contactID, items := range snapshot {
		if err := flushFn(contactID, items); err != nil {
			b.requeue(contactID, items)
		}
	}
}

// Stop triggers a final flush and waits for it, or until ctx expires.
func (b *messageBuffer) Stop(ctx context.Context) {
	select {
	case <-b.stop:
	default:
		close(b.stop)
	}
	select {
	case <-b.done:
	case <-ctx.Done():
	}
}
