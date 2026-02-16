package zoho_functions

import (
	"DarkCS/entity"
	"sync"
	"time"
)

// messageBuffer accumulates ZohoMessageItems per contact ID for batched flushing.
type messageBuffer struct {
	mu   sync.Mutex
	data map[string][]entity.ZohoMessageItem
}

func newMessageBuffer() *messageBuffer {
	return &messageBuffer{
		data: make(map[string][]entity.ZohoMessageItem),
	}
}

// Add appends a message item to the buffer for a given contact.
func (b *messageBuffer) Add(contactID string, item entity.ZohoMessageItem) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.data[contactID] = append(b.data[contactID], item)
}

// Start launches a goroutine that flushes the buffer every 2 minutes.
// On each tick it swaps the buffer contents and calls flushFn for each contact.
func (b *messageBuffer) Start(flushFn func(contactID string, items []entity.ZohoMessageItem)) {
	go func() {
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()

		for range ticker.C {
			b.mu.Lock()
			snapshot := b.data
			b.data = make(map[string][]entity.ZohoMessageItem)
			b.mu.Unlock()

			for contactID, items := range snapshot {
				flushFn(contactID, items)
			}
		}
	}()
}
