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

package ka

import "sync"

// EventSink receives an event published by the single watcher attached to a
// pooled KA session. Sinks own their delivery context and may route events to
// an A2A EventBridge or another in-process consumer.
type EventSink func(InvestigationEvent)

// EventRouter fans events from one pooled KA session out to its active
// subscribers. KASessionPool owns one router per (rr_id, username) entry,
// preserving the pool's existing user-isolation boundary without storing
// request contexts or allowing multiple goroutines to consume the KA channel.
//
// Delivery is best-effort and in-process. Publish snapshots subscribers under
// the mutex and invokes sinks after unlocking so a sink cannot block router
// lifecycle operations or deadlock Subscribe/Unsubscribe.
type EventRouter struct {
	mu          sync.RWMutex
	nextID      uint64
	subscribers map[uint64]EventSink
}

// NewEventRouter creates an empty session event router.
func NewEventRouter() *EventRouter {
	return &EventRouter{subscribers: make(map[uint64]EventSink)}
}

// Subscribe registers a sink and returns a token-specific unsubscribe
// function. Calling the returned function more than once is safe.
func (r *EventRouter) Subscribe(sink EventSink) (unsubscribe func()) {
	if r == nil || sink == nil {
		return func() {}
	}

	r.mu.Lock()
	r.nextID++
	id := r.nextID
	if r.subscribers == nil {
		r.subscribers = make(map[uint64]EventSink)
	}
	r.subscribers[id] = sink
	r.mu.Unlock()

	return func() {
		r.mu.Lock()
		delete(r.subscribers, id)
		r.mu.Unlock()
	}
}

// Publish delivers evt to all subscribers that were registered when the
// publish began. It returns the number of sinks selected for delivery.
func (r *EventRouter) Publish(evt InvestigationEvent) int {
	if r == nil {
		return 0
	}

	r.mu.RLock()
	sinks := make([]EventSink, 0, len(r.subscribers))
	for _, sink := range r.subscribers {
		sinks = append(sinks, sink)
	}
	r.mu.RUnlock()

	for _, sink := range sinks {
		sink(evt)
	}
	return len(sinks)
}
