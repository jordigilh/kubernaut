/*
Copyright 2026 Jordi Gil.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package mcp

import "sync"

type subscription struct {
	sessionID string
	ch        chan Notification
}

// InMemoryNotificationBus implements NotificationBus with bounded, non-blocking
// delivery. Slow consumers are dropped (channel full = skip), ensuring the
// publisher never blocks. DD-INTERACTIVE-002.
type InMemoryNotificationBus struct {
	mu          sync.RWMutex
	subscribers map[string][]*subscription // correlationID -> subscriptions
	bufferSize  int
}

// NewInMemoryNotificationBus creates a bus with the given per-subscriber buffer.
func NewInMemoryNotificationBus(bufferSize int) *InMemoryNotificationBus {
	if bufferSize <= 0 {
		bufferSize = 10
	}
	return &InMemoryNotificationBus{
		subscribers: make(map[string][]*subscription),
		bufferSize:  bufferSize,
	}
}

// Subscribe creates a buffered channel that receives notifications for the
// given correlationID. The caller must eventually call Unsubscribe.
func (b *InMemoryNotificationBus) Subscribe(correlationID, sessionID string) <-chan Notification {
	ch := make(chan Notification, b.bufferSize)
	sub := &subscription{sessionID: sessionID, ch: ch}

	b.mu.Lock()
	b.subscribers[correlationID] = append(b.subscribers[correlationID], sub)
	b.mu.Unlock()

	return ch
}

// Publish sends a notification to all subscribers of the given correlationID.
// Non-blocking: if a subscriber's channel is full, the notification is dropped.
// Holds RLock during iteration to prevent Unsubscribe from closing channels mid-send.
func (b *InMemoryNotificationBus) Publish(correlationID string, n Notification) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, sub := range b.subscribers[correlationID] {
		select {
		case sub.ch <- n:
		default:
		}
	}
}

// DrainAll closes all subscriber channels and removes all subscriptions.
// Called during shutdown to prevent goroutine leaks.
func (b *InMemoryNotificationBus) DrainAll() {
	b.mu.Lock()
	defer b.mu.Unlock()

	for corrID, subs := range b.subscribers {
		for _, sub := range subs {
			close(sub.ch)
		}
		delete(b.subscribers, corrID)
	}
}

// Unsubscribe removes the subscription and closes its channel.
func (b *InMemoryNotificationBus) Unsubscribe(correlationID, sessionID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[correlationID]
	for i, sub := range subs {
		if sub.sessionID == sessionID {
			close(sub.ch)
			b.subscribers[correlationID] = append(subs[:i], subs[i+1:]...)
			if len(b.subscribers[correlationID]) == 0 {
				delete(b.subscribers, correlationID)
			}
			return
		}
	}
}
