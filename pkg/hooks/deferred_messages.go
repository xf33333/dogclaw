package hooks

import (
	"sync"
	"time"
)

// DeferredMessage represents a message that should be processed later
// Translated from useDeferredHookMessages.ts
type DeferredMessage struct {
	ID        string
	Message   string
	Timestamp time.Time
	Priority  int
	Callback  func()
}

// DeferredMessageQueue manages deferred hook messages
type DeferredMessageQueue struct {
	mu       sync.Mutex
	messages []DeferredMessage
}

// NewDeferredMessageQueue creates a new deferred message queue
func NewDeferredMessageQueue() *DeferredMessageQueue {
	return &DeferredMessageQueue{
		messages: make([]DeferredMessage, 0),
	}
}

// Add adds a deferred message to the queue
func (dq *DeferredMessageQueue) Add(msg DeferredMessage) {
	dq.mu.Lock()
	defer dq.mu.Unlock()
	msg.Timestamp = time.Now()
	dq.messages = append(dq.messages, msg)
}

// Process processes all deferred messages in order
func (dq *DeferredMessageQueue) Process() {
	dq.mu.Lock()
	msgs := make([]DeferredMessage, len(dq.messages))
	copy(msgs, dq.messages)
	dq.messages = nil
	dq.mu.Unlock()

	for _, msg := range msgs {
		if msg.Callback != nil {
			msg.Callback()
		}
	}
}

// Clear removes all pending messages
func (dq *DeferredMessageQueue) Clear() {
	dq.mu.Lock()
	defer dq.mu.Unlock()
	dq.messages = nil
}

// Len returns the number of pending messages
func (dq *DeferredMessageQueue) Len() int {
	dq.mu.Lock()
	defer dq.mu.Unlock()
	return len(dq.messages)
}
