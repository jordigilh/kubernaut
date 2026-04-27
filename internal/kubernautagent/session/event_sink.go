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

package session

import (
	"context"
	"sync"
)

type eventSinkKey struct{}
type sessionIDKey struct{}

// LazySink holds a channel reference that can be set after the context is
// created. This allows Subscribe to attach the event sink lazily while the
// investigation goroutine's context is already in flight.
type LazySink struct {
	mu sync.RWMutex
	ch chan<- InvestigationEvent
}

// Set assigns the channel. Safe for concurrent use.
func (ls *LazySink) Set(ch chan<- InvestigationEvent) {
	ls.mu.Lock()
	ls.ch = ch
	ls.mu.Unlock()
}

// Get returns the channel, or nil if not yet set.
func (ls *LazySink) Get() chan<- InvestigationEvent {
	ls.mu.RLock()
	defer ls.mu.RUnlock()
	return ls.ch
}

// WithLazySink returns a derived context carrying a LazySink.
// The investigator retrieves the current channel via EventSinkFromContext.
func WithLazySink(ctx context.Context, ls *LazySink) context.Context {
	return context.WithValue(ctx, eventSinkKey{}, ls)
}

// WithEventSink returns a derived context carrying a pre-set event channel.
// Retained for backward compatibility with tests that set the sink eagerly.
func WithEventSink(ctx context.Context, ch chan<- InvestigationEvent) context.Context {
	ls := &LazySink{}
	ls.Set(ch)
	return context.WithValue(ctx, eventSinkKey{}, ls)
}

// EventSinkFromContext retrieves the event sink channel from ctx, or nil if
// none was attached (or the lazy sink has not been activated yet).
// Callers must nil-check before sending.
func EventSinkFromContext(ctx context.Context) chan<- InvestigationEvent {
	ls, ok := ctx.Value(eventSinkKey{}).(*LazySink)
	if !ok || ls == nil {
		return nil
	}
	return ls.Get()
}

// WithSessionID returns a derived context carrying the session ID so that
// lower-level code (e.g. the investigator) can cross-reference audit events.
func WithSessionID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, sessionIDKey{}, id)
}

// SessionIDFromContext retrieves the session ID from ctx, or "" if absent.
func SessionIDFromContext(ctx context.Context) string {
	if v, ok := ctx.Value(sessionIDKey{}).(string); ok {
		return v
	}
	return ""
}
